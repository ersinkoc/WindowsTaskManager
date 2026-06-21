package anomaly

import (
	"testing"

	"github.com/ersinkoc/WindowsTaskManager/internal/config"
)

func TestIsIgnoredProcessBranches(t *testing.T) {
	cfg := &config.Config{}
	cfg.Anomaly.IgnoreProcesses = []string{"svchost.exe", "  ", "search"}

	if isIgnoredProcess(nil, "foo") {
		t.Fatal("nil cfg should not match")
	}
	if isIgnoredProcess(cfg, "") {
		t.Fatal("empty name should not match")
	}
	if isIgnoredProcess(&config.Config{}, "anything") {
		t.Fatal("empty ignore list should not match")
	}
	if !isIgnoredProcess(cfg, "SVCHOST.EXE") {
		t.Fatal("expected case-insensitive match")
	}
	if !isIgnoredProcess(cfg, "searchindexer.exe") {
		t.Fatal("expected substring match")
	}
	if isIgnoredProcess(cfg, "explorer.exe") {
		t.Fatal("expected no match")
	}
}

func TestIsIgnoredProcessSkipsBlankPatterns(t *testing.T) {
	cfg := &config.Config{}
	cfg.Anomaly.IgnoreProcesses = []string{"   ", ""}
	if isIgnoredProcess(cfg, "x.exe") {
		t.Fatal("should not match blank patterns")
	}
}
