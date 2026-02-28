<div align="center">

```
  ███████╗██╗███╗   ██╗ █████╗ ██████╗  ██████╗██╗  ██╗
  ██╔════╝██║████╗  ██║██╔══██╗██╔══██╗██╔════╝██║  ██║
  █████╗  ██║██╔██╗ ██║███████║██████╔╝██║     ███████║
  ██╔══╝  ██║██║╚██╗██║██╔══██║██╔══██╗██║     ██╔══██║
  ██║     ██║██║ ╚████║██║  ██║██║  ██║╚██████╗██║  ██║
  ╚═╝     ╚═╝╚═╝  ╚═══╝╚═╝  ╚═╝╚═╝  ╚═╝ ╚═════╝╚═╝  ╚═╝
```

**记账 · 报销 · 统计，轻量高效的多用户财务管理工具**

[![Go](https://img.shields.io/badge/Go-1.24-00ADD8?style=flat-square&logo=go&logoColor=white)](https://golang.org)
[![React](https://img.shields.io/badge/React-19-61DAFB?style=flat-square&logo=react&logoColor=black)](https://react.dev)
[![Vite](https://img.shields.io/badge/Vite-7-646CFF?style=flat-square&logo=vite&logoColor=white)](https://vitejs.dev)
[![Tailwind CSS](https://img.shields.io/badge/Tailwind_CSS-v4-06B6D4?style=flat-square&logo=tailwindcss&logoColor=white)](https://tailwindcss.com)
[![SQLite](https://img.shields.io/badge/SQLite-WAL-003B57?style=flat-square&logo=sqlite&logoColor=white)](https://sqlite.org)
[![Docker](https://img.shields.io/badge/Docker-multi--stage-2496ED?style=flat-square&logo=docker&logoColor=white)](https://docker.com)
[![GitHub Actions](https://img.shields.io/badge/CI%2FCD-GitHub_Actions-2088FF?style=flat-square&logo=githubactions&logoColor=white)](https://github.com/KaikiDeishuuu/FinArch/actions)
[![License](https://img.shields.io/badge/License-MIT-green?style=flat-square)](LICENSE)

**[在线访问 →](https://fund.wulab.tech)**

</div>

---

## 功能概览

### 📒 收支记录
- 支持**收入 / 支出**两个方向，来源区分公司账户与个人垫付
- 记录金额、币种（CNY / USD / EUR / JPY / GBP）、分类、所属项目、备注
- 支持上传附件（发票/收据），可按上传状态筛选
- 日期默认今日，支持自定义

### 💸 报销管理
- 一键标记单笔记录为「已报销」，个人待报销金额在总览页实时汇总
- **智能金额匹配**：输入报销单总额，自动找出金额完全吻合的交易组合（多重背包算法），简化凑单对账流程

### 📊 统计分析
- 年度 / 月度收支趋势柱状图
- 分类支出占比饼图
- 项目维度收支汇总表
- 净结余计算自动扣除已报销部分

### 👥 多用户
- 每个账号数据相互独立，互不可见
- 注册 / 登录支持 [Cloudflare Turnstile](https://dash.cloudflare.com/?to=/:account/turnstile) 人机验证（可选）

### 📄 导出
- 一键导出当前筛选结果为 **PDF**，含用户信息、收支汇总及完整明细表

### 🔐 账户安全
- 邮箱验证注册，支持密码重置（链接有效期 1 小时）
- **邮箱变更双重验证**：先向旧邮箱发送授权确认，再向新邮箱发送最终验证，防止越权修改
- **密码修改即时踢出**：密码变更后其他所有设备的 Token 立即失效，强制重新登录
- 密码强度实时校验；用户名支持修改

---

## 技术栈

| 层 | 技术 |
|---|---|
| 后端 | Go 1.24 · Gin · SQLite (WAL 模式) |
| 前端 | React 19 · Vite 7 · Tailwind CSS v4 · Recharts |
| 容器 | Docker 多阶段构建 · Nginx 反代 |
| 安全 | JWT (HMAC-HS256 · 15 min TTL) · Cloudflare Turnstile |
| 邮件 | [Resend](https://resend.com) API |
| CI/CD | GitHub Actions → GHCR · Watchtower 自动滚动更新 |
| 备份 | Litestream 实时流式备份至 Cloudflare R2（可选） |

---

## 本地开发

**前置条件**：Go 1.24+、Node.js 20+

```bash
# 1. 克隆
git clone https://github.com/KaikiDeishuuu/FinArch.git
cd FinArch

# 2. 启动后端（监听 :8080）
go run ./cmd/cli serve

# 3. 另开终端启动前端（监听 :5173）
cd frontend
npm install
npm run dev
```

前端开发服务器已配置 `/api` 代理至 `http://localhost:8080`，无需手动修改跨域设置。

> 本地开发时无需配置邮件服务，所有环境变量均可留空，相关功能自动降级跳过。

---

## 生产部署

详细部署步骤见 [DEPLOYMENT.md](DEPLOYMENT.md)，以下为快速概览。

### 环境变量（`.env`）

```env
# ── 必填 ──────────────────────────────────────────────────────
JWT_SECRET=                         # 建议：openssl rand -hex 32
APP_BASE_URL=https://yourdomain.com

# ── 邮箱服务（留空则禁用邮箱验证，注册后直接登录）────────────
RESEND_API_KEY=                     # 获取：https://resend.com → API Keys
RESEND_FROM_EMAIL=hello@yourdomain.com  # 须在 Resend 控制台已验证的域名

# ── Cloudflare Turnstile 人机验证（留空则跳过）────────────────
TURNSTILE_SITE_KEY=
TURNSTILE_SECRET=

# ── Litestream 备份至 R2（可选，留空则不备份）────────────────
LITESTREAM_ACCESS_KEY_ID=
LITESTREAM_SECRET_ACCESS_KEY=
LITESTREAM_BUCKET=
LITESTREAM_ENDPOINT=
```

### 首次部署

```bash
# 1. 克隆仓库
git clone https://github.com/KaikiDeishuuu/FinArch.git ~/FinArch
cd ~/FinArch

# 2. 配置环境变量
cp .env.example .env   # 或手动创建 .env，填写上方变量

# 3. 拉起服务（镜像由 GitHub Actions 预构建，服务器无需安装编译工具）
docker compose up -d

# 启用实时备份（需配置 Litestream 变量）
docker compose --profile backup up -d
```

服务绑定 `127.0.0.1:8080`，通过 Nginx 反代对外提供 HTTPS。

### CI/CD 自动更新

```
git push
  └─→ GitHub Actions 构建并推送镜像至 ghcr.io/kaikideishuuu/finarch:latest
                            ↓（每 5 分钟）
              Watchtower 检测到新镜像，自动 pull + 无缝重启容器
```

VPS 端无需任何手动操作。

---

## 项目结构

```
FinArch/
├── cmd/cli/                    命令行入口（serve 子命令）
├── internal/
│   ├── domain/
│   │   ├── model/              领域模型（User, Transaction, Tag…）
│   │   ├── repository/         仓储接口定义
│   │   └── service/            业务逻辑（Auth, Transaction, Stats, Matching…）
│   ├── infrastructure/
│   │   ├── auth/               JWT 签发与验证
│   │   ├── db/                 SQLite + 迁移脚本（v1–v8）
│   │   ├── email/              Resend 邮件发送
│   │   └── repository/         SQLite 仓储实现
│   └── interface/apiv1/        Gin 路由与 Handler
├── frontend/src/
│   ├── api/                    API 客户端 + TypeScript 类型定义
│   ├── components/             公共组件（Layout、图表等）
│   ├── pages/                  页面（Dashboard, Transactions, Stats, Settings…）
│   └── utils/                  工具函数（PDF 导出、格式化等）
├── .github/workflows/          GitHub Actions CI/CD
├── docker-compose.yml          生产编排配置
├── Dockerfile                  多阶段构建（Go + Node → 单一镜像）
├── litestream.yml              Litestream 备份配置
└── DEPLOYMENT.md               详细部署文档
```

---

## 开源协议

本项目基于 [MIT License](LICENSE) 开源。
