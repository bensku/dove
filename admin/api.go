package admin

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	"github.com/bensku/dove/zone"
	"github.com/miekg/dns"
)

type acmeUpdate struct {
	Subdomain string `json:"subdomain"`
	Txt       string `json:"txt"`
}

type acmeResponse struct {
	Txt string `json:"txt"`
}

func New(ctx context.Context, addr string,
	storage zone.ZoneStorage, apiKeys []string) {
	mux := http.NewServeMux()

	// Zone listing
	mux.HandleFunc("GET /api/v1/zone", func(w http.ResponseWriter, r *http.Request) {
		zones, err := storage.ListZones(r.Context())
		if err != nil {
			slog.Error("failed to list zones: %v", "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		data, err := json.Marshal(zones)
		if err != nil {
			slog.Error("failed to serialize zones: %v", "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Write(data)
	})

	// TODO zone GET to list all records

	// Zone manipulation
	mux.HandleFunc("PUT /api/v1/zone/{zone}", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("zone")
		storage.AddZone(r.Context(), name)
	})
	mux.HandleFunc("DELETE /api/v1/zone/{zone}", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("zone")
		storage.DeleteZone(r.Context(), name)
	})

	// DNS record manipulation
	mux.HandleFunc("PUT /api/v1/zone/{zone}/{record}", func(w http.ResponseWriter, r *http.Request) {
		zoneId := r.PathValue("zone")
		recordId := r.PathValue("record")

		body, err := io.ReadAll(r.Body)
		if err != nil {
			slog.Error("failed to read request body: %v", "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		record, err := dns.NewRR(string(body))
		if err != nil {
			slog.Error("failed to parse record: %v", "error", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		storage.Patch(r.Context(), zoneId, zone.DnsRecord{
			Id:     recordId,
			Record: record,
		})
	})
	mux.HandleFunc("DELETE /api/v1/zone/{zone}/{record}", func(w http.ResponseWriter, r *http.Request) {
		zoneId := r.PathValue("zone")
		recordId := r.PathValue("record")

		storage.Delete(r.Context(), zoneId, recordId)
	})

	// acme-dns compatibility
	mux.HandleFunc("GET /api/v1/zone/{zone}/acme/health", func(w http.ResponseWriter, r *http.Request) {
		// TODO actually health check the storage?
	})
	mux.HandleFunc("POST /api/v1/zone/{zone}/acme/update", func(w http.ResponseWriter, r *http.Request) {
		zoneId := r.PathValue("zone")

		body, err := io.ReadAll(r.Body)
		if err != nil {
			slog.Error("failed to read request body: %v", "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		var update acmeUpdate
		err = json.Unmarshal(body, &update)
		if err != nil {
			slog.Error("failed to parse acme-dns update: %v", "error", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		storage.Patch(r.Context(), zoneId, zone.DnsRecord{
			Id:     "acme-" + update.Subdomain,
			Record: &dns.TXT{Hdr: dns.RR_Header{Name: update.Subdomain, Rrtype: dns.TypeTXT}, Txt: []string{update.Txt}},
		})

		data, err := json.Marshal(acmeResponse{Txt: update.Txt})
		if err != nil {
			slog.Error("failed to serialize acme-dns response: %v", "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Write(data)
	})

	server := &http.Server{
		Addr:    addr,
		Handler: withAuth(mux, apiKeys),
	}
	go server.ListenAndServe()

	go func() {
		<-ctx.Done()
		server.Close()
	}()
}
