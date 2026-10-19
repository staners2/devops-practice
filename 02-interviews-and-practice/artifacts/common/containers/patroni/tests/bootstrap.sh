#!/bin/sh
set -eu
temporary=$(mktemp -d /tmp/patroni-bootstrap-XXXXXXXX)
trap 'pg_ctl -D "$temporary/data" -m fast -w stop >/dev/null 2>&1 || true; rm -rf "$temporary"' EXIT
initdb -D "$temporary/data" --auth-local=trust --auth-host=reject >/dev/null
pg_ctl -D "$temporary/data" -o "-k $temporary -c listen_addresses='' -p 55432" -l "$temporary/server.log" -w start >/dev/null
export DEVOPS_PASSWORD=build_check_only PGHOST="$temporary" PGPORT=55432 PGUSER=postgres PGDATABASE=postgres
python /opt/patroni/post-bootstrap.py "host=$temporary port=55432 user=postgres dbname=postgres"
test "$(psql -X -At -c "SELECT rolsuper OR rolcreatedb OR rolcreaterole OR rolreplication FROM pg_roles WHERE rolname = 'devops'")" = f
test "$(psql -X -At -c "SELECT pg_get_userbyid(datdba) FROM pg_database WHERE datname = 'devops_practice'")" = devops
PGUSER=devops PGDATABASE=devops_practice psql -X -v ON_ERROR_STOP=1 -q -c 'CREATE TABLE bootstrap_check (id integer PRIMARY KEY); INSERT INTO bootstrap_check VALUES (1)'
test "$(PGUSER=devops PGDATABASE=devops_practice psql -X -At -c 'SELECT count(*) FROM bootstrap_check')" = 1
printf 'PostgreSQL bootstrap passed on this image architecture\n'
