# Custom 分支功能台账

本文记录自定义分支相对上游 `main` 维护的全部功能扩展，供后续同步上游（rebase / merge）时快速排查冲突、检索同类能力与验证回归。

后续新增、调整或下线任何相对上游的功能扩展时，请同步更新本文。

---

## 1. CI / Release 与测试稳定性

- **发布流程不回写默认分支**：移除 `.github/workflows/release.yml` 中的 `sync-version-file` Job，发版不再自动提交默认分支的 `backend/cmd/server/VERSION`。
- **四段式版本号支持**：放宽 `.github/release-tools/release_matrix.py` 的 `VERSION_RE`，支持 `v0.2.24.1` 等自定义四段版本号。
- **全分支 CI 触发**：`.github/workflows/backend-ci.yml` 在 `push` 时匹配 `branches: ['**']`，避免仅在 `main` 触发并跳过 tag 推送触发。
- **CI 测试隔离**：
  - `backend/internal/service/gateway_thinking_filter_precheck_test.go`：通过子进程隔离 `AllocsPerRun` 测量，防止同包后台任务污染分配统计。
  - `backend/internal/service/ratelimit_service_ollama_429_test.go`：放宽异步回调短冷却窗口（`5s` → `10s`），降低共享 runner 抖动。

上游检索关键词：`sync-version-file`、`VERSION_RE`、`SUB2API_THINKING_FILTER_ALLOCS_TEST_CHILD`。

---

## 2. 站点基础定制

### 2.1 站点域名访问守卫
- **配置项**：`site_domain`（留空不启用；支持域名或 `host:port`，保存时统一规范化为小写并去除末尾 `/`）。
- **核心实现**：
  - 统一规范化函数 `service.NormalizeSiteDomain`，支持 `:80` / `:443` 默认端口等价比较。
  - 非规范 Host 访问 `/api/v1/*` 面板接口返回 `403`；访问前端页面、静态资源及 `/setup/status` 返回 `307` 重定向到规范域名；AI 网关路由与 `/health` 不受限制。
  - 设置更新时通过统一回调实时刷新运行中的守卫，无需重启。
- **主要文件**：`backend/internal/server/middleware/site_domain_guard.go`、`backend/internal/service/setting_custom.go`、`backend/internal/server/router.go`、`backend/internal/web/embed_on.go`。

### 2.2 客服二维码图片
- **配置项**：`contact_qr_image_url`（支持 `http://`、`https://` 或 `data:image/` Base64，上限 300 KiB）。
- **校验逻辑**：`backend/internal/service/setting_custom.go` 中的 `ValidateContactQRImageURL` 在后端校验协议白名单与解码大小上限，防止非法协议或超大 Base64 写入公开设置与嵌入式 HTML。
- **展示入口**：顶部用户菜单（`AppHeader.vue`）、个人资料页（`ProfileView.vue`）、兑换页（`RedeemView.vue`），与 `contact_info` 独立展示。

### 2.3 自定义菜单新标签页打开
- **配置项**：`custom_menu_items[*].open_in_new_tab`（复用现有 JSON，不新增数据库字段）。
- **行为**：开启后点击侧边栏自定义菜单以原生 `<a target="_blank" rel="noopener noreferrer">` 在新标签页打开目标 URL 或 Markdown 自定义页。

### 2.4 精简公开首页展示
- **实现方式**：在 `frontend/src/views/HomeView.vue` 使用局部开关常量 `showLandingPromoSections = false` 条件隐藏“订阅转 API”标签与“已支持的 AI 模型”静态展示区，保持与上游首页模板最小 diff。

上游检索关键词：`site_domain`、`NormalizeSiteDomain`、`SiteDomainGuard`、`contact_qr_image_url`、`ValidateContactQRImageURL`、`open_in_new_tab`、`showLandingPromoSections`。

---

## 3. 客户端一键导入（CC-Switch & Magpie）

- **共享协议目标推导**：`frontend/src/utils/providerImportTargets.ts` 统一维护平台与客户端目标映射及 URL 规范化函数。
- **CC-Switch 多端点导入**：主端点 `api_base_url` 与公开设置中的 `custom_endpoints` 合并去重后通过逗号分隔写入 `endpoint`；`homepage` 按 `site_domain` → `api_base_url` → `window.location.origin` 回退。
- **Magpie 一键导入**：`frontend/src/utils/magpieImport.ts` 生成 `https://usemagpie.ai/import#...` 链接，在 `KeysView.vue` 提供一键导入。

上游检索关键词：`getProviderImportTargets`、`buildCcSwitchImportDeeplink`、`buildMagpieImportUrl`、`additionalEndpoints`。

---

## 4. 返利邀请码注册

- **配置项**：`affiliate_code_registration_enabled`（默认关闭）。
- **核心实现**：
  - 开启邀请码注册时允许使用返利码（手工输入或 URL `aff_code`）代替一次性邀请码，一次性邀请码优先；覆盖邮箱与全部 OAuth / Pending OAuth 新账号注册路径。
  - 账号创建与邀请码消耗 / 返利绑定保持原子性，错误语义与上游邀请码校验保持一致（失败统一返回 `ErrInvitationCodeInvalid` 并回滚）。

上游检索关键词：`affiliate_code_registration_enabled`、`ValidateRegistrationInvitation`。

---

## 5. Ops 运维告警增强（Telegram 通知渠道）

- **配置与接口**：复用 `ops_email_notification_config` 的 `alert.telegram`（零新增数据库迁移），API 返回经 `PublicConfig()` 脱敏（隐藏 `bot_token`，暴露 `bot_token_configured`），支持 `POST /api/v1/admin/ops/email-notification/telegram-test` 发送测试消息。
- **核心实现**：
  - 支持与邮件通知并行或单独开启（单独启用 Telegram 时也正确记录告警已发送并支持配置 `min_severity`），使用独立 `telegramLimiter` 与邮件限流解耦。
  - `telegram_notification.go` 复用共享 `telegramHTTPClient`，支持可选自定义 `api_base_url`（需 `https://`，用于反代环境），失败时安全透出 Telegram API `description`（防 Bot Token 泄露）。
  - `OpsSettingsDialog.vue` 完整支持中英文 i18n。

上游检索关键词：`OpsTelegramAlertConfig`、`sendTelegramAlert`、`SendTelegramTestNotification`、`telegramLimiter`。
