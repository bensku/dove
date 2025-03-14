package zone

import (
	"time"
)

type Zone struct {
	Id          string
	Records     []DnsRecord
	LastUpdated time.Time
}
