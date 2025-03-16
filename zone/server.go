package zone

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

type ZoneServer struct {
	context context.Context

	primary  ZoneStorage
	fallback ZoneStorage

	ZoneLock      sync.RWMutex
	ZoneIds       []string
	Zones         map[string]*Zone
	onZoneUpdated func(name string, zone *Zone)

	refreshTicker *time.Ticker
}

func (s *ZoneServer) FindZone(query string) *Zone {
	// Find the most specific zone that matches this query
	zoneName := ""
	bestMatch := ""

	s.ZoneLock.RLock()
	defer s.ZoneLock.RUnlock()
	for id := range s.Zones {
		// Keep track of the longest (most specific) match
		if len(id) > len(bestMatch) {
			bestMatch = id
			zoneName = id
		}
	}
	if zoneName == "" {
		return nil
	}
	return s.Zones[zoneName]
}

func (s *ZoneServer) loadZones(fallback bool) error {
	ctx, cancelFunc := context.WithTimeout(s.context, 10*time.Second)
	defer cancelFunc()

	var storage ZoneStorage
	if fallback {
		storage = s.fallback
	} else {
		storage = s.primary
	}

	// Load zone ids
	zoneIds, err := storage.ListZones(ctx)
	if err != nil {
		return err
	}
	s.ZoneLock.Lock()
	oldZoneIds := s.Zones
	s.ZoneIds = zoneIds
	s.ZoneLock.Unlock()

	// Update the loaded zones
	for _, zoneId := range s.ZoneIds {
		current, err := storage.IsCurrent(ctx, s.Zones[zoneId])
		if err != nil {
			return err
		}
		if !current {
			// Newer zone available
			s.ZoneLock.Lock()
			zone, err := storage.Load(ctx, zoneId)
			if err != nil {
				s.ZoneLock.Unlock()
				return err
			}
			s.Zones[zoneId] = &zone

			// Notify listener
			if s.onZoneUpdated != nil {
				s.onZoneUpdated(zoneId, s.Zones[zoneId])
			}

			s.ZoneLock.Unlock()
			slog.Info("loaded zone", "zoneId", zoneId)
		}
	}

	// Check if we removed any zones and call listener for them
	s.ZoneLock.Lock()
	for zoneId := range oldZoneIds {
		if _, ok := s.Zones[zoneId]; !ok {
			if s.onZoneUpdated != nil {
				s.onZoneUpdated(zoneId, nil)
			}
		}
	}
	s.ZoneLock.Unlock()

	return nil
}

func (s *ZoneServer) zoneRefresher() {
	for {
		select {
		case <-s.refreshTicker.C:
			err := s.loadZones(false)
			if err != nil {
				slog.Error("failed to refresh zones from primary, serving stale data!", "error", err)
			}
		case <-s.context.Done():
			return
		}
	}
}

func (s *ZoneServer) Close() {
	s.refreshTicker.Stop()
}

func NewZoneServer(ctx context.Context, primary ZoneStorage, fallback ZoneStorage) *ZoneServer {
	server := &ZoneServer{
		ZoneIds:       make([]string, 0),
		context:       ctx,
		primary:       primary,
		fallback:      fallback,
		Zones:         make(map[string]*Zone),
		refreshTicker: time.NewTicker(5 * time.Second),
	}

	// Initial zone load
	slog.Info("loading zones from primary")
	err := server.loadZones(false)
	if err != nil {
		slog.Error("failed to load zones from primary", "error", err)
		err = server.loadZones(true)
		if err != nil {
			slog.Error("failed to load zones from fallback", "error", err)
		}
	}
	if len(server.Zones) == 0 {
		slog.Warn("no DNS zones loaded")
	}

	go server.zoneRefresher()

	return server
}
