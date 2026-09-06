# FinArch Frontend

FinArch 的 React 19 单页应用，包含 WORK / LIFE 财务模式、浅色 / 深色 / 跟随系统主题、中英文界面和可安装 PWA。

## 技术栈

- React 19 + TypeScript 5.9（strict）
- Vite 8 + Tailwind CSS 4
- React Router 7 + TanStack Query / Virtual
- Framer Motion + Recharts
- react-i18next
- Playwright
- vite-plugin-pwa / Workbox

## 本地开发

```bash
npm install
npm run dev
```

Vite 在 `http://localhost:5173` 启动，并将 `/api` 代理到本地后端。代理必须保留 `changeOrigin: false`，以兼容服务端的 Origin / Host CSRF 校验。

## 常用命令

```bash
npm run lint          # ESLint
npm test              # Node 源码与策略测试
npm run build         # TypeScript 检查 + 生产构建 + PWA 生成
npm run test:e2e      # Playwright 端到端测试
npm run pwa:assets    # 由 SVG / HTML 来源重新生成 PWA PNG 资源
npm run preview       # 预览生产构建
```

`pwa:assets` 使用锁定版本的 Geist 和 Noto Sans SC 字体，因此不同构建机生成的文字排版一致。命令会先确保对应的 Playwright Chromium 已安装，再生成资源：

```bash
npm run pwa:assets
```

## 设计系统

- 全局语义 token 和主题变量位于 `src/index.css`。
- React 共享原语位于 `src/components/ui/`。
- 品牌和版本常量位于 `src/constants/`。
- 主字体为 Geist + Noto Sans SC；金额使用等宽数字。
- 主操作使用近黑色；蓝色保留给链接、焦点、选中态和活动导航；绿 / 红 / 琥珀分别表示收入或成功、支出或危险、警告或待处理。
- WORK / LIFE 只改变局部模式色，不改变整页主题。

主题保存在 `finarch-theme`，业务模式保存在 `finarch_mode`，语言保存在 `finarch-lang`。不要更改这些键，也不要移除布局中的 `.scroll-main`，交易虚拟列表依赖该滚动容器。

## PWA 约束

- 注册更新策略为提示用户确认（`registerType: 'prompt'`）。
- `/api` 始终使用 Workbox `NetworkOnly`，不得缓存。
- 页面导航 fallback 为 `/index.html`，并排除 `/api`。
- `index.html` 中的预绘主题 / 语言检测、`#splash` 和 `.splash-fade-out` 是首屏启动流程的一部分。
- `public/*.svg` 是图标和窄屏商店截图的可维护来源，PNG 是通过 `npm run pwa:assets` 生成的发布资源。

## 安全边界

- Access token 只保存在进程内存中；不得写入 localStorage。
- Refresh token 保持为 HttpOnly cookie。
- 一次性操作 token 会先从 URL 移除，再由用户明确点击后提交。
- `index.html` 的 `referrer=no-referrer` 不得移除。
