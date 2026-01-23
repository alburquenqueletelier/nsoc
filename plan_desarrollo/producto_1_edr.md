# Plan de Desarrollo: AI-Driven EDR (Low Resource)

## 1. Estrategia de Producto: "El Sistema Inmune Digital"
Inspirado en **Darktrace**, no venderemos solo "bloqueo de virus", sino una **IA que aprende el comportamiento normal** de la PYME y reacciona ante anomalías.

*   **Core**: Wazuh (Recolección de datos y reglas base).
*   **Diferenciador IA**: Módulo "Sentinel AI" (Script propio).
    *   Uso de modelos **LLM locales (Ollama/Mistral)** para analizar logs en lenguaje natural y explicar incidentes.
    *   Análisis estadístico simple (Media/Desviación estándar) para detectar anomalías de tráfico o procesos.

## 2. Plan de Implementación
**Fase 1: Infraestructura + Ingesta (Mes 1)**
*   Instalar Wazuh Manager.
*   **Agregado IA**: Instalar **Ollama** en el mismo servidor (o uno secundario con GPU barata).
*   Crear un script Python que consuma la API de Wazuh y envíe alertas sospechosas al LLM: *"Analiza este log JSON y dime si parece un ataque de movimiento lateral. Responde SI/NO y por qué"*.

**Fase 2: "Normal Behavior Learning" (Mes 2)**
*   El sistema pasa 2 semanas en "modo aprendizaje" (solo lectura).
*   Script calcula líneas base: "¿A qué hora se conecta usualmente el Gerente de Finanzas?".
*   Si se conecta a las 3 AM -> **Alerta de Anomalía**. (Esto es lo que vende Darktrace).

**Fase 3: Respuesta Autónoma (Mes 3)**
*   Conectar la salida del módulo de IA con la capacidad de respuesta activa de Wazuh.
*   Si IA Confidence > 90% -> Aislar equipo automáticamente.

## 3. Requerimientos IA
*   **Hardware**: VPS con al menos 16GB RAM (para correr modelos cuantizados 7B como Mistral). Costo sube un poco (~40-60 USD), pero el valor de venta se duplica.
*   **Tecnología**: Python, LangChain (para orquestar el LLM), Wazuh API.
