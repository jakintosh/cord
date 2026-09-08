package testutil

import (
	"errors"
	"net/http"
)

// NewOfflineHTTPClient fails requests immediately without accessing the network.
// Use it when a fixture has no server and background sync is expected to fail.
func NewOfflineHTTPClient() *http.Client {
	return &http.Client{Transport: offlineTransport{}}
}

type offlineTransport struct{}

func (offlineTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := req.Context().Err(); err != nil {
		return nil, err
	}
	return nil, errors.New("test server unavailable")
}
