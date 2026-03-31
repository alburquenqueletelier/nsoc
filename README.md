# NSOC — AI-Driven SOC for SMBs

NSOC is a cybersecurity platform that automates Level-1 SOC analyst work for small and medium businesses in Latin America. It uses local open-source LLMs (Ollama) for threat analysis — no cloud dependency for inference.

**Current product: Sentinel AI** — an AI-driven EDR (Endpoint Detection and Response) that detects threats on endpoints, analyzes them with heuristics + LLM, and can autonomously respond (kill malicious processes, block IPs).

## Architecture

```
┌─────────────────────┐
│  Endpoint Agent     │     POST /logs, /processes, /heartbeat
│  (Rust)             │─────────────────────────────────┐
│                     │                                 │
│  LogCollector       │     GET /commands?hostname=X    │
│  ProcessMonitor     │◄────────────────────────────────┤
│  CommandExecutor    │                                 │
└─────────────────────┘                                 │
                                                        ▼
                                             ┌──────────────────┐
                                             │  Backend (Go)    │
                                             │  :8080           │
                                             │                  │──► PostgreSQL :5432
                                             │  /heartbeat      │
                                             │  /logs ──────────┼──► Brain :5000/analyze
                                             │  /processes ─────┼──► Brain :5000/analyze
                                             │  /commands (GET) │
                                             │  /alerts  (GET)  │
                                             └──────────────────┘
                                                        │
                                                        ▼
                                             ┌──────────────────┐
                                             │  Brain (Python)  │
                                             │  :5000           │
                                             │                  │
                                             │  Heuristics      │
                                             │  Ollama LLM      │──► localhost:11434
                                             │  Severity Rating │
                                             │  Action Suggest  │
                                             └──────────────────┘
```

**Data flow:** Agent collects logs and processes from the endpoint, POSTs them to the Go Backend. Backend runs keyword detection, then forwards to the Python Brain for AI analysis (heuristics + LLM). If a threat is confirmed, the Brain recommends an action (e.g., kill process). The Backend enqueues the command, and the Agent polls for it and executes it. All telemetry and alerts are persisted in PostgreSQL.

## Tech Stack

| Component | Technology | Why |
|-----------|-----------|-----|
| Agent | Rust + Tokio | Memory-safe, zero-GC, runs on client endpoints |
| Backend | Go (stdlib `net/http`) | Goroutine concurrency, single static binary |
| Brain | Python (stdlib `http.server`) | Access to ML/LLM ecosystem |
| Database | PostgreSQL 17 | ACID, `jsonb` for flexible payloads |
| LLM | Ollama (llama3 / mistral) | Local inference, no cloud dependency |
| Orchestration | mprocs / Docker Compose | Dev environment management |

## Requirements

### System

- Linux (tested on Debian 13 / Ubuntu)
- 16GB+ RAM (if running Ollama with 7B models)
- Docker and Docker Compose

### Toolchain

| Tool | Version | Install |
|------|---------|---------|
| Rust | edition 2021 | [rustup.rs](https://rustup.rs) |
| Go | 1.21+ | [go.dev/dl](https://go.dev/dl) |
| Python | 3.10+ | System package or [pyenv](https://github.com/pyenv/pyenv) |
| Docker | 20.10+ | [docs.docker.com](https://docs.docker.com/engine/install/) |
| Docker Compose | v2+ | Included with Docker Desktop / `docker compose` |
| mprocs | latest | [github.com/pvolok/mprocs](https://github.com/pvolok/mprocs) |
| Ollama | latest | [ollama.com](https://ollama.com) (optional — Brain has mock fallback) |

## Installation

### 1. Clone the repository

```bash
git clone <repo-url> nsoc
cd nsoc
```

### 2. Start PostgreSQL

```bash
docker compose up -d
```

This starts a PostgreSQL 17 container on port 5432 (user: `nsoc`, password: `nsoc`, database: `nsoc`). The Go Backend auto-creates all tables on first startup.

### 3. Set up the Python Brain

```bash
cd src/edr/brain
python3 -m venv venv
./venv/bin/pip install -r requirements.txt
cd ../../..
```

### 4. Install Go dependencies

```bash
cd src/edr/backend
go mod tidy
cd ../../..
```

### 5. Build the Rust Agent

```bash
cd src/edr/agent
cargo build
cd ../../..
```

### 6. (Optional) Pull an Ollama model

```bash
ollama pull llama3
```

If Ollama is not running, the Brain falls back to a mock analysis string. This does not block development.

## Usage (Development)

### Option A: Run all services with mprocs (recommended)

```bash
./start_edr.sh
```

This opens a terminal multiplexer with all three services running side by side. Press `q` to quit.

**Note:** Set `DATABASE_URL` before running if your PostgreSQL is not on the default `localhost:5432`:

```bash
export DATABASE_URL="postgres://nsoc:nsoc@localhost:5432/nsoc?sslmode=disable"
./start_edr.sh
```

### Option B: Run services individually

In separate terminals:

```bash
# Terminal 1 — Backend
cd src/edr/backend && go run main.go

# Terminal 2 — Brain
cd src/edr/brain && ./venv/bin/python main.py

# Terminal 3 — Agent
cd src/edr/agent && cargo run
```

### Testing the pipeline

```bash
# Trigger a log alert (Agent reads this file, sends to Backend, Backend forwards to Brain)
echo "sudo failed unauthorized login attempt" >> /tmp/nsoc_test.log

# Check alerts stored in the database
curl -s http://localhost:8080/alerts | python3 -m json.tool

# Check pending commands for an agent
curl -s "http://localhost:8080/commands?hostname=$(hostname)" | python3 -m json.tool

# Simulate a malicious process detection directly
curl -s -X POST http://localhost:8080/processes \
  -H 'Content-Type: application/json' \
  -d '{"hostname":"test","pid":1234,"name":"cryptominer","cmd":"./cryptominer --mine"}'

# Verify the kill command was enqueued
curl -s "http://localhost:8080/commands?hostname=test" | python3 -m json.tool
```

### Inspect the database directly

```bash
docker exec -it nsoc-postgres psql -U nsoc -d nsoc

# Useful queries:
SELECT * FROM alerts ORDER BY created_at DESC LIMIT 10;
SELECT * FROM agents;
SELECT * FROM log_events ORDER BY received_at DESC LIMIT 10;
SELECT * FROM commands WHERE dispatched = FALSE;
```

## Service Endpoints

| Service | Address | Notes |
|---------|---------|-------|
| Backend API | `http://localhost:8080` | REST API |
| Brain API | `http://localhost:5000` | POST /analyze only |
| PostgreSQL | `localhost:5432` | User: nsoc, DB: nsoc |
| Ollama | `http://localhost:11434` | Optional |

## Project Structure

```
nsoc/
├── README.md
├── CLAUDE.md                 # AI assistant context
├── GEMINI.md                 # Original project vision (by Gemini)
├── MVP_PLAN.md               # Development plan
├── docker-compose.yml        # PostgreSQL 17
├── start_edr.sh              # Launch all services via mprocs
│
├── src/edr/
│   ├── agent/                # Rust endpoint agent
│   │   └── src/
│   │       ├── main.rs             # Heartbeat loop + command polling
│   │       ├── log_collector.rs    # Tails logs → POST /logs
│   │       ├── process_monitor.rs  # Detects new PIDs → POST /processes
│   │       └── command_executor.rs # Kills processes, blocks IPs
│   │
│   ├── backend/              # Go REST API
│   │   └── main.go           # All routes, Brain forwarding, DB persistence
│   │
│   └── brain/                # Python AI engine
│       └── main.py           # Heuristics + Ollama LLM + HTTP API on :5000
│
├── plan_desarrollo/          # Product strategy docs (English)
├── estrategia/               # Market positioning
├── negocio/                  # Business/scaling strategy
├── financiamiento/           # CORFO funding plan
└── analisis/                 # Competitive analysis
```

## License

Proprietary. All rights reserved.
