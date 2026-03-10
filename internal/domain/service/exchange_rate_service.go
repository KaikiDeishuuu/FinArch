package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type ExchangeRateResult struct {
	Rate      *big.Rat
	RateFloat float64
	Source    string
	At        time.Time
}

type ExchangeRateService interface {
	GetRate(ctx context.Context, from, to string, at time.Time) (ExchangeRateResult, error)
}

type httpExchangeRateService struct {
	client *http.Client
	base   string
	ttl    time.Duration
	mu     sync.RWMutex
	cache  map[string]cachedRate
}

type cachedRate struct {
	result    ExchangeRateResult
	expiresAt time.Time
}

func NewHTTPExchangeRateService() ExchangeRateService {
	return &httpExchangeRateService{
		client: &http.Client{Timeout: 5 * time.Second},
		base:   "https://api.frankfurter.app",
		ttl:    15 * time.Minute,
		cache:  map[string]cachedRate{},
	}
}

func (s *httpExchangeRateService) GetRate(ctx context.Context, from, to string, at time.Time) (ExchangeRateResult, error) {
	from = strings.ToUpper(strings.TrimSpace(from))
	to = strings.ToUpper(strings.TrimSpace(to))
	if from == "" || to == "" {
		return ExchangeRateResult{}, fmt.Errorf("currency is required")
	}
	if from == to {
		r := big.NewRat(1, 1)
		return ExchangeRateResult{Rate: r, RateFloat: 1, Source: "identity", At: at.UTC()}, nil
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	dateKey := at.UTC().Format("2006-01-02")
	key := from + ":" + to + ":" + dateKey
	if got, ok := s.readCache(key); ok {
		return got, nil
	}

	result, err := s.fetchRate(ctx, "/"+dateKey, from, to)
	if err == nil {
		s.writeCache(key, result)
		return result, nil
	}
	latestResult, latestErr := s.fetchRate(ctx, "/latest", from, to)
	if latestErr != nil {
		return ExchangeRateResult{}, fmt.Errorf("fetch historical rate failed: %w; latest fallback failed: %v", err, latestErr)
	}
	latestResult.Source = "frankfurter:latest_fallback"
	s.writeCache(key, latestResult)
	return latestResult, nil
}

func (s *httpExchangeRateService) fetchRate(ctx context.Context, path, from, to string) (ExchangeRateResult, error) {
	u, _ := url.Parse(s.base + path)
	q := u.Query()
	q.Set("from", from)
	q.Set("to", to)
	u.RawQuery = q.Encode()

	var lastErr error
	for i := 0; i < 2; i++ {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		resp, err := s.client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		body, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			continue
		}
		if resp.StatusCode >= 300 {
			lastErr = fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
			continue
		}
		var payload struct {
			Date  string                 `json:"date"`
			Rates map[string]json.Number `json:"rates"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			lastErr = err
			continue
		}
		n, ok := payload.Rates[to]
		if !ok {
			lastErr = fmt.Errorf("missing rate for %s", to)
			continue
		}
		rateStr := n.String()
		rateRat, ok := new(big.Rat).SetString(rateStr)
		if !ok || rateRat.Sign() <= 0 {
			lastErr = fmt.Errorf("invalid rate %s", rateStr)
			continue
		}
		rateFloat, _ := n.Float64()
		asOf := time.Now().UTC()
		if payload.Date != "" {
			if d, err := time.Parse("2006-01-02", payload.Date); err == nil {
				asOf = d.UTC()
			}
		}
		return ExchangeRateResult{Rate: rateRat, RateFloat: rateFloat, Source: "frankfurter", At: asOf}, nil
	}
	return ExchangeRateResult{}, lastErr
}

func (s *httpExchangeRateService) readCache(key string) (ExchangeRateResult, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.cache[key]
	if !ok || time.Now().After(item.expiresAt) {
		return ExchangeRateResult{}, false
	}
	return item.result, true
}

func (s *httpExchangeRateService) writeCache(key string, result ExchangeRateResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cache[key] = cachedRate{result: result, expiresAt: time.Now().Add(s.ttl)}
}
