package config

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"os"
	"strconv"
	"time"
)

// Config holds all application configuration.
type Config struct {
	Server    Server
	Database  Database
	Redis     Redis
	WireGuard WireGuard
	Auth      Auth
	OIDC      OIDC
	LDAP      LDAP
	SAML      SAML
	Gateway   Gateway
	Proxy     Proxy
	Log       Log
	SMTP      SMTP
	NAT       NAT
}

// NAT holds NAT traversal (STUN/TURN) configuration.
type NAT struct {
	STUNPort   int
	TURNPort   int
	TURNRealm  string
	ExternalIP string
	Enabled    bool
}

// SMTP holds SMTP mail server configuration.
type SMTP struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	FromName string
	TLS      bool
}

// Server holds HTTP and gRPC listener configuration.
type Server struct {
	HTTPAddr string
	GRPCAddr string
	TLSCert  string
	TLSKey   string
}

// Database holds PostgreSQL connection parameters.
type Database struct {
	Host     string
	Port     int
	Name     string
	User     string
	Password string
	SSLMode  string
	MaxConns int32
	MinConns int32
}

// Redis holds Redis connection parameters.
type Redis struct {
	Addr     string
	Password string
	DB       int
}

// WireGuard holds WireGuard interface configuration.
type WireGuard struct {
	InterfaceName string
	ListenPort    int
	MTU           int
}

// Auth holds authentication and session parameters.
type Auth struct {
	JWTSecret  string
	TokenTTL   time.Duration
	SessionTTL time.Duration
}

// OIDC holds built-in OpenID Connect provider configuration.
type OIDC struct {
	Issuer     string
	SigningKey  string // path to RSA private key PEM file
}

// LDAP holds LDAP/Active Directory sync configuration.
type LDAP struct {
	Enabled      bool
	URL          string
	BindDN       string
	BindPassword string
	BaseDN       string
	UserFilter   string
	GroupFilter  string
	TLS          bool
	SkipVerify   bool
	SyncInterval time.Duration
}

// SAML holds SAML 2.0 Service Provider configuration.
type SAML struct {
	Enabled        bool
	EntityID       string
	ACSURL         string
	IDPMetadataURL string
	CertFile       string
	KeyFile        string
}

// Gateway holds configuration for the gateway component.
type Gateway struct {
	Token    string
	CoreAddr string
}

// Proxy holds configuration for the proxy component.
type Proxy struct {
	ListenAddr string
	CoreAddr   string
}

// Log holds logging configuration.
type Log struct {
	Level  string
	Format string
}

// Load reads configuration from environment variables with the OUTPOST_ prefix.
// Missing variables fall back to sensible defaults.
func Load() *Config {
	return &Config{
		Server: Server{
			HTTPAddr: env("OUTPOST_HTTP_ADDR", ":8080"),
			GRPCAddr: env("OUTPOST_GRPC_ADDR", ":9090"),
			TLSCert:  env("OUTPOST_TLS_CERT", ""),
			TLSKey:   env("OUTPOST_TLS_KEY", ""),
		},
		Database: Database{
			Host:     env("OUTPOST_DB_HOST", "localhost"),
			Port:     envInt("OUTPOST_DB_PORT", 5432),
			Name:     env("OUTPOST_DB_NAME", "outpost"),
			User:     env("OUTPOST_DB_USER", "outpost"),
			Password: env("OUTPOST_DB_PASSWORD", ""),
			SSLMode:  env("OUTPOST_DB_SSLMODE", "disable"),
			MaxConns: int32(envInt("OUTPOST_DB_MAX_CONNS", 20)),
			MinConns: int32(envInt("OUTPOST_DB_MIN_CONNS", 2)),
		},
		Redis: Redis{
			Addr:     env("OUTPOST_REDIS_ADDR", "localhost:6379"),
			Password: env("OUTPOST_REDIS_PASSWORD", ""),
			DB:       envInt("OUTPOST_REDIS_DB", 0),
		},
		WireGuard: WireGuard{
			InterfaceName: env("OUTPOST_WG_INTERFACE", "wg0"),
			ListenPort:    envInt("OUTPOST_WG_LISTEN_PORT", 51820),
			MTU:           envInt("OUTPOST_WG_MTU", 1420),
		},
		Auth: Auth{
			JWTSecret:  envJWTSecret(),
			TokenTTL:   envDuration("OUTPOST_TOKEN_TTL", 15*time.Minute),
			SessionTTL: envDuration("OUTPOST_SESSION_TTL", 24*time.Hour),
		},
		OIDC: OIDC{
			Issuer:    env("OUTPOST_OIDC_ISSUER", "http://localhost:8080"),
			SigningKey: env("OUTPOST_OIDC_SIGNING_KEY", ""),
		},
		LDAP: LDAP{
			Enabled:      env("OUTPOST_LDAP_ENABLED", "false") == "true",
			URL:          env("OUTPOST_LDAP_URL", ""),
			BindDN:       env("OUTPOST_LDAP_BIND_DN", ""),
			BindPassword: env("OUTPOST_LDAP_BIND_PASSWORD", ""),
			BaseDN:       env("OUTPOST_LDAP_BASE_DN", ""),
			UserFilter:   env("OUTPOST_LDAP_USER_FILTER", "(objectClass=person)"),
			GroupFilter:  env("OUTPOST_LDAP_GROUP_FILTER", "(objectClass=group)"),
			TLS:          env("OUTPOST_LDAP_TLS", "false") == "true",
			SkipVerify:   env("OUTPOST_LDAP_SKIP_VERIFY", "false") == "true",
			SyncInterval: envDuration("OUTPOST_LDAP_SYNC_INTERVAL", 15*time.Minute),
		},
		SAML: SAML{
			Enabled:        env("OUTPOST_SAML_ENABLED", "false") == "true",
			EntityID:       env("OUTPOST_SAML_ENTITY_ID", ""),
			ACSURL:         env("OUTPOST_SAML_ACS_URL", ""),
			IDPMetadataURL: env("OUTPOST_SAML_IDP_METADATA_URL", ""),
			CertFile:       env("OUTPOST_SAML_CERT_FILE", ""),
			KeyFile:        env("OUTPOST_SAML_KEY_FILE", ""),
		},
		Gateway: Gateway{
			Token:    env("OUTPOST_GATEWAY_TOKEN", ""),
			CoreAddr: env("OUTPOST_GATEWAY_CORE_ADDR", "localhost:9090"),
		},
		Proxy: Proxy{
			ListenAddr: env("OUTPOST_PROXY_LISTEN_ADDR", ":8081"),
			CoreAddr:   env("OUTPOST_PROXY_CORE_ADDR", "localhost:9090"),
		},
		Log: Log{
			Level:  env("OUTPOST_LOG_LEVEL", "info"),
			Format: env("OUTPOST_LOG_FORMAT", "json"),
		},
		NAT: NAT{
			STUNPort:   envInt("OUTPOST_STUN_PORT", 3478),
			TURNPort:   envInt("OUTPOST_TURN_PORT", 3479),
			TURNRealm:  env("OUTPOST_TURN_REALM", "outpost"),
			ExternalIP: env("OUTPOST_EXTERNAL_IP", ""),
			Enabled:    env("OUTPOST_NAT_ENABLED", "false") == "true",
		},
		SMTP: SMTP{
			Host:     env("OUTPOST_SMTP_HOST", ""),
			Port:     envInt("OUTPOST_SMTP_PORT", 587),
			Username: env("OUTPOST_SMTP_USERNAME", ""),
			Password: env("OUTPOST_SMTP_PASSWORD", ""),
			From:     env("OUTPOST_SMTP_FROM", ""),
			FromName: env("OUTPOST_SMTP_FROM_NAME", "Outpost VPN"),
			TLS:      env("OUTPOST_SMTP_TLS", "false") == "true",
		},
	}
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func envDuration(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return d
}

// envJWTSecret reads OUTPOST_JWT_SECRET from the environment. If empty, it
// generates a random 32-byte hex secret and logs a warning. An empty JWT
// secret is a critical security vulnerability — anyone can forge tokens.
func envJWTSecret() string {
	if v := os.Getenv("OUTPOST_JWT_SECRET"); v != "" {
		return v
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("failed to generate random JWT secret: " + err.Error())
	}
	secret := hex.EncodeToString(b)
	slog.Warn("OUTPOST_JWT_SECRET not set — generated random secret (tokens will not survive restart)")
	return secret
}
