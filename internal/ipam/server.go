
package ipam

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

type IPAMService struct {
	poolURL string
	client  *http.Client
}

func NewIPAMService(poolURL string) *IPAMService {
	return &IPAMService{
		poolURL: strings.TrimRight(poolURL, "/"),
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

func (s *IPAMService) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.health)
	mux.HandleFunc("/allocate", s.allocate)
	mux.HandleFunc("/release", s.release)
	mux.HandleFunc("/ips", s.listIPs)
	return mux
}

func (s *IPAMService) health(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"service":"IPAM","status":"UP"}`))
}

func (s *IPAMService) allocate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var request struct {
		Network  string `json:"network"`
		Hostname string `json:"hostname"`
		MAC      string `json:"mac"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	log.Printf("[IPAM] Allocate request for %s", request.Hostname)
	log.Println("[IPAM] Calling IP-POOL service")

	body, _ := json.Marshal(map[string]string{
		"network":  request.Network,
		"hostname": request.Hostname,
		"mac":      request.MAC,
		"owner":    "ipam",
	})

	response, err := s.client.Post(
		s.poolURL+"/allocate",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		http.Error(w, "IP-POOL service unavailable: "+err.Error(),
			http.StatusServiceUnavailable)
		return
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		http.Error(w, "IP-POOL allocation failed", response.StatusCode)
		return
	}

	log.Println("[IPAM] Allocation completed successfully")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(response.StatusCode)
	json.NewEncoder(w).Encode(json.RawMessage(readBody(response)))
}

func readBody(response *http.Response) []byte {
	var result json.RawMessage
	json.NewDecoder(response.Body).Decode(&result)
	return result
}

func (s *IPAMService) release(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var request struct {
		IP string `json:"ip"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	log.Printf("[IPAM] Manual release requested for %s", request.IP)
	log.Println("[IPAM] Calling IP-POOL service")

	body, _ := json.Marshal(map[string]string{
		"ip":    request.IP,
		"owner": "ipam",
	})

	response, err := s.client.Post(
		s.poolURL+"/release",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		http.Error(w, "IP-POOL service unavailable",
			http.StatusServiceUnavailable)
		return
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		http.Error(w, "IP release failed", response.StatusCode)
		return
	}

	log.Printf("[IPAM] IP %s released successfully", request.IP)
	w.Write([]byte(`{"status":"released"}`))
}

func (s *IPAMService) listIPs(w http.ResponseWriter, r *http.Request) {
	response, err := s.client.Get(s.poolURL + "/ips")
	if err != nil {
		http.Error(w, "IP-POOL service unavailable",
			http.StatusServiceUnavailable)
		return
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		http.Error(w, fmt.Sprintf("IP-POOL returned %d", response.StatusCode),
			response.StatusCode)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(json.RawMessage(readBody(response)))
}

