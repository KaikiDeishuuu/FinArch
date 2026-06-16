package ocr

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"finarch/internal/domain/model"
)

func TestPaddleAIStudioProviderExtractSuccess(t *testing.T) {
	const token = "test-token-secret"
	var baseURL string
	polls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/jobs":
			if r.Method != http.MethodPost {
				t.Fatalf("submit method = %s", r.Method)
			}
			if got := r.Header.Get("Authorization"); got != "bearer "+token {
				t.Fatalf("submit authorization = %q", got)
			}
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Fatalf("parse multipart: %v", err)
			}
			if got := r.FormValue("model"); got != "PaddleOCR-VL-1.6" {
				t.Fatalf("model = %q", got)
			}
			if got := r.FormValue("optionalPayload"); !strings.Contains(got, "useDocOrientationClassify") {
				t.Fatalf("optionalPayload = %q", got)
			}
			file, header, err := r.FormFile("file")
			if err != nil {
				t.Fatalf("form file: %v", err)
			}
			defer file.Close()
			if header.Filename != "receipt.png" {
				t.Fatalf("filename = %q", header.Filename)
			}
			_, _ = w.Write([]byte(`{"data":{"jobId":"job-123"}}`))
		case "/jobs/job-123":
			if r.Method != http.MethodGet {
				t.Fatalf("poll method = %s", r.Method)
			}
			if got := r.Header.Get("Authorization"); got != "bearer "+token {
				t.Fatalf("poll authorization = %q", got)
			}
			polls++
			if polls == 1 {
				_, _ = w.Write([]byte(`{"data":{"state":"pending"}}`))
				return
			}
			_, _ = w.Write([]byte(fmt.Sprintf(`{"data":{"state":"done","resultUrl":{"jsonUrl":"%s/result.jsonl"}}}`, baseURL)))
		case "/result.jsonl":
			if got := r.Header.Get("Authorization"); got != "" {
				t.Fatalf("result download should not send auth header, got %q", got)
			}
			_, _ = w.Write([]byte(`{"result":{"layoutParsingResults":[{"markdown":{"text":"# Receipt\nTotal: 12.34"}}]}}` + "\n"))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()
	baseURL = server.URL

	provider := NewPaddleAIStudioProvider(PaddleAIStudioConfig{
		JobURL:          server.URL + "/jobs",
		Token:           token,
		Model:           "PaddleOCR-VL-1.6",
		OptionalPayload: `{"useDocOrientationClassify":false}`,
		Timeout:         time.Second,
		PollInterval:    time.Millisecond,
		Client:          server.Client(),
	})

	result, err := provider.Extract(context.Background(), model.Attachment{OriginalFilename: "receipt.png"}, strings.NewReader("fake-image"))
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if result.Provider != PaddleAIStudioProviderName {
		t.Fatalf("provider = %q", result.Provider)
	}
	if !strings.Contains(result.Text, "Total: 12.34") {
		t.Fatalf("text = %q", result.Text)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	if strings.Contains(string(encoded), token) {
		t.Fatalf("result leaked token: %s", encoded)
	}
}

func TestPaddleAIStudioProviderJobFailureRedactsToken(t *testing.T) {
	const token = "test-token-secret"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/jobs":
			_, _ = w.Write([]byte(`{"data":{"jobId":"job-123"}}`))
		case "/jobs/job-123":
			_, _ = w.Write([]byte(`{"data":{"state":"failed","errorMsg":"bad token test-token-secret"}}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	provider := NewPaddleAIStudioProvider(PaddleAIStudioConfig{
		JobURL:       server.URL + "/jobs",
		Token:        token,
		Model:        "PaddleOCR-VL-1.6",
		Timeout:      time.Second,
		PollInterval: time.Millisecond,
		Client:       server.Client(),
	})

	_, err := provider.Extract(context.Background(), model.Attachment{OriginalFilename: "receipt.png"}, strings.NewReader("fake-image"))
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), token) {
		t.Fatalf("error leaked token: %v", err)
	}
	if !strings.Contains(err.Error(), "[redacted]") {
		t.Fatalf("error should mention redaction, got %v", err)
	}
}

func TestPaddleAIStudioProviderTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/jobs":
			_, _ = w.Write([]byte(`{"data":{"jobId":"job-123"}}`))
		case "/jobs/job-123":
			_, _ = w.Write([]byte(`{"data":{"state":"running"}}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	provider := NewPaddleAIStudioProvider(PaddleAIStudioConfig{
		JobURL:       server.URL + "/jobs",
		Token:        "test-token",
		Model:        "PaddleOCR-VL-1.6",
		Timeout:      5 * time.Millisecond,
		PollInterval: time.Millisecond,
		Client:       server.Client(),
	})

	_, err := provider.Extract(context.Background(), model.Attachment{OriginalFilename: "receipt.png"}, strings.NewReader("fake-image"))
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "timed out") {
		t.Fatalf("expected timeout error, got %v", err)
	}
}

func TestPaddleAIStudioProviderUnavailableWhenMisconfigured(t *testing.T) {
	provider := NewPaddleAIStudioProvider(PaddleAIStudioConfig{Model: "PaddleOCR-VL-1.6"})
	if provider.Available(context.Background()) {
		t.Fatal("provider should be unavailable without token")
	}
	_, err := provider.Extract(context.Background(), model.Attachment{OriginalFilename: "receipt.png"}, strings.NewReader("fake-image"))
	if err == nil || !strings.Contains(err.Error(), "FINARCH_OCR_AISTUDIO_TOKEN") {
		t.Fatalf("expected missing token error, got %v", err)
	}
}

func TestPaddleAIStudioProviderRejectsMalformedJSONL(t *testing.T) {
	var baseURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/jobs":
			_, _ = w.Write([]byte(`{"data":{"jobId":"job-123"}}`))
		case "/jobs/job-123":
			_, _ = w.Write([]byte(fmt.Sprintf(`{"data":{"state":"done","resultUrl":{"jsonUrl":"%s/result.jsonl"}}}`, baseURL)))
		case "/result.jsonl":
			_, _ = w.Write([]byte(`not-json`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()
	baseURL = server.URL

	provider := NewPaddleAIStudioProvider(PaddleAIStudioConfig{
		JobURL:       server.URL + "/jobs",
		Token:        "test-token",
		Model:        "PaddleOCR-VL-1.6",
		Timeout:      time.Second,
		PollInterval: time.Millisecond,
		Client:       server.Client(),
	})

	_, err := provider.Extract(context.Background(), model.Attachment{OriginalFilename: "receipt.png"}, strings.NewReader("fake-image"))
	if err == nil || !strings.Contains(err.Error(), "JSONL") {
		t.Fatalf("expected JSONL parse error, got %v", err)
	}
}

func TestPaddleProviderSidecarCompatibility(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ocr" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("parse multipart: %v", err)
		}
		if _, _, err := r.FormFile("file"); err != nil {
			t.Fatalf("form file: %v", err)
		}
		if got := r.FormValue("lang"); got != "ch" {
			t.Fatalf("lang = %q", got)
		}
		_, _ = w.Write([]byte(`{"text":"hello","suggestion":{"amount_yuan":12.34,"currency":"cny"}}`))
	}))
	defer server.Close()

	provider := NewPaddleProvider(server.URL+"/ocr", "ch", time.Second)
	result, err := provider.Extract(context.Background(), model.Attachment{OriginalFilename: "receipt.png"}, strings.NewReader("fake-image"))
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if result.Provider != "paddle" {
		t.Fatalf("provider = %q", result.Provider)
	}
	if result.Text != "hello" {
		t.Fatalf("text = %q", result.Text)
	}
	if result.Suggestion.AmountYuan == nil || *result.Suggestion.AmountYuan != 12.34 {
		t.Fatalf("amount suggestion = %#v", result.Suggestion)
	}
	if result.Suggestion.Currency != "CNY" {
		t.Fatalf("currency = %q", result.Suggestion.Currency)
	}
}
