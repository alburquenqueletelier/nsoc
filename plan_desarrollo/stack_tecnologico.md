# Technology Stack Definition

This document defines the technical architecture and selected technologies for the NSOC platform. The choices prioritize performance, security, and maintainability.

## 1. Stack Topology

| Layer | Technology | Technical Justification |
| :--- | :--- | :--- |
| **Network Sensor / Agent** | **Rust** | **Security and Performance**. Running on the client endpoint, it cannot crash (Rust's memory safety) or consume excessive resources (no Garbage Collector). Ideal for low-level network handling. |
| **AI Engine** | **Python** | **Industry Standard**. Native access to ML libraries (PyTorch, Scikit-learn, LangChain) and ease of prototyping complex heuristic logic. |
| **Backend / API** | **Go (Golang)** | **Concurrency and Deployment**. Excellent handling of multiple simultaneous connections (goroutines) and compilation to a single static binary, making deployment easy on any Linux server. |
| **Frontend / GUI** | **TypeScript + Vue 3** | **Reactivity and DX**. Vue 3 (Composition API) offers a perfect balance between performance and development ease. TypeScript adds the robustness needed for a security-critical application. |
| **Persistence (Data)** | **PostgreSQL** | **Robust Relational**. For user management, tenants, configurations, and relationships between assets. |
| **Persistence (Logs)** | **Elasticsearch** | **Search and Logs**. Proven engine for massive security event ingestion and fast full-text searches. |
| **Communication** | **REST (MVP) / gRPC (Future)** | Simple start with REST for fast iteration. Migration to gRPC for efficient inter-service communication at scale. |
| **Messaging** | **Direct (MVP) / NATS (Future)** | Start with direct calls (HTTP). Introduction of NATS JetStream to decouple services and handle load spikes in the future. |

## 2. Architecture Diagram (High Level)

```mermaid
graph TD
    subgraph "Client (SMB)"
        A[Rust Sensor] -->|Logs/Alerts| B(Load Balancer)
        C[Browser / GUI] -->|HTTPS| B
    end

    subgraph "NSOC Cloud (Backend)"
        B --> D[API Gateway / Backend (Go)]
        D -->|Queries| E[(PostgreSQL)]
        D -->|Massive Logs| F[(Elasticsearch)]

        D -->|Async Analysis| G[AI Engine (Python)]
        G -->|Results| D
    end
```

## 3. MVP Strategy (Minimum Viable Product)
For the first functional version, we simplify the architecture ("Keep It Simple"):
1.  **No Message Bus**: The Backend (Go) will call the AI Engine (Python) via synchronous HTTP or simple workers. NATS/Kafka is added only when traffic demands it.
2.  **REST over gRPC**: All internal communications will be REST JSON for easy debugging and rapid development.
3.  **Modular Monolith in Go**: Instead of pure microservices, the backend will be a single well-structured binary, separating logical domains.

## 4. Key Decision Justifications

### Why Rust for the Agent?
EDR agents run at or near the kernel level. A "Panic" in Go or an exception in Python could be fatal or resource-costly. Rust guarantees `memory safety` without overhead.

### Why Python for AI?
Although Go/Rust are fast, the AI ecosystem (Hugging Face, GPU Drivers, Tensor Libraries) lives in Python. We won't reinvent the wheel.

### Why Vue 3 and not React?
Preference for the clear separation of HTML/JS/CSS and the fine-grained reactivity of the Composition API, which fits well with real-time data control panels.
