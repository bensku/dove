package zone

import "github.com/miekg/dns"

type DnsRecord struct {
	// Id of this record, must be unique within zone
	Id string

	// Underlying DNS record
	Record dns.RR
}
