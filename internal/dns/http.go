
package dns

import (
	"encoding/json"
	"log"
	"net/http"
)

type DNSHTTPServer struct {
	service *DNSService
}

func NewHTTPHandler(service *DNSService) http.Handler {
	server := &DNSHTTPServer{service: service}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", server.health)
	mux.HandleFunc("/zones", server.zones)
	mux.HandleFunc("/records", server.records)

	return mux
}

func (s *DNSHTTPServer) health(w http.ResponseWriter, r *http.Request) {
	writeDNSJSON(w, 200, map[string]string{
		"service": "DNS",
		"status":  "UP",
	})
}

func (s *DNSHTTPServer) zones(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}

	var req struct {
		Zone string `json:"zone"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", 400)
		return
	}

	log.Printf("[DNS] Create zone request: %s", req.Zone)

	if err := s.service.CreateZone(req.Zone); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	log.Printf("[DNS] Zone created: %s", req.Zone)
	writeDNSJSON(w, 200, map[string]string{
		"status": "created",
		"zone":   req.Zone,
	})
}

func (s *DNSHTTPServer) records(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.addRecord(w, r)
	case http.MethodGet:
		s.getRecords(w, r)
	case http.MethodDelete:
		s.deleteRecord(w, r)
	default:
		http.Error(w, "method not allowed", 405)
	}
}

func (s *DNSHTTPServer) addRecord(w http.ResponseWriter, r *http.Request) {
	var record DNSRecord

	if err := json.NewDecoder(r.Body).Decode(&record); err != nil {
		http.Error(w, "invalid JSON", 400)
		return
	}

	log.Printf("[DNS] Add record request: %s type=%s",
		record.Name, record.Type)

	if err := s.service.AddRecord(record); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	log.Printf("[DNS] Record created: %s -> %s",
		record.Name, record.Value)

	writeDNSJSON(w, 201, record)
}

func (s *DNSHTTPServer) getRecords(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	recordType := r.URL.Query().Get("type")
	zone := r.URL.Query().Get("zone")

	if name != "" {
		record, err := s.service.GetRecord(
		    name,
    		    recordType,
    		    r.URL.Query().Get("zone"),
		)
		if err != nil {
			http.Error(w, err.Error(), 404)
			return
		}

		writeDNSJSON(w, 200, record)
		return
	}

	records, err := s.service.ListRecords(zone)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	writeDNSJSON(w, 200, records)
}

func (s *DNSHTTPServer) deleteRecord(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	recordType := r.URL.Query().Get("type")

	if name == "" || recordType == "" {
		http.Error(w, "name and type are required", 400)
		return
	}

	log.Printf("[DNS] Delete record request: %s type=%s",
		name, recordType)

	if err := s.service.DeleteRecord(
    	name,
    	recordType,
    	r.URL.Query().Get("zone"),
	); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	log.Printf("[DNS] Record deleted: %s", name)

	writeDNSJSON(w, 200, map[string]string{
		"status": "deleted",
	})
}

func writeDNSJSON(w http.ResponseWriter, status int, value interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(value)
}

