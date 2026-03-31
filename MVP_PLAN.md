# MVP Development Plan — Sentinel AI (EDR)

> Date: 2025-03-25
> Goal: Close the Agent → Backend → Brain → Agent loop to have a functional, demonstrable MVP.
> Execution: Each service is developed by an independent subagent in parallel. Integration is validated after all three converge.

---

## Current State

Three services exist but operate in isolation:

| Component | What works | What's missing |
|-----------|-----------|----------------|
| **Agent** (Rust) | Heartbeat POSTs to backend every 5s. LogCollector tails file and prints to stdout. ProcessMonitor detects new PIDs and prints to stdout. CommandExecutor can kill processes and block IPs. | LogCollector and ProcessMonitor **never POST to the backend**. CommandExecutor is only exercised in a `[SIMULATION]` block. No mechanism to receive commands from backend. |
| **Backend** (Go) | Receives heartbeat, logs, and processes via REST. Keyword-based alert detection in goroutines. | No forwarding to Brain. No command dispatch back to Agent. No persistence — alerts go to stdout and are lost. No alert storage or retrieval API. |
| **Brain** (Python) | Watches `/tmp/nsoc_test.log` via watchdog. Heuristic anomaly detection. LLM backend switchable via `.env`: `USE_LOCAL_LLM=true` → Ollama (local/sovereign), `USE_LOCAL_LLM=false` → Cloudflare Workers AI (cloud fallback). Ollama model name now configurable via `OLLAMA_MODEL` env var (was hardcoded). `.env.example` documents all options. | Operates completely independently — not connected to Backend. No HTTP API to receive events. No way to send analysis results or response commands back. |

---

## Target State

```
┌─────────────────────┐
│  Endpoint (Rust)    │
│  Agent              │
│                     │  POST /logs, /processes, /heartbeat
│  LogCollector ──────┼──────────────────────────────┐
│  ProcessMonitor ────┼──────────────────────────────┤
│  Heartbeat loop ────┼──────────────────────────────┤
│                     │                              │
│  CommandExecutor ◄──┼──── GET /commands?host=X ◄───┤
└─────────────────────┘                              │
                                                     ▼
                                          ┌──────────────────┐
                                          │  Backend (Go)    │
                                          │  :8080           │
                                          │                  │
                                          │  /heartbeat      │
                                          │  /logs ──────────┼──► POST to Brain /analyze
                                          │  /processes ─────┼──► POST to Brain /analyze
                                          │  /commands (GET) │◄── Brain enqueues commands
                                          │  /alerts  (GET)  │──► Returns stored alerts
                                          └──────────────────┘
                                                     │
                                                     ▼
                                          ┌──────────────────┐
                                          │  Brain (Python)  │
                                          │  :5000           │
                                          │                  │
                                          │  POST /analyze   │──► Parse → Anomaly → LLM
                                          │                  │──► Returns verdict + action
                                          └──────────────────┘
```

---

## API Contracts (Shared by all subagents)

These contracts are the **source of truth**. Each subagent must implement their side exactly as specified.

### Agent → Backend

**POST /heartbeat** (already working — no changes)
```json
// Request
{ "hostname": "string", "timestamp": "RFC3339", "os": "string" }
// Response: 200 "ACK"
```

**POST /logs**
```json
// Request
{
  "hostname": "string",
  "log": "string",
  "timestamp": "RFC3339"
}
// Response: 200 OK
```

**POST /processes**
```json
// Request
{
  "hostname": "string",
  "pid": 1234,
  "name": "string",
  "cmd": "string"
}
// Response: 200 OK
```

**GET /commands?hostname={hostname}**
```json
// Response: 200
{
  "commands": [
    { "type": "kill_process", "pid": 1234 },
    { "type": "block_ip", "ip": "192.168.1.100" }
  ]
}
// Response when no pending commands: 200
{ "commands": [] }
```

### Backend → Brain

**POST http://localhost:5000/analyze**
```json
// Request
{
  "source": "log" | "process",
  "hostname": "string",
  "data": {
    // For source="log":
    "log": "raw log line string",
    "timestamp": "RFC3339"
    // For source="process":
    "pid": 1234,
    "name": "string",
    "cmd": "string"
  }
}
// Response: 200
{
  "is_threat": true | false,
  "severity": "low" | "medium" | "high" | "critical",
  "reason": "string describing why",
  "recommended_action": null | { "type": "kill_process", "pid": 1234 } | { "type": "block_ip", "ip": "..." }
}
```

### Backend Internal: Alert Storage (in-memory for MVP)

```go
type Alert struct {
    ID        string    `json:"id"`
    Hostname  string    `json:"hostname"`
    Source    string    `json:"source"`    // "log" or "process"
    Detail    string    `json:"detail"`    // the raw log or process name
    Severity  string    `json:"severity"`
    Reason    string    `json:"reason"`
    Action    *Command  `json:"action"`    // recommended action from Brain
    Timestamp time.Time `json:"timestamp"`
}
```

**GET /alerts** (optional but useful for demo)
```json
// Response: 200
{ "alerts": [ { Alert }, ... ] }
```

---

## Phase 1: Parallel Service Work (3 independent subagents)

### Subagent A — Rust Agent

**Scope**: Wire LogCollector and ProcessMonitor to POST to Backend. Add command polling loop. Remove simulation block.

**Files to modify**:
- `src/edr/agent/src/log_collector.rs`
- `src/edr/agent/src/process_monitor.rs`
- `src/edr/agent/src/main.rs`

#### Task A1: LogCollector → POST /logs

File: `src/edr/agent/src/log_collector.rs`

Current state: The `run()` method reads new lines and does `println!("[LogCollector] NEW LOG: {}", line.trim())` (line 48). It never sends data anywhere.

Changes:
1. Accept a `reqwest::Client` and the backend base URL (`http://localhost:8080`) as constructor parameters.
2. Define a `LogEntry` struct with `hostname`, `log`, `timestamp` fields (matching the POST /logs contract above).
3. In the line-reading branch (line 45-49), after detecting a non-empty line:
   - Build a `LogEntry` with `System::host_name()`, the trimmed line, and `chrono::Utc::now().to_rfc3339()`.
   - POST it to `{base_url}/logs` using the shared `reqwest::Client`.
   - Keep the `println!` for local debug output.
   - Log errors but do not crash — fire-and-forget pattern.

#### Task A2: ProcessMonitor → POST /processes

File: `src/edr/agent/src/process_monitor.rs`

Current state: The `run()` method detects new PIDs and does `println!("[ProcessMonitor] NEW PROCESS: ...")` (line 42-47). It never sends data anywhere.

Changes:
1. Accept a `reqwest::Client` and backend base URL as constructor parameters.
2. Define a `ProcessEntry` struct with `hostname`, `pid`, `name`, `cmd` fields (matching POST /processes contract).
3. In the new-PID detection block (lines 40-49), after detecting a new process:
   - Build a `ProcessEntry` from the process info. Use `process.name().to_string_lossy().to_string()` for name, `process.cmd().join(" ")` for cmd, `pid.as_u32()` as an integer.
   - POST it to `{base_url}/processes`.
   - Keep the `println!` for debug. Fire-and-forget.

#### Task A3: Command Polling Loop

File: `src/edr/agent/src/main.rs`

Add a new Tokio task that polls `GET /commands?hostname={hostname}` every 5 seconds:
1. Define a response struct matching the `/commands` contract.
2. On each poll, deserialize the response. For each command:
   - `"kill_process"` → `CommandExecutor::execute(AgentCommand::KillProcess(pid))`
   - `"block_ip"` → `CommandExecutor::execute(AgentCommand::BlockIp(ip))`
3. Log results of each execution.

#### Task A4: Clean up main.rs

File: `src/edr/agent/src/main.rs`

1. **Remove the entire `[SIMULATION]` block** (lines 54-86). This is test scaffolding that spawns `sleep 100` and tries to kill it.
2. Pass the shared `reqwest::Client` and `backend_url` base (`"http://localhost:8080"`) to both `LogCollector::new()` and `ProcessMonitor::new()`.
3. Spawn the command polling loop as a new `tokio::spawn` task.

#### Verification (Agent)
After completing all tasks, the agent should:
- Still compile with `cargo build` without warnings.
- On `cargo run`, POST heartbeats, logs, and processes to `localhost:8080`.
- Poll `/commands` and execute any pending commands.
- Not contain the `[SIMULATION]` block.

---

### Subagent B — Go Backend

**Scope**: Add Brain forwarding, command queue, alert storage, and new endpoints.

**File to modify**: `src/edr/backend/main.go` (single-file monolith, keep it that way).

#### Task B1: In-Memory Alert and Command Storage

Add package-level variables:
```go
var (
    alerts     []Alert
    alertsMu   sync.Mutex
    commandQueue   map[string][]CommandEntry  // hostname → pending commands
    commandsMu     sync.Mutex
)
```

Define structs:
```go
type Alert struct {
    ID        string    `json:"id"`
    Hostname  string    `json:"hostname"`
    Source    string    `json:"source"`
    Detail    string    `json:"detail"`
    Severity  string    `json:"severity"`
    Reason    string    `json:"reason"`
    Action    *CommandEntry `json:"action,omitempty"`
    Timestamp time.Time `json:"timestamp"`
}

type CommandEntry struct {
    Type string `json:"type"`
    PID  int    `json:"pid,omitempty"`
    IP   string `json:"ip,omitempty"`
}

type BrainRequest struct {
    Source   string      `json:"source"`
    Hostname string     `json:"hostname"`
    Data     interface{} `json:"data"`
}

type BrainResponse struct {
    IsThreat          bool         `json:"is_threat"`
    Severity          string       `json:"severity"`
    Reason            string       `json:"reason"`
    RecommendedAction *CommandEntry `json:"recommended_action"`
}
```

#### Task B2: Brain Forwarding

Create a function `forwardToBrain(source string, hostname string, data interface{})`:
1. Build a `BrainRequest` and POST it as JSON to `http://localhost:5000/analyze`.
2. Use a 5-second timeout (Brain might call Ollama which takes time).
3. If Brain is unreachable, log a warning and return — do not block or crash.
4. On success, deserialize `BrainResponse`.
5. If `is_threat == true`:
   - Generate an alert ID (e.g., timestamp-based or incrementing counter).
   - Store an `Alert` in the `alerts` slice (mutex-protected).
   - Log it: `[ALERT-AI] severity: reason`.
   - If `recommended_action` is not null, enqueue it in `commandQueue[hostname]`.

#### Task B3: Update handleLogs and handleProcesses

Modify `handleLogs` (line 69-81):
- Keep the existing `go analyzeLog(entry)` for fast keyword detection.
- Add: `go forwardToBrain("log", entry.Hostname, map[string]interface{}{"log": entry.Log, "timestamp": entry.Timestamp})`.

Modify `handleProcesses` (line 83-95):
- Keep the existing `go analyzeProcess(entry)`.
- Add: `go forwardToBrain("process", entry.Hostname, map[string]interface{}{"pid": entry.PID, "name": entry.Name, "cmd": entry.Cmd})`.

#### Task B4: New Endpoint — GET /commands

Handler `handleCommands`:
1. Accept only GET.
2. Read `hostname` from query parameter: `r.URL.Query().Get("hostname")`.
3. Lock `commandsMu`, drain all commands for that hostname, unlock.
4. Return `{"commands": [...]}` (empty array if none).
5. Register: `http.HandleFunc("/commands", handleCommands)` in `main()`.

#### Task B5: New Endpoint — GET /alerts

Handler `handleAlerts`:
1. Accept only GET.
2. Lock `alertsMu`, return the full `alerts` slice as JSON, unlock.
3. Register: `http.HandleFunc("/alerts", handleAlerts)` in `main()`.

#### Task B6: Initialize Storage in main()

In `main()`, before `http.ListenAndServe`:
```go
commandQueue = make(map[string][]CommandEntry)
```

#### Verification (Backend)
After completing all tasks:
- `go build` should succeed with no errors.
- `go run main.go` starts on :8080.
- POST to `/logs` with a suspicious keyword should:
  - Trigger `[ALERT]` from keyword detection (existing).
  - Forward to Brain at `:5000/analyze` (new).
  - Store alert in memory if Brain says `is_threat: true` (new).
- GET `/commands?hostname=test` returns `{"commands": []}`.
- GET `/alerts` returns stored alerts as JSON array.

---

### Subagent C — Python Brain

**Scope**: Add an HTTP API (Flask-free, use stdlib `http.server`) so the Backend can send events for analysis. Keep the existing watchdog pipeline as a secondary input.

**File to modify**: `src/edr/brain/main.py`
**File to modify**: `src/edr/brain/requirements.txt` (no new deps needed — use stdlib `http.server`)

> **Completed (LLM backend):** `LLMClient` now supports two backends controlled by the `USE_LOCAL_LLM` environment variable. When `true`, it calls Ollama at `localhost:11434` (sovereign, no cloud dependency); when `false`, it calls Cloudflare Workers AI. The Ollama model is no longer hardcoded — it reads from `OLLAMA_MODEL` (default: `llama3`). Cloudflare requires `CLOUDFLARE_ACCOUNT_ID`, `CLOUDFLARE_API_TOKEN`, and `CLOUDFLARE_MODEL`. All variables are documented in `.env.example`. The 2-second mock fallback behavior is preserved when the selected backend is unreachable.

#### Task C1: HTTP API with stdlib

Add an HTTP server on port 5000 using `http.server.HTTPServer` and `BaseHTTPRequestHandler`. Do NOT add Flask or any framework — keep it stdlib.

Implement `POST /analyze`:
1. Read and parse JSON body.
2. Extract `source`, `hostname`, `data`.
3. Route through existing analysis pipeline:
   - For `source == "log"`: Call `LogParser.parse(data["log"])`, then `AnomalyModel.check(parsed)`, then `LLMClient.analyze()` if anomaly.
   - For `source == "process"`: Call `AnomalyModel.check_process(data)` (new method, see Task C2).
4. Build response:
   ```json
   {
     "is_threat": bool,
     "severity": "low|medium|high|critical",
     "reason": "string",
     "recommended_action": null or {"type": "kill_process", "pid": N} or {"type": "block_ip", "ip": "..."}
   }
   ```
5. Return 200 with JSON response.

#### Task C2: Process Analysis in AnomalyModel

Add a new static method `AnomalyModel.check_process(data)`:
```python
@staticmethod
def check_process(data):
    """Check if a process is suspicious."""
    name = data.get("name", "").lower()
    cmd = data.get("cmd", "").lower()

    malicious = ["nc", "ncat", "cryptominer", "mimikatz", "reverse", "shell", "meterpreter"]
    for kw in malicious:
        if kw in name or kw in cmd:
            return True, f"Malicious process keyword: {kw}"

    return False, None
```

#### Task C3: Severity Classification

Add a static method `AnomalyModel.classify_severity(reason, source)`:
- Keywords like "mimikatz", "meterpreter", "cryptominer" → `"critical"`
- Keywords like "sudo", "root", "unauthorized" → `"high"`
- Keywords like "failed", "error" → `"medium"`
- Length anomaly → `"low"`

This is used in the `/analyze` response.

#### Task C4: Recommend Actions

In the `/analyze` handler, after determining it's a threat:
- If `source == "process"` and severity is `"critical"`: recommend `{"type": "kill_process", "pid": data["pid"]}`.
- If LLM analysis mentions "block" or severity is `"critical"` and there's an IP in the log: recommend `{"type": "block_ip", "ip": extracted_ip}`.
- Otherwise: `null` (alert only, no automatic action).

Keep recommendations conservative for MVP — only auto-kill for critical process matches.

#### Task C5: Run HTTP Server Alongside Watchdog

Modify `main()` to run both:
1. Start the watchdog observer (existing code, keep as-is).
2. Start the HTTP server on `:5000` in a separate thread (`threading.Thread`).
3. Both run concurrently — the watchdog handles direct file monitoring (useful for standalone testing), and the HTTP API handles Backend-forwarded events.

```python
def main():
    # ... existing watchdog setup ...

    # Start HTTP API in a thread
    server = HTTPServer(('0.0.0.0', 5000), BrainHTTPHandler)
    api_thread = threading.Thread(target=server.serve_forever, daemon=True)
    api_thread.start()
    print("[BRAIN] HTTP API listening on :5000")

    # ... existing observer.start() and while True loop ...
```

#### Task C6: Update start_edr.sh (if needed)

The Brain already runs via `./venv/bin/python main.py`. No changes needed to `start_edr.sh` since the HTTP server starts within the same process.

#### Verification (Brain)
After completing all tasks:
- `./venv/bin/python main.py` should start both the watchdog and HTTP API on :5000.
- `curl -X POST http://localhost:5000/analyze -H 'Content-Type: application/json' -d '{"source":"log","hostname":"test","data":{"log":"sudo failed unauthorized","timestamp":"2025-03-25T00:00:00Z"}}'` should return a threat verdict with severity and reason.
- `curl -X POST http://localhost:5000/analyze -d '{"source":"process","hostname":"test","data":{"pid":1234,"name":"cryptominer","cmd":"./cryptominer --mine"}}'` should return `is_threat: true, severity: critical, recommended_action: {type: kill_process, pid: 1234}`.
- The watchdog file-monitoring still works: `echo "sudo failed" >> /tmp/nsoc_test.log` still prints `[BRAIN] ANOMALY DETECTED`.

---

## Phase 2: Integration Verification

After all three subagents complete their work, verify the full loop manually:

### Test 1: Log Alert Pipeline
```bash
# Start all services
./start_edr.sh

# Append a suspicious log line
echo "Mar 25 12:00:00 server sudo: FAILED su for root by attacker" >> /tmp/nsoc_test.log
```

**Expected flow**:
1. Agent LogCollector reads the line → POSTs to Backend `/logs`.
2. Backend keyword analysis triggers `[ALERT]` (existing).
3. Backend forwards to Brain `/analyze`.
4. Brain returns `{is_threat: true, severity: "high", reason: "Keyword match: sudo"}`.
5. Backend stores alert in memory.
6. `GET /alerts` returns the alert.

### Test 2: Malicious Process → Auto-Kill
```bash
# In another terminal, simulate a malicious process
nc -l 9999 &
```

**Expected flow**:
1. Agent ProcessMonitor detects `nc` → POSTs to Backend `/processes`.
2. Backend keyword analysis triggers `[ALERT]` for "nc".
3. Backend forwards to Brain.
4. Brain returns `{is_threat: true, severity: "critical", recommended_action: {type: "kill_process", pid: X}}`.
5. Backend enqueues command for hostname.
6. Agent polls `GET /commands` → receives `kill_process` → CommandExecutor kills the `nc` process.

### Test 3: No False Positives
```bash
echo "Normal system operation completed successfully" >> /tmp/nsoc_test.log
```

**Expected**: No alert generated. Brain returns `{is_threat: false}`.

### Test 4: Brain Down Gracefully
1. Stop the Brain process.
2. Agent sends logs/processes to Backend.
3. Backend tries to forward to Brain, gets connection refused.
4. Backend logs warning but continues operating — keyword detection still works.
5. Agent continues without issues.

---

## Phase 3: PostgreSQL Persistence (Subagent B — Go Backend)

After Phase 1 and 2 are validated, replace in-memory storage with PostgreSQL. The Go Backend owns all database writes — the Brain remains stateless.

### Why PostgreSQL Only (No Elasticsearch Yet)

- MVP volume is tiny (one agent, keyword alerts). PostgreSQL handles it trivially.
- PostgreSQL `jsonb` columns give semi-structured flexibility for raw payloads without needing a separate NoSQL store.
- One connection pool, one service writing — no coordination issues.
- Elasticsearch is deferred until 5+ agents or 10M+ rows justify full-text search at scale.

### Why Go Writes, Not Python

- All telemetry already flows through Go — writing to DB at that point is the natural place.
- One writer, one connection pool, no race conditions.
- If Brain goes down, persistence still works (keyword alerts still get stored).
- Brain stays stateless and replaceable (swap Python for Rust ML engine later, nothing about persistence changes).

### Database Schema

```sql
-- Initialize database
CREATE DATABASE nsoc;

-- Registered agents (from heartbeats)
CREATE TABLE agents (
    id            SERIAL PRIMARY KEY,
    hostname      VARCHAR(255) UNIQUE NOT NULL,
    os            VARCHAR(100),
    last_heartbeat TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    first_seen    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Raw log events received from agents
CREATE TABLE log_events (
    id         SERIAL PRIMARY KEY,
    hostname   VARCHAR(255) NOT NULL,
    log        TEXT NOT NULL,
    timestamp  TIMESTAMPTZ NOT NULL,
    received_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_log_events_hostname_ts ON log_events (hostname, timestamp DESC);

-- Raw process events received from agents
CREATE TABLE process_events (
    id         SERIAL PRIMARY KEY,
    hostname   VARCHAR(255) NOT NULL,
    pid        INTEGER NOT NULL,
    name       VARCHAR(255) NOT NULL,
    cmd        TEXT,
    received_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_process_events_hostname ON process_events (hostname, received_at DESC);

-- Alerts (from keyword detection + Brain AI verdicts)
CREATE TABLE alerts (
    id          VARCHAR(64) PRIMARY KEY,
    hostname    VARCHAR(255) NOT NULL,
    source      VARCHAR(20) NOT NULL,  -- 'log' or 'process'
    detail      TEXT NOT NULL,
    severity    VARCHAR(20) NOT NULL,  -- 'low', 'medium', 'high', 'critical'
    reason      TEXT NOT NULL,
    action_type VARCHAR(20),           -- 'kill_process', 'block_ip', or NULL
    action_data JSONB,                 -- {"pid": 1234} or {"ip": "..."}
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_alerts_hostname ON alerts (hostname, created_at DESC);
CREATE INDEX idx_alerts_severity ON alerts (severity);

-- Commands dispatched to agents (audit trail)
CREATE TABLE commands (
    id          SERIAL PRIMARY KEY,
    hostname    VARCHAR(255) NOT NULL,
    type        VARCHAR(20) NOT NULL,  -- 'kill_process', 'block_ip'
    payload     JSONB NOT NULL,
    dispatched  BOOLEAN NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    dispatched_at TIMESTAMPTZ
);
CREATE INDEX idx_commands_pending ON commands (hostname, dispatched) WHERE dispatched = FALSE;
```

### Task B7: Add PostgreSQL Driver

File: `src/edr/backend/go.mod`

Add dependency: `github.com/lib/pq` (standard PostgreSQL driver for Go). This is the one exception to the "stdlib only" rule — Go's `database/sql` is stdlib but needs a driver.

```bash
cd src/edr/backend && go get github.com/lib/pq
```

### Task B8: Database Connection Pool

File: `src/edr/backend/main.go`

1. Add a package-level `*sql.DB` variable.
2. In `main()`, connect using `sql.Open("postgres", connStr)` before starting the HTTP server.
3. Connection string from environment variable: `DATABASE_URL` (default: `postgres://nsoc:nsoc@localhost:5432/nsoc?sslmode=disable`).
4. Set pool limits: `db.SetMaxOpenConns(10)`, `db.SetMaxIdleConns(5)`.

### Task B9: Persist Telemetry on Receive

Modify handlers to write to DB:

**handleHeartbeat**: Upsert into `agents` table (INSERT ON CONFLICT hostname DO UPDATE SET last_heartbeat, os).

**handleLogs**: INSERT into `log_events` table. Keep the existing goroutine for keyword analysis + Brain forwarding unchanged.

**handleProcesses**: INSERT into `process_events` table. Keep the existing goroutine for analysis + Brain forwarding unchanged.

All DB writes should be fire-and-forget in goroutines — do not block the HTTP response.

### Task B10: Persist Alerts from Brain

Modify `forwardToBrain()`:
- When Brain returns `is_threat: true`, INSERT into `alerts` table instead of (or in addition to) the in-memory slice.
- If there's a `recommended_action`, INSERT into `commands` table with `dispatched = FALSE`.

### Task B11: Update GET /commands to Use DB

Modify `handleCommands`:
1. SELECT from `commands` WHERE `hostname = $1 AND dispatched = FALSE`.
2. Return the commands as JSON.
3. UPDATE those rows SET `dispatched = TRUE, dispatched_at = NOW()`.
4. Use a transaction to prevent race conditions.

### Task B12: Update GET /alerts to Use DB

Modify `handleAlerts`:
1. SELECT from `alerts` ORDER BY `created_at DESC` LIMIT 100.
2. Optional query params: `?hostname=X`, `?severity=critical`.
3. Return as JSON array.

### Task B13: Schema Migration on Startup

Add a simple `initDB()` function that runs the CREATE TABLE statements with `IF NOT EXISTS`. Called once in `main()` after connecting. No migration framework needed for MVP.

### Verification (Phase 3)

1. PostgreSQL running locally on :5432 with database `nsoc` created.
2. `go run main.go` connects to DB and creates tables on startup.
3. Agent heartbeat → `agents` table gets a row.
4. Suspicious log → `log_events` + `alerts` tables get rows.
5. `GET /alerts` returns data from PostgreSQL.
6. `GET /commands` drains from `commands` table and marks as dispatched.
7. Restart backend → data survives (no more in-memory loss).

### Environment Setup for Development

```bash
# Install PostgreSQL (if not present)
sudo apt install postgresql postgresql-client

# Create database and user
sudo -u postgres psql -c "CREATE USER nsoc WITH PASSWORD 'nsoc';"
sudo -u postgres psql -c "CREATE DATABASE nsoc OWNER nsoc;"

# Set environment variable (add to .bashrc or .zshrc)
export DATABASE_URL="postgres://nsoc:nsoc@localhost:5432/nsoc?sslmode=disable"
```

---

## Out of Scope (Deferred)

These are explicitly **not part of this MVP plan**:
- Frontend dashboard
- Elasticsearch (deferred until scale requires full-text log search)
- mTLS / authentication
- Multi-tenant isolation
- NATS / gRPC migration
- Products 2 and 3 (ZTNA, Cloud Security)
- Formal test suites (`cargo test`, `go test`, `pytest`)
- Docker / container packaging

---

## File Change Summary

| File | Subagent | Phase | Changes |
|------|----------|-------|---------|
| `src/edr/agent/src/log_collector.rs` | A | 1 | Accept client+URL, POST new lines to `/logs` |
| `src/edr/agent/src/process_monitor.rs` | A | 1 | Accept client+URL, POST new PIDs to `/processes` |
| `src/edr/agent/src/main.rs` | A | 1 | Remove simulation block, pass client to modules, add command polling loop |
| `src/edr/backend/main.go` | B | 1 | Add Brain forwarding, command queue, alert storage, GET /commands, GET /alerts |
| `src/edr/brain/main.py` | C | 1 | Add HTTP API (:5000) with POST /analyze, process analysis, severity classification |
| `src/edr/brain/main.py` | C | 1 | **Done.** `LLMClient` now supports two backends: Ollama (local) and Cloudflare Workers AI (cloud). Switchable via `USE_LOCAL_LLM` env var. `OLLAMA_MODEL` env var replaces hardcoded model name. |
| `.env.example` | C | 1 | **Done.** Documents all Brain env vars: `USE_LOCAL_LLM`, `OLLAMA_MODEL`, `CLOUDFLARE_ACCOUNT_ID`, `CLOUDFLARE_API_TOKEN`, `CLOUDFLARE_MODEL`. |
| `src/edr/backend/main.go` | B | 3 | Add PostgreSQL connection, persist telemetry/alerts/commands to DB |
| `src/edr/backend/go.mod` | B | 3 | Add `github.com/lib/pq` dependency |

Phase 1: No new files, no new dependencies (Python uses stdlib `http.server` and `threading`).
Phase 3: One new dependency (`lib/pq`). PostgreSQL must be running locally.

---

## Sentinel AI vs Wazuh (HIDS clásico)

| | Wazuh | Sentinel AI (NSOC) |
|---|---|---|
| Detección | Reglas estáticas (XML) | Heurística + LLM semántico |
| Explicación | "Rule 5503 matched" | Análisis en lenguaje natural del por qué es una amenaza |
| Respuesta | Scripts manuales / activos preconfigurados | Loop autónomo: Brain decide → Backend ordena → Agente ejecuta (kill/block) |
| Operador requerido | Analista L1 que interpreta alertas | El LLM hace el trabajo del L1 |
| Datos al cloud | Depende config | Soberano: LLM corre local, nada sale de la red |
| Audiencia | Equipos SOC con experiencia | PYMEs sin equipo de seguridad |
