package zone_test

import (
	"context"
	"testing"

	"github.com/bensku/dove/zone"
	"github.com/miekg/dns"
)

func TestFileStorage(t *testing.T) {
	// FileStorage does not support all features that EtcdStorage does
	// So we'll test only what it DOES support: loading, inserting and clearing
	storage, err := zone.NewFileStorage("/tmp/dove-test")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	err = storage.Clear(ctx, "test")
	if err != nil {
		t.Fatal(err)
	}

	// Add some records
	rr1, _ := dns.NewRR("@ A 127.0.0.1")
	apex := zone.DnsRecord{
		Id:     "record",
		Record: rr1,
	}
	err = storage.Patch(ctx, "test", apex)
	if err != nil {
		t.Fatal(err)
	}

	rr2, _ := dns.NewRR("www A 127.0.0.1")
	www := zone.DnsRecord{
		Id:     "www",
		Record: rr2,
	}
	err = storage.Patch(ctx, "test", www)
	if err != nil {
		t.Fatal(err)
	}

	// Load to see if it worked
	testZone, err := storage.Load(ctx, "test")
	if err != nil {
		t.Fatal(err)
	}
	if len(testZone.Records) != 2 {
		t.Fatal("record count should be 2, is", len(testZone.Records))
	}
	if testZone.Records[0].Id != apex.Id {
		t.Fatal("wrong record id")
	}
	if testZone.Records[0].Record.String() != apex.Record.String() {
		t.Fatal("wrong record")
	}
	if testZone.Records[1].Id != www.Id {
		t.Fatal("wrong record id")
	}
	if testZone.Records[1].Record.String() != www.Record.String() {
		t.Fatal("wrong record")
	}

	storage.Clear(ctx, "test")
}
