package ocr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"finarch/internal/domain/model"
)

const (
	PaddleAIStudioProviderName        = "paddle_aistudio"
	DefaultPaddleAIStudioJobURL       = "https://paddleocr.aistudio-app.com/api/v2/ocr/jobs"
	DefaultPaddleAIStudioPollInterval = 2 * time.Second
	DefaultPaddleAIStudioMaxResult    = int64(10 << 20) // 10 MiB
)

// PaddleAIStudioConfig configures the hosted PaddleOCR AIStudio job API.
type PaddleAIStudioConfig struct {
	JobURL             string
	Token              string
	Model              string
	OptionalPayload    string
	Timeout            time.Duration
	PollInterval       time.Duration
	MaxResultBytes     int64
	AllowedResultHosts []string
	Client             *http.Client
}

// PaddleAIStudioProvider calls PaddleOCR AIStudio's async OCR job API and
// normalizes the JSONL result into FinArch's synchronous OCR result shape.
type PaddleAIStudioProvider struct {
	jobURL                   string
	token                    string
	model                    string
	optionalPayload          string
	timeout                  time.Duration
	pollInterval             time.Duration
	maxResultBytes           int64
	client                   *http.Client
	configErr                string
	jobAuthority             string
	allowedResultAuthorities map[string]struct{}
}

func NewPaddleAIStudioProvider(cfg PaddleAIStudioConfig) *PaddleAIStudioProvider {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 45 * time.Second
	}
	pollInterval := cfg.PollInterval
	if pollInterval <= 0 {
		pollInterval = DefaultPaddleAIStudioPollInterval
	}
	maxResultBytes := cfg.MaxResultBytes
	if maxResultBytes <= 0 {
		maxResultBytes = DefaultPaddleAIStudioMaxResult
	}
	jobURL := strings.TrimSpace(cfg.JobURL)
	if jobURL == "" {
		jobURL = DefaultPaddleAIStudioJobURL
	}
	optionalPayload := strings.TrimSpace(cfg.OptionalPayload)
	configErr := ""
	if optionalPayload != "" {
		var payload any
		if err := json.Unmarshal([]byte(optionalPayload), &payload); err != nil {
			configErr = "FINARCH_OCR_AISTUDIO_OPTIONAL_PAYLOAD must be valid JSON"
		} else if _, ok := payload.(map[string]any); !ok {
			configErr = "FINARCH_OCR_AISTUDIO_OPTIONAL_PAYLOAD must be a JSON object"
		}
	}
	client := cfg.Client
	if client == nil {
		client = &http.Client{Timeout: timeout}
	}
	allowedResultAuthorities := make(map[string]struct{}, len(cfg.AllowedResultHosts)+1)
	jobAuthority := ""
	if parsed, err := url.Parse(jobURL); err == nil {
		jobAuthority = canonicalAuthority(parsed)
		if jobAuthority != "" {
			allowedResultAuthorities[jobAuthority] = struct{}{}
		}
	}
	for _, host := range cfg.AllowedResultHosts {
		host = strings.ToLower(strings.TrimSpace(host))
		if host != "" {
			allowedResultAuthorities[host] = struct{}{}
		}
	}
	return &PaddleAIStudioProvider{
		jobURL:                   jobURL,
		token:                    strings.TrimSpace(cfg.Token),
		model:                    strings.TrimSpace(cfg.Model),
		optionalPayload:          optionalPayload,
		timeout:                  timeout,
		pollInterval:             pollInterval,
		maxResultBytes:           maxResultBytes,
		client:                   client,
		configErr:                configErr,
		jobAuthority:             jobAuthority,
		allowedResultAuthorities: allowedResultAuthorities,
	}
}

func (p *PaddleAIStudioProvider) Name() string { return PaddleAIStudioProviderName }

func (p *PaddleAIStudioProvider) Available(context.Context) bool {
	return p != nil && p.validateConfig() == nil
}

func (p *PaddleAIStudioProvider) Extract(ctx context.Context, attachment model.Attachment, r io.Reader) (model.OCRResult, error) {
	if err := p.validateConfig(); err != nil {
		return model.OCRResult{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	jobID, err := p.submitJob(ctx, attachment, r)
	if err != nil {
		return model.OCRResult{}, err
	}
	jsonURL, pollCount, err := p.pollJob(ctx, jobID)
	if err != nil {
		return model.OCRResult{}, err
	}
	text, lineCount, blockCount, suggestion, err := p.downloadJSONL(ctx, jsonURL)
	if err != nil {
		return model.OCRResult{}, err
	}

	return model.OCRResult{
		Provider:   p.Name(),
		Text:       text,
		Suggestion: suggestion,
		Raw: map[string]any{
			"job_id":          jobID,
			"state":           "done",
			"poll_count":      pollCount,
			"jsonl_lines":     lineCount,
			"markdown_blocks": blockCount,
		},
	}, nil
}

func (p *PaddleAIStudioProvider) validateConfig() error {
	if p == nil {
		return fmt.Errorf("PaddleOCR AIStudio provider is not configured")
	}
	if p.configErr != "" {
		return errors.New(p.configErr)
	}
	if p.token == "" {
		return fmt.Errorf("FINARCH_OCR_AISTUDIO_TOKEN is required")
	}
	if p.model == "" {
		return fmt.Errorf("FINARCH_OCR_AISTUDIO_MODEL is required")
	}
	if p.jobURL == "" {
		return fmt.Errorf("FINARCH_OCR_AISTUDIO_JOB_URL is required")
	}
	parsed, err := url.Parse(p.jobURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("FINARCH_OCR_AISTUDIO_JOB_URL must be an absolute URL")
	}
	if err := validateAIStudioURL(parsed, canonicalAuthority(parsed), true); err != nil {
		return fmt.Errorf("invalid FINARCH_OCR_AISTUDIO_JOB_URL: %w", err)
	}
	return nil
}

func canonicalAuthority(parsed *url.URL) string {
	if parsed == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(parsed.Host))
}

func isLoopbackHost(host string) bool {
	host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func validateAIStudioURL(parsed *url.URL, requiredAuthority string, allowLoopbackHTTP bool) error {
	if parsed == nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("must be an absolute URL")
	}
	if parsed.User != nil {
		return fmt.Errorf("must not contain URL credentials")
	}
	if parsed.Fragment != "" {
		return fmt.Errorf("must not contain a fragment")
	}
	authority := canonicalAuthority(parsed)
	if requiredAuthority != "" && authority != strings.ToLower(strings.TrimSpace(requiredAuthority)) {
		return fmt.Errorf("host %q is not allowed", authority)
	}
	switch strings.ToLower(parsed.Scheme) {
	case "https":
		return nil
	case "http":
		if allowLoopbackHTTP && isLoopbackHost(parsed.Hostname()) {
			return nil
		}
		return fmt.Errorf("must use HTTPS (HTTP is allowed only for loopback development servers)")
	default:
		return fmt.Errorf("must use HTTPS")
	}
}

func (p *PaddleAIStudioProvider) validateResultURL(parsed *url.URL) error {
	authority := canonicalAuthority(parsed)
	if _, ok := p.allowedResultAuthorities[authority]; !ok {
		return fmt.Errorf("host %q is not in FINARCH_OCR_AISTUDIO_ALLOWED_RESULT_HOSTS", authority)
	}
	return validateAIStudioURL(parsed, authority, isLoopbackHost(parsed.Hostname()))
}

func (p *PaddleAIStudioProvider) do(req *http.Request, resultRequest bool) (*http.Response, error) {
	validate := func(candidate *url.URL) error {
		if resultRequest {
			return p.validateResultURL(candidate)
		}
		return validateAIStudioURL(candidate, p.jobAuthority, isLoopbackHost(candidate.Hostname()))
	}
	if err := validate(req.URL); err != nil {
		return nil, err
	}
	client := *p.client
	previousCheck := client.CheckRedirect
	client.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if err := validate(next.URL); err != nil {
			return err
		}
		if previousCheck != nil {
			return previousCheck(next, via)
		}
		if len(via) >= 10 {
			return fmt.Errorf("stopped after 10 redirects")
		}
		return nil
	}
	return client.Do(req)
}

func (p *PaddleAIStudioProvider) submitJob(ctx context.Context, attachment model.Attachment, r io.Reader) (string, error) {
	pipeReader, pipeWriter := io.Pipe()
	defer pipeReader.Close()
	mw := multipart.NewWriter(pipeWriter)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.jobURL, pipeReader)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "bearer "+p.token)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	go func() {
		writeErr := func() error {
			part, err := mw.CreateFormFile("file", attachment.OriginalFilename)
			if err != nil {
				return err
			}
			if _, err := io.Copy(part, r); err != nil {
				return err
			}
			if err := mw.WriteField("model", p.model); err != nil {
				return err
			}
			if p.optionalPayload != "" {
				if err := mw.WriteField("optionalPayload", p.optionalPayload); err != nil {
					return err
				}
			}
			return mw.Close()
		}()
		_ = pipeWriter.CloseWithError(writeErr)
	}()

	resp, err := p.do(req, false)
	if err != nil {
		_ = pipeReader.CloseWithError(err)
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("PaddleOCR AIStudio submit returned %s", resp.Status)
	}

	var payload struct {
		Data struct {
			JobID string `json:"jobId"`
		} `json:"data"`
	}
	if err := decodeLimitedJSONResponse(resp.Body, DefaultOCRMaxJSONResponseBytes, &payload); err != nil {
		return "", fmt.Errorf("decode PaddleOCR AIStudio submit response: %w", err)
	}
	jobID := strings.TrimSpace(payload.Data.JobID)
	if jobID == "" {
		return "", fmt.Errorf("PaddleOCR AIStudio submit response missing jobId")
	}
	return jobID, nil
}

func (p *PaddleAIStudioProvider) pollJob(ctx context.Context, jobID string) (string, int, error) {
	pollURL := strings.TrimRight(p.jobURL, "/") + "/" + url.PathEscape(jobID)
	pollCount := 0
	for {
		pollCount++
		state, jsonURL, errMsg, err := p.fetchJobState(ctx, pollURL)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return "", pollCount, fmt.Errorf("PaddleOCR AIStudio job timed out: %w", err)
			}
			return "", pollCount, err
		}
		switch state {
		case "done":
			if jsonURL == "" {
				return "", pollCount, fmt.Errorf("PaddleOCR AIStudio job completed without jsonUrl")
			}
			return jsonURL, pollCount, nil
		case "pending", "running":
			if err := sleepWithContext(ctx, p.pollInterval); err != nil {
				return "", pollCount, fmt.Errorf("PaddleOCR AIStudio job timed out: %w", err)
			}
		case "failed":
			if errMsg == "" {
				errMsg = "unknown error"
			}
			return "", pollCount, fmt.Errorf("PaddleOCR AIStudio job failed: %s", p.redact(errMsg))
		case "":
			return "", pollCount, fmt.Errorf("PaddleOCR AIStudio job status response missing state")
		default:
			return "", pollCount, fmt.Errorf("PaddleOCR AIStudio job returned unknown state %q", p.redact(state))
		}
	}
}

func (p *PaddleAIStudioProvider) fetchJobState(ctx context.Context, pollURL string) (string, string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pollURL, nil)
	if err != nil {
		return "", "", "", err
	}
	req.Header.Set("Authorization", "bearer "+p.token)
	resp, err := p.do(req, false)
	if err != nil {
		return "", "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", "", fmt.Errorf("PaddleOCR AIStudio poll returned %s", resp.Status)
	}

	var payload struct {
		Data struct {
			State     string `json:"state"`
			ErrorMsg  string `json:"errorMsg"`
			ResultURL struct {
				JSONURL string `json:"jsonUrl"`
			} `json:"resultUrl"`
		} `json:"data"`
		ErrorMsg string `json:"errorMsg"`
		Message  string `json:"message"`
	}
	if err := decodeLimitedJSONResponse(resp.Body, DefaultOCRMaxJSONResponseBytes, &payload); err != nil {
		return "", "", "", fmt.Errorf("decode PaddleOCR AIStudio poll response: %w", err)
	}
	errMsg := strings.TrimSpace(payload.Data.ErrorMsg)
	if errMsg == "" {
		errMsg = strings.TrimSpace(payload.ErrorMsg)
	}
	if errMsg == "" {
		errMsg = strings.TrimSpace(payload.Message)
	}
	return strings.ToLower(strings.TrimSpace(payload.Data.State)), strings.TrimSpace(payload.Data.ResultURL.JSONURL), errMsg, nil
}

func (p *PaddleAIStudioProvider) downloadJSONL(ctx context.Context, jsonURL string) (string, int, int, model.OCRSuggestion, error) {
	parsed, err := url.Parse(strings.TrimSpace(jsonURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", 0, 0, model.OCRSuggestion{}, fmt.Errorf("PaddleOCR AIStudio result jsonUrl is not an absolute URL")
	}
	if err := p.validateResultURL(parsed); err != nil {
		return "", 0, 0, model.OCRSuggestion{}, fmt.Errorf("PaddleOCR AIStudio result jsonUrl rejected: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return "", 0, 0, model.OCRSuggestion{}, err
	}
	resp, err := p.do(req, true)
	if err != nil {
		return "", 0, 0, model.OCRSuggestion{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", 0, 0, model.OCRSuggestion{}, fmt.Errorf("PaddleOCR AIStudio result download returned %s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, p.maxResultBytes+1))
	if err != nil {
		return "", 0, 0, model.OCRSuggestion{}, err
	}
	if int64(len(data)) > p.maxResultBytes {
		return "", 0, 0, model.OCRSuggestion{}, fmt.Errorf("PaddleOCR AIStudio result exceeds %d bytes", p.maxResultBytes)
	}
	return parseAIStudioJSONL(data)
}

func parseAIStudioJSONL(data []byte) (string, int, int, model.OCRSuggestion, error) {
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	var markdownBlocks []string
	var suggestion model.OCRSuggestion
	hasSuggestion := false
	lineCount := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lineCount++
		var raw map[string]any
		if err := decodeJSONUseNumber([]byte(line), &raw); err != nil {
			return "", lineCount, len(markdownBlocks), model.OCRSuggestion{}, fmt.Errorf("parse PaddleOCR AIStudio JSONL line %d: %w", lineCount, err)
		}
		if !hasSuggestion {
			suggestion = parseSuggestion(raw)
			hasSuggestion = suggestionHasData(suggestion)
		}
		result, _ := raw["result"].(map[string]any)
		if result != nil {
			if !hasSuggestion {
				suggestion = parseSuggestion(result)
				hasSuggestion = suggestionHasData(suggestion)
			}
			markdownBlocks = append(markdownBlocks, extractMarkdownBlocks(result)...)
		}
	}
	if len(markdownBlocks) == 0 {
		return "", lineCount, 0, suggestion, fmt.Errorf("PaddleOCR AIStudio result contained no markdown text")
	}
	return strings.Join(markdownBlocks, "\n\n"), lineCount, len(markdownBlocks), suggestion, nil
}

func extractMarkdownBlocks(result map[string]any) []string {
	layouts, _ := result["layoutParsingResults"].([]any)
	blocks := make([]string, 0, len(layouts))
	for _, item := range layouts {
		layout, _ := item.(map[string]any)
		if layout == nil {
			continue
		}
		markdown, _ := layout["markdown"].(map[string]any)
		if markdown == nil {
			continue
		}
		text, _ := markdown["text"].(string)
		text = strings.TrimSpace(text)
		if text != "" {
			blocks = append(blocks, text)
		}
	}
	return blocks
}

func suggestionHasData(s model.OCRSuggestion) bool {
	return s.AmountCents != nil ||
		s.AmountYuan != nil ||
		s.Currency != "" ||
		s.OccurredAt != "" ||
		s.Merchant != "" ||
		s.InvoiceNumber != "" ||
		s.Category != "" ||
		s.Note != "" ||
		s.Confidence != 0
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (p *PaddleAIStudioProvider) redact(value string) string {
	if p == nil || p.token == "" {
		return value
	}
	return strings.ReplaceAll(value, p.token, "[redacted]")
}
