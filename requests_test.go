package main

import (
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/joho/godotenv"
)

var sessionId string

func init() {
	sessionId = ""
	log.SetLevel(log.DebugLevel)
}

func TestSendRequestWithRetry_Monitoring(t *testing.T) {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	monitorPayload := MonitorPayload{
		Cmd:       133,
		Method:    "GET",
		Language:  "EN",
		SessionId: "",
	}

	client := &http.Client{Timeout: 10 * time.Second}

	url := fmt.Sprintf("http://%s/cgi-bin/http.cgi", os.Getenv("IP"))

	_, err = sendRequestWithRetry(nil, client, url, monitorPayload, "Monitoring")

	if err != nil {
		t.Fatalf("Monitoring failed: %s", err)
	} else {
		t.Logf("Monitoring OK")
	}

}

func TestSendRequestWithRetry_Login(t *testing.T) {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
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

	client := &http.Client{Timeout: 10 * time.Second}

	url := fmt.Sprintf("http://%s/cgi-bin/http.cgi", os.Getenv("IP"))

	responseData, err := sendRequestWithRetry(nil, client, url, loginPayload, "Login")

	if err != nil || responseData.SessionId == nil {
		t.Fatalf("Login failed: %s", err)
	} else {
		sessionId = responseData.SessionId.(string)
		t.Logf("Login OK")
	}

}

func TestSendRequestWithRetry_Reboot(t *testing.T) {

	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	rebootPayload := RebootPayload{
		Cmd:        6,
		RebootType: 1,
		Method:     "POST",
		SessionId:  sessionId,
		Language:   "EN",
	}

	client := &http.Client{Timeout: 10 * time.Second}

	url := fmt.Sprintf("http://%s/cgi-bin/http.cgi", os.Getenv("IP"))

	responseData, err := sendRequestWithRetry(nil, client, url, rebootPayload, "Reboot")

	if err != nil || !responseData.Success {
		t.Fatalf("Reboot failed: %s", err)
	} else {
		t.Logf("Reboot OK")
	}
}
