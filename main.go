package main

import (
	"log"
	"net/http"
	"os"
	"strconv"
)

var (
	appID               int64
	installationID      int64 // remain dynamically set in webhook
	privateKeyPath      string
	githubWebhookSecret string
)

func init() {
	// Fetch values from environment variables
	var err error

	appIDEnv := os.Getenv("APP_ID")
	appID, err = strconv.ParseInt(appIDEnv, 10, 64)
	if err != nil {
		log.Fatalf("Error parsing APP_ID: %v", err)
	}

	privateKeyPath = os.Getenv("PRIVATE_KEY_PATH")
	if privateKeyPath == "" {
		log.Fatalf("PRIVATE_KEY_PATH environment variable is not set")
	}

	githubWebhookSecret = os.Getenv("GITHUB_WEBHOOK_SECRET")
	if githubWebhookSecret == "" {
		log.Fatalf("GITHUB_WEBHOOK_SECRET environment variable is not set")
	}
}

func main() {
	go runScheduledJobs() // Start scheduler in background

	http.HandleFunc("/webhook", handleWebhook)
	log.Println("Listening on :8080")
	http.ListenAndServe(":8080", nil)
}
