package zone

import (
	"context"
	"fmt"
)

type ZoneStorage interface {
	ListZones(ctx context.Context) ([]string, error)
	AddZone(ctx context.Context, zoneId string) error
	DeleteZone(ctx context.Context, zoneId string) error

	Load(ctx context.Context, zoneId string) (Zone, error)
	IsCurrent(ctx context.Context, zone *Zone) (bool, error)
	Patch(ctx context.Context, zoneId string, record DnsRecord) error
	Delete(ctx context.Context, zoneId string, id string) error
	Clear(ctx context.Context, zoneId string) error
}

func InternalTransfer(ctx context.Context, zone Zone, to ZoneStorage) error {
	err := to.Clear(ctx, zone.Name)
	if err != nil {
		return fmt.Errorf("failed to clear transfer target: %v", err)
	}
	for _, record := range zone.Records {
		err = to.Patch(ctx, zone.Name, record)
		if err != nil {
			return fmt.Errorf("failed to transfer record: %v", err)
		}
	}
	return nil
}
