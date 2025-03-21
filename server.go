package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/miekg/dns"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// DNSRecord represents a DNS record stored in etcd
type DNSRecord struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Class string `json:"class"`
	TTL   int    `json:"ttl"`
	Data  string `json:"data"`
}

// CacheEntry represents a cached DNS response
type CacheEntry struct {
	Records   []dns.RR
	ExpiresAt time.Time
}

// DNSServer handles DNS queries using etcd as a backend
type DNSServer struct {
	etcdClient       *clientv3.Client
	cache            map[string]CacheEntry
	cacheMutex       sync.RWMutex
	defaultTTL       time.Duration
	cacheCleaner     *time.Ticker
	cacheCleanerCtx  context.Context
	cacheCleanerStop context.CancelFunc
}

// NewDNSServer creates a new DNS server instance
func NewDNSServer(etcdEndpoints []string, defaultTTL time.Duration) (*DNSServer, error) {
	// Initialize etcd client
	etcdClient, err := clientv3.New(clientv3.Config{
		Endpoints:   etcdEndpoints,
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create etcd client: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Create the DNS server
	server := &DNSServer{
		etcdClient:       etcdClient,
		cache:            make(map[string]CacheEntry),
		defaultTTL:       defaultTTL,
		cacheCleaner:     time.NewTicker(5 * time.Minute),
		cacheCleanerCtx:  ctx,
		cacheCleanerStop: cancel,
	}

	// Start cache cleaner
	go server.startCacheCleaner()

	return server, nil
}

// startCacheCleaner periodically removes expired cache entries
func (s *DNSServer) startCacheCleaner() {
	for {
		select {
		case <-s.cacheCleaner.C:
			s.cleanCache()
		case <-s.cacheCleanerCtx.Done():
			s.cacheCleaner.Stop()
			return
		}
	}
}

// cleanCache removes expired entries from the cache
func (s *DNSServer) cleanCache() {
	now := time.Now()
	s.cacheMutex.Lock()
	defer s.cacheMutex.Unlock()

	beforeCount := len(s.cache)
	for key, entry := range s.cache {
		if now.After(entry.ExpiresAt) {
			delete(s.cache, key)
		}
	}
	afterCount := len(s.cache)

	if beforeCount != afterCount {
		log.Printf("Cache cleanup: removed %d expired entries, %d remaining",
			beforeCount-afterCount, afterCount)
	}
}

// handleQuery processes the DNS query
func (s *DNSServer) handleQuery(w dns.ResponseWriter, r *dns.Msg) {
	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative = true

	// Process each question
	for _, q := range r.Question {
		log.Printf("Query for %s (type %s)", q.Name, dns.TypeToString[q.Qtype])

		// Try to get the answer from cache
		cacheKey := fmt.Sprintf("%s:%d", q.Name, q.Qtype)
		s.cacheMutex.RLock()
		entry, found := s.cache[cacheKey]
		s.cacheMutex.RUnlock()

		if found && time.Now().Before(entry.ExpiresAt) {
			// Cache hit
			log.Printf("Cache hit for %s", cacheKey)
			m.Answer = append(m.Answer, entry.Records...)
			continue
		}

		// Cache miss or expired, fetch from etcd
		records, err := s.lookupEtcd(q.Name, q.Qtype)
		if err != nil {
			log.Printf("Error looking up %s in etcd: %v", q.Name, err)
			continue
		}

		if len(records) > 0 {
			// Update cache
			ttl := s.defaultTTL
			// If we have records with TTL, use the minimum TTL
			for _, rr := range records {
				if rr.Header().Ttl > 0 && time.Duration(rr.Header().Ttl)*time.Second < ttl {
					ttl = time.Duration(rr.Header().Ttl) * time.Second
				}
			}

			s.cacheMutex.Lock()
			s.cache[cacheKey] = CacheEntry{
				Records:   records,
				ExpiresAt: time.Now().Add(ttl),
			}
			s.cacheMutex.Unlock()

			// Add to response
			m.Answer = append(m.Answer, records...)
		} else if len(m.Answer) == 0 {
			// No records found, set NXDOMAIN
			m.SetRcode(r, dns.RcodeNameError)
		}
	}

	w.WriteMsg(m)
}

// lookupEtcd fetches DNS records from etcd
func (s *DNSServer) lookupEtcd(name string, qtype uint16) ([]dns.RR, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Ensure name ends with a dot
	if !strings.HasSuffix(name, ".") {
		name = name + "."
	}

	// Construct etcd key prefix
	keyPrefix := fmt.Sprintf("/dns/%s", name)

	// Get all records for this name
	resp, err := s.etcdClient.Get(ctx, keyPrefix, clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("etcd query failed: %v", err)
	}

	var records []dns.RR

	// Process each record
	for _, kv := range resp.Kvs {
		var record DNSRecord
		if err := json.Unmarshal(kv.Value, &record); err != nil {
			log.Printf("Failed to unmarshal record: %v", err)
			continue
		}

		// Check if this record matches the requested type
		if qtype != dns.TypeANY {
			recordType, exists := dns.StringToType[record.Type]
			if !exists || recordType != qtype {
				continue
			}
		}

		// Create RR based on record type
		rr, err := createRR(record)
		if err != nil {
			log.Printf("Failed to create RR: %v", err)
			continue
		}

		records = append(records, rr)
	}

	return records, nil
}

// createRR creates a specific resource record based on its type
func createRR(record DNSRecord) (dns.RR, error) {
	// Construct the RR string based on record type
	rrStr := fmt.Sprintf("%s %d %s %s ", record.Name, record.TTL, record.Class, record.Type)

	switch record.Type {
	case "A":
		rrStr += record.Data
	case "AAAA":
		rrStr += record.Data
	case "CNAME":
		rrStr += record.Data
	case "MX":
		parts := strings.Split(record.Data, " ")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid MX data format: %s", record.Data)
		}
		pref, err := strconv.Atoi(parts[0])
		if err != nil {
			return nil, fmt.Errorf("invalid MX preference: %s", parts[0])
		}
		rrStr += fmt.Sprintf("%d %s", pref, parts[1])
	case "TXT":
		rrStr += fmt.Sprintf("\"%s\"", record.Data)
	case "SRV":
		parts := strings.Split(record.Data, " ")
		if len(parts) != 4 {
			return nil, fmt.Errorf("invalid SRV data format: %s", record.Data)
		}
		priority, err1 := strconv.Atoi(parts[0])
		weight, err2 := strconv.Atoi(parts[1])
		port, err3 := strconv.Atoi(parts[2])
		if err1 != nil || err2 != nil || err3 != nil {
			return nil, fmt.Errorf("invalid SRV record values")
		}
		rrStr += fmt.Sprintf("%d %d %d %s", priority, weight, port, parts[3])
	case "NS":
		rrStr += record.Data
	case "PTR":
		rrStr += record.Data
	case "SOA":
		parts := strings.Split(record.Data, " ")
		if len(parts) != 7 {
			return nil, fmt.Errorf("invalid SOA data format: %s", record.Data)
		}
		rrStr += strings.Join(parts, " ")
	default:
		return nil, fmt.Errorf("unsupported record type: %s", record.Type)
	}

	return dns.NewRR(rrStr)
}

// AddRecord adds a DNS record to etcd
func (s *DNSServer) AddRecord(record DNSRecord) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Ensure name ends with a dot
	if !strings.HasSuffix(record.Name, ".") {
		record.Name = record.Name + "."
	}

	// Set default class if not provided
	if record.Class == "" {
		record.Class = "IN"
	}

	// Set default TTL if not provided
	if record.TTL <= 0 {
		record.TTL = int(s.defaultTTL.Seconds())
	}

	// Validate record
	if _, err := createRR(record); err != nil {
		return fmt.Errorf("invalid record: %v", err)
	}

	// Construct etcd key
	key := fmt.Sprintf("/dns/%s/%s", record.Name, record.Type)

	// Serialize record to JSON
	value, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("failed to marshal record: %v", err)
	}

	// Store in etcd
	_, err = s.etcdClient.Put(ctx, key, string(value))
	if err != nil {
		return fmt.Errorf("failed to store record in etcd: %v", err)
	}

	return nil
}

// Start starts the DNS server
func (s *DNSServer) Start(addr string) error {
	// Create DNS server for UDP
	serverUDP := &dns.Server{Addr: addr, Net: "udp"}

	// Create DNS server for TCP
	serverTCP := &dns.Server{Addr: addr, Net: "tcp"}

	// Set handler
	dns.HandleFunc(".", s.handleQuery)

	log.Printf("Starting DNS server on %s (UDP and TCP)", addr)

	// Start servers in goroutines
	go func() {
		if err := serverUDP.ListenAndServe(); err != nil {
			log.Fatalf("Failed to start UDP server: %v", err)
		}
	}()

	go func() {
		if err := serverTCP.ListenAndServe(); err != nil {
			log.Fatalf("Failed to start TCP server: %v", err)
		}
	}()

	return nil
}

// Close closes the DNS server and releases resources
func (s *DNSServer) Close() {
	s.cacheCleanerStop()
	if s.etcdClient != nil {
		s.etcdClient.Close()
	}
}

func main() {
	// Parse command-line flags
	etcdEndpoints := flag.String("etcd", "localhost:2379", "Comma-separated list of etcd endpoints")
	listenAddr := flag.String("listen", ":53", "Address to listen on (IP:port)")
	ttl := flag.Duration("ttl", 5*time.Minute, "Default TTL for cached entries")
	flag.Parse()

	// Split etcd endpoints
	endpoints := strings.Split(*etcdEndpoints, ",")

	// Create the DNS server
	server, err := NewDNSServer(endpoints, *ttl)
	if err != nil {
		log.Fatalf("Failed to create DNS server: %v", err)
	}
	defer server.Close()

	// Handle signals for graceful shutdown
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	// Start the server
	if err := server.Start(*listenAddr); err != nil {
		log.Fatalf("Failed to start DNS server: %v", err)
	}

	// Wait for signal
	<-sig
	log.Println("Shutting down...")
}
