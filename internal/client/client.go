// Copyright (c) 2026 Pete Beegle
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/time/rate"
)

const (
	defaultRPS         = 6
	defaultBurst       = 6
	defaultBackoffBase = time.Second
	defaultBackoffMax  = 30 * time.Second
	defaultMaxRetries  = 5
	defaultHTTPTimeout = 60 * time.Second

	listLimit = 5000

	headerPageCount = "X-Pagination-Page-Count"
)

// HTTPClient is a Lokalise API client backed by net/http.
type HTTPClient struct {
	baseURL    string
	token      string
	httpClient *http.Client
	limiter    *rate.Limiter

	backoffBase time.Duration
	backoffMax  time.Duration
	maxRetries  int
}

var _ API = (*HTTPClient)(nil)

// Option configures an HTTPClient.
type Option func(*HTTPClient)

// WithHTTPClient overrides the underlying *http.Client.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *HTTPClient) {
		c.httpClient = hc
	}
}

// WithRateLimit overrides the request rate limit shared across all calls.
func WithRateLimit(rps float64, burst int) Option {
	return func(c *HTTPClient) {
		c.limiter = rate.NewLimiter(rate.Limit(rps), burst)
	}
}

// WithBackoff overrides the base and max durations used for retry backoff.
func WithBackoff(base, max time.Duration) Option {
	return func(c *HTTPClient) {
		c.backoffBase = base
		c.backoffMax = max
	}
}

// WithMaxRetries overrides the number of retries attempted after the
// initial request.
func WithMaxRetries(n int) Option {
	return func(c *HTTPClient) {
		c.maxRetries = n
	}
}

// New constructs an HTTPClient for the Lokalise API at baseURL, authenticating
// with token.
func New(baseURL, token string, opts ...Option) *HTTPClient {
	c := &HTTPClient{
		baseURL:     strings.TrimRight(baseURL, "/"),
		token:       token,
		httpClient:  &http.Client{Timeout: defaultHTTPTimeout},
		limiter:     rate.NewLimiter(rate.Limit(defaultRPS), defaultBurst),
		backoffBase: defaultBackoffBase,
		backoffMax:  defaultBackoffMax,
		maxRetries:  defaultMaxRetries,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// do performs a single logical request against the Lokalise API, retrying
// with full-jitter exponential backoff, bounded by ctx and c.maxRetries. 429
// responses are retried for every method. 5xx responses and network errors
// are retried only for methods other than POST: a failed POST may already
// have created the resource, and retrying it could create a duplicate. It
// encodes body as JSON when non-nil and decodes a successful response body
// into out when non-nil.
func (c *HTTPClient) do(ctx context.Context, method, path string, query url.Values, body, out any) (http.Header, error) {
	var reqBody []byte
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("lokalise: encode request: %w", err)
		}
		reqBody = b
	}

	fullURL := c.baseURL + path
	if len(query) > 0 {
		fullURL += "?" + query.Encode()
	}

	retryServerErrors := method != http.MethodPost

	for attempt := 0; ; attempt++ {
		if err := c.limiter.Wait(ctx); err != nil {
			return nil, err
		}

		var bodyReader io.Reader
		if reqBody != nil {
			bodyReader = bytes.NewReader(reqBody)
		}

		req, err := http.NewRequestWithContext(ctx, method, fullURL, bodyReader)
		if err != nil {
			return nil, fmt.Errorf("lokalise: build request: %w", err)
		}
		req.Header.Set("X-Api-Token", c.token)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if !retryServerErrors || attempt >= c.maxRetries {
				return nil, fmt.Errorf("lokalise: request failed: %w", err)
			}
			if serr := sleepCtx(ctx, backoffDuration(attempt, c.backoffBase, c.backoffMax)); serr != nil {
				return nil, serr
			}
			continue
		}

		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return resp.Header, fmt.Errorf("lokalise: read response: %w", err)
		}

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			if out != nil && len(respBody) > 0 {
				if err := json.Unmarshal(respBody, out); err != nil {
					return resp.Header, fmt.Errorf("lokalise: decode response: %w", err)
				}
			}
			return resp.Header, nil
		}

		apiErr := parseAPIError(resp.StatusCode, respBody)
		retryable := resp.StatusCode == http.StatusTooManyRequests || (retryServerErrors && resp.StatusCode >= 500)
		if !retryable || attempt >= c.maxRetries {
			return resp.Header, apiErr
		}
		if serr := sleepCtx(ctx, backoffDuration(attempt, c.backoffBase, c.backoffMax)); serr != nil {
			return resp.Header, serr
		}
	}
}

// listAll pages through path (limit=5000, page=1..) and decodes the JSON
// array at key into a []T, stopping when page reaches the response's
// X-Pagination-Page-Count header, or after the first page if the header is
// absent.
func listAll[T any](ctx context.Context, c *HTTPClient, path, key string) ([]T, error) {
	var all []T

	for page := 1; ; page++ {
		query := url.Values{
			"limit": {strconv.Itoa(listLimit)},
			"page":  {strconv.Itoa(page)},
		}

		var raw map[string]json.RawMessage
		headers, err := c.do(ctx, http.MethodGet, path, query, nil, &raw)
		if err != nil {
			return nil, err
		}

		if data, ok := raw[key]; ok {
			var items []T
			if err := json.Unmarshal(data, &items); err != nil {
				return nil, fmt.Errorf("lokalise: decode %q: %w", key, err)
			}
			all = append(all, items...)
		}

		pageCountHeader := headers.Get(headerPageCount)
		if pageCountHeader == "" {
			break
		}
		pageCount, err := strconv.Atoi(pageCountHeader)
		if err != nil {
			return nil, fmt.Errorf("lokalise: parse %s: %w", headerPageCount, err)
		}
		if page >= pageCount {
			break
		}
	}

	return all, nil
}

// parseAPIError builds an *APIError from a non-2xx response, decoding the
// Lokalise `{"error":{"code":..,"message":..,"details":..}}` shape (details,
// when present, appended as compact JSON) and falling back to the raw body
// text.
func parseAPIError(statusCode int, body []byte) *APIError {
	var eb errorBody
	if err := json.Unmarshal(body, &eb); err == nil && eb.Error.Message != "" {
		msg := eb.Error.Message
		if d := bytes.TrimSpace(eb.Error.Details); len(d) > 0 && !bytes.Equal(d, []byte("null")) {
			var buf bytes.Buffer
			if json.Compact(&buf, d) == nil {
				d = buf.Bytes()
			}
			msg += " (details: " + string(d) + ")"
		}
		return &APIError{StatusCode: statusCode, Message: msg}
	}
	return &APIError{StatusCode: statusCode, Message: strings.TrimSpace(string(body))}
}

// backoffDuration returns a full-jitter exponential backoff duration for the
// given attempt (0-indexed), bounded by [0, max].
func backoffDuration(attempt int, base, max time.Duration) time.Duration {
	if base <= 0 {
		return 0
	}
	capDur := float64(base) * math.Pow(2, float64(attempt))
	if capDur > float64(max) || capDur <= 0 {
		capDur = float64(max)
	}
	return time.Duration(rand.Float64() * capDur)
}

// sleepCtx sleeps for d, returning early with ctx.Err() if ctx is done first.
func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
