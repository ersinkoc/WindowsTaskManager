# WTM API Documentation

Base URL: `http://localhost:19876/api/v1`

## Authentication

All endpoints (except `/health`) require a CSRF token header:

```
X-WTM-CSRF: <token>
```

The token is provided in the page's meta tag:
```html
<meta name="wtm-csrf-token" content="<token>">
```

## Endpoints

### System

#### `GET /system`
Returns complete system snapshot (CPU, memory, network, processes, etc.)

**Response:**
```json
{
  "timestamp": 1715894400000,
  "cpu": {
    "name": "Intel(R) Core(TM) i7-10700K",
    "num_logical": 16,
    "num_physical": 8,
    "total_percent": 23.5,
    "per_core": [12.1, 34.2, ...]
  },
  "memory": {
    "total_phys": 34359738368,
    "used_phys": 17179869184,
    "used_percent": 50.0
  },
  "network": {
    "total_down_bps": 1024000,
    "total_up_bps": 512000,
    "interfaces": [...]
  },
  "processes": [...],
  "port_bindings": [...]
}
```

#### `GET /cpu`
Returns CPU metrics only.

#### `GET /memory`
Returns memory metrics only.

#### `GET /gpu`
Returns GPU metrics (if available).

#### `GET /disk`
Returns disk metrics.

#### `GET /network`
Returns network interface metrics.

#### `GET /history?seconds=120`
Returns historical metrics for the specified duration.

---

### Processes

#### `GET /processes`
Returns list of all processes.

**Query Parameters:**
- `search` — Filter by name or PID
- `sort` — Sort by `cpu`, `memory`, `name`, `threads`, `connections`, `pid`
- `order` — `asc` or `desc`

**Response:**
```json
{
  "processes": [
    {
      "pid": 1234,
      "name": "chrome.exe",
      "cpu_percent": 12.5,
      "working_set": 104857600,
      "thread_count": 24,
      "connections": 5,
      "is_critical": false,
      "is_system": false,
      "state": "Running"
    }
  ]
}
```

#### `GET /processes/:pid`
Returns details for a specific process.

#### `GET /processes/:pid/history`
Returns process history for the last collection interval.

#### `GET /processes/:pid/children`
Returns child processes (process tree node).

#### `GET /processes/:pid/connections`
Returns port bindings for this process.

#### `POST /processes/:pid/kill`
Terminates the process.

**Request:**
```json
{"action": "kill"}
```

#### `POST /processes/:pid/kill-tree`
Terminates the process and all its children.

---

### Process Tree

#### `GET /processes/tree`
Returns the full process tree.

**Response:**
```json
{
  "tree": [
    {
      "pid": 0,
      "name": "System Idle Process",
      "children": [
        {
          "pid": 4,
          "name": "System",
          "children": [...]
        }
      ]
    }
  ]
}
```

---

### Ports

#### `GET /ports`
Returns all port bindings with process info.

**Query Parameters:**
- `protocol` — Filter by `tcp`, `udp`, or `all`
- `state` — Filter by `listening`, `active`, `all`

---

### Alerts

#### `GET /alerts`
Returns current active and resolved alerts.

**Response:**
```json
{
  "active": [
    {
      "id": "uuid",
      "type": "runaway_cpu",
      "severity": "warning",
      "message": "CPU spike detected",
      "pid": 1234,
      "timestamp": 1715894400000,
      "data": {}
    }
  ],
  "resolved": [...]
}
```

#### `POST /alerts/:id/dismiss`
Dismisses an alert.

---

### Rules

#### `GET /rules`
Returns all configured rules.

**Response:**
```json
{
  "rules": [
    {
      "name": "High CPU guard",
      "enabled": true,
      "match": "chrome.exe",
      "metric": "cpu_percent",
      "op": ">=",
      "threshold": 90,
      "for_seconds": 30,
      "action": "alert",
      "cooldown_seconds": 300
    }
  ]
}
```

#### `PUT /rules`
Updates rules (full replacement).

---

### Configuration

#### `GET /config`
Returns current configuration.

#### `PUT /config`
Updates configuration.

---

### AI

#### `GET /ai/status`
Returns AI configuration status.

#### `GET /ai/presets`
Returns available AI presets.

#### `POST /ai/config`
Updates AI configuration.

#### `POST /ai/suggest`
Requests AI suggestion for a process or alert.

---

### Telegram

#### `GET /telegram/config`
Returns Telegram bot configuration.

#### `PUT /telegram/config`
Updates Telegram configuration.

#### `POST /telegram/test`
Sends a test message to verify configuration.

---

### System

#### `GET /health`
Health check endpoint. No authentication required.

**Response:**
```json
{"status": "ok"}
```

#### `GET /info`
Returns system information (version, PID, CPU count, etc.).

---

## SSE Streaming

### `GET /stream`

Server-Sent Events stream for real-time updates.

**Event Types:**
- `snapshot` — Full system snapshot every second
- `alert` — New alert triggered
- `error` — Error occurred

**Headers:**
```
Accept: text/event-stream
Cache-Control: no-cache
```

---

## Error Responses

All errors return JSON with the following structure:

```json
{
  "error": {
    "code": "error_code",
    "message": "Human-readable message"
  }
}
```

**Common Error Codes:**
- `invalid_request` — Missing or invalid parameters
- `unauthorized` — CSRF token missing or invalid
- `not_found` — Resource not found
- `forbidden` — Action not allowed (e.g., killing protected process)
- `internal_error` — Server error
