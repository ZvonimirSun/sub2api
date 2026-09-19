# Codex 续期监控：部署与 AI 交接

## 先读结论

代码全部在本仓库，但运行时是 **Sub2API 主容器 + 单独的监控容器**。根目录 `Dockerfile` 和现有 release CI 只构建主应用，**不会构建、发布或启动监控容器**。已有监控容器可在主应用升级后继续运行；首次安装或更新探针适配器时，还需要单独构建和部署监控镜像。本次仅整理交接文档，不新增打包实现、不改变现有主应用发布流程，也不操作正式服务器。

PR #2：<https://github.com/ZvonimirSun/sub2api/pull/2>，目标分支 `custom-tmp`。更新 PR 不等于合并或发布。不得为了上线本功能，把用户未上线的其他 custom 功能合并到 main。

本文不包含客户凭据。实际主机、容器、部署目录和回滚版本以项目的私有部署记录为准；示例名称不能直接代替现场值。

## 功能与边界

- 管理员左侧 **Codex 续期监控**：顶部直接管理需要探针的账号和模型。使用原生账号选择器按名称选择已有 OpenAI OAuth/setup-token 账号，按账号添加或移除模型；这不是普通账号管理页面。
- 常用模型为 `gpt-6-astra`、`gpt-5.6-sol`、`gpt-5.6-terra`，支持手动输入其他模型。每个账号/模型独立配置和保存状态；给一个模型续期不会覆盖其他模型。
- 移除模型停止后续探针，已有状态自然到期；移除最后一个模型停止该账号监控。暂停也不删除有效状态。
- 支持静态代理批量导入 `host:port:username:password`、动态固定网关、动态提取 API。代理凭据仅保存在保护的运行时文件，不进入前端列表、Git 或交接文档。
- 一轮维护共享随机静态临时池：同一个 host 不因端口/密码不同重复计入；串行探针，静态池用完才进入动态来源。动态服务商实际出口由服务商决定，不能仅凭网关地址保证出口不同。
- 后台默认每 60 秒检查本地状态是否到期；未到探针时间不会因此调用模型。默认提前 15 分钟续期；缺失、失效、长度不符或到期的状态才进入处理，遵守退避和 Retry-After。不要以验证为由连续强制续期。
- 页面仅在进入、手动刷新和完成管理操作后读取数据；已删除 15 秒定时重绘和内部轮询间隔展示。页面时间是最后一次数据更新时间，不是实时倒计时。
- 292 是状态长度观测，不能证明模型质量、首字速度或账号没有限流。请求模型匹配记录也不能替代质量测试。

## 代码地图与复用契约

| 位置 | 职责 |
| --- | --- |
| `tools/codex-turn-state-manager/manager.py`、`probe_stats.py`、`probe_diagnostics.py`、`panel.html` | 原有稳定工具原样快照，哈希见同目录 `runtime-sha256.json` |
| 同目录 `integrated.py` | 唯一受支持入口；代理导入、共享池、进程锁、禁止旧通知和旧监控默认值 |
| 同目录 `legacy_host_adapter.py` | 适配本项目 Unicode 账号、原生 Codex 指纹身份、DB 写入和 scheduler outbox |
| `frontend/src/views/admin/CodexTurnStateView.vue` | 原生管理员页面、账号选择器、受限 iframe 通信 |
| `frontend/src/assets/codex-turn-state-panel.html` | 可修改的 UI 派生副本，不是冻结快照 |
| `backend/internal/handler/admin/codex_turn_state_panel_handler.go` | 现有管理员鉴权后的 API 白名单代理 |
| `backend/internal/service/openai_codex_pinned_turn_state.go` | 网关按账号/模型读取有效状态；没有有效状态时保留原行为 |

详细固定 revision、测试与差异见 `docs/features/codex-turn-state-monitor.md` 和 `codex-turn-state-audit.md`。修改冻结逻辑前先比较固定版本源码、测试与契约；不要重新实现稳定核心。旧飞书/Vault通知、旧 HealthService 和旧服务器流程不接入本项目。保留 LGPL-3.0 及分发源码义务；镜像使用 pyc 不改变许可要求。

主应用通过 `CODEX_TURN_STATE_PANEL_URL` 访问私有 Python API；浏览器只调用主应用管理员 API。iframe 不持有管理员令牌，保留 opaque sandbox 和现有 nonce CSP，不得为修页面关闭 CSP 或增加 allow-same-origin。

## 运行前提与持久化

1. 每个项目只运行一个 manager，不能随网关副本数量扩容。进程锁只保护同一状态目录，不能保护多个主机/不同目录。
2. 当前适配器通过 Docker socket 执行主应用容器内的 `psql`，复用其 `DATABASE_*` 环境。主容器必须有 `sh`、`psql`，且数据库配置齐全。根目录现有主镜像已包含 psql。
3. 监控镜像需要 Python 3（含 sqlite3）、支持 HTTP/2 的 curl、CA 证书及 Docker CLI。Docker socket 即使只读挂载，仍有高权限 Docker API 能力；不要公开监听器或把该容器当普通租户服务。
4. 使用目标项目原生 Codex 版本与身份设置；账号须符合适配器资格。缺失/关闭/无效指纹应修正项目配置，不要生成随机身份绕过校验。现有适配器校验最低 Codex 版本 0.154。
5. 实际 pin 在数据库 `accounts.extra.pinned_codex_turn_states`；账号凭据仍在原生数据库。运行时目录包含账号/模型 overlay、代理源、历史、统计 SQLite 和锁文件。**同时备份数据库和整个状态目录**，SQLite 运行中不要只复制主 db 而漏掉 WAL；优先 SQLite backup API 或短暂停止 manager 后复制。
6. 运行时目录权限 0700，config/含秘密文件 0600；凭据不进 Git/镜像/日志。第三方测试数据不自动迁入个人 Vault，按所有者指定方式保管和清理。停止测试时不能误删正式状态。

## 打包与新安装：需要接手方明确处理

当前仓库没有独立监控镜像的 Dockerfile 或 CI 发布任务。已有正式监控镜像是本次交付时单独打包的运行产物，**只拉仓库执行根目录 docker build 不能重建完整部署**。已有用户升级主应用可以保留现有监控容器；新安装必须另外准备监控镜像和启动定义。这是当前交付边界，不要对用户宣称主镜像已经包含完整后台监控。

如后续获得授权补充标准打包，接手 AI 应：

1. 在目标分支/固定提交构建主应用，继续使用原 CI；另设监控镜像构建任务。不要仅改主应用 entrypoint 塞入第二后台进程。
2. 监控镜像安装 Python 3（sqlite3）、HTTP/2 curl、CA、Docker CLI；把 `manager.py`、`probe_stats.py`、`probe_diagnostics.py`、`legacy_host_adapter.py`、`integrated.py` 在同一 Python 小版本下编译为同目录 pyc，运行 `python3 /opt/cts/integrated.pyc --config /run/cts/config.json --daemon`。如需要旧私有根页面资源，一并放入冻结 panel.html；真正管理员页面资源在主应用内。
3. 按服务器架构构建，先隔离空账号启动验证依赖、API与退出行为。使用所有者授权的镜像仓库或 docker save/load 传输镜像，服务器不放业务源码 checkout。pyc 分发仍须遵守源码许可义务。
4. 新安装创建专用保护运行目录（0700）、config（0600）和持久化 state。配置必须明确本项目主容器、独立绝对 state_dir，以及 `alerts.enabled=false`、`degraded.enabled=false`。只运行 integrated 入口，不能直接启动冻结 manager.py。
5. 监控使用私有共享 Docker 网络，无 host port；该网络需要正常外网出口。主应用设置 `CODEX_TURN_STATE_PANEL_URL=http://YOUR_MONITOR_CONTAINER:8787`，追加共享网络时保留数据库/Redis/Nginx原有网络。面板监听 `0.0.0.0:8787` 只用于容器间连接，不能暴露公网。
6. 挂载保护配置、持久化 state 和 Docker socket；单 manager，建议沿用已验证的128MiB、0.25CPU、128pids及 restart unless-stopped。注意 restart 策略不等于卡死自动修复。
7. 保留原 Compose 完整参数，只重建必要服务，禁止整套 down/up；正式已有实例不能套用空配置或改 state_dir 导致数据消失。

新安装配置结构示意（不能覆盖当前正式配置）：

```json
{
  "state_dir": "/var/lib/THIS_PROJECT_CTS_STATE",
  "sub2api": {"container": "YOUR_EXISTING_APP_CONTAINER", "use_sudo": false},
  "accounts": [],
  "proxies": {"sources": []},
  "panel": {"enabled": true, "bind": "0.0.0.0", "port": 8787, "read_only": false},
  "degraded": {"enabled": false},
  "alerts": {"enabled": false},
  "poll_interval_seconds": 60,
  "max_probes_per_pass": 2,
  "request_timeout_seconds": 20,
  "failure_backoff_seconds": 300
}
```

管理员完成项目原生合规确认后，在面板顶部选择账号、逐个添加模型并导入代理。空 accounts 不会自动选择客户账号；不要绕过合规门禁。真实部署路径、镜像和网络名见另交付的私有现场附录。

## 升级、验证和回滚

- 只有前端/Go 变更：升级主镜像；监控容器继续保留。`integrated.py`、适配器或冻结工具变更：重新构建监控镜像并升级该服务。两侧 API 契约同时变更时配套升级。
- 每次记录两镜像的 tag/digest、Git SHA、当前 Compose/配置备份；确认已有数据库和 state 挂载仍指向原数据。升级监控只会短暂停止调度，数据库内有效 pin 不因此删除。
- 检查主应用健康、管理员监控页面能加载、匿名管理 API 被拒绝；账号/模型数量正确，能管理模型、导入源，页面无反复重绘。确认代理密码和原始状态没有泄漏。
- 只读检查 pin 长度/到期时间、正常调度日志和最近成功时间；在正常续期窗口观察成功写入及 outbox 通知。不要人为缩短周期、清空状态或强制反复探针。业务请求质量由所有者按正式接口测试。
- 出现问题先回滚受影响镜像和对应 Compose 配置；保留数据库及状态目录。不要运行 `down -v`、清空 pins 或重建数据库。只停止监控会停止续期，但已有状态自然到期；主应用回滚为不含本功能的版本时需评估网关是否还消费已有 pins。
- 原生 WebSocket 的新 pin 在新上游连接握手时生效，旧连接可能需要正常重连；不要把 HTTP 验证当作 WS/Anthropic 全路径已验收。

## 排障速查

| 现象 | 检查 |
| --- | --- |
| 页面加载中/初始化错误 | 主页面 nonce 是否注入、iframe CSP 错误、静态资源缓存/版本；保留 CSP，修桥接或主配置失败 |
| 主应用管理 API 502/连接失败 | URL、容器 DNS/共享网络、监控进程及私有端口；localhost 不是另一个容器 |
| 401/403/423 | 管理员登录权限、原生合规门禁；不要绕过 |
| 页面数据不自动变化 | 现在按需刷新，点击“刷新”；这与后台续期无关 |
| 没有账号/模型 | state_dir 和挂载是否错位、overlay 是否保留、顶部管理是否实际加入模型 |
| 有效 292 但体验仍差 | 分别检查请求/返回模型、首字时间、账号限流和路由；长度不是质量证明 |
| 续期失败 | 原生账号身份/版本、psql与数据库环境、代理可用性、429退避；不泄露完整日志秘密 |

## 交给对方 AI 的操作指令

> 请先读本文件、仓库 AGENTS.md、功能文档和本项目私有部署记录。先只读核对当前分支、PR #2、两容器镜像、Compose、配置路径及数据库/state 挂载，报告差异。此功能核心复用已冻结工具，仅适配目标 Sub2API，禁止带入旧飞书通知、旧监控 token、旧服务器流程或改写稳定核心。主应用 CI 不包含监控镜像，按本文件分别构建，服务器只放编译产物。不得把未上线 custom 功能顺带发布到 main。保留现有账号凭据、代理和 pins/state；禁止强制连续续期测试、降低退避或清空状态。通过现有管理员页面管理需要探针的账号/模型，后台正常到期调度即可。任何部署仅在项目所有者授权范围内执行，保存镜像和配置回滚点，只重建目标服务，不动其他容器。最后报告真实验证结果、未验证路径和可执行回滚方式，不以 292 长度承诺质量。

## 本次证据与限制

截至本轮交接，前端/Go 适配和原有运行镜像已做正式实例验证；正常定时续期有日志证据，页面自动重绘已取消。Python 74 项测试、冻结文件哈希、前端定向测试和构建已通过。真实动态提供商回退及完整 WS/Anthropic 业务路径未完成现场验证。本次只交付文档，不新增 Dockerfile/Compose/CI，不重新构建或部署镜像。通用监控镜像打包自动化尚未接入仓库；未来补充时需独立构建及隔离验收，不能把此前手工镜像的成功等同于未来 recipe 已验收。
