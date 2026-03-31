# Development Plan: Smart ZTNA (Low Resource)

## 1. Product Strategy: "Adaptive Access"
VPNs are dumb (if you have the key, you're in). Our ZTNA uses AI to understand the user's context.

*   **Core**: NetBird / Headscale.
*   **AI Differentiator**: "Identity Risk Engine". A parallel service that monitors the access logs of the overlay network.

## 2. Implementation Plan
**Phase 1: Base Connectivity (Month 1)**
*   Standard deployment of NetBird/Headscale.
*   Ensure connection logs (who, from which IP, to which resource) are stored centrally.

**Phase 2: UEBA (User and Entity Behavior Analytics) "Light" (Month 2)**
*   Develop a "watchdog" (Python script) that uses a Random Forest model (Scikit-learn, very lightweight) trained on the logs.
*   **Features**: Time of day, GeoIP, Data Volume, Destination Server.
*   Train model with 30 days of "clean" data.
*   Detect outliers: "Why is the Marketing user trying to SSH into the Database server?"

**Phase 3: Dynamic Blocking (Month 3)**
*   If the "Risk Score" goes above 80, the script calls the Control Plane API and temporarily disables the user's key.
*   Notification via Slack/Teams to admin: "User blocked due to anomalous behavior. Approve access?"

## 3. Requirements
*   **Hardware**: Same VPS as the ZTNA controller. The classic Machine Learning model (Random Forest) consumes very little CPU/RAM.
*   **Stack**: Python, Pandas, Scikit-learn.
