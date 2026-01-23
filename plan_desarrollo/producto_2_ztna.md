# Plan de Desarrollo: ZTNA Inteligente (Low Resource)

## 1. Estrategia de Producto: "Acceso Adaptativo"
Las VPNs son tontas (si tienes la llave, entras). Nuestro ZTNA usa IA para entender el contexto del usuario.

*   **Core**: NetBird / Headscale.
*   **Diferenciador IA**: "Identity Risk Engine". Un servicio paralelo que monitorea los logs de acceso de la red overlay.

## 2. Plan de Implementación
**Fase 1: Conectividad Base (Mes 1)**
*   Despliegue estándar de NetBird/Headscale.
*   Asegurar que los logs de conexión (quién, desde qué IP, a qué recurso) se guarden centralizadamente.

**Fase 2: UEBA (User and Entity Behavior Analytics) "Light" (Mes 2)**
*   Desarrollar un "vigilante" (Python script) que use un modelo de Random Forest (Scikit-learn, muy liviano) entrenado con los logs.
*   **Features**: Hora del día, GeoIP, Volumen de Datos, Servidor Destino.
*   Entrenar modelo con 30 días de datos "limpios".
*   Detectar outliers: "¿Por qué el usuario de Marketing está intentando acceder por SSH al servidor de Base de Datos?".

**Fase 3: Bloqueo Dinámico (Mes 3)**
*   Si el "Risk Score" sube de 80, el script llama a la API del Control Plane y deshabilita la llave del usuario temporalmente.
*   Notificación vía Slack/Teams al admin: "Usuario bloqueado por comportamiento anómalo. ¿Aprobar acceso?".

## 3. Requerimientos
*   **Hardware**: Mismo VPS del controlador ZTNA. El modelo de Machine Learning clásico (Random Forest) consume muy poca CPU/RAM.
*   **Stack**: Python, Pandas, Scikit-learn.
