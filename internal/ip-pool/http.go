package ippool

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
)

type HTTPServer struct {
	pool *IPPool
}

func NewHTTPHandler(pool *IPPool) http.Handler {
	server := &HTTPServer{pool: pool}
	mux := http.NewServeMux()

	mux.HandleFunc("/health", server.health)
	mux.HandleFunc("/ips", server.listIPs)
	mux.HandleFunc("/ips/", server.getIP)
	mux.HandleFunc("/allocate", server.allocate)
	mux.HandleFunc("/release", server.release)

	return mux
}

func (s *HTTPServer) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"service": "IP-POOL",
		"status":  "UP",
	})
}

func (s *HTTPServer) listIPs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	log.Println("[IP-POOL] Received ListIPs request")

	records, err := s.pool.ListIPs()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, records)
}

func (s *HTTPServer) getIP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ip := strings.TrimPrefix(r.URL.Path, "/ips/")
	if ip == "" {
		http.Error(w, "IP address required", http.StatusBadRequest)
		return
	}

	log.Printf("[IP-POOL] Received GetIP request: %s", ip)

	record, err := s.pool.GetIP(ip)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	writeJSON(w, http.StatusOK, record)
}

func (s *HTTPServer) allocate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var request struct {
		Network  string `json:"network"`
		Hostname string `json:"hostname"`
		MAC      string `json:"mac"`
		Owner    string `json:"owner"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	log.Printf("[IP-POOL] Allocation request: network=%s owner=%s",
		request.Network, request.Owner)

	record, err := s.pool.AllocateNextIP(
		request.Network,
		request.Hostname,
		request.MAC,
		request.Owner,
	)
	if err != nil {
		log.Printf("[IP-POOL] Allocation failed: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	log.Printf("[IP-POOL] Successfully allocated IP %s", record.IP)

	writeJSON(w, http.StatusOK, record)
}

func writeJSON(w http.ResponseWriter, status int, value interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(value)
}

func (s *HTTPServer) release(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var request struct {
		IP    string `json:"ip"`
		Owner string `json:"owner"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	log.Printf("[IP-POOL] Release request: IP=%s Owner=%s",
		request.IP, request.Owner)

	if request.Owner != "ipam" && request.Owner != "dhcp" {
		http.Error(w, "invalid owner", http.StatusBadRequest)
		return
	}

	if err := s.pool.ReleaseIP(request.IP, request.Owner); err != nil {
		log.Printf("[IP-POOL] Release failed: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	log.Printf("[IP-POOL] Released IP %s", request.IP)
	writeJSON(w, http.StatusOK, map[string]string{"status": "released"})
}
