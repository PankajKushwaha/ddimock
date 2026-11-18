package main

import (
	"log"
	"net/http"

	"ddimock/internal/dns"
)

func main() {
	log.Println("[DNS] Starting DNS microservice")

	service := dns.NewDNSService("data/dns-records.csv")

	log.Println("[DNS] DNS record storage initialized")
	log.Println("[DNS] Listening on port 5003")

	if err := http.ListenAndServe(":5003", dns.NewHTTPHandler(service)); err != nil {
		log.Fatalf("[DNS] Server stopped: %v", err)
	}
}
