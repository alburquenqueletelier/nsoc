#!/bin/bash

# Ensure we are in the project root
cd "$(dirname "$0")"

# Check if mprocs is installed
if ! command -v mprocs &> /dev/null; then
    echo "mprocs could not be found. Please install it first."
    exit 1
fi

echo "Starting NSOC EDR with mprocs..."

# Launch components
# 1. Backend (Go) - Port 8080
# 2. Brain (Python) - Analyzes logs
# 3. Agent (Rust) - Generates telemetry
mprocs \
    "cd src/edr/backend && go run main.go" \
    "cd src/edr/brain && ./venv/bin/python main.py" \
    "cd src/edr/agent && cargo run"
