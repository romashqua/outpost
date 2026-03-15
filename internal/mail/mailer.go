package mail

import (
	"context"
	"crypto/tls"
	"fmt"
	"html/template"
	"log/slog"
	"net"
	"net/smtp"
	"strings"
)

// Config holds SMTP connection settings for the mailer.
type Config struct {
	SMTPHost    string
	SMTPPort    int
	FromAddress string
	FromName    string
	Username    string
	Password    string
	TLS         bool
}

// Mailer sends email notifications via SMTP.
type Mailer struct {
	cfg    Config
	logger *slog.Logger
}

// NewMailer creates a Mailer with the given SMTP configuration and logger.
func NewMailer(cfg Config, logger *slog.Logger) *Mailer {
	return &Mailer{cfg: cfg, logger: logger}
}

// Send delivers an email with the given HTML body to the specified recipient.
func (m *Mailer) Send(ctx context.Context, to, subject, htmlBody string) error {
	addr := net.JoinHostPort(m.cfg.SMTPHost, fmt.Sprintf("%d", m.cfg.SMTPPort))

	from := m.cfg.FromAddress
	if m.cfg.FromName != "" {
		from = fmt.Sprintf("%s <%s>", m.cfg.FromName, m.cfg.FromAddress)
	}

	headers := strings.Join([]string{
		"From: " + from,
		"To: " + to,
		"Subject: " + subject,
		"MIME-Version: 1.0",
		"Content-Type: text/html; charset=UTF-8",
	}, "\r\n")

	msg := []byte(headers + "\r\n\r\n" + htmlBody)

	var auth smtp.Auth
	if m.cfg.Username != "" {
		auth = smtp.PlainAuth("", m.cfg.Username, m.cfg.Password, m.cfg.SMTPHost)
	}

	if m.cfg.TLS {
		return m.sendTLS(addr, auth, m.cfg.FromAddress, to, msg)
	}

	if err := smtp.SendMail(addr, auth, m.cfg.FromAddress, []string{to}, msg); err != nil {
		m.logger.ErrorContext(ctx, "failed to send email", "to", to, "error", err)
		return fmt.Errorf("sending email to %s: %w", to, err)
	}

	m.logger.InfoContext(ctx, "email sent", "to", to, "subject", subject)
	return nil
}

// sendTLS connects to the SMTP server over TLS and sends the message.
func (m *Mailer) sendTLS(addr string, auth smtp.Auth, from, to string, msg []byte) error {
	tlsCfg := &tls.Config{ServerName: m.cfg.SMTPHost}
	conn, err := tls.Dial("tcp", addr, tlsCfg)
	if err != nil {
		return fmt.Errorf("TLS dial %s: %w", addr, err)
	}

	client, err := smtp.NewClient(conn, m.cfg.SMTPHost)
	if err != nil {
		conn.Close()
		return fmt.Errorf("SMTP client: %w", err)
	}
	defer client.Close()

	if auth != nil {
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("SMTP auth: %w", err)
		}
	}
	if err := client.Mail(from); err != nil {
		return fmt.Errorf("SMTP MAIL FROM: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("SMTP RCPT TO: %w", err)
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("SMTP DATA: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("writing message: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("closing data writer: %w", err)
	}
	return client.Quit()
}

// renderTemplate parses and executes the named template with the given data,
// returning the resulting HTML string.
func renderTemplate(name, tmplStr string, data any) (string, error) {
	t, err := template.New(name).Parse(tmplStr)
	if err != nil {
		return "", fmt.Errorf("parsing template %s: %w", name, err)
	}
	var buf strings.Builder
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("executing template %s: %w", name, err)
	}
	return buf.String(), nil
}

// SendMFACode sends a formatted MFA verification email with the given code.
func (m *Mailer) SendMFACode(ctx context.Context, to, code string) error {
	body, err := renderTemplate("mfa", mfaCodeTemplate, map[string]string{
		"Code": code,
	})
	if err != nil {
		return err
	}
	return m.Send(ctx, to, "Your verification code — Outpost VPN", body)
}

// SendEnrollmentInvite sends a device enrollment invitation email.
func (m *Mailer) SendEnrollmentInvite(ctx context.Context, to, enrollURL, instanceName string) error {
	body, err := renderTemplate("enroll", enrollmentInviteTemplate, map[string]string{
		"EnrollURL":    enrollURL,
		"InstanceName": instanceName,
	})
	if err != nil {
		return err
	}
	return m.Send(ctx, to, "Device enrollment invitation — "+instanceName, body)
}

// SendPasswordReset sends a password reset email with a link.
func (m *Mailer) SendPasswordReset(ctx context.Context, to, resetURL string) error {
	body, err := renderTemplate("reset", passwordResetTemplate, map[string]string{
		"ResetURL": resetURL,
	})
	if err != nil {
		return err
	}
	return m.Send(ctx, to, "Password reset — Outpost VPN", body)
}

// SendWelcome sends a welcome email to a new user.
func (m *Mailer) SendWelcome(ctx context.Context, to, username, instanceName string) error {
	body, err := renderTemplate("welcome", welcomeTemplate, map[string]string{
		"Username":     username,
		"InstanceName": instanceName,
	})
	if err != nil {
		return err
	}
	return m.Send(ctx, to, "Welcome to "+instanceName, body)
}

// SendDeviceConfig sends a WireGuard configuration to the user's email.
// The config is included as a preformatted block that can be copy-pasted
// into any WireGuard client.
func (m *Mailer) SendDeviceConfig(ctx context.Context, to, deviceName, configText string) error {
	body, err := renderTemplate("device_config", deviceConfigTemplate, map[string]string{
		"DeviceName": deviceName,
		"Config":     configText,
	})
	if err != nil {
		return err
	}
	return m.Send(ctx, to, "WireGuard configuration for "+deviceName+" — Outpost VPN", body)
}

// ---- Embedded HTML Templates ------------------------------------------------

const baseStyle = `
<style>
  body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; margin: 0; padding: 0; background: #f4f6f9; }
  .container { max-width: 560px; margin: 40px auto; background: #ffffff; border-radius: 8px; padding: 40px; box-shadow: 0 2px 8px rgba(0,0,0,0.06); }
  h1 { color: #1a1a2e; font-size: 22px; margin-top: 0; }
  p { color: #4a4a68; font-size: 15px; line-height: 1.6; }
  .code { display: inline-block; font-size: 32px; font-weight: 700; letter-spacing: 6px; color: #1a1a2e; background: #f0f2f5; padding: 12px 24px; border-radius: 6px; margin: 16px 0; }
  .btn { display: inline-block; background: #3b82f6; color: #ffffff; text-decoration: none; padding: 12px 28px; border-radius: 6px; font-weight: 600; font-size: 15px; margin: 16px 0; }
  .footer { margin-top: 32px; font-size: 12px; color: #9ca3af; }
</style>`

const mfaCodeTemplate = `<!DOCTYPE html>
<html><head><meta charset="UTF-8">` + baseStyle + `</head><body>
<div class="container">
  <h1>Verification Code</h1>
  <p>Use the following code to complete your sign-in. This code expires in 10 minutes.</p>
  <div class="code">{{.Code}}</div>
  <p>If you did not request this code, you can safely ignore this email.</p>
  <div class="footer">Outpost VPN</div>
</div>
</body></html>`

const enrollmentInviteTemplate = `<!DOCTYPE html>
<html><head><meta charset="UTF-8">` + baseStyle + `</head><body>
<div class="container">
  <h1>Device Enrollment</h1>
  <p>You have been invited to enroll a new device on <strong>{{.InstanceName}}</strong>.</p>
  <p>Click the button below to begin the enrollment process:</p>
  <a href="{{.EnrollURL}}" class="btn">Enroll Device</a>
  <p>Or copy this link into your browser:</p>
  <p style="word-break: break-all; font-size: 13px; color: #6b7280;">{{.EnrollURL}}</p>
  <div class="footer">Outpost VPN</div>
</div>
</body></html>`

const passwordResetTemplate = `<!DOCTYPE html>
<html><head><meta charset="UTF-8">` + baseStyle + `</head><body>
<div class="container">
  <h1>Password Reset</h1>
  <p>We received a request to reset your password. Click the button below to choose a new password:</p>
  <a href="{{.ResetURL}}" class="btn">Reset Password</a>
  <p>Or copy this link into your browser:</p>
  <p style="word-break: break-all; font-size: 13px; color: #6b7280;">{{.ResetURL}}</p>
  <p>If you did not request a password reset, please ignore this email. The link will expire in 1 hour.</p>
  <div class="footer">Outpost VPN</div>
</div>
</body></html>`

const deviceConfigTemplate = `<!DOCTYPE html>
<html><head><meta charset="UTF-8">` + baseStyle + `</head><body>
<div class="container">
  <h1>WireGuard Configuration</h1>
  <p>Your VPN configuration for device <strong>{{.DeviceName}}</strong> is ready.</p>
  <p>Copy the configuration below into your WireGuard client:</p>
  <pre style="background: #1a1a2e; color: #00ff88; padding: 16px; border-radius: 6px; font-family: 'JetBrains Mono', monospace; font-size: 13px; overflow-x: auto; white-space: pre-wrap;">{{.Config}}</pre>
  <p style="font-size: 13px; color: #6b7280;">You can also import this configuration from the Outpost dashboard under <strong>My Devices</strong>.</p>
  <p style="font-size: 13px; color: #ef4444;"><strong>Important:</strong> This email contains your private key. Delete it after saving the configuration to your device.</p>
  <div class="footer">Outpost VPN</div>
</div>
</body></html>`

const welcomeTemplate = `<!DOCTYPE html>
<html><head><meta charset="UTF-8">` + baseStyle + `</head><body>
<div class="container">
  <h1>Welcome, {{.Username}}!</h1>
  <p>Your account on <strong>{{.InstanceName}}</strong> has been created successfully.</p>
  <p>You can now sign in and enroll your devices to connect to the VPN.</p>
  <p>If you have any questions, please contact your administrator.</p>
  <div class="footer">Outpost VPN</div>
</div>
</body></html>`
