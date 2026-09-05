package ocr

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"

	"finarch/internal/domain/model"
)

// DefaultOCRMaxJSONResponseBytes limits small provider control/sidecar JSON
// responses independently from the larger AIStudio JSONL result artifact.
const DefaultOCRMaxJSONResponseBytes = int64(1 << 20) // 1 MiB

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
	if err := decodeLimitedJSONResponse(resp.Body, DefaultOCRMaxJSONResponseBytes, &raw); err != nil {
		return model.OCRResult{}, fmt.Errorf("decode PaddleOCR response: %w", err)
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
	if cents, ok := jsonInteger(raw["amount_cents"]); ok && cents > 0 {
		out.AmountCents = &cents
		yuan := float64(cents) / 100
		out.AmountYuan = &yuan
	} else if v, ok := jsonFloat(raw["amount_yuan"]); ok && v > 0 {
		if cents, err := model.Money(v).Cents(); err == nil && cents > 0 {
			out.AmountYuan = &v
			out.AmountCents = &cents
		}
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
	if v, ok := jsonFloat(raw["confidence"]); ok {
		out.Confidence = v
	}
	return out
}

func jsonInteger(value any) (int64, bool) {
	switch number := value.(type) {
	case json.Number:
		parsed, err := strconv.ParseInt(number.String(), 10, 64)
		return parsed, err == nil
	case float64:
		// Support direct in-process provider maps, but reject values beyond the
		// exact integer range of float64 instead of silently changing cents.
		if math.IsNaN(number) || math.IsInf(number, 0) || number != math.Trunc(number) || math.Abs(number) > 9007199254740991 {
			return 0, false
		}
		return int64(number), true
	default:
		return 0, false
	}
}

func jsonFloat(value any) (float64, bool) {
	switch number := value.(type) {
	case json.Number:
		parsed, err := number.Float64()
		return parsed, err == nil && !math.IsNaN(parsed) && !math.IsInf(parsed, 0)
	case float64:
		return number, !math.IsNaN(number) && !math.IsInf(number, 0)
	default:
		return 0, false
	}
}

func decodeLimitedJSONResponse(r io.Reader, maxBytes int64, dst any) error {
	if maxBytes <= 0 {
		maxBytes = DefaultOCRMaxJSONResponseBytes
	}
	data, err := io.ReadAll(io.LimitReader(r, maxBytes+1))
	if err != nil {
		return err
	}
	if int64(len(data)) > maxBytes {
		return fmt.Errorf("OCR JSON response exceeds %d bytes", maxBytes)
	}
	return decodeJSONUseNumber(data, dst)
}

func decodeJSONUseNumber(data []byte, dst any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("OCR JSON response contains multiple values")
		}
		return err
	}
	return nil
}
