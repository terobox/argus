package alert

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ntfy priority levels. 3 is the default, 4 is high, 5 is urgent.
const (
	PriorityNormal = 3
	PriorityHigh   = 4
	PriorityUrgent = 5
)

// Sender delivers a single notification. Dispatcher implements it; tests can
// supply a recording fake.
type Sender interface {
	Notify(ctx context.Context, title, message string, priority int) error
}

// Dispatcher posts notifications to an ntfy-compatible HTTP endpoint. ntfy's
// plain-HTTP protocol carries the message in the body and the title, priority,
// and tags in headers, so no ntfy client library is required.
type Dispatcher struct {
	url    string
	client *http.Client
}

// NewDispatcher returns a Dispatcher for url, an absolute ntfy topic URL such as
// "https://ntfy.sh/mytopic".
func NewDispatcher(url string) *Dispatcher {
	return &Dispatcher{
		url:    url,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

// Notify sends one notification. A non-2xx response is an error, as is any
// transport failure.
func (d *Dispatcher) Notify(ctx context.Context, title, message string, priority int) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.url, strings.NewReader(message))
	if err != nil {
		return fmt.Errorf("alert: build request: %w", err)
	}
	req.Header.Set("Title", title)
	req.Header.Set("Priority", strconv.Itoa(priority))
	switch {
	case priority >= PriorityUrgent:
		req.Header.Set("Tags", "rotating_light")
	case priority >= PriorityHigh:
		req.Header.Set("Tags", "warning")
	default:
		req.Header.Set("Tags", "white_check_mark")
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("alert: post webhook: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("alert: webhook returned %s", resp.Status)
	}
	return nil
}
