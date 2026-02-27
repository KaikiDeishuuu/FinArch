# FinArch

科研经费管理系统 — Go + SQLite + React 19 + Tailwind v4

**在线访问：** https://fund.wulab.tech

## 功能

- 收支记录管理（个人垫付 / 公司账户）
- 上传与报销状态跟踪
- 子集匹配算法：自动寻找与报销总额精确匹配的交易组合
- 月度趋势、分类与项目统计图表

## 技术栈

| 层 | 技术 |
|---|---|
| 后端 | Go 1.24 · Gin · SQLite（WAL） |
| 前端 | React 19 · Vite 7 · Tailwind CSS v4 |
| 部署 | Docker multi-stage build · Nginx 反代 |

## 本地开发

```bash
# 后端
cp .env.example .env   # 填写 JWT_SECRET
go run ./cmd/server

# 前端（另开终端）
cd frontend
npm install
npm run dev            # http://localhost:5173
```

## Docker 部署

```bash
echo "JWT_SECRET=$(openssl rand -hex 32)" > .env
docker compose up --build -d
```

服务监听 `127.0.0.1:8080`，通过 Nginx 反代对外提供服务。

## 项目结构

```
cmd/server/          HTTP 服务入口
internal/
  domain/            领域模型、服务、仓储接口
  infrastructure/    SQLite 实现、JWT、数据库迁移
  interface/apiv1/   Gin 路由与 Handler
frontend/src/        React 前端
```
