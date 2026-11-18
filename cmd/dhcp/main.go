package main

import (
	"log"
	"net/http"
	"time"

	"ddimock/internal/dhcp"
)

func main() {
	log.Println("[DHCP] Starting DHCP microservice")

	service, err := dhcp.NewDHCPService(
		"http://localhost:5004",
		"data/dhcp-leases.csv",
	)
	if err != nil {
		log.Fatalf("[DHCP] Initialization failed: %v", err)
	}

	// Check for expired leases every 10 seconds.
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			service.ExpireLeases()
		}
	}()

	log.Println("[DHCP] Connected to IP-POOL at localhost:5004")
	log.Println("[DHCP] Listening on port 5002")

	if err := http.ListenAndServe(":5002", service.Handler()); err != nil {
		log.Fatalf("[DHCP] Server stopped: %v", err)
	}
}
