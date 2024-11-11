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
		SessionId:     os.Getenv("SESSION_ID"),
		Username:      os.Getenv("USERNAME"),
		Passwd:        os.Getenv("PASSWORD_HASH"),
		IsAutoUpgrade: "0",
		Language:      "EN",
	}

	client := &http.Client{Timeout: 10 * time.Second}

	url := fmt.Sprintf("http://%s/cgi-bin/http.cgi", os.Getenv("IP"))

	_, err = sendRequestWithRetry(nil, client, url, loginPayload, "Login")

	if err != nil {
		t.Fatalf("Login failed: %s", err)
	} else {
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
		SessionId:  os.Getenv("SESSION_ID"),
		Language:   "EN",
	}

	client := &http.Client{Timeout: 10 * time.Second}

	url := fmt.Sprintf("http://%s/cgi-bin/http.cgi", os.Getenv("IP"))

	_, err = sendRequestWithRetry(nil, client, url, rebootPayload, "Reboot")

	if err != nil {
		t.Fatalf("Reboot failed: %s", err)
	} else {
		t.Logf("Reboot OK")
	}
}
