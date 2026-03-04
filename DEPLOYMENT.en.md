# FinArch Deployment & Operations Guide

[简体中文](DEPLOYMENT.md) | English

## Table of Contents
- [Requirements](#requirements)
- [First Deployment](#first-deployment)
- [Environment Variables](#environment-variables)
- [Nginx Reverse Proxy](#nginx-reverse-proxy)
- [Email Verification & Password Reset](#email-verification--password-reset)
- [Data Backup](#data-backup)
  - [Option 1: Litestream Real-Time Backup to Cloudflare R2](#option-1-litestream-real-time-backup-to-cloudflare-r2)
  - [Option 2: In-App Manual Backup](#option-2-in-app-manual-backup)
- [Data Restore](#data-restore)
  - [Via Application UI](#via-application-ui)
  - [Disaster Recovery from R2](#disaster-recovery-from-r2)
  - [Disaster Recovery via Web (No Auth Required)](#disaster-recovery-via-web-no-auth-required)
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

# 2. Create .env (see variable reference below)
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
# JWT signing secret — use a random string
# Generate: openssl rand -hex 32
JWT_SECRET=your-secret-here

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
        proxy_set_header   Host $host;
        proxy_set_header   X-Real-IP $remote_addr;
        proxy_set_header   X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header   X-Forwarded-Proto $scheme;
        client_max_body_size 100m;   # for backup file uploads
    }
}

server {
    listen 80;
    server_name yourdomain.com;
    return 301 https://$host$request_uri;
}
```

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

Authenticated endpoint:

```
GET /api/v1/backup/litestream-health
```

Returns status file data (`last_snapshot_at`, `replication_lag_seconds`) plus current SQLite `journal_mode`.

### Option 2: In-App Manual Backup

Navigate to **Settings** → **Data Backup** → click "Download Backup" to save a `.db` snapshot locally.

Tip: **Download a backup before any major operation.** Store it in a password manager vault or encrypted storage.

---

## Data Restore

### Via Application UI

1. Go to **Settings** → **Data Restore**
2. Select a previously downloaded `.db` backup file
3. Read the warning and click "I understand, confirm restore"
4. Click "Restore now"

> Restore overwrites all current data and cannot be undone. Download a current backup first.

---

### Disaster Recovery from R2

When VPS data is completely lost (disk failure / accidental deletion / server migration):

```bash
# Clone the repo on the new machine and configure .env
git clone https://github.com/KaikiDeishuuu/FinArch.git
cd FinArch
# Fill in .env (including LITESTREAM_* variables)

# Restore database from R2 to local volume
docker run --rm \
  -v finarch_finarch-data:/data \
  -v $(pwd)/litestream.yml:/etc/litestream.yml:ro \
  -e LITESTREAM_ACCESS_KEY_ID=${LITESTREAM_ACCESS_KEY_ID} \
  -e LITESTREAM_SECRET_ACCESS_KEY=${LITESTREAM_SECRET_ACCESS_KEY} \
  -e LITESTREAM_BUCKET=${LITESTREAM_BUCKET} \
  -e LITESTREAM_ENDPOINT=${LITESTREAM_ENDPOINT} \
  litestream/litestream:latest \
  restore -config /etc/litestream.yml /data/finarch.db

# Alternative (inside app container)
./app restore --from-r2 --target=/data/finarch.db

# Start the service after restore
docker compose --profile backup up -d
```

---

### Disaster Recovery via Web (No Auth Required)

If all server data is lost (including auth data), users can restore from a local `.db` backup without logging in:

1. Navigate to the login page and click the **"Disaster Recovery"** link.
2. Upload the `.db` backup file.
3. The system sends a 6-digit verification code to the email address found in the backup.
4. Enter the code to complete the restore.

> This public endpoint is rate-limited and requires email verification to prevent abuse.

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

```bash
cd ~/FinArch

# Pull latest code
git pull

# Pull latest image and restart
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
| Backup download fails | Confirm backend is running; database file is not corrupt |
| Data unchanged after restore | Refresh the page; confirm the uploaded file is a valid SQLite `.db` |
| Container unhealthy | Typically startup state; wait ~15s for it to become healthy |
| PWA showing stale content | Clear site data in browser settings, or uninstall and reinstall the PWA |
