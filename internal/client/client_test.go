// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestDo_SendsTokenHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Api-Token"); got != "tok" {
			t.Errorf("X-Api-Token = %q, want %q", got, "tok")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	var out map[string]any
	_, err := c.do(context.Background(), http.MethodGet, "/ping", nil, nil, &out)
	if err != nil {
		t.Fatalf("do() error = %v", err)
	}
}

func TestDo_Retries429ThenSucceeds(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&hits, 1)
		if n <= 2 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok", WithBackoff(time.Millisecond, 5*time.Millisecond), WithRateLimit(1000, 1000))
	var out map[string]any
	_, err := c.do(context.Background(), http.MethodGet, "/ping", nil, nil, &out)
	if err != nil {
		t.Fatalf("do() error = %v", err)
	}
	if hits != 3 {
		t.Fatalf("hits = %d, want 3", hits)
	}
}

func TestDo_Retries5xx(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&hits, 1)
		if n == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok", WithBackoff(time.Millisecond, 5*time.Millisecond), WithRateLimit(1000, 1000))
	var out map[string]any
	_, err := c.do(context.Background(), http.MethodGet, "/ping", nil, nil, &out)
	if err != nil {
		t.Fatalf("do() error = %v", err)
	}
	if hits != 2 {
		t.Fatalf("hits = %d, want 2", hits)
	}
}

func TestDo_DoesNotRetry4xx(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	c := New(srv.URL, "tok", WithRateLimit(1000, 1000))
	var out map[string]any
	_, err := c.do(context.Background(), http.MethodGet, "/ping", nil, nil, &out)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if hits != 1 {
		t.Fatalf("hits = %d, want 1", hits)
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v, want *APIError", err)
	}
	if apiErr.StatusCode != 400 {
		t.Fatalf("StatusCode = %d, want 400", apiErr.StatusCode)
	}
}

func TestDo_GivesUpAfterMaxRetries(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	c := New(srv.URL, "tok", WithBackoff(time.Millisecond, 5*time.Millisecond), WithRateLimit(1000, 1000), WithMaxRetries(2))
	var out map[string]any
	_, err := c.do(context.Background(), http.MethodGet, "/ping", nil, nil, &out)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if hits != 3 {
		t.Fatalf("hits = %d, want 3", hits)
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v, want *APIError", err)
	}
	if apiErr.StatusCode != 429 {
		t.Fatalf("StatusCode = %d, want 429", apiErr.StatusCode)
	}
}

func TestDo_StopsOnContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	c := New(srv.URL, "tok", WithBackoff(time.Hour, time.Hour), WithRateLimit(1000, 1000))
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	var out map[string]any
	_, err := c.do(ctx, http.MethodGet, "/ping", nil, nil, &out)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestAPIError_DecodesErrorBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":{"code":404,"message":"Not Found"}}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok", WithRateLimit(1000, 1000))
	var out map[string]any
	_, err := c.do(context.Background(), http.MethodGet, "/ping", nil, nil, &out)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !IsNotFound(err) {
		t.Fatalf("IsNotFound(%v) = false, want true", err)
	}
	if err.Error() != "lokalise: 404 Not Found" {
		t.Fatalf("Error() = %q, want %q", err.Error(), "lokalise: 404 Not Found")
	}
}

func TestListAll_FollowsPages(t *testing.T) {
	type item struct {
		ID int `json:"id"`
	}

	var gotPages []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("limit") != "5000" {
			t.Errorf("limit = %q, want 5000", q.Get("limit"))
		}
		page := q.Get("page")
		gotPages = append(gotPages, page)

		var id int
		switch page {
		case "1":
			id = 1
		case "2":
			id = 2
		case "3":
			id = 3
		}

		w.Header().Set("X-Pagination-Page-Count", "3")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"items":[{"id":%d}]}`, id)
	}))
	defer srv.Close()

	c := New(srv.URL, "tok", WithRateLimit(1000, 1000))
	items, err := listAll[item](context.Background(), c, "/items", "items")
	if err != nil {
		t.Fatalf("listAll() error = %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("len(items) = %d, want 3", len(items))
	}
	want := []string{"1", "2", "3"}
	if len(gotPages) != 3 || gotPages[0] != want[0] || gotPages[1] != want[1] || gotPages[2] != want[2] {
		t.Fatalf("gotPages = %v, want %v", gotPages, want)
	}
}

func TestRateLimit_Throttles(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok", WithRateLimit(10, 1))
	start := time.Now()
	for i := 0; i < 5; i++ {
		var out map[string]any
		if _, err := c.do(context.Background(), http.MethodGet, "/ping", nil, nil, &out); err != nil {
			t.Fatalf("do() error = %v", err)
		}
	}
	elapsed := time.Since(start)
	if elapsed < 350*time.Millisecond {
		t.Fatalf("elapsed = %v, want >= 350ms", elapsed)
	}
}

// TestDo_POSTNotRetriedOn5xx asserts a POST is not retried on a 5xx: the
// request may already have created the resource, and retrying would create
// a duplicate.
func TestDo_POSTNotRetriedOn5xx(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	c := New(srv.URL, "tok", WithBackoff(time.Millisecond, 5*time.Millisecond), WithRateLimit(1000, 1000))
	_, err := c.do(context.Background(), http.MethodPost, "/things", nil, map[string]any{"a": 1}, nil)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusBadGateway {
		t.Fatalf("err = %v, want 502 *APIError", err)
	}
	if hits != 1 {
		t.Fatalf("hits = %d, want 1", hits)
	}
}

// TestDo_POSTNotRetriedOnNetworkError asserts a POST is not retried when the
// connection drops before a response.
func TestDo_POSTNotRetriedOnNetworkError(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Errorf("hijack: %v", err)
			return
		}
		conn.Close()
	}))
	defer srv.Close()

	c := New(srv.URL, "tok", WithBackoff(time.Millisecond, 5*time.Millisecond), WithRateLimit(1000, 1000))
	_, err := c.do(context.Background(), http.MethodPost, "/things", nil, map[string]any{"a": 1}, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if hits != 1 {
		t.Fatalf("hits = %d, want 1", hits)
	}
}

func TestDo_POSTRetries429(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&hits, 1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok", WithBackoff(time.Millisecond, 5*time.Millisecond), WithRateLimit(1000, 1000))
	if _, err := c.do(context.Background(), http.MethodPost, "/things", nil, map[string]any{"a": 1}, nil); err != nil {
		t.Fatalf("do() error = %v", err)
	}
	if hits != 2 {
		t.Fatalf("hits = %d, want 2", hits)
	}
}

// TestDo_IdempotentMethodsRetry5xxAndNetworkErrors asserts GET, PUT and
// DELETE keep retrying 5xx responses and dropped connections.
func TestDo_IdempotentMethodsRetry5xxAndNetworkErrors(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			var hits int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch atomic.AddInt32(&hits, 1) {
				case 1:
					w.WriteHeader(http.StatusServiceUnavailable)
				case 2:
					conn, _, _ := w.(http.Hijacker).Hijack()
					conn.Close()
				default:
					w.Header().Set("Content-Type", "application/json")
					w.Write([]byte(`{"ok":true}`))
				}
			}))
			defer srv.Close()

			c := New(srv.URL, "tok", WithBackoff(time.Millisecond, 5*time.Millisecond), WithRateLimit(1000, 1000))
			if _, err := c.do(context.Background(), method, "/things", nil, nil, nil); err != nil {
				t.Fatalf("do() error = %v", err)
			}
			if hits != 3 {
				t.Fatalf("hits = %d, want 3", hits)
			}
		})
	}
}

func TestNew_DefaultHTTPClientHasTimeout(t *testing.T) {
	c := New("http://example.invalid", "tok")
	if c.httpClient == http.DefaultClient {
		t.Fatal("default httpClient is http.DefaultClient, which has no timeout")
	}
	if c.httpClient.Timeout != 60*time.Second {
		t.Fatalf("httpClient.Timeout = %s, want 60s", c.httpClient.Timeout)
	}
}

func TestAPIError_IncludesDetails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":{"code":400,"message":"Invalid request","details":{"lang_iso":"Language code is not valid"}}}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok", WithRateLimit(1000, 1000))
	_, err := c.do(context.Background(), http.MethodGet, "/ping", nil, nil, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	want := `lokalise: 400 Invalid request (details: {"lang_iso":"Language code is not valid"})`
	if err.Error() != want {
		t.Fatalf("Error() = %q, want %q", err.Error(), want)
	}
}
