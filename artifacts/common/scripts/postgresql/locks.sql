\set ON_ERROR_STOP on
SELECT a.pid, a.state, a.wait_event_type, a.wait_event, l.locktype, l.mode,
       l.granted, l.relation::regclass, pg_blocking_pids(a.pid) AS blocking_pids
FROM pg_stat_activity a JOIN pg_locks l USING (pid)
WHERE a.datname = current_database()
ORDER BY a.pid, l.granted, l.locktype;
