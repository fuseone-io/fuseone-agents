#!/bin/sh
set -eu

directory=${1:?certificate directory is required}
ca="$directory/ca.crt"
certificate="$directory/server.crt"
key="$directory/server.key"

mkdir -p "$directory"
umask 077
rm -f "$directory/ca.key" "$directory/server.csr" "$ca" "$certificate" "$key"

openssl req -x509 -newkey rsa:2048 -sha256 -nodes -days 7 \
	-keyout "$directory/ca.key" -out "$ca" \
	-subj "/CN=FuseOne disposable SQL test CA" >/dev/null 2>&1

openssl req -newkey rsa:2048 -sha256 -nodes \
	-keyout "$key" -out "$directory/server.csr" \
	-subj "/CN=localhost" \
	-addext "subjectAltName=DNS:localhost,DNS:sql-postgres,IP:127.0.0.1" >/dev/null 2>&1

openssl x509 -req -sha256 -days 7 \
	-in "$directory/server.csr" -CA "$ca" -CAkey "$directory/ca.key" \
	-CAcreateserial -copy_extensions copy -out "$certificate" >/dev/null 2>&1

# The CA exists only to sign this disposable server certificate. Keeping its
# private key would create a reusable authority where the test needs only a
# trust root.
rm -f "$directory/ca.key" "$directory/ca.srl" "$directory/server.csr"
chmod 600 "$key"
chmod 644 "$ca" "$certificate"
