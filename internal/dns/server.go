
package dns

import (
	"encoding/csv"
	"fmt"
	"os"
	"strings"
	"sync"
)

type DNSRecord struct {
	Name  string
	Type  string
	Value string
	TTL   string
	Zone  string
}

type DNSService struct {
	filePath string
	mutex    sync.Mutex
}

func NewDNSService(filePath string) *DNSService {
	return &DNSService{
		filePath: filePath,
	}
}

// CreateZone creates a DNS zone if it does not already exist.
func (s *DNSService) CreateZone(zone string) error {
	if strings.TrimSpace(zone) == "" {
		return fmt.Errorf("zone cannot be empty")
	}

	records, err := s.loadRecords()
	if err != nil {
		return err
	}

	for _, record := range records {
		if record.Zone == zone {
			return nil
		}
	}

	return nil
}

// AddRecord adds a DNS record.
func (s *DNSService) AddRecord(record DNSRecord) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	record.Name = strings.TrimSpace(record.Name)
	record.Zone = strings.TrimSpace(record.Zone)
	record.Type = strings.ToUpper(strings.TrimSpace(record.Type))

	if record.Name == "" || record.Zone == "" || record.Value == "" {
		return fmt.Errorf("name, zone and value are required")
	}

	switch record.Type {
	case "A", "AAAA", "CNAME":
	default:
		return fmt.Errorf("unsupported DNS record type: %s", record.Type)
	}

	records, err := s.loadRecords()
	if err != nil {
		return err
	}

	for _, existing := range records {
		if existing.Name == record.Name &&
			existing.Type == record.Type &&
			existing.Zone == record.Zone {
			return fmt.Errorf("DNS record already exists")
		}
	}

	records = append(records, record)
	return s.saveRecords(records)
}

// GetRecord retrieves a DNS record.
func (s *DNSService) GetRecord(
	name string,
	recordType string,
	zone string,
) (DNSRecord, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	records, err := s.loadRecords()
	if err != nil {
		return DNSRecord{}, err
	}

	for _, record := range records {
		if record.Name == name &&
			record.Type == strings.ToUpper(recordType) &&
			record.Zone == zone {
			return record, nil
		}
	}

	return DNSRecord{}, fmt.Errorf("DNS record not found")
}

// ListRecords lists records belonging to a zone.
func (s *DNSService) ListRecords(zone string) ([]DNSRecord, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	records, err := s.loadRecords()
	if err != nil {
		return nil, err
	}

	var result []DNSRecord

	for _, record := range records {
		if record.Zone == zone {
			result = append(result, record)
		}
	}

	return result, nil
}

// DeleteRecord deletes a DNS record.
func (s *DNSService) DeleteRecord(
	name string,
	recordType string,
	zone string,
) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	records, err := s.loadRecords()
	if err != nil {
		return err
	}

	for i, record := range records {
		if record.Name == name &&
			record.Type == strings.ToUpper(recordType) &&
			record.Zone == zone {
			records = append(records[:i], records[i+1:]...)
			return s.saveRecords(records)
		}
	}

	return fmt.Errorf("DNS record not found")
}

func (s *DNSService) loadRecords() ([]DNSRecord, error) {
	file, err := os.Open(s.filePath)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	rows, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}

	var records []DNSRecord

	for _, row := range rows {
		if len(row) != 5 {
			return nil, fmt.Errorf("invalid DNS record in CSV")
		}

		records = append(records, DNSRecord{
			Name:  row[0],
			Type:  row[1],
			Value: row[2],
			TTL:   row[3],
			Zone:  row[4],
		})
	}

	return records, nil
}

func (s *DNSService) saveRecords(records []DNSRecord) error {
	if err := os.MkdirAll("data", 0755); err != nil {
		return err
	}

	file, err := os.Create(s.filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)

	for _, record := range records {
		row := []string{
			record.Name,
			record.Type,
			record.Value,
			record.TTL,
			record.Zone,
		}

		if err := writer.Write(row); err != nil {
			return err
		}
	}

	writer.Flush()
	return writer.Error()
}

