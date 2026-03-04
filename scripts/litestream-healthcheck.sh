#!/bin/sh
set -eu

STATUS_FILE="${LITESTREAM_STATUS_FILE:-/data/litestream_status.json}"
CHECKED_AT="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

OUT="$(litestream snapshots -config /etc/litestream.yml /data/finarch.db 2>&1)" || {
  printf '{"status":"error","checked_at":"%s","last_snapshot_at":"","replication_lag_seconds":-1,"error":"%s"}\n' "$CHECKED_AT" "$(echo "$OUT" | tail -n 1 | tr '"' "'" )" > "$STATUS_FILE"
  echo "$OUT"
  exit 1
}

LAST_TS="$(echo "$OUT" | grep -Eo '[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9:\.]+Z' | tail -n 1 || true)"
LAG=-1
if [ -n "$LAST_TS" ]; then
  NOW_EPOCH="$(date -u +%s)"
  SNAP_EPOCH="$(date -u -d "$LAST_TS" +%s 2>/dev/null || echo '')"
  if [ -n "$SNAP_EPOCH" ]; then
    LAG=$((NOW_EPOCH - SNAP_EPOCH))
  fi
fi

printf '{"status":"ok","checked_at":"%s","last_snapshot_at":"%s","replication_lag_seconds":%s,"error":""}\n' "$CHECKED_AT" "$LAST_TS" "$LAG" > "$STATUS_FILE"
exit 0
