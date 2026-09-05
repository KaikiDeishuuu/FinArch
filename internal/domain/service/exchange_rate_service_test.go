package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func testHTTPExchangeRateService(baseURL string) *httpExchangeRateService {
	return &httpExchangeRateService{
		client: &http.Client{Timeout: time.Second},
		base:   baseURL,
		ttl:    time.Minute,
		cache:  map[string]cachedRate{},
	}
}

func TestHTTPExchangeRateServiceAcceptsBoundedSuccessResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{\"date\":\"2026-09-04\",\"rates\":{\"EUR\":0.125}}"))
	}))
	defer server.Close()

	result, err := testHTTPExchangeRateService(server.URL).GetRate(
		context.Background(), "CNY", "EUR", time.Date(2026, 9, 4, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("GetRate: %v", err)
	}
	if result.Rate == nil || result.Rate.RatString() != "1/8" || result.RateFloat != 0.125 {
		t.Fatalf("unexpected rate: %#v", result)
	}
}

func TestHTTPExchangeRateServiceRejectsOversizedSuccessResponse(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, "{\"date\":\"2026-09-04\",\"rates\":{\"EUR\":1},\"padding\":\"%s\"}",
			strings.Repeat("x", int(exchangeRateResponseMaxBytes)))
	}))
	defer server.Close()

	_, err := testHTTPExchangeRateService(server.URL).GetRate(
		context.Background(), "CNY", "EUR", time.Date(2026, 9, 4, 0, 0, 0, 0, time.UTC),
	)
	if err == nil || !strings.Contains(err.Error(), "exceeds 65536 bytes") {
		t.Fatalf("oversized response error = %v", err)
	}
	if calls.Load() != 4 {
		t.Fatalf("upstream calls = %d, want two historical and two fallback attempts", calls.Load())
	}
}

func TestHTTPExchangeRateServiceBoundsAndEscapesErrorBody(t *testing.T) {
	const tailMarker = "TAIL-MUST-NOT-APPEAR"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("upstream failure\nforged-log-line:" +
			strings.Repeat("x", int(exchangeRateResponseMaxBytes)) + tailMarker))
	}))
	defer server.Close()

	_, err := testHTTPExchangeRateService(server.URL).GetRate(
		context.Background(), "CNY", "EUR", time.Date(2026, 9, 4, 0, 0, 0, 0, time.UTC),
	)
	if err == nil {
		t.Fatal("expected upstream error")
	}
	message := err.Error()
	if len(message) > 3_000 {
		t.Fatalf("error message was not bounded: %d bytes", len(message))
	}
	if !strings.Contains(message, "[truncated]") || strings.Contains(message, tailMarker) {
		t.Fatalf("unexpected bounded error: %q", message)
	}
	if strings.Contains(message, "\nforged-log-line") {
		t.Fatalf("error body can inject a log line: %q", message)
	}
}
