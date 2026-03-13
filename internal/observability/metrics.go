package observability

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// HTTP metrics.
var (
	HTTPRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "Total number of HTTP requests.",
	}, []string{"method", "path", "status"})

	HTTPRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "Duration of HTTP requests in seconds.",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "path"})
)

// gRPC metrics.
var GRPCRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "grpc_requests_total",
	Help: "Total number of gRPC requests.",
}, []string{"method", "status"})

// WireGuard metrics.
var (
	WireGuardPeersTotal = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "wireguard_peers_total",
		Help: "Current number of WireGuard peers.",
	}, []string{"gateway"})

	WireGuardRxBytesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "wireguard_rx_bytes_total",
		Help: "Total bytes received via WireGuard.",
	}, []string{"gateway"})

	WireGuardTxBytesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "wireguard_tx_bytes_total",
		Help: "Total bytes transmitted via WireGuard.",
	}, []string{"gateway"})
)

// Site-to-site tunnel metrics.
var (
	S2STunnelsActive = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "s2s_tunnels_active",
		Help: "Number of active site-to-site tunnels.",
	})

	S2STunnelHealth = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "s2s_tunnel_health",
		Help: "Health of a site-to-site tunnel (1=healthy, 0=unhealthy).",
	}, []string{"tunnel", "remote_gateway"})
)
