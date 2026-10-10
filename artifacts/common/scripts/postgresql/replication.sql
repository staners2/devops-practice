\set ON_ERROR_STOP on
SELECT pg_is_in_recovery() AS is_replica \gset
\if :is_replica
SELECT status, sender_host, slot_name, received_tli,
       written_lsn, flushed_lsn, latest_end_lsn
FROM pg_stat_wal_receiver;
SELECT pg_last_wal_receive_lsn(), pg_last_wal_replay_lsn(),
       pg_last_xact_replay_timestamp();
\else
SELECT application_name, state, sync_state, sent_lsn, write_lsn, flush_lsn,
       replay_lsn, pg_wal_lsn_diff(pg_current_wal_lsn(), replay_lsn) AS replay_lag_bytes
FROM pg_stat_replication;
SELECT slot_name, active, restart_lsn, wal_status,
       pg_wal_lsn_diff(pg_current_wal_lsn(), restart_lsn) AS retained_wal_bytes
FROM pg_replication_slots;
\endif
