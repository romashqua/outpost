package config

import (
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
	Gateway   Gateway
	Proxy     Proxy
	Log       Log
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
			JWTSecret:  env("OUTPOST_JWT_SECRET", ""),
			TokenTTL:   envDuration("OUTPOST_TOKEN_TTL", 15*time.Minute),
			SessionTTL: envDuration("OUTPOST_SESSION_TTL", 24*time.Hour),
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
