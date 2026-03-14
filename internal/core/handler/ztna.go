package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/romashqua/outpost/internal/ztna"
)

// ZTNAHandler provides endpoints for Zero Trust Network Access.
type ZTNAHandler struct {
	pool *pgxpool.Pool
	log  *slog.Logger
}

// NewZTNAHandler creates a ZTNAHandler.
func NewZTNAHandler(pool *pgxpool.Pool, logger ...*slog.Logger) *ZTNAHandler {
	l := slog.Default()
	if len(logger) > 0 && logger[0] != nil {
		l = logger[0]
	}
	return &ZTNAHandler{pool: pool, log: l.With("handler", "ztna")}
}

// Routes returns a chi.Router with ZTNA endpoints.
func (h *ZTNAHandler) Routes() chi.Router {
	r := chi.NewRouter()

	// Trust scores.
	r.Get("/trust-scores/{deviceId}", h.getDeviceTrustScore)
	r.Get("/trust-scores", h.listTrustScores)
	r.Get("/trust-config", h.getTrustConfig)
	r.Put("/trust-config", h.updateTrustConfig)
	r.Get("/trust-history/{deviceId}", h.getTrustHistory)

	// ZTNA policies.
	r.Get("/policies", h.listPolicies)
	r.Post("/policies", h.createPolicy)
	r.Get("/policies/{id}", h.getPolicy)
	r.Put("/policies/{id}", h.updatePolicy)
	r.Delete("/policies/{id}", h.deletePolicy)

	// DNS rules.
	r.Get("/dns-rules", h.listDNSRules)
	r.Post("/dns-rules", h.createDNSRule)
	r.Delete("/dns-rules/{id}", h.deleteDNSRule)

	return r
}

// --- Trust Score endpoints ---

func (h *ZTNAHandler) getDeviceTrustScore(w http.ResponseWriter, r *http.Request) {
	deviceID, err := parseUUID(r, "deviceId")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	config := h.loadTrustConfig(r)
	calc := ztna.NewTrustScoreCalculator(h.pool, config)
	result, err := calc.Calculate(r.Context(), deviceID)
	if err != nil {
		h.log.Error("failed to calculate trust score", "error", err)
		respondError(w, http.StatusInternalServerError, "failed to calculate trust score")
		return
	}

	respondJSON(w, http.StatusOK, result)
}

type trustScoreSummary struct {
	DeviceID    uuid.UUID       `json:"device_id"`
	DeviceName  string          `json:"device_name"`
	UserID      uuid.UUID       `json:"user_id"`
	Username    string          `json:"username"`
	Score       int             `json:"score"`
	Level       ztna.TrustLevel `json:"level"`
	EvaluatedAt time.Time       `json:"evaluated_at"`
}

func (h *ZTNAHandler) listTrustScores(w http.ResponseWriter, r *http.Request) {
	rows, err := h.pool.Query(r.Context(), `
		SELECT DISTINCT ON (dts.device_id)
			dts.device_id, d.name, d.user_id, u.username,
			dts.score, dts.level, dts.evaluated_at
		FROM device_trust_scores dts
		JOIN devices d ON d.id = dts.device_id
		JOIN users u ON u.id = d.user_id
		ORDER BY dts.device_id, dts.evaluated_at DESC
	`)
	if err != nil {
		h.log.Error("failed to query trust scores", "error", err)
		respondError(w, http.StatusInternalServerError, "failed to query trust scores")
		return
	}
	defer rows.Close()

	scores := make([]trustScoreSummary, 0)
	for rows.Next() {
		var s trustScoreSummary
		if err := rows.Scan(&s.DeviceID, &s.DeviceName, &s.UserID, &s.Username,
			&s.Score, &s.Level, &s.EvaluatedAt); err != nil {
			h.log.Error("failed to scan trust score", "error", err)
			respondError(w, http.StatusInternalServerError, "failed to scan trust score")
			return
		}
		scores = append(scores, s)
	}
	if err := rows.Err(); err != nil {
		h.log.Error("rows iteration error", "error", err)
		respondError(w, http.StatusInternalServerError, "failed to list trust scores")
		return
	}

	respondJSON(w, http.StatusOK, scores)
}

func (h *ZTNAHandler) getTrustConfig(w http.ResponseWriter, r *http.Request) {
	var config ztna.TrustScoreConfig
	err := h.pool.QueryRow(r.Context(), `
		SELECT weight_disk_encryption, weight_screen_lock, weight_antivirus,
		       weight_firewall, weight_os_version, weight_mfa,
		       threshold_high, threshold_medium, threshold_low,
		       auto_restrict_below_medium, auto_block_below_low
		FROM trust_score_config WHERE id = 1
	`).Scan(
		&config.WeightDiskEncryption, &config.WeightScreenLock, &config.WeightAntivirus,
		&config.WeightFirewall, &config.WeightOSVersion, &config.WeightMFA,
		&config.ThresholdHigh, &config.ThresholdMedium, &config.ThresholdLow,
		&config.AutoRestrictBelowMedium, &config.AutoBlockBelowLow,
	)
	if err != nil {
		config = ztna.DefaultTrustScoreConfig()
	}

	respondJSON(w, http.StatusOK, config)
}

func (h *ZTNAHandler) updateTrustConfig(w http.ResponseWriter, r *http.Request) {
	var config ztna.TrustScoreConfig
	if err := parseBody(r, &config); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	weights := []int{config.WeightDiskEncryption, config.WeightScreenLock,
		config.WeightAntivirus, config.WeightFirewall,
		config.WeightOSVersion, config.WeightMFA}
	totalWeight := 0
	for _, wt := range weights {
		if wt < 0 {
			respondError(w, http.StatusBadRequest, "weights must not be negative")
			return
		}
		totalWeight += wt
	}
	if totalWeight != 100 {
		respondError(w, http.StatusBadRequest, "component weights must sum to 100")
		return
	}

	if config.ThresholdHigh <= config.ThresholdMedium || config.ThresholdMedium <= config.ThresholdLow {
		respondError(w, http.StatusBadRequest, "thresholds must be high > medium > low")
		return
	}

	_, err := h.pool.Exec(r.Context(), `
		INSERT INTO trust_score_config (id, weight_disk_encryption, weight_screen_lock,
			weight_antivirus, weight_firewall, weight_os_version, weight_mfa,
			threshold_high, threshold_medium, threshold_low,
			auto_restrict_below_medium, auto_block_below_low)
		VALUES (1, $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (id) DO UPDATE SET
			weight_disk_encryption = $1, weight_screen_lock = $2,
			weight_antivirus = $3, weight_firewall = $4,
			weight_os_version = $5, weight_mfa = $6,
			threshold_high = $7, threshold_medium = $8, threshold_low = $9,
			auto_restrict_below_medium = $10, auto_block_below_low = $11,
			updated_at = now()
	`, config.WeightDiskEncryption, config.WeightScreenLock,
		config.WeightAntivirus, config.WeightFirewall,
		config.WeightOSVersion, config.WeightMFA,
		config.ThresholdHigh, config.ThresholdMedium, config.ThresholdLow,
		config.AutoRestrictBelowMedium, config.AutoBlockBelowLow)
	if err != nil {
		h.log.Error("failed to update trust config", "error", err)
		respondError(w, http.StatusInternalServerError, "failed to update trust config")
		return
	}

	respondJSON(w, http.StatusOK, config)
}

type trustHistoryEntry struct {
	Score       int       `json:"score"`
	Level       string    `json:"level"`
	EvaluatedAt time.Time `json:"evaluated_at"`
}

func (h *ZTNAHandler) getTrustHistory(w http.ResponseWriter, r *http.Request) {
	deviceID, err := parseUUID(r, "deviceId")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	rows, err := h.pool.Query(r.Context(), `
		SELECT score, level, evaluated_at
		FROM device_trust_scores
		WHERE device_id = $1
		ORDER BY evaluated_at DESC
		LIMIT 100
	`, deviceID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to query trust history")
		return
	}
	defer rows.Close()

	history := make([]trustHistoryEntry, 0)
	for rows.Next() {
		var e trustHistoryEntry
		if err := rows.Scan(&e.Score, &e.Level, &e.EvaluatedAt); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to scan trust history")
			return
		}
		history = append(history, e)
	}
	if err := rows.Err(); err != nil {
		h.log.Error("rows iteration error", "error", err)
		respondError(w, http.StatusInternalServerError, "failed to list trust history")
		return
	}

	respondJSON(w, http.StatusOK, history)
}

// --- ZTNA Policy endpoints ---

type ztnaPolicy struct {
	ID          uuid.UUID  `json:"id"`
	Name        string     `json:"name"`
	Description *string    `json:"description"`
	IsActive    bool       `json:"is_active"`
	Conditions  any        `json:"conditions"`
	Action      string     `json:"action"`
	NetworkIDs  []uuid.UUID `json:"network_ids"`
	Priority    int        `json:"priority"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

func (h *ZTNAHandler) listPolicies(w http.ResponseWriter, r *http.Request) {
	rows, err := h.pool.Query(r.Context(), `
		SELECT id, name, description, is_active, conditions, action, network_ids, priority, created_at, updated_at
		FROM ztna_policies ORDER BY priority ASC, created_at DESC
	`)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to query policies")
		return
	}
	defer rows.Close()

	policies := make([]ztnaPolicy, 0)
	for rows.Next() {
		var p ztnaPolicy
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.IsActive,
			&p.Conditions, &p.Action, &p.NetworkIDs, &p.Priority,
			&p.CreatedAt, &p.UpdatedAt); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to scan policy")
			return
		}
		policies = append(policies, p)
	}
	if err := rows.Err(); err != nil {
		h.log.Error("rows iteration error", "error", err)
		respondError(w, http.StatusInternalServerError, "failed to list policies")
		return
	}

	respondJSON(w, http.StatusOK, policies)
}

type createPolicyRequest struct {
	Name        string      `json:"name"`
	Description *string     `json:"description"`
	Conditions  any         `json:"conditions"`
	Action      string      `json:"action"`
	NetworkIDs  []uuid.UUID `json:"network_ids"`
	Priority    int         `json:"priority"`
}

func (h *ZTNAHandler) createPolicy(w http.ResponseWriter, r *http.Request) {
	var req createPolicyRequest
	if err := parseBody(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if req.Name == "" {
		respondError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Action == "" {
		req.Action = "allow"
	}
	if req.Action != "allow" && req.Action != "restrict" && req.Action != "deny" {
		respondError(w, http.StatusBadRequest, "action must be allow, restrict, or deny")
		return
	}
	if req.NetworkIDs == nil {
		req.NetworkIDs = []uuid.UUID{}
	}

	var p ztnaPolicy
	err := h.pool.QueryRow(r.Context(), `
		INSERT INTO ztna_policies (name, description, conditions, action, network_ids, priority)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, name, description, is_active, conditions, action, network_ids, priority, created_at, updated_at
	`, req.Name, req.Description, req.Conditions, req.Action, req.NetworkIDs, req.Priority,
	).Scan(&p.ID, &p.Name, &p.Description, &p.IsActive,
		&p.Conditions, &p.Action, &p.NetworkIDs, &p.Priority,
		&p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		h.log.Error("failed to create policy", "error", err)
		respondError(w, http.StatusInternalServerError, "failed to create policy")
		return
	}

	respondJSON(w, http.StatusCreated, p)
}

func (h *ZTNAHandler) getPolicy(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	var p ztnaPolicy
	err = h.pool.QueryRow(r.Context(), `
		SELECT id, name, description, is_active, conditions, action, network_ids, priority, created_at, updated_at
		FROM ztna_policies WHERE id = $1
	`, id).Scan(&p.ID, &p.Name, &p.Description, &p.IsActive,
		&p.Conditions, &p.Action, &p.NetworkIDs, &p.Priority,
		&p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondError(w, http.StatusNotFound, "policy not found")
		} else {
			h.log.Error("failed to get policy", "error", err)
			respondError(w, http.StatusInternalServerError, "failed to get policy")
		}
		return
	}

	respondJSON(w, http.StatusOK, p)
}

type updatePolicyRequest struct {
	Name        *string     `json:"name,omitempty"`
	Description *string     `json:"description,omitempty"`
	IsActive    *bool       `json:"is_active,omitempty"`
	Conditions  any         `json:"conditions,omitempty"`
	Action      *string     `json:"action,omitempty"`
	NetworkIDs  []uuid.UUID `json:"network_ids,omitempty"`
	Priority    *int        `json:"priority,omitempty"`
}

func (h *ZTNAHandler) updatePolicy(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	var req updatePolicyRequest
	if err := parseBody(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if req.Action != nil && *req.Action != "allow" && *req.Action != "restrict" && *req.Action != "deny" {
		respondError(w, http.StatusBadRequest, "action must be allow, restrict, or deny")
		return
	}

	var p ztnaPolicy
	err = h.pool.QueryRow(r.Context(), `
		UPDATE ztna_policies SET
			name = COALESCE($2, name),
			description = COALESCE($3, description),
			is_active = COALESCE($4, is_active),
			conditions = COALESCE($5, conditions),
			action = COALESCE($6, action),
			network_ids = COALESCE($7, network_ids),
			priority = COALESCE($8, priority),
			updated_at = now()
		WHERE id = $1
		RETURNING id, name, description, is_active, conditions, action, network_ids, priority, created_at, updated_at
	`, id, req.Name, req.Description, req.IsActive, req.Conditions, req.Action, req.NetworkIDs, req.Priority,
	).Scan(&p.ID, &p.Name, &p.Description, &p.IsActive,
		&p.Conditions, &p.Action, &p.NetworkIDs, &p.Priority,
		&p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondError(w, http.StatusNotFound, "policy not found")
		} else {
			h.log.Error("failed to update policy", "error", err)
			respondError(w, http.StatusInternalServerError, "failed to update policy")
		}
		return
	}

	respondJSON(w, http.StatusOK, p)
}

func (h *ZTNAHandler) deletePolicy(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	tag, err := h.pool.Exec(r.Context(), `DELETE FROM ztna_policies WHERE id = $1`, id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to delete policy")
		return
	}

	if tag.RowsAffected() == 0 {
		respondError(w, http.StatusNotFound, "policy not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// --- DNS Rule endpoints ---

type dnsRule struct {
	ID        uuid.UUID `json:"id"`
	NetworkID uuid.UUID `json:"network_id"`
	Domain    string    `json:"domain"`
	DNSServer string    `json:"dns_server"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
}

func (h *ZTNAHandler) listDNSRules(w http.ResponseWriter, r *http.Request) {
	networkID := r.URL.Query().Get("network_id")
	query := `SELECT id, network_id, domain, dns_server, is_active, created_at FROM dns_rules`
	args := []any{}

	if networkID != "" {
		nid, parseErr := uuid.Parse(networkID)
		if parseErr != nil {
			respondError(w, http.StatusBadRequest, "invalid network_id")
			return
		}
		query += ` WHERE network_id = $1`
		args = append(args, nid)
	}
	query += ` ORDER BY domain`

	rows, err := h.pool.Query(r.Context(), query, args...)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to query DNS rules")
		return
	}
	defer rows.Close()

	rules := make([]dnsRule, 0)
	for rows.Next() {
		var dr dnsRule
		if err := rows.Scan(&dr.ID, &dr.NetworkID, &dr.Domain, &dr.DNSServer, &dr.IsActive, &dr.CreatedAt); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to scan DNS rule")
			return
		}
		rules = append(rules, dr)
	}
	if err := rows.Err(); err != nil {
		h.log.Error("rows iteration error", "error", err)
		respondError(w, http.StatusInternalServerError, "failed to list DNS rules")
		return
	}

	respondJSON(w, http.StatusOK, rules)
}

type createDNSRuleRequest struct {
	NetworkID uuid.UUID `json:"network_id"`
	Domain    string    `json:"domain"`
	DNSServer string    `json:"dns_server"`
}

func (h *ZTNAHandler) createDNSRule(w http.ResponseWriter, r *http.Request) {
	var req createDNSRuleRequest
	if err := parseBody(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if req.Domain == "" {
		respondError(w, http.StatusBadRequest, "domain is required")
		return
	}
	if req.DNSServer == "" {
		respondError(w, http.StatusBadRequest, "dns_server is required")
		return
	}
	if req.NetworkID == uuid.Nil {
		respondError(w, http.StatusBadRequest, "network_id is required")
		return
	}

	var rule dnsRule
	err := h.pool.QueryRow(r.Context(), `
		INSERT INTO dns_rules (network_id, domain, dns_server)
		VALUES ($1, $2, $3)
		RETURNING id, network_id, domain, dns_server, is_active, created_at
	`, req.NetworkID, req.Domain, req.DNSServer,
	).Scan(&rule.ID, &rule.NetworkID, &rule.Domain, &rule.DNSServer, &rule.IsActive, &rule.CreatedAt)
	if err != nil {
		h.log.Error("failed to create DNS rule", "error", err)
		respondError(w, http.StatusInternalServerError, "failed to create DNS rule")
		return
	}

	respondJSON(w, http.StatusCreated, rule)
}

func (h *ZTNAHandler) deleteDNSRule(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	tag, err := h.pool.Exec(r.Context(), `DELETE FROM dns_rules WHERE id = $1`, id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to delete DNS rule")
		return
	}

	if tag.RowsAffected() == 0 {
		respondError(w, http.StatusNotFound, "DNS rule not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// --- Helpers ---

func (h *ZTNAHandler) loadTrustConfig(r *http.Request) ztna.TrustScoreConfig {
	var config ztna.TrustScoreConfig
	err := h.pool.QueryRow(r.Context(), `
		SELECT weight_disk_encryption, weight_screen_lock, weight_antivirus,
		       weight_firewall, weight_os_version, weight_mfa,
		       threshold_high, threshold_medium, threshold_low,
		       auto_restrict_below_medium, auto_block_below_low
		FROM trust_score_config WHERE id = 1
	`).Scan(
		&config.WeightDiskEncryption, &config.WeightScreenLock, &config.WeightAntivirus,
		&config.WeightFirewall, &config.WeightOSVersion, &config.WeightMFA,
		&config.ThresholdHigh, &config.ThresholdMedium, &config.ThresholdLow,
		&config.AutoRestrictBelowMedium, &config.AutoBlockBelowLow,
	)
	if err != nil {
		return ztna.DefaultTrustScoreConfig()
	}
	return config
}
