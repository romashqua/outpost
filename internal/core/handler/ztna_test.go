package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	pgxmock "github.com/pashagolub/pgxmock/v4"
	"github.com/romashqua/outpost/internal/auth"
	"github.com/romashqua/outpost/internal/ztna"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ztnaAdminReq returns a request with admin claims in the context.
func ztnaAdminReq(req *http.Request) *http.Request {
	ctx := auth.ContextWithClaims(req.Context(), &auth.TokenClaims{
		UserID:  "00000000-0000-0000-0000-000000000001",
		IsAdmin: true,
	})
	return req.WithContext(ctx)
}

// --- Trust Scores ---

func TestZTNAListTrustScores(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	h := NewZTNAHandler(mock)
	now := time.Now()
	deviceID := uuid.New()
	userID := uuid.New()

	mock.ExpectQuery(`SELECT DISTINCT ON`).
		WillReturnRows(pgxmock.NewRows([]string{"device_id", "name", "user_id", "username", "score", "level", "evaluated_at"}).
			AddRow(deviceID, "laptop", userID, "alice", 85, ztna.TrustLevelHigh, now))

	r := chi.NewRouter()
	r.Get("/ztna/trust-scores", h.listTrustScores)

	req := httptest.NewRequest(http.MethodGet, "/ztna/trust-scores", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var scores []trustScoreSummary
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &scores))
	assert.Len(t, scores, 1)
	assert.Equal(t, 85, scores[0].Score)
	assert.Equal(t, ztna.TrustLevelHigh, scores[0].Level)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestZTNAGetDeviceTrustScore(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	h := NewZTNAHandler(mock)
	deviceID := uuid.New()
	userID := uuid.New()

	// loadTrustConfig query — return error to use defaults
	mock.ExpectQuery(`SELECT weight_disk_encryption`).
		WillReturnError(pgx.ErrNoRows)

	// TrustScoreCalculator.Calculate queries:
	// 1. Get posture data
	mock.ExpectQuery(`SELECT d.user_id, dp.os_type, dp.os_version`).
		WithArgs(deviceID).
		WillReturnRows(pgxmock.NewRows([]string{"user_id", "os_type", "os_version", "disk_encrypted", "screen_lock", "antivirus", "firewall", "score", "checked_at"}).
			AddRow(userID, "linux", "6.1", true, true, true, true, 90, time.Now()))

	// 2. Check MFA
	mock.ExpectQuery(`SELECT mfa_enabled FROM users WHERE id = \$1`).
		WithArgs(userID).
		WillReturnRows(pgxmock.NewRows([]string{"mfa_enabled"}).AddRow(true))

	// 3. Store trust score
	mock.ExpectExec(`INSERT INTO device_trust_scores`).
		WithArgs(deviceID, pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	r := chi.NewRouter()
	r.Get("/ztna/trust-scores/{deviceId}", h.getDeviceTrustScore)

	req := httptest.NewRequest(http.MethodGet, "/ztna/trust-scores/"+deviceID.String(), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var result map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
	assert.Equal(t, float64(100), result["score"])
	require.NoError(t, mock.ExpectationsWereMet())
}

// --- Trust Config ---

func TestZTNAGetTrustConfig(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	h := NewZTNAHandler(mock)

	// Return error to get default config
	mock.ExpectQuery(`SELECT weight_disk_encryption`).
		WillReturnError(pgx.ErrNoRows)

	r := chi.NewRouter()
	r.Get("/ztna/trust-config", h.getTrustConfig)

	req := httptest.NewRequest(http.MethodGet, "/ztna/trust-config", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var config ztna.TrustScoreConfig
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &config))
	assert.Equal(t, 25, config.WeightDiskEncryption)
	assert.Equal(t, 80, config.ThresholdHigh)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestZTNAUpdateTrustConfig(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	h := NewZTNAHandler(mock)

	mock.ExpectExec(`INSERT INTO trust_score_config`).
		WithArgs(25, 10, 20, 15, 15, 15, 80, 50, 20, false, false).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	r := chi.NewRouter()
	r.Put("/ztna/trust-config", h.updateTrustConfig)

	config := ztna.DefaultTrustScoreConfig()
	bodyBytes, _ := json.Marshal(config)
	req := httptest.NewRequest(http.MethodPut, "/ztna/trust-config", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req = ztnaAdminReq(req)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp ztna.TrustScoreConfig
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 25, resp.WeightDiskEncryption)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestZTNAUpdateTrustConfig_InvalidWeights(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	h := NewZTNAHandler(mock)

	r := chi.NewRouter()
	r.Put("/ztna/trust-config", h.updateTrustConfig)

	config := ztna.DefaultTrustScoreConfig()
	config.WeightDiskEncryption = 50 // now sum = 125, not 100
	bodyBytes, _ := json.Marshal(config)
	req := httptest.NewRequest(http.MethodPut, "/ztna/trust-config", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req = ztnaAdminReq(req)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var resp map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Contains(t, resp["message"], "sum to 100")
}

// --- ZTNA Policies CRUD ---

func TestZTNAListPolicies(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	h := NewZTNAHandler(mock)
	now := time.Now()
	policyID := uuid.New()

	mock.ExpectQuery(`SELECT id, name, description, is_active, conditions, action, network_ids, priority, created_at, updated_at\s+FROM ztna_policies`).
		WillReturnRows(pgxmock.NewRows([]string{"id", "name", "description", "is_active", "conditions", "action", "network_ids", "priority", "created_at", "updated_at"}).
			AddRow(policyID, "pol1", nil, true, []byte(`{"min_score":50}`), "allow", []uuid.UUID{}, 10, now, now))

	r := chi.NewRouter()
	r.Get("/ztna/policies", h.listPolicies)

	req := httptest.NewRequest(http.MethodGet, "/ztna/policies", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var policies []ztnaPolicy
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &policies))
	assert.Len(t, policies, 1)
	assert.Equal(t, "pol1", policies[0].Name)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestZTNACreatePolicy(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	h := NewZTNAHandler(mock)
	now := time.Now()
	policyID := uuid.New()

	mock.ExpectQuery(`INSERT INTO ztna_policies`).
		WithArgs("test-policy", (*string)(nil), pgxmock.AnyArg(), "allow", []uuid.UUID{}, 10).
		WillReturnRows(pgxmock.NewRows([]string{"id", "name", "description", "is_active", "conditions", "action", "network_ids", "priority", "created_at", "updated_at"}).
			AddRow(policyID, "test-policy", nil, true, []byte(`{"min_score":50}`), "allow", []uuid.UUID{}, 10, now, now))

	r := chi.NewRouter()
	r.Post("/ztna/policies", h.createPolicy)

	body := `{"name":"test-policy","conditions":{"min_score":50},"action":"allow","priority":10}`
	req := httptest.NewRequest(http.MethodPost, "/ztna/policies", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = ztnaAdminReq(req)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var p ztnaPolicy
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &p))
	assert.Equal(t, "test-policy", p.Name)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestZTNAGetPolicy_Found(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	h := NewZTNAHandler(mock)
	now := time.Now()
	id := uuid.New()

	mock.ExpectQuery(`SELECT id, name, description, is_active, conditions, action, network_ids, priority, created_at, updated_at\s+FROM ztna_policies WHERE id = \$1`).
		WithArgs(id).
		WillReturnRows(pgxmock.NewRows([]string{"id", "name", "description", "is_active", "conditions", "action", "network_ids", "priority", "created_at", "updated_at"}).
			AddRow(id, "pol1", nil, true, []byte(`{}`), "allow", []uuid.UUID{}, 10, now, now))

	r := chi.NewRouter()
	r.Get("/ztna/policies/{id}", h.getPolicy)

	req := httptest.NewRequest(http.MethodGet, "/ztna/policies/"+id.String(), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestZTNAGetPolicy_NotFound(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	h := NewZTNAHandler(mock)
	id := uuid.New()

	mock.ExpectQuery(`SELECT id, name, description`).
		WithArgs(id).
		WillReturnError(pgx.ErrNoRows)

	r := chi.NewRouter()
	r.Get("/ztna/policies/{id}", h.getPolicy)

	req := httptest.NewRequest(http.MethodGet, "/ztna/policies/"+id.String(), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestZTNAUpdatePolicy(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	h := NewZTNAHandler(mock)
	now := time.Now()
	id := uuid.New()

	newName := "updated"
	mock.ExpectQuery(`UPDATE ztna_policies SET`).
		WithArgs(id, &newName, (*string)(nil), (*bool)(nil), pgxmock.AnyArg(), (*string)(nil), ([]uuid.UUID)(nil), (*int)(nil)).
		WillReturnRows(pgxmock.NewRows([]string{"id", "name", "description", "is_active", "conditions", "action", "network_ids", "priority", "created_at", "updated_at"}).
			AddRow(id, "updated", nil, true, []byte(`{}`), "allow", []uuid.UUID{}, 10, now, now))

	r := chi.NewRouter()
	r.Put("/ztna/policies/{id}", h.updatePolicy)

	body := `{"name":"updated"}`
	req := httptest.NewRequest(http.MethodPut, "/ztna/policies/"+id.String(), bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = ztnaAdminReq(req)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var p ztnaPolicy
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &p))
	assert.Equal(t, "updated", p.Name)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestZTNADeletePolicy_Found(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	h := NewZTNAHandler(mock)
	id := uuid.New()

	mock.ExpectExec(`DELETE FROM ztna_policies WHERE id = \$1`).
		WithArgs(id).
		WillReturnResult(pgxmock.NewResult("DELETE", 1))

	r := chi.NewRouter()
	r.Delete("/ztna/policies/{id}", h.deletePolicy)

	req := httptest.NewRequest(http.MethodDelete, "/ztna/policies/"+id.String(), nil)
	req = ztnaAdminReq(req)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestZTNADeletePolicy_NotFound(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	h := NewZTNAHandler(mock)
	id := uuid.New()

	mock.ExpectExec(`DELETE FROM ztna_policies WHERE id = \$1`).
		WithArgs(id).
		WillReturnResult(pgxmock.NewResult("DELETE", 0))

	r := chi.NewRouter()
	r.Delete("/ztna/policies/{id}", h.deletePolicy)

	req := httptest.NewRequest(http.MethodDelete, "/ztna/policies/"+id.String(), nil)
	req = ztnaAdminReq(req)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// --- DNS Rules ---

func TestZTNAListDNSRules(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	h := NewZTNAHandler(mock)
	now := time.Now()
	ruleID := uuid.New()
	networkID := uuid.New()

	mock.ExpectQuery(`SELECT id, network_id, domain, dns_server, is_active, created_at FROM dns_rules`).
		WillReturnRows(pgxmock.NewRows([]string{"id", "network_id", "domain", "dns_server", "is_active", "created_at"}).
			AddRow(ruleID, networkID, "example.com", "8.8.8.8", true, now))

	r := chi.NewRouter()
	r.Get("/ztna/dns-rules", h.listDNSRules)

	req := httptest.NewRequest(http.MethodGet, "/ztna/dns-rules", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var rules []dnsRule
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &rules))
	assert.Len(t, rules, 1)
	assert.Equal(t, "example.com", rules[0].Domain)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestZTNACreateDNSRule(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	h := NewZTNAHandler(mock)
	now := time.Now()
	ruleID := uuid.New()
	networkID := uuid.New()

	mock.ExpectQuery(`INSERT INTO dns_rules`).
		WithArgs(networkID, "internal.corp", "10.0.0.53").
		WillReturnRows(pgxmock.NewRows([]string{"id", "network_id", "domain", "dns_server", "is_active", "created_at"}).
			AddRow(ruleID, networkID, "internal.corp", "10.0.0.53", true, now))

	r := chi.NewRouter()
	r.Post("/ztna/dns-rules", h.createDNSRule)

	body := `{"network_id":"` + networkID.String() + `","domain":"internal.corp","dns_server":"10.0.0.53"}`
	req := httptest.NewRequest(http.MethodPost, "/ztna/dns-rules", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = ztnaAdminReq(req)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var rule dnsRule
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &rule))
	assert.Equal(t, "internal.corp", rule.Domain)
	assert.Equal(t, "10.0.0.53", rule.DNSServer)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestZTNADeleteDNSRule_Found(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	h := NewZTNAHandler(mock)
	id := uuid.New()

	mock.ExpectExec(`DELETE FROM dns_rules WHERE id = \$1`).
		WithArgs(id).
		WillReturnResult(pgxmock.NewResult("DELETE", 1))

	r := chi.NewRouter()
	r.Delete("/ztna/dns-rules/{id}", h.deleteDNSRule)

	req := httptest.NewRequest(http.MethodDelete, "/ztna/dns-rules/"+id.String(), nil)
	req = ztnaAdminReq(req)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestZTNADeleteDNSRule_NotFound(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	h := NewZTNAHandler(mock)
	id := uuid.New()

	mock.ExpectExec(`DELETE FROM dns_rules WHERE id = \$1`).
		WithArgs(id).
		WillReturnResult(pgxmock.NewResult("DELETE", 0))

	r := chi.NewRouter()
	r.Delete("/ztna/dns-rules/{id}", h.deleteDNSRule)

	req := httptest.NewRequest(http.MethodDelete, "/ztna/dns-rules/"+id.String(), nil)
	req = ztnaAdminReq(req)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}
