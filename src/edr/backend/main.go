package main

import (
	"encoding/json"
	"log"
	"net/http"
	"time"
)

type Heartbeat struct {
	Hostname  string    `json:"hostname"`
	Timestamp time.Time `json:"timestamp"`
	OS        string    `json:"os"`
}

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

	log.Printf("[HEARTBEAT] Host: %s, OS: %s, Time: %v", hb.Hostname, hb.OS, hb.Timestamp)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ACK"))
}

func main() {
	port := ":8080"
	http.HandleFunc("/heartbeat", handleHeartbeat)

	log.Printf("Backend Service starting on port %s...", port)
	if err := http.ListenAndServe(port, nil); err != nil {
		log.Fatal(err)
	}
}
