# Changelog

All notable changes to this project will be documented in this file.

The format is based on Keep a Changelog, and this project aims to follow
Semantic Versioning.

## [Unreleased]

### Added

- Placeholder for upcoming changes.

## [0.3.1] - 2026-05-10

### Added

- Ports page redesigned with Dev Ports, Known Ports (34 registered ports with app names), and Node.js server instance panels.
- Main content area now has independent scroll — sidebar stays fixed while pages scroll.
- Ports page query optimization: eliminated O(n²) repeated `.filter()` calls with single-pass `Map` lookup.
- Ports page debounced search (300ms) to avoid filter runs on every keystroke.

### Changed

- Removed dead minimize/maximize/close buttons from topbar — Windows native title bar already handles window management, the buttons were decorative noise.
- `/network` page removed — redundant since `/overview` already shows per-interface CPU and network sparklines.

### Fixed

- Fixed ports-page Badge `className` prop — Badge component doesn't accept className, replaced with inline span styling.

## [0.3.0] - 2026-05-08

### Added

- Sparkline charts on Dashboard and Overview pages showing CPU and memory history trends.
- AI data preview toggle — see what system data is sent to the AI before submitting.
- AI conversation history management with Clear button and 20-turn cap.
- Load more pagination with aggregate summary on alerts history page.
- Clickable PIDs in processes table — clicking filters the process list to that PID.
- Keyboard navigation (j/k arrows) in processes table rows.
- Test Telegram and Test AI buttons in settings to validate credentials.
- Version and build info with links in About page.
- Undo duplicate meterBarClassName to shared utility.
- Kill/Suspend/Resume inline actions in process tree page.
- Alert count badge on sidebar Alerts nav item.
- Search match highlighting in process tree.

### Changed

- Sidebar is now always visible on desktop — no more disappearing on resize.
- Go toolchain updated to 1.26.3 to fix net/http vulnerabilities (CVE-2026-4971, CVE-2026-4918).

### Fixed

- Removed unreachable Window.Destroy deadcode in desktop package.
- Fixed ai_config.go result.Text undefined error (should use result.Answer).
- Fixed processes-page unused navigate import.
- Fixed tree-page TreeNode/TreeCard missing navigate prop.

## [0.2.0] - 2026-04-25

### Added

- Added the React/Vite frontend application with embedded assets, routed pages, shared UI components, live data bridge, tests, and typed API client.
- Added expanded project documentation covering architecture, production readiness, roadmap, and web UI notes.
- Added CI coverage for govulncheck, deadcode, unparam, module verification, and race-detector tests where supported.

### Changed

- Reworked the README with clearer setup, feature, configuration, and release guidance.
- Updated the GitHub Actions release workflow to use the Go version from `go.mod`, run additional analyzers, and publish versioned Windows artifacts with checksums.
- Removed legacy static web assets in favor of the new embedded frontend build pipeline.
- Removed compiled executables from version control and expanded ignore rules for generated binaries and frontend outputs.

### Fixed

- Strengthened collector, config, controller, server, storage, Telegram, and AI behavior with broader unit coverage and defensive request handling.
- Improved Windows collector and WinAPI integrations for disk, GPU, network, process, port, and runtime telemetry paths.

## [0.1.0] - 2026-04-11

### Added

- Initial public release of Windows Task Manager as a single-binary, pure-Go Windows utility.
- Live local web dashboard with CPU, memory, GPU, disk, network, process list, process tree, ports, alerts, and SSE updates.
- Process control operations including kill, suspend, resume, priority, affinity, and Job Object-based resource limits.
- Per-process protect and ignore toggles persisted into `config.yaml`.
- Conservative anomaly engine with built-in detectors for runaway CPU, memory leaks, spawn storms, and optional detectors for hung, orphaned, port, network, and suspicious new processes.
- User-defined automation rules editable from YAML and the web UI.
- AI advisor with approve-before-execute actions, provider presets, background watch mode, and dry-run auto-action policy evaluation.
- Telegram rescue bot with status, top CPU, alert inspection, and confirm-gated destructive actions.
- Native Windows tray integration with notifications and dashboard reopening.
- GitHub Actions release workflow that tests, vets, builds, and uploads versioned Windows release assets.

### Changed

- Added single-instance startup protection so a second `wtm.exe` copy refuses to launch.
- Added self-protection so the running WTM process cannot be killed or suspended from the UI, AI suggestions, rules, or Telegram.
- Hardened config persistence with atomic saves and better hot-reload propagation.
- Updated AI examples, defaults, and presets to newer model names and current provider recommendations.

### Fixed

- Fixed local-only middleware to correctly allow IPv6 loopback requests.
- Tightened JSON request parsing to reject oversized bodies and trailing garbage.
- Improved runtime config updates for collectors, anomaly analysis, tray sync, and other long-lived loops.

### Security

- Enforced local-only API access and stronger destructive-action guards for protected, critical, and self processes.
- Added Telegram confirmation codes for remote kill and suspend style actions.
