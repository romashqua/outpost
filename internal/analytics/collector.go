package analytics

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// FlowRecord represents a network flow record.
type FlowRecord struct {
	Timestamp time.Time
	GatewayID string
	DeviceID  string
	UserID    string
	SrcIP     string
	DstIP     string
	Protocol  string
	DstPort   int
	BytesSent int64
	BytesRecv int64
	Duration  time.Duration
}

// UserBandwidth contains aggregate bandwidth for a user.
type UserBandwidth struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	RxBytes  int64  `json:"rx_bytes"`
	TxBytes  int64  `json:"tx_bytes"`
	Total    int64  `json:"total"`
}

// BandwidthBucket contains bandwidth aggregated into a time bucket.
type BandwidthBucket struct {
	Bucket  time.Time `json:"bucket"`
	RxBytes int64     `json:"rx_bytes"`
	TxBytes int64     `json:"tx_bytes"`
}

// HourlyConnections contains connection count for a specific hour/day of week.
type HourlyConnections struct {
	Hour      int `json:"hour"`       // 0-23
	DayOfWeek int `json:"day_of_week"` // 0=Mon, 6=Sun
	Count     int `json:"count"`
}

// Collector aggregates flow records and produces analytics.
type Collector struct {
	pool *pgxpool.Pool
}

// NewCollector creates a new analytics collector.
func NewCollector(pool *pgxpool.Pool) *Collector {
	return &Collector{pool: pool}
}

// Record stores a flow record in the database.
func (c *Collector) Record(ctx context.Context, flow FlowRecord) error {
	_, err := c.pool.Exec(ctx,
		`INSERT INTO flow_records
			(gateway_id, device_id, user_id, src_ip, dst_ip, protocol, dst_port,
			 bytes_sent, bytes_recv, duration_ms, recorded_at)
		 VALUES ($1, $2, $3, $4::inet, $5::inet, $6, $7, $8, $9, $10, $11)`,
		flow.GatewayID, flow.DeviceID, flow.UserID,
		flow.SrcIP, flow.DstIP, flow.Protocol, flow.DstPort,
		flow.BytesSent, flow.BytesRecv, flow.Duration.Milliseconds(),
		flow.Timestamp,
	)
	if err != nil {
		return fmt.Errorf("insert flow record: %w", err)
	}
	return nil
}

// TopUsers returns top N users by total bandwidth in a time range.
func (c *Collector) TopUsers(ctx context.Context, from, to time.Time, limit int) ([]UserBandwidth, error) {
	rows, err := c.pool.Query(ctx,
		`SELECT fr.user_id,
			COALESCE(u.username, fr.user_id::text) AS username,
			SUM(fr.bytes_recv) AS rx_bytes,
			SUM(fr.bytes_sent) AS tx_bytes,
			SUM(fr.bytes_sent + fr.bytes_recv) AS total
		 FROM flow_records fr
		 LEFT JOIN users u ON u.id = fr.user_id
		 WHERE fr.recorded_at >= $1 AND fr.recorded_at < $2
		 GROUP BY fr.user_id, u.username
		 ORDER BY total DESC
		 LIMIT $3`,
		from, to, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query top users: %w", err)
	}
	defer rows.Close()

	results := make([]UserBandwidth, 0)
	for rows.Next() {
		var ub UserBandwidth
		if err := rows.Scan(&ub.UserID, &ub.Username, &ub.RxBytes, &ub.TxBytes, &ub.Total); err != nil {
			return nil, fmt.Errorf("scan top users row: %w", err)
		}
		results = append(results, ub)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate top users: %w", err)
	}

	return results, nil
}

// BandwidthOverTime returns bandwidth aggregated by time buckets.
func (c *Collector) BandwidthOverTime(ctx context.Context, from, to time.Time, bucketSize time.Duration) ([]BandwidthBucket, error) {
	bucketSecs := int(bucketSize.Seconds())
	if bucketSecs < 1 {
		bucketSecs = 3600 // default to 1 hour
	}

	rows, err := c.pool.Query(ctx,
		`SELECT
			to_timestamp(FLOOR(EXTRACT(EPOCH FROM recorded_at) / $3) * $3) AS bucket,
			SUM(bytes_recv) AS rx_bytes,
			SUM(bytes_sent) AS tx_bytes
		 FROM flow_records
		 WHERE recorded_at >= $1 AND recorded_at < $2
		 GROUP BY bucket
		 ORDER BY bucket`,
		from, to, bucketSecs,
	)
	if err != nil {
		return nil, fmt.Errorf("query bandwidth over time: %w", err)
	}
	defer rows.Close()

	results := make([]BandwidthBucket, 0)
	for rows.Next() {
		var bb BandwidthBucket
		if err := rows.Scan(&bb.Bucket, &bb.RxBytes, &bb.TxBytes); err != nil {
			return nil, fmt.Errorf("scan bandwidth bucket: %w", err)
		}
		results = append(results, bb)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate bandwidth buckets: %w", err)
	}

	return results, nil
}

// ConnectionsByHour returns connection counts grouped by day of week and hour (for heatmap).
func (c *Collector) ConnectionsByHour(ctx context.Context, from, to time.Time) ([]HourlyConnections, error) {
	rows, err := c.pool.Query(ctx,
		`SELECT EXTRACT(HOUR FROM recorded_at)::int AS hour,
			(EXTRACT(ISODOW FROM recorded_at)::int - 1) AS day_of_week,
			COUNT(*) AS count
		 FROM flow_records
		 WHERE recorded_at >= $1 AND recorded_at < $2
		 GROUP BY hour, day_of_week
		 ORDER BY day_of_week, hour`,
		from, to,
	)
	if err != nil {
		return nil, fmt.Errorf("query connections by hour: %w", err)
	}
	defer rows.Close()

	results := make([]HourlyConnections, 0)
	for rows.Next() {
		var hc HourlyConnections
		if err := rows.Scan(&hc.Hour, &hc.DayOfWeek, &hc.Count); err != nil {
			return nil, fmt.Errorf("scan hourly connections: %w", err)
		}
		results = append(results, hc)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate hourly connections: %w", err)
	}

	return results, nil
}
