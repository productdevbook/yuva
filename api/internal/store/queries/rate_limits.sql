-- name: TakeRateLimits :many
WITH c AS (SELECT coalesce(sqlc.narg(at)::timestamptz, clock_timestamp()) AS now)
INSERT INTO rate_limits AS r (key, started_at, ends_at, checked_at, lim, want, count, granted)
SELECT u.key, c.now, c.now + u.window_ms * interval '1 millisecond', c.now, u.lim, u.want, LEAST(u.want, u.lim), LEAST(u.want, u.lim)
FROM c, (SELECT unnest(@keys::bytea[]) AS key, unnest(@window_ms::bigint[]) AS window_ms, unnest(@lims::int[]) AS lim, unnest(@wants::int[]) AS want) u
WHERE true
ON CONFLICT (key) DO UPDATE SET (started_at, ends_at, checked_at, lim, want, count, granted) = (
    SELECT
        CASE WHEN x.reset THEN excluded.started_at ELSE r.started_at END,
        CASE WHEN x.reset THEN excluded.ends_at ELSE r.ends_at END,
        excluded.checked_at, excluded.lim, excluded.want,
        CASE WHEN x.reset THEN excluded.count ELSE GREATEST(r.count, LEAST(r.count + excluded.want, excluded.lim)) END,
        CASE WHEN x.reset THEN excluded.granted ELSE GREATEST(0, LEAST(r.count + excluded.want, excluded.lim) - r.count) END
    FROM (SELECT r.ends_at <= excluded.started_at OR r.started_at > excluded.ends_at AS reset) x
)
RETURNING key, granted, ends_at, checked_at;

-- name: TakeRateSlot :one
INSERT INTO rate_slots AS r (key, slot, holder, expires_at)
SELECT @key::bytea, s.n, @holder::uuid, s.now + make_interval(secs => @lease_seconds::float8)
FROM (SELECT g.n, coalesce(sqlc.narg(at)::timestamptz, clock_timestamp()) AS now FROM generate_series(0, @last::int) AS g (n)) s
WHERE NOT EXISTS (SELECT 1 FROM rate_slots x WHERE x.key = @key AND x.slot = s.n AND x.expires_at > s.now)
ORDER BY s.n
LIMIT 1
ON CONFLICT (key, slot) DO UPDATE SET holder = excluded.holder, expires_at = excluded.expires_at
WHERE r.expires_at <= excluded.expires_at - make_interval(secs => @lease_seconds::float8)
RETURNING slot;

-- name: ReleaseRateSlot :exec
DELETE FROM rate_slots WHERE key = @key AND slot = @slot AND holder = @holder;

-- name: SweepRateLimits :execrows
DELETE FROM rate_limits WHERE ends_at <= coalesce(sqlc.narg(at)::timestamptz, clock_timestamp());

-- name: SweepRateSlots :execrows
DELETE FROM rate_slots WHERE expires_at <= coalesce(sqlc.narg(at)::timestamptz, clock_timestamp());
