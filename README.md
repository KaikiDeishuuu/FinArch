<div align="center">

<img src="frontend/public/logo.svg" width="80" height="80" alt="FinArch" />

# FinArch

**收支 · 报销 · 统计**

轻量高效的多用户财务管理系统

[![Go](https://img.shields.io/badge/Go-1.24-00ADD8?style=flat-square&logo=go&logoColor=white)](https://golang.org)
[![React](https://img.shields.io/badge/React-19-61DAFB?style=flat-square&logo=react&logoColor=black)](https://react.dev)
[![License](https://img.shields.io/badge/License-MIT-green?style=flat-square)](LICENSE)

**[在线体验 →](https://farc.dev)**

简体中文 | [English](README.en.md)

</div>

<br/>

## ✨ 功能亮点

<table>
<tr>
<td width="50%">

### 📒 收支记录
记录每一笔收入与支出，区分**个人垫付**与**公共资金**，支持多币种（CNY / USD / EUR / JPY / GBP）、分类、项目归属、备注和附件上传。

### 💸 智能报销
一键标记报销状态，自动汇总待报销金额。输入报销单总额即可**智能匹配**对应交易组合，告别手动凑单。

### 📊 可视化统计
年度/月度收支趋势、分类占比饼图、项目维度汇总——个人与公共账户分别统计，净结余自动扣除已报销部分。支持按来源、账户、类别、项目多维筛选。

</td>
<td width="50%">

### 🏦 多账户管理
自由创建和管理多个个人/公共账户，余额由系统自动维护。总览页一目了然各账户状态。筛选器自动根据来源类型过滤可选账户。

### 💱 实时汇率
接入欧洲央行数据，所有汇总自动折算为人民币。离线时自动降级为内置备用汇率，确保不间断使用。

### 📄 PDF 导出
一键导出筛选结果为精排版 PDF，含品牌水印、用户信息及个人/公共分组统计。

</td>
</tr>
</table>

### 更多特性

- **👥 多用户隔离**：每个账号数据独立，互不可见
- **🔐 企业级安全**：邮箱验证注册 · 密码重置 · 邮箱变更双重验证 · 改密即时踢出全部设备
- **📱 PWA 支持**：可安装至桌面/主屏，原生应用体验
- **☁️ 自动备份**：可选 Litestream 实时流式备份至 Cloudflare R2
- **🛡️ 灾难恢复**：邮箱验证的公开恢复流程，即使 JWT 认证不可用也能恢复数据
- **📡 在线设备监控**：Dashboard 实时显示当前在线设备数量（心跳机制，2 分钟间隔）
- **🤖 人机验证**：可选 Cloudflare Turnstile 防护
- **🧹 自动清理**：未验证账户 24 小时后自动清除，设备心跳 10 分钟超时自动回收

---

## 🚀 快速开始

### 本地开发

> 前置条件：Go 1.24+、Node.js 20+

```bash
git clone https://github.com/KaikiDeishuuu/FinArch.git
cd FinArch

# 启动后端（:8080）
go run ./cmd/cli serve

# 另开终端，启动前端（:5173）
cd frontend && npm install && npm run dev
```

前端已配置 `/api` 代理，开箱即用。本地开发无需配置邮件等环境变量。

### 生产部署

```bash
git clone https://github.com/KaikiDeishuuu/FinArch.git && cd FinArch
cp .env.example .env   # 编辑 .env 填写配置
docker compose up -d
```

主要环境变量：

| 变量 | 说明 | 必填 |
|------|------|:----:|
| `JWT_SECRET` | Token 签名密钥 | ✅ |
| `APP_BASE_URL` | 站点地址 | ✅ |
| `RESEND_API_KEY` / `RESEND_FROM_EMAIL` | 邮件服务 | 可选 |
| `TURNSTILE_SITE_KEY` / `TURNSTILE_SECRET` | 人机验证 | 可选 |
| `LITESTREAM_*` | R2 备份 | 可选 |

> 留空可选变量时，相关功能自动跳过。详细部署指南见 [DEPLOYMENT.md](DEPLOYMENT.md)（[English](DEPLOYMENT.en.md)）。

### CI/CD

```
git push → GitHub Actions 构建镜像 → GHCR → VPS 拉取并重启
```

---

## 🏗 项目结构

```
FinArch/
├── cmd/
│   ├── cli/                 命令行入口（本地开发）
│   ├── server/              生产服务器入口（Docker）
│   └── desktop/             桌面端入口（Wails）
├── internal/
│   ├── domain/
│   │   ├── model/           领域模型
│   │   ├── repository/      仓储接口
│   │   └── service/         业务逻辑服务
│   ├── infrastructure/
│   │   ├── auth/            JWT · 密码 · 限流 · 验证码
│   │   ├── db/              SQLite 迁移与触发器
│   │   ├── email/           邮件发送（Resend）
│   │   ├── repository/      SQLite 仓储实现
│   │   └── plugin/          插件系统
│   └── interface/
│       ├── apiv1/           REST API 路由与处理器
│       └── httpserver/      嵌入式文件服务
├── frontend/src/
│   ├── api/                 Axios API 客户端
│   ├── components/          公共组件（Select · DatePicker · Brand …）
│   ├── contexts/            Auth · ExchangeRate · Config
│   ├── hooks/               useTransactions · useAccounts · useHeartbeat …
│   ├── motion/              Framer Motion 动画系统
│   ├── pages/               页面组件
│   ├── utils/               工具函数（格式化 · 汇率 · PDF 导出）
│   └── workers/             Web Worker（子集匹配）
├── .github/workflows/       CI/CD（Build → GHCR → SSH deploy）
├── docker-compose.yml       生产编排
├── Dockerfile               多阶段构建（Node → Go → Alpine）
└── DEPLOYMENT.md            部署文档
```

---

## 技术栈

| | |
|---|---|
| **后端** | Go 1.24 · Gin · SQLite (WAL) |
| **前端** | React 19 · Vite 7 · Tailwind CSS v4 · Framer Motion · Recharts |
| **部署** | Docker 多阶段构建 · GitHub Actions → GHCR → SSH Deploy |
| **安全** | JWT (HMAC HS256) · Cloudflare Turnstile · IP 限流 · 账户锁定 |
| **邮件** | Resend（验证 · 重置 · 灾难恢复） |
| **备份** | Litestream → Cloudflare R2 · 应用内下载/恢复 · 灾难恢复 |
| **PWA** | Workbox Service Worker · 离线缓存 · 主屏安装 |

---

## 📄 License

[MIT](LICENSE)
