# FinArch 部署与运维手册

简体中文 | [English](DEPLOYMENT.en.md)

## 目录
- [环境要求](#环境要求)
- [首次部署](#首次部署)
- [环境变量说明](#环境变量说明)
- [Nginx 反向代理](#nginx-反向代理)
- [数据备份](#数据备份)
  - [方案一：Litestream 实时备份到 Cloudflare R2](#方案一litestream-实时备份到-cloudflare-r2)
  - [方案二：应用内手动备份](#方案二应用内手动备份)
- [数据恢复](#数据恢复)
  - [通过应用界面恢复](#通过应用界面恢复)
  - [从 R2 灾难恢复](#从-r2-灾难恢复)
  - [通过网页灾难恢复（无需登录）](#通过网页灾难恢复无需登录)
- [.env 安全备份](#env-安全备份)
- [日常运维命令](#日常运维命令)
- [更新部署](#更新部署)
- [故障排查](#故障排查)

---

## 环境要求

- Docker >= 24
- Docker Compose >= 2.20
- 已配置 Nginx（用于反向代理 + HTTPS）

---

## 首次部署

```bash
# 1. 克隆代码
git clone https://github.com/KaikiDeishuuu/FinArch.git
cd FinArch

# 2. 创建 .env（参考下方变量说明）
cp .env.example .env   # 如不存在则手动新建
nano .env

# 3. 启动服务
docker compose up -d

# 4. 查看运行状态
docker compose ps
docker logs finarch-api -f
```

---

## 环境变量说明

在项目根目录创建 `.env` 文件：

```env
# ── 必填 ────────────────────────────────────────────
# JWT 签名密钥，建议使用随机字符串
# 生成命令：openssl rand -hex 32
JWT_SECRET=your-secret-here

# ── Cloudflare Turnstile 人机验证（可选）────────────
# 留空则禁用验证码，本地开发时无需填写
# 获取：https://dash.cloudflare.com/?to=/:account/turnstile
TURNSTILE_SECRET=
TURNSTILE_SITE_KEY=

# ── Litestream R2 实时备份（可选）───────────────────
# 仅在使用 --profile backup 启动时生效
# 获取：Cloudflare Dashboard → R2 → Manage API Tokens
LITESTREAM_ACCESS_KEY_ID=
LITESTREAM_SECRET_ACCESS_KEY=
LITESTREAM_BUCKET=finarch-backup
# 格式：https://<Account ID>.r2.cloudflarestorage.com
LITESTREAM_ENDPOINT=https://xxxxxxxx.r2.cloudflarestorage.com

# ── 邮件发送 / 邮箱验证（可选）──────────────────────
# 留空则禁用邮箱验证，注册后直接登录（与旧版行为一致）
# 获取 API Key：https://resend.com → API Keys
RESEND_API_KEY=
# 发件人地址（须在 Resend 控制台中已验证的域名下的地址）
RESEND_FROM_EMAIL=hello@yourdomain.com
# 应用外部访问 URL（用于邮件中的验证/重置链接）
APP_BASE_URL=https://yourdomain.com
```

---

## Nginx 反向代理

```nginx
server {
    listen 443 ssl;
    server_name farc.dev;

    ssl_certificate     /path/to/cert.pem;
    ssl_certificate_key /path/to/key.pem;

    location / {
        proxy_pass         http://127.0.0.1:8080;
        proxy_set_header   Host $host;
        proxy_set_header   X-Real-IP $remote_addr;
        proxy_set_header   X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header   X-Forwarded-Proto $scheme;
        client_max_body_size 100m;   # 用于备份文件上传
    }
}

server {
    listen 80;
    server_name farc.dev;
    return 301 https://$host$request_uri;
}
```

---

## 邮箱验证与密码重置

> 此功能为**可选**。不配置 `RESEND_API_KEY` 时，注册后直接登录，行为与旧版完全相同。

### 配置步骤

1. 注册 [Resend](https://resend.com) 并创建 API Key。
2. 在 Resend 控制台验证你的发件域名（DNS 添加 DKIM、SPF 记录）。
3. 在 `.env` 中填写：

   ```env
   RESEND_API_KEY=re_xxxxxxxxxxxx
   RESEND_FROM_EMAIL=hello@yourdomain.com
   APP_BASE_URL=https://yourdomain.com
   ```

4. 重启服务：`docker compose up -d`

### 行为说明

| 场景 | 已配置 RESEND_API_KEY | 未配置 |
|------|----------------------|--------|
| 注册 | 发送验证邮件，需点击链接激活 | 直接登录 |
| 登录（未验证） | 返回 403，可重发验证邮件 | 不适用 |
| 忘记密码 | 发送重置链接（1 小时有效） | 入口隐藏 |

### 已有用户

数据库迁移（v5）将 `email_verified` 默认值设为 `1`，**已有用户不受影响**，无需重新验证。

---

## 数据备份

### 方案一：Litestream 实时备份到 Cloudflare R2

**前置条件：** 已在 Cloudflare 创建 R2 存储桶并获取 API Token，`.env` 中已填写 `LITESTREAM_*` 变量。

```bash
# 启动（初次或每次更新后）
docker compose --profile backup up -d

# 验证同步状态（看到 "snapshot complete" 即正常）
docker logs finarch-litestream -f
```

**同步频率：** WAL 写入后约 1 秒内上传，快照每 1 小时整合一次，保留最近 7 天数据。

**R2 存储桶结构：**
```
finarch-backup/
  finarch/
    generations/
      <id>/
        snapshots/   ← 全量快照
        wal/         ← 增量 WAL 段（自动清理过期数据）
```

---

### 方案二：应用内手动备份

登录后进入 **设置页** → **数据备份** → 点击「下载备份」，将在本地保存一份当前数据库的 `.db` 快照文件。

建议：**每次重大操作前手动下载一份**，存入密码管理器附件或个人加密存储。

---

## 数据恢复

### 通过应用界面恢复

1. 进入 **设置页** → **数据恢复**
2. 选择之前下载的 `.db` 备份文件
3. 阅读警告后点击「我已了解，确认恢复」
4. 点击「立即恢复数据」

> 恢复操作会覆盖当前所有数据，且无法撤销，操作前建议先下载一份当前备份。

---

### 从 R2 灾难恢复

当 VPS 数据全部丢失（磁盘损坏/误删/迁移新机器）时：

```bash
# 在新机器上克隆代码并配置好 .env
git clone https://github.com/KaikiDeishuuu/FinArch.git
cd FinArch
# 填写 .env（包含 LITESTREAM_* 变量）

# 从 R2 恢复数据库到本地 volume
docker run --rm \
  -v finarch_finarch-data:/data \
  -v $(pwd)/litestream.yml:/etc/litestream.yml:ro \
  -e LITESTREAM_ACCESS_KEY_ID=${LITESTREAM_ACCESS_KEY_ID} \
  -e LITESTREAM_SECRET_ACCESS_KEY=${LITESTREAM_SECRET_ACCESS_KEY} \
  -e LITESTREAM_BUCKET=${LITESTREAM_BUCKET} \
  -e LITESTREAM_ENDPOINT=${LITESTREAM_ENDPOINT} \
  litestream/litestream:latest \
  restore -config /etc/litestream.yml /data/finarch.db

# 恢复完成后启动服务
docker compose --profile backup up -d
```

---

### 通过网页灾难恢复（无需登录）

当服务器数据全部丢失（包括认证数据）时，用户可通过本地 `.db` 备份文件在未登录状态下恢复数据：

1. 在登录页点击 **「灾难恢复」** 链接
2. 上传 `.db` 备份文件
3. 系统会向备份文件中的邮箱地址发送 6 位验证码
4. 输入验证码完成恢复

> 此公开端点已启用频率限制并要求邮箱验证，以防止滥用。

---

## .env 安全备份

`.env` 包含所有密钥，**绝不能明文提交 Git 或上传公开存储**。

**推荐方式（任选其一）：**

1. **密码管理器**（最简单）：将完整 `.env` 内容作为 Secure Note 存入 Bitwarden / 1Password
2. **GPG 加密后存本地：**
   ```bash
   # 在 VPS 上加密并下载到本地
   gpg --symmetric --cipher-algo AES256 -o env_backup.gpg .env
   scp root@farc.dev:~/FinArch/env_backup.gpg ~/
   # 本地妥善保存 env_backup.gpg，解密时：gpg -o .env env_backup.gpg
   ```
3. **直接 SCP 到本地后删除：**
   ```bash
   scp root@farc.dev:~/FinArch/.env ~/finarch_env.txt
   # 保存后立即存入密码管理器，删除本地明文
   ```

---

## 日常运维命令

```bash
# 查看服务状态
docker compose ps
docker compose --profile backup ps

# 查看日志
docker logs finarch-api -f
docker logs finarch-litestream -f

# 重启服务
docker compose restart api
docker compose --profile backup restart litestream

# 停止所有服务
docker compose --profile backup down

# 查看数据库文件大小
docker exec finarch-api ls -lh /data/finarch.db

# 手动进入数据库（调试用）
docker run --rm -it \
  -v finarch_finarch-data:/data \
  keinos/sqlite3 sqlite3 /data/finarch.db
```

---

## 更新部署

```bash
cd ~/FinArch

# 拉取最新代码
git pull

# 拉取最新镜像并重启
docker compose --profile backup pull
docker compose --profile backup up -d

# 确认新版本运行正常
docker compose ps
docker logs finarch-api --tail 20
```

---

## 故障排查

| 现象 | 排查步骤 |
|------|---------|
| 页面无法访问 | `docker compose ps` 确认容器状态；`docker logs finarch-api` 查看错误 |
| API 返回 401 | 检查 `JWT_SECRET` 是否与之前一致（更改后所有 Token 失效）|
| Turnstile 验证一直失败 | 检查 `TURNSTILE_SECRET` / `TURNSTILE_SITE_KEY` 是否与域名匹配 |
| Litestream 容器退出 | `docker logs finarch-litestream` 查看错误；常见原因是 R2 凭据错误 |
| 备份下载失败 | 确认后端正常运行；数据库文件未损坏 |
| 恢复后数据未变化 | 刷新页面；确认上传的是 `.db` 格式的有效 SQLite 文件 |
| 容器 unhealthy | 通常为启动中状态，等待约 15 秒后自动变为 healthy |
| PWA 显示旧内容 | 在浏览器设置中清除站点数据，或卸载后重新安装 PWA |
