//go:build windows

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/ersinkoc/WindowsTaskManager/frontend"
	"github.com/ersinkoc/WindowsTaskManager/internal/ai"
	"github.com/ersinkoc/WindowsTaskManager/internal/anomaly"
	"github.com/ersinkoc/WindowsTaskManager/internal/collector"
	"github.com/ersinkoc/WindowsTaskManager/internal/config"
	"github.com/ersinkoc/WindowsTaskManager/internal/controller"
	"github.com/ersinkoc/WindowsTaskManager/internal/desktop"
	"github.com/ersinkoc/WindowsTaskManager/internal/event"
	"github.com/ersinkoc/WindowsTaskManager/internal/platform"
	"github.com/ersinkoc/WindowsTaskManager/internal/server"
	"github.com/ersinkoc/WindowsTaskManager/internal/storage"
	"github.com/ersinkoc/WindowsTaskManager/internal/telegram"
	"github.com/ersinkoc/WindowsTaskManager/internal/tray"
	"github.com/ersinkoc/WindowsTaskManager/internal/winapi"
)

// version is injected at build time via -ldflags "-X main.version=X.Y.Z".
// Stays as "dev" for plain `go build`.
var version = "dev"

func main() {
	var (
		configPath  = flag.String("config", "", "config file path (default %APPDATA%\\WindowsTaskManager\\config.yaml)")
		showVersion = flag.Bool("version", false, "print version and exit")
		noTray      = flag.Bool("no-tray", false, "disable system tray icon")
		noBrowser   = flag.Bool("no-browser", false, "do not open dashboard in browser")
		noWindow    = flag.Bool("no-window", false, "server-only mode: do not show desktop window (equivalent to --no-browser but keeps API server running)")
	)
	flag.Parse()

	if *showVersion {
		fmt.Printf("Windows Task Manager %s\n", version)
		return
	}

	cfgPath := resolveConfigPath(*configPath)
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	log.Printf("loading config from %s", cfgPath)

	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	releaseInstance, err := platform.AcquireSingleInstance(`Local\WindowsTaskManager.Singleton`)
	if err == platform.ErrAlreadyRunning {
		dashURL := fmt.Sprintf("http://%s:%d", cfg.Server.Host, cfg.Server.Port)
		log.Printf("another instance is already running; refusing to start a second copy")
		if !*noBrowser {
			_ = winapi.ShellExecute("open", dashURL, "", "", winapi.SW_SHOWNORMAL)
		}
		return
	}
	if err != nil {
		log.Fatalf("single-instance guard: %v", err)
	}
	defer releaseInstance()

	// Core wiring.
	emitter := event.NewEmitter()
	historyCap := int(cfg.Monitoring.HistoryDuration / cfg.Monitoring.Interval)
	if historyCap < 60 {
		historyCap = 60
	}
	store := storage.NewStore(historyCap, 60)

	cpuName, cpuMHz := collector.CPUInfoFromRegistry()
	mgr := collector.NewManager(cfg, store, emitter, cpuName, cpuMHz)
	ctrl := controller.NewController(cfg, store, emitter)
	alerts := anomaly.NewAlertStore(256)
	alerts.SetMaxActive(cfg.Anomaly.MaxActiveAlerts)
	engine := anomaly.NewEngine(cfg, store, emitter, alerts)
	engine.SetActuator(ctrl) // rules engine can kill/suspend via the controller
	advisor := ai.NewAdvisor(cfg, store, alerts.Active, emitter)
	var srv *server.Server
	tgBot := telegram.New(cfg, store, alerts, ctrl, advisor, func(suggestion ai.Suggestion) error {
		if srv == nil {
			return fmt.Errorf("server not initialized")
		}
		return srv.ExecuteAISuggestion(suggestion, true)
	}, emitter)
	var trayInst *tray.Tray

	// Prime first snapshot before serving HTTP.
	mgr.CollectOnce()

	// applyConfig fans a new config out to every long-lived component.
	// It is called both by the file watcher and by the in-process AI
	// config update endpoint.
	applyConfig := func(newCfg *config.Config) {
		mgr.ApplyConfig(newCfg)
		ctrl.SetConfig(newCfg)
		engine.SetConfig(newCfg)
		advisor.SetConfig(newCfg)
		tgBot.SetConfig(newCfg)
		if trayInst != nil {
			trayInst.SetConfig(newCfg)
		}
		alerts.SetMaxActive(newCfg.Anomaly.MaxActiveAlerts)
		// Purge active alerts whose detector is now disabled — otherwise the
		// UI keeps showing stale entries that will never refresh or clear.
		clearDisabledDetectorAlerts(alerts, newCfg)
	}
	// Apply once right after startup so a fresh load (including schema
	// migration that flipped a detector off) immediately drops stale alerts
	// from any previous run's active set.
	clearDisabledDetectorAlerts(alerts, cfg)

	srv, err = server.New(server.Options{
		Cfg:        cfg,
		CfgPath:    cfgPath,
		OnCfgApply: applyConfig,
		Store:      store,
		Controller: ctrl,
		Alerts:     alerts,
		Emitter:    emitter,
		Advisor:    advisor,
		StaticFS:   frontend.FS(),
		Version:    version,
	})
	if err != nil {
		log.Fatalf("initialize server: %v", err)
	}

	rootCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	advisor.Start(rootCtx)
	mgr.Start(rootCtx)
	engine.Start(rootCtx)
	tgBot.Start(rootCtx)

	// Config hot-reload watcher.
	watcher := config.NewWatcher(cfgPath, func(newCfg *config.Config) {
		log.Printf("config reloaded")
		applyConfig(newCfg)
		srv.SetConfig(newCfg)
	})
	go watcher.Start(rootCtx)

	// HTTP server in its own goroutine.
	srvErr := make(chan error, 1)
	go func() {
		srvErr <- srv.Start()
	}()

	dashURL := fmt.Sprintf("http://%s:%d", cfg.Server.Host, cfg.Server.Port)

	// Determine what UI to show: desktop window (WebView2), browser, or headless.
	showDesktop := !*noWindow && cfg.Server.OpenBrowser
	showBrowser := !showDesktop && cfg.Server.OpenBrowser && !*noBrowser

	if showBrowser {
		go func() {
			time.Sleep(400 * time.Millisecond)
			_ = winapi.ShellExecute("open", dashURL, "", "", winapi.SW_SHOWNORMAL)
		}()
	}

	var trayWG sync.WaitGroup
	if !*noTray {
		trayInst = tray.New(cfg, dashURL, cfgPath, emitter, func() {
			cancel()
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer shutdownCancel()
			_ = srv.Shutdown(shutdownCtx)
		})
		trayWG.Add(1)
		go func() {
			defer trayWG.Done()
			trayInst.Run()
		}()
	}

	// Main shutdown sequence: triggered by signal, server error, or desktop window close.
	doShutdown := func() {
		cancel()
		if trayInst != nil {
			trayInst.Stop()
		}
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		_ = srv.Shutdown(shutdownCtx)
		trayWG.Wait()
	}

	if showDesktop {
		// WebView2 must run on the main thread (COM requirement).
		// Wait for server to be ready, then show window.
		time.Sleep(400 * time.Millisecond)
		win := desktop.New(dashURL, "Windows Task Manager", 1280, 800, doShutdown)
		if win == nil {
			// WebView2 unavailable — fall back to browser
			if !*noBrowser {
				go func() {
					_ = winapi.ShellExecute("open", dashURL, "", "", winapi.SW_SHOWNORMAL)
				}()
			}
		} else {
			win.Run()
		}
	} else {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
		select {
		case s := <-sig:
			log.Printf("signal: %v — shutting down", s)
		case err := <-srvErr:
			log.Printf("http server: %v", err)
		}
		doShutdown()
	}
}

// clearDisabledDetectorAlerts wipes any active alerts whose detector was just
// turned off. Types must match the string names each detector returns from
// its Name() method.
func clearDisabledDetectorAlerts(alerts *anomaly.AlertStore, cfg *config.Config) {
	a := cfg.Anomaly
	if !a.HungProcess.Enabled {
		alerts.ClearByType("hung_process")
	}
	if !a.Orphan.Enabled {
		alerts.ClearByType("orphan")
	}
	if !a.PortConflict.Enabled {
		alerts.ClearByType("port_conflict")
	}
	if !a.NetworkAnomaly.Enabled {
		alerts.ClearByType("network_anomaly")
		alerts.ClearByType("network_anomaly_system")
	}
	if !a.SpawnStorm.Enabled {
		alerts.ClearByType("spawn_storm")
	}
	if !a.MemoryLeak.Enabled {
		alerts.ClearByType("memory_leak")
	}
	if !a.RunawayCPU.Enabled {
		alerts.ClearByType("runaway_cpu")
	}
	if !a.NewProcess.Enabled {
		alerts.ClearByType("new_process")
	}
}

// resolveConfigPath chooses the active config file.
//
// Precedence:
//  1. --config flag
//  2. config.yaml next to the executable
//  3. %APPDATA%\WindowsTaskManager\config.yaml
func resolveConfigPath(flagVal string) string {
	if flagVal != "" {
		return flagVal
	}
	exe, err := os.Executable()
	if err == nil {
		local := filepath.Join(filepath.Dir(exe), "config.yaml")
		if _, statErr := os.Stat(local); statErr == nil {
			return local
		}
	}
	appData := os.Getenv("APPDATA")
	if appData == "" {
		appData = "."
	}
	return filepath.Join(appData, "WindowsTaskManager", "config.yaml")
}
