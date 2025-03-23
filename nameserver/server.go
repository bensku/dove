package nameserver

import (
	"context"
	"strings"

	"github.com/bensku/dove/zone"
	"github.com/miekg/dns"
)

type Server struct {
	zones zone.ZoneServer
	dns   *dns.Server
}

func handleRequest(zone *zone.Zone, w dns.ResponseWriter, r *dns.Msg) {
	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative = true

	for _, q := range r.Question {
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

	w.WriteMsg(m)
}

func New(ctx context.Context, listenAddr string, primary zone.ZoneStorage, fallback zone.ZoneStorage) *Server {
	handler := dns.NewServeMux()

	onZoneUpdated := func(name string, zone *zone.Zone) {
		if zone == nil {
			// Previously existing zone was removed, clear handler
			handler.HandleRemove(name)
		} else {
			// New zone was loaded or existing zone was updated (=replaced)
			handler.HandleRemove(name) // Remove old handler (no-op if it doesn't exist)
			handler.HandleFunc(name, func(w dns.ResponseWriter, m *dns.Msg) {
				handleRequest(zone, w, m)
			})
		}
	}

	server := Server{
		zones: *zone.NewZoneServer(ctx, primary, fallback, onZoneUpdated),
		dns:   &dns.Server{Addr: listenAddr, Net: "udp", Handler: handler},
	}

	// Shutdown the DNS server when context is done
	go func() {
		<-ctx.Done()
		server.dns.Shutdown()
	}()

	go server.dns.ListenAndServe()

	return &server
}
