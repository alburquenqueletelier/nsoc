import time
import json
import requests
import os
from watchdog.observers import Observer
from watchdog.events import FileSystemEventHandler

OLLAMA_URL = "http://localhost:11434/api/generate"
LOG_FILE = "/tmp/nsoc_test.log"

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

class LLMClient:
    @staticmethod
    def analyze(context):
        """
        Query Ollama for semantic analysis. 
        Falls back to Mock if Ollama is unreachable.
        """
        prompt = f"Analyze this log for security threats. Be concise. Log: {context}"
        
        payload = {
            "model": "llama3", # or mistral
            "prompt": prompt,
            "stream": False
        }

        try:
            response = requests.post(OLLAMA_URL, json=payload, timeout=2)
            if response.status_code == 200:
                return response.json().get("response", "No response from model")
        except requests.exceptions.RequestException:
            pass # Fallback
            
        return "[MOCK AI] This log indicates potential privilege escalation attempt based on keyword patterns."

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
                print(f"[BRAIN] 🚨 ANOMALY DETECTED: {reason}")
                print(f"        Log: {data['raw']}")
                
                # 3. LLM Analysis (only for anomalies)
                print(f"[BRAIN] 🧠 Asking AI Model...")
                analysis = LLMClient.analyze(data['raw'])
                print(f"[BRAIN] 🤖 Analysis: {analysis}")
                print("-" * 50)
            else:
                print(f"[BRAIN] ✅ Normal: {data['raw']}")

def main():
    if not os.path.exists(LOG_FILE):
        with open(LOG_FILE, 'w') as f:
            f.write("")
    
    print(f"[BRAIN] Starting AI Engine (Watchdog) on {LOG_FILE}...")
    event_handler = LogHandler(LOG_FILE)
    observer = Observer()
    observer.schedule(event_handler, path=os.path.dirname(LOG_FILE), recursive=False)
    observer.start()

    try:
        while True:
            time.sleep(1)
    except KeyboardInterrupt:
        observer.stop()
    observer.join()

if __name__ == "__main__":
    main()
