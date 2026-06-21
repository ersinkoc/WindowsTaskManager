# Windows Task Manager (WTM)

WTM is a local-first Windows monitoring and control console shipped as a single executable. It collects live machine telemetry, exposes a localhost REST API, serves an embedded web UI, detects suspicious or unhealthy process behavior, and can optionally layer AI and Telegram workflows on top.

Current release: [`v0.4.0`](https://github.com/ersinkoc/WindowsTaskManager/releases/tag/v0.4.0)

Project history lives in [`CHANGELOG.md`](CHANGELOG.md). Release notes and shipping steps live in [`RELEASING.md`](RELEASING.md).

## Highlights

- Pure Go on the backend. No CGo, WMI, or PowerShell collectors in the hot path.
- Single executable deployment. The frontend build from `frontend/dist` is embedded into the Go binary with `embed.FS`.
- Single-instance guard. Starting a second copy reuses the existing dashboard instead of launching a duplicate monitor.
- Live telemetry:
  - CPU, per-core CPU, memory
  - GPU utilization and VRAM
  - Disk usage plus read/write throughput
  - Network throughput and interface stats
  - Processes, process tree, TCP/UDP bindings
- Process actions:
  - kill, suspend, resume
  - priority and affinity changes
  - resource limits
  - self-protection for the WTM process
- Automation rules:
  - create and edit rules from the UI
  - enable, disable, delete, and preview rules
  - AI rule suggestions can open directly in the Rules page
- Alerts and anomaly detection for runaway CPU, memory growth, spawn storms, and more
- AI advisor with approve-before-execute suggestions
- Telegram bot for alerting, triage, confirm-before-action process control, and AI usage
- Live config reload from `config.yaml`

## Architecture

```text
cmd/wtm                 entry point, flags, lifecycle wiring
frontend/               React + Vite web UI, embedded from dist/
internal/ai             advisor, prompt shaping, action parsing, background watch
internal/anomaly        anomaly engine, alert store, rule evaluation
internal/collector      CPU, memory, process, tree, ports, disk, GPU, network collection
internal/config         YAML config, validation, schema migration, watcher
internal/controller     process actions and safeguards
internal/event          pub/sub emitter
internal/server         REST API, SSE stream, config endpoints, embedded UI serving
internal/storage        in-memory snapshot and history store
internal/telegram       Telegram bot, filtering, confirm flow, AI commands
internal/tray           system tray integration
internal/winapi         raw Win32 wrappers
configs/                reference config
```

## Prerequisites

- Windows
- Go `1.25+` (`go.mod` pins toolchain `go1.26.4`)
- Node.js `20+` if you want to change the frontend

## Build

### Release-style build

```powershell
.\build.ps1
```

This produces `wtm.exe`.

### Direct Go build

```powershell
go build -o wtm.exe ./cmd/wtm
```

### Frontend build

If you change anything under `frontend/`, rebuild the embedded UI first:

```powershell
cd frontend
npm install
npm run build
cd ..
go build -o wtm.exe ./cmd/wtm
```

`build.ps1` now runs `go mod tidy`, `go mod verify`, `go fmt`, `go test`, `go vet`, `govulncheck`, `deadcode`, `unparam`, and then builds the Windows binary. If `gcc` is available it also runs `go test -race`; otherwise it skips that step with a warning. It does not run the frontend build for you.

## Run

```powershell
.\wtm.exe
```

By default the dashboard opens at:

```text
http://127.0.0.1:19876
```

The tray icon appears in the notification area unless you disable it with a flag.

### Flags

| Flag | Description |
|---|---|
| `--config <path>` | Override config location |
| `--no-tray` | Disable tray icon |
| `--no-browser` | Do not auto-open the dashboard |
| `--version` | Print version and exit |

## Frontend

The current UI is in `frontend/` and is built with:

- React 19
- React Router 7
- TypeScript
- Vite
- TanStack Query
- Zustand
- React Hook Form
- Zod

Key pages:

- `Overview`
- `Processes`
- `Tree`
- `Ports`
- `Disks`
- `Alerts`
- `Rules`
- `AI`
- `Settings`
- `About`

Notable UI behaviors:

- live status strip on most pages
- sortable process and port views
- process safety guardrails around destructive actions
- rule presets, preview, inline edit, and delete confirmation
- AI suggestions that can be approved directly or opened in Rules

## Config

Default config location:

```text
%APPDATA%\WindowsTaskManager\config.yaml
```

If a `config.yaml` exists next to `wtm.exe`, WTM prefers it.

A reference config lives at [`configs/default.yaml`](configs/default.yaml).

WTM hot-reloads config changes through the native watcher and applies them live.

Important config areas:

- `monitoring`: collector cadence, history, max process limits
- `controller`: protection list and kill confirmation behavior
- `anomaly`: detector toggles and thresholds
- `notifications`: tray balloon settings
- `ai`: provider, endpoint, model, limits, scheduler, auto-action policy
- `telegram`: bot token, chat allowlist, confirm flow, notification filtering
- `ui`: theme, default sort, refresh rate, table defaults

## Settings UI

The `Settings` page is the main control surface for:

- AI provider and model setup
- Telegram bot configuration
- notification mode and alert type allowlist
- collector/runtime settings
- UI defaults

The old "AI tab owns all provider settings" flow is no longer the main path. Provider and Telegram configuration now live in `Settings`.

## Alerts and anomaly detection

WTM ships with a conservative default posture. Some detectors are enabled by default, while noisier ones can stay off until you want them.

Examples of supported alert types include:

- `runaway_cpu`
- `memory_leak`
- `spawn_storm`
- `hung_process`
- `orphan`
- `port_conflict`
- `network_anomaly`
- `new_process`
- `rule:<name>`

The UI supports:

- active alert review
- history review
- dismiss
- snooze

Telegram supports a higher-signal notification mode so noisy criticals do not flood chat by default.

## Rules

Rules let you describe conditions like:

> If `chrome.exe` uses more than `4 GB` for `30 seconds`, raise an alert.

or

> If `some-worker.exe` exceeds `300` threads for `20 seconds`, suspend it.

Current rule workflow:

- add rule from the Rules page
- choose metric, operator, threshold, action, hold time, cooldown
- preview the final behavior before saving
- edit existing rules inline
- enable or disable existing rules
- delete rules with confirmation
- open AI-generated `add_rule` suggestions directly in the Rules form

Rules are persisted through `POST /api/v1/rules`, which replaces the current ruleset with the submitted list.

## AI advisor

WTM supports:

- interactive AI chat
- one-shot snapshot analysis
- structured action suggestions
- background AI watch for critical alerts

Supported suggestion types:

- `kill`
- `suspend`
- `protect`
- `ignore`
- `add_rule`

Nothing is executed automatically from the normal UI flow. Suggestions must be explicitly approved.

### Supported provider modes

- `provider: anthropic`
- `provider: openai` for OpenAI-compatible endpoints

That means you can point WTM at providers such as:

- Anthropic
- OpenAI
- OpenRouter
- Groq
- DeepSeek
- Ollama
- LM Studio

### AI endpoints

| Method | Path | Description |
|---|---|---|
| GET | `/api/v1/ai/status` | AI status and current provider/model |
| GET | `/api/v1/ai/watch` | background AI watch state |
| POST | `/api/v1/ai/analyze` | one-shot snapshot analysis |
| POST | `/api/v1/ai/chat` | multi-turn chat |
| POST | `/api/v1/ai/execute` | execute an approved suggestion |
| GET | `/api/v1/ai/config` | current AI config |
| POST | `/api/v1/ai/config` | update AI config |
| GET | `/api/v1/ai/presets` | starter provider presets |
| GET | `/api/v1/ai/models` | model discovery endpoint |

## Telegram bot

The Telegram bot is intended for "the box is unhealthy and I need a remote control path" scenarios.

It supports:

- alert notifications
- status inspection
- process actions with confirm codes
- AI chat and AI analysis

Useful commands:

- `/status`
- `/topcpu`
- `/alerts`
- `/kill <pid>`
- `/suspend <pid>`
- `/resume <pid>`
- `/killtop`
- `/suspendtop`
- `/confirm <code>`
- `/cancel <code>`
- `/ask <question>`
- `/analyze <question>`

Notification behavior:

- `notification_mode: high_value` is the safer default
- `notification_types` can be tuned so only valuable alerts reach chat
- destructive actions still pass through the same controller safeguards as the local UI

## REST API

WTM serves a local-only API on loopback and protects mutating routes with localhost assumptions, origin checks, and a CSRF header.

Selected endpoints:

### System

| Method | Path |
|---|---|
| GET | `/api/v1/system` |
| GET | `/api/v1/cpu` |
| GET | `/api/v1/memory` |
| GET | `/api/v1/gpu` |
| GET | `/api/v1/disk` |
| GET | `/api/v1/network` |
| GET | `/api/v1/history` |
| GET | `/api/v1/info` |
| GET | `/api/v1/health` |

### Processes

| Method | Path |
|---|---|
| GET | `/api/v1/processes` |
| GET | `/api/v1/processes/tree` |
| GET | `/api/v1/processes/:pid` |
| GET | `/api/v1/processes/:pid/history` |
| GET | `/api/v1/processes/:pid/children` |
| GET | `/api/v1/processes/:pid/connections` |
| POST | `/api/v1/processes/:pid/kill` |
| POST | `/api/v1/processes/:pid/kill-tree` |
| POST | `/api/v1/processes/:pid/suspend` |
| POST | `/api/v1/processes/:pid/resume` |
| POST | `/api/v1/processes/:pid/priority` |
| POST | `/api/v1/processes/:pid/affinity` |
| POST | `/api/v1/processes/:pid/limit` |
| DELETE | `/api/v1/processes/:pid/limit` |
| GET | `/api/v1/processes/limits` |

### Ports, alerts, rules, config

| Method | Path |
|---|---|
| GET | `/api/v1/ports` |
| GET | `/api/v1/connections` |
| GET | `/api/v1/alerts` |
| GET | `/api/v1/alerts/history` |
| POST | `/api/v1/alerts/clear` |
| POST | `/api/v1/alerts/:type/dismiss` |
| POST | `/api/v1/alerts/:type/:pid/dismiss` |
| POST | `/api/v1/alerts/:type/snooze` |
| POST | `/api/v1/alerts/:type/:pid/snooze` |
| GET | `/api/v1/rules` |
| POST | `/api/v1/rules` |
| GET | `/api/v1/config` |
| PUT | `/api/v1/config` |
| POST | `/api/v1/config/protect` |
| POST | `/api/v1/config/ignore` |
| GET | `/api/v1/stream` |

## Security and safety

- Localhost-only API
- mutation guard with CSRF token
- confirm-before-action behavior for risky paths
- WTM protects its own process
- protected-process list is enforced across UI, AI, and Telegram control paths

## Development checks

Backend:

```powershell
go test ./... -count=1
go vet ./...
go run golang.org/x/vuln/cmd/govulncheck@latest ./...
go build ./cmd/wtm
```

Race detector on Windows needs a C toolchain such as MinGW/MSYS2 `gcc`:

```powershell
$env:CGO_ENABLED = "1"
go test -race ./...
```

Frontend:

```powershell
cd frontend
npm test
npm run build
```

## License

MIT. See [LICENSE](LICENSE).
