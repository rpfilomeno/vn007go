package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/log"
	"github.com/joho/godotenv"
)

// Styles
var (
	headerStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			PaddingLeft(1).PaddingRight(1)

	freq5GStyle = lipgloss.NewStyle().
			Bold(true).
			PaddingLeft(1)

	titleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("99"))

	logStyle = lipgloss.NewStyle().
			PaddingLeft(2)
)

// Model represents the application state
type model struct {
	viewport       viewport.Model
	logs           []string
	freq5GValue    string
	uptimeValue    int
	lastRebootTime string
	ready          bool
}

// Message types for the TUI
type freqUpdateMsg string
type uptimeUpdateMsg string
type lastRebootTimeMsg string
type logMsg string

type MonitorPayload struct {
	Cmd       int    `json:"cmd"`
	Method    string `json:"method"`
	Language  string `json:"language"`
	SessionId string `json:"sessionId"`
}

type RebootPayload struct {
	Cmd           int    `json:"cmd"`
	RebootType    int    `json:"rebootType"`
	Method        string `json:"method"`
	SessionId     string `json:"sessionId"`
	Username      string `json:"username"`
	Passwd        string `json:"passwd"`
	IsAutoUpgrade string `json:"isAutoUpgrade"`
	Language      string `json:"language"`
}

type ResponseData struct {
	FREQ_5G interface{} `json:"FREQ_5G"`
	Success bool        `json:"success"`
	Uptime  interface{} `json:"uptime"`
}

const (
	maxRetries    = 5
	baseDelay     = 3 * time.Second
	maxDelay      = 32 * time.Second
	rebootTimeout = 30 * time.Second
	rebootWait    = 120 * time.Second
	maxLogs       = 1000 // Maximum number of logs to keep in memory
)

// Custom writer for capturing log output
type logWriter struct {
	program *tea.Program
}

func (l logWriter) Write(p []byte) (n int, err error) {
	l.program.Send(logMsg(strings.TrimSpace(string(p))))
	return len(p), nil
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "q" || msg.String() == "ctrl+c" {
			return m, tea.Quit
		}

	case tea.WindowSizeMsg:
		headerHeight := 8
		footerHeight := 0
		verticalMarginHeight := headerHeight + footerHeight

		if !m.ready {
			m.viewport = viewport.New(msg.Width, msg.Height-verticalMarginHeight)
			m.viewport.Style = logStyle
			m.ready = true
		} else {
			m.viewport.Width = msg.Width
			m.viewport.Height = msg.Height - verticalMarginHeight
		}

	case freqUpdateMsg:
		m.freq5GValue = string(msg)

	case lastRebootTimeMsg:
		m.lastRebootTime = string(msg)

	case uptimeUpdateMsg:
		uptimeValue, err := strconv.Atoi(string(msg))
		m.uptimeValue = uptimeValue
		if err != nil {
			m.uptimeValue = 0
		}

	case logMsg:
		m.logs = append(m.logs, string(msg))
		if len(m.logs) > maxLogs {
			m.logs = m.logs[len(m.logs)-maxLogs:]
		}
		m.viewport.SetContent(strings.Join(m.logs, "\n"))
		m.viewport.GotoBottom()
	}

	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m model) View() string {
	if !m.ready {
		return "Initializing..."
	}

	// Header with FREQ_5G value
	var freq5GDisplay string
	if m.freq5GValue == "" || m.freq5GValue == "NONE" {
		freq5GDisplay = freq5GStyle.Copy().
			Foreground(lipgloss.Color("211")). // pink
			Render("NONE")
	} else {
		freq5GDisplay = freq5GStyle.Copy().
			Foreground(lipgloss.Color("82")). // lime
			Render(m.freq5GValue)
	}

	// Header with Uptime value
	uptimeDisplay := "0"
	if m.uptimeValue < 300 {
		uptimeDisplay = freq5GStyle.Copy().
			Foreground(lipgloss.Color("211")). // pink
			Render(fmt.Sprintf("%ss", strconv.Itoa(m.uptimeValue)))
	} else {
		uptimeDisplay = freq5GStyle.Copy().
			Foreground(lipgloss.Color("82")). // lime
			Render(fmt.Sprintf("%ss", strconv.Itoa(m.uptimeValue)))
	}

	// Header with Uptime value
	rebootDisplay := "NONE"
	if m.lastRebootTime != "NONE" {
		rebootDisplay = freq5GStyle.Copy().
			Foreground(lipgloss.Color("211")). // pink
			Render(fmt.Sprintf("%ss", m.lastRebootTime))
	} else {
		rebootDisplay = freq5GStyle.Copy().
			Foreground(lipgloss.Color("82")). // lime
			Render("NONE")
	}

	header := headerStyle.Render(
		fmt.Sprintf("%s%s\n %s%s\n%s%s\npress 'q' to stop.", titleStyle.Render("FREQ_5G: "), freq5GDisplay, titleStyle.Render("Uptime: "),
			uptimeDisplay, titleStyle.Render(" Reboot: "), rebootDisplay))

	// Viewport with logs
	return fmt.Sprintf("Vn007 Auto-Restart\n%s\n%s", header, m.viewport.View())
}

func calculateBackoff(attempt int) time.Duration {
	delay := baseDelay * time.Duration(1<<uint(attempt))
	if delay > maxDelay {
		delay = maxDelay
	}
	return delay
}

func monitorService(program *tea.Program, client *http.Client, url string) {
	monitorPayload := MonitorPayload{
		Cmd:       133,
		Method:    "GET",
		Language:  "EN",
		SessionId: "",
	}

	rebootPayload := RebootPayload{
		Cmd:           6,
		RebootType:    1,
		Method:        "POST",
		SessionId:     os.Getenv("SESSION_ID"),
		Username:      os.Getenv("USERNAME"),
		Passwd:        os.Getenv("PASSWORD_HASH"),
		IsAutoUpgrade: "0",
		Language:      "EN",
	}

	for {
		responseData, err := sendRequestWithRetry(program, client, url, monitorPayload, false)
		if err != nil {
			log.Error("monitoring cycle failed", "error", err)
			time.Sleep(baseDelay)
			continue
		}

		uptimeStr := responseData.Uptime.(string)
		uptime, err := strconv.Atoi(uptimeStr)

		program.Send(uptimeUpdateMsg(uptimeStr))
		if (uptime > 300) && (err == nil) {
			if responseData.FREQ_5G == nil {
				program.Send(freqUpdateMsg("NONE"))
				log.Warn("FREQ_5G not present, initiating reboot")

				_, err := sendRequestWithRetry(program, client, url, rebootPayload, true)
				if err != nil {
					log.Error("reboot sequence failed", "error", err)
					time.Sleep(baseDelay)
				} else {
					program.Send(lastRebootTimeMsg(time.Now().String()))
					log.Info("reboot sequence completed")
				}
			} else {
				freq5GStr := fmt.Sprintf("%v", responseData.FREQ_5G)
				program.Send(freqUpdateMsg(freq5GStr))
				log.Info("monitoring check passed", "FREQ_5G", freq5GStr)
			}
		} else {
			uptimeWait := 300 - uptime
			log.Info("waiting", fmt.Sprintf("%v", uptimeWait), "secs uptime before checking")
			log.Debug("Uptime", fmt.Sprintf("%v secs", uptimeStr))
		}

		time.Sleep(baseDelay)
	}
}

func sendRequestWithRetry(program *tea.Program, client *http.Client, url string, payload interface{}, isRebootRequest bool) (*ResponseData, error) {
	var lastErr error
	reqType := "Monitoring"
	if isRebootRequest {
		reqType = "Reboot"
	}

	for attempt := 0; attempt < maxRetries; attempt++ {
		jsonData, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("error marshaling JSON: %v", err)
		}

		req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
		if err != nil {
			return nil, fmt.Errorf("error creating request: %v", err)
		}

		req.Header.Set("Content-Type", "application/json")

		log.Debug("sending request", "type", reqType, "attempt", attempt+1)

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			delay := calculateBackoff(attempt)
			log.Error("request failed", "type", reqType, "attempt", attempt+1, "error", err)
			time.Sleep(delay)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = err
			delay := calculateBackoff(attempt)
			log.Error("failed to read response", "type", reqType, "attempt", attempt+1, "error", err)
			time.Sleep(delay)
			continue
		}

		var responseData ResponseData
		err = json.Unmarshal(body, &responseData)
		if err != nil {
			lastErr = err
			delay := calculateBackoff(attempt)
			log.Error("invalid JSON response", "type", reqType, "attempt", attempt+1, "error", err)
			time.Sleep(delay)
			continue
		}

		if responseData.Success {
			log.Info("request successful", "type", reqType, "attempt", attempt+1)
			if isRebootRequest {
				log.Warn("reboot initiated", "wait_time", rebootTimeout.String())
				time.Sleep(rebootTimeout)
			}
			return &responseData, nil
		}

		lastErr = fmt.Errorf("request failed with success=false")
		delay := calculateBackoff(attempt)
		log.Error("request unsuccessful", "type", reqType, "attempt", attempt+1)
		time.Sleep(delay)
	}

	return nil, fmt.Errorf("max retries (%d) exceeded: %v", maxRetries, lastErr)
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	// Initial model
	m := model{
		logs:           make([]string, 0, maxLogs),
		freq5GValue:    "NONE",
		uptimeValue:    0,
		lastRebootTime: "NONE",
	}

	// Initialize the program
	p := tea.NewProgram(m, tea.WithAltScreen())

	// Configure custom log writer
	log.SetOutput(logWriter{program: p})
	if os.Getenv("DEBUG") == "Yes" {
		log.SetLevel(log.DebugLevel)
	} else {
		log.SetLevel(log.InfoLevel & log.WarnLevel & log.FatalLevel)
	}
	log.SetReportCaller(false)
	log.SetTimeFormat("15:04:05")

	// Start monitoring service in a goroutine
	client := &http.Client{Timeout: 10 * time.Second}
	//url := "http://192.168.0.1/cgi-bin/http.cgi"
	url := fmt.Sprintf("http://%s/cgi-bin/http.cgi", os.Getenv("IP"))

	go monitorService(p, client, url)

	// Run the program
	if _, err := p.Run(); err != nil {
		fmt.Println("Error running program:", err)
	}
}
