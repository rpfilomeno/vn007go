package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/log"
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

type LoginPayload struct {
	Cmd           int    `json:"cmd"`
	Method        string `json:"method"`
	Language      string `json:"language"`
	SessionId     string `json:"sessionId"`
	Username      string `json:"username"`
	Passwd        string `json:"passwd"`
	IsAutoUpgrade string `json:"isAutoUpgrade"`
}
type MonitorPayload struct {
	Cmd       int    `json:"cmd"`
	Method    string `json:"method"`
	Language  string `json:"language"`
	SessionId string `json:"sessionId"`
}

type RebootPayload struct {
	Cmd        int    `json:"cmd"`
	RebootType int    `json:"rebootType"`
	Method     string `json:"method"`
	SessionId  string `json:"sessionId"`
	Language   string `json:"language"`
}

type GetInfoPayload struct {
	Cmd       int    `json:"cmd"`
	Method    string `json:"method"`
	SessionId string `json:"sessionId"`
	Language  string `json:"language"`
}
type ResponseData struct {
	FREQ_5G   interface{} `json:"FREQ_5G"`
	FREQ      interface{} `json:"FREQ"`
	Success   bool        `json:"success"`
	Uptime    interface{} `json:"uptime"`
	SessionId interface{} `json:"sessionId"`
	RSRQ      interface{} `json:"RSRQ"`
	RSRQ_5G   interface{} `json:"RSRQ_5G"`
	WAN_rX    interface{} `json:"wan_rx_bytes"`
	WAN_tX    interface{} `json:"wan_tx_bytes"`
}

const (
	maxRetries   = 5
	baseDelay    = 1 * time.Second
	maxDelay     = 32 * time.Second
	rebootSleep  = 60 * time.Second //sleep after reboot command is sent
	rebootWait   = 60 * 4           // max uptime secs before it can reboot
	recoverTime  = 5                //max secs to allow 5g signal to recover before rebooting
	maxLogs      = 15               // Maximum number of logs to keep in memory
	recoverBytes = 10000000         // Maximum bytes allowed to be used during %g recovery failure default: 10000000 (10MB)
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
		if msg.String() == "q" || msg.String() == "ctrl+c" {
			return m, tea.Quit
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
		titleStyle.Width(32).Align(lipgloss.Center).Render("press 'q' to stop."))

	header = headerStyle.Render(header)
	// Viewport with logsq
	return fmt.Sprintf("%s\n%s", header, m.viewport.View())
}

func calculateBackoff(attempt int) time.Duration {
	delay := baseDelay * time.Duration(1<<uint(attempt))
	if delay > maxDelay {
		delay = maxDelay
	}
	return delay
}

func secondsToTime(seconds int) (hours, minutes, secs int) {
	hours = seconds / 3600
	minutes = (seconds % 3600) / 60
	secs = seconds % 60
	return
}

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

	for {
		responseData, err := sendRequestWithRetry(program, client, url, monitorPayload, "Monitoring")

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

		// The most important check
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

		responseData, err = sendRequestWithRetry(program, client, url, loginPayload, "Login")

		if (err != nil) || (!responseData.Success) {
			log.Warn("login failed", "error", err, "sleep", baseDelay)
			time.Sleep(180)
			continue
		}

		rebootPayload.SessionId = responseData.SessionId.(string)
		_, err = sendRequestWithRetry(program, client, url, rebootPayload, "Reboot")
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

func sendRequestWithRetry(program *tea.Program, client *http.Client, url string, payload interface{}, reqType string) (*ResponseData, error) {

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

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	// Initial model
	m := model{
		logs:           make([]string, 0, maxLogs),
		freq5GValue:    "NA",
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
