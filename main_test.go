package main_test

import (
	"fmt"
	"io"
	"net/http"
	"testing"
)

func request(method, url string) string {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		panic(err)
	}
	req.Header.Set("Authorization", "test-api-key")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		panic(err)
	}
	if res.StatusCode >= 400 {
		panic(fmt.Errorf("request failed with status %d", res.StatusCode))
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		panic(err)
	}
	return string(body)
}

func TestAdminApi(t *testing.T) {
	zones := request("GET", "http://localhost:8080/api/v1/zone")
	if zones != "[]" {
		t.Errorf("expected no zones, got %s", zones)
	}

	request("PUT", "http://localhost:8080/api/v1/zone/dove.test.")
	zones = request("GET", "http://localhost:8080/api/v1/zone")
	if zones != `["dove.test."]` {
		t.Errorf("expected dove.test., got %s", zones)
	}

	request("DELETE", "http://localhost:8080/api/v1/zone/dove.test.")
	zones = request("GET", "http://localhost:8080/api/v1/zone")
	if zones != "[]" {
		t.Errorf("zone was not properly deleted: %s", zones)
	}
}
