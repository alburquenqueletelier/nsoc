# Roadmap de Desarrollo de Software (Técnico)

Este documento detalla los módulos de software a construir para cada producto, asignando las tecnologías definidas en el stack tecnológico.

## 1. Producto EDR: Sentinel AI

### A. Agente de Endpoint (Rust)
*   **Responsabilidad**: Recolección de telemetría y ejecución de respuestas.
*   **Módulos**:
    *   `LogCollector`: Lectura eficiente de logs del sistema (Syslog/Windows Event Log).
    *   `ProcessMonitor`: Monitoreo de creación de procesos y uso de red.
    *   `CommandExecutor`: Ejecución de acciones de mitigación (ej. bloquear IP vía iptables, matar proceso) recibidas del Backend.
    *   `Heartbeat`: Comunicación segura (mTBS) con el Backend.

### B. Motor de Análisis (Python)
*   **Responsabilidad**: Detección de anomalías basada en reglas e IA.
*   **Módulos**:
    *   `LogParser`: Normalización de eventos recibidos.
    *   `AnomalyModel`: Implementación de Isolation Forest / Autoencoder básico para detectar desviaciones estadísticas.
    *   `LLMClient`: Interfaz con Ollama/Llama-3 para análisis semántico (ej. "Explica esta línea de comando sospechosa").

### C. Backend (Go)
*   **Responsabilidad**: Gestión de agentes y orquestación de alertas.
*   **Módulos**:
    *   `AgentAPI`: Endpoints REST para recibir telemetría de agentes.
    *   `AlertManager`: Lógica de deduplicación y severidad de alertas.
    *   `NotificationService`: Integración con Email/Slack.

### D. Frontend (Vue 3 + TS)
*   **Componentes**:
    *   `AgentList`: Tabla de estado de sensores conectados.
    *   `ThreatMap`: Visualización de alertas activas.
    *   `InvestigationView`: Chat contextual con la IA sobre una alerta específica.

---

## 2. Producto ZTNA: Identity Risk Engine

### A. Sensor de Red (Rust/Go - Integración)
*   **Nota**: Aquí usaremos principalmente la integración con **NetBird (Go)**, pero podríamos necesitar un 'sidecar' en Rust si requerimos inspección de paquetes profunda.
*   **Módulo Custom**:
    *   `NetLogShipper`: Servicio ligero que envía logs de conexión (NetBird) al motor de análisis en tiempo real.

### B. Motor de Riesgo (Python)
*   **Responsabilidad**: Calcular el "Trust Score" de cada usuario.
*   **Módulos**:
    *   `UEBA_Engine`: Modelo (Random Forest) entrenado con: Hora, GeoIP, Dispositivo, Recurso.
    *   `PolicyEnforcer`: Si Score < Umbral -> Llamar API de NetBird para bloquear peer.

### C. Backend (Go)
*   **Responsabilidad**: API para configuración de políticas y proxy de NetBird.
*   **Módulos**:
    *   `PolicyAPI`: CRUD de reglas de riesgo (ej. "Bloquear si accede desde país X").
    *   `UserSync`: Sincronización de usuarios con IdP (Google/Microsoft).

---

## 3. Producto Cloud Sec: Remediator

### A. Scanner Runner (Go)
*   **Responsabilidad**: Ejecución eficiente de herramientas de terceros (Prowler).
*   **Módulos**:
    *   `JobScheduler`: Cola de trabajos para ejecutar escaneos programados.
    *   `ProwlerWrapper`: Ejecución controlada de Prowler y captura de salida JSON.

### B. Generador de Remedios (Python)
*   **Responsabilidad**: Traducir hallazgos a código IaC.
*   **Módulos**:
    *   `PromptBuilder`: Construcción de contextos para el LLM (Hallazgo + Contexto de Infraestructura).
    *   `CodeCleaner`: Validación básica del código Terraform generado (linter).

### C. Frontend (Vue 3 + TS)
*   **Componentes**:
    *   `CloudHealthDashboard`: Gráficos de cumplimiento (CIS Benchmark).
    *   `RemediationCenter`: Lista de fallos con botón "Generar Fix" y editor de código integrado (Monaco Editor) para previsualizar el Terraform.
