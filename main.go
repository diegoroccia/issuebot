package main

import (
	"log"
	"net/http"
)

var (
	appID          = int64(1252582)      // replace with your GitHub App ID
	installationID = int64(0)            // populated dynamically per webhook
	privateKeyPath = "./private-key.pem" // path to your GitHub App private key
)

func main() {
	go runScheduledJobs() // Start scheduler in background

	http.HandleFunc("/webhook", handleWebhook)
	log.Println("Listening on :3000")
	http.ListenAndServe(":8080", nil)
}
