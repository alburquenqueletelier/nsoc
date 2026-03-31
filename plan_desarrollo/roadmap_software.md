# Software Development Roadmap (Technical)

This document details the software modules to build for each product, assigning the technologies defined in the tech stack.

## 1. EDR Product: Sentinel AI

### A. Endpoint Agent (Rust)
*   **Responsibility**: Telemetry collection and response execution.
*   **Modules**:
    *   `LogCollector`: Efficient reading of system logs (Syslog/Windows Event Log).
    *   `ProcessMonitor`: Monitoring of process creation and network usage.
    *   `CommandExecutor`: Execution of mitigation actions (e.g., block IP via iptables, kill process) received from the Backend.
    *   `Heartbeat`: Secure communication (mTLS) with the Backend.

### B. Analysis Engine (Python)
*   **Responsibility**: Rule-based and AI-driven anomaly detection.
*   **Modules**:
    *   `LogParser`: Normalization of received events.
    *   `AnomalyModel`: Implementation of Isolation Forest / basic Autoencoder to detect statistical deviations.
    *   `LLMClient`: Interface with Ollama/Llama-3 for semantic analysis (e.g., "Explain this suspicious command line").

### C. Backend (Go)
*   **Responsibility**: Agent management and alert orchestration.
*   **Modules**:
    *   `AgentAPI`: REST endpoints to receive telemetry from agents.
    *   `AlertManager`: Alert deduplication and severity logic.
    *   `NotificationService`: Email/Slack integration.

### D. Frontend (Vue 3 + TS)
*   **Components**:
    *   `AgentList`: Table showing connected sensor status.
    *   `ThreatMap`: Active alert visualization.
    *   `InvestigationView`: Contextual AI chat about a specific alert.

---

## 2. ZTNA Product: Identity Risk Engine

### A. Network Sensor (Rust/Go — Integration)
*   **Note**: Here we'll primarily use **NetBird (Go)** integration, but we may need a Rust 'sidecar' if deep packet inspection is required.
*   **Custom Module**:
    *   `NetLogShipper`: Lightweight service that sends connection logs (NetBird) to the analysis engine in real time.

### B. Risk Engine (Python)
*   **Responsibility**: Calculate each user's "Trust Score".
*   **Modules**:
    *   `UEBA_Engine`: Model (Random Forest) trained with: Time, GeoIP, Device, Resource.
    *   `PolicyEnforcer`: If Score < Threshold -> Call NetBird API to block peer.

### C. Backend (Go)
*   **Responsibility**: API for policy configuration and NetBird proxy.
*   **Modules**:
    *   `PolicyAPI`: CRUD for risk rules (e.g., "Block if accessing from country X").
    *   `UserSync`: User synchronization with IdP (Google/Microsoft).

---

## 3. Cloud Security Product: Remediator

### A. Scanner Runner (Go)
*   **Responsibility**: Efficient execution of third-party tools (Prowler).
*   **Modules**:
    *   `JobScheduler`: Job queue for running scheduled scans.
    *   `ProwlerWrapper`: Controlled execution of Prowler and JSON output capture.

### B. Remedy Generator (Python)
*   **Responsibility**: Translate findings into IaC code.
*   **Modules**:
    *   `PromptBuilder`: Context construction for the LLM (Finding + Infrastructure Context).
    *   `CodeCleaner`: Basic validation of generated Terraform code (linter).

### C. Frontend (Vue 3 + TS)
*   **Components**:
    *   `CloudHealthDashboard`: Compliance charts (CIS Benchmark).
    *   `RemediationCenter`: List of findings with "Generate Fix" button and integrated code editor (Monaco Editor) to preview the Terraform.
