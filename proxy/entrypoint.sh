#!/bin/sh
# Downloads fullchain.pem and privkey.pem from CERTS_URL, starts nginx, and re-downloads them every
# CERTS_REFRESH seconds, reloading nginx when they changed.
set -eu

base="${CERTS_URL%/}"
dir=/certs
mkdir -p "$dir"

# fetch succeeds only when new certificates were installed.
fetch() {
	tmp=$(mktemp -d)
	if wget -q -T 30 -O "$tmp/fullchain.pem" "$base/fullchain.pem" &&
		wget -q -T 30 -O "$tmp/privkey.pem" "$base/privkey.pem" &&
		grep -q 'BEGIN CERTIFICATE' "$tmp/fullchain.pem" &&
		grep -q 'PRIVATE KEY' "$tmp/privkey.pem"; then
		if cmp -s "$tmp/fullchain.pem" "$dir/fullchain.pem" && cmp -s "$tmp/privkey.pem" "$dir/privkey.pem"; then
			rm -rf "$tmp"
			return 1
		fi
		chmod 600 "$tmp/privkey.pem"
		mv "$tmp/fullchain.pem" "$tmp/privkey.pem" "$dir/"
		rm -rf "$tmp"
		echo "certs: installed new certificates from $base"
		return 0
	fi
	echo "certs: download from $base failed" >&2
	rm -rf "$tmp"
	return 1
}

fetch || true
if [ ! -s "$dir/fullchain.pem" ] || [ ! -s "$dir/privkey.pem" ]; then
	echo "certs: no certificates available, exiting" >&2
	exit 1
fi

(
	while sleep "${CERTS_REFRESH:-21600}"; do
		if fetch; then
			nginx -s reload || echo "certs: nginx reload failed" >&2
		fi
	done
) &

exec nginx -g 'daemon off;'
