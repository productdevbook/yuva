-- name: TakeRateLimits :many
INSERT INTO rate_limits AS r (key, started_at, ends_at, lim, want, count, granted)
SELECT u.key, @now::timestamptz, u.ends_at, u.lim, u.want, LEAST(u.want, u.lim), LEAST(u.want, u.lim)
FROM (SELECT unnest(@keys::bytea[]) AS key, unnest(@ends::timestamptz[]) AS ends_at, unnest(@lims::int[]) AS lim, unnest(@wants::int[]) AS want) u
WHERE true
ON CONFLICT (key) DO UPDATE SET
    started_at = CASE WHEN r.ends_at <= excluded.started_at OR r.started_at > excluded.ends_at THEN excluded.started_at ELSE r.started_at END,
    ends_at = CASE WHEN r.ends_at <= excluded.started_at OR r.started_at > excluded.ends_at THEN excluded.ends_at ELSE r.ends_at END,
    lim = excluded.lim,
    want = excluded.want,
    count = CASE WHEN r.ends_at <= excluded.started_at OR r.started_at > excluded.ends_at THEN excluded.count
        ELSE GREATEST(r.count, LEAST(r.count + excluded.want, excluded.lim)) END,
    granted = CASE WHEN r.ends_at <= excluded.started_at OR r.started_at > excluded.ends_at THEN excluded.granted
        ELSE GREATEST(0, LEAST(r.count + excluded.want, excluded.lim) - r.count) END
RETURNING key, granted, ends_at;

-- name: TakeRateSlot :one
INSERT INTO rate_slots AS r (key, slot, holder, expires_at)
SELECT @key::bytea, s.n, @holder::uuid, @expires_at::timestamptz
FROM generate_series(0, @last::int) AS s (n)
WHERE NOT EXISTS (SELECT 1 FROM rate_slots x WHERE x.key = @key AND x.slot = s.n AND x.expires_at > @now::timestamptz)
ORDER BY s.n
LIMIT 1
ON CONFLICT (key, slot) DO UPDATE SET holder = excluded.holder, expires_at = excluded.expires_at
WHERE r.expires_at <= @now
RETURNING slot;

-- name: ReleaseRateSlot :exec
DELETE FROM rate_slots WHERE key = @key AND slot = @slot AND holder = @holder;

-- name: SweepRateLimits :execrows
DELETE FROM rate_limits WHERE ends_at <= @now;

-- name: SweepRateSlots :execrows
DELETE FROM rate_slots WHERE expires_at <= @now;
