<div align="center">

<img src="frontend/public/logo.svg" width="80" height="80" alt="FinArch" />

# FinArch

**Expenses · Reimbursement · Analytics**

A lightweight, multi-user financial management system

[![Go](https://img.shields.io/badge/Go-1.24-00ADD8?style=flat-square&logo=go&logoColor=white)](https://golang.org)
[![React](https://img.shields.io/badge/React-19-61DAFB?style=flat-square&logo=react&logoColor=black)](https://react.dev)
[![License](https://img.shields.io/badge/License-MIT-green?style=flat-square)](LICENSE)

**[Live Demo →](https://fund.wulab.tech)**

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
- **🔐 Enterprise Security** — Email-verified registration · Password reset · Dual email-change verification · Password change instantly revokes all sessions
- **📱 PWA Support** — Installable to desktop/home screen with native-like experience
- **☁️ Auto Backup** — Optional Litestream real-time streaming backup to Cloudflare R2
- **🛡️ Disaster Recovery** — Email-verified public restore flow, works even when JWT auth is unavailable
- **📡 Online Device Monitoring** — Dashboard shows real-time online device count (heartbeat mechanism, 2-min interval)
- **🤖 Bot Protection** — Optional Cloudflare Turnstile CAPTCHA
- **🧹 Auto Cleanup** — Unverified accounts purged after 24h; stale device heartbeats recycled after 10min

---

## 🚀 Quick Start

### Local Development

> Prerequisites: Go 1.24+, Node.js 20+

```bash
git clone https://github.com/KaikiDeishuuu/FinArch.git
cd FinArch

# Start backend (:8080)
go run ./cmd/cli serve

# In another terminal, start frontend (:5173)
cd frontend && npm install && npm run dev
```

The frontend is pre-configured with a `/api` proxy — works out of the box. No email or other env vars needed for local dev.

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
| `APP_BASE_URL` | Public site URL | ✅ |
| `RESEND_API_KEY` / `RESEND_FROM_EMAIL` | Email service | Optional |
| `TURNSTILE_SITE_KEY` / `TURNSTILE_SECRET` | CAPTCHA | Optional |
| `LITESTREAM_*` | R2 backup | Optional |

> Optional variables left empty will gracefully disable the feature. See [DEPLOYMENT.en.md](DEPLOYMENT.en.md) for the full guide.

### CI/CD

```
git push → GitHub Actions builds image → GHCR → VPS pulls & restarts
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
| **Backend** | Go 1.24 · Gin · SQLite (WAL) |
| **Frontend** | React 19 · Vite 7 · Tailwind CSS v4 · Framer Motion · Recharts |
| **Deployment** | Docker multi-stage build · GitHub Actions → GHCR → SSH Deploy |
| **Security** | JWT (HMAC HS256) · Cloudflare Turnstile · IP rate limiting · Account lockout |
| **Email** | Resend (verification · reset · disaster recovery) |
| **Backup** | Litestream → Cloudflare R2 · In-app download/restore · Disaster recovery |
| **PWA** | Workbox Service Worker · Offline caching · Home screen install |

---

## 📄 License

[MIT](LICENSE)
