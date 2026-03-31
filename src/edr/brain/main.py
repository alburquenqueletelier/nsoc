import time
import json
import requests
import os
import threading
from http.server import HTTPServer, BaseHTTPRequestHandler
from watchdog.observers import Observer
from watchdog.events import FileSystemEventHandler

OLLAMA_URL = os.getenv("OLLAMA_URL", "http://localhost:11434/api/generate")
OLLAMA_TIMEOUT = int(os.getenv("OLLAMA_TIMEOUT", "30"))
LOG_FILE = os.getenv("BRAIN_LOG_FILE", "/tmp/nsoc_test.log")
BRAIN_PORT = int(os.getenv("BRAIN_PORT", "5000"))
USE_LOCAL_LLM = os.getenv("USE_LOCAL_LLM", "true").lower() == "true"
CLOUDFLARE_ACCOUNT_ID = os.getenv("CLOUDFLARE_ACCOUNT_ID", "")
CLOUDFLARE_API_TOKEN = os.getenv("CLOUDFLARE_API_TOKEN", "")
CLOUDFLARE_MODEL = os.getenv("CLOUDFLARE_MODEL", "@cf/meta/llama-3-8b-instruct")
OLLAMA_MODEL = os.getenv("OLLAMA_MODEL", "llama3")

class LogParser:
    @staticmethod
    def parse(line):
        """Simple normalizer. In real world, use regex/grok."""
        return {
            "raw": line.strip(),
            "timestamp": time.time()
        }

class AnomalyModel:
    @staticmethod
    def check(data):
        """
        Simple heuristic:
        - Long lines (> 100 chars) are suspicious.
        - Keywords like 'sudo', 'failed'.
        """
        raw = data.get("raw", "").lower()
        if len(raw) > 100:
            return True, "Length anomaly (>100 chars)"

        suspicious = ["sudo", "failed", "error", "unauthorized", "root"]
        for kw in suspicious:
            if kw in raw:
                return True, f"Keyword match: {kw}"

        return False, None

    @staticmethod
    def check_process(data):
        """Check if a process is suspicious."""
        name = data.get("name", "").lower()
        cmd = data.get("cmd", "").lower()
        malicious = ["nc", "ncat", "cryptominer", "mimikatz", "reverse", "shell", "meterpreter"]
        for kw in malicious:
            if kw in name or kw in cmd:
                return True, f"Malicious process keyword: {kw}"
        return False, None

    @staticmethod
    def classify_severity(reason, source):
        """Classify threat severity based on reason and source."""
        reason_lower = reason.lower() if reason else ""
        critical_kw = ["mimikatz", "meterpreter", "cryptominer", "reverse"]
        high_kw = ["sudo", "root", "unauthorized"]
        medium_kw = ["failed", "error"]

        for kw in critical_kw:
            if kw in reason_lower:
                return "critical"
        for kw in high_kw:
            if kw in reason_lower:
                return "high"
        for kw in medium_kw:
            if kw in reason_lower:
                return "medium"
        return "low"

class LLMClient:
    @staticmethod
    def _analyze_local(context):
        """
        Query Ollama for semantic analysis.
        Falls back to mock if Ollama is unreachable.
        """
        prompt = f"Analyze this log for security threats. Be concise. Log: {context}"

        payload = {
            "model": OLLAMA_MODEL, # or mistral
            "prompt": prompt,
            "stream": False
        }

        try:
            response = requests.post(OLLAMA_URL, json=payload, timeout=OLLAMA_TIMEOUT)
            if response.status_code == 200:
                return response.json().get("response", "No response from model")
        except requests.exceptions.RequestException:
            pass # Fallback

        return "[MOCK AI] This log indicates potential privilege escalation attempt based on keyword patterns."

    @staticmethod
    def _analyze_cloudflare(context):
        """
        Query Cloudflare Workers AI for semantic analysis.
        Falls back to mock if the API is unreachable or credentials are missing.
        """
        prompt = f"Analyze this log for security threats. Be concise. Log: {context}"

        url = f"https://api.cloudflare.com/client/v4/accounts/{CLOUDFLARE_ACCOUNT_ID}/ai/run/{CLOUDFLARE_MODEL}"
        headers = {
            "Authorization": f"Bearer {CLOUDFLARE_API_TOKEN}",
            "Content-Type": "application/json"
        }
        payload = {
            "messages": [{"role": "user", "content": prompt}]
        }

        try:
            response = requests.post(url, json=payload, headers=headers, timeout=OLLAMA_TIMEOUT)
            if response.status_code == 200:
                return response.json().get("result", {}).get("response", "No response from Cloudflare")
        except Exception:
            pass # Fallback

        return "[MOCK AI] Cloudflare unreachable. Fallback mock analysis."

    @staticmethod
    def analyze(context):
        """
        Dispatcher: routes analysis to local Ollama or Cloudflare Workers AI
        based on the USE_LOCAL_LLM environment variable.
        """
        if USE_LOCAL_LLM:
            print("[BRAIN] Using local LLM (Ollama)")
            return LLMClient._analyze_local(context)
        else:
            print("[BRAIN] Using Cloudflare Workers AI")
            return LLMClient._analyze_cloudflare(context)


class AnalyzeHandler(BaseHTTPRequestHandler):
    """HTTP request handler for the /analyze endpoint."""

    def log_message(self, format, *args):
        """Override to use [BRAIN] prefix for access logs."""
        print(f"[BRAIN] HTTP {args[0]}")

    def do_POST(self):
        if self.path != "/analyze":
            self._send_not_found()
            return

        try:
            content_length = int(self.headers.get("Content-Length", 0))
            body = self.rfile.read(content_length)
            payload = json.loads(body)
        except (json.JSONDecodeError, ValueError) as e:
            self._send_json(400, {"error": f"Invalid JSON: {e}"})
            return

        source = payload.get("source", "")
        hostname = payload.get("hostname", "unknown")
        data = payload.get("data", {})

        is_threat = False
        reason = None
        severity = "low"
        recommended_action = None

        if source == "log":
            parsed = LogParser.parse(data.get("log", ""))
            is_anomaly, anomaly_reason = AnomalyModel.check(parsed)
            if is_anomaly:
                is_threat = True
                reason = anomaly_reason
                llm_result = LLMClient.analyze(parsed["raw"])
                reason = f"{anomaly_reason} | AI: {llm_result}"
                severity = AnomalyModel.classify_severity(anomaly_reason, source)
                print(f"[BRAIN] ANOMALY from {hostname}: {anomaly_reason}")

        elif source == "process":
            is_anomaly, anomaly_reason = AnomalyModel.check_process(data)
            if is_anomaly:
                is_threat = True
                reason = anomaly_reason
                llm_context = f"Process: {data.get('name', '')} cmd: {data.get('cmd', '')}"
                llm_result = LLMClient.analyze(llm_context)
                reason = f"{anomaly_reason} | AI: {llm_result}"
                severity = AnomalyModel.classify_severity(anomaly_reason, source)
                print(f"[BRAIN] SUSPICIOUS PROCESS from {hostname}: {anomaly_reason}")

                # Recommend kill for critical processes
                if severity == "critical" and "pid" in data:
                    recommended_action = {"type": "kill_process", "pid": data["pid"]}

        response_body = {
            "is_threat": is_threat,
            "severity": severity,
            "reason": reason,
            "recommended_action": recommended_action
        }

        self._send_json(200, response_body)

    def do_GET(self):
        self._send_not_found()

    def do_PUT(self):
        self._send_not_found()

    def do_DELETE(self):
        self._send_not_found()

    def _send_not_found(self):
        self._send_json(404, {"error": "Not found"})

    def _send_json(self, status_code, data):
        body = json.dumps(data).encode("utf-8")
        self.send_response(status_code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)


class LogHandler(FileSystemEventHandler):
    def __init__(self, filename):
        self.filename = filename
        self.file = open(filename, 'r')
        self.file.seek(0, 2) # Seek to end

    def on_modified(self, event):
        if event.src_path == self.filename:
            self.process_new_lines()

    def process_new_lines(self):
        lines = self.file.readlines()
        for line in lines:
            if not line.strip():
                continue

            # 1. Parse
            data = LogParser.parse(line)

            # 2. Anomaly Detection
            is_anomaly, reason = AnomalyModel.check(data)

            if is_anomaly:
                print(f"[BRAIN] ANOMALY DETECTED: {reason}")
                print(f"        Log: {data['raw']}")

                # 3. LLM Analysis (only for anomalies)
                print(f"[BRAIN] Asking AI Model...")
                analysis = LLMClient.analyze(data['raw'])
                print(f"[BRAIN] Analysis: {analysis}")
                print("-" * 50)
            else:
                print(f"[BRAIN] Normal: {data['raw']}")

def main():
    if not os.path.exists(LOG_FILE):
        with open(LOG_FILE, 'w') as f:
            f.write("")

    # Start watchdog observer for file-based log monitoring
    print(f"[BRAIN] Starting AI Engine (Watchdog) on {LOG_FILE}...")
    event_handler = LogHandler(LOG_FILE)
    observer = Observer()
    observer.schedule(event_handler, path=os.path.dirname(LOG_FILE), recursive=False)
    observer.start()

    # Start HTTP API server in a daemon thread
    server = HTTPServer(("0.0.0.0", BRAIN_PORT), AnalyzeHandler)
    server_thread = threading.Thread(target=server.serve_forever, daemon=True)
    server_thread.start()
    print(f"[BRAIN] HTTP API listening on :{BRAIN_PORT}")

    try:
        while True:
            time.sleep(1)
    except KeyboardInterrupt:
        observer.stop()
        server.shutdown()
    observer.join()

if __name__ == "__main__":
    main()
