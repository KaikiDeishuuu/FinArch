# FinArch Deployment & Operations Guide

[简体中文](DEPLOYMENT.md) | English

## Table of Contents
- [Requirements](#requirements)
- [First Deployment](#first-deployment)
- [Environment Variables](#environment-variables)
- [Nginx Reverse Proxy](#nginx-reverse-proxy)
- [Browser Session Security Boundary](#browser-session-security-boundary)
- [Email Verification & Password Reset](#email-verification--password-reset)
- [Data Backup](#data-backup)
  - [Option 1: Litestream Real-Time Backup to Cloudflare R2](#option-1-litestream-real-time-backup-to-cloudflare-r2)
  - [Option 2: Maintenance-Window Physical Backup](#option-2-maintenance-window-physical-backup)
- [Data Restore](#data-restore)
  - [Maintenance-Window Upload Restore](#maintenance-window-upload-restore)
  - [Disaster Recovery from R2](#disaster-recovery-from-r2)
- [.env Security Backup](#env-security-backup)
- [Daily Operations](#daily-operations)
- [Updating the Deployment](#updating-the-deployment)
- [Troubleshooting](#troubleshooting)

---

## Requirements

- Docker >= 24
- Docker Compose >= 2.20
- Nginx configured (for reverse proxy + HTTPS)

---

## First Deployment

```bash
# 1. Clone the repo
git clone https://github.com/KaikiDeishuuu/FinArch.git
cd FinArch

# 2. Create .env (set the complete trusted proxy chain below)
cp .env.example .env   # or create manually
nano .env

# 3. Start the service
docker compose up -d

# 4. Check status
docker compose ps
docker logs finarch-api -f
```

---

## Environment Variables

Create a `.env` file in the project root:

```env
# ── Required ────────────────────────────────────────
# Root secret for access-JWT signing and server-side session-key derivation
# Generate: openssl rand -hex 32
JWT_SECRET=your-secret-here

# Immutable GHCR image tag to deploy; use sha-<commit sha> in production
FINARCH_IMAGE_TAG=sha-your-commit-sha

# ── System database operations (disabled by default) ──
# Enable only during a maintenance window. The independent secret must have
# 32+ characters, must not reuse JWT_SECRET, and must be sent in the
# X-FinArch-Operations-Secret request header.
FINARCH_ENABLE_SYSTEM_OPERATIONS=false
FINARCH_SYSTEM_OPERATIONS_SECRET=

# Compose fixes FINARCH_BEHIND_PROXY=true. List the direct peer seen by the app
# and every controlled intermediary to the right of the client in X-Forwarded-For.
# Use narrow /32, /128, or official provider CIDRs; /0 and client ranges are forbidden.
FINARCH_TRUSTED_PROXY_CIDRS=<direct proxy IP>/32[,<controlled upstream proxy CIDR>...]

# For explicit Authorization bearer API calls only; browser cookie sessions do
# not support cross-origin use. Never add Access-Control-Allow-Credentials.
# FINARCH_CORS_ALLOWED_ORIGINS=https://app.example.com

# Docker Compose temporary-space cap for backup/restore. Default: 512m. Ensure
# the host has enough memory before increasing it.
# FINARCH_TMPFS_SIZE=512m

# ── Production HTTP lifecycle (optional) ────────────
# 100 MiB restore/backup and long OCR calls need generous body-read and
# response-write windows.
# FINARCH_HTTP_READ_HEADER_TIMEOUT=10s
# FINARCH_HTTP_READ_TIMEOUT=30m
# FINARCH_HTTP_WRITE_TIMEOUT=30m
# FINARCH_HTTP_IDLE_TIMEOUT=2m
# FINARCH_HTTP_SHUTDOWN_TIMEOUT=5m
# FINARCH_HTTP_MAX_HEADER_BYTES=65536

# ── Cloudflare Turnstile CAPTCHA (optional) ─────────
# Leave empty to disable CAPTCHA; not needed for local dev
# Get keys: https://dash.cloudflare.com/?to=/:account/turnstile
TURNSTILE_SECRET=
TURNSTILE_SITE_KEY=

# ── Litestream R2 Real-Time Backup (optional) ──────
# Only active when using --profile backup
# Get credentials: Cloudflare Dashboard → R2 → Manage API Tokens
LITESTREAM_ACCESS_KEY_ID=
LITESTREAM_SECRET_ACCESS_KEY=
LITESTREAM_BUCKET=finarch-backup
# Format: https://<Account ID>.r2.cloudflarestorage.com
LITESTREAM_ENDPOINT=https://xxxxxxxx.r2.cloudflarestorage.com

# ── Email / Verification (optional) ─────────────────
# Leave empty to skip email verification (register → instant login)
# Get API Key: https://resend.com → API Keys
RESEND_API_KEY=
# Sender address (must be under a verified domain in Resend console)
RESEND_FROM_EMAIL=hello@yourdomain.com
# Public-facing app URL (used in verification/reset email links)
APP_BASE_URL=https://yourdomain.com

# ── Attachment storage safeguards ─────────────────────
FINARCH_ATTACHMENT_MAX_BYTES=20971520
FINARCH_ATTACHMENT_MAX_FILES_PER_USER=500
FINARCH_ATTACHMENT_MAX_TOTAL_BYTES_PER_USER=1073741824
FINARCH_ATTACHMENT_UPLOADS_PER_MINUTE=30
# Timed-out, unlinked attachments enter the durable deletion queue first; cleanup uses the DB write lease.
FINARCH_ATTACHMENT_ORPHAN_TTL=24h
FINARCH_ATTACHMENT_ORPHAN_CLEANUP_INTERVAL=1h
FINARCH_ATTACHMENT_ORPHAN_CLEANUP_BATCH_SIZE=100

# ── Attachment OCR (optional) ─────────────────────────
# none: disabled; paddle: HTTP sidecar; paddle_aistudio: PaddleOCR AIStudio cloud API
FINARCH_OCR_PROVIDER=none
# Enable the following for PaddleOCR AIStudio. Keep the real token only in the server .env; never commit it.
# FINARCH_OCR_PROVIDER=paddle_aistudio
# FINARCH_OCR_AISTUDIO_TOKEN=
# FINARCH_OCR_AISTUDIO_MODEL=PaddleOCR-VL-1.6
# FINARCH_OCR_AISTUDIO_JOB_URL=https://paddleocr.aistudio-app.com/api/v2/ocr/jobs
# Explicit comma-separated allowlist when result files use a different object-storage host (host or host:port).
# FINARCH_OCR_AISTUDIO_ALLOWED_RESULT_HOSTS=
# FINARCH_OCR_AISTUDIO_OPTIONAL_PAYLOAD={"useDocOrientationClassify":false,"useDocUnwarping":false,"useChartRecognition":false}
# FINARCH_OCR_AISTUDIO_POLL_INTERVAL=5s
# FINARCH_OCR_AISTUDIO_MAX_RESULT_BYTES=10485760
# FINARCH_OCR_TIMEOUT=2m
```

---

## Nginx Reverse Proxy

```nginx
server {
    listen 443 ssl;
    server_name yourdomain.com;

    ssl_certificate     /path/to/cert.pem;
    ssl_certificate_key /path/to/key.pem;

    location / {
        proxy_pass         http://127.0.0.1:8080;
        # Origin checks require the client's original Host, including any port.
        proxy_set_header   Host $http_host;
        proxy_set_header   X-Real-IP $remote_addr;
        # This sample Nginx is the public edge: overwrite untrusted forwarding headers.
        proxy_set_header   X-Forwarded-For $remote_addr;
        proxy_set_header   X-Forwarded-Proto $scheme;
        client_max_body_size 101m;   # 100 MiB file plus multipart overhead
        client_body_timeout 30m;
        proxy_send_timeout 30m;
        proxy_read_timeout 30m;
    }
}

server {
    listen 80;
    server_name yourdomain.com;
    return 301 https://$host$request_uri;
}
```

This example assumes Nginx receives untrusted clients directly, so it overwrites
`X-Forwarded-For` with the single `$remote_addr`. In that topology,
`FINARCH_TRUSTED_PROXY_CIDRS` needs only the Nginx address actually seen by the
application. Do not use `$proxy_add_x_forwarded_for` at a public edge without
first sanitizing client input: a malformed injected hop makes the application
conservatively use the shared direct-proxy rate-limit bucket.

If a CDN, load balancer, or another proxy sits in front of Nginx, either allow
and validate only those upstreams, resolve the final client in Nginx, and still
overwrite XFF with one address; or preserve a chain already sanitized by the
outermost edge and add the direct proxy plus every controlled intermediary to
the right of the real client to `FINARCH_TRUSTED_PROXY_CIDRS`. The application
validates the entire XFF chain before walking it from right to left. Any empty or
malformed hop invalidates the chain and falls back to the direct peer. When XFF
is present, the application never switches to a conflicting `X-Real-IP` value.

By default, the application allows 10 seconds for request headers and caps
headers at 64 KiB. Full request reads and response writes each have a 30-minute
window for 100 MiB maintenance restore/backup and long OCR operations, so the
Nginx body/upstream timeouts should be at least as long. On `SIGINT`/`SIGTERM`,
the service stops accepting new connections and gives in-flight requests up to
5 minutes to finish; Compose uses the same value for `stop_grace_period` before
forcing the container down.

The production service always issues `Secure` cookies, so every public entry
point must use HTTPS. TLS may terminate at the trusted reverse proxy directly
connected to the app, but that proxy must preserve the client's original
`Host`; otherwise the exact Origin checks on login, registration, refresh, and
logout fail. Compose fixes `FINARCH_BEHIND_PROXY=true` and requires a non-empty,
fully valid `FINARCH_TRUSTED_PROXY_CIDRS`; it controls only whether a forwarded real
client IP is trusted for rate limiting and audit. It never relaxes Origin,
cookie, or CORS policy.
Compose or the application refuses to start when this configuration is missing,
invalid, or contains `0.0.0.0/0` or `::/0`. Configure the direct proxy observed by
the application and every controlled proxy in the trusted XFF suffix, using the
narrowest host or official provider CIDRs. Never trust public, all-private,
client-address, or otherwise uncontrolled networks. Omitting an intermediary
makes its downstream clients share that proxy's login/session flood budget;
trusting too broadly lets clients spoof forwarding headers. When `cmd/server` is run
directly, `FINARCH_BEHIND_PROXY` defaults to `false` and forwarding headers are
ignored. Setting it to `true` also requires a strictly valid trust list. Boolean
values accept only lowercase `true` or `false`, so misspellings cannot silently
downgrade the deployment.

---

## Browser Session Security Boundary

- The production refresh cookie is named `__Host-finarch_refresh` and always
  uses `HttpOnly`, `Secure`, `SameSite=Strict`, `Path=/`, and no `Domain`
  attribute. The opaque refresh token is never exposed to JavaScript or a JSON
  response. The browser keeps the short-lived access token in memory rather
  than `localStorage`.
- Access tokens live for 15 minutes. Refresh tokens have a sliding 7-day
  lifetime and sessions have a 30-day absolute lifetime. Every refresh rotates
  the token: retries within the 10-second grace period recover the same
  successor, while replaying an old token after that grace period revokes the
  entire session family.
- Browser calls to `POST /auth/login`, `POST /auth/register`,
  `POST /auth/refresh`, and `POST /auth/logout` must carry an `Origin` exactly
  matching the HTTPS `Host`. Serve the frontend and API from the same origin
  (the scheme, host, and port must all match).
- `FINARCH_CORS_ALLOWED_ORIGINS` allows explicit bearer API requests from the
  listed origins only. Responses intentionally omit
  `Access-Control-Allow-Credentials`; this cannot support cross-origin cookie
  sessions, and a CDN or reverse proxy must not add credential support.
- Only `go run ./cmd/cli serve` bound to `127.0.0.1`, `::1`, or `localhost` may
  use the non-`Secure` development cookie `finarch_refresh` and accept
  originless non-browser clients. Never expose that development mode to a LAN
  or the public internet.

---

## Email Verification & Password Reset

> This is **optional**. Without `RESEND_API_KEY`, registration goes straight to login — same as legacy behavior.

### Setup Steps

1. Sign up at [Resend](https://resend.com) and create an API Key.
2. Verify your sender domain in the Resend console (add DKIM & SPF DNS records).
3. Add to `.env`:

   ```env
   RESEND_API_KEY=re_xxxxxxxxxxxx
   RESEND_FROM_EMAIL=hello@yourdomain.com
   APP_BASE_URL=https://yourdomain.com
   ```

4. Restart the service: `docker compose up -d`

### Behavior Matrix

| Scenario | RESEND_API_KEY configured | Not configured |
|----------|--------------------------|----------------|
| Registration | Sends verification email; must click link to activate | Instant login |
| Login (unverified) | Returns 403; can resend verification email | N/A |
| Forgot password | Sends reset link (valid 1 hour) | Feature hidden |

### Existing Users

Database migration (v5) defaults `email_verified` to `1` — **existing users are unaffected** and don't need to re-verify.

---

## Data Backup

### Option 1: Litestream Real-Time Backup to Cloudflare R2

**Prerequisites:** R2 bucket created in Cloudflare with API Token; `LITESTREAM_*` variables set in `.env`.

```bash
# Start (first time or after updates)
docker compose --profile backup up -d

# Verify sync status
docker logs finarch-litestream -f

# Healthcheck status file (shared with API)
cat /var/lib/docker/volumes/finarch_finarch-data/_data/litestream_status.json
```

**Sync frequency:** WAL uploaded ~every second; snapshots every 30 minutes; 30-day retention.

**Coverage:** the current Litestream configuration replicates only
`/data/finarch.db`; it does **not** replicate `/data/attachments`. Back up the
attachment directory separately in encrypted, versioned storage and keep each
database snapshot associated with the matching attachment recovery point. An R2
SQLite snapshot alone is not a complete disaster-recovery backup.

**R2 Bucket Structure:**
```
finarch-backup/
  finarch/
    generations/
      <id>/
        snapshots/   ← Full snapshots
        wal/         ← Incremental WAL segments (auto-pruned)
```

---

### Litestream Health Endpoint

This system-level endpoint is disabled by default. Once operations mode is
enabled, provide both a valid JWT and the operations-secret header:

```
GET /api/v1/backup/litestream-health
X-FinArch-Operations-Secret: <operations-secret>
```

Returns status file data (`last_snapshot_at`, `replication_lag_seconds`) plus current SQLite `journal_mode`.

### Option 2: Maintenance-Window Physical Backup

A physical backup contains **every tenant, authentication data, and password
hashes**. It is not a personal-data export. Temporarily enable system operations,
restart the service, and call the backup API only from a controlled operations
client carrying both a JWT and `X-FinArch-Operations-Secret`.

Disable operations mode immediately afterwards and keep the backup encrypted.
The operations secret must come from a server-side secret manager and be sent
only by a controlled CLI/operations client. Never place it in a web page,
frontend build-time variables, browser storage, or requests visible to ordinary
users.

A physical backup creates a database snapshot and archive in `/tmp`. The
512 MiB default suits small deployments; check temporary-space requirements
against the database and attachment size first, then increase
`FINARCH_TMPFS_SIZE` only when the host has sufficient memory.

A controlled client must first request a one-time export token, then download
with POST and the dedicated header. Never place the token in a URL or use the
legacy GET download form:

```http
POST /api/v1/backup/export-request
Authorization: Bearer <access-token>
X-FinArch-Operations-Secret: <operations-secret>
Content-Type: application/json

{"current_password":"<current-password>"}
```

```http
POST /api/v1/backup/download
Authorization: Bearer <access-token>
X-FinArch-Operations-Secret: <operations-secret>
X-FinArch-Export-Token: <one-time-export-token>
```

The export token is short-lived and single-use. The service rejects both
`GET /api/v1/backup/download` and requests that provide only
`?export_token=...`.

---

## Data Restore

### Maintenance-Window Upload Restore

Upload restore replaces the entire live database and is restricted to downtime
maintenance. The application caps uploads at 100 MiB and total ZIP expansion at
200 MiB. Compose allocates a 512 MiB `/tmp` so one controlled restore can hold
the multipart temporary file, upload copy, and extracted data:

1. Stop public traffic and copy the current database and attachment directory.
2. Temporarily enable system operations with an independent secret.
3. Confirm no other restore is running; increase `FINARCH_TMPFS_SIZE` first if
   concurrent work or more temporary headroom is required.
4. Use a controlled client carrying a JWT and `X-FinArch-Operations-Secret`.
5. Validate the result, disable operations mode, and rotate the secret.

Before replacement, the restore engine creates a safety snapshot through the
SQLite Backup API and journals every attachment that will be overwritten or
created. If restore, migration, attachment writes, or post-restore integrity
checks fail, it automatically restores both the database and attachments.

Automatic rollback is not a substitute for an offline backup. Stop traffic and
copy the current database and attachment directory outside the application data
volume before every restore. If automatic rollback itself fails, the service
stays in maintenance mode and retains a mode-`0600` `failed-restore-*.db`
snapshot and/or a `failed-restore-attachments-*` journal under
`/data/safety_backups`. Do not reopen traffic: copy those artifacts, inspect the
service log for `RESTORE_ROLLBACK_FAILED`, then have an operator recover the
data manually and restart the service.

---

### Disaster Recovery from R2

When VPS data is completely lost (disk failure / accidental deletion / server migration):

Restore `/data/attachments` from its independent backup first, then restore the
matching database snapshot. The application CLI checks every attachment against
its `size_bytes` and `sha256` metadata before replacing the database; a missing
or mismatched file aborts the restore. Invoking Litestream directly bypasses
that application check, so use it only with an empty volume where the target
database does not exist, and never start the API until all attachments have
been restored and verified. For an existing volume, first preserve the
database, attachment directory, and SQLite sidecars offline instead of
overwriting them in place.

```bash
# Clone the repo on the new machine and configure .env
git clone https://github.com/KaikiDeishuuu/FinArch.git
cd FinArch
# Fill in .env (including LITESTREAM_* variables)

# Stop both the API and Litestream; no process may hold the database open
docker compose --profile backup down

# First restore the matching attachment backup into /data/attachments in the
# finarch-data volume, and verify that /data/finarch.db plus its -wal, -shm, and
# -journal sidecars do not exist

# Restore the database from R2 only into an empty volume
docker run --rm \
  -v finarch_finarch-data:/data \
  -v $(pwd)/litestream.yml:/etc/litestream.yml:ro \
  -e LITESTREAM_ACCESS_KEY_ID=${LITESTREAM_ACCESS_KEY_ID} \
  -e LITESTREAM_SECRET_ACCESS_KEY=${LITESTREAM_SECRET_ACCESS_KEY} \
  -e LITESTREAM_BUCKET=${LITESTREAM_BUCKET} \
  -e LITESTREAM_ENDPOINT=${LITESTREAM_ENDPOINT} \
  litestream/litestream:0.3.13 \
  restore -config /etc/litestream.yml /data/finarch.db

# Alternative: use the application CLI. When the target exists it retains a
# mode-0600 finarch.db.pre-restore.* safety copy; apply your retention policy
# only after validating the successful restore.
./app restore --from-r2 --target=/data/finarch.db

# Start services only after attachment and database validation succeeds
docker compose --profile backup up -d
```

---

## .env Security Backup

`.env` contains all secrets — **never commit it to Git or upload to public storage**.

**Recommended approaches (pick one):**

1. **Password Manager** (simplest): Store the full `.env` content as a Secure Note in Bitwarden / 1Password
2. **GPG encrypt and download:**
   ```bash
   # Encrypt on VPS and download locally
   gpg --symmetric --cipher-algo AES256 -o env_backup.gpg .env
   scp root@yourdomain.com:~/FinArch/env_backup.gpg ~/
   # Store env_backup.gpg safely; decrypt: gpg -o .env env_backup.gpg
   ```
3. **SCP directly:**
   ```bash
   scp root@yourdomain.com:~/FinArch/.env ~/finarch_env.txt
   # Immediately move into password manager, delete local plaintext
   ```

---

## Daily Operations

```bash
# Check service status
docker compose ps
docker compose --profile backup ps

# View logs
docker logs finarch-api -f
docker logs finarch-litestream -f

# Restart services
docker compose restart api
docker compose --profile backup restart litestream

# Stop all services
docker compose --profile backup down

# Check database file size
docker exec finarch-api ls -lh /data/finarch.db

# Manual database access (debug only)
docker run --rm -it \
  -v finarch_finarch-data:/data \
  keinos/sqlite3 sqlite3 /data/finarch.db
```

---

## Updating the Deployment

After a push to `main`, the deployment workflow builds and deploys the
`sha-<full commit SHA>` image only if that commit's CI succeeded and it is still
the tip of `main`. Deployments are serialized, and the runtime tag is never the
mutable `latest`. For a manual update, select a full commit SHA that passed CI
and use that same SHA for both the source and image:

```bash
cd ~/FinArch

# Replace with the full 40-character commit SHA that passed CI
FINARCH_DEPLOY_SHA=0123456789abcdef0123456789abcdef01234567
git fetch origin main
git switch main
git merge --ff-only "$FINARCH_DEPLOY_SHA"
test "$(git rev-parse HEAD)" = "$FINARCH_DEPLOY_SHA"

# Pin the production runtime image to that same commit
sed -i "s/^FINARCH_IMAGE_TAG=.*/FINARCH_IMAGE_TAG=sha-${FINARCH_DEPLOY_SHA}/" .env

# Pull the pinned image and restart
docker compose --profile backup pull
docker compose --profile backup up -d

# Confirm the new version is running
docker compose ps
docker logs finarch-api --tail 20
```

---

## Troubleshooting

| Symptom | Steps |
|---------|-------|
| Page not loading | `docker compose ps` to check container status; `docker logs finarch-api` for errors |
| API returns 401 | Check that `JWT_SECRET` matches the previous value (changing it invalidates all tokens) |
| Turnstile keeps failing | Verify `TURNSTILE_SECRET` / `TURNSTILE_SITE_KEY` match your domain |
| Litestream container exits | `docker logs finarch-litestream` — usually incorrect R2 credentials |
| Physical backup fails | Confirm operations mode, JWT, and the independent operations header are valid; check `/tmp` capacity |
| Physical restore fails | Confirm a maintenance window, a valid in-limit upload, and sufficient `/tmp` capacity |
| Container unhealthy | Typically startup state; wait ~15s for it to become healthy |
| PWA showing stale content | Clear site data in browser settings, or uninstall and reinstall the PWA |
