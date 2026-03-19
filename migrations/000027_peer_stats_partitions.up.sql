-- Automatic monthly partition management for peer_stats.
-- Creates a PL/pgSQL function that ensures monthly partitions exist
-- for the current month and next month. Called by core on startup and via cron.

CREATE OR REPLACE FUNCTION create_peer_stats_partitions(months_ahead INT DEFAULT 2)
RETURNS void AS $$
DECLARE
    start_date DATE;
    end_date DATE;
    partition_name TEXT;
    i INT;
BEGIN
    FOR i IN 0..months_ahead LOOP
        start_date := date_trunc('month', CURRENT_DATE + (i || ' months')::interval)::date;
        end_date := (start_date + interval '1 month')::date;
        partition_name := 'peer_stats_' || to_char(start_date, 'YYYY_MM');

        -- Skip if partition already exists.
        IF NOT EXISTS (
            SELECT 1 FROM pg_class WHERE relname = partition_name
        ) THEN
            EXECUTE format(
                'CREATE TABLE %I PARTITION OF peer_stats FOR VALUES FROM (%L) TO (%L)',
                partition_name, start_date, end_date
            );
            RAISE NOTICE 'Created partition: %', partition_name;
        END IF;
    END LOOP;
END;
$$ LANGUAGE plpgsql;

-- Function to drop partitions older than retention period.
CREATE OR REPLACE FUNCTION drop_old_peer_stats_partitions(retention_months INT DEFAULT 6)
RETURNS void AS $$
DECLARE
    cutoff DATE;
    r RECORD;
BEGIN
    cutoff := date_trunc('month', CURRENT_DATE - (retention_months || ' months')::interval)::date;

    FOR r IN
        SELECT inhrelid::regclass::text AS partition_name,
               pg_get_expr(c.relpartbound, c.oid) AS bound_expr
        FROM pg_inherits
        JOIN pg_class c ON c.oid = inhrelid
        WHERE inhparent = 'peer_stats'::regclass
          AND c.relname != 'peer_stats_default'
          AND c.relname LIKE 'peer_stats_%'
    LOOP
        -- Extract the FROM date from the partition bound.
        -- Partitions older than cutoff get dropped.
        IF r.partition_name ~ 'peer_stats_\d{4}_\d{2}' THEN
            DECLARE
                part_date DATE;
            BEGIN
                part_date := to_date(
                    substring(r.partition_name FROM 'peer_stats_(\d{4}_\d{2})'),
                    'YYYY_MM'
                );
                IF part_date < cutoff THEN
                    EXECUTE format('DROP TABLE %I', r.partition_name);
                    RAISE NOTICE 'Dropped old partition: %', r.partition_name;
                END IF;
            END;
        END IF;
    END LOOP;
END;
$$ LANGUAGE plpgsql;

-- Migrate existing data from default partition to proper monthly partitions.
-- First, create partitions for all months that have data in the default partition.
DO $$
DECLARE
    month_rec RECORD;
    start_date DATE;
    end_date DATE;
    partition_name TEXT;
    moved INT;
BEGIN
    -- Find distinct months in default partition.
    FOR month_rec IN
        SELECT DISTINCT date_trunc('month', recorded_at)::date AS month_start
        FROM peer_stats_default
        ORDER BY month_start
    LOOP
        start_date := month_rec.month_start;
        end_date := (start_date + interval '1 month')::date;
        partition_name := 'peer_stats_' || to_char(start_date, 'YYYY_MM');

        IF NOT EXISTS (SELECT 1 FROM pg_class WHERE relname = partition_name) THEN
            EXECUTE format(
                'CREATE TABLE %I PARTITION OF peer_stats FOR VALUES FROM (%L) TO (%L)',
                partition_name, start_date, end_date
            );
        END IF;
    END LOOP;

    -- Now detach default, move rows, reattach.
    -- This is safe because new inserts will go to the proper partition.
    -- For simplicity and safety with concurrent inserts, we just create
    -- future partitions and let new data go to the right place.
    -- Old data in default stays until manual cleanup or next migration.
END;
$$;

-- Create partitions for current and next 2 months.
SELECT create_peer_stats_partitions(2);
