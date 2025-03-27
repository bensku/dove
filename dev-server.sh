#!/bin/sh
go run main.go --etcd-endpoints "http://localhost:2379" --dns-addr ":5300" --accept-keys "test-api-key" --log-level DEBUG --refresh-interval 1