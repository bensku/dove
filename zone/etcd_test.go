package zone_test

import (
	"context"
	"testing"

	"github.com/bensku/dove/zone"
	"github.com/miekg/dns"
	clientv3 "go.etcd.io/etcd/client/v3"
)

func TestEtcdStorage(t *testing.T) {
	client, err := clientv3.New(clientv3.Config{
		Endpoints: []string{"http://localhost:2379", "http://localhost:22379", "http://localhost:32379"},
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	storage := zone.NewEtcdStorage(client, "testZones/")

	// // Empty zone load
	testZone, err := storage.Load(ctx, "test")
	if err != nil {
		t.Fatal(err)
	}
	if len(testZone.Records) != 0 {
		t.Fatal("nothing saved yet, records should be empty")
	}

	// Adding records!
	rr1, _ := dns.NewRR("@ A 127.0.0.1")
	apex := zone.DnsRecord{
		Id:     "record",
		Record: rr1,
	}
	err = storage.Patch(ctx, "test", apex)
	if err != nil {
		t.Fatal(err)
	}
	testZone, err = storage.Load(ctx, "test")
	if err != nil {
		t.Fatal(err)
	}
	current, err := storage.IsCurrent(ctx, &testZone)
	if err != nil {
		t.Fatal(err)
	}
	if !current {
		t.Fatal("zone should be current")
	}
	if len(testZone.Records) != 1 {
		t.Fatal("wrong amount of records", testZone.Records)
	}
	if testZone.Records[0].Id != apex.Id {
		t.Fatal("record id corrupted", testZone.Records[0].Id, apex.Id)
	}
	if testZone.Records[0].Record.String() != apex.Record.String() {
		t.Fatal("actual record corrupted", testZone.Records[0].Record.String(), apex.Record.String())
	}

	// Overwriting (patching) existing records
	rr2, _ := dns.NewRR("foo A 127.0.0.2")
	record2 := zone.DnsRecord{
		Id:     "record",
		Record: rr2,
	}
	err = storage.Patch(ctx, "test", record2)
	if err != nil {
		t.Fatal(err)
	}

	current, err = storage.IsCurrent(ctx, &testZone)
	if err != nil {
		t.Fatal(err)
	}
	if current {
		t.Fatal("zone should be outdated")
	}

	testZone, err = storage.Load(ctx, "test")
	if err != nil {
		t.Fatal(err)
	}
	current, err = storage.IsCurrent(ctx, &testZone)
	if err != nil {
		t.Fatal(err)
	}
	if !current {
		t.Fatal("zone should be current")
	}

	if len(testZone.Records) != 1 {
		t.Fatal("added new record, should've overwritten", testZone.Records)
	}
	if testZone.Records[0].Id != record2.Id {
		t.Fatal("record id corrupted", testZone.Records[0].Id, apex.Id)
	}
	if testZone.Records[0].Record.String() != record2.Record.String() {
		t.Fatal("actual record corrupted", testZone.Records[0].Record.String(), apex.Record.String())
	}

	// Deleting records
	err = storage.Delete(ctx, "test", "record")
	if err != nil {
		t.Fatal(err)
	}
	testZone, err = storage.Load(ctx, "test")
	if err != nil {
		t.Fatal(err)
	}
	if len(testZone.Records) != 0 {
		t.Fatal("record was not deleted", testZone.Records)
	}

	// Clearing (deleting all) records AND lastUpdated
	err = storage.Clear(ctx, "test")
	if err != nil {
		t.Fatal(err)
	}
	testZone, err = storage.Load(ctx, "test")
	if err != nil {
		t.Fatal(err)
	}
	if len(testZone.Records) != 0 {
		t.Fatal("records were not cleared", testZone.Records)
	}
}

func TestEtcdZoneList(t *testing.T) {
	client, err := clientv3.New(clientv3.Config{
		Endpoints: []string{"http://localhost:2379", "http://localhost:22379", "http://localhost:32379"},
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	storage := zone.NewEtcdStorage(client, "testZones/")

	zoneIds, err := storage.ListZones(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(zoneIds) != 0 {
		t.Fatal("no zones expected")
	}

	// Adding zones!
	err = storage.AddZone(ctx, "test.")
	if err != nil {
		t.Fatal(err)
	}
	zoneIds, err = storage.ListZones(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(zoneIds) != 1 {
		t.Fatal("wrong amount of zones", zoneIds)
	}
	if zoneIds[0] != "test." {
		t.Fatal("zone id corrupted", zoneIds[0])
	}

	// Deleting zones!
	err = storage.DeleteZone(ctx, "test.")
	if err != nil {
		t.Fatal(err)
	}
	zoneIds, err = storage.ListZones(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(zoneIds) != 0 {
		t.Fatal("zone was not deleted", zoneIds)
	}
}
