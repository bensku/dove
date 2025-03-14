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
	server := zone.NewEtcdStorage(client, "testZones/")

	// Empty zone, lastUpdated
	time, err := server.LastUpdated(ctx, "test")
	if err != nil {
		t.Fatal(err)
	}
	if time.Unix() != 0 {
		t.Fatal("nothing saved yet, lastUpdated should be Unix epoch")
	}

	// // Empty zone load
	testZone, err := server.Load(ctx, "test")
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
	err = server.Patch(ctx, "test", apex)
	if err != nil {
		t.Fatal(err)
	}
	testZone, err = server.Load(ctx, "test")
	if err != nil {
		t.Fatal(err)
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
	err = server.Patch(ctx, "test", record2)
	if err != nil {
		t.Fatal(err)
	}
	testZone, err = server.Load(ctx, "test")
	if err != nil {
		t.Fatal(err)
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
	err = server.Delete(ctx, "test", "record")
	if err != nil {
		t.Fatal(err)
	}
	testZone, err = server.Load(ctx, "test")
	if err != nil {
		t.Fatal(err)
	}
	if len(testZone.Records) != 0 {
		t.Fatal("record was not deleted", testZone.Records)
	}

	// Clearing (deleting all) records AND lastUpdated
	err = server.Clear(ctx, "test")
	if err != nil {
		t.Fatal(err)
	}
	testZone, err = server.Load(ctx, "test")
	if err != nil {
		t.Fatal(err)
	}
	if len(testZone.Records) != 0 {
		t.Fatal("records were not cleared", testZone.Records)
	}

	time, err = server.LastUpdated(ctx, "test")
	if err != nil {
		t.Fatal(err)
	}
	if time.Unix() != 0 {
		t.Fatal("lastUpdated was not cleared")
	}
}
