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
	SendAccountDeletion(toEmail, toName, token string) error
	// SendEmailChangeOldVerify sends an authorization request to the OLD email address.
	// The user must click this link before the system will send a verification to the new email.
	SendEmailChangeOldVerify(toOldEmail, toUsername, newEmail, token string) error
	// SendEmailChange sends a verification link to the NEW email address to complete the change.
	SendEmailChange(toNewEmail, toUsername, token string) error
	// SendRestoreCode sends a 6-digit verification code for disaster recovery restore.
	SendRestoreCode(toEmail, toName, code string) error
}

// ResendSender sends emails via the Resend API.
type ResendSender struct {
	apiKey  string
	from    string
	baseURL string // base URL of the app, used to build links
}

// NewResendSender returns a ResendSender. If apiKey is empty it returns a NoopSender.
// Logo URL is read from the EMAIL_LOGO_URL environment variable.
func NewResendSender(apiKey, from, baseURL string) Sender {
	if apiKey == "" {
		return &NoopSender{}
	}
	if from == "" {
		from = "FinArch <hello@farc.dev>"
	}
	return &ResendSender{
		apiKey:  apiKey,
		from:    from,
		baseURL: baseURL,
	}
}

// buildEmailHTML wraps body content in a consistent email shell.
// The header contains an inline HTML bar-chart logo (email-client safe, no SVG/image needed).
func buildEmailHTML(_, bodyHTML string) string {
	// Inline 3-bar logo: indigo→violet→emerald, bottom-aligned via vertical-align
	inlineLogo := `
          <table cellpadding="0" cellspacing="0" role="presentation" style="display:inline-table;margin-right:12px;vertical-align:middle">
            <tr>
              <td style="width:9px;height:16px;background:#818cf8;border-radius:3px 3px 0 0;vertical-align:bottom"></td>
              <td style="width:5px"></td>
              <td style="width:9px;height:24px;background:#a78bfa;border-radius:3px 3px 0 0;vertical-align:bottom"></td>
              <td style="width:5px"></td>
              <td style="width:9px;height:34px;background:#34d399;border-radius:3px 3px 0 0;vertical-align:bottom"></td>
            </tr>
          </table>
          <span style="font-size:22px;font-weight:800;color:#ffffff;vertical-align:middle;letter-spacing:-0.3px">FinArch</span>`

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="zh-CN">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width,initial-scale=1">
  <title>FinArch</title>
</head>
<body style="margin:0;padding:0;background-color:#f0f2f5;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,'Helvetica Neue',Arial,sans-serif">
  <table width="100%%" cellpadding="0" cellspacing="0" role="presentation" style="background-color:#f0f2f5;padding:40px 16px">
    <tr>
      <td align="center">
        <table width="560" cellpadding="0" cellspacing="0" role="presentation" style="max-width:560px;width:100%%">

          <!-- ── Header ── -->
          <tr>
            <td align="center" style="background:linear-gradient(135deg,#0d0b14 0%%,#170f26 100%%);border-radius:14px 14px 0 0;padding:28px 40px 24px">
              %s
            </td>
          </tr>

          <!-- ── Accent bar ── -->
          <tr>
            <td style="height:4px;background:linear-gradient(90deg,#818cf8,#a78bfa,#34d399)"></td>
          </tr>

          <!-- ── Body ── -->
          <tr>
            <td style="background:#ffffff;padding:40px 40px 36px;border-left:1px solid #e5e7eb;border-right:1px solid #e5e7eb">
              %s
            </td>
          </tr>

          <!-- ── Footer ── -->
          <tr>
            <td align="center" style="background:#f9fafb;border:1px solid #e5e7eb;border-top:none;border-radius:0 0 14px 14px;padding:20px 40px">
              <p style="margin:0;color:#9ca3af;font-size:12px;line-height:1.8">
                此邮件由 <strong style="color:#6b7280">FinArch</strong> 系统自动发送，请勿直接回复。<br>
                &copy; %d FinArch
              </p>
            </td>
          </tr>

        </table>
      </td>
    </tr>
  </table>
</body>
</html>`, inlineLogo, bodyHTML, 2026)
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
	body := fmt.Sprintf(`
      <h1 style="margin:0 0 6px;font-size:22px;font-weight:700;color:#111827">验证您的邮箱地址</h1>
      <p style="margin:0 0 28px;color:#6b7280;font-size:14px">完成注册，激活您的 FinArch 账号</p>
      <p style="margin:0 0 12px;color:#374151;font-size:15px">您好，<strong>%s</strong>，</p>
      <p style="margin:0 0 32px;color:#374151;font-size:15px;line-height:1.7">
        感谢您注册 FinArch！请点击下方按钮完成邮箱验证，<br>验证链接有效期为 <strong>24 小时</strong>。
      </p>
      <table cellpadding="0" cellspacing="0" role="presentation" style="margin:0 0 32px">
        <tr>
          <td style="border-radius:8px;background:#22c55e">
            <a href="%s" style="display:inline-block;padding:14px 36px;color:#ffffff;font-size:15px;font-weight:600;text-decoration:none;letter-spacing:0.1px">✓ &nbsp;立即验证邮箱</a>
          </td>
        </tr>
      </table>
      <p style="margin:0 0 8px;color:#6b7280;font-size:13px">按钮无法点击？请复制以下链接到浏览器：</p>
      <p style="margin:0 0 24px;word-break:break-all">
        <a href="%s" style="color:#2563eb;font-size:12px;text-decoration:none">%s</a>
      </p>
      <hr style="border:none;border-top:1px solid #e5e7eb;margin:24px 0">
      <p style="margin:0;color:#9ca3af;font-size:12px">如果您没有注册 FinArch 账号，请忽略此邮件，无需进行任何操作。</p>`,
		toName, link, link, link)
	html := buildEmailHTML("", body)
	return s.send(toEmail, "验证您的 FinArch 邮箱地址", html)
}

func (s *ResendSender) SendPasswordReset(toEmail, toName, token string) error {
	link := s.baseURL + "/reset-password?token=" + token
	body := fmt.Sprintf(`
      <h1 style="margin:0 0 6px;font-size:22px;font-weight:700;color:#111827">重置您的密码</h1>
      <p style="margin:0 0 28px;color:#6b7280;font-size:14px">我们收到了您的密码重置申请</p>
      <p style="margin:0 0 12px;color:#374151;font-size:15px">您好，<strong>%s</strong>，</p>
      <p style="margin:0 0 32px;color:#374151;font-size:15px;line-height:1.7">
        请点击下方按钮设置新密码，链接有效期为 <strong>1 小时</strong>。<br>
        过期后需要重新发起重置请求。
      </p>
      <table cellpadding="0" cellspacing="0" role="presentation" style="margin:0 0 32px">
        <tr>
          <td style="border-radius:8px;background:#f97316">
            <a href="%s" style="display:inline-block;padding:14px 36px;color:#ffffff;font-size:15px;font-weight:600;text-decoration:none;letter-spacing:0.1px">→ &nbsp;重置我的密码</a>
          </td>
        </tr>
      </table>
      <p style="margin:0 0 8px;color:#6b7280;font-size:13px">按钮无法点击？请复制以下链接到浏览器：</p>
      <p style="margin:0 0 24px;word-break:break-all">
        <a href="%s" style="color:#2563eb;font-size:12px;text-decoration:none">%s</a>
      </p>
      <hr style="border:none;border-top:1px solid #e5e7eb;margin:24px 0">
      <p style="margin:0;color:#9ca3af;font-size:12px">如果您没有发起此请求，请忽略此邮件，您的密码不会被更改。为了账户安全，请勿将此链接分享给任何人。</p>`,
		toName, link, link, link)
	html := buildEmailHTML("", body)
	return s.send(toEmail, "FinArch 密码重置申请", html)
}

func (s *ResendSender) SendAccountDeletion(toEmail, toName, token string) error {
	link := s.baseURL + "/confirm-delete-account?token=" + token
	body := fmt.Sprintf(`
      <h1 style="margin:0 0 6px;font-size:22px;font-weight:700;color:#111827">确认注销您的账户</h1>
      <p style="margin:0 0 28px;color:#6b7280;font-size:14px">此操作不可撤销，请谨慎确认</p>
      <p style="margin:0 0 12px;color:#374151;font-size:15px">您好，<strong>%s</strong>，</p>
      <p style="margin:0 0 32px;color:#374151;font-size:15px;line-height:1.7">
        我们收到了您的账户注销申请。点击下方按钮将<strong>永久删除</strong>您的账户及所有数据（包括标签、资金池、交易记录），<strong>此操作不可撤销</strong>。<br>
        链接有效期为 <strong>30 分钟</strong>。
      </p>
      <table cellpadding="0" cellspacing="0" role="presentation" style="margin:0 0 32px">
        <tr>
          <td style="border-radius:8px;background:#dc2626">
            <a href="%s" style="display:inline-block;padding:14px 36px;color:#ffffff;font-size:15px;font-weight:600;text-decoration:none;letter-spacing:0.1px">确认注销账户</a>
          </td>
        </tr>
      </table>
      <p style="margin:0 0 8px;color:#6b7280;font-size:13px">按钮无法点击？请复制以下链接到浏览器：</p>
      <p style="margin:0 0 24px;word-break:break-all">
        <a href="%s" style="color:#2563eb;font-size:12px;text-decoration:none">%s</a>
      </p>
      <hr style="border:none;border-top:1px solid #e5e7eb;margin:24px 0">
      <p style="margin:0;color:#9ca3af;font-size:12px">如果您没有发起此请求，请忽略此邮件并立即修改密码以保护账户安全。</p>`,
		toName, link, link, link)
	html := buildEmailHTML("", body)
	return s.send(toEmail, "⚠️ 确认注销您的 FinArch 账户", html)
}

func (s *ResendSender) SendEmailChangeOldVerify(toOldEmail, toUsername, newEmail, token string) error {
	link := s.baseURL + "/confirm-email-change-old?token=" + token
	body := fmt.Sprintf(`
      <h1 style="margin:0 0 6px;font-size:22px;font-weight:700;color:#111827">授权更换登录邮箱</h1>
      <p style="margin:0 0 28px;color:#6b7280;font-size:14px">请在您的当前邮箱确认此次变更请求</p>
      <p style="margin:0 0 12px;color:#374151;font-size:15px">您好，<strong>%s</strong>，</p>
      <p style="margin:0 0 28px;color:#374151;font-size:15px;line-height:1.7">
        我们收到了将您的登录邮箱更换为 <strong style="color:#2563eb">%s</strong> 的申请。<br>
        请点击下方按钮确认您本人发起了此次更换，系统随后将向新邮箱发送二次验证邮件。<br>
        链接有效期为 <strong>1 小时</strong>。
      </p>
      <table cellpadding="0" cellspacing="0" role="presentation" style="margin:0 0 32px">
        <tr>
          <td style="border-radius:8px;background:#2563eb">
            <a href="%s" style="display:inline-block;padding:14px 36px;color:#ffffff;font-size:15px;font-weight:600;text-decoration:none;letter-spacing:0.1px">✓ &nbsp;确认，发送新邮箱验证</a>
          </td>
        </tr>
      </table>
      <p style="margin:0 0 8px;color:#6b7280;font-size:13px">按钮无法点击？请复制以下链接到浏览器：</p>
      <p style="margin:0 0 24px;word-break:break-all">
        <a href="%s" style="color:#2563eb;font-size:12px;text-decoration:none">%s</a>
      </p>
      <hr style="border:none;border-top:1px solid #e5e7eb;margin:24px 0">
      <p style="margin:0;color:#9ca3af;font-size:12px">如果您没有发起此请求，请忽略此邮件并立即修改密码以保护账户安全。</p>`,
		toUsername, newEmail, link, link, link)
	html := buildEmailHTML("", body)
	return s.send(toOldEmail, "FinArch 登录邮箱变更授权", html)
}

func (s *ResendSender) SendEmailChange(toNewEmail, toUsername, token string) error {
	link := s.baseURL + "/confirm-email-change?token=" + token
	body := fmt.Sprintf(`
      <h1 style="margin:0 0 6px;font-size:22px;font-weight:700;color:#111827">验证您的新邮箱</h1>
      <p style="margin:0 0 28px;color:#6b7280;font-size:14px">您申请更换登录邮箱</p>
      <p style="margin:0 0 12px;color:#374151;font-size:15px">您好，<strong>%s</strong>，</p>
      <p style="margin:0 0 32px;color:#374151;font-size:15px;line-height:1.7">
        请点击下方按钮将此邮箱地址设为您的新登录邮箱。链接有效期为 <strong>1 小时</strong>。<br>
        未验证前，您的原邮箱仍可正常使用。
      </p>
      <table cellpadding="0" cellspacing="0" role="presentation" style="margin:0 0 32px">
        <tr>
          <td style="border-radius:8px;background:#2563eb">
            <a href="%s" style="display:inline-block;padding:14px 36px;color:#ffffff;font-size:15px;font-weight:600;text-decoration:none;letter-spacing:0.1px">验证新邮箱</a>
          </td>
        </tr>
      </table>
      <p style="margin:0 0 8px;color:#6b7280;font-size:13px">按钮无法点击？请复制以下链接到浏览器：</p>
      <p style="margin:0 0 24px;word-break:break-all">
        <a href="%s" style="color:#2563eb;font-size:12px;text-decoration:none">%s</a>
      </p>
      <hr style="border:none;border-top:1px solid #e5e7eb;margin:24px 0">
      <p style="margin:0;color:#9ca3af;font-size:12px">如果您没有发起此请求，请忽略此邮件并尽快修改密码以保护账户安全。</p>`,
		toUsername, link, link, link)
	html := buildEmailHTML("", body)
	return s.send(toNewEmail, "FinArch 登录邮箱验证", html)
}

func (s *ResendSender) SendRestoreCode(toEmail, toName, code string) error {
	body := fmt.Sprintf(`
      <h1 style="margin:0 0 6px;font-size:22px;font-weight:700;color:#111827">灾难恢复验证</h1>
      <p style="margin:0 0 28px;color:#6b7280;font-size:14px">有人正在尝试恢复与您邮箱关联的 FinArch 数据</p>
      <p style="margin:0 0 12px;color:#374151;font-size:15px">您好，<strong>%s</strong>，</p>
      <p style="margin:0 0 28px;color:#374151;font-size:15px;line-height:1.7">
        系统收到了一个数据恢复请求，请使用以下验证码完成身份验证。<br>
        验证码有效期为 <strong>10 分钟</strong>。
      </p>
      <table cellpadding="0" cellspacing="0" role="presentation" style="margin:0 0 32px">
        <tr>
          <td style="border-radius:12px;background:#f3f4f6;padding:20px 40px;text-align:center">
            <span style="font-size:36px;font-weight:800;color:#111827;letter-spacing:8px;font-family:monospace">%s</span>
          </td>
        </tr>
      </table>
      <hr style="border:none;border-top:1px solid #e5e7eb;margin:24px 0">
      <p style="margin:0;color:#9ca3af;font-size:12px">如果您没有发起此请求，请忽略此邮件。请勿将验证码分享给任何人。</p>`,
		toName, code)
	html := buildEmailHTML("", body)
	return s.send(toEmail, "FinArch 灾难恢复验证码", html)
}

// NoopSender discards all emails (used when RESEND_API_KEY is not set).
type NoopSender struct{}

func (n *NoopSender) SendVerification(_, _, _ string) error            { return nil }
func (n *NoopSender) SendPasswordReset(_, _, _ string) error           { return nil }
func (n *NoopSender) SendAccountDeletion(_, _, _ string) error         { return nil }
func (n *NoopSender) SendEmailChangeOldVerify(_, _, _, _ string) error { return nil }
func (n *NoopSender) SendEmailChange(_, _, _ string) error             { return nil }
func (n *NoopSender) SendRestoreCode(_, _, _ string) error             { return nil }

// IsConfigured returns true if RESEND_API_KEY env var is set.
func IsConfigured() bool { return os.Getenv("RESEND_API_KEY") != "" }
