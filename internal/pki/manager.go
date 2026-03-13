package pki

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// DefaultKeyLifetime is the default validity period for a key pair.
const DefaultKeyLifetime = 90 * 24 * time.Hour // 90 days

// KeyPair represents a WireGuard key pair with metadata.
type KeyPair struct {
	ID         string    `json:"id"`
	DeviceID   string    `json:"device_id,omitempty"`
	GatewayID  string    `json:"gateway_id,omitempty"`
	PublicKey  string    `json:"public_key"`
	CreatedAt  time.Time `json:"created_at"`
	ExpiresAt  time.Time `json:"expires_at"`
	IsActive   bool      `json:"is_active"`
	RotationID int       `json:"rotation_id"`
}

// Manager handles key lifecycle operations.
type Manager struct {
	pool *pgxpool.Pool
}

// NewManager creates a new PKI Manager.
func NewManager(pool *pgxpool.Pool) *Manager {
	return &Manager{pool: pool}
}

// RotateDeviceKey generates a new key pair for a device and marks the old one
// as inactive. Returns the new key pair (with the private key available only
// at rotation time).
func (m *Manager) RotateDeviceKey(ctx context.Context, deviceID string) (*KeyPair, error) {
	privKey, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		return nil, fmt.Errorf("generate private key: %w", err)
	}
	pubKey := privKey.PublicKey().String()

	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Get next rotation ID for this device.
	var nextRotation int
	err = tx.QueryRow(ctx,
		`SELECT COALESCE(MAX(rotation_id), 0) + 1 FROM key_pairs WHERE device_id = $1`,
		deviceID,
	).Scan(&nextRotation)
	if err != nil {
		return nil, fmt.Errorf("get rotation id: %w", err)
	}

	// Deactivate existing keys.
	_, err = tx.Exec(ctx,
		`UPDATE key_pairs SET is_active = false WHERE device_id = $1 AND is_active = true`,
		deviceID,
	)
	if err != nil {
		return nil, fmt.Errorf("deactivate old keys: %w", err)
	}

	// Insert new key pair.
	now := time.Now().UTC()
	kp := KeyPair{
		ID:         uuid.New().String(),
		DeviceID:   deviceID,
		PublicKey:  pubKey,
		CreatedAt:  now,
		ExpiresAt:  now.Add(DefaultKeyLifetime),
		IsActive:   true,
		RotationID: nextRotation,
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO key_pairs (id, device_id, public_key, created_at, expires_at, is_active, rotation_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		kp.ID, kp.DeviceID, kp.PublicKey, kp.CreatedAt, kp.ExpiresAt, kp.IsActive, kp.RotationID,
	)
	if err != nil {
		return nil, fmt.Errorf("insert key pair: %w", err)
	}

	// Update the device's wireguard public key.
	_, err = tx.Exec(ctx,
		`UPDATE devices SET wireguard_pubkey = $1, updated_at = now() WHERE id = $2`,
		pubKey, deviceID,
	)
	if err != nil {
		return nil, fmt.Errorf("update device pubkey: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}

	return &kp, nil
}

// RotateGatewayKey generates a new key pair for a gateway and marks the old one
// as inactive.
func (m *Manager) RotateGatewayKey(ctx context.Context, gatewayID string) (*KeyPair, error) {
	privKey, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		return nil, fmt.Errorf("generate private key: %w", err)
	}
	pubKey := privKey.PublicKey().String()

	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Get next rotation ID for this gateway.
	var nextRotation int
	err = tx.QueryRow(ctx,
		`SELECT COALESCE(MAX(rotation_id), 0) + 1 FROM key_pairs WHERE gateway_id = $1`,
		gatewayID,
	).Scan(&nextRotation)
	if err != nil {
		return nil, fmt.Errorf("get rotation id: %w", err)
	}

	// Deactivate existing keys.
	_, err = tx.Exec(ctx,
		`UPDATE key_pairs SET is_active = false WHERE gateway_id = $1 AND is_active = true`,
		gatewayID,
	)
	if err != nil {
		return nil, fmt.Errorf("deactivate old keys: %w", err)
	}

	// Insert new key pair.
	now := time.Now().UTC()
	kp := KeyPair{
		ID:         uuid.New().String(),
		GatewayID:  gatewayID,
		PublicKey:  pubKey,
		CreatedAt:  now,
		ExpiresAt:  now.Add(DefaultKeyLifetime),
		IsActive:   true,
		RotationID: nextRotation,
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO key_pairs (id, gateway_id, public_key, created_at, expires_at, is_active, rotation_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		kp.ID, kp.GatewayID, kp.PublicKey, kp.CreatedAt, kp.ExpiresAt, kp.IsActive, kp.RotationID,
	)
	if err != nil {
		return nil, fmt.Errorf("insert key pair: %w", err)
	}

	// Update the gateway's wireguard public key.
	_, err = tx.Exec(ctx,
		`UPDATE gateways SET wireguard_pubkey = $1, updated_at = now() WHERE id = $2`,
		pubKey, gatewayID,
	)
	if err != nil {
		return nil, fmt.Errorf("update gateway pubkey: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}

	return &kp, nil
}

// GetExpiring returns key pairs expiring within the given duration.
func (m *Manager) GetExpiring(ctx context.Context, within time.Duration) ([]KeyPair, error) {
	deadline := time.Now().UTC().Add(within)

	rows, err := m.pool.Query(ctx,
		`SELECT id, device_id, gateway_id, public_key, created_at, expires_at, is_active, rotation_id
		 FROM key_pairs
		 WHERE is_active = true AND expires_at <= $1
		 ORDER BY expires_at`,
		deadline,
	)
	if err != nil {
		return nil, fmt.Errorf("query expiring keys: %w", err)
	}
	defer rows.Close()

	var keys []KeyPair
	for rows.Next() {
		var kp KeyPair
		var deviceID, gatewayID *string
		if err := rows.Scan(&kp.ID, &deviceID, &gatewayID, &kp.PublicKey,
			&kp.CreatedAt, &kp.ExpiresAt, &kp.IsActive, &kp.RotationID); err != nil {
			return nil, fmt.Errorf("scan key pair: %w", err)
		}
		if deviceID != nil {
			kp.DeviceID = *deviceID
		}
		if gatewayID != nil {
			kp.GatewayID = *gatewayID
		}
		keys = append(keys, kp)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate key pairs: %w", err)
	}

	return keys, nil
}

// AutoRotate is a background job that finds and rotates all expiring keys.
// Keys expiring within 7 days are automatically rotated.
func (m *Manager) AutoRotate(ctx context.Context) error {
	expiringKeys, err := m.GetExpiring(ctx, 7*24*time.Hour)
	if err != nil {
		return fmt.Errorf("get expiring keys: %w", err)
	}

	for _, kp := range expiringKeys {
		if kp.DeviceID != "" {
			if _, err := m.RotateDeviceKey(ctx, kp.DeviceID); err != nil {
				return fmt.Errorf("rotate device key %s: %w", kp.DeviceID, err)
			}
		}
		if kp.GatewayID != "" {
			if _, err := m.RotateGatewayKey(ctx, kp.GatewayID); err != nil {
				return fmt.Errorf("rotate gateway key %s: %w", kp.GatewayID, err)
			}
		}
	}

	return nil
}
