package nameserver

import (
	"context"
	"log/slog"
	"strings"
	"time"

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
		name := strings.TrimSuffix(q.Name, zone.Name)
		if name == "" {
			name = "."
		}
		slog.Debug("incoming query", "query", name, "type", dns.TypeToString[q.Qtype])
		// IMPORTANT! Order of records we get from storage may be random!
		exactResults := false
		for _, record := range zone.Records {
			slog.Debug("matching record", "name", record.Record.Header().Name, "type", dns.TypeToString[record.Record.Header().Rrtype])
			recordName := record.Record.Header().Name

			// Direct match
			if recordName == name {
				if q.Qtype == dns.TypeANY || record.Record.Header().Rrtype == q.Qtype {
					// Create a new record with the queried name
					newRecord := dns.Copy(record.Record)
					newRecord.Header().Name = q.Name
					m.Answer = append(m.Answer, newRecord)
					exactResults = true
					continue
				}
			}
		}
		if exactResults {
			continue // Skip wildcard matching
		}

		// If no results, try wildcard matching
		for _, record := range zone.Records {
			recordName := record.Record.Header().Name
			if recordName[0] == '*' {
				var wildcardSuffix string
				if recordName[1] == '.' {
					wildcardSuffix = recordName[2:]
				} else {
					wildcardSuffix = recordName[1:]
				}

				if strings.HasSuffix(name, wildcardSuffix) {
					if q.Qtype == dns.TypeANY || record.Record.Header().Rrtype == q.Qtype {
						// Create a new record with the queried name
						newRecord := dns.Copy(record.Record)
						newRecord.Header().Name = q.Name
						m.Answer = append(m.Answer, newRecord)
						break // Do not allow many wildcards!
					}
				}
			}
		}
	}

	w.WriteMsg(m)
}

func New(ctx context.Context, listenAddr string, primary zone.ZoneStorage, fallback zone.ZoneStorage, refreshInterval time.Duration) *Server {
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
		zones: *zone.NewZoneServer(ctx, primary, fallback, onZoneUpdated, refreshInterval),
		dns:   &dns.Server{Addr: listenAddr, Net: "udp", Handler: handler},
	}

	// Shutdown the DNS server when context is done
	go func() {
		<-ctx.Done()
		server.dns.Shutdown()
	}()

	go func() {
		err := server.dns.ListenAndServe()
		if err != nil {
			slog.Error("DNS server failed to start", "error", err)
		}
	}()

	return &server
}
