# Definición del Stack Tecnológico

Este documento define la arquitectura técnica y las tecnologías seleccionadas para el desarrollo de la plataforma NSOC. La elección prioriza rendimiento, seguridad y mantenibilidad.

## 1. Topología del Stack

| Capa | Tecnología | Justificación Técnica |
| :--- | :--- | :--- |
| **Sensor de Red / Agente** | **Rust** | **Seguridad y Rendimiento**. Al ejecutarse en el endpoint del cliente, no puede fallar (seguridad de memoria de Rust) ni consumir recursos excesivos (sin Garbage Collector). Ideal para manejo de bajo nivel de red. |
| **Motor de IA** | **Python** | **Estándar de la Industria**. Acceso nativo a bibliotecas de ML (PyTorch, Scikit-learn, LangChain) y facilidad para prototipar lógica heurística compleja. |
| **Backend / API** | **Go (Golang)** | **Concurrencia y Despliegue**. Excelente manejo de múltiples conexiones simultáneas (goroutines) y compilación a binario estático único, facilitando el despliegue en cualquier servidor Linux. |
| **Frontend / GUI** | **TypeScript + Vue 3** | **Reactividad y DX**. Vue 3 (Composition API) ofrece un equilibrio perfecto entre rendimiento y facilidad de desarrollo. TypeScript añade la robustez necesaria para una aplicación de seguridad crítica. |
| **Persistencia (Datos)** | **PostgreSQL** | **Relacional Robusto**. Para gestión de usuarios, inquilinos (tenants), configuraciones y relaciones entre activos. |
| **Persistencia (Logs)** | **Elasticsearch** | **Búsqueda y Logs**. Motor probado para ingesta masiva de eventos de seguridad y búsquedas full-text rápidas. |
| **Comunicación** | **REST (MVP) / gRPC (Futuro)** | Inicio simple con REST para iteración rápida. Migración a gRPC para comunicación eficiente entre microservicios cuando escale. |
| **Mensajería** | **Directa (MVP) / NATS (Futuro)** | Inicio con llamadas directas (HTTP). Introducción de NATS JetStream para desacoplar servicios y manejar picos de carga en el futuro. |

## 2. Diagrama de Arquitectura (Alto Nivel)

```mermaid
graph TD
    subgraph "Cliente (Pyme)"
        A[Sensor Rust] -->|Logs/Alertas| B(Load Balancer)
        C[Browser / GUI] -->|HTTPS| B
    end

    subgraph "Cloud NSOC (Backend)"
        B --> D[API Gateway / Backend (Go)]
        D -->|Consultas| E[(PostgreSQL)]
        D -->|Logs Masivos| F[(Elasticsearch)]
        
        D -->|Análisis Asíncrono| G[Motor IA (Python)]
        G -->|Resultados| D
    end
```

## 3. Estrategia de MVP (Minimum Viable Product)
Para la primera versión funcional, simplificaremos la arquitectura ("Keep It Simple"):
1.  **Sin Bus de Mensajes**: El Backend (Go) llamará al Motor IA (Python) vía HTTP síncrono o workers simples. NATS/Kafka se añaden solo cuando el tráfico lo exija.
2.  **REST sobre gRPC**: Todas las comunicaciones internas serán REST JSON para facilitar depuración y desarrollo rápido.
3.  **Monolito Modular en Go**: En lugar de microservicios puros, el backend será un solo binario bien estructurado, separando dominios lógicos.

## 4. Justificación de Decisiones Clave

### Por qué Rust en el Agente?
Los agentes EDR corren en el kernel o cerca de él. Un "Panico" en Go o una excepción en Python podrían ser fatales o costosos en recursos. Rust garantiza `memory safety` sin overhead.

### Por qué Python para IA?
Aunque Go/Rust son rápidos, la ecosistema de IA (Hugging Face, Drivers de GPU, Librerías de Tensores) vive en Python. No reinventaremos la rueda.

### Por qué Vue 3 y no React?
Preferencia por la separación clara de HTML/JS/CSS y la reactividad fina de la Composition API, que encaja bien con paneles de control de datos en tiempo real.
