package zone

type Zone struct {
	Name        string
	Records     []DnsRecord
	UpdatedHash string
}
