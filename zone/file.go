package zone

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/miekg/dns"
)

type FileStorage struct {
	Path string
}

func NewFileStorage(path string) (*FileStorage, error) {
	err := os.MkdirAll(path, 0o744)
	if err != nil {
		return nil, fmt.Errorf("failed to create zone data directory: %v", err)
	}
	return &FileStorage{Path: path}, nil
}

func readVarString(data []byte, offset int) (string, int) {
	length := int(data[offset])
	offset++
	return string(data[offset : offset+length]), offset + length
}

func writeVarString(data []byte, offset int, str string) int {
	data[offset] = byte(len(str))
	offset++
	copy(data[offset:], str)
	return offset + len(str)
}

func (storage *FileStorage) Load(ctx context.Context, zoneId string) (Zone, error) {
	data, err := os.ReadFile(storage.Path + "/" + zoneId)
	if err != nil {
		return Zone{}, fmt.Errorf("failed to read zone data file: %v", err)
	}

	// Read records - not delimiters needed, UnpackRR will tell us new offset
	offset := 0
	records := make([]DnsRecord, 0)
	for {
		id, end := readVarString(data, offset)
		rr, end, err := dns.UnpackRR(data, end)
		if err != nil {
			return Zone{}, fmt.Errorf("failed to unpack DNS record: %v", err)
		}
		offset = end
		records = append(records, DnsRecord{Id: id, Record: rr})
		if end == len(data) {
			break // Got to end of zone file
		}
	}

	return Zone{
		Id:      zoneId,
		Records: records,
	}, nil
}

func (storage *FileStorage) LastUpdated(ctx context.Context, zoneId string) (time.Time, error) {
	// Never try to update in-memory zone data from here
	// File storage is meant to serve as backup against etcd failures
	return time.Unix(0, 0), nil
}

func (storage *FileStorage) Patch(ctx context.Context, zoneId string, record DnsRecord) error {
	data := make([]byte, 1+len(record.Id)+dns.Len(record.Record))
	offset := writeVarString(data, 0, record.Id)
	_, err := dns.PackRR(record.Record, data, offset, nil, false)
	if err != nil {
		return fmt.Errorf("failed to pack DNS record: %v", err)
	}

	file, err := os.OpenFile(storage.Path+"/"+zoneId, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return fmt.Errorf("failed to open zone file: %v", err)
	}
	defer file.Close()

	_, err = file.Write(data)
	if err != nil {
		return fmt.Errorf("failed to append record to zone file: %v", err)
	}
	return nil
}

func (storage *FileStorage) Delete(ctx context.Context, zoneId string, name string) error {
	return fmt.Errorf("not implemented")
}

func (storage *FileStorage) Clear(ctx context.Context, zoneId string) error {
	_, err := os.Stat(storage.Path + "/" + zoneId)
	if err != nil {
		return nil // Assume that it just didn't exist
	}
	err = os.Truncate(storage.Path+"/"+zoneId, 0)
	if err != nil {
		return fmt.Errorf("failed to truncate zone file: %v", err)
	}
	return nil
}

var _ ZoneStorage = (*FileStorage)(nil)
