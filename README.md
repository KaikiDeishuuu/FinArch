# FinArch

科研经费管理系统 — Go + SQLite + React 19 + Tailwind v4

**在线访问：** https://fund.wulab.tech

## 功能

- **收支记录**：支持公司账户与个人垫付，记录金额、分类、项目、备注
- **报销跟踪**：一键标记已报销，个人待报销金额实时汇总
- **子集匹配**：自动寻找与报销单总额精确匹配的交易组合（多重背包算法）
- **统计分析**：年度收支趋势、分类支出饼图、项目汇总，净结余自动抵消已报销金额
- **账户设置**：修改密码

## 技术栈

| 层 | 技术 |
|---|---|
| 后端 | Go 1.24 · Gin · SQLite（CGO + WAL） |
| 前端 | React 19 · Vite 7 · Tailwind CSS v4 · Recharts |
| 容器 | Docker multi-stage build · Nginx 反代 |
| CI/CD | GitHub Actions → GHCR · Watchtower 自动更新 |

## 本地开发

```bash
# 后端
go run ./cmd/cli serve          # http://localhost:8080

# 前端（另开终端）
cd frontend
npm install
npm run dev                     # http://localhost:5173
```

## 生产部署（首次）

```bash
# 1. 在服务器上克隆仓库
git clone https://github.com/KaikiDeishuuu/FinArch.git ~/FinArch
cd ~/FinArch

# 2. 配置环境变量
echo "JWT_SECRET=$(openssl rand -hex 32)" > .env

# 3. （可选）登录 GHCR，或将镜像包设为公开
echo "YOUR_PAT" | docker login ghcr.io -u KaikiDeishuuu --password-stdin

# 4. 启动（镜像由 GitHub Actions 预构建，不在服务器上编译）
docker compose up -d
```

服务监听 `127.0.0.1:8080`，通过 Nginx 反代对外提供服务。

## CI/CD 流程

```
git push → GitHub Actions 构建镜像
         → 推送至 ghcr.io/kaikideishuuu/finarch:latest
                        ↓ 每 5 分钟
              Watchtower 检测到新镜像
                        ↓
              自动 pull + 无缝重启容器
```

VPS 无需安装构建工具，也无需手动重启。

## 项目结构

```
cmd/cli/             命令行入口（serve 子命令）
internal/
  domain/
    model/           领域模型（Transaction, Reimbursement...）
    repository/      仓储接口定义
    service/         业务逻辑（Auth, Stats, Reimbursement...）
  infrastructure/
    db/              SQLite 迁移脚本
    repository/      SQLite 仓储实现
  interface/apiv1/   Gin 路由与 Handler
frontend/src/
  api/               API 客户端 + 类型定义
  components/        Layout 等公共组件
  pages/             各页面（Transactions, Stats, Match, Settings...）
.github/workflows/   GitHub Actions CI/CD
```
