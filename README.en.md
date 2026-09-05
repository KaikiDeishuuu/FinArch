<div align="center">

<img src="frontend/public/logo.svg" width="80" height="80" alt="FinArch" />

# FinArch

**Expenses · Reimbursement · Analytics**

A lightweight, multi-user financial management system

[![Go](https://img.shields.io/badge/Go-1.26.6-00ADD8?style=flat-square&logo=go&logoColor=white)](https://go.dev)
[![React](https://img.shields.io/badge/React-19-61DAFB?style=flat-square&logo=react&logoColor=black)](https://react.dev)
[![License](https://img.shields.io/badge/License-MIT-green?style=flat-square)](LICENSE)

**[Live Demo →](https://farc.dev)**

[简体中文](README.md) | English

</div>

<br/>

## ✨ Features

<table>
<tr>
<td width="50%">

### 📒 Expense Tracking
Record every income and expense, distinguishing between **personal advances** and **public funds**. Supports multi-currency (CNY / USD / EUR / JPY / GBP), categories, project tags, notes, and attachments.

### 💸 Smart Reimbursement
One-click reimbursement status marking with automatic outstanding amount summary. Enter a reimbursement total and **smart-match** the exact combination of transactions — no more manual lookups.

### 📊 Visual Analytics
Annual/monthly trends, category pie charts, project-level summary — personal and public accounts analyzed separately with net balance auto-deducting reimbursed amounts. Multi-dimensional filtering by source, account, category, and project.

</td>
<td width="50%">

### 🏦 Multi-Account Management
Create and manage multiple personal/public accounts with system-maintained balances. Dashboard provides an at-a-glance overview. Account filters auto-adjust based on selected source type.

### 💱 Live Exchange Rates
Powered by ECB data, all summaries auto-convert to CNY. Graceful fallback to built-in rates when offline.

### 📄 PDF Export
One-click export of filtered results to a professionally formatted PDF with brand watermark, user info, and personal/public grouped totals.

</td>
</tr>
</table>

### More

- **👥 Multi-User Isolation** — Each account's data is completely isolated
- **🔐 Session Security** — 15-minute access tokens · Rotating HttpOnly refresh cookie · Password changes revoke every session
- **📱 PWA Support** — Installable to desktop/home screen with native-like experience
- **☁️ Auto Backup** — Optional Litestream real-time streaming backup to Cloudflare R2
- **🛡️ Controlled Recovery** — Litestream R2 restore plus a disabled-by-default, maintenance-window physical-operations API
- **📡 Online Device Monitoring** — Dashboard shows real-time online device count (heartbeat mechanism, 2-min interval)
- **🤖 Bot Protection** — Optional Cloudflare Turnstile CAPTCHA
- **🧹 Auto Cleanup** — Unverified accounts purged after 24h; stale device heartbeats recycled after 10min

---

## 🚀 Quick Start

### Local Development

> Prerequisites: Go 1.26.6+, Node.js 22+

```bash
git clone https://github.com/KaikiDeishuuu/FinArch.git
cd FinArch

# Start backend (:8080)
go run ./cmd/cli serve

# In another terminal, start frontend (:5173)
cd frontend && npm install && npm run dev
```

The frontend is pre-configured with a `/api` proxy — works out of the box. No
email or other env vars are needed for local development. The CLI development
server uses a non-`Secure` cookie and therefore must remain loopback-only; never
expose it to a LAN or the public internet.

### Production Deployment

```bash
git clone https://github.com/KaikiDeishuuu/FinArch.git && cd FinArch
cp .env.example .env   # Edit .env with your config
docker compose up -d
```

Key environment variables:

| Variable | Description | Required |
|----------|-------------|:--------:|
| `JWT_SECRET` | Token signing secret | ✅ |
| `FINARCH_IMAGE_TAG` | Immutable image tag to deploy; use `sha-<commit sha>` in production | ✅ |
| `FINARCH_TRUSTED_PROXY_CIDRS` | Exact IP/CIDR of the direct proxy and every controlled proxy in the trusted XFF suffix; Compose refuses to start without it | ✅ |
| `APP_BASE_URL` | Public URL used in email links; the server default is used when empty | Optional |
| `RESEND_API_KEY` / `RESEND_FROM_EMAIL` | Email service | Optional |
| `TURNSTILE_SITE_KEY` / `TURNSTILE_SECRET` | CAPTCHA | Optional |
| `FINARCH_OCR_PROVIDER` / `FINARCH_OCR_AISTUDIO_*` | Attachment OCR, optionally PaddleOCR AIStudio | Optional |
| `LITESTREAM_*` | R2 backup | Optional |

> Compose always enables proxy mode. Configure the complete, exact, controlled proxy chain. `0.0.0.0/0` and `::/0` are rejected, and all private networks must never be trusted as a convenient default. A directly run server ignores forwarding headers by default. Other optional variables gracefully disable their features when empty. See [DEPLOYMENT.en.md](DEPLOYMENT.en.md) for the full guide.

### CI/CD

```
push main → CI passes and commit is still main tip → build SHA image → GHCR → serialized VPS deploy
```

---

## 🏗 Project Structure

```
FinArch/
├── cmd/
│   ├── cli/                 CLI entry (local dev)
│   ├── server/              Production server entry (Docker)
│   └── desktop/             Desktop entry (Wails)
├── internal/
│   ├── domain/
│   │   ├── model/           Domain models
│   │   ├── repository/      Repository interfaces
│   │   └── service/         Business logic services
│   ├── infrastructure/
│   │   ├── auth/            JWT · Password · Rate limiting · CAPTCHA
│   │   ├── db/              SQLite migrations & triggers
│   │   ├── email/           Email sending (Resend)
│   │   ├── repository/      SQLite repository implementations
│   │   └── plugin/          Plugin system
│   └── interface/
│       ├── apiv1/           REST API routes & handlers
│       └── httpserver/      Embedded static file server
├── frontend/src/
│   ├── api/                 Axios API client
│   ├── components/          Shared components (Select · DatePicker · Brand …)
│   ├── contexts/            Auth · ExchangeRate · Config
│   ├── hooks/               useTransactions · useAccounts · useHeartbeat …
│   ├── motion/              Framer Motion animation system
│   ├── pages/               Page components
│   ├── utils/               Utilities (formatting · exchange rates · PDF export)
│   └── workers/             Web Worker (subset-sum matching)
├── .github/workflows/       CI/CD (Build → GHCR → SSH deploy)
├── docker-compose.yml       Production orchestration
├── Dockerfile               Multi-stage build (Node → Go → Alpine)
└── DEPLOYMENT.md            Deployment guide
```

---

## Tech Stack

| | |
|---|---|
| **Backend** | Go 1.26.6 · Gin · SQLite (WAL) |
| **Frontend** | React 19 · Vite 7 · Tailwind CSS v4 · Framer Motion · Recharts |
| **Deployment** | Docker multi-stage build · GitHub Actions → GHCR → SSH Deploy |
| **Security** | JWT (HMAC HS256) · HttpOnly refresh sessions · Cloudflare Turnstile · IP rate limiting · Account lockout |
| **Email** | Resend (verification · reset · email change) |
| **Backup** | Litestream → Cloudflare R2 · Maintenance-window physical backup/restore |
| **PWA** | Workbox Service Worker · Offline caching · Home screen install |

---

## 📄 License

[MIT](LICENSE)
