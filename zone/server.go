package zone

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

type ZoneServer struct {
	ZoneIds []string
	context context.Context

	primary  ZoneStorage
	fallback ZoneStorage

	ZoneLock sync.RWMutex
	Zones    map[string]Zone

	refreshTicker *time.Ticker
}

func (server *ZoneServer) loadZones(fallback bool) error {
	ctx, cancelFunc := context.WithTimeout(server.context, 10*time.Second)
	defer cancelFunc()

	var storage ZoneStorage
	if fallback {
		storage = server.fallback
	} else {
		storage = server.primary
	}
	for _, zoneId := range server.ZoneIds {
		updatedAt, err := storage.LastUpdated(ctx, zoneId)
		if err != nil {
			return err
		}
		if updatedAt.Compare(server.Zones[zoneId].LastUpdated) > 0 {
			// Newer zone available
			server.ZoneLock.Lock()
			server.Zones[zoneId], err = storage.Load(ctx, zoneId)
			if err != nil {
				server.ZoneLock.Unlock()
				return err
			}
			server.ZoneLock.Unlock()
			slog.Info("loaded zone", "zoneId", zoneId, "updatedAt", updatedAt)
		}
	}
	return nil
}

func (server *ZoneServer) zoneRefresher() {
	for {
		select {
		case <-server.refreshTicker.C:
			err := server.loadZones(false)
			if err != nil {
				slog.Error("failed to refresh zones from primary, serving stale data!", "error", err)
			}
		case <-server.context.Done():
			return
		}
	}
}

func NewZoneServer(ctx context.Context, zoneIds []string, primary ZoneStorage, fallback ZoneStorage) *ZoneServer {
	server := &ZoneServer{
		ZoneIds:       zoneIds,
		context:       ctx,
		primary:       primary,
		fallback:      fallback,
		Zones:         make(map[string]Zone),
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
