package zone

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
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

func (storage *EtcdStorage) ListZones(ctx context.Context) ([]string, error) {
	prefix := storage.prefix + "__zones/"
	resp, err := storage.client.KV.Get(ctx, prefix, clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("failed to list zones: %v", err)
	}
	zones := make([]string, 0)
	for _, kv := range resp.Kvs {
		zones = append(zones, string(kv.Key[len(prefix):]))
	}
	return zones, nil
}

func (storage *EtcdStorage) AddZone(ctx context.Context, zoneId string) error {
	_, err := storage.client.KV.Put(ctx, storage.prefix+"__zones/"+zoneId, "true")
	if err != nil {
		return fmt.Errorf("failed to add zone: %v", err)
	}
	return nil
}

func (storage *EtcdStorage) DeleteZone(ctx context.Context, zoneId string) error {
	_, err := storage.client.KV.Delete(ctx, storage.prefix+"__zones/"+zoneId)
	if err != nil {
		return fmt.Errorf("failed to delete zone: %v", err)
	}
	return nil
}

func (storage *EtcdStorage) Load(ctx context.Context, zoneId string) (Zone, error) {
	prefix := storage.etcdPrefix(zoneId)
	resp, err := storage.client.KV.Get(ctx, prefix, clientv3.WithPrefix())
	if err != nil {
		return Zone{}, fmt.Errorf("failed to lookup zone: %v", err)
	}

	// Load entire zone from etcd as binary data
	records := make([]DnsRecord, 0)
	updatedKey := []byte(prefix + "__updatedHash")
	updatedHash := ""
	for _, kv := range resp.Kvs {
		if bytes.Equal(kv.Key, updatedKey) {
			updatedHash = string(kv.Value)
			continue
		}

		rr, _, err := dns.UnpackRR(kv.Value, 0)
		if err != nil {
			return Zone{}, fmt.Errorf("failed to unpack record: %v", err)
		}

		id := kv.Key[len(prefix):]
		records = append(records, DnsRecord{Id: string(id), Record: rr})
	}
	slog.Debug("loaded zone from etcd", "zone", zoneId, "records", records)

	return Zone{
		Name:        zoneId,
		Records:     records,
		UpdatedHash: updatedHash,
	}, nil
}

func (storage *EtcdStorage) IsCurrent(ctx context.Context, zone *Zone) (bool, error) {
	if zone == nil {
		slog.Debug("zone not up to date: not loaded!")
		return false, nil // The zone is in fact not loaded at all!
	}
	resp, err := storage.client.KV.Get(ctx, storage.etcdPrefix(zone.Name)+"__updatedHash")
	if err != nil {
		return false, fmt.Errorf("failed to lookup updatedHash: %v", err)
	}
	if len(resp.Kvs) == 0 {
		slog.Debug("zone up to date: has never had records", "zone", zone.Name)
		return true, nil
	}
	upToDate := string(resp.Kvs[0].Value) == zone.UpdatedHash
	slog.Debug("zone up-to-date check", "zone", zone.Name, "upToDate", upToDate, "loadedHash", zone.UpdatedHash, "etcdHash", string(resp.Kvs[0].Value))
	return upToDate, nil
}

func (storage *EtcdStorage) Patch(ctx context.Context, zoneId string, record DnsRecord) error {
	slog.Debug("patching record", "zone", zoneId, "id", record.Id, "record", record.Record)
	data := make([]byte, dns.Len(record.Record))
	end, err := dns.PackRR(record.Record, data, 0, nil, false)
	if err != nil {
		return fmt.Errorf("failed to pack DNS record: %v", err)
	}

	updatedHash := uuid.New().String()
	prefix := storage.etcdPrefix(zoneId)
	txn := storage.client.KV.Txn(ctx).Then(
		clientv3.OpPut(prefix+record.Id, string(data[:end])),
		clientv3.OpPut(prefix+"__updatedHash", updatedHash),
	)
	_, err = txn.Commit()
	if err != nil {
		return fmt.Errorf("failed to patch record: %v", err)
	}
	return nil
}

func (storage *EtcdStorage) Delete(ctx context.Context, zoneId string, id string) error {
	slog.Debug("deleting record", "zone", zoneId, "id", id)

	// Remember to also mark zone as updated
	updatedHash := uuid.New().String()
	prefix := storage.etcdPrefix(zoneId)
	txn := storage.client.KV.Txn(ctx).Then(
		clientv3.OpDelete(prefix+id),
		clientv3.OpPut(prefix+"__updatedHash", updatedHash),
	)
	_, err := txn.Commit()
	if err != nil {
		return fmt.Errorf("failed to delete record: %v", err)
	}
	return nil
}

func (storage *EtcdStorage) Clear(ctx context.Context, zoneId string) error {
	slog.Debug("clearing zone", "zone", zoneId)
	_, err := storage.client.KV.Delete(ctx, storage.prefix+zoneId, clientv3.WithPrefix())
	if err != nil {
		return fmt.Errorf("etcd delete failed: %v", err)
	}
	return nil
}

var _ ZoneStorage = (*EtcdStorage)(nil)
