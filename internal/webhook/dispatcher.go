package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Event represents an outbound webhook event.
type Event struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	Data      any       `json:"data"`
}

// Subscription represents a webhook subscription.
type Subscription struct {
	ID        string    `json:"id"`
	URL       string    `json:"url"`
	Secret    string    `json:"-"`
	Events    []string  `json:"events"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
}

// Dispatcher manages outbound webhook delivery.
type Dispatcher struct {
	pool       *pgxpool.Pool
	logger     *slog.Logger
	httpClient *http.Client
	subs       []Subscription
	mu         sync.RWMutex
}

// NewDispatcher creates a Dispatcher backed by the given database pool.
// It loads existing webhook subscriptions from the database on startup.
func NewDispatcher(pool *pgxpool.Pool, logger *slog.Logger) *Dispatcher {
	d := &Dispatcher{
		pool:   pool,
		logger: logger,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}

	// Load existing subscriptions so webhooks fire after server restart.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := d.LoadSubscriptions(ctx); err != nil {
		logger.Warn("failed to load webhook subscriptions on startup", "error", err)
	}

	return d
}

// LoadSubscriptions loads all active subscriptions from the database.
func (d *Dispatcher) LoadSubscriptions(ctx context.Context) error {
	rows, err := d.pool.Query(ctx,
		`SELECT id, url, secret, events, is_active, created_at
		 FROM webhook_subscriptions
		 WHERE is_active = true`)
	if err != nil {
		return fmt.Errorf("querying webhook subscriptions: %w", err)
	}
	defer rows.Close()

	var subs []Subscription
	for rows.Next() {
		var s Subscription
		if err := rows.Scan(&s.ID, &s.URL, &s.Secret, &s.Events, &s.IsActive, &s.CreatedAt); err != nil {
			return fmt.Errorf("scanning subscription: %w", err)
		}
		subs = append(subs, s)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating subscriptions: %w", err)
	}

	d.mu.Lock()
	d.subs = subs
	d.mu.Unlock()

	d.logger.InfoContext(ctx, "loaded webhook subscriptions", "count", len(subs))
	return nil
}

// Dispatch fans out an event to all matching subscribers asynchronously.
func (d *Dispatcher) Dispatch(ctx context.Context, event Event) error {
	if event.ID == "" {
		event.ID = uuid.New().String()
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}

	d.mu.RLock()
	subs := make([]Subscription, len(d.subs))
	copy(subs, d.subs)
	d.mu.RUnlock()

	for _, sub := range subs {
		if !matchesEvent(sub.Events, event.Type) {
			continue
		}
		go func(s Subscription) {
			if err := d.deliverWebhook(s, event); err != nil {
				d.logger.Error("webhook delivery failed",
					"subscription_id", s.ID,
					"url", s.URL,
					"event_type", event.Type,
					"error", err,
				)
			}
		}(sub)
	}
	return nil
}

// deliverWebhook POSTs the event to the subscriber's URL with HMAC-SHA256
// signature. It retries up to 3 times with exponential backoff.
func (d *Dispatcher) deliverWebhook(sub Subscription, event Event) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshaling event: %w", err)
	}

	sig := signPayload([]byte(sub.Secret), payload)

	const maxRetries = 3
	backoff := 1 * time.Second

	for attempt := range maxRetries {
		req, err := http.NewRequest(http.MethodPost, sub.URL, bytes.NewReader(payload))
		if err != nil {
			return fmt.Errorf("creating request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Outpost-Signature-256", "sha256="+sig)
		req.Header.Set("X-Outpost-Event-ID", event.ID)
		req.Header.Set("X-Outpost-Event-Type", event.Type)

		resp, err := d.httpClient.Do(req)
		if err != nil {
			d.logger.Warn("webhook request failed",
				"attempt", attempt+1,
				"url", sub.URL,
				"error", err,
			)
			time.Sleep(backoff)
			backoff *= 2
			continue
		}
		resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			d.logger.Info("webhook delivered",
				"subscription_id", sub.ID,
				"event_type", event.Type,
				"status", resp.StatusCode,
			)
			return nil
		}

		d.logger.Warn("webhook non-2xx response",
			"attempt", attempt+1,
			"url", sub.URL,
			"status", resp.StatusCode,
		)
		time.Sleep(backoff)
		backoff *= 2
	}

	return fmt.Errorf("webhook delivery to %s failed after %d attempts", sub.URL, maxRetries)
}

// signPayload computes HMAC-SHA256 of the payload with the given secret key.
func signPayload(secret, payload []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

// matchesEvent returns true if the subscription's event filters match the given
// event type. A filter of "*" matches all events.
func matchesEvent(filters []string, eventType string) bool {
	for _, f := range filters {
		if f == "*" || f == eventType {
			return true
		}
	}
	return false
}

// ---- HTTP Handlers ----------------------------------------------------------

// Routes returns a chi.Router with webhook subscription management endpoints.
func (d *Dispatcher) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", d.listSubscriptions)
	r.Post("/", d.createSubscription)
	r.Get("/{id}", d.getSubscription)
	r.Delete("/{id}", d.deleteSubscription)
	r.Post("/{id}/test", d.testSubscription)
	return r
}

func (d *Dispatcher) listSubscriptions(w http.ResponseWriter, r *http.Request) {
	rows, err := d.pool.Query(r.Context(),
		`SELECT id, url, secret, events, is_active, created_at
		 FROM webhook_subscriptions
		 ORDER BY created_at DESC`)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list subscriptions")
		return
	}
	defer rows.Close()

	var subs []Subscription
	for rows.Next() {
		var s Subscription
		if err := rows.Scan(&s.ID, &s.URL, &s.Secret, &s.Events, &s.IsActive, &s.CreatedAt); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to scan subscription")
			return
		}
		subs = append(subs, s)
	}
	if subs == nil {
		subs = []Subscription{}
	}

	respondJSON(w, http.StatusOK, subs)
}

type createSubscriptionRequest struct {
	URL    string   `json:"url"`
	Secret string   `json:"secret"`
	Events []string `json:"events"`
}

func (d *Dispatcher) createSubscription(w http.ResponseWriter, r *http.Request) {
	var req createSubscriptionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.URL == "" {
		respondError(w, http.StatusBadRequest, "url is required")
		return
	}
	if req.Secret == "" {
		respondError(w, http.StatusBadRequest, "secret is required")
		return
	}
	if len(req.Events) == 0 {
		req.Events = []string{"*"}
	}

	var sub Subscription
	err := d.pool.QueryRow(r.Context(),
		`INSERT INTO webhook_subscriptions (url, secret, events)
		 VALUES ($1, $2, $3)
		 RETURNING id, url, secret, events, is_active, created_at`,
		req.URL, req.Secret, req.Events,
	).Scan(&sub.ID, &sub.URL, &sub.Secret, &sub.Events, &sub.IsActive, &sub.CreatedAt)
	if err != nil {
		d.logger.ErrorContext(r.Context(), "failed to create subscription", "error", err)
		respondError(w, http.StatusInternalServerError, "failed to create subscription")
		return
	}

	// Reload subscriptions cache.
	_ = d.LoadSubscriptions(r.Context())

	respondJSON(w, http.StatusCreated, sub)
}

func (d *Dispatcher) getSubscription(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, err := uuid.Parse(id); err != nil {
		respondError(w, http.StatusBadRequest, "invalid subscription ID")
		return
	}

	var sub Subscription
	err := d.pool.QueryRow(r.Context(),
		`SELECT id, url, secret, events, is_active, created_at
		 FROM webhook_subscriptions
		 WHERE id = $1`, id,
	).Scan(&sub.ID, &sub.URL, &sub.Secret, &sub.Events, &sub.IsActive, &sub.CreatedAt)
	if err != nil {
		respondError(w, http.StatusNotFound, "subscription not found")
		return
	}

	respondJSON(w, http.StatusOK, sub)
}

func (d *Dispatcher) deleteSubscription(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, err := uuid.Parse(id); err != nil {
		respondError(w, http.StatusBadRequest, "invalid subscription ID")
		return
	}

	tag, err := d.pool.Exec(r.Context(),
		`DELETE FROM webhook_subscriptions WHERE id = $1`, id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to delete subscription")
		return
	}
	if tag.RowsAffected() == 0 {
		respondError(w, http.StatusNotFound, "subscription not found")
		return
	}

	// Reload subscriptions cache.
	_ = d.LoadSubscriptions(r.Context())

	w.WriteHeader(http.StatusNoContent)
}

func (d *Dispatcher) testSubscription(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, err := uuid.Parse(id); err != nil {
		respondError(w, http.StatusBadRequest, "invalid subscription ID")
		return
	}

	var sub Subscription
	err := d.pool.QueryRow(r.Context(),
		`SELECT id, url, secret, events, is_active, created_at
		 FROM webhook_subscriptions
		 WHERE id = $1`, id,
	).Scan(&sub.ID, &sub.URL, &sub.Secret, &sub.Events, &sub.IsActive, &sub.CreatedAt)
	if err != nil {
		respondError(w, http.StatusNotFound, "subscription not found")
		return
	}

	testEvent := Event{
		ID:        uuid.New().String(),
		Type:      "webhook.test",
		Timestamp: time.Now().UTC(),
		Data: map[string]string{
			"message": "This is a test event from Outpost VPN.",
		},
	}

	if err := d.deliverWebhook(sub, testEvent); err != nil {
		respondError(w, http.StatusBadGateway, "test delivery failed: "+err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "delivered"})
}

// ---- Response helpers -------------------------------------------------------

func respondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data != nil {
		_ = json.NewEncoder(w).Encode(data)
	}
}

func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, map[string]string{"error": message, "message": message})
}
