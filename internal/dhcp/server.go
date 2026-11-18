
package dhcp

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Lease struct {
	ID        string    `json:"id"`
	IP        string    `json:"ip"`
	Hostname  string    `json:"hostname"`
	MAC       string    `json:"mac"`
	ExpiresAt time.Time `json:"expires_at"`
}

type DHCPService struct {
	poolURL   string
	leaseFile string
	client    *http.Client
	mutex     sync.Mutex
	leases    map[string]Lease
}

func NewDHCPService(poolURL, leaseFile string) (*DHCPService, error) {
	s := &DHCPService{
		poolURL:   strings.TrimRight(poolURL, "/"),
		leaseFile: leaseFile,
		client:    &http.Client{Timeout: 5 * time.Second},
		leases:    make(map[string]Lease),
	}

	if err := s.loadLeases(); err != nil {
		return nil, err
	}

	return s, nil
}

func (s *DHCPService) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.health)
	mux.HandleFunc("/allocate", s.allocate)
	mux.HandleFunc("/renew", s.renew)
	mux.HandleFunc("/release", s.release)
	mux.HandleFunc("/leases", s.listLeases)
	return mux
}

func (s *DHCPService) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"service": "DHCP",
		"status":  "UP",
	})
}

func (s *DHCPService) allocate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}

	var req struct {
		Network        string `json:"network"`
		Hostname       string `json:"hostname"`
		MAC            string `json:"mac"`
		DurationSecond int    `json:"duration_seconds"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", 400)
		return
	}

	if req.DurationSecond <= 0 {
		http.Error(w, "duration_seconds must be positive", 400)
		return
	}

	log.Printf("[DHCP] Allocate lease requested for %s", req.Hostname)
	log.Println("[DHCP] Requesting dynamic IP from IP-POOL")

	body, _ := json.Marshal(map[string]string{
		"network":  req.Network,
		"hostname": req.Hostname,
		"mac":      req.MAC,
		"owner":    "dhcp",
	})

	response, err := s.client.Post(
		s.poolURL+"/allocate",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		http.Error(w, "IP-POOL unavailable: "+err.Error(), 503)
		return
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		data, _ := ioutil.ReadAll(response.Body)
		http.Error(w, string(data), response.StatusCode)
		return
	}

	var allocated struct {
		IP string `json:"IP"`
	}
	if err := json.NewDecoder(response.Body).Decode(&allocated); err != nil {
		http.Error(w, "invalid IP-POOL response", 502)
		return
	}

	if allocated.IP == "" {
		http.Error(w, "IP-POOL response did not contain an IP", 502)
		return
	}

	lease := Lease{
		ID:        fmt.Sprintf("%d", time.Now().UnixNano()),
		IP:        allocated.IP,
		Hostname:  req.Hostname,
		MAC:       req.MAC,
		ExpiresAt: time.Now().Add(time.Duration(req.DurationSecond) * time.Second),
	}

	s.mutex.Lock()
	s.leases[lease.ID] = lease
	err = s.saveLeasesLocked()
	s.mutex.Unlock()

	if err != nil {
		// Roll back the allocation if the lease could not be persisted.
		s.releasePoolIP(lease.IP)
		http.Error(w, "could not save lease", 500)
		return
	}

	log.Printf("[DHCP] Lease created: ID=%s IP=%s", lease.ID, lease.IP)
	writeJSON(w, 200, lease)
}

func (s *DHCPService) renew(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}

	var req struct {
		LeaseID        string `json:"lease_id"`
		DurationSecond int    `json:"duration_seconds"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", 400)
		return
	}

	if req.DurationSecond <= 0 {
		http.Error(w, "duration_seconds must be positive", 400)
		return
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()

	lease, ok := s.leases[req.LeaseID]
	if !ok {
		http.Error(w, "lease not found", 404)
		return
	}

	if time.Now().After(lease.ExpiresAt) {
		http.Error(w, "lease expired", 410)
		return
	}

	lease.ExpiresAt = time.Now().Add(
		time.Duration(req.DurationSecond) * time.Second,
	)
	s.leases[req.LeaseID] = lease

	if err := s.saveLeasesLocked(); err != nil {
		http.Error(w, "could not save lease", 500)
		return
	}

	log.Printf("[DHCP] Lease renewed: ID=%s IP=%s", lease.ID, lease.IP)
	writeJSON(w, 200, lease)
}

func (s *DHCPService) release(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}

	var req struct {
		LeaseID string `json:"lease_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", 400)
		return
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()

	lease, ok := s.leases[req.LeaseID]
	if !ok {
		http.Error(w, "lease not found", 404)
		return
	}

	if err := s.releasePoolIP(lease.IP); err != nil {
		http.Error(w, "IP-POOL release failed: "+err.Error(), 502)
		return
	}

	delete(s.leases, req.LeaseID)
	if err := s.saveLeasesLocked(); err != nil {
		http.Error(w, "could not save lease state", 500)
		return
	}

	log.Printf("[DHCP] Lease released: ID=%s IP=%s", lease.ID, lease.IP)
	writeJSON(w, 200, map[string]string{"status": "released"})
}

func (s *DHCPService) listLeases(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()

	result := make([]Lease, 0, len(s.leases))
	for _, lease := range s.leases {
		result = append(result, lease)
	}

	writeJSON(w, 200, result)
}

// Called periodically by the DHCP process.
func (s *DHCPService) ExpireLeases() {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	now := time.Now()
	changed := false

	for id, lease := range s.leases {
		if now.After(lease.ExpiresAt) {
			if err := s.releasePoolIP(lease.IP); err != nil {
				log.Printf("[DHCP] Could not release expired IP %s: %v",
					lease.IP, err)
				continue
			}

			delete(s.leases, id)
			changed = true
			log.Printf("[DHCP] Lease expired: ID=%s IP=%s", id, lease.IP)
		}
	}

	if changed {
		if err := s.saveLeasesLocked(); err != nil {
			log.Printf("[DHCP] Failed to save expired leases: %v", err)
		}
	}
}

func (s *DHCPService) releasePoolIP(ip string) error {
	body, _ := json.Marshal(map[string]string{
		"ip":    ip,
		"owner": "dhcp",
	})

	response, err := s.client.Post(
		s.poolURL+"/release",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		data, _ := ioutil.ReadAll(response.Body)
		return fmt.Errorf("%s", string(data))
	}

	return nil
}

func (s *DHCPService) loadLeases() error {
	file, err := os.Open(s.leaseFile)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if len(row) != 5 {
			continue
		}

		expiry, err := time.Parse(time.RFC3339, row[4])
		if err != nil {
			continue
		}

		lease := Lease{
			ID:        row[0],
			IP:        row[1],
			Hostname:  row[2],
			MAC:       row[3],
			ExpiresAt: expiry,
		}
		s.leases[lease.ID] = lease
	}

	return nil
}

func (s *DHCPService) saveLeasesLocked() error {
	file, err := os.Create(s.leaseFile)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	for _, lease := range s.leases {
		err := writer.Write([]string{
			lease.ID,
			lease.IP,
			lease.Hostname,
			lease.MAC,
			lease.ExpiresAt.Format(time.RFC3339),
		})
		if err != nil {
			return err
		}
	}
	writer.Flush()
	return writer.Error()
}

func writeJSON(w http.ResponseWriter, status int, value interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(value)
}

func ParseLeaseDuration(value string) (time.Duration, error) {
	seconds, err := strconv.Atoi(value)
	if err != nil || seconds <= 0 {
		return 0, fmt.Errorf("invalid lease duration")
	}
	return time.Duration(seconds) * time.Second, nil
}

