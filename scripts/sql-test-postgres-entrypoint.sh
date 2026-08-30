#!/bin/sh
set -eu

target=/var/lib/postgresql/sql-test-certs
mkdir -p "$target"
cp /test-certs/server.crt "$target/server.crt"
cp /test-certs/server.key "$target/server.key"
chown -R postgres:postgres "$target"
chmod 600 "$target/server.key"
chmod 644 "$target/server.crt"

exec /usr/local/bin/docker-entrypoint.sh "$@"
