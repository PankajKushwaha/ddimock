package main

import (
	"log"
	"net/http"

	"ddimock/internal/ipam"
)

func main() {
	log.Println("[IPAM] Starting IPAM microservice")

	service := ipam.NewIPAMService("http://localhost:5004")

	log.Println("[IPAM] Connected to IP-POOL at localhost:5004")
	log.Println("[IPAM] Listening on port 5001")

	if err := http.ListenAndServe(":5001", service.Handler()); err != nil {
		log.Fatalf("[IPAM] Server stopped: %v", err)
	}
}
