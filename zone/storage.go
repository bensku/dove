package zone

import (
	"context"
	"fmt"
	"time"
)

type ZoneStorage interface {
	Load(ctx context.Context, zoneId string) (Zone, error)
	LastUpdated(ctx context.Context, zoneId string) (time.Time, error)
	Patch(ctx context.Context, zoneId string, record DnsRecord) error
	Delete(ctx context.Context, zoneId string, id string) error
	Clear(ctx context.Context, zoneId string) error
}

func InternalTransfer(ctx context.Context, zone Zone, to ZoneStorage) error {
	err := to.Clear(ctx, zone.Id)
	if err != nil {
		return fmt.Errorf("failed to clear transfer target: %v", err)
	}
	for _, record := range zone.Records {
		err = to.Patch(ctx, zone.Id, record)
		if err != nil {
			return fmt.Errorf("failed to transfer record: %v", err)
		}
	}
	return nil
}
