package hugo

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"

	"github.com/fernandezvara/hugo-manager/internal/config"
)

// Status represents the Hugo server status
type Status string

const (
	StatusStopped  Status = "stopped"
	StatusStarting Status = "starting"
	StatusRunning  Status = "running"
	StatusError    Status = "error"
)

// Manager handles the Hugo server process
type Manager struct {
	projectDir  string
	config      config.HugoConfig
	cmd         *exec.Cmd
	status      Status
	statusMsg   string
	logs        []LogEntry
	logMu       sync.RWMutex
	statusMu    sync.RWMutex
	subscribers []chan LogEntry
	subMu       sync.RWMutex
	maxLogs     int
}

// LogEntry represents a single log entry
type LogEntry struct {
	Time    time.Time `json:"time"`
	Message string    `json:"message"`
	Type    string    `json:"type"` // "stdout", "stderr", "system"
}

// NewManager creates a new Hugo manager
func NewManager(projectDir string, cfg config.HugoConfig) *Manager {
	return &Manager{
		projectDir: projectDir,
		config:     cfg,
		status:     StatusStopped,
		logs:       make([]LogEntry, 0, 1000),
		maxLogs:    1000,
	}
}

// Start starts the Hugo server
func (m *Manager) Start() error {
	m.statusMu.Lock()
	if m.status == StatusRunning || m.status == StatusStarting {
		m.statusMu.Unlock()
		return fmt.Errorf("Hugo is already running")
	}
	m.status = StatusStarting
	m.statusMsg = "Starting Hugo server..."
	m.statusMu.Unlock()

	m.addLog("Starting Hugo server...", "system")

	// Build Hugo command
	args := []string{"server"}
	args = append(args, "--port", fmt.Sprintf("%d", m.config.Port))

	if m.config.DisableFastRender {
		args = append(args, "--disableFastRender")
	}

	args = append(args, m.config.AdditionalArgs...)

	m.cmd = exec.Command("hugo", args...)
	m.cmd.Dir = m.projectDir

	// Get stdout and stderr pipes
	stdout, err := m.cmd.StdoutPipe()
	if err != nil {
		m.setStatus(StatusError, fmt.Sprintf("Failed to get stdout: %v", err))
		return err
	}

	stderr, err := m.cmd.StderrPipe()
	if err != nil {
		m.setStatus(StatusError, fmt.Sprintf("Failed to get stderr: %v", err))
		return err
	}

	// Start the process
	if err := m.cmd.Start(); err != nil {
		m.setStatus(StatusError, fmt.Sprintf("Failed to start Hugo: %v", err))
		return err
	}

	// Monitor stdout
	go m.streamLogs(stdout, "stdout")
	go m.streamLogs(stderr, "stderr")

	// Monitor process
	go func() {
		err := m.cmd.Wait()
		if err != nil {
			m.addLog(fmt.Sprintf("Hugo exited with error: %v", err), "system")
			m.setStatus(StatusError, err.Error())
		} else {
			m.addLog("Hugo server stopped", "system")
			m.setStatus(StatusStopped, "")
		}
	}()

	// Give Hugo a moment to start, then update status
	go func() {
		time.Sleep(2 * time.Second)
		m.statusMu.Lock()
		if m.status == StatusStarting {
			m.status = StatusRunning
			m.statusMsg = fmt.Sprintf("Running on port %d", m.config.Port)
		}
		m.statusMu.Unlock()
	}()

	return nil
}

// Stop stops the Hugo server
func (m *Manager) Stop() error {
	m.statusMu.Lock()
	if m.status == StatusStopped {
		m.statusMu.Unlock()
		return fmt.Errorf("Hugo is not running")
	}
	m.statusMu.Unlock()

	m.addLog("Stopping Hugo server...", "system")

	if m.cmd != nil && m.cmd.Process != nil {
		if err := m.cmd.Process.Kill(); err != nil {
			m.addLog(fmt.Sprintf("Error stopping Hugo: %v", err), "system")
			return err
		}
	}

	m.setStatus(StatusStopped, "")
	return nil
}

// Restart restarts the Hugo server
func (m *Manager) Restart() error {
	m.addLog("Restarting Hugo server...", "system")

	if m.status != StatusStopped {
		if err := m.Stop(); err != nil {
			return err
		}
		// Wait for process to fully stop
		time.Sleep(500 * time.Millisecond)
	}

	return m.Start()
}

// GetStatus returns the current status
func (m *Manager) GetStatus() (Status, string) {
	m.statusMu.RLock()
	defer m.statusMu.RUnlock()
	return m.status, m.statusMsg
}

// GetLogs returns the recent logs
func (m *Manager) GetLogs(limit int) []LogEntry {
	m.logMu.RLock()
	defer m.logMu.RUnlock()

	if limit <= 0 || limit > len(m.logs) {
		limit = len(m.logs)
	}

	start := len(m.logs) - limit
	if start < 0 {
		start = 0
	}

	result := make([]LogEntry, limit)
	copy(result, m.logs[start:])
	return result
}

// Subscribe creates a new log subscription channel
func (m *Manager) Subscribe() chan LogEntry {
	ch := make(chan LogEntry, 100)
	m.subMu.Lock()
	m.subscribers = append(m.subscribers, ch)
	m.subMu.Unlock()
	return ch
}

// Unsubscribe removes a log subscription channel
func (m *Manager) Unsubscribe(ch chan LogEntry) {
	m.subMu.Lock()
	defer m.subMu.Unlock()

	for i, sub := range m.subscribers {
		if sub == ch {
			m.subscribers = append(m.subscribers[:i], m.subscribers[i+1:]...)
			close(ch)
			return
		}
	}
}

// GetPort returns the Hugo server port
func (m *Manager) GetPort() int {
	return m.config.Port
}

func (m *Manager) setStatus(status Status, msg string) {
	m.statusMu.Lock()
	m.status = status
	m.statusMsg = msg
	m.statusMu.Unlock()
}

func (m *Manager) addLog(message, logType string) {
	entry := LogEntry{
		Time:    time.Now(),
		Message: message,
		Type:    logType,
	}

	m.logMu.Lock()
	m.logs = append(m.logs, entry)
	if len(m.logs) > m.maxLogs {
		m.logs = m.logs[len(m.logs)-m.maxLogs:]
	}
	m.logMu.Unlock()

	// Broadcast to subscribers
	m.subMu.RLock()
	for _, ch := range m.subscribers {
		select {
		case ch <- entry:
		default:
			// Skip if channel is full
		}
	}
	m.subMu.RUnlock()
}

func (m *Manager) streamLogs(reader io.Reader, logType string) {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()
		m.addLog(line, logType)

		// Detect successful startup
		if logType == "stdout" && (contains(line, "Web Server is available") || contains(line, "Serving pages from")) {
			m.setStatus(StatusRunning, fmt.Sprintf("Running on port %d", m.config.Port))
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr))
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
