#!/bin/sh
# Bootstraps this container as a Postgres streaming replica of $PRIMARY_HOST on first
# start (empty PGDATA), then hands off to the standard postgres entrypoint. On every
# later restart PGDATA is already a standby, so the pg_basebackup step is skipped.
set -e

if [ -z "$(ls -A "$PGDATA" 2>/dev/null)" ]; then
	echo "replica-entrypoint: empty PGDATA, cloning from $PRIMARY_HOST via pg_basebackup..."
	until PGPASSWORD="$REPLICATION_PASSWORD" pg_basebackup \
		-h "$PRIMARY_HOST" -U "$REPLICATION_USER" \
		-D "$PGDATA" -Fp -Xs -P -R; do
		echo "replica-entrypoint: primary not ready yet, retrying in 2s..."
		sleep 2
	done
	chmod 0700 "$PGDATA"
	echo "replica-entrypoint: clone complete, standby.signal + primary_conninfo written by -R"
fi

exec docker-entrypoint.sh postgres
