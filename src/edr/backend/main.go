package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	_ "github.com/lib/pq"
)

// ---------------------------------------------------------------------------
// Structs
// ---------------------------------------------------------------------------

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

type CommandEntry struct {
	Type string `json:"type"`
	PID  int    `json:"pid,omitempty"`
	IP   string `json:"ip,omitempty"`
}

type Alert struct {
	ID        string       `json:"id"`
	Hostname  string       `json:"hostname"`
	Source    string       `json:"source"`
	Detail    string       `json:"detail"`
	Severity  string       `json:"severity"`
	Reason    string       `json:"reason"`
	Action    *CommandEntry `json:"action,omitempty"`
	Timestamp time.Time    `json:"timestamp"`
}

type BrainRequest struct {
	Source   string      `json:"source"`
	Hostname string     `json:"hostname"`
	Data     interface{} `json:"data"`
}

type BrainResponse struct {
	IsThreat          bool         `json:"is_threat"`
	Severity          string       `json:"severity"`
	Reason            string       `json:"reason"`
	RecommendedAction *CommandEntry `json:"recommended_action"`
}

// ---------------------------------------------------------------------------
// In-memory storage (fallback when DB is unavailable)
// ---------------------------------------------------------------------------

var (
	alertsMu sync.Mutex
	alerts   []Alert

	commandsMu sync.Mutex
	// commands maps hostname -> list of pending CommandEntry
	commands map[string][]CommandEntry

	db *sql.DB
)

func init() {
	alerts = make([]Alert, 0)
	commands = make(map[string][]CommandEntry)
}

// ---------------------------------------------------------------------------
// Database helpers
// ---------------------------------------------------------------------------

func connectDB() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://nsoc:nsoc@localhost:5432/nsoc?sslmode=disable"
	}

	var err error
	db, err = sql.Open("postgres", dsn)
	if err != nil {
		log.Printf("[WARN] Could not open database connection: %v — running with in-memory storage only", err)
		db = nil
		return
	}

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)

	if err = db.Ping(); err != nil {
		log.Printf("[WARN] Could not reach database: %v — running with in-memory storage only", err)
		db = nil
		return
	}

	log.Printf("[INFO] Connected to PostgreSQL database")
}

func initDB() {
	if db == nil {
		return
	}

	statements := []string{
		`CREATE TABLE IF NOT EXISTS agents (
			id             SERIAL PRIMARY KEY,
			hostname       VARCHAR(255) UNIQUE NOT NULL,
			os             VARCHAR(100),
			last_heartbeat TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			first_seen     TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS log_events (
			id          SERIAL PRIMARY KEY,
			hostname    VARCHAR(255) NOT NULL,
			log         TEXT NOT NULL,
			timestamp   TIMESTAMPTZ NOT NULL,
			received_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS process_events (
			id          SERIAL PRIMARY KEY,
			hostname    VARCHAR(255) NOT NULL,
			pid         INTEGER NOT NULL,
			name        VARCHAR(255) NOT NULL,
			cmd         TEXT,
			received_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS alerts (
			id          VARCHAR(64) PRIMARY KEY,
			hostname    VARCHAR(255) NOT NULL,
			source      VARCHAR(20) NOT NULL,
			detail      TEXT NOT NULL,
			severity    VARCHAR(20) NOT NULL,
			reason      TEXT NOT NULL,
			action_type VARCHAR(20),
			action_data JSONB,
			created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS commands (
			id            SERIAL PRIMARY KEY,
			hostname      VARCHAR(255) NOT NULL,
			type          VARCHAR(20) NOT NULL,
			payload       JSONB NOT NULL,
			dispatched    BOOLEAN NOT NULL DEFAULT FALSE,
			created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			dispatched_at TIMESTAMPTZ
		)`,
	}

	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			log.Printf("[WARN] Migration statement failed: %v", err)
		}
	}

	// Indexes
	indexes := []string{
		`CREATE INDEX IF NOT EXISTS idx_log_events_hostname ON log_events(hostname)`,
		`CREATE INDEX IF NOT EXISTS idx_process_events_hostname ON process_events(hostname)`,
		`CREATE INDEX IF NOT EXISTS idx_alerts_hostname ON alerts(hostname)`,
		`CREATE INDEX IF NOT EXISTS idx_alerts_severity ON alerts(severity)`,
		`CREATE INDEX IF NOT EXISTS idx_alerts_created_at ON alerts(created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_commands_hostname_dispatched ON commands(hostname, dispatched)`,
		`CREATE INDEX IF NOT EXISTS idx_agents_hostname ON agents(hostname)`,
	}

	for _, idx := range indexes {
		if _, err := db.Exec(idx); err != nil {
			log.Printf("[WARN] Index creation failed: %v", err)
		}
	}

	log.Printf("[INFO] Database migration completed")
}

// ---------------------------------------------------------------------------
// Brain forwarding
// ---------------------------------------------------------------------------

func forwardToBrain(source, hostname string, data interface{}) {
	brainReq := BrainRequest{
		Source:   source,
		Hostname: hostname,
		Data:     data,
	}

	body, err := json.Marshal(brainReq)
	if err != nil {
		log.Printf("[WARN] Failed to marshal BrainRequest: %v", err)
		return
	}

	brainURL := os.Getenv("BRAIN_URL")
	if brainURL == "" {
		brainURL = "http://localhost:5000/analyze"
	}
	brainTimeout := 30 * time.Second
	if v := os.Getenv("BRAIN_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			brainTimeout = d
		}
	}
	client := &http.Client{Timeout: brainTimeout}
	resp, err := client.Post(brainURL, "application/json", bytes.NewReader(body))
	if err != nil {
		log.Printf("[WARN] Brain unreachable: %v", err)
		return
	}
	defer resp.Body.Close()

	var brainResp BrainResponse
	if err := json.NewDecoder(resp.Body).Decode(&brainResp); err != nil {
		log.Printf("[WARN] Failed to decode BrainResponse: %v", err)
		return
	}

	if !brainResp.IsThreat {
		return
	}

	alertID := fmt.Sprintf("alert-%d", time.Now().UnixNano())
	alert := Alert{
		ID:        alertID,
		Hostname:  hostname,
		Source:    source,
		Detail:    fmt.Sprintf("Brain detected threat from %s source", source),
		Severity:  brainResp.Severity,
		Reason:    brainResp.Reason,
		Action:    brainResp.RecommendedAction,
		Timestamp: time.Now(),
	}

	// Store in memory
	alertsMu.Lock()
	alerts = append(alerts, alert)
	alertsMu.Unlock()

	log.Printf("[ALERT] Brain threat: id=%s host=%s severity=%s reason=%s", alertID, hostname, brainResp.Severity, brainResp.Reason)

	// Enqueue command in memory if action recommended
	if brainResp.RecommendedAction != nil {
		commandsMu.Lock()
		commands[hostname] = append(commands[hostname], *brainResp.RecommendedAction)
		commandsMu.Unlock()
	}

	// Persist alert to DB
	if db != nil {
		go func() {
			var actionType *string
			var actionData []byte
			if alert.Action != nil {
				t := alert.Action.Type
				actionType = &t
				actionData, _ = json.Marshal(alert.Action)
			}
			_, err := db.Exec(
				`INSERT INTO alerts (id, hostname, source, detail, severity, reason, action_type, action_data, created_at)
				 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
				alert.ID, alert.Hostname, alert.Source, alert.Detail, alert.Severity,
				alert.Reason, actionType, actionData, alert.Timestamp,
			)
			if err != nil {
				log.Printf("[WARN] Failed to persist alert to DB: %v", err)
			}
		}()

		// Persist command to DB
		if brainResp.RecommendedAction != nil {
			go func() {
				payload, _ := json.Marshal(brainResp.RecommendedAction)
				_, err := db.Exec(
					`INSERT INTO commands (hostname, type, payload, dispatched)
					 VALUES ($1, $2, $3, FALSE)`,
					hostname, brainResp.RecommendedAction.Type, payload,
				)
				if err != nil {
					log.Printf("[WARN] Failed to persist command to DB: %v", err)
				}
			}()
		}
	}
}

// ---------------------------------------------------------------------------
// Alert & Process analysis (existing logic)
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

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

	// Persist to DB (upsert)
	if db != nil {
		go func() {
			_, err := db.Exec(
				`INSERT INTO agents (hostname, os, last_heartbeat)
				 VALUES ($1, $2, NOW())
				 ON CONFLICT (hostname) DO UPDATE SET last_heartbeat = NOW(), os = $2`,
				hb.Hostname, hb.OS,
			)
			if err != nil {
				log.Printf("[WARN] Failed to upsert agent in DB: %v", err)
			}
		}()
	}

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
	go analyzeLog(entry)
	go forwardToBrain("log", entry.Hostname, map[string]interface{}{
		"log":       entry.Log,
		"timestamp": entry.Timestamp,
	})

	// Persist to DB
	if db != nil {
		go func() {
			ts, err := time.Parse(time.RFC3339, entry.Timestamp)
			if err != nil {
				ts = time.Now()
			}
			_, err = db.Exec(
				`INSERT INTO log_events (hostname, log, timestamp) VALUES ($1, $2, $3)`,
				entry.Hostname, entry.Log, ts,
			)
			if err != nil {
				log.Printf("[WARN] Failed to persist log event to DB: %v", err)
			}
		}()
	}

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
	go analyzeProcess(entry)
	go forwardToBrain("process", entry.Hostname, map[string]interface{}{
		"pid":  entry.PID,
		"name": entry.Name,
		"cmd":  entry.Cmd,
	})

	// Persist to DB
	if db != nil {
		go func() {
			_, err := db.Exec(
				`INSERT INTO process_events (hostname, pid, name, cmd) VALUES ($1, $2, $3, $4)`,
				entry.Hostname, entry.PID, entry.Name, entry.Cmd,
			)
			if err != nil {
				log.Printf("[WARN] Failed to persist process event to DB: %v", err)
			}
		}()
	}

	w.WriteHeader(http.StatusOK)
}

func handleCommands(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	hostname := r.URL.Query().Get("hostname")
	if hostname == "" {
		http.Error(w, "hostname query parameter required", http.StatusBadRequest)
		return
	}

	// DB-backed path
	if db != nil {
		tx, err := db.Begin()
		if err != nil {
			log.Printf("[WARN] Failed to begin transaction for commands: %v", err)
			// Fall through to in-memory
		} else {
			rows, err := tx.Query(
				`SELECT id, type, payload FROM commands WHERE hostname = $1 AND dispatched = FALSE`,
				hostname,
			)
			if err != nil {
				tx.Rollback()
				log.Printf("[WARN] Failed to query commands from DB: %v", err)
			} else {
				var cmds []CommandEntry
				var ids []int
				for rows.Next() {
					var id int
					var cmdType string
					var payload []byte
					if err := rows.Scan(&id, &cmdType, &payload); err != nil {
						log.Printf("[WARN] Failed to scan command row: %v", err)
						continue
					}
					var cmd CommandEntry
					if err := json.Unmarshal(payload, &cmd); err != nil {
						// Fallback: just use type
						cmd = CommandEntry{Type: cmdType}
					}
					cmds = append(cmds, cmd)
					ids = append(ids, id)
				}
				rows.Close()

				// Mark as dispatched
				for _, id := range ids {
					_, err := tx.Exec(
						`UPDATE commands SET dispatched = TRUE, dispatched_at = NOW() WHERE id = $1`,
						id,
					)
					if err != nil {
						log.Printf("[WARN] Failed to mark command %d as dispatched: %v", id, err)
					}
				}

				if err := tx.Commit(); err != nil {
					log.Printf("[WARN] Failed to commit commands transaction: %v", err)
				} else {
					if cmds == nil {
						cmds = []CommandEntry{}
					}
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(map[string]interface{}{"commands": cmds})
					return
				}
			}
		}
	}

	// In-memory fallback
	commandsMu.Lock()
	pending, exists := commands[hostname]
	if exists {
		delete(commands, hostname)
	}
	commandsMu.Unlock()

	if pending == nil {
		pending = []CommandEntry{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"commands": pending})
}

func handleAlerts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	hostnameFilter := r.URL.Query().Get("hostname")
	severityFilter := r.URL.Query().Get("severity")

	// DB-backed path
	if db != nil {
		query := `SELECT id, hostname, source, detail, severity, reason, action_type, action_data, created_at FROM alerts`
		var conditions []string
		var args []interface{}
		argIdx := 1

		if hostnameFilter != "" {
			conditions = append(conditions, fmt.Sprintf("hostname = $%d", argIdx))
			args = append(args, hostnameFilter)
			argIdx++
		}
		if severityFilter != "" {
			conditions = append(conditions, fmt.Sprintf("severity = $%d", argIdx))
			args = append(args, severityFilter)
			argIdx++
		}

		if len(conditions) > 0 {
			query += " WHERE " + strings.Join(conditions, " AND ")
		}
		query += " ORDER BY created_at DESC LIMIT 100"

		rows, err := db.Query(query, args...)
		if err != nil {
			log.Printf("[WARN] Failed to query alerts from DB: %v — falling back to in-memory", err)
		} else {
			defer rows.Close()
			var result []Alert
			for rows.Next() {
				var a Alert
				var actionType sql.NullString
				var actionData []byte
				var createdAt time.Time
				if err := rows.Scan(&a.ID, &a.Hostname, &a.Source, &a.Detail, &a.Severity, &a.Reason, &actionType, &actionData, &createdAt); err != nil {
					log.Printf("[WARN] Failed to scan alert row: %v", err)
					continue
				}
				a.Timestamp = createdAt
				if actionType.Valid && len(actionData) > 0 {
					var cmd CommandEntry
					if err := json.Unmarshal(actionData, &cmd); err == nil {
						a.Action = &cmd
					}
				}
				result = append(result, a)
			}
			if result == nil {
				result = []Alert{}
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(result)
			return
		}
	}

	// In-memory fallback
	alertsMu.Lock()
	snapshot := make([]Alert, len(alerts))
	copy(snapshot, alerts)
	alertsMu.Unlock()

	// Apply filters on in-memory data
	var filtered []Alert
	for _, a := range snapshot {
		if hostnameFilter != "" && a.Hostname != hostnameFilter {
			continue
		}
		if severityFilter != "" && a.Severity != severityFilter {
			continue
		}
		filtered = append(filtered, a)
	}
	if filtered == nil {
		filtered = []Alert{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(filtered)
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

func main() {
	port := os.Getenv("BACKEND_PORT")
	if port == "" {
		port = "8080"
	}
	port = ":" + port

	// Database connection (optional)
	connectDB()
	if db != nil {
		initDB()
	}

	http.HandleFunc("/heartbeat", handleHeartbeat)
	http.HandleFunc("/logs", handleLogs)
	http.HandleFunc("/processes", handleProcesses)
	http.HandleFunc("/commands", handleCommands)
	http.HandleFunc("/alerts", handleAlerts)

	log.Printf("Backend Service starting on port %s...", port)
	if err := http.ListenAndServe(port, nil); err != nil {
		log.Fatal(err)
	}
}
