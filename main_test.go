package main_test

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/miekg/dns"
)

func request(method, url string, payload []byte) string {
	var req *http.Request
	var err error
	if payload != nil {
		req, err = http.NewRequest(method, url, bytes.NewReader(payload))
	} else {
		req, err = http.NewRequest(method, url, nil)
	}
	if err != nil {
		panic(err)
	}
	req.Header.Set("Authorization", "test-api-key")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		panic(err)
	}
	if res.StatusCode >= 400 {
		panic(fmt.Errorf("request failed with status %d", res.StatusCode))
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		panic(err)
	}
	return string(body)
}

func queryRecords(domain string, recordType uint16) []dns.RR {
	server := "127.0.0.1:5300"
	c := new(dns.Client)
	c.Timeout = 5 * time.Second

	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(domain), recordType)
	m.RecursionDesired = true

	r, _, err := c.Exchange(m, server)
	if err != nil {
		panic(err)
	}

	if r.Rcode != dns.RcodeSuccess {
		panic(fmt.Errorf("DNS query failed with code: %s", dns.RcodeToString[r.Rcode]))
	}

	return r.Answer
}

func TestAdminApi(t *testing.T) {
	zones := request("GET", "http://localhost:8080/api/v1/zone", nil)
	if zones != "[]" {
		// t.Errorf("expected no zones, got %s", zones)
	}

	// Create zone
	request("PUT", "http://localhost:8080/api/v1/zone/dove.test.", nil)
	zones = request("GET", "http://localhost:8080/api/v1/zone", nil)
	if zones != `["dove.test."]` {
		t.Errorf("expected dove.test., got %s", zones)
	}
	time.Sleep(7 * time.Second) // FIXME this ugly hack, but DNS needs time to propagate

	// Check that the zone doesn't yet have any records
	rr := queryRecords("dove.test.", dns.TypeA)
	if !slices.Equal(rr, []dns.RR{}) {
		t.Errorf("expected no records yet, got %s", rr)
	}

	// Add some records
	request("PUT", "http://localhost:8080/api/v1/zone/dove.test./testid", []byte("@ 300 IN A 1.2.3.4"))
	time.Sleep(7 * time.Second) // FIXME this ugly hack, but DNS needs time to propagate
	rr = queryRecords("dove.test.", dns.TypeA)
	correct := []dns.RR{
		&dns.A{
			Hdr: dns.RR_Header{
				Name:   "dove.test.",
				Rrtype: dns.TypeA,
				Class:  dns.ClassINET,
				Ttl:    300,
			},
			A: net.ParseIP("1.2.3.4").To4(),
		},
	}
	if fmt.Sprint(rr) != fmt.Sprint(correct) {
		t.Errorf("expected %s records, got %s", correct, rr)
	}

	// Delete the zone
	request("DELETE", "http://localhost:8080/api/v1/zone/dove.test.", nil)
	zones = request("GET", "http://localhost:8080/api/v1/zone", nil)
	if zones != "[]" {
		t.Errorf("zone was not properly deleted: %s", zones)
	}
}
