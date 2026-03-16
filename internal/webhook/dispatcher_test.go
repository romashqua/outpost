package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// newTestDispatcher creates a Dispatcher without a database pool, suitable for
// unit tests that exercise delivery, signing, and matching logic.
func newTestDispatcher(subs []Subscription, client *http.Client) *Dispatcher {
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Second}
	}
	d := &Dispatcher{
		logger:     slog.Default(),
		httpClient: client,
		subs:       subs,
	}
	return d
}

// ---------------------------------------------------------------------------
// signPayload
// ---------------------------------------------------------------------------

func TestSignPayload(t *testing.T) {
	secret := []byte("my-secret")
	payload := []byte(`{"type":"user.created"}`)

	got := signPayload(secret, payload)

	// Compute expected independently.
	mac := hmac.New(sha256.New, secret)
	mac.Write(payload)
	want := hex.EncodeToString(mac.Sum(nil))

	if got != want {
		t.Fatalf("signPayload() = %s, want %s", got, want)
	}
}

func TestSignPayload_DifferentSecrets(t *testing.T) {
	payload := []byte(`same payload`)
	sig1 := signPayload([]byte("secret-a"), payload)
	sig2 := signPayload([]byte("secret-b"), payload)
	if sig1 == sig2 {
		t.Fatal("different secrets should produce different signatures")
	}
}

func TestSignPayload_DifferentPayloads(t *testing.T) {
	secret := []byte("same-secret")
	sig1 := signPayload(secret, []byte("payload-a"))
	sig2 := signPayload(secret, []byte("payload-b"))
	if sig1 == sig2 {
		t.Fatal("different payloads should produce different signatures")
	}
}

func TestSignPayload_EmptySecret(t *testing.T) {
	sig := signPayload([]byte{}, []byte("data"))
	if sig == "" {
		t.Fatal("signPayload with empty secret should still produce a hex string")
	}
	if len(sig) != 64 {
		t.Fatalf("expected 64-char hex string, got length %d", len(sig))
	}
}

// ---------------------------------------------------------------------------
// matchesEvent
// ---------------------------------------------------------------------------

func TestMatchesEvent_Wildcard(t *testing.T) {
	if !matchesEvent([]string{"*"}, "user.created") {
		t.Fatal("wildcard should match any event")
	}
}

func TestMatchesEvent_ExactMatch(t *testing.T) {
	if !matchesEvent([]string{"user.created", "device.enrolled"}, "device.enrolled") {
		t.Fatal("exact match should return true")
	}
}

func TestMatchesEvent_NoMatch(t *testing.T) {
	if matchesEvent([]string{"user.created", "device.enrolled"}, "gateway.online") {
		t.Fatal("non-matching event should return false")
	}
}

func TestMatchesEvent_EmptyFilters(t *testing.T) {
	if matchesEvent([]string{}, "user.created") {
		t.Fatal("empty filter list should match nothing")
	}
}

func TestMatchesEvent_NilFilters(t *testing.T) {
	if matchesEvent(nil, "user.created") {
		t.Fatal("nil filter list should match nothing")
	}
}

func TestMatchesEvent_WildcardAmongOthers(t *testing.T) {
	if !matchesEvent([]string{"user.created", "*"}, "anything.at.all") {
		t.Fatal("wildcard among other filters should still match everything")
	}
}

// ---------------------------------------------------------------------------
// validateWebhookURL
// ---------------------------------------------------------------------------

func TestValidateWebhookURL_ValidHTTPS(t *testing.T) {
	// Use a well-known public domain that resolves to non-private IPs.
	err := validateWebhookURL("https://example.com/webhook")
	if err != nil {
		t.Fatalf("expected no error for valid HTTPS URL, got: %v", err)
	}
}

func TestValidateWebhookURL_ValidHTTP(t *testing.T) {
	err := validateWebhookURL("http://example.com/webhook")
	if err != nil {
		t.Fatalf("expected no error for valid HTTP URL, got: %v", err)
	}
}

func TestValidateWebhookURL_PrivateIP_Localhost(t *testing.T) {
	err := validateWebhookURL("http://127.0.0.1/hook")
	if err == nil {
		t.Fatal("expected error for localhost address")
	}
}

func TestValidateWebhookURL_PrivateIP_RFC1918(t *testing.T) {
	err := validateWebhookURL("http://192.168.1.1/hook")
	if err == nil {
		t.Fatal("expected error for private IP address")
	}
}

func TestValidateWebhookURL_InvalidScheme_FTP(t *testing.T) {
	err := validateWebhookURL("ftp://example.com/file")
	if err == nil {
		t.Fatal("expected error for ftp scheme")
	}
}

func TestValidateWebhookURL_EmptyHostname(t *testing.T) {
	err := validateWebhookURL("http:///path")
	if err == nil {
		t.Fatal("expected error for empty hostname")
	}
}

func TestValidateWebhookURL_UnresolvableHost(t *testing.T) {
	err := validateWebhookURL("https://this-domain-definitely-does-not-exist-outpost-test.invalid/hook")
	if err == nil {
		t.Fatal("expected error for unresolvable hostname")
	}
}

func TestValidateWebhookURL_InvalidScheme_Empty(t *testing.T) {
	err := validateWebhookURL("://example.com")
	if err == nil {
		t.Fatal("expected error for empty scheme")
	}
}

// ---------------------------------------------------------------------------
// generateSecret
// ---------------------------------------------------------------------------

func TestGenerateSecret_Length(t *testing.T) {
	secret, err := generateSecret()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 32 bytes -> 64 hex chars.
	if len(secret) != 64 {
		t.Fatalf("expected 64-char hex string, got length %d", len(secret))
	}
}

func TestGenerateSecret_ValidHex(t *testing.T) {
	secret, err := generateSecret()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := hex.DecodeString(secret); err != nil {
		t.Fatalf("secret is not valid hex: %v", err)
	}
}

func TestGenerateSecret_Uniqueness(t *testing.T) {
	s1, err := generateSecret()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s2, err := generateSecret()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s1 == s2 {
		t.Fatal("two generated secrets should not be identical")
	}
}

// ---------------------------------------------------------------------------
// deliverWebhook
// ---------------------------------------------------------------------------

func TestDeliverWebhook_Success(t *testing.T) {
	var (
		gotBody      []byte
		gotSig       string
		gotEventID   string
		gotEventType string
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSig = r.Header.Get("X-Outpost-Signature-256")
		gotEventID = r.Header.Get("X-Outpost-Event-ID")
		gotEventType = r.Header.Get("X-Outpost-Event-Type")
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	sub := Subscription{
		ID:     "sub-1",
		URL:    srv.URL,
		Secret: "test-secret",
		Events: []string{"*"},
	}
	event := Event{
		ID:        "evt-1",
		Type:      "user.created",
		Timestamp: time.Now().UTC(),
		Data:      map[string]string{"user": "alice"},
	}

	d := newTestDispatcher(nil, srv.Client())
	err := d.deliverWebhook(sub, event)
	if err != nil {
		t.Fatalf("expected successful delivery, got: %v", err)
	}

	// Verify signature header.
	expectedSig := "sha256=" + signPayload([]byte(sub.Secret), gotBody)
	if gotSig != expectedSig {
		t.Fatalf("signature mismatch: got %s, want %s", gotSig, expectedSig)
	}
	if gotEventID != event.ID {
		t.Fatalf("event ID header: got %s, want %s", gotEventID, event.ID)
	}
	if gotEventType != event.Type {
		t.Fatalf("event type header: got %s, want %s", gotEventType, event.Type)
	}

	// Verify payload is valid JSON with expected fields.
	var decoded Event
	if err := json.Unmarshal(gotBody, &decoded); err != nil {
		t.Fatalf("payload is not valid JSON: %v", err)
	}
	if decoded.Type != "user.created" {
		t.Fatalf("decoded event type: got %s, want user.created", decoded.Type)
	}
}

func TestDeliverWebhook_RetryOn500(t *testing.T) {
	var attempts atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	sub := Subscription{ID: "sub-2", URL: srv.URL, Secret: "s", Events: []string{"*"}}
	event := Event{ID: "e1", Type: "test", Timestamp: time.Now().UTC()}

	// Use a transport that removes backoff delay for fast tests.
	d := newTestDispatcher(nil, srv.Client())
	// Override httpClient timeout to be generous.
	d.httpClient.Timeout = 30 * time.Second

	err := d.deliverWebhook(sub, event)
	if err != nil {
		t.Fatalf("expected eventual success, got: %v", err)
	}
	if attempts.Load() != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts.Load())
	}
}

func TestDeliverWebhook_AllRetriesFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	sub := Subscription{ID: "sub-3", URL: srv.URL, Secret: "s", Events: []string{"*"}}
	event := Event{ID: "e2", Type: "test", Timestamp: time.Now().UTC()}

	d := newTestDispatcher(nil, srv.Client())
	err := d.deliverWebhook(sub, event)
	if err == nil {
		t.Fatal("expected error after all retries exhausted")
	}
}

func TestDeliverWebhook_NetworkError(t *testing.T) {
	// Point at a server that immediately closes.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	srvURL := srv.URL
	srv.Close() // close immediately so connections fail

	sub := Subscription{ID: "sub-4", URL: srvURL, Secret: "s", Events: []string{"*"}}
	event := Event{ID: "e3", Type: "test", Timestamp: time.Now().UTC()}

	d := newTestDispatcher(nil, &http.Client{Timeout: 1 * time.Second})
	err := d.deliverWebhook(sub, event)
	if err == nil {
		t.Fatal("expected error for network failure")
	}
}

// ---------------------------------------------------------------------------
// Dispatch
// ---------------------------------------------------------------------------

func TestDispatch_FanoutAndFiltering(t *testing.T) {
	var userHits, deviceHits, catchAllHits atomic.Int32

	userSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userHits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer userSrv.Close()

	deviceSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		deviceHits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer deviceSrv.Close()

	catchAllSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		catchAllHits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer catchAllSrv.Close()

	subs := []Subscription{
		{ID: "s1", URL: userSrv.URL, Secret: "a", Events: []string{"user.created"}, IsActive: true},
		{ID: "s2", URL: deviceSrv.URL, Secret: "b", Events: []string{"device.enrolled"}, IsActive: true},
		{ID: "s3", URL: catchAllSrv.URL, Secret: "c", Events: []string{"*"}, IsActive: true},
	}

	d := newTestDispatcher(subs, &http.Client{Timeout: 5 * time.Second})

	event := Event{Type: "user.created", Data: map[string]string{"id": "u1"}}
	if err := d.Dispatch(context.Background(), event); err != nil {
		t.Fatalf("Dispatch returned error: %v", err)
	}

	// Dispatch is async — wait for goroutines to finish.
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if userHits.Load() >= 1 && catchAllHits.Load() >= 1 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if userHits.Load() != 1 {
		t.Fatalf("expected user subscriber to be called once, got %d", userHits.Load())
	}
	if deviceHits.Load() != 0 {
		t.Fatalf("expected device subscriber NOT to be called, got %d", deviceHits.Load())
	}
	if catchAllHits.Load() != 1 {
		t.Fatalf("expected catch-all subscriber to be called once, got %d", catchAllHits.Load())
	}
}

func TestDispatch_SetsIDAndTimestamp(t *testing.T) {
	eventCh := make(chan Event, 1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var ev Event
		json.Unmarshal(body, &ev)
		eventCh <- ev
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	subs := []Subscription{
		{ID: "s1", URL: srv.URL, Secret: "s", Events: []string{"*"}, IsActive: true},
	}
	d := newTestDispatcher(subs, srv.Client())

	// Dispatch with empty ID and zero timestamp.
	event := Event{Type: "test.event", Data: nil}
	_ = d.Dispatch(context.Background(), event)

	select {
	case receivedEvent := <-eventCh:
		if receivedEvent.ID == "" {
			t.Fatal("Dispatch should auto-generate an event ID")
		}
		if receivedEvent.Timestamp.IsZero() {
			t.Fatal("Dispatch should auto-set timestamp when zero")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for webhook delivery")
	}
}

func TestDispatch_NoSubscribers(t *testing.T) {
	d := newTestDispatcher(nil, nil)
	err := d.Dispatch(context.Background(), Event{Type: "test"})
	if err != nil {
		t.Fatalf("Dispatch with no subscribers should not error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// respondJSON / respondError
// ---------------------------------------------------------------------------

func TestRespondJSON(t *testing.T) {
	w := httptest.NewRecorder()
	data := map[string]string{"status": "ok"}
	respondJSON(w, http.StatusOK, data)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if ct != "application/json" {
		t.Fatalf("expected application/json, got %s", ct)
	}

	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("expected status=ok, got %s", body["status"])
	}
}

func TestRespondJSON_NilData(t *testing.T) {
	w := httptest.NewRecorder()
	respondJSON(w, http.StatusNoContent, nil)

	resp := w.Result()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if len(body) != 0 {
		t.Fatalf("expected empty body for nil data, got %q", string(body))
	}
}

func TestRespondJSON_CustomStatusCode(t *testing.T) {
	w := httptest.NewRecorder()
	respondJSON(w, http.StatusCreated, map[string]int{"id": 42})

	resp := w.Result()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
}

func TestRespondError(t *testing.T) {
	w := httptest.NewRecorder()
	respondError(w, http.StatusBadRequest, "missing field")

	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}

	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if body["error"] != "missing field" {
		t.Fatalf("expected error='missing field', got %s", body["error"])
	}
	if body["message"] != "missing field" {
		t.Fatalf("expected message='missing field', got %s", body["message"])
	}
}

func TestRespondError_InternalServer(t *testing.T) {
	w := httptest.NewRecorder()
	respondError(w, http.StatusInternalServerError, "something broke")

	resp := w.Result()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", resp.StatusCode)
	}
}
