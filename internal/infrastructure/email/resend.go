package email

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

// Sender is the interface for sending transactional emails.
type Sender interface {
	SendVerification(toEmail, toName, token string) error
	SendPasswordReset(toEmail, toName, token string) error
}

// ResendSender sends emails via the Resend API.
type ResendSender struct {
	apiKey  string
	from    string
	baseURL string // base URL of the app, used to build links
}

// NewResendSender returns a ResendSender. If apiKey is empty it returns a NoopSender.
func NewResendSender(apiKey, from, baseURL string) Sender {
	if apiKey == "" {
		return &NoopSender{}
	}
	if from == "" {
		from = "FinArch <noreply@finarch.app>"
	}
	return &ResendSender{apiKey: apiKey, from: from, baseURL: baseURL}
}

func (s *ResendSender) send(to, subject, html string) error {
	body, _ := json.Marshal(map[string]any{
		"from":    s.from,
		"to":      []string{to},
		"subject": subject,
		"html":    html,
	})
	req, err := http.NewRequest(http.MethodPost, "https://api.resend.com/emails", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("resend: create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("resend: send: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("resend: API returned %d", resp.StatusCode)
	}
	return nil
}

func (s *ResendSender) SendVerification(toEmail, toName, token string) error {
	link := s.baseURL + "/verify-email?token=" + token
	html := fmt.Sprintf(`
<div style="font-family:sans-serif;max-width:480px;margin:0 auto;padding:24px">
  <h2 style="color:#1d4ed8">验证您的 FinArch 账号</h2>
  <p>您好，%s，</p>
  <p>请点击下方按钮完成邮箱验证，链接有效期 <strong>24 小时</strong>。</p>
  <p style="margin:24px 0">
    <a href="%s" style="background:#2563eb;color:#fff;padding:12px 24px;border-radius:8px;text-decoration:none;font-weight:600">验证邮箱</a>
  </p>
  <p style="color:#6b7280;font-size:12px">如果您没有注册 FinArch，请忽略此邮件。</p>
  <p style="color:#9ca3af;font-size:11px">%s</p>
</div>`, toName, link, link)
	return s.send(toEmail, "验证您的 FinArch 账号", html)
}

func (s *ResendSender) SendPasswordReset(toEmail, toName, token string) error {
	link := s.baseURL + "/reset-password?token=" + token
	html := fmt.Sprintf(`
<div style="font-family:sans-serif;max-width:480px;margin:0 auto;padding:24px">
  <h2 style="color:#1d4ed8">重置 FinArch 密码</h2>
  <p>您好，%s，</p>
  <p>我们收到了您的密码重置请求。请点击下方按钮设置新密码，链接有效期 <strong>1 小时</strong>。</p>
  <p style="margin:24px 0">
    <a href="%s" style="background:#2563eb;color:#fff;padding:12px 24px;border-radius:8px;text-decoration:none;font-weight:600">重置密码</a>
  </p>
  <p style="color:#6b7280;font-size:12px">如果您没有发起此请求，请忽略此邮件，您的密码不会被更改。</p>
  <p style="color:#9ca3af;font-size:11px">%s</p>
</div>`, toName, link, link)
	return s.send(toEmail, "重置您的 FinArch 密码", html)
}

// NoopSender discards all emails (used when RESEND_API_KEY is not set).
type NoopSender struct{}

func (n *NoopSender) SendVerification(_, _, _ string) error  { return nil }
func (n *NoopSender) SendPasswordReset(_, _, _ string) error { return nil }

// IsConfigured returns true if RESEND_API_KEY env var is set.
func IsConfigured() bool { return os.Getenv("RESEND_API_KEY") != "" }
