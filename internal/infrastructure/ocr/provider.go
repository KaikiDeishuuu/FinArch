package ocr

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"finarch/internal/domain/model"
)

// NoneProvider keeps OCR optional when no engine is configured.
type NoneProvider struct{}

func (NoneProvider) Name() string                   { return "none" }
func (NoneProvider) Available(context.Context) bool { return false }
func (NoneProvider) Extract(context.Context, model.Attachment, io.Reader) (model.OCRResult, error) {
	return model.OCRResult{}, fmt.Errorf("OCR provider unavailable")
}

// PaddleProvider calls an HTTP PaddleOCR sidecar.
type PaddleProvider struct {
	url     string
	lang    string
	client  *http.Client
	timeout time.Duration
}

func NewPaddleProvider(url, lang string, timeout time.Duration) *PaddleProvider {
	if timeout <= 0 {
		timeout = 45 * time.Second
	}
	return &PaddleProvider{url: strings.TrimSpace(url), lang: strings.TrimSpace(lang), timeout: timeout, client: &http.Client{Timeout: timeout}}
}

func (p *PaddleProvider) Name() string                   { return "paddle" }
func (p *PaddleProvider) Available(context.Context) bool { return p != nil && p.url != "" }

func (p *PaddleProvider) Extract(ctx context.Context, attachment model.Attachment, r io.Reader) (model.OCRResult, error) {
	if !p.Available(ctx) {
		return model.OCRResult{}, fmt.Errorf("PaddleOCR URL is not configured")
	}
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("file", attachment.OriginalFilename)
	if err != nil {
		return model.OCRResult{}, err
	}
	if _, err := io.Copy(part, r); err != nil {
		return model.OCRResult{}, err
	}
	if p.lang != "" {
		_ = mw.WriteField("lang", p.lang)
	}
	if err := mw.Close(); err != nil {
		return model.OCRResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.url, &body)
	if err != nil {
		return model.OCRResult{}, err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := p.client.Do(req)
	if err != nil {
		return model.OCRResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return model.OCRResult{}, fmt.Errorf("PaddleOCR returned %s", resp.Status)
	}
	var raw map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return model.OCRResult{}, err
	}
	result := model.OCRResult{Provider: p.Name(), Raw: raw}
	if text, ok := raw["text"].(string); ok {
		result.Text = text
	}
	if suggestion, ok := raw["suggestion"].(map[string]any); ok {
		result.Suggestion = parseSuggestion(suggestion)
	} else {
		result.Suggestion = parseSuggestion(raw)
	}
	return result, nil
}

func parseSuggestion(raw map[string]any) model.OCRSuggestion {
	var out model.OCRSuggestion
	if v, ok := raw["amount_cents"].(float64); ok && v > 0 {
		cents := int64(v)
		out.AmountCents = &cents
		yuan := float64(cents) / 100
		out.AmountYuan = &yuan
	} else if v, ok := raw["amount_yuan"].(float64); ok && v > 0 {
		out.AmountYuan = &v
		cents := int64(v*100 + 0.5)
		out.AmountCents = &cents
	}
	if v, ok := raw["currency"].(string); ok {
		out.Currency = strings.ToUpper(strings.TrimSpace(v))
	}
	if v, ok := raw["occurred_at"].(string); ok {
		out.OccurredAt = v
	}
	if v, ok := raw["merchant"].(string); ok {
		out.Merchant = v
	}
	if v, ok := raw["invoice_number"].(string); ok {
		out.InvoiceNumber = v
	}
	if v, ok := raw["category"].(string); ok {
		out.Category = v
	}
	if v, ok := raw["note"].(string); ok {
		out.Note = v
	}
	if v, ok := raw["confidence"].(float64); ok {
		out.Confidence = v
	}
	return out
}
