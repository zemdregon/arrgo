#!/bin/sh
set -eu

ROLE="${ARRGO_ROLE:-master}"
ROLE=$(printf '%s' "$ROLE" | tr '[:upper:]' '[:lower:]')

printf 'arrgo entrypoint: launching %s role\n' "$ROLE" >&2

case "$ROLE" in
        master)
                exec /usr/local/bin/arrgo-master "$@"
                ;;
        worker)
                exec /usr/local/bin/arrgo-worker "$@"
                ;;
        *)
                echo "Unknown ARRGO_ROLE: $ROLE" >&2
                exit 1
                ;;
esac
