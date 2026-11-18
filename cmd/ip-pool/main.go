package main

import (
	"log"
	"net/http"

	ippool "ddimock/internal/ip-pool"
)

func main() {
	log.Println("[IP-POOL] Starting IP Pool microservice")

	pool, err := ippool.NewIPPool("data/ip-pool.txt")
	if err != nil {
		log.Fatalf("[IP-POOL] Failed to initialize: %v", err)
	}

	handler := ippool.NewHTTPHandler(pool)

	log.Println("[IP-POOL] Inventory loaded successfully")
	log.Println("[IP-POOL] Listening on port 5004")

	if err := http.ListenAndServe(":5004", handler); err != nil {
		log.Fatalf("[IP-POOL] Server stopped: %v", err)
	}
}
