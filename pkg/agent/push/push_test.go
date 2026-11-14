package push

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/terobox/argus/pkg/metrics"
	"github.com/terobox/argus/pkg/protocol"
)

func TestSendHeadersAndBody(t *testing.T) {
	var gotToken, gotEncoding string
	var got protocol.Report

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != protocol.IngestPath {
			t.Errorf("path = %q, want %q", r.URL.Path, protocol.IngestPath)
		}
		gotToken = r.Header.Get(protocol.NodeTokenHeader)
		gotEncoding = r.Header.Get("Content-Encoding")
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode body: %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := New(srv.URL, "secret-token", false)
	r := &protocol.Report{Version: protocol.Version, NodeID: "n1", CollectedAt: 1, Samples: []metrics.Sample{{TS: 1}}}
	if err := c.Send(context.Background(), r); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if gotToken != "secret-token" {
		t.Errorf("node token header = %q, want secret-token", gotToken)
	}
	if gotEncoding != "" {
		t.Errorf("Content-Encoding = %q, want empty", gotEncoding)
	}
	if got.NodeID != "n1" || len(got.Samples) != 1 {
		t.Errorf("report = %+v", got)
	}
}

func TestSendGzip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Encoding") != "gzip" {
			t.Errorf("Content-Encoding = %q, want gzip", r.Header.Get("Content-Encoding"))
		}
		zr, err := gzip.NewReader(r.Body)
		if err != nil {
			t.Errorf("gzip reader: %v", err)
			return
		}
		var rep protocol.Report
		if err := json.NewDecoder(zr).Decode(&rep); err != nil {
			t.Errorf("decode: %v", err)
		}
		if rep.NodeID != "n1" {
			t.Errorf("node id = %q, want n1", rep.NodeID)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := New(srv.URL, "t", true)
	r := &protocol.Report{Version: protocol.Version, NodeID: "n1", CollectedAt: 1}
	if err := c.Send(context.Background(), r); err != nil {
		t.Fatalf("Send gzip: %v", err)
	}
}

func TestSendNon2xxWrapsHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	c := New(srv.URL, "t", false)
	r := &protocol.Report{Version: protocol.Version, NodeID: "n1", CollectedAt: 1}
	err := c.Send(context.Background(), r)
	if err == nil {
		t.Fatal("expected error for 400, got nil")
	}
	var he *HTTPError
	if !errors.As(err, &he) || he.StatusCode != http.StatusBadRequest {
		t.Errorf("err = %v, want *HTTPError{400}", err)
	}
}

func TestSendWithRetryTransientThenSuccess(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := New(srv.URL, "t", false)
	r := &protocol.Report{Version: protocol.Version, NodeID: "n1", CollectedAt: 1}
	if err := c.SendWithRetry(context.Background(), r, 3, time.Millisecond); err != nil {
		t.Fatalf("SendWithRetry: %v", err)
	}
	if atomic.LoadInt32(&calls) != 2 {
		t.Errorf("calls = %d, want 2", calls)
	}
}

func TestSendWithRetryPermanentStops(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := New(srv.URL, "t", false)
	r := &protocol.Report{Version: protocol.Version, NodeID: "n1", CollectedAt: 1}
	if err := c.SendWithRetry(context.Background(), r, 3, time.Millisecond); err == nil {
		t.Fatal("expected error for 401, got nil")
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Errorf("calls = %d, want 1 (permanent failure must not retry)", calls)
	}
}

func TestSendConnectionRefusedRetries(t *testing.T) {
	// A server that is already closed yields a transport error, which is
	// retryable but can never succeed; SendWithRetry must honor attempts.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()

	c := New(url, "t", false)
	r := &protocol.Report{Version: protocol.Version, NodeID: "n1", CollectedAt: 1}
	if err := c.SendWithRetry(context.Background(), r, 2, time.Millisecond); err == nil {
		t.Fatal("expected transport error, got nil")
	}
}

func TestSendHost(t *testing.T) {
	var gotPath, gotToken string
	var got protocol.HostReport
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotToken = r.Header.Get(protocol.NodeTokenHeader)
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode: %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := New(srv.URL, "secret", false)
	h := &protocol.HostReport{
		Version: protocol.Version, NodeID: "n1", AgentVer: "dev",
		Info: metrics.HostInfo{Hostname: "web-1", AgentVer: "dev"},
	}
	if err := c.SendHost(context.Background(), h); err != nil {
		t.Fatalf("SendHost: %v", err)
	}
	if gotPath != protocol.HostPath {
		t.Errorf("path = %q, want %q", gotPath, protocol.HostPath)
	}
	if gotToken != "secret" {
		t.Errorf("token = %q, want secret", gotToken)
	}
	if got.Info.Hostname != "web-1" {
		t.Errorf("host info = %+v", got.Info)
	}
}
