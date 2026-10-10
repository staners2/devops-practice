\set ON_ERROR_STOP on
SELECT current_database() = :'expected_database' AS correct_database \gset
\if :correct_database
\else
\echo 'unexpected database; stop before business smoke'
\quit 1
\endif
SELECT version, applied_at FROM schema_migrations ORDER BY version;
SELECT count(*) AS requests, count(*) FILTER (WHERE status = 'processed') AS processed FROM requests;
SELECT EXISTS (SELECT 1 FROM requests WHERE id = :'known_id'::uuid) AS known_row_present \gset
\if :known_row_present
SELECT id, status, created_at, updated_at FROM requests WHERE id = :'known_id'::uuid;
\else
\echo 'known pre-backup row is missing'
\quit 1
\endif
