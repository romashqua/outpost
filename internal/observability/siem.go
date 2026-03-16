package observability

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// sanitizeSyslogValue escapes characters that could inject into RFC 5424
// structured data values (quotes, backslashes, closing brackets, newlines).
func sanitizeSyslogValue(s string) string {
	r := strings.NewReplacer(
		`\`, `\\`,
		`"`, `\"`,
		`]`, `\]`,
		"\n", `\n`,
		"\r", `\r`,
	)
	return r.Replace(s)
}

// SIEMExporter sends audit events to external SIEM systems via webhook and syslog.
type SIEMExporter struct {
	pool       *pgxpool.Pool
	logger     *slog.Logger
	webhookURL string
	syslogAddr string
	hmacSecret string
	mu         sync.RWMutex
	httpClient *http.Client
	lastID     int64
}

// NewSIEMExporter creates a SIEMExporter backed by the given connection pool.
func NewSIEMExporter(pool *pgxpool.Pool, logger *slog.Logger) *SIEMExporter {
	return &SIEMExporter{
		pool:   pool,
		logger: logger,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
				DialContext: safeDialContext,
			},
		},
	}
}

// Configure sets the webhook URL and syslog address for event export.
// Either value may be empty to disable that export target.
func (s *SIEMExporter) Configure(webhookURL, syslogAddr string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.webhookURL = webhookURL
	s.syslogAddr = syslogAddr
}

// SetHMACSecret sets the secret used to sign webhook payloads.
func (s *SIEMExporter) SetHMACSecret(secret string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hmacSecret = secret
}

// ExportEvent sends an audit entry to all configured export targets.
func (s *SIEMExporter) ExportEvent(ctx context.Context, entry AuditEntry) error {
	s.mu.RLock()
	webhookURL := s.webhookURL
	syslogAddr := s.syslogAddr
	s.mu.RUnlock()

	var errs []error

	if webhookURL != "" {
		if err := s.ExportToWebhook(ctx, entry); err != nil {
			errs = append(errs, fmt.Errorf("webhook export: %w", err))
		}
	}

	if syslogAddr != "" {
		if err := s.ExportToSyslog(entry); err != nil {
			errs = append(errs, fmt.Errorf("syslog export: %w", err))
		}
	}

	if len(errs) > 0 {
		// Return first error; all are logged individually.
		return errs[0]
	}
	return nil
}

// ExportToWebhook POSTs the audit entry as JSON to the configured webhook URL.
// The payload is signed with HMAC-SHA256 and the signature is sent in the
// X-Outpost-Signature header.
func (s *SIEMExporter) ExportToWebhook(ctx context.Context, entry AuditEntry) error {
	s.mu.RLock()
	url := s.webhookURL
	secret := s.hmacSecret
	s.mu.RUnlock()

	if url == "" {
		return nil
	}

	payload, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal audit entry: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	if secret != "" {
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(payload)
		sig := hex.EncodeToString(mac.Sum(nil))
		req.Header.Set("X-Outpost-Signature", "sha256="+sig)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}

	return nil
}

// ExportToSyslog sends the audit entry via UDP syslog in RFC 5424 format.
func (s *SIEMExporter) ExportToSyslog(entry AuditEntry) error {
	s.mu.RLock()
	addr := s.syslogAddr
	s.mu.RUnlock()

	if addr == "" {
		return nil
	}

	conn, err := net.DialTimeout("udp", addr, 5*time.Second)
	if err != nil {
		return fmt.Errorf("dial syslog: %w", err)
	}
	defer conn.Close()

	// RFC 5424: <PRI>VERSION TIMESTAMP HOSTNAME APP-NAME PROCID MSGID STRUCTURED-DATA MSG
	// PRI = facility(16=local0) * 8 + severity(6=informational) = 134
	userID := "-"
	if entry.UserID != nil {
		userID = *entry.UserID
	}

	details := "-"
	if entry.Details != nil {
		if b, err := json.Marshal(entry.Details); err == nil {
			details = string(b)
		}
	}

	msg := fmt.Sprintf(
		"<134>1 %s outpost audit-log - - [audit user_id=\"%s\" action=\"%s\" resource=\"%s\" ip=\"%s\"] %s",
		entry.Timestamp.UTC().Format(time.RFC3339),
		sanitizeSyslogValue(userID),
		sanitizeSyslogValue(entry.Action),
		sanitizeSyslogValue(entry.Resource),
		sanitizeSyslogValue(entry.IPAddress),
		details,
	)

	if _, err := fmt.Fprint(conn, msg); err != nil {
		return fmt.Errorf("write syslog: %w", err)
	}

	return nil
}

// StartBatchExporter runs a background goroutine that periodically queries for
// new audit events (those with id > lastID) and exports them to all configured
// targets. It stops when ctx is cancelled. The cursor is persisted in the
// settings table so that restarts don't re-export already-sent events.
func (s *SIEMExporter) StartBatchExporter(ctx context.Context, interval time.Duration) {
	// Restore cursor from DB on startup.
	s.loadCursor(ctx)

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				s.logger.Info("batch exporter stopped")
				return
			case <-ticker.C:
				s.exportBatch(ctx)
			}
		}
	}()
}

const siemCursorKey = "siem_export_cursor"

// loadCursor restores lastID from the settings table.
func (s *SIEMExporter) loadCursor(ctx context.Context) {
	var val string
	err := s.pool.QueryRow(ctx,
		`SELECT value FROM settings WHERE key = $1`, siemCursorKey,
	).Scan(&val)
	if err != nil {
		return // no saved cursor — start from 0
	}
	n, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return
	}
	s.mu.Lock()
	s.lastID = n
	s.mu.Unlock()
	s.logger.Info("batch exporter: restored cursor", "last_id", n)
}

// saveCursor persists lastID to the settings table.
func (s *SIEMExporter) saveCursor(ctx context.Context, id int64) {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO settings (key, value) VALUES ($1, $2)
		ON CONFLICT (key) DO UPDATE SET value = $2
	`, siemCursorKey, strconv.FormatInt(id, 10))
	if err != nil {
		s.logger.Warn("batch exporter: failed to persist cursor", "error", err)
	}
}

// isPrivateIP returns true if the IP belongs to a loopback, private, or
// link-local range that should not be reachable by outbound SIEM webhooks.
func isPrivateIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified()
}

// safeDialContext resolves the target hostname and validates that all resolved
// IPs are public before establishing a connection, preventing SSRF attacks.
func safeDialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("invalid address %q: %w", addr, err)
	}

	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("DNS lookup failed for %s: %w", host, err)
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("no IPs resolved for %s", host)
	}

	for _, ipAddr := range ips {
		if isPrivateIP(ipAddr.IP) {
			return nil, fmt.Errorf("resolved IP %s for host %s is in a private/internal range", ipAddr.IP, host)
		}
	}

	dialer := &net.Dialer{Timeout: 5 * time.Second}
	return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
}

// exportBatch fetches new audit events since the last checkpoint and exports them.
func (s *SIEMExporter) exportBatch(ctx context.Context) {
	s.mu.RLock()
	webhookURL := s.webhookURL
	syslogAddr := s.syslogAddr
	s.mu.RUnlock()

	if webhookURL == "" && syslogAddr == "" {
		return
	}

	const query = `SELECT id, timestamp, user_id, action, resource, details, ip_address, user_agent
		FROM audit_log WHERE id > $1 ORDER BY id ASC LIMIT 500`

	rows, err := s.pool.Query(ctx, query, s.lastID)
	if err != nil {
		s.logger.Error("batch exporter: query failed", "error", err)
		return
	}
	defer rows.Close()

	var maxID int64
	exported := 0

	for rows.Next() {
		var e AuditEntry
		var detailsRaw []byte
		if err := rows.Scan(&e.ID, &e.Timestamp, &e.UserID, &e.Action, &e.Resource, &detailsRaw, &e.IPAddress, &e.UserAgent); err != nil {
			s.logger.Error("batch exporter: scan failed", "error", err)
			continue
		}
		if len(detailsRaw) > 0 {
			var details any
			if err := json.Unmarshal(detailsRaw, &details); err == nil {
				e.Details = details
			}
		}

		if err := s.ExportEvent(ctx, e); err != nil {
			s.logger.Error("batch exporter: export failed", "id", e.ID, "error", err)
			// Stop at first failure so we don't skip events.
			break
		}

		if e.ID > maxID {
			maxID = e.ID
		}
		exported++
	}
	if err := rows.Err(); err != nil {
		s.logger.Error("batch exporter: rows iteration failed", "error", err)
	}

	if maxID > 0 {
		s.mu.Lock()
		s.lastID = maxID
		s.mu.Unlock()
		s.saveCursor(ctx, maxID)
		s.logger.Debug("batch exporter: exported events", "count", exported, "last_id", maxID)
	}
}
