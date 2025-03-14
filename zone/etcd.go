package zone

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/miekg/dns"
	clientv3 "go.etcd.io/etcd/client/v3"
)

type EtcdStorage struct {
	client *clientv3.Client
	prefix string
}

func NewEtcdStorage(client *clientv3.Client, prefix string) *EtcdStorage {
	return &EtcdStorage{
		client: client,
		prefix: prefix,
	}
}

func (storage *EtcdStorage) etcdPrefix(zoneId string) string {
	return storage.prefix + zoneId + "/"
}

func (storage *EtcdStorage) Load(ctx context.Context, zoneId string) (Zone, error) {
	prefix := storage.etcdPrefix(zoneId)
	resp, err := storage.client.KV.Get(ctx, prefix, clientv3.WithPrefix())
	if err != nil {
		return Zone{}, fmt.Errorf("failed to lookup zone: %v", err)
	}

	// Load entire zone from etcd as binary data
	records := make([]DnsRecord, 0)
	updatedKey := []byte(prefix + "__lastUpdated")
	lastUpdated := time.Unix(0, 0)
	for _, kv := range resp.Kvs {
		if bytes.Equal(kv.Key, updatedKey) {
			lastUpdated = time.Unix(int64(binary.LittleEndian.Uint64(kv.Value)), 0)
			continue
		}

		rr, _, err := dns.UnpackRR(kv.Value, 0)
		if err != nil {
			return Zone{}, fmt.Errorf("failed to unpack record: %v", err)
		}

		id := kv.Key[len(prefix):]
		records = append(records, DnsRecord{Id: string(id), Record: rr})
	}

	return Zone{
		Id:          zoneId,
		Records:     records,
		LastUpdated: lastUpdated,
	}, nil
}

func (storage *EtcdStorage) LastUpdated(ctx context.Context, zoneId string) (time.Time, error) {
	resp, err := storage.client.KV.Get(ctx, storage.etcdPrefix(zoneId)+"__lastUpdated")
	if err != nil {
		return time.Unix(0, 0), fmt.Errorf("failed to lookup lastUpdated: %v", err)
	}
	if len(resp.Kvs) == 0 {
		return time.Unix(0, 0), nil
	}
	return time.Unix(int64(binary.LittleEndian.Uint64(resp.Kvs[0].Value)), 0), nil
}

func (storage *EtcdStorage) Patch(ctx context.Context, zoneId string, record DnsRecord) error {
	data := make([]byte, dns.Len(record.Record))
	end, err := dns.PackRR(record.Record, data, 0, nil, false)
	if err != nil {
		return fmt.Errorf("failed to pack DNS record: %v", err)
	}

	lastUpdated := make([]byte, 8)
	binary.LittleEndian.PutUint64(lastUpdated, uint64(time.Now().Unix()))
	prefix := storage.etcdPrefix(zoneId)
	txn := storage.client.KV.Txn(ctx).Then(
		clientv3.OpPut(prefix+record.Id, string(data[:end])),
		clientv3.OpPut(prefix+"__lastUpdated", string(lastUpdated)),
	)
	_, err = txn.Commit()
	if err != nil {
		return fmt.Errorf("failed to patch record: %v", err)
	}
	return nil
}

func (storage *EtcdStorage) Delete(ctx context.Context, zoneId string, id string) error {
	_, err := storage.client.KV.Delete(ctx, storage.etcdPrefix(zoneId)+id)
	if err != nil {
		return fmt.Errorf("failed to delete record: %v", err)
	}
	return nil
}

func (storage *EtcdStorage) Clear(ctx context.Context, zoneId string) error {
	_, err := storage.client.KV.Delete(ctx, storage.prefix+zoneId, clientv3.WithPrefix())
	if err != nil {
		return fmt.Errorf("etcd delete failed: %v", err)
	}
	return nil
}

var _ ZoneStorage = (*EtcdStorage)(nil)
