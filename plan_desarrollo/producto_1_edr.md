# Development Plan: AI-Driven EDR (Low Resource)

## 1. Product Strategy: "The Digital Immune System"
Inspired by **Darktrace**, we won't just sell "virus blocking" — we'll sell an **AI that learns the normal behavior** of the SMB and reacts to anomalies.

*   **Core**: Wazuh (Data collection and base rules).
*   **AI Differentiator**: "Sentinel AI" module (custom script).
    *   Use of **local LLM models (Ollama/Mistral)** to analyze logs in natural language and explain incidents.
    *   Simple statistical analysis (Mean/Standard Deviation) to detect traffic or process anomalies.

## 2. Implementation Plan
**Phase 1: Infrastructure + Ingestion (Month 1)**
*   Install Wazuh Manager.
*   **AI Addition**: Install **Ollama** on the same server (or a secondary one with a cheap GPU).
*   Create a Python script that consumes the Wazuh API and sends suspicious alerts to the LLM: *"Analyze this JSON log and tell me if it looks like a lateral movement attack. Answer YES/NO and why."*

**Phase 2: "Normal Behavior Learning" (Month 2)**
*   The system spends 2 weeks in "learning mode" (read-only).
*   Script calculates baselines: "What time does the Finance Manager usually connect?"
*   If they connect at 3 AM -> **Anomaly Alert**. (This is what Darktrace sells).

**Phase 3: Autonomous Response (Month 3)**
*   Connect the AI module output with Wazuh's active response capability.
*   If AI Confidence > 90% -> Automatically isolate the endpoint.

## 3. AI Requirements
*   **Hardware**: VPS with at least 16GB RAM (to run quantized 7B models like Mistral). Cost increases slightly (~40-60 USD), but the selling value doubles.
*   **Technology**: Python, LangChain (for LLM orchestration), Wazuh API.
