package testutil_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"git.studiopollinator.com/pollinator/cord/internal/client/testutil"
)

func TestNewOfflineHTTPClient_PreservesCancellation(
	t *testing.T,
) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://offline.invalid", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	_, err = testutil.NewOfflineHTTPClient().Do(req)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("request error = %v, want context.Canceled", err)
	}
}
