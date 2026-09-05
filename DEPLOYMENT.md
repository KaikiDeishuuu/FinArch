# FinArch 部署与运维手册

简体中文 | [English](DEPLOYMENT.en.md)

## 目录
- [环境要求](#环境要求)
- [首次部署](#首次部署)
- [环境变量说明](#环境变量说明)
- [Nginx 反向代理](#nginx-反向代理)
- [浏览器会话安全边界](#浏览器会话安全边界)
- [邮箱验证与密码重置](#邮箱验证与密码重置)
- [数据备份](#数据备份)
  - [方案一：Litestream 实时备份到 Cloudflare R2](#方案一litestream-实时备份到-cloudflare-r2)
  - [方案二：维护窗口物理备份](#方案二维护窗口物理备份)
- [数据恢复](#数据恢复)
  - [维护窗口上传恢复](#维护窗口上传恢复)
  - [从 R2 灾难恢复](#从-r2-灾难恢复)
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

# 2. 创建 .env（参考下方变量说明；必须填写完整可信代理链的 FINARCH_TRUSTED_PROXY_CIDRS）
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
# access JWT 签名及服务端会话密钥派生根密钥，必须使用随机字符串
# 生成命令：openssl rand -hex 32
JWT_SECRET=your-secret-here

# 要部署的不可变 GHCR 镜像标签；生产环境使用 sha-<commit sha>
FINARCH_IMAGE_TAG=sha-your-commit-sha

# ── 系统级数据库运维（默认关闭）──────────────────────
# 仅在维护窗口设为 true；secret 至少 32 字符、不得复用 JWT_SECRET，并通过
# X-FinArch-Operations-Secret 请求头提供。
FINARCH_ENABLE_SYSTEM_OPERATIONS=false
FINARCH_SYSTEM_OPERATIONS_SECRET=

# Compose 固定 FINARCH_BEHIND_PROXY=true。列出应用实际看到的直连代理，以及
# X-Forwarded-For 中真实客户端右侧的每个受控中间代理，否则会按错误的共享 IP 限流。
# 仅用最窄的 /32、/128 或代理商正式 CIDR；/0、客户端网段和不受控网段均不可用。
FINARCH_TRUSTED_PROXY_CIDRS=<直连代理 IP>/32[,<受控上游代理 CIDR>...]

# 仅供显式 Authorization bearer API 使用；浏览器 cookie 会话不支持跨域。
# 不要在代理层补 Access-Control-Allow-Credentials。
# FINARCH_CORS_ALLOWED_ORIGINS=https://app.example.com

# Docker Compose 的备份/恢复临时空间上限；默认 512m，提高前确认主机内存充足。
# FINARCH_TMPFS_SIZE=512m

# ── 生产 HTTP 生命周期（可选）────────────────────────
# 100 MiB 恢复/备份与长耗时 OCR 需要宽裕的 body 读取和响应写入窗口。
# FINARCH_HTTP_READ_HEADER_TIMEOUT=10s
# FINARCH_HTTP_READ_TIMEOUT=30m
# FINARCH_HTTP_WRITE_TIMEOUT=30m
# FINARCH_HTTP_IDLE_TIMEOUT=2m
# FINARCH_HTTP_SHUTDOWN_TIMEOUT=5m
# FINARCH_HTTP_MAX_HEADER_BYTES=65536

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

# ── 附件存储保护 ────────────────────────────────────────
FINARCH_ATTACHMENT_MAX_BYTES=20971520
FINARCH_ATTACHMENT_MAX_FILES_PER_USER=500
FINARCH_ATTACHMENT_MAX_TOTAL_BYTES_PER_USER=1073741824
FINARCH_ATTACHMENT_UPLOADS_PER_MINUTE=30
# 超时且未关联交易的附件会先进入耐久删除队列；清理任务使用数据库写租约
FINARCH_ATTACHMENT_ORPHAN_TTL=24h
FINARCH_ATTACHMENT_ORPHAN_CLEANUP_INTERVAL=1h
FINARCH_ATTACHMENT_ORPHAN_CLEANUP_BATCH_SIZE=100

# ── 附件 OCR（可选）────────────────────────────────────
# none：关闭；paddle：HTTP sidecar；paddle_aistudio：PaddleOCR AIStudio 云 API
FINARCH_OCR_PROVIDER=none
# 使用 PaddleOCR AIStudio 时开启下面配置；真实 token 只放服务器 .env，不要提交到 Git
# FINARCH_OCR_PROVIDER=paddle_aistudio
# FINARCH_OCR_AISTUDIO_TOKEN=
# FINARCH_OCR_AISTUDIO_MODEL=PaddleOCR-VL-1.6
# FINARCH_OCR_AISTUDIO_JOB_URL=https://paddleocr.aistudio-app.com/api/v2/ocr/jobs
# 若返回结果位于其他对象存储域名，必须显式列出（逗号分隔，支持 host 或 host:port）
# FINARCH_OCR_AISTUDIO_ALLOWED_RESULT_HOSTS=
# FINARCH_OCR_AISTUDIO_OPTIONAL_PAYLOAD={"useDocOrientationClassify":false,"useDocUnwarping":false,"useChartRecognition":false}
# FINARCH_OCR_AISTUDIO_POLL_INTERVAL=5s
# FINARCH_OCR_AISTUDIO_MAX_RESULT_BYTES=10485760
# FINARCH_OCR_TIMEOUT=2m
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
        # 会话 Origin 校验依赖客户端原始 Host；不要改写或丢弃端口。
        proxy_set_header   Host $http_host;
        proxy_set_header   X-Real-IP $remote_addr;
        # 本示例的 Nginx 是公网第一层，必须覆盖客户端伪造或畸形的转发头。
        proxy_set_header   X-Forwarded-For $remote_addr;
        proxy_set_header   X-Forwarded-Proto $scheme;
        client_max_body_size 101m;   # 与应用 100 MiB 文件 + multipart 开销上限匹配
        client_body_timeout 30m;
        proxy_send_timeout 30m;
        proxy_read_timeout 30m;
    }
}

server {
    listen 80;
    server_name farc.dev;
    return 301 https://$host$request_uri;
}
```

上例假设 Nginx 直接接收不可信客户端流量，因此把 `X-Forwarded-For` 覆盖为单个
`$remote_addr`；此时 `FINARCH_TRUSTED_PROXY_CIDRS` 只需包含应用实际看到的该 Nginx
直连地址。不要在公网第一层未经清洗就使用 `$proxy_add_x_forwarded_for`，否则客户端可
注入畸形 hop，让应用保守退回共享的直连代理限流桶。

若 Nginx 前还有 CDN、负载均衡器或其他代理，可二选一：让 Nginx 只接受并验证这些上游，
解析出最终客户端后仍覆盖为单个地址；或保留已经由最外层清洗的完整链，并把应用直连代理
以及链中真实客户端右侧的每个受控中间代理都加入 `FINARCH_TRUSTED_PROXY_CIDRS`。应用先
验证完整 XFF 链，再从右向左取首个不可信地址；任一空白/畸形 hop 都会使整链失效并退回
直连 peer。只要 XFF 存在，就不会改从可能冲突的 `X-Real-IP` 取值。

应用默认只给请求头 10 秒，并将 header 限制为 64 KiB；完整请求读取和响应写入窗口均为
30 分钟，以容纳 100 MiB 维护恢复/备份及长耗时 OCR。Nginx 的 body/upstream 超时应至少
保持同样长度。收到 `SIGINT`/`SIGTERM` 后，服务停止接收新连接，并最多等待 5 分钟让
在途请求完成；Compose 的 `stop_grace_period` 使用同一配置，超时后才强制关闭连接。

生产服务固定签发 `Secure` cookie，因此公网入口必须全程使用 HTTPS。TLS 可以在直接
连接应用的可信反向代理终止，但代理必须把客户端的原始 `Host` 原样传给应用；否则
登录、注册、刷新和登出的精确 Origin 校验会失败。Compose 固定设置
`FINARCH_BEHIND_PROXY=true`，并要求非空、全部合法的 `FINARCH_TRUSTED_PROXY_CIDRS`；它只决定
是否信任代理提供的真实客户端 IP，用于限流和审计，不会放宽 Origin、cookie 或 CORS。
配置缺失、包含非法项或包含 `0.0.0.0/0`、`::/0` 时，Compose/应用直接拒绝启动。请填写
应用实际看到的直连代理，以及 XFF 可信后缀中的全部受控代理；使用最窄的主机或官方代理
网段，绝不能为了方便信任公网、全部私网、客户端地址空间或其他不受控网段。漏掉中间代理
会让其后的客户端共享该代理的登录与会话防洪额度，信任过宽则会允许伪造地址绕过额度。
直接运行 `cmd/server` 时
`FINARCH_BEHIND_PROXY` 默认 `false` 并忽略转发头；若手动设为 `true`，同样必须提供严格合法的
可信代理列表。布尔值只接受小写 `true` 或 `false`，防止拼写错误静默降级。

---

## 浏览器会话安全边界

- 生产 refresh cookie 名为 `__Host-finarch_refresh`，固定设置 `HttpOnly`、`Secure`、
  `SameSite=Strict`、`Path=/` 且不设置 `Domain`，不向 JavaScript 或 JSON 响应暴露
  refresh token。浏览器只在内存中保存短期 access token，不写入 `localStorage`。
- access token 有效期为 15 分钟；refresh token 滑动有效期为 7 天，会话绝对有效期为
  30 天。refresh 每次使用都会轮换；10 秒重试宽限期内返回同一个后继 token，宽限期后
  重放旧 token 会撤销整个会话族。
- 浏览器的 `POST /auth/login`、`POST /auth/register`、`POST /auth/refresh` 和
  `POST /auth/logout` 必须带与 HTTPS `Host` 精确一致的 `Origin`。前端和 API 必须部署
  在同一个 origin（scheme、host、port 均一致）。
- `FINARCH_CORS_ALLOWED_ORIGINS` 仅允许来自所列 origin 的显式 bearer API 请求；响应
  故意不包含 `Access-Control-Allow-Credentials`，不能用于跨域 cookie 会话，也不应由
  CDN 或反向代理补加 credentials 支持。
- 只有绑定 `127.0.0.1`、`::1` 或 `localhost` 的 `go run ./cmd/cli serve` 开发服务
  可以使用非 `Secure` 的 `finarch_refresh` cookie 并接受无 Origin 的非浏览器客户端。
  不要把这种开发模式暴露到局域网或公网。

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

**同步频率：** WAL 写入后约 1 秒内上传，快照每 30 分钟整合一次，保留最近 30 天数据。

**覆盖范围：** 当前 Litestream 配置只复制 `/data/finarch.db`，**不会**复制
`/data/attachments`。必须另外对附件目录做加密、带版本的备份，并让数据库快照与附件备份
能够对应到同一恢复点。仅有 R2 中的 SQLite 快照不构成完整灾备。

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

### Litestream 健康检查接口

该接口包含系统级备份状态，默认关闭。启用系统运维开关后，请同时提供有效
JWT 和运维密钥请求头：

```
GET /api/v1/backup/litestream-health
X-FinArch-Operations-Secret: <operations-secret>
```

返回最近快照时间、复制延迟秒数和当前 SQLite journal_mode。

### 方案二：维护窗口物理备份

物理备份包含**全部租户、认证数据和密码哈希**，不能作为普通用户的数据导出功能。
需要时先设置 `FINARCH_ENABLE_SYSTEM_OPERATIONS=true` 和独立运维密钥，重启服务，
再由受控运维客户端携带 JWT 与 `X-FinArch-Operations-Secret` 调用备份 API。

完成后立即重新关闭开关，并将备份保存到加密存储。运维密钥只能来自服务端 secret
管理器并由受控 CLI/运维客户端发送，绝不能写入网页、前端构建环境变量、浏览器存储
或普通用户可见的请求。

物理备份会在 `/tmp` 创建数据库快照和归档。默认 512 MiB 适合小型部署；执行前应根据
数据库与附件总量检查临时空间，并在主机内存允许时通过 `FINARCH_TMPFS_SIZE` 调大。

受控客户端应先申请一次性导出令牌，再用 POST 和专用请求头下载；不要把令牌放入 URL，
也不要使用旧的 GET 下载形式：

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

导出令牌短时有效且只能消费一次。服务会拒绝
`GET /api/v1/backup/download` 以及仅通过 `?export_token=...` 传递令牌的请求。

---

## 数据恢复

### 维护窗口上传恢复

上传恢复会覆盖整个 live database，仅适用于停机维护。应用限制上传文件为 100 MiB、
ZIP 解压后总量为 200 MiB；Compose 为 `/tmp` 分配 512 MiB，以容纳一次受控恢复期间的
multipart 临时文件、上传副本及解压内容：

1. 停止公网流量，并先复制当前数据库和附件目录
2. 临时启用系统运维开关并设置独立密钥
3. 确认没有其他恢复任务；如需并发或额外临时空间，先调大 `FINARCH_TMPFS_SIZE`
4. 使用同时带 JWT 和 `X-FinArch-Operations-Secret` 的受控客户端调用恢复 API
5. 校验恢复结果后立即关闭运维开关并轮换密钥

恢复引擎会在替换前通过 SQLite Backup API 创建当前数据库的安全快照，并为将被
覆盖或新增的附件创建回滚日志。恢复、迁移、附件写入或后置完整性校验失败时，系统会
自动回灌数据库并恢复附件。

自动回滚不能替代离线备份：操作前仍应停止流量，并将当前数据库和附件目录复制到
应用数据卷之外。如果自动回滚本身失败，服务会保持维护状态，并在
`/data/safety_backups` 保留权限为 `0600` 的 `failed-restore-*.db` 数据库快照和/或
`failed-restore-attachments-*` 附件日志。此时不要重新开放流量；先复制这些文件，
检查服务日志中的 `RESTORE_ROLLBACK_FAILED`，再由运维人员完成手工恢复并重启服务。

---

### 从 R2 灾难恢复

当 VPS 数据全部丢失（磁盘损坏/误删/迁移新机器）时：

先从独立备份恢复 `/data/attachments`，再恢复与它对应的数据库快照。应用 CLI 会在替换
数据库前按 `size_bytes` 和 `sha256` 核验每个附件；任何文件缺失或不匹配都会拒绝恢复。
直接运行 Litestream 则绕过这层核验，只能用于目标数据库文件不存在的空数据卷，并且在
确认全部附件已恢复且校验完成前不得启动 API。现有数据卷应先离线保存数据库、附件目录及
SQLite sidecar，不要直接覆盖。

```bash
# 在新机器上克隆代码并配置好 .env
git clone https://github.com/KaikiDeishuuu/FinArch.git
cd FinArch
# 填写 .env（包含 LITESTREAM_* 变量）

# 停止 API 和 Litestream；恢复期间不得有进程打开数据库
docker compose --profile backup down

# 先把同一恢复点的附件备份还原到 finarch-data 卷的 /data/attachments
# 并确认 /data/finarch.db、-wal、-shm、-journal 均不存在

# 仅在空数据卷中，从 R2 恢复数据库
docker run --rm \
  -v finarch_finarch-data:/data \
  -v $(pwd)/litestream.yml:/etc/litestream.yml:ro \
  -e LITESTREAM_ACCESS_KEY_ID=${LITESTREAM_ACCESS_KEY_ID} \
  -e LITESTREAM_SECRET_ACCESS_KEY=${LITESTREAM_SECRET_ACCESS_KEY} \
  -e LITESTREAM_BUCKET=${LITESTREAM_BUCKET} \
  -e LITESTREAM_ENDPOINT=${LITESTREAM_ENDPOINT} \
  litestream/litestream:0.3.13 \
  restore -config /etc/litestream.yml /data/finarch.db

# 可选：使用应用 CLI。若目标数据库已存在，它会保留 0600 权限的
# finarch.db.pre-restore.* 安全副本；成功确认后再按保留策略处理。
./app restore --from-r2 --target=/data/finarch.db

# 核对附件校验与数据库完整性成功后再启动服务
docker compose --profile backup up -d
```

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

推送到 `main` 后，只有对应 CI 成功且该提交仍是 `main` 最新提交时，部署工作流才会构建
并部署 `sha-<完整提交 SHA>` 镜像。部署任务串行执行，不使用可变的 `latest` 作为运行标签。
手动更新时也应选定一个已通过 CI 的完整提交 SHA，并使用同一 SHA 更新代码与镜像：

```bash
cd ~/FinArch

# 替换为已通过 CI 的完整 40 位提交 SHA
FINARCH_DEPLOY_SHA=0123456789abcdef0123456789abcdef01234567
git fetch origin main
git switch main
git merge --ff-only "$FINARCH_DEPLOY_SHA"
test "$(git rev-parse HEAD)" = "$FINARCH_DEPLOY_SHA"

# 生产运行标签固定到同一个提交
sed -i "s/^FINARCH_IMAGE_TAG=.*/FINARCH_IMAGE_TAG=sha-${FINARCH_DEPLOY_SHA}/" .env

# 拉取固定镜像并重启
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
| 物理备份失败 | 确认运维开关、JWT 与独立运维请求头均有效，并检查 `/tmp` 临时空间 |
| 物理恢复失败 | 确认处于维护窗口、上传文件有效且未超限，并检查 `/tmp` 临时空间 |
| 容器 unhealthy | 通常为启动中状态，等待约 15 秒后自动变为 healthy |
| PWA 显示旧内容 | 在浏览器设置中清除站点数据，或卸载后重新安装 PWA |
