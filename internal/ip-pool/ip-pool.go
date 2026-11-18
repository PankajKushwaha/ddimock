package ippool

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	StateAvailable  = "AVAILABLE"
	StateAllocated  = "ALLOCATED"
	StateReserved   = "RESERVED"
	StateQuarantine = "QUARANTINE"

	OwnerIPAM = "ipam"
	OwnerDHCP = "dhcp"
)

type IPRecord struct {
	IP          string
	Network     string
	State       string
	Hostname    string
	MAC         string
	Owner       string
	LastUpdated string
}

type IPPool struct {
	filePath string
}

// The mutex coordinates goroutines across all IPPool instances
// in this process. Flock coordinates separate processes.
var processMutex sync.Mutex

func NewIPPool(filePath string) (*IPPool, error) {
	if strings.TrimSpace(filePath) == "" {
		return nil, fmt.Errorf("inventory file path cannot be empty")
	}

	pool := &IPPool{
		filePath: filePath,
	}

	if _, err := os.Stat(filePath); err != nil {
		return nil, fmt.Errorf("cannot access inventory file: %v", err)
	}

	return pool, nil
}

// ListIPs returns all IP records.
func (p *IPPool) ListIPs() ([]IPRecord, error) {
	var records []IPRecord

	err := p.withLock(syscall.LOCK_SH, func() error {
		var err error
		records, err = p.readInventory()
		return err
	})

	return records, err
}

// GetIP returns the record for a specific IP.
func (p *IPPool) GetIP(ip string) (IPRecord, error) {
	var result IPRecord

	err := p.withLock(syscall.LOCK_SH, func() error {
		records, err := p.readInventory()
		if err != nil {
			return err
		}

		index := findIP(records, ip)
		if index == -1 {
			return fmt.Errorf("IP %s not found", ip)
		}

		result = records[index]
		return nil
	})

	return result, err
}

// AllocateIP allocates a specific available IP.
// owner must be "ipam" or "dhcp".
func (p *IPPool) AllocateIP(
	ip string,
	hostname string,
	mac string,
	owner string,
) error {
	owner = strings.ToLower(strings.TrimSpace(owner))

	if owner != OwnerIPAM && owner != OwnerDHCP {
		return fmt.Errorf("owner must be ipam or dhcp")
	}

	if strings.TrimSpace(hostname) == "" {
		return fmt.Errorf("hostname cannot be empty")
	}

	if err := validateMAC(mac); err != nil {
		return err
	}

	return p.updateInventory(func(records []IPRecord) ([]IPRecord, error) {
		index := findIP(records, ip)
		if index == -1 {
			return nil, fmt.Errorf("IP %s not found", ip)
		}

		if records[index].State != StateAvailable {
			return nil, fmt.Errorf(
				"IP %s is %s; only AVAILABLE IPs can be allocated",
				ip,
				records[index].State,
			)
		}

		records[index].State = StateAllocated
		records[index].Hostname = hostname
		records[index].MAC = mac
		records[index].Owner = owner
		records[index].LastUpdated = currentTimestamp()

		return records, nil
	})
}

// AllocateNextIP allocates the first available IP in a network.
// An empty network argument searches all networks.
func (p *IPPool) AllocateNextIP(
	network string,
	hostname string,
	mac string,
	owner string,
) (IPRecord, error) {
	var allocated IPRecord

	owner = strings.ToLower(strings.TrimSpace(owner))

	if owner != OwnerIPAM && owner != OwnerDHCP {
		return allocated, fmt.Errorf("owner must be ipam or dhcp")
	}

	if strings.TrimSpace(hostname) == "" {
		return allocated, fmt.Errorf("hostname cannot be empty")
	}

	if err := validateMAC(mac); err != nil {
		return allocated, err
	}

	err := p.updateInventory(func(records []IPRecord) ([]IPRecord, error) {
		for i := range records {
			if records[i].State != StateAvailable {
				continue
			}

			if network != "" && records[i].Network != network {
				continue
			}

			records[i].State = StateAllocated
			records[i].Hostname = hostname
			records[i].MAC = mac
			records[i].Owner = owner
			records[i].LastUpdated = currentTimestamp()

			allocated = records[i]
			return records, nil
		}

		if network == "" {
			return nil, fmt.Errorf("no available IP addresses")
		}

		return nil, fmt.Errorf("no available IP addresses in network %s", network)
	})

	return allocated, err
}

// ReleaseIP releases an allocated IP.
// The caller must own the allocation.
func (p *IPPool) ReleaseIP(ip string, owner string) error {
	owner = strings.ToLower(strings.TrimSpace(owner))

	return p.updateInventory(func(records []IPRecord) ([]IPRecord, error) {
		index := findIP(records, ip)
		if index == -1 {
			return nil, fmt.Errorf("IP %s not found", ip)
		}

		record := &records[index]

		if record.State != StateAllocated {
			return nil, fmt.Errorf(
				"IP %s is %s; only ALLOCATED IPs can be released",
				ip,
				record.State,
			)
		}

		if record.Owner != owner {
			return nil, fmt.Errorf(
				"IP %s is owned by %s, not %s",
				ip,
				record.Owner,
				owner,
			)
		}

		clearRecord(record)
		record.State = StateAvailable
		record.LastUpdated = currentTimestamp()

		return records, nil
	})
}

// ReserveIP reserves an available IP for DHCP.
func (p *IPPool) ReserveIP(
	ip string,
	hostname string,
	mac string,
) error {
	if strings.TrimSpace(hostname) == "" {
		return fmt.Errorf("hostname cannot be empty")
	}

	if err := validateMAC(mac); err != nil {
		return err
	}

	return p.updateInventory(func(records []IPRecord) ([]IPRecord, error) {
		index := findIP(records, ip)
		if index == -1 {
			return nil, fmt.Errorf("IP %s not found", ip)
		}

		if records[index].State != StateAvailable {
			return nil, fmt.Errorf(
				"IP %s is %s; only AVAILABLE IPs can be reserved",
				ip,
				records[index].State,
			)
		}

		records[index].State = StateReserved
		records[index].Hostname = hostname
		records[index].MAC = mac
		records[index].Owner = OwnerDHCP
		records[index].LastUpdated = currentTimestamp()

		return records, nil
	})
}

// ReleaseReservation removes a DHCP reservation.
func (p *IPPool) ReleaseReservation(ip string) error {
	return p.updateInventory(func(records []IPRecord) ([]IPRecord, error) {
		index := findIP(records, ip)
		if index == -1 {
			return nil, fmt.Errorf("IP %s not found", ip)
		}

		record := &records[index]

		if record.State != StateReserved {
			return nil, fmt.Errorf("IP %s is not RESERVED", ip)
		}

		if record.Owner != OwnerDHCP {
			return nil, fmt.Errorf("IP %s is not owned by DHCP", ip)
		}

		clearRecord(record)
		record.State = StateAvailable
		record.LastUpdated = currentTimestamp()

		return records, nil
	})
}

// QuarantineIP moves an available IP into quarantine.
func (p *IPPool) QuarantineIP(ip string) error {
	return p.updateInventory(func(records []IPRecord) ([]IPRecord, error) {
		index := findIP(records, ip)
		if index == -1 {
			return nil, fmt.Errorf("IP %s not found", ip)
		}

		if records[index].State != StateAvailable {
			return nil, fmt.Errorf(
				"only AVAILABLE IPs can be quarantined; current state: %s",
				records[index].State,
			)
		}

		records[index].State = StateQuarantine
		records[index].LastUpdated = currentTimestamp()

		return records, nil
	})
}

// RestoreIP moves a quarantined IP back to the available pool.
func (p *IPPool) RestoreIP(ip string) error {
	return p.updateInventory(func(records []IPRecord) ([]IPRecord, error) {
		index := findIP(records, ip)
		if index == -1 {
			return nil, fmt.Errorf("IP %s not found", ip)
		}

		if records[index].State != StateQuarantine {
			return nil, fmt.Errorf("IP %s is not QUARANTINED", ip)
		}

		records[index].State = StateAvailable
		records[index].LastUpdated = currentTimestamp()

		return records, nil
	})
}

// AddIP adds a new IP address to the inventory.
func (p *IPPool) AddIP(ip string, network string) error {
	if net.ParseIP(ip) == nil {
		return fmt.Errorf("invalid IP address: %s", ip)
	}

	_, parsedNetwork, err := net.ParseCIDR(network)
	if err != nil {
		return fmt.Errorf("invalid network: %s", network)
	}

	if !parsedNetwork.Contains(net.ParseIP(ip)) {
		return fmt.Errorf("IP %s does not belong to network %s", ip, network)
	}

	return p.updateInventory(func(records []IPRecord) ([]IPRecord, error) {
		if findIP(records, ip) != -1 {
			return nil, fmt.Errorf("IP %s already exists", ip)
		}

		records = append(records, IPRecord{
			IP:          ip,
			Network:     network,
			State:       StateAvailable,
			LastUpdated: currentTimestamp(),
		})

		return records, nil
	})
}

// DeleteIP only deletes an AVAILABLE IP.
// Allocated IPs must first be released.
// DHCP reservations must first be removed.
func (p *IPPool) DeleteIP(ip string) error {
	return p.updateInventory(func(records []IPRecord) ([]IPRecord, error) {
		index := findIP(records, ip)
		if index == -1 {
			return nil, fmt.Errorf("IP %s not found", ip)
		}

		if records[index].State != StateAvailable {
			return nil, fmt.Errorf(
				"cannot delete IP %s in state %s; release or restore it first",
				ip,
				records[index].State,
			)
		}

		return append(records[:index], records[index+1:]...), nil
	})
}

// updateInventory performs a complete read-modify-write transaction
// while holding an exclusive process and file lock.
func (p *IPPool) updateInventory(
	update func([]IPRecord) ([]IPRecord, error),
) error {
	return p.withLock(syscall.LOCK_EX, func() error {
		records, err := p.readInventory()
		if err != nil {
			return err
		}

		updated, err := update(records)
		if err != nil {
			return err
		}

		return p.writeInventory(updated)
	})
}

// withLock coordinates concurrent goroutines and processes.
func (p *IPPool) withLock(lockType int, operation func() error) error {
	processMutex.Lock()
	defer processMutex.Unlock()

	lockPath := p.filePath + ".lock"

	lockFile, err := os.OpenFile(
		lockPath,
		os.O_CREATE|os.O_RDWR,
		0600,
	)
	if err != nil {
		return err
	}
	defer lockFile.Close()

	if err := syscall.Flock(int(lockFile.Fd()), lockType); err != nil {
		return err
	}

	defer syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)

	return operation()
}

// readInventory reads the seven-column CSV inventory.
func (p *IPPool) readInventory() ([]IPRecord, error) {
	file, err := os.Open(p.filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.Comment = '#'
	reader.FieldsPerRecord = 7
	reader.TrimLeadingSpace = true

	var records []IPRecord
	seen := make(map[string]bool)

	for {
		row, err := reader.Read()

		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading inventory CSV: %v", err)
		}

		record := IPRecord{
			IP:          row[0],
			Network:     row[1],
			State:       strings.ToUpper(strings.TrimSpace(row[2])),
			Hostname:    row[3],
			MAC:         row[4],
			Owner:       strings.ToLower(strings.TrimSpace(row[5])),
			LastUpdated: row[6],
		}

		if err := validateRecord(record); err != nil {
			return nil, fmt.Errorf("invalid inventory record: %v", err)
		}

		if seen[record.IP] {
			return nil, fmt.Errorf("duplicate IP in inventory: %s", record.IP)
		}
		seen[record.IP] = true

		records = append(records, record)
	}

	return records, nil
}

// writeInventory writes the complete inventory using atomic replacement.
func (p *IPPool) writeInventory(records []IPRecord) error {
	var buffer bytes.Buffer

	buffer.WriteString("# ip,network,state,hostname,mac,owner,last_updated\n")

	writer := csv.NewWriter(&buffer)

	for _, record := range records {
		if err := validateRecord(record); err != nil {
			return err
		}

		row := []string{
			record.IP,
			record.Network,
			record.State,
			record.Hostname,
			record.MAC,
			record.Owner,
			record.LastUpdated,
		}

		if err := writer.Write(row); err != nil {
			return err
		}
	}

	writer.Flush()

	if err := writer.Error(); err != nil {
		return err
	}

	return p.atomicWrite(buffer.Bytes())
}

// atomicWrite writes to a temporary file, syncs it, and renames it.
func (p *IPPool) atomicWrite(data []byte) error {
	dir := filepath.Dir(p.filePath)

	info, err := os.Stat(p.filePath)
	if err != nil {
		return err
	}

	tempFile, err := ioutil.TempFile(dir, ".ip-pool-*.tmp")
	if err != nil {
		return err
	}

	tempPath := tempFile.Name()
	defer os.Remove(tempPath)

	if err := tempFile.Chmod(info.Mode().Perm()); err != nil {
		tempFile.Close()
		return err
	}

	if _, err := tempFile.Write(data); err != nil {
		tempFile.Close()
		return err
	}

	if err := tempFile.Sync(); err != nil {
		tempFile.Close()
		return err
	}

	if err := tempFile.Close(); err != nil {
		return err
	}

	if err := os.Rename(tempPath, p.filePath); err != nil {
		return err
	}

	dirFile, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer dirFile.Close()

	return dirFile.Sync()
}

func validateRecord(record IPRecord) error {
	parsedIP := net.ParseIP(record.IP)
	if parsedIP == nil {
		return fmt.Errorf("invalid IP address: %s", record.IP)
	}

	_, network, err := net.ParseCIDR(record.Network)
	if err != nil {
		return fmt.Errorf("invalid network: %s", record.Network)
	}

	if !network.Contains(parsedIP) {
		return fmt.Errorf(
			"IP %s does not belong to network %s",
			record.IP,
			record.Network,
		)
	}

	switch record.State {
	case StateAvailable, StateAllocated, StateReserved, StateQuarantine:
	default:
		return fmt.Errorf("invalid IP state: %s", record.State)
	}

	if record.LastUpdated != "" {
		if _, err := time.Parse(time.RFC3339, record.LastUpdated); err != nil {
			return fmt.Errorf("invalid timestamp: %s", record.LastUpdated)
		}
	}

	return nil
}

func validateMAC(mac string) error {
	hw, err := net.ParseMAC(mac)
	if err != nil {
		return fmt.Errorf("invalid MAC address: %s", mac)
	}

	if len(hw) != 6 {
		return fmt.Errorf("expected a six-byte MAC address: %s", mac)
	}

	return nil
}

func findIP(records []IPRecord, ip string) int {
	for i := range records {
		if records[i].IP == ip {
			return i
		}
	}
	return -1
}

func clearRecord(record *IPRecord) {
	record.Hostname = ""
	record.MAC = ""
	record.Owner = ""
}

func currentTimestamp() string {
	return time.Now().UTC().Format(time.RFC3339)
}
