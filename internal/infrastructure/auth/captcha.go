package auth

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const turnstileVerifyURL = "https://challenges.cloudflare.com/turnstile/v0/siteverify"

// TurnstileVerifier verifies Cloudflare Turnstile challenge tokens.
// When secret is empty it is a no-op (useful for local development).
type TurnstileVerifier struct {
	secret string
	client *http.Client
}

// NewTurnstileVerifier creates a verifier using the given site secret.
// Pass an empty string to disable verification (development mode).
func NewTurnstileVerifier(secret string) *TurnstileVerifier {
	return &TurnstileVerifier{
		secret: secret,
		client: &http.Client{Timeout: 5 * time.Second},
	}
}

// Enabled reports whether the verifier is active.
func (v *TurnstileVerifier) Enabled() bool {
	return v.secret != ""
}

// Verify checks a Turnstile token against Cloudflare's siteverify API.
// remoteIP is optional; pass an empty string to omit it.
func (v *TurnstileVerifier) Verify(token, remoteIP string) error {
	if v.secret == "" {
		// Verification disabled – allow all requests (dev/test mode).
		return nil
	}
	if token == "" {
		return fmt.Errorf("captcha token is missing")
	}

	form := url.Values{}
	form.Set("secret", v.secret)
	form.Set("response", token)
	if remoteIP != "" {
		form.Set("remoteip", remoteIP)
	}

	resp, err := v.client.Post(turnstileVerifyURL, "application/x-www-form-urlencoded",
		strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("captcha verify request failed: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Success bool     `json:"success"`
		Errors  []string `json:"error-codes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("captcha response decode error: %w", err)
	}
	if !result.Success {
		return fmt.Errorf("captcha verification failed: %v", result.Errors)
	}
	return nil
}
