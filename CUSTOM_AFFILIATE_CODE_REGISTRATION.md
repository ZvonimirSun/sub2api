# Custom 分支功能记录

本文仅记录 `custom` 分支相对上游 `main` 新增的功能点，供后续同步上游代码时快速检查是否已有同类实现。

后续新增或调整任何相对上游的功能扩展时，必须同步更新本文，记录功能语义、主要配置或入口及上游检索关键词。

## 允许使用返利邀请码注册

- 新增设置 `affiliate_code_registration_enabled`，默认关闭。
- 开启邀请码注册时，允许返利邀请码代替一次性邀请码作为注册凭证。
- 手工输入支持一次性邀请码和返利邀请码，一次性邀请码优先。
- 没有手工输入时，支持使用邀请链接保存的 `aff_code` 完成注册。
- 手工输入的返利邀请码会正常绑定邀请关系，并覆盖 URL 中的返利码。
- 使用一次性邀请码注册时，URL 中的返利码仍可正常绑定邀请关系。
- 返利码作为注册凭证时，账号创建与邀请关系绑定保持原子性，绑定失败不创建账号。
- 已有账号登录或 OAuth 绑定不会重新绑定邀请人。
- 邮箱注册及 GitHub、Google、Email OAuth、LinuxDo、OIDC、微信、钉钉和 pending OAuth 新账号路径均支持该功能。
- 邀请码预校验接口支持区分手工输入和 URL 返利码，并返回邀请码类型。
- 管理后台新增“允许使用返利邀请码注册”开关及中英文文案。
- 公开设置、管理设置、前端类型和设置审计增加对应字段。
- 增加邀请码优先级、返利绑定、错误兼容、并发使用和事务回滚测试。

## 发布流程不回写默认分支

- 保留现有构建、发布和通知流程。
- 移除发布成功后的 `sync-version-file` Job。
- 发布标签或手工触发 Release 后，不再自动修改默认分支的 `backend/cmd/server/VERSION`。
- 放宽 `.github/release-tools/release_matrix.py` 中的版本正则 `VERSION_RE`，兼容 `v0.2.8.1` 等四段式自定义版本号。
- 避免 custom 分支发版时由 GitHub Actions 向上游同步分支或默认分支产生额外提交。

## 精简公开首页展示

- 移除首页“订阅转 API”功能标签，保留会话保持和按量计费标签。
- 移除首页“已支持的 AI 模型”展示区及 Claude、GPT、Gemini、Antigravity 等静态卡片。
- 同步删除中英文 `home.tags.subscriptionToApi` 和 `home.providers` 文案，避免遗留无引用翻译。
- 不影响后台平台配置、模型能力、API 路由及实际转发功能，仅调整公开首页展示。

## 站点域名访问守卫

- 新增设置 `site_domain`，用于配置面板和前端页面的规范域名，留空时不启用守卫。
- 管理后台提供“站点域名”配置项；只接受不带协议、路径、查询参数、Fragment 和用户信息的域名，可包含端口。
- 保存时去除首尾空白和末尾 `/`，并统一转为小写；非法值返回 `INVALID_SITE_DOMAIN`。
- 非规范域名访问前端页面或静态资源时，以 `307 Temporary Redirect` 跳转到规范域名，并保留原路径和查询参数。
- 非规范域名访问 `/api/v1` 面板 API 时返回 `403`，防止从非规范 Host 使用站内接口。
- AI 网关路由不受域名限制，包括 Claude、OpenAI、Gemini、Antigravity、图片、视频、语音和其他兼容入口。
- 路由判断使用 Gin 已匹配路由结果，避免依赖易遗漏的 API 路径白名单，也避免 SPA 中间件误接管后端路由。
- 设置更新后会刷新运行中的域名守卫，不需要重启服务。
- 管理设置 DTO、审计、API Contract、中英文文案及前后端类型均增加对应字段。
- 增加域名规范化、页面跳转、面板 API 拒绝、AI 路由放行和嵌入式前端路由测试。

## CC-Switch 导入增强

- CC-Switch 的 `ccswitch://v1/import` 协议使用单个 `endpoint` 参数承载逗号分隔的多个端点；首项为主端点，其余项导入为自定义端点。
- 导入时以 `api_base_url` 为主端点，并追加公开设置中的全部 `custom_endpoints`。
- 导入前过滤空端点并去重，保持主端点在第一位。
- 每个端点都会应用对应平台的路径规则，例如 Grok 补齐 `/v1`、Antigravity 补齐 `/antigravity`。
- `homepage` 不再固定复用 API 地址，按 `site_domain`、`api_base_url`、当前页面 `window.location.origin` 的顺序回退。
- `site_domain` 作为非敏感站点配置加入公共设置响应及嵌入式前端设置注入，前端按当前页面协议补成完整站点 URL。
- Provider 名称继续使用站点名称，API Key、用量查询脚本和平台默认模型等既有导入行为保持不变。
- 增加多端点去重、独立官网地址和 Grok 多端点规范化测试。

## OIDC SSO 直连免行为验证码

- 新增设置 `oidc_connect_skip_action_captcha`，对应配置项 `oidc_connect.skip_action_captcha`，默认关闭。
- 开启后可直接访问 `GET /api/v1/auth/oauth/oidc/start?redirect=/dashboard` 发起 OIDC SSO。
- 该开关仅跳过腾讯天御和阿里云行为验证码；仅启用 Cloudflare Turnstile 时，OAuth start 按既有逻辑本就直接放行。
- 登录页和注册页不增加 OIDC 专用绕过逻辑，免行为验证码入口由上述直达地址提供。
- 其他 OAuth Provider 的验证码行为不受影响。
- OIDC 配置校验、state、浏览器会话 Cookie、PKCE、nonce、ID Token 和 callback 校验保持不变。
- 配置文件、数据库设置、管理设置 DTO、审计、API Contract、中英文文案及前后端类型均增加对应字段。

## 客服二维码图片

- 在上游已有文本联系方式 `contact_info` 之外，新增独立设置 `contact_qr_image_url`，支持图片 URL 或 Base64 数据。
- 管理后台复用图片上传控件配置客服二维码，图片大小上限为 300 KB。
- 公开设置、管理设置、嵌入式前端设置、前后端类型、设置审计和 API Contract 均增加对应字段。
- 顶部用户菜单、个人资料页和兑换页均支持展示客服二维码。
- 文本联系方式与二维码彼此独立，任一已配置时均显示联系区域；未配置文本时可只展示二维码。
- 图片按可用宽度自适应展示，不增加点击或悬停交互。

## OpenAI OAuth Codex Base URL

- 为 OpenAI OAuth/Setup Token 账号新增 `extra.openai_oauth_base_url`，模拟 Codex 客户端的 `openai_base_url` 配置。
- 配置值是 Codex API 前缀（例如 `https://relay.example.com/backend-api/codex`），请求时再拼接 `/responses`、`/responses/compact` 等 HTTP 路径。
- 未配置时继续使用官方 `https://chatgpt.com/backend-api/codex`；API Key、OAuth token 刷新、账号信息、额度、图片和 WebSocket 等链路保持原有行为。
- 自定义地址不强制设置 `Host: chatgpt.com`，避免中继按 Host 路由失败。
- 配置保存在账号 `extra` JSONB 中，不增加数据库字段。

## 模型真实指纹与降智监控 (ModelTrace)

- 基于 ModelTrace 统计指纹对各账号配置的模型进行真实身份归因检测，识别是否发生降智或串号。
- **严格遵循 ModelTrace 原生数学算法与高保真度**：
  - 嵌入官方 `unified_bank.json` 统计指纹库（18 个 GPT & Claude 模型），指纹库随代码版本更新直接维护，避免数据库热更新复杂度。
  - 纯 Go 原生实现，向量基底投影严格对齐 NumPy 原生矩阵乘法 `(projected @ basis.T) @ basis`，连乘内积与 Hellinger 距离、4-Split 块结构特征、末位数字分布及多样本 Softmax 温度标定完全对齐原 Python 实现。
  - 中文挑战 Prompt 生成（前缀、动作、标点风格组合，请求整数数量在 `[292, 332]` 区间内不重复抽样）。
  - 有效数字序列校验阈值：`len(nums) >= max(80, ceil(expected * 0.55))`。
  - 多轮采样（默认 3 次，可配置）与等待间隔（默认 1 秒，可配置），多轮有效特征做均值，并根据实际有效轮数动态进行 Beta 温度标定（`calibration[min(len(valid), 3)]`）。
  - 默认采用 `low` 思考强度（`reasoning_effort: "low"`），降低耗时与消耗。
- **深度复用 AccountTestService 请求管线（无特征偏离）**：
  - 探测请求完全复用 `AccountTestService.RunModelTraceProbe`，与日常测试及代理转发保持完全相同的 TLS 特征（`WithHTTPUpstreamProfile`）、Codex/Claude 客户端请求头伪装、代理链路与 Token 管理，不引入任何人工自造的异构 HTTP 请求指纹。
- **全局总开关与独立参数配置**：
  - 全局总开关（`modeltrace_global_config.enabled`）默认关闭，未保存设置、读取失败或设置损坏时均不启动后台探测；保留已明确保存的全局开关状态。
  - 全局参数支持配置探测样本数、轮次等待间隔、思考强度及调度 Cron。
  - 配置存储于现有 `settings` 表（`modeltrace_global_config`），零数据库迁移。
- **显式添加账号与单账号串行调度**：
  - 页面只展示已添加的监控账号；添加时分页搜索账号，再设置待测模型，取消添加不会产生监控配置。
  - 账号只配置待测模型；启停、Cron 和采样参数统一使用全局设置，不提供账号级开关和定时设置。
  - 可以移除监控及其历史记录，账号本身不受影响。列表查询在数据库中筛选已添加的监控，不受 1000 个账号限制。
  - 定时轮询调度器对不同账号并行执行，但单账号内部的多个待测模型严格串行执行，避免探测任务瞬时消耗账号并发槽位。
  - 账号独立配置待测模型列表（解耦用户端白名单）。
- **低侵入性与零数据库改动 (Zero Migration)**：
  - 无任何 SQL 迁移文件，配置与历史记录保存在 `accounts.extra["modeltrace"]` JSONB 字段中。
- **轻量环形历史记录**：每个模型仅保留最新 1 次测试结果及最近 3~5 次历史运行快照，避免存储随时间膨胀。
- **纯客观数据呈现与人工研判**：界面与接口客观返回推断模型、置信度（Probability）、耗时（Latency）与时间戳，不对账号做任何自动阻断或暂停；针对如 `gpt-6.1-sol` 与 `gpt-6-astra` 指纹一致等情况，完全交由管理员人工判定，无误报干扰。
- 管理后台提供独立导航入口（`/admin/modeltrace`）、全局配置弹窗、模型配置弹窗、即时测试及历史记录弹窗。
- 调度器每分钟检查是否命中全局 Cron，保存后下一次检查即生效。重叠周期跳过，全局关闭或监控移除后不继续启动后续模型探测。
- 观察模式保留认证、代理与客户端特征，但不因 401/403/429 修改账号错误或限流状态。结果仅写入 `extra.modeltrace`，保存前重新读取配置并串行化短写操作，保留期间的暂停、模型删除与其他 extra 修改。
- 手动探测后端最长运行 10 分钟，前端等待上限 11 分钟；取消后停止后续轮次，保存失败会返回错误。
- 算法对照基线：ModelTrace commit `6cd4a8ff0f3f4e49d6d6969dd2fbadfa1569fde2`；嵌入库 Git blob hash `24cc43f171bf1f374cf7e6da3915b35c99d6e60d` 与原库相同。`backend/internal/pkg/modeltrace/testdata/generate_parity.py` 从未修改的 Python 原版生成基准；Go 回归测试逐项比较各模型分数、概率及模型/家族排名，容差 `1e-10`。

## 上游更新时检索

### 返利邀请码注册

```text
affiliate_code_registration_enabled
ValidateRegistrationInvitation
registrationInvitation
validate-invitation-code
invitation_code
aff_code
BindInviterByCode
```

### 发布流程

```text
sync-version-file
backend/cmd/server/VERSION
github.event.repository.default_branch
```

### 首页展示

```text
home.tags.subscriptionToApi
home.providers
Supported Providers
subscriptionToApi
```

### 站点域名守卫

```text
site_domain
SettingKeySiteDomain
SiteDomainGuard
normalizeSiteDomain
RequireHost
RedirectPage
SetSiteDomainGuard
INVALID_SITE_DOMAIN
```

### CC-Switch 导入

```text
ccswitch://v1/import
buildCcSwitchImportDeeplink
additionalEndpoints
custom_endpoints
homepage
endpoint
site_domain
```

### OIDC SSO 直连

```text
oidc_connect.skip_action_captcha
oidc_connect_skip_action_captcha
SkipActionCaptcha
OIDCOAuthStart
requireActionCaptchaForOAuthLoginStart
VerifyActionCaptchaIfEnabled
/api/v1/auth/oauth/oidc/start
```

### 客服二维码图片

```text
contact_qr_image_url
SettingKeyContactQRImageURL
ContactQRImageURL
ImageUpload
contactQrImageUrl
```

### OpenAI OAuth Codex Base URL

```text
openai_oauth_base_url
GetOpenAIOAuthBaseURL
IsCustomOpenAIOAuthBaseURL
openai_base_url
backend-api/codex
```

### 模型指纹与降智监控 (ModelTrace)

```text
modeltrace
ModelTraceService
RunModelTraceProbe
unified_bank.json
modeltrace_global_config
DecodeModelTraceConfig
ProbeModel
RunScheduledCycle
/api/v1/admin/modeltrace
AdminModelTrace
```
