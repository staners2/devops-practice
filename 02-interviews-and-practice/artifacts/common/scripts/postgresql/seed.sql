\set ON_ERROR_STOP on
\if :{?rows}
\else
\set rows 100000
\endif
SELECT :'rows'::integer BETWEEN 1 AND 100000 AS valid_rows,
       current_database() LIKE 'devops_practice_benchmark_%' AS benchmark_database \gset
\if :valid_rows
\else
\echo 'rows must be between 1 and 100000'
\quit 1
\endif
\if :benchmark_database
\else
\echo 'seed is allowed only in a devops_practice_benchmark_ database'
\quit 1
\endif
INSERT INTO requests (id, title, status, created_at, updated_at)
SELECT md5('devops-practice-benchmark-05:' || n)::uuid,
       'devops-practice-benchmark-05:' || n,
       CASE WHEN n % 100 = 0 THEN 'created' ELSE 'processed' END,
       now() - (n % 172800) * interval '1 second',
       now() - (n % 172800) * interval '1 second'
FROM generate_series(1, :'rows'::integer) AS n
ON CONFLICT (id) DO NOTHING;
ANALYZE requests;
