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
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

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
		userID,
		entry.Action,
		entry.Resource,
		entry.IPAddress,
		details,
	)

	if _, err := fmt.Fprint(conn, msg); err != nil {
		return fmt.Errorf("write syslog: %w", err)
	}

	return nil
}

// StartBatchExporter runs a background goroutine that periodically queries for
// new audit events (those with id > lastID) and exports them to all configured
// targets. It stops when ctx is cancelled.
func (s *SIEMExporter) StartBatchExporter(ctx context.Context, interval time.Duration) {
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
		s.logger.Debug("batch exporter: exported events", "count", exported, "last_id", maxID)
	}
}
