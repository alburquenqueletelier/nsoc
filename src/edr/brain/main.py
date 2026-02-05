from fastapi import FastAPI
from pydantic import BaseModel

app = FastAPI()

class LogEntry(BaseModel):
    log_content: str
    source: str

@app.post("/analyze")
async def analyze_log(entry: LogEntry):
    # Mock AI analysis
    is_suspicious = "sudo" in entry.log_content.lower()
    return {
        "analysis": "Suspicious activity detected" if is_suspicious else "Clean",
        "confidence": 0.85 if is_suspicious else 0.99,
        "model": "Sentinel-AI-v0.1"
    }

if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host="0.0.0.0", port=8000)
