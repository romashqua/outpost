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
	UserID    string `json:"user_id"`
	BytesSent int64  `json:"bytes_sent"`
	BytesRecv int64  `json:"bytes_recv"`
	Total     int64  `json:"total"`
}

// BandwidthBucket contains bandwidth aggregated into a time bucket.
type BandwidthBucket struct {
	Timestamp time.Time `json:"timestamp"`
	BytesSent int64     `json:"bytes_sent"`
	BytesRecv int64     `json:"bytes_recv"`
}

// HourlyConnections contains connection count for a specific hour of day.
type HourlyConnections struct {
	Hour  int `json:"hour"` // 0-23
	Count int `json:"count"`
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
		`SELECT user_id,
			SUM(bytes_sent) AS bytes_sent,
			SUM(bytes_recv) AS bytes_recv,
			SUM(bytes_sent + bytes_recv) AS total
		 FROM flow_records
		 WHERE recorded_at >= $1 AND recorded_at < $2
		 GROUP BY user_id
		 ORDER BY total DESC
		 LIMIT $3`,
		from, to, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query top users: %w", err)
	}
	defer rows.Close()

	var results []UserBandwidth
	for rows.Next() {
		var ub UserBandwidth
		if err := rows.Scan(&ub.UserID, &ub.BytesSent, &ub.BytesRecv, &ub.Total); err != nil {
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
			SUM(bytes_sent) AS bytes_sent,
			SUM(bytes_recv) AS bytes_recv
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

	var results []BandwidthBucket
	for rows.Next() {
		var bb BandwidthBucket
		if err := rows.Scan(&bb.Timestamp, &bb.BytesSent, &bb.BytesRecv); err != nil {
			return nil, fmt.Errorf("scan bandwidth bucket: %w", err)
		}
		results = append(results, bb)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate bandwidth buckets: %w", err)
	}

	return results, nil
}

// ConnectionsByHour returns connection counts grouped by hour of day (for heatmap).
func (c *Collector) ConnectionsByHour(ctx context.Context, from, to time.Time) ([]HourlyConnections, error) {
	rows, err := c.pool.Query(ctx,
		`SELECT EXTRACT(HOUR FROM recorded_at)::int AS hour,
			COUNT(*) AS count
		 FROM flow_records
		 WHERE recorded_at >= $1 AND recorded_at < $2
		 GROUP BY hour
		 ORDER BY hour`,
		from, to,
	)
	if err != nil {
		return nil, fmt.Errorf("query connections by hour: %w", err)
	}
	defer rows.Close()

	var results []HourlyConnections
	for rows.Next() {
		var hc HourlyConnections
		if err := rows.Scan(&hc.Hour, &hc.Count); err != nil {
			return nil, fmt.Errorf("scan hourly connections: %w", err)
		}
		results = append(results, hc)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate hourly connections: %w", err)
	}

	return results, nil
}
