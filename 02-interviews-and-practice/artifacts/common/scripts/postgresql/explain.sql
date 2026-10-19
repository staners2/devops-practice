\set ON_ERROR_STOP on
EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON)
SELECT id, title, status, created_at, updated_at
FROM requests
WHERE status = 'created' AND created_at >= now() - interval '1 day'
ORDER BY created_at DESC, id DESC
LIMIT 50;
