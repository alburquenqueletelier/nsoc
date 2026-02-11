package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"
)

type Heartbeat struct {
	Hostname  string    `json:"hostname"`
	Timestamp time.Time `json:"timestamp"`
	OS        string    `json:"os"`
}

type LogEntry struct {
	Hostname  string `json:"hostname"`
	Log       string `json:"log"`
	Timestamp string `json:"timestamp"`
}

type ProcessEntry struct {
	Hostname string `json:"hostname"`
	PID      int    `json:"pid"`
	Name     string `json:"name"`
	Cmd      string `json:"cmd"`
}

// AlertManager Logic
func analyzeLog(entry LogEntry) {
	keywords := []string{"failed", "error", "sudo", "root", "unauthorized"}
	for _, kw := range keywords {
		if strings.Contains(strings.ToLower(entry.Log), kw) {
			log.Printf("[ALERT] Suspicious LOG from %s: %s (Match: %s)", entry.Hostname, entry.Log, kw)
			return
		}
	}
	log.Printf("[INFO] Log received from %s: %s", entry.Hostname, entry.Log)
}

func analyzeProcess(entry ProcessEntry) {
	badProcs := []string{"nc", "ncat", "cryptominer", "mimikatz"}
	for _, bp := range badProcs {
		if strings.Contains(strings.ToLower(entry.Name), bp) {
			log.Printf("[ALERT] Malicious PROCESS from %s: %s (PID: %d)", entry.Hostname, entry.Name, entry.PID)
			return
		}
	}
	log.Printf("[INFO] Process event from %s: %s (PID: %d)", entry.Hostname, entry.Name, entry.PID)
}

// Handlers
func handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var hb Heartbeat
	if err := json.NewDecoder(r.Body).Decode(&hb); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	log.Printf("[HEARTBEAT] Host: %s, OS: %s", hb.Hostname, hb.OS)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ACK"))
}

func handleLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var entry LogEntry
	if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	go analyzeLog(entry) // Async analysis
	w.WriteHeader(http.StatusOK)
}

func handleProcesses(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var entry ProcessEntry
	if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	go analyzeProcess(entry) // Async analysis
	w.WriteHeader(http.StatusOK)
}

func main() {
	port := ":8080"
	http.HandleFunc("/heartbeat", handleHeartbeat)
	http.HandleFunc("/logs", handleLogs)
	http.HandleFunc("/processes", handleProcesses)

	log.Printf("Backend Service starting on port %s...", port)
	if err := http.ListenAndServe(port, nil); err != nil {
		log.Fatal(err)
	}
}
