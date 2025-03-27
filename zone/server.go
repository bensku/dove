package zone

import (
	"context"
	"log/slog"
	"slices"
	"time"
)

type ZoneServer struct {
	context context.Context

	primary  ZoneStorage
	fallback ZoneStorage

	ZoneIds       []string
	Zones         map[string]*Zone
	onZoneUpdated func(name string, zone *Zone)

	refreshTicker *time.Ticker
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
	oldZoneIds := s.ZoneIds
	s.ZoneIds = zoneIds

	// Update the loaded zones
	for _, zoneId := range s.ZoneIds {
		current, err := storage.IsCurrent(ctx, s.Zones[zoneId])
		if err != nil {
			return err
		}
		if !current {
			// Newer zone available
			zone, err := storage.Load(ctx, zoneId)
			if err != nil {
				return err
			}
			s.Zones[zoneId] = &zone

			// Notify listener
			if s.onZoneUpdated != nil {
				s.onZoneUpdated(zoneId, s.Zones[zoneId])
			}

			// Transfer to local storage in case we lose etcd
			InternalTransfer(ctx, zone, s.fallback)

			slog.Info("loaded zone", "zoneId", zoneId)
		}
		slog.Debug("checked zone for update", "zoneId", zoneId, "updated", !current)
	}

	// Check if we removed any zones and call listener for them
	for _, zoneId := range oldZoneIds {
		if !slices.Contains(zoneIds, zoneId) {
			if s.onZoneUpdated != nil {
				delete(s.Zones, zoneId)
				s.onZoneUpdated(zoneId, nil)
				slog.Info("unloaded zone", "zoneId", zoneId)
			}
		}
	}

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

func NewZoneServer(ctx context.Context, primary ZoneStorage, fallback ZoneStorage,
	onZoneUpdated func(name string, zone *Zone), refreshInterval time.Duration) *ZoneServer {
	server := &ZoneServer{
		ZoneIds:       make([]string, 0),
		context:       ctx,
		primary:       primary,
		fallback:      fallback,
		Zones:         make(map[string]*Zone),
		onZoneUpdated: onZoneUpdated,
		refreshTicker: time.NewTicker(refreshInterval),
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
