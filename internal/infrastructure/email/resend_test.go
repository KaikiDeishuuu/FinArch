package email

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type capturedResendRequest struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	HTML    string   `json:"html"`
}

func capturingSender(baseURL string, captured *capturedResendRequest) *ResendSender {
	return &ResendSender{
		apiKey:  "test-api-key",
		from:    "FinArch <test@example.com>",
		baseURL: baseURL,
		client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if err := json.NewDecoder(req.Body).Decode(captured); err != nil {
				return nil, fmt.Errorf("decode outbound request: %w", err)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{}`)),
				Request:    req,
			}, nil
		})},
	}
}

func TestActionLinkUsesEncodedFragmentAndNormalizesBaseURL(t *testing.T) {
	sender := &ResendSender{baseURL: " https://app.example/tenant path/?legacy=1#stale "}
	token := `token with spaces&next=evil#"'<>/%`

	got, err := sender.actionLink("/reset-password", token)
	if err != nil {
		t.Fatalf("actionLink() error = %v", err)
	}
	beforeFragment, rawFragment, found := strings.Cut(got, "#")
	if !found {
		t.Fatalf("actionLink() = %q, want fragment", got)
	}
	parsed, err := url.Parse(beforeFragment)
	if err != nil {
		t.Fatalf("parse action link: %v", err)
	}
	if parsed.Scheme != "https" || parsed.Host != "app.example" {
		t.Fatalf("actionLink() origin = %q://%q", parsed.Scheme, parsed.Host)
	}
	if parsed.Path != "/tenant path/reset-password" {
		t.Fatalf("actionLink() path = %q", parsed.Path)
	}
	if parsed.RawQuery != "" {
		t.Fatalf("actionLink() leaked base query %q", parsed.RawQuery)
	}
	values, err := url.ParseQuery(rawFragment)
	if err != nil {
		t.Fatalf("parse action fragment: %v", err)
	}
	if gotToken := values.Get("token"); gotToken != token {
		t.Fatalf("fragment token = %q, want %q", gotToken, token)
	}
	if strings.Contains(got, "?token=") || strings.Contains(got, token) {
		t.Fatalf("actionLink() did not isolate and encode token: %q", got)
	}
}

func TestActionLinkRejectsUnsafeOrRelativeBaseURL(t *testing.T) {
	for _, baseURL := range []string{
		"",
		"/relative",
		"javascript:alert(1)",
		"ftp://app.example",
		"https://[::1",
	} {
		t.Run(baseURL, func(t *testing.T) {
			sender := &ResendSender{baseURL: baseURL}
			if _, err := sender.actionLink("/verify-email", "token"); err == nil {
				t.Fatalf("actionLink(%q) unexpectedly succeeded", baseURL)
			}
		})
	}
}

func TestActionEmailBodiesEscapeUserContentAndUseFragmentTokens(t *testing.T) {
	const (
		baseURL       = "https://app.example/root/?discard=yes#stale"
		recipient     = "recipient@example.com"
		injectedName  = `Alice</strong><img id="name-injection" src=x><strong>`
		injectedEmail = `new@example.com</strong><img id="email-injection" src=x><strong>`
		injectedToken = `token&next=bad#"'<> /%`
	)

	tests := []struct {
		name      string
		route     string
		send      func(*ResendSender) error
		extraText string
	}{
		{
			name:  "verification",
			route: "/verify-email",
			send: func(sender *ResendSender) error {
				return sender.SendVerification(recipient, injectedName, injectedToken)
			},
		},
		{
			name:  "password reset",
			route: "/reset-password",
			send: func(sender *ResendSender) error {
				return sender.SendPasswordReset(recipient, injectedName, injectedToken)
			},
		},
		{
			name:  "account deletion",
			route: "/confirm-delete-account",
			send: func(sender *ResendSender) error {
				return sender.SendAccountDeletion(recipient, injectedName, injectedToken)
			},
		},
		{
			name:      "old email confirmation",
			route:     "/confirm-email-change-old",
			extraText: injectedEmail,
			send: func(sender *ResendSender) error {
				return sender.SendEmailChangeOldVerify(recipient, injectedName, injectedEmail, injectedToken)
			},
		},
		{
			name:  "new email confirmation",
			route: "/confirm-email-change",
			send: func(sender *ResendSender) error {
				return sender.SendEmailChange(recipient, injectedName, injectedToken)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var captured capturedResendRequest
			sender := capturingSender(baseURL, &captured)
			if err := test.send(sender); err != nil {
				t.Fatalf("send action email: %v", err)
			}
			if len(captured.To) != 1 || captured.To[0] != recipient {
				t.Fatalf("recipient = %#v", captured.To)
			}
			if strings.Contains(captured.HTML, injectedName) ||
				strings.Contains(captured.HTML, `<img id="name-injection"`) {
				t.Fatalf("email contains unescaped username: %s", captured.HTML)
			}
			if !strings.Contains(captured.HTML, escapeEmailText(injectedName)) {
				t.Fatalf("email does not contain escaped username")
			}
			if test.extraText != "" {
				if strings.Contains(captured.HTML, test.extraText) ||
					strings.Contains(captured.HTML, `<img id="email-injection"`) {
					t.Fatalf("email contains unescaped email address: %s", captured.HTML)
				}
				if !strings.Contains(captured.HTML, escapeEmailText(test.extraText)) {
					t.Fatalf("email does not contain escaped email address")
				}
			}

			link, err := sender.actionLink(test.route, injectedToken)
			if err != nil {
				t.Fatalf("build expected link: %v", err)
			}
			if strings.Contains(captured.HTML, "?token=") {
				t.Fatalf("email still exposes token in query string")
			}
			if !strings.Contains(captured.HTML, `href="`+escapeEmailAttribute(link)+`"`) {
				t.Fatalf("email does not contain escaped fragment href: %q", link)
			}
			if !strings.Contains(captured.HTML, ">"+escapeEmailText(link)+"</a>") {
				t.Fatalf("email does not contain escaped visible link: %q", link)
			}
		})
	}
}

func TestRestoreEmailBodyEscapesNameAndCode(t *testing.T) {
	const (
		injectedName = `Alice</strong><img id="name-injection" src=x><strong>`
		injectedCode = `123</span><img id="code-injection" src=x><span>`
	)
	var captured capturedResendRequest
	sender := capturingSender("https://app.example", &captured)
	if err := sender.SendRestoreCode("recipient@example.com", injectedName, injectedCode); err != nil {
		t.Fatalf("SendRestoreCode() error = %v", err)
	}
	for _, injected := range []string{injectedName, injectedCode} {
		if strings.Contains(captured.HTML, injected) {
			t.Fatalf("restore email contains raw injected content %q", injected)
		}
		if !strings.Contains(captured.HTML, escapeEmailText(injected)) {
			t.Fatalf("restore email does not contain escaped content %q", injected)
		}
	}
	if strings.Contains(captured.HTML, `<img id="code-injection"`) {
		t.Fatalf("restore email contains injected markup")
	}
}
