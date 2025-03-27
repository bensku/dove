# dove DNS server
Dove is a highly available, API-driven DNS server.

Zones and DNS records are defined using a HTTP API. Their information is
persisted to and replicated by [etcd](https://etcd.io/) database.
While (properly deployed) etcd can be very resilient, Dove also caches
the zone data locally - because having DNS fail when you accidentally
blow up your etcd cluster might make getting it back up quite a bit harder!

## Features
* HTTP API with API key -based authentication
* etcd as primary data store, with fallback to local disk
* Multiple zones per server
* Backed by [miekg/dns](https://github.com/miekg/dns) - all DNS records supported
* Wildcard record support

## Usage
To build a self-contained binary, run:
```sh
go build
```

To run a development server:
```sh
# Launch development etcd cluster
docker-compose up -d # podman-compose also confirmed to work
./dev-server.sh # Launches in foreground
```

To run automated tests, make sure you have the development server up, and run:
```sh
go test
go test -v ./zone
```

For deploying into production: you'll need to build it yourself.
Prebuilt executables and/or container images coming soon!