// Package push implements the agent's reporting HTTP client: it marshals a
// report to JSON (optionally gzip-compressed) and POSTs it to the server's
// ingest endpoint, retrying transient failures without dropping samples.
package push

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/terobox/argus/pkg/protocol"
)

// Client posts reports to an argus server.
type Client struct {
	serverURL string // base URL, no trailing slash
	token     string
	gzip      bool
	hc        *http.Client
}

// New returns a Client for the given server base URL and node token. When gzip
// is true the report body is gzip-compressed.
func New(serverURL, token string, gzip bool) *Client {
	return &Client{
		serverURL: strings.TrimRight(serverURL, "/"),
		token:     token,
		gzip:      gzip,
		hc:        &http.Client{Timeout: 30 * time.Second},
	}
}

// HTTPError is a non-2xx response from the server.
type HTTPError struct {
	StatusCode int
	Status     string
}

func (e *HTTPError) Error() string { return "server returned " + e.Status }

// Retryable reports whether this status deserves a retry: rate-limited (429) or
// a server-side fault (5xx).
func (e *HTTPError) Retryable() bool {
	return e.StatusCode == http.StatusTooManyRequests || e.StatusCode >= 500
}

// Send posts one report. It returns an error for transport failures and for any
// non-2xx response (wrapped in *HTTPError).
func (c *Client) Send(ctx context.Context, r *protocol.Report) error {
	body, err := json.Marshal(r)
	if err != nil {
		return fmt.Errorf("marshal report: %w", err)
	}
	return c.post(ctx, protocol.IngestPath, body)
}

// SendHost posts a host-info report to the host-info endpoint.
func (c *Client) SendHost(ctx context.Context, h *protocol.HostReport) error {
	body, err := json.Marshal(h)
	if err != nil {
		return fmt.Errorf("marshal host report: %w", err)
	}
	return c.post(ctx, protocol.HostPath, body)
}

// post sends an already-marshaled JSON body to path, gzip-compressing it when
// configured, and returns an error for transport failures or non-2xx responses.
func (c *Client) post(ctx context.Context, path string, body []byte) error {
	encoding := ""
	if c.gzip {
		var err error
		if body, err = gzipBytes(body); err != nil {
			return fmt.Errorf("gzip body: %w", err)
		}
		encoding = "gzip"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.serverURL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if encoding != "" {
		req.Header.Set("Content-Encoding", encoding)
	}
	req.Header.Set(protocol.NodeTokenHeader, c.token)

	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("post: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body) // drain so the connection can be reused

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &HTTPError{StatusCode: resp.StatusCode, Status: resp.Status}
	}
	return nil
}

// SendWithRetry calls Send up to attempts times, retrying transient failures
// (network errors, 5xx, 429) with exponential backoff starting at base.
// Permanent failures (4xx, context cancellation) return immediately.
func (c *Client) SendWithRetry(ctx context.Context, r *protocol.Report, attempts int, base time.Duration) error {
	if attempts <= 0 {
		attempts = 1
	}
	var err error
	delay := base
	for i := 0; i < attempts; i++ {
		if i > 0 {
			if !sleepCtx(ctx, delay) {
				return ctx.Err()
			}
			delay *= 2
		}
		if err = c.Send(ctx, r); err == nil {
			return nil
		}
		if !retryable(err) {
			return err
		}
	}
	return err
}

func retryable(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var he *HTTPError
	if errors.As(err, &he) {
		return he.Retryable()
	}
	return true // transport errors are transient
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

func gzipBytes(b []byte) ([]byte, error) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(b); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
