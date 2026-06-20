package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
)

type Transport struct {
	socketPath string
	http       *http.Client
}

func NewTransport(
	socketPath string,
) *Transport {
	return &Transport{
		socketPath: socketPath,
		http: &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					var d net.Dialer
					return d.DialContext(ctx, "unix", socketPath)
				},
			},
		},
	}
}

func (t *Transport) Get(
	ctx context.Context,
	path string,
) (
	*http.Response,
	error,
) {
	return t.http.Get("http://unix" + path)
}

func (t *Transport) Post(
	ctx context.Context,
	path string,
	v any,
) (
	*http.Response,
	error,
) {
	body, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	return t.http.Post("http://unix"+path, "application/json", bytes.NewReader(body))
}

func (t *Transport) Delete(
	ctx context.Context,
	path string,
) (
	*http.Response,
	error,
) {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, "http://unix"+path, nil)
	if err != nil {
		return nil, err
	}
	return t.http.Do(req)
}

func (t *Transport) Patch(
	ctx context.Context,
	path string,
	v any,
) (
	*http.Response,
	error,
) {
	body, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, "http://unix"+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return t.http.Do(req)
}

func DecodeResponse[T any](
	resp *http.Response,
) (
	T,
	error,
) {
	var zero T
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		body, _ := io.ReadAll(resp.Body)
		return zero, fmt.Errorf("unexpected status %s: %s", resp.Status, string(body))
	}
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return zero, fmt.Errorf("decode: %w", err)
	}
	if len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return zero, nil
	}
	var v T
	if err := json.Unmarshal(envelope.Data, &v); err != nil {
		return zero, fmt.Errorf("decode: %w", err)
	}
	return v, nil
}
