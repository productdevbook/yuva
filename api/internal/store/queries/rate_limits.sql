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

-- TakeRateLimitAll counts want against the key only when all of it fits, or nothing.
-- name: TakeRateLimitAll :one
WITH c AS (SELECT coalesce(sqlc.narg(at)::timestamptz, clock_timestamp()) AS now)
INSERT INTO rate_limits AS r (key, started_at, ends_at, checked_at, lim, want, count, granted)
SELECT u.key, c.now, c.now + u.window_ms * interval '1 millisecond', c.now, u.lim, u.want,
    CASE WHEN u.want <= u.lim THEN u.want ELSE 0 END, CASE WHEN u.want <= u.lim THEN u.want ELSE 0 END
FROM c, (SELECT sqlc.arg(key)::bytea AS key, sqlc.arg(window_ms)::bigint AS window_ms, sqlc.arg(lim)::int AS lim, sqlc.arg(want)::int AS want) u
WHERE true
ON CONFLICT (key) DO UPDATE SET (started_at, ends_at, checked_at, lim, want, count, granted) = (
    SELECT
        CASE WHEN x.reset THEN excluded.started_at ELSE r.started_at END,
        CASE WHEN x.reset THEN excluded.ends_at ELSE r.ends_at END,
        excluded.checked_at, excluded.lim, excluded.want,
        CASE WHEN x.reset THEN excluded.count WHEN r.count + excluded.want <= excluded.lim THEN r.count + excluded.want ELSE r.count END,
        CASE WHEN x.reset THEN excluded.granted WHEN r.count + excluded.want <= excluded.lim THEN excluded.want ELSE 0 END
    FROM (SELECT r.ends_at <= excluded.started_at OR r.started_at > excluded.ends_at AS reset) x
)
RETURNING granted, ends_at, checked_at;

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

-- EnsureRateLimits opens an empty window for each key that has none, in key order, so
-- LockRateLimits finds a row for every key.
-- name: EnsureRateLimits :exec
WITH c AS (SELECT coalesce(sqlc.narg(at)::timestamptz, clock_timestamp()) AS now)
INSERT INTO rate_limits (key, started_at, ends_at, checked_at, lim, want, count, granted)
SELECT u.key, c.now, c.now + u.window_ms * interval '1 millisecond', c.now, u.lim, 0, 0, 0
FROM c, (SELECT unnest(@keys::bytea[]) AS key, unnest(@window_ms::bigint[]) AS window_ms, unnest(@lims::int[]) AS lim) u
ORDER BY u.key
ON CONFLICT (key) DO NOTHING;

-- LockRateLimits locks the keys' windows in key order and says how much each has used now.
-- name: LockRateLimits :many
WITH c AS (SELECT coalesce(sqlc.narg(at)::timestamptz, clock_timestamp()) AS now)
SELECT r.key, CASE WHEN r.ends_at <= c.now OR r.started_at > c.now THEN 0 ELSE r.count END::int AS used,
       r.ends_at, c.now::timestamptz AS checked_at
FROM rate_limits r, c WHERE r.key = ANY(@keys::bytea[])
ORDER BY r.key
FOR UPDATE OF r;

-- RateLimitsUsed says how much each key has used in its current window, without counting.
-- name: RateLimitsUsed :many
WITH c AS (SELECT coalesce(sqlc.narg(at)::timestamptz, clock_timestamp()) AS now)
SELECT r.key, r.count::int AS used, r.ends_at, c.now::timestamptz AS checked_at
FROM rate_limits r, c WHERE r.key = ANY(@keys::bytea[]) AND r.ends_at > c.now AND r.started_at <= c.now;
