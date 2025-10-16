package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/log"
	"github.com/gen2brain/beeep"
	"github.com/joho/godotenv"
)

// Define regular expressions for log levels
var (
	infoRegex  = regexp.MustCompile(`(?i)\bINFO\b`)
	warnRegex  = regexp.MustCompile(`(?i)\bWARN\b`)
	errorRegex = regexp.MustCompile(`(?i)\bERRO\b`)
	fatalRegex = regexp.MustCompile(`(?i)\bFATA\b`)
	debugRegex = regexp.MustCompile(`(?i)\bDEBU\b`)
)

// Define styles using lipgloss or any other styling package
var (
	infoStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true) // Green
	warnStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true) // Yellow
	errorStyle = lipgloss.NewStyle().Background(lipgloss.Color("9")).Bold(true)  // Red
	fatalStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)  // Red FG
	debugStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Bold(true)  // Cyan

)

// Styles
var (
	headerStyle = lipgloss.NewStyle().Width(40).Margin(1).PaddingLeft(3).
			Border(lipgloss.DoubleBorder(), true, true, true, true)

	textStyle  = lipgloss.NewStyle()
	titleStyle = lipgloss.NewStyle().Bold(true)
	logStyle   = lipgloss.NewStyle().PaddingLeft(2)
	lastlog    string
)

// Model represents the application state
type model struct {
	exePath        string
	viewport       viewport.Model
	logs           []string
	freqValue      string
	freq5GValue    string
	txBytes        int
	rxBytes        int
	rsrqValue      int
	rsrq5GValue    int
	uptimeValue    int
	lastRebootTime string
	ready          bool
	inputBuffer    string
	client         *http.Client
	url            string
}

// Message types for the TUI
type freqUpdateMsg string
type freq5gUpdateMsg string
type uptimeUpdateMsg string
type lastRebootTimeMsg string
type logMsg string
type rxMsg string
type txMsg string
type rsrqMsg string
type rsrq5gMsg string

// LoginPayload represents the structure for authentication requests
type LoginPayload struct {
	Cmd           int    `json:"cmd"`           // Command identifier for login
	Method        string `json:"method"`        // HTTP method (POST)
	Language      string `json:"language"`      // Interface language
	SessionId     string `json:"sessionId"`     // Session identifier
	Username      string `json:"username"`      // Login username
	Passwd        string `json:"passwd"`        // Password hash
	IsAutoUpgrade string `json:"isAutoUpgrade"` // Auto upgrade flag
}

// MonitorPayload represents the structure for monitoring requests
type MonitorPayload struct {
	Cmd       int    `json:"cmd"`       // Command identifier for monitoring
	Method    string `json:"method"`    // HTTP method (GET)
	Language  string `json:"language"`  // Interface language
	SessionId string `json:"sessionId"` // Session identifier
}

// RebootPayload represents the structure for reboot requests
type RebootPayload struct {
	Cmd        int    `json:"cmd"`        // Command identifier for reboot
	RebootType int    `json:"rebootType"` // Type of reboot to perform
	Method     string `json:"method"`     // HTTP method (POST)
	SessionId  string `json:"sessionId"`  // Session identifier
	Language   string `json:"language"`   // Interface language
}

// ResponseData represents the structure of API responses
type ResponseData struct {
	FREQ_5G   interface{} `json:"FREQ_5G"`      // 5G frequency
	FREQ      interface{} `json:"FREQ"`         // 4G frequency
	Success   bool        `json:"success"`      // Operation success flag
	Uptime    interface{} `json:"uptime"`       // System uptime
	SessionId interface{} `json:"sessionId"`    // Session identifier
	RSRQ      interface{} `json:"RSRQ"`         // 4G Reference Signal Received Quality
	RSRQ_5G   interface{} `json:"RSRQ_5G"`      // 5G Reference Signal Received Quality
	WAN_rX    interface{} `json:"wan_rx_bytes"` // WAN received bytes
	WAN_tX    interface{} `json:"wan_tx_bytes"` // WAN transmitted bytes
}

// Important constants for application behavior
const (
	maxRetries   = 5                // Maximum number of API retry attempts
	baseDelay    = 1 * time.Second  // Initial retry delay
	maxDelay     = 32 * time.Second // Maximum retry delay
	rebootSleep  = 60 * time.Second // Sleep duration after reboot command
	rebootWait   = 60 * 4           // Minimum uptime before allowing reboot
	recoverTime  = 5                // Time allowed for 5G signal recovery
	maxLogs      = 15               // Maximum number of logs to keep in memory
	recoverBytes = 10000000         // Maximum bytes during 5G recovery (10MB)
)

// Custom writer for capturing log output
type logWriter struct {
	program *tea.Program
}

func (l logWriter) Write(p []byte) (n int, err error) {
	log := strings.TrimSpace(string(p))
	if lastlog != log {
		l.program.Send(logMsg(log))
		lastlog = log
	}
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
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		m.inputBuffer += msg.String()
		if strings.HasSuffix(m.inputBuffer, "/q") {
			return m, tea.Quit
		}
		if strings.HasSuffix(m.inputBuffer, "/r") {
			m.inputBuffer = "" // Clear buffer to prevent re-triggering
			return m, m.rebootCmd()
		}
		if len(m.inputBuffer) > 2 {
			m.inputBuffer = m.inputBuffer[len(m.inputBuffer)-2:]
		}

	case tea.WindowSizeMsg:
		headerHeight := 15
		footerHeight := 1
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
		m.freqValue = string(msg)

	case freq5gUpdateMsg:
		m.freq5GValue = string(msg)

	case lastRebootTimeMsg:
		m.lastRebootTime = string(msg)

	case rxMsg:
		rxBytes, err := strconv.Atoi(string(msg))
		if err == nil {
			m.rxBytes = rxBytes
		}

	case txMsg:
		txBytes, err := strconv.Atoi(string(msg))
		if err == nil {
			m.txBytes = txBytes
		}

	case rsrqMsg:
		rsrqValue, err := strconv.Atoi(string(msg))
		if err == nil {
			m.rsrqValue = rsrqValue
		}

	case rsrq5gMsg:
		rsrq5GValue, err := strconv.Atoi(string(msg))
		if err == nil {
			m.rsrq5GValue = rsrq5GValue
		}

	case uptimeUpdateMsg:
		uptimeValue, err := strconv.Atoi(string(msg))
		if err == nil {
			m.uptimeValue = uptimeValue
		}

	case logMsg:
		m.logs = append(m.logs, string(msg))
		if len(m.logs) > maxLogs {
			m.logs = m.logs[len(m.logs)-maxLogs:]
		}

		// Notify on INFO log
		if strings.Contains(string(msg), "INFO") {
			err := beeep.Notify("vn007go", string(msg), filepath.Join(m.exePath, "assets/vn007logo.png"))
			if err != nil {
				log.Error("notification failed", "error", err)
			}
		}

		// Apply color formatting based on log level
		for i, log := range m.logs {
			switch {
			case errorRegex.MatchString(log):
				m.logs[i] = errorRegex.ReplaceAllString(log, errorStyle.Render("ERRO"))
			case warnRegex.MatchString(log):
				m.logs[i] = warnRegex.ReplaceAllString(log, warnStyle.Render("WARN"))
			case infoRegex.MatchString(log):
				m.logs[i] = infoRegex.ReplaceAllString(log, infoStyle.Render("INFO"))
			case debugRegex.MatchString(log):
				m.logs[i] = debugRegex.ReplaceAllString(log, debugStyle.Render("DEBU"))
			case fatalRegex.MatchString(log):
				m.logs[i] = fatalRegex.ReplaceAllString(log, fatalStyle.Render("FATA"))

			}
		}

		m.viewport.SetContent(strings.Join(m.logs, "\n"))
		m.viewport.GotoBottom()
	}

	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m model) rebootCmd() tea.Cmd {
	return func() tea.Msg {
		go manualReboot(m.client, m.url)
		return nil
	}
}

func manualReboot(client *http.Client, url string) {
	loginPayload := LoginPayload{
		Cmd:           100,
		Method:        "POST",
		SessionId:     "",
		Username:      os.Getenv("UNICOM_USER"),
		Passwd:        os.Getenv("PASSWORD_HASH"),
		IsAutoUpgrade: "0",
		Language:      "EN",
	}

	rebootPayload := RebootPayload{
		Cmd:        6,
		RebootType: 1,
		Method:     "POST",
		SessionId:  "",
		Language:   "EN",
	}

	log.Info("Manual reboot initiated by user")

	responseData, err := sendRequestWithRetry(client, url, loginPayload, "Login")

	if err != nil || (responseData != nil && !responseData.Success) {
		log.Warn("login failed for manual reboot", "error", err)
		return
	}

	if responseData == nil {
		log.Error("login response was nil for manual reboot")
		return
	}

	rebootPayload.SessionId = responseData.SessionId.(string)
	_, err = sendRequestWithRetry(client, url, rebootPayload, "Reboot")
	if err != nil {
		log.Error("manual reboot sequence failed", "error", err)
		return
	}

	log.Info("manual reboot sequence completed")
}

func (m model) View() string {
	if !m.ready {
		return "Initializing..."
	}

	// Header with FREQG value
	var freqDisplay string
	if m.freqValue == "" || m.freqValue == "NA" {
		freqDisplay = textStyle.Background(lipgloss.Color("211")). // pink
										Render("NA")
	} else {
		freqDisplay = textStyle.Foreground(lipgloss.Color("82")). // lime
										Render(fmt.Sprintf("%7s", m.freqValue))
	}

	// Header with FREQ_5G value
	var freq5GDisplay string
	if m.freq5GValue == "" || m.freq5GValue == "NA" {
		freq5GDisplay = textStyle.Copy().
			Background(lipgloss.Color("211")). // pink
			Render("NA")
	} else {
		freq5GDisplay = textStyle.Foreground(lipgloss.Color("82")). // lime
										Render(fmt.Sprintf("%7s", m.freq5GValue))
	}

	// Header with Uptime value
	uptimeDisplay := "0"
	hh, mm, ss := secondsToTime(m.uptimeValue)
	if m.uptimeValue < rebootWait {
		uptimeDisplay = textStyle.Foreground(lipgloss.Color("211")). // pink
										Render(fmt.Sprintf("%d:%02d:%02d", hh, mm, ss))
	} else {
		uptimeDisplay = textStyle.Foreground(lipgloss.Color("82")). // lime
										Render(fmt.Sprintf("%d:%02d:%02d", hh, mm, ss))
	}

	rsrqDisplay := "0"

	if m.rsrqValue < -15 {
		rsrqDisplay = textStyle.Foreground(lipgloss.Color("#ff38c7")). // pink
										Render(fmt.Sprintf("%3d ■□□□", m.rsrqValue))

	} else if m.rsrqValue <= -10 {
		rsrqDisplay = textStyle.Foreground(lipgloss.Color("#ffd438")). // yellow
										Render(fmt.Sprintf("%3d ■■□□", m.rsrqValue))

	} else if m.rsrqValue <= -5 {
		rsrqDisplay = textStyle.Foreground(lipgloss.Color("#68e1fc")). // cyab
										Render(fmt.Sprintf("%3d ■■■□", m.rsrqValue))

	} else {
		rsrqDisplay = textStyle.Foreground(lipgloss.Color("#80fc68")). // line
										Render(fmt.Sprintf("%3d ■■■■", m.rsrqValue))

	}

	rsrq5GDisplay := "0"

	if m.rsrq5GValue <= -15 {
		rsrq5GDisplay = textStyle.Foreground(lipgloss.Color("#ff38c7")). // pink
											Render(fmt.Sprintf("%3d ■□□□", m.rsrq5GValue))

	} else if m.rsrq5GValue <= -9 {
		rsrq5GDisplay = textStyle.Foreground(lipgloss.Color("#ffd438")). // yellow
											Render(fmt.Sprintf("%3d ■■□□", m.rsrq5GValue))

	} else if m.rsrq5GValue <= -5 {
		rsrq5GDisplay = textStyle.Foreground(lipgloss.Color("#68e1fc")). // cyab
											Render(fmt.Sprintf("%3d ■■■□", m.rsrq5GValue))

	} else {
		rsrq5GDisplay = textStyle.Foreground(lipgloss.Color("#80fc68")). // line
											Render(fmt.Sprintf("%3d ■■■■", m.rsrq5GValue))

	}

	// Header with Uptime value
	rebootDisplay := "NONE"
	if m.lastRebootTime != "NONE" {
		rebootDisplay = textStyle.Foreground(lipgloss.Color("211")). // pink
										Render(fmt.Sprintf("%ss", m.lastRebootTime))
	} else {
		rebootDisplay = textStyle.Foreground(lipgloss.Color("82")). // lime
										Render("NONE")
	}

	header := fmt.Sprintf("%s\n%s\n\n%s%s \t   %s%s \n%s%s \t  %s%s \n%s%8.2fMB \t %s%8.2fMB \n%s%s \n%s%s \n\n%s",
		titleStyle.Width(32).Align(lipgloss.Center).Render("Vn007 Auto-Restart"),
		titleStyle.Width(32).Align(lipgloss.Center).Render("------------------"),
		titleStyle.Render("4G "), freqDisplay, titleStyle.Render("5G "), freq5GDisplay,
		titleStyle.Render("ᯤ: "), rsrqDisplay, titleStyle.Render("ᯤ: "), rsrq5GDisplay,
		titleStyle.Render("↑U"), float32(m.txBytes)*0.000001, titleStyle.Render("↓D"), float32(m.rxBytes)*0.000001,
		titleStyle.Render("UPtime: "), uptimeDisplay,
		titleStyle.Render("REboot: "), rebootDisplay,
		titleStyle.Width(32).Align(lipgloss.Center).Render("press '/q' to stop, '/r' to reboot."))

	header = headerStyle.Render(header)
	// Viewport with logsq
	return fmt.Sprintf("%s\n%s", header, m.viewport.View())
}

// calculateBackoff implements exponential backoff for retries
func calculateBackoff(attempt int) time.Duration {
	delay := baseDelay * time.Duration(1<<uint(attempt))
	if delay > maxDelay {
		delay = maxDelay
	}
	return delay
}

// secondsToTime converts seconds into hours, minutes, and seconds
func secondsToTime(seconds int) (hours, minutes, secs int) {
	hours = seconds / 3600
	minutes = (seconds % 3600) / 60
	secs = seconds % 60
	return
}

// monitorService continuously monitors the router's status and manages reboots
// It handles:
// - Monitoring 4G/5G connectivity
// - Tracking data usage
// - Managing automatic reboots when 5G connection is lost
// - Session management and authentication
func monitorService(program *tea.Program, client *http.Client, url string) {

	var uptime5g int
	var bytes5G int
	uptime5g = 0
	bytes5G = 0

	monitorPayload := MonitorPayload{
		Cmd:       133,
		Method:    "GET",
		Language:  "EN",
		SessionId: "",
	}

	loginPayload := LoginPayload{
		Cmd:           100,
		Method:        "POST",
		SessionId:     "",
		Username:      os.Getenv("UNICOM_USER"),
		Passwd:        os.Getenv("PASSWORD_HASH"),
		IsAutoUpgrade: "0",
		Language:      "EN",
	}

	rebootPayload := RebootPayload{
		Cmd:        6,
		RebootType: 1,
		Method:     "POST",
		SessionId:  "",
		Language:   "EN",
	}

	log.Info("Starting monitoring service")

	for {
		responseData, err := sendRequestWithRetry(client, url, monitorPayload, "Monitoring")

		if err != nil {
			log.Error("monitoring cycle failed", "error", err, "sleep", baseDelay)
			time.Sleep(baseDelay)
			continue
		}

		if responseData.Uptime == nil {
			log.Warn("uptime not found", "sleep", baseDelay)
			time.Sleep(baseDelay)
			continue
		}
		uptimeStr := responseData.Uptime.(string)
		uptime, err := strconv.Atoi(uptimeStr)
		if err != nil {
			log.Warn("uptime not found", "sleep", baseDelay)
			time.Sleep(baseDelay)
			continue
		}
		program.Send(uptimeUpdateMsg(uptimeStr))

		if responseData.WAN_rX == nil {
			log.Warn("WAN_rx not found", "sleep", baseDelay)
			time.Sleep(baseDelay)
			continue
		}
		rxStr := responseData.WAN_rX.(string)
		rx, err := strconv.Atoi(rxStr)
		if err != nil {
			log.Warn("WAN_rX not found", "sleep", baseDelay)
			time.Sleep(baseDelay)
			continue
		}
		program.Send(rxMsg(rxStr))

		if responseData.WAN_tX == nil {
			log.Warn("WAN_tX not found", "sleep", baseDelay)
			time.Sleep(baseDelay)
			continue
		}
		txStr := responseData.WAN_tX.(string)
		tx, err := strconv.Atoi(txStr)
		if err != nil {
			log.Warn("WAN_tX not found", "sleep", baseDelay)
			time.Sleep(baseDelay)
			continue
		}
		program.Send(txMsg(txStr))

		rsrq5tr := "0"
		if responseData.RSRQ == nil {
			log.Warn("RSRQ not found")
		} else {
			rsrq5tr = responseData.RSRQ.(string)
			_, err = strconv.Atoi(rsrq5tr)
			if err != nil {
				log.Warn("RSRQ not found")
			}
		}

		program.Send(rsrqMsg(rsrq5tr))

		rsrq5gStr := "0"
		if responseData.RSRQ_5G == nil {
			log.Warn("RSRQ 5G not found")
		} else {
			rsrq5gStr = responseData.RSRQ_5G.(string)
			_, err = strconv.Atoi(rsrq5gStr)
			if err != nil {
				log.Warn("RSRQ 5G not found")
			}

		}

		program.Send(rsrq5gMsg(rsrq5gStr))

		log.Debug("Total traffic", "MB", float32(tx+rx)*0.000001)

		if responseData.FREQ == nil {
			program.Send(freqUpdateMsg("NA"))
			log.Debug("No Data Connection", "sleep", baseDelay)
			time.Sleep(baseDelay)
			continue
		}
		_, fqerr := strconv.Atoi(responseData.FREQ.(string))
		if fqerr != nil {
			program.Send(freqUpdateMsg("NA"))
			log.Debug("No Data Connection", "sleep", baseDelay)
			time.Sleep(baseDelay)
			continue
		}
		program.Send(freqUpdateMsg(responseData.FREQ.(string)))
		log.Debug("4G available", "FREQ", responseData.FREQ.(string))

		if responseData.FREQ_5G != nil {
			_, fqerr := strconv.Atoi(responseData.FREQ_5G.(string))
			if fqerr == nil {
				program.Send(freq5gUpdateMsg(responseData.FREQ_5G.(string)))
				log.Debug("5G available", "FREQ_5G", responseData.FREQ_5G.(string))
				uptime5g = uptime
				bytes5G = tx + rx
				time.Sleep(baseDelay)
				continue
			}
		}

		program.Send(freq5gUpdateMsg("NA"))

		if uptime5g == 0 {
			uptime5g = uptime
		}

		if bytes5G == 0 {
			bytes5G = tx + rx
		}

		timediff := uptime - uptime5g
		bytesdiff := tx + rx - bytes5G

		if (timediff < recoverTime) && bytesdiff < recoverBytes {
			log.Warn("5G recovery", "downtime(sec)", timediff)
			log.Warn("4G data used", "MB", float32(bytesdiff)*0.000001)
			//no delay
			continue
		}

		log.Warn("FREQ_5G not present, initiating reboot")

		responseData, err = sendRequestWithRetry(client, url, loginPayload, "Login")

		if (err != nil) || (!responseData.Success) {
			log.Warn("login failed", "error", err, "sleep", baseDelay)
			time.Sleep(180)
			continue
		}

		rebootPayload.SessionId = responseData.SessionId.(string)
		_, err = sendRequestWithRetry(client, url, rebootPayload, "Reboot")
		if err != nil {
			log.Error("reboot sequence failed", "error", err, "sleep", rebootSleep)
			time.Sleep(120 * time.Second)
			continue
		}

		program.Send(lastRebootTimeMsg(time.Now().Format("January 2, 2006 3:04:05 PM")))
		log.Info("reboot sequence completed", "sleep", rebootSleep)
		time.Sleep(rebootSleep)
		uptime5g = 0
	}
}

// sendRequestWithRetry sends API requests with retry logic
// It implements:
// - Exponential backoff
// - Request marshaling
// - Response handling
// - Error management
func sendRequestWithRetry(client *http.Client, url string, payload interface{}, reqType string) (*ResponseData, error) {

	var lastErr error

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

		// log.Debug(fmt.Sprintf("REQ <<< %s", jsonData))

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
		// log.Debug(fmt.Sprintf("RESP >>> %s", body))

		if reqType == "Reboot" && resp.StatusCode == 200 {
			log.Debug("request successful", "type", reqType, "attempt", attempt+1)
			responseData.Success = true
			responseData.FREQ_5G = "-"
			responseData.Uptime = 0
			return &responseData, nil
		}

		err = json.Unmarshal(body, &responseData)
		if err != nil {
			lastErr = err
			delay := calculateBackoff(attempt)
			log.Error("invalid JSON response", "type", reqType, "attempt", attempt+1, "error", err)
			time.Sleep(delay)
			continue
		}

		if reqType == "Login" && responseData.SessionId == nil {
			log.Debug("authentication failed", "type", reqType, "attempt", attempt+1)
			responseData.Success = false
			return &responseData, nil
		}

		if responseData.Success {
			log.Debug("request successful", "type", reqType, "attempt", attempt+1)
			return &responseData, nil
		}

		lastErr = fmt.Errorf("request failed with success=false")
		delay := calculateBackoff(attempt)
		log.Error("request unsuccessful", "type", reqType, "attempt", attempt+1)
		time.Sleep(delay)
	}

	return nil, fmt.Errorf("max retries (%d) exceeded with error: %v", maxRetries, lastErr)
}

// main initializes and runs the application
// It sets up:
// - Environment configuration
// - Logging
// - TUI (Terminal User Interface)
// - Monitoring service
func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	exePath, err := filepath.Abs(filepath.Dir(os.Args[0]))
	if err != nil {
		log.Fatal(err)
	}

	// Start monitoring service in a goroutine
	client := &http.Client{Timeout: 10 * time.Second}
	url := fmt.Sprintf("http://%s/cgi-bin/http.cgi", os.Getenv("IP"))

	// Initial model
	m := model{
		logs:           make([]string, 0, maxLogs),
		freq5GValue:    "NA",
		uptimeValue:    0,
		lastRebootTime: "NONE",
		exePath:        exePath,
		client:         client,
		url:            url,
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

	go monitorService(p, client, url)

	// Run the program
	if _, err := p.Run(); err != nil {
		fmt.Println("Error running program:", err)
	}
}
