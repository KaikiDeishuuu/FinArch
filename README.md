<div align="center">

```
  ███████╗██╗███╗   ██╗ █████╗ ██████╗  ██████╗██╗  ██╗
  ██╔════╝██║████╗  ██║██╔══██╗██╔══██╗██╔════╝██║  ██║
  █████╗  ██║██╔██╗ ██║███████║██████╔╝██║     ███████║
  ██╔══╝  ██║██║╚██╗██║██╔══██║██╔══██╗██║     ██╔══██║
  ██║     ██║██║ ╚████║██║  ██║██║  ██║╚██████╗██║  ██║
  ╚═╝     ╚═╝╚═╝  ╚═══╝╚═╝  ╚═╝╚═╝  ╚═╝ ╚═════╝╚═╝  ╚═╝
```

**轻量级多用户收支与报销管理系统**

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

### 收支记录
- 支持**收入 / 支出**两个方向，来源区分公司账户与个人垫付
- 记录金额、币种（CNY / USD / EUR / JPY / GBP）、分类、所属项目、备注
- 支持上传附件（发票/收据），可按上传状态筛选
- 日期默认今日，支持自定义

### 报销管理
- 一键标记单笔记录为「已报销」
- 个人待报销金额在总览页实时汇总
- **智能金额匹配**：输入报销单总额，自动找出金额完全吻合的交易组合（多重背包算法），简化凑单对账流程

### 统计分析
- 年度月度收支趋势柱状图
- 分类支出占比饼图
- 项目维度收支汇总表
- 净结余计算自动扣除已报销部分

### 多用户
- 每个账号数据相互独立，互不可见
- 登录支持 Cloudflare Turnstile 人机验证（可选）

### 导出
- 一键导出当前筛选结果为 **PDF**，含用户信息、收支汇总及完整明细表

---

## 技术栈

| 层 | 技术 |
|---|---|
| 后端 | ![Go](https://img.shields.io/badge/Go_1.24-00ADD8?style=flat-square&logo=go&logoColor=white) ![Gin](https://img.shields.io/badge/Gin-008ECF?style=flat-square&logo=go&logoColor=white) ![SQLite](https://img.shields.io/badge/SQLite_WAL-003B57?style=flat-square&logo=sqlite&logoColor=white) |
| 前端 | ![React](https://img.shields.io/badge/React_19-61DAFB?style=flat-square&logo=react&logoColor=black) ![Vite](https://img.shields.io/badge/Vite_7-646CFF?style=flat-square&logo=vite&logoColor=white) ![Tailwind](https://img.shields.io/badge/Tailwind_v4-06B6D4?style=flat-square&logo=tailwindcss&logoColor=white) ![Recharts](https://img.shields.io/badge/Recharts-22B5BF?style=flat-square) |
| 容器 | ![Docker](https://img.shields.io/badge/Docker-2496ED?style=flat-square&logo=docker&logoColor=white) ![Nginx](https://img.shields.io/badge/Nginx-009639?style=flat-square&logo=nginx&logoColor=white) |
| 安全 | ![JWT](https://img.shields.io/badge/JWT-000000?style=flat-square&logo=jsonwebtokens&logoColor=white) ![Cloudflare](https://img.shields.io/badge/Turnstile_CAPTCHA-F38020?style=flat-square&logo=cloudflare&logoColor=white) |
| CI/CD | ![GitHub Actions](https://img.shields.io/badge/GitHub_Actions-2088FF?style=flat-square&logo=githubactions&logoColor=white) ![GHCR](https://img.shields.io/badge/GHCR-181717?style=flat-square&logo=github&logoColor=white) ![Watchtower](https://img.shields.io/badge/Watchtower-auto_update-blue?style=flat-square) |

---

## 本地开发

```bash
# 后端（默认监听 :8080）
go run ./cmd/cli serve

# 前端（另开终端，默认监听 :5173）
cd frontend
npm install
npm run dev
```

前端开发服务器已配置 `/api` 代理至 `http://localhost:8080`，无需手动修改跨域设置。

---

## 生产部署

### 环境变量（`.env`）

```env
# 必填：JWT 签名密钥
JWT_SECRET=your-random-secret

# 可选：Cloudflare Turnstile（留空则跳过人机验证）
TURNSTILE_SITE_KEY=
TURNSTILE_SECRET=
```

### 首次部署

```bash
# 1. 克隆仓库
git clone https://github.com/KaikiDeishuuu/FinArch.git ~/FinArch
cd ~/FinArch

# 2. 配置环境变量
echo "JWT_SECRET=$(openssl rand -hex 32)" > .env

# 3. 拉起服务（镜像由 GitHub Actions 预构建，无需在服务器上编译）
docker compose up -d
```

服务绑定 `127.0.0.1:8080`，通过 Nginx 反代对外提供 HTTPS 服务。

### CI/CD 流程

```
git push
  └─→ GitHub Actions 构建镜像
        └─→ 推送至 ghcr.io/kaikideishuuu/finarch:latest
                          ↓（每 5 分钟）
              Watchtower 检测到新镜像
                          ↓
              自动 pull + 无缝重启容器
```

VPS 端无需安装构建工具，也无需手动操作。

---

## 项目结构

```
cmd/cli/                    命令行入口（serve 子命令）
internal/
  domain/
    model/                  领域模型（User, Transaction, Reimbursement…）
    repository/             仓储接口定义
    service/                业务逻辑（Auth, Transaction, Stats, Matching, Reimbursement）
  infrastructure/
    db/                     SQLite 迁移脚本
    repository/             SQLite 仓储实现
  interface/apiv1/          Gin 路由与 Handler
frontend/src/
  api/                      API 客户端 + TypeScript 类型定义
  components/               公共组件（Layout 等）
  pages/                    页面（Dashboard, Transactions, Stats, Match, Settings…）
  utils/                    工具函数（PDF 导出等）
.github/workflows/          GitHub Actions CI/CD
docker-compose.yml          生产编排配置
Dockerfile                  多阶段构建（Go + Node → 单一镜像）
```
