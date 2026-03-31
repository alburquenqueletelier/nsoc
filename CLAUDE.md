# CLAUDE.md — NSOC Project Guide

> Last updated: 2026-03-17
> This file is the authoritative context document for AI assistants working on this codebase.
> The original strategic context lives in `GEMINI.md` (written by Google's Gemini AI at project inception).

---

## Project Overview

NSOC (AI-Driven SOC for PYMEs Latam) is a cybersecurity SaaS platform aimed at automating Level-1 SOC analyst work for small and medium businesses in Latin America. The positioning is "Darktrace for PYMEs" — advanced threat detection at accessible cost using open-source LLMs running locally (sovereign AI, no cloud dependency for inference).

Three planned products:
1. **Sentinel AI** — AI-driven EDR (Endpoint Detection and Response). **Currently in development (Phase 1 MVP).**
2. **Identity Risk Engine** — Smart ZTNA (Zero Trust Network Access). Planned.
3. **Cloud Security Autopilot** — Cloud misconfiguration scanner + IaC auto-remediation. Planned.

The platform is targeting CORFO Semilla Inicia funding (~15M CLP) for hardware (LLM inference) and commercial validation.

---

## Technology Stack

| Layer | Technology | Notes |
|---|---|---|
| Endpoint Agent | Rust (edition 2021) | Memory-safe, zero-GC; runs on client endpoints |
| Backend API | Go 1.25 | Goroutine concurrency; stdlib HTTP only, no frameworks |
| AI Brain | Python 3.13 | Heuristics + Ollama LLM integration |
| Frontend | Vue 3 + TypeScript | NOT STARTED |
| Primary DB | PostgreSQL | NOT IMPLEMENTED (planned) |
| Log Storage | Elasticsearch | NOT IMPLEMENTED (planned) |
| LLM Runtime | Ollama (llama3 / mistral) | Must run locally; 16GB+ RAM required |
| Process Orchestration | mprocs | Used by `start_edr.sh` to run all services in parallel |
| Protocol (current) | REST / JSON over HTTP | gRPC planned for future scaling |
| Messaging (current) | Direct HTTP calls | NATS JetStream planned for future scaling |

---

## Project Structure

```
nsoc/
├── CLAUDE.md                        # This file — AI assistant context
├── GEMINI.md                        # Original AI context (do not modify unless asked)
├── start_edr.sh                     # Launches all three EDR services via mprocs
│
├── src/
│   └── edr/
│       ├── agent/                   # Rust endpoint agent (Sentinel AI sensor)
│       │   ├── Cargo.toml
│       │   └── src/
│       │       ├── main.rs          # Entry point: orchestrates modules, heartbeat loop
│       │       ├── log_collector.rs # Tails log file for new lines
│       │       ├── process_monitor.rs # Polls for new system processes
│       │       └── command_executor.rs # Executes KillProcess / BlockIP commands
│       │
│       ├── backend/                 # Go REST API (agent telemetry broker)
│       │   ├── go.mod               # Module: nsoc/edr/backend
│       │   └── main.go              # All routes and logic in single file (MVP monolith)
│       │
│       └── brain/                   # Python AI analysis engine
│           ├── requirements.txt     # requests, watchdog
│           ├── main.py              # LogParser, AnomalyModel, LLMClient, LogHandler
│           └── venv/                # Python virtual environment (use ./venv/bin/python)
│
├── plan_desarrollo/
│   ├── stack_tecnologico.md         # Tech stack rationale and architecture diagram
│   ├── roadmap_software.md          # Module-level roadmap for all 3 products
│   ├── producto_1_edr.md            # EDR product strategy
│   ├── producto_2_ztna.md           # ZTNA product strategy
│   └── producto_3_cloud_sec.md      # Cloud Security Autopilot strategy
│
├── estrategia/
│   └── top_3_mercado.md             # Market positioning and AI-first focus
│
├── negocio/
│   └── estrategia_escalamiento.md   # AI-driven scaling strategy
│
├── financiamiento/
│   └── plan_corfo.md                # CORFO Semilla Inicia funding plan
│
└── analisis/
    └── servicios_sparkfound.md      # Competitive analysis
```

---

## Architecture and Design Patterns

### Data Flow (Phase 1 MVP)

```
Endpoint (Rust Agent)
  |-- POST /heartbeat  (every 5s)   -->  Backend (Go :8080)
  |-- POST /logs       (new lines)  -->  Backend (Go :8080)
  |-- POST /processes  (new PIDs)   -->  Backend (Go :8080)

Backend (Go)
  |-- analyzeLog()   -- keyword match --> [ALERT] to stdout
  |-- analyzeProcess() -- name match  --> [ALERT] to stdout
  (goroutines: analysis is non-blocking)

Brain (Python) -- operates independently
  |-- watchdog observes /tmp/nsoc_test.log
  |-- AnomalyModel heuristics
  |-- LLMClient --> Ollama :11434 (or mock fallback)
  --> output to stdout only
```

Note: The Brain currently operates independently of the Go backend. It watches the same log file the Agent reads from (`/tmp/nsoc_test.log`), but there is no direct channel from Backend to Brain yet. This integration is future work.

### Architectural Pattern
- **Distributed agent-broker model**: Rust agent on endpoints pushes telemetry to a central Go broker.
- **MVP Monolith in Go**: All backend logic lives in a single `main.go` file. Intentional for rapid iteration.
- **Async-by-default in Rust**: All agent operations use Tokio async tasks (`tokio::spawn`).
- **Fire-and-forget analysis in Go**: Log and process analysis runs in goroutines; HTTP response returns immediately.

### Key Design Decisions
- No external HTTP framework in Go (stdlib `net/http` only) — keeps binary static and deployment simple.
- Rust agent uses `sysinfo` crate for cross-platform process management rather than calling OS commands.
- BlockIP uses `iptables` CLI directly — requires root. KillProcess uses `sysinfo.process.kill()` — no root needed.
- LLM fallback: if Ollama is unreachable (timeout 2s), Brain returns a mock analysis string rather than failing.
- `mprocs` is the local developer orchestrator — no Docker Compose or similar at this stage.

---

## Coding Standards

### Rust (Agent)
- Use `async/await` with Tokio for all I/O operations.
- Use `serde` with `#[derive(Serialize, Deserialize)]` for all structs that cross service boundaries.
- Error handling: return `Result<T, String>` or `Result<T, Box<dyn std::error::Error>>`. Do not panic in production paths.
- Module organization: one file per logical module (`log_collector.rs`, `process_monitor.rs`, `command_executor.rs`), declared with `mod` in `main.rs`.
- Use `env_logger` for logging; `println!` is acceptable for MVP debug output.
- Prefix log output with module name in brackets: `[LogCollector]`, `[ProcessMonitor]`, `[SIMULATION]`.

### Go (Backend)
- Standard library only — no third-party HTTP frameworks.
- All structs use JSON tags: `json:"field_name"`.
- Spawn goroutines for non-blocking analysis: `go analyzeLog(entry)`.
- Handler functions follow the pattern: validate method -> decode JSON -> process -> respond.
- Use `log.Printf` for all output (not `fmt.Println`).
- Alert output prefix convention: `[ALERT]`, `[INFO]`, `[HEARTBEAT]`.

### Python (Brain)
- Plain scripts — no heavy frameworks (no FastAPI, no Django).
- Class-based organization: `LogParser`, `AnomalyModel`, `LLMClient`, `LogHandler`.
- Static methods preferred for stateless operations.
- Use `./venv/bin/python` to run — never the system Python.
- Output prefix convention: `[BRAIN]` for all print statements.

### Language of Documentation
- Strategy/business docs: Spanish (Chilean market focus).
- Code comments: English.
- This file: English.

---

## Key Components

### Agent (`src/edr/agent/`)

| File | Responsibility |
|---|---|
| `main.rs` | Spawns LogCollector, ProcessMonitor, and CommandExecutor simulation as Tokio tasks. Runs heartbeat loop every 5 seconds. |
| `log_collector.rs` | Opens log file, seeks to EOF, polls for new lines every 500ms. Prints new lines prefixed with `[LogCollector]`. Does not yet POST to backend. |
| `process_monitor.rs` | Takes initial PID snapshot, polls every 2s, reports new PIDs. Does not yet POST to backend. |
| `command_executor.rs` | Executes `AgentCommand::KillProcess(u32)` via `sysinfo` and `AgentCommand::BlockIp(String)` via `iptables` CLI. |

Important: LogCollector and ProcessMonitor currently only print locally. The pipeline to POST events to the backend (`POST /logs`, `POST /processes`) is not yet implemented in these modules. Only the heartbeat is wired to the backend.

### Backend (`src/edr/backend/main.go`)

Three endpoints, all POST only:

| Endpoint | Payload | Action |
|---|---|---|
| `POST /heartbeat` | `{hostname, timestamp, os}` | Logs host name and OS, returns `ACK` |
| `POST /logs` | `{hostname, log, timestamp}` | Async keyword scan; alerts on: failed, error, sudo, root, unauthorized |
| `POST /processes` | `{hostname, pid, name, cmd}` | Async name scan; alerts on: nc, ncat, cryptominer, mimikatz |

### Brain (`src/edr/brain/main.py`)

Pipeline for each new log line:
1. `LogParser.parse(line)` — normalizes to `{raw, timestamp}`.
2. `AnomalyModel.check(data)` — heuristic check (line length > 100 or keyword match).
3. If anomaly: `LLMClient.analyze(raw)` — sends to Ollama with 2s timeout, falls back to mock string.

---

## Development Guidelines

### Adding a New Endpoint to the Backend
1. Define a struct with JSON tags in `main.go`.
2. Write a handler function following the existing pattern (method check, decode, async analyze, respond).
3. Register with `http.HandleFunc("/path", handlerFunc)` in `main()`.

### Adding a New Agent Module
1. Create `src/edr/agent/src/new_module.rs`.
2. Declare it in `main.rs` with `mod new_module;`.
3. Implement a `run(&self) -> ()` async method following the `LogCollector` or `ProcessMonitor` pattern.
4. Spawn it in `main()` with `tokio::spawn(async move { module.run().await; })`.

### Adding New Keyword Detection
- Backend log keywords: edit the `keywords` slice in `analyzeLog()` in `main.go`.
- Backend process keywords: edit the `badProcs` slice in `analyzeProcess()` in `main.go`.
- Brain anomaly keywords: edit the `suspicious` slice in `AnomalyModel.check()` in `main.py`.

### Connecting Agent Modules to the Backend
The next logical step is wiring LogCollector and ProcessMonitor to POST their findings to `POST /logs` and `POST /processes`. Follow the heartbeat pattern in `main.rs` using `reqwest::Client`.

### Python Dependencies
Always activate or use the venv: `src/edr/brain/venv/bin/python` and `src/edr/brain/venv/bin/pip`.

### Testing
No formal test suite exists at this stage (Phase 1 MVP). Manual testing procedure:
```bash
# Generate test log entries to trigger the Brain pipeline
echo "sudo failed unauthorized" >> /tmp/nsoc_test.log
```

---

## Current Development State

### Completed (Phase 1 MVP)
- Rust agent: heartbeat loop posting to Go backend every 5 seconds.
- Rust agent: LogCollector module (tails file, prints new lines locally).
- Rust agent: ProcessMonitor module (detects new PIDs, prints locally).
- Rust agent: CommandExecutor with KillProcess (via sysinfo) and BlockIP (via iptables).
- Go backend: REST API with /heartbeat, /logs, /processes endpoints.
- Go backend: Keyword-based synchronous alert detection with async goroutine dispatch.
- Python brain: File-watch pipeline via watchdog.
- Python brain: Heuristic anomaly detection (AnomalyModel).
- Python brain: Ollama LLM integration with mock fallback (LLMClient).
- mprocs orchestration via `start_edr.sh`.

### In Progress / Immediate Next Steps
- Wiring LogCollector and ProcessMonitor to POST telemetry to the backend (the data pipeline gap).
- Connecting Brain output to Backend (closing the Agent -> Backend -> Brain -> Action loop).

### Not Started
- Frontend dashboard (Vue 3 + TypeScript): AgentList, ThreatMap, InvestigationView.
- Database persistence (PostgreSQL for config/tenants, Elasticsearch for logs).
- mTLS / authentication between Agent and Backend.
- Autonomous response: Brain triggering commands back to Agent via Backend.
- Multi-tenant isolation.
- NotificationService (Email/Slack integration in Backend).
- Product 2: ZTNA / Identity Risk Engine.
- Product 3: Cloud Security Autopilot.
- gRPC migration (post-MVP).
- NATS JetStream messaging (post-MVP).

### Known Issues / Technical Debt
- `main.rs` contains a hardcoded simulation block (`[SIMULATION]`) that spawns a dummy `sleep 100` process and tests CommandExecutor on startup. This is test scaffolding, not production behavior.
- BlockIP validation in `command_executor.rs` only checks for three dots in the IP string — not a proper IPv4 validator.
- Brain's `LogHandler` opens the file in its `__init__` and never closes it — acceptable for MVP, needs cleanup for production.
- Go module path in `go.mod` is `nsoc/edr/backend` — uses a relative-style path, not a real domain (e.g., `github.com/org/nsoc`). Should be updated before open-sourcing or deploying.
- No authentication on any backend endpoint. Any host on the network can POST to port 8080.

---

## Important Commands

```bash
# Run all three services simultaneously (recommended)
# Requires mprocs to be installed
./start_edr.sh

# Run backend only
cd src/edr/backend && go run main.go

# Run brain only
cd src/edr/brain && ./venv/bin/python main.py

# Run agent only
cd src/edr/agent && cargo run

# Build agent binary
cd src/edr/agent && cargo build

# Install Python dependencies (brain)
cd src/edr/brain && ./venv/bin/pip install -r requirements.txt

# Generate test log events for brain pipeline
echo "sudo failed login attempt" >> /tmp/nsoc_test.log

# Install mprocs (if not present, Ubuntu/Debian)
# Check: https://github.com/pvolok/mprocs for install instructions
```

---

## Service Endpoints and External Dependencies

| Service | Address | Notes |
|---|---|---|
| Backend API | `http://localhost:8080` | Go REST API |
| Ollama LLM | `http://localhost:11434/api/generate` | Must be running; Brain has mock fallback |
| Agent log source | `/var/log/syslog` or `/tmp/nsoc_test.log` | Falls back to tmp if syslog absent |
| Brain log watch | `/tmp/nsoc_test.log` | Created if not present |

---

## Notes for AI Assistants

1. **Do not modify strategy/business documents** (`GEMINI.md`, `plan_desarrollo/`, `estrategia/`, `negocio/`, `financiamiento/`, `analisis/`) unless explicitly asked. These are planning and funding documents.

2. **Current critical gap**: LogCollector and ProcessMonitor collect data but do not send it to the backend. The heartbeat in `main.rs` is the only agent-to-backend communication path. Any work that closes this gap is high priority.

3. **Brain and Backend are not yet connected**. Brain watches `/tmp/nsoc_test.log` independently. The intended architecture has Backend forwarding events to Brain and Brain sending responses back to trigger Agent commands. This loop is not implemented.

4. **BlockIP requires root**. The CommandExecutor will fail with a permission error when run as a normal user. This is expected and acknowledged in the code comments.

5. **Ollama is optional for development**. The Brain falls back gracefully to a mock string if Ollama is not running. Do not treat a missing Ollama as a blocker for local dev.

6. **No tests exist yet**. When implementing features, manual testing via appending to `/tmp/nsoc_test.log` is the current method. Adding a test suite (e.g., `cargo test` for Rust, `go test` for Go) is a future concern.

7. **Go backend is intentionally a monolith** (`main.go` only). Per the stack strategy, microservice splitting is deferred until scale requires it. Do not refactor into packages without explicit request.

8. **LLM model names**: Brain uses `llama3` in the Ollama payload. `mistral` is also mentioned in a comment as a viable alternative. The model must be pulled in Ollama before use (`ollama pull llama3`).

9. **Hardware note**: Running a 7B quantized model locally requires 16GB+ RAM. Factor this into development environment expectations.
