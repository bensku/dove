package nameserver

import (
	"strings"

	"github.com/bensku/dove/zone"
	"github.com/miekg/dns"
)

type Server struct {
	zones   zone.ZoneServer
	handler *dns.Server
}

func (s *Server) Close() {
	s.handler.Shutdown()
}

func (s *Server) handleRequest(w dns.ResponseWriter, r *dns.Msg) {
	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative = true

	missingZones := false
	for _, q := range r.Question {
		zone := s.zones.FindZone(q.Name)
		if zone == nil {
			missingZones = true // So that we know to return NXDOMAIN
			continue            // No matching zone
		}

		for _, record := range zone.Records {
			recordName := record.Record.Header().Name

			// Direct match
			if recordName == q.Name {
				if q.Qtype == dns.TypeANY || record.Record.Header().Rrtype == q.Qtype {
					m.Answer = append(m.Answer, record.Record)
					continue
				}
			}

			// Wildcard match
			// Check if this is a wildcard record (starts with "*.")
			if strings.HasPrefix(recordName, "*.") {
				// Remove "*." and check if query ends with this suffix
				wildcardSuffix := recordName[2:]
				if strings.HasSuffix(q.Name, wildcardSuffix) &&
					!strings.Contains(q.Name[:len(q.Name)-len(wildcardSuffix)], ".") {
					// Create a new record with the queried name
					newRecord := dns.Copy(record.Record)
					newRecord.Header().Name = q.Name

					if q.Qtype == dns.TypeANY || record.Record.Header().Rrtype == q.Qtype {
						m.Answer = append(m.Answer, newRecord)
					}
				}
			}
		}
	}

	// If we have nothing to return AND at least one of zones is unknown to us -> NXDOMAIN
	if missingZones && len(m.Answer) == 0 {
		m.Rcode = dns.RcodeNameError
	}

	w.WriteMsg(m)
}

func New(listenAddr string) *Server {
	server := Server{
		handler: &dns.Server{Addr: listenAddr, Net: "udp"},
	}

	go server.handler.ListenAndServe()

	return &server
}
