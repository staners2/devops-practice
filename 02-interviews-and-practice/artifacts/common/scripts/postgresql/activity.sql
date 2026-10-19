\set ON_ERROR_STOP on
SELECT pid, usename, application_name, state, wait_event_type, wait_event,
       now() - xact_start AS transaction_age, now() - query_start AS query_age,
       pg_blocking_pids(pid) AS blocking_pids, left(query, 160) AS query
FROM pg_stat_activity
WHERE datname = current_database() AND pid <> pg_backend_pid()
ORDER BY query_start;
