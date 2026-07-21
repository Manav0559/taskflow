#!/bin/sh
# Runs once on the primary's first startup (mounted into /docker-entrypoint-initdb.d/).
# POSTGRES_HOST_AUTH_METHOD=trust only covers regular ("all") databases in the
# generated pg_hba.conf, not the special "replication" pseudo-database - that needs
# its own explicit line, which is what this adds. The entrypoint restarts postgres
# after all init scripts run, so this line is in effect before the container is
# considered healthy.
set -e
echo "host replication replicator all trust" >> "$PGDATA/pg_hba.conf"
