package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"syscall"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://unix"+path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := t.http.Do(req)
	if err != nil {
		return nil, t.friendlyDialErr(err)
	}
	return resp, nil
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
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://unix"+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.http.Do(req)
	if err != nil {
		return nil, t.friendlyDialErr(err)
	}
	return resp, nil
}

// PostRaw sends body verbatim as the request body, without JSON
// re-encoding. Use it when the caller already holds an opaque JSON
// payload (e.g. an invitation file) that the daemon should parse and
// validate itself.
func (t *Transport) PostRaw(
	ctx context.Context,
	path string,
	body []byte,
) (
	*http.Response,
	error,
) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://unix"+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.http.Do(req)
	if err != nil {
		return nil, t.friendlyDialErr(err)
	}
	return resp, nil
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
	resp, err := t.http.Do(req)
	if err != nil {
		return nil, t.friendlyDialErr(err)
	}
	return resp, nil
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
	resp, err := t.http.Do(req)
	if err != nil {
		return nil, t.friendlyDialErr(err)
	}
	return resp, nil
}

// friendlyDialErr detects unix-socket dial failures (daemon not running,
// socket file missing) and rewrites them into a message naming the socket
// path, rather than surfacing the raw *url.Error/*net.OpError chain.
func (t *Transport) friendlyDialErr(
	err error,
) error {
	var urlErr *url.Error
	if !errors.As(err, &urlErr) {
		return err
	}
	var opErr *net.OpError
	if !errors.As(urlErr.Err, &opErr) {
		return err
	}
	if errors.Is(opErr.Err, syscall.ECONNREFUSED) || errors.Is(opErr.Err, syscall.ENOENT) || os.IsNotExist(opErr.Err) {
		return fmt.Errorf("cannot reach cord daemon at %s (is the daemon running?)", t.socketPath)
	}
	return err
}

// DecodeResponse decodes the wire envelope from resp. On a non-2xx status,
// it attempts to decode the wire error envelope and returns an error
// containing just the message; if the body isn't a valid envelope, it falls
// back to reporting the raw status and body.
func DecodeResponse[T any](
	resp *http.Response,
) (
	T,
	error,
) {
	var zero T
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return zero, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		var envelope struct {
			Error *wire.Error `json:"error"`
		}
		if err := json.Unmarshal(body, &envelope); err == nil && envelope.Error != nil && envelope.Error.Message != "" {
			return zero, errors.New(envelope.Error.Message)
		}
		return zero, fmt.Errorf("unexpected status %s: %s", resp.Status, string(body))
	}
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
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
