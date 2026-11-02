# OnlineJudge

面向东华理工大学计算机协会的训练与校赛平台。当前已有持久化/租约/固定材料 Worker 实验，以及显式 HTTPS 下普通账号的登录、会话、改密与浏览器面板；Docker Desktop 专用容器验证 GNU++17 沙箱。完整注册、权限、训练产品及 M0 整体验收仍待交付，未部署。

## 当前交付与下一步

- [开发与交付计划](docs/phase-1-development-plan.md)：发布范围、组件引入时机、P0—P7 与里程碑。
- [M0 记录](docs/m0-experiment.md)：Worker、数据库与会话实验的证据及剩余缺口，是当前进度入口。
- [会话实验](tests/sessionexperiment/README.md)：独立测试模块的复现与限制，尚未接入正式登录。
- [密码记录核心](docs/architecture/session-design.md#71-同步密码记录核心)与[主体凭据](docs/architecture/session-design.md#72-主体凭据持久化与条件更新)：显式策略的 Argon2id、PostgreSQL 状态与条件更新，HTTP 接线见下述入口。
- [密码计算准入](docs/architecture/session-design.md#73-密码计算准入)：创建/验证共享实例额度、有限排队及取消后保留实际计算名额；多 API 总预算与生产容量仍待验证。
- [密码验证用例](docs/architecture/session-design.md#74-密码验证与条件升级用例)：同密码升级、双版本条件更新与末次状态复核；结果只描述时点，不发放会话或资源权限。
- [主体状态协调](docs/architecture/session-design.md#75-会话存储操作的主体状态协调)：共享行锁协调受信会话存储操作与账号变更；回调失败仍可能有外部效果，恢复编排待接入。
- [全局恢复状态](docs/architecture/session-design.md#76-正式全局恢复状态与双层协调)：schema 4 默认闭门、显式换代次与条件开门，同事务全局→主体双层检查；没有自动 Redis 清理或恢复编排。
- [个人邮箱映射](docs/architecture/session-design.md#77-个人邮箱映射与原子初始化)：schema 5 原子初始化凭据与未验证邮箱，整址忽略大小写、保留原始显示；当前限 ASCII 裸址，无公开注册或邮件验证接口。
- [个人凭据认证路径](docs/architecture/session-design.md#78-个人凭据到会话门禁的认证路径)：在邮箱查找与密码验证前捕获恢复代次，携带同一代次进入最终双层门禁。
- [普通 Redis 会话协议](docs/architecture/session-design.md#79-普通-redis-会话协议)：正式核心的三名额、固定创建尝试、结果确认、续期和墓碑撤销；真实 Redis/PostgreSQL 专项接入集成 CI，HTTP 发放由下述两步适配承担。
- [已有会话 HTTPS 适配](docs/architecture/session-design.md#710-已有普通会话的-https-适配)：只读身份、显式活动与确认退出，固定 HTTPS 来源和严格 Cookie；尚未挂载默认 API，也不发放登录 Cookie。
- [跨请求登录与 Cookie 发放](docs/architecture/session-design.md#711-跨请求登录尝试与-cookie-发放)：无会话写入的准备、密文候选恢复、实际执行点截止和确认后发放；[独立密钥文件加载](docs/architecture/session-design.md#712-登录密钥文件加载)，独立 HTTPS Handler，默认 API 未挂载。
- [共享登录限流](docs/architecture/session-design.md#713-共享登录限流)：跨副本全局/来源/邮箱联合额度、独立恢复额度与失败关闭；严格 Handler 构造入口供后续默认 API 使用，生产阈值待验。
- [单跳受信反代](docs/architecture/session-design.md#714-单跳受信反向代理)：显式 mTLS 证书指纹、固定公开来源与严格覆写头，独立代理入口要求登录限流；默认 Compose、生产代理配置和其余浏览器兼容仍待验。
- [会话运行时装配](docs/architecture/session-design.md#715-显式配置与运行时装配)：显式预算、单一身份权威、启动只读检查及关闭排空；typed owner 由下述显式 API 入口接线。
- [敏感对象输出保护](docs/architecture/session-design.md#716-会话敏感对象的输出保护)：六种会话敏感类型的直接值/指针合法格式保护，登录配置 JSON 隐藏密钥；不替代禁止秘密进入日志的规则。
- [显式会话 API 入口](docs/architecture/session-design.md#717-显式会话-api-入口)：`OJ_SESSION_CONFIG_FILE` 启用严格配置的 TLS/mTLS 命令入口，必须同时指定 `OJ_API_ADDR`；默认 Compose 未启用。
- [已登录改密](docs/architecture/session-design.md#718-已登录用户修改密码)：显式能力、独立三维额度和弱密码规则，核验当前密码后原子换代并撤销旧会话；[接口契约](api/account.openapi.yaml)。
- [浏览器账号面板](docs/account-browser.md)：已有账号登录、原尝试恢复、检查/主动续期/退出和改密；[六路径契约](api/session.openapi.yaml)。默认 HTTP 预览阻止密码操作，不代表注册、训练或完整权限已开放。
- [Web 界面](docs/frontend-design.md)：简洁题库首页、独立登录页和双栏做题预览；示例可搜索、筛选、编辑和导入导出，正式提交尚未开放。

## 运行环境

Docker 工具链使用 Go 1.26.10、pnpm 11.25.0、Node 24.19.0，与 `go.mod`、[Node 版本](web/.node-version)及 [package.json](web/package.json)对应；依赖锁文件为 `go.sum` 和 `web/pnpm-lock.yaml`。宿主无需另外安装这些构建工具。

构建工具链由 Dockerfile 固定，应用依赖以 go.sum 和 pnpm-lock.yaml 为准。Docker Desktop 迁移的验证结果见交付日志；历史 Colima 证据不能替代本次验证。

## 本地 HTTPS 账号环境（一键）

需要在页面上实际登录时，在仓库根目录运行：

```sh
python3 scripts/dev-up.py
```

打开 [本地 HTTPS 登录页](https://localhost:8443/login)，点击“一键进入测试账号”，或使用私有初始化凭据走普通表单登录。首次访问需确认本地证书，或自行信任脚本导出的公开 `.local/dev-auth/ca.crt`；脚本不更改系统信任。账号、密钥、PostgreSQL 和 Redis 数据持久保留，重复运行不重设密码。完整边界、停止方式与验证见[本地开发指南](docs/local-development.md)。此独立项目不替代生产部署，也不影响下面的 HTTP 骨架预览。

## Docker 构建与一键启动（默认）

默认使用 Docker 构建和 Compose 启动。macOS 使用 Docker Desktop 的 `desktop-linux` context；Linux CI 使用 Docker Engine 与 Compose v2+，省略下面命令中的 `--context desktop-linux`。首次构建需联网获取 Dockerfile 中固定摘要的基础镜像和锁定依赖。

在仓库根目录执行：

```sh
docker --context desktop-linux compose up -d --build --wait
```

打开 [Docker 本机预览](http://127.0.0.1:8080/) 查看示例题库；连接诊断位于 `/account` 的折叠区域。Nginx 提供 Vite 构建后的静态文件，将 `/api/` 转发到 Compose 内部的 `api:8080`；后端不发布宿主机端口。若 8080 被占用，执行 `OJ_WEB_PORT=18080 docker --context desktop-linux compose up -d --build --wait`，再访问 18080。后续源码修改需要重建，此入口不是热更新开发服务器。

```sh
docker --context desktop-linux compose build            # 构建默认应用镜像
docker --context desktop-linux compose ps               # 查看服务和健康状态
docker --context desktop-linux compose logs --tail=100   # 查看启动或请求错误
docker --context desktop-linux compose down             # 停止本项目容器、网络
```

当前只启动前端与基础 API，健康检查只证明进程/代理连通；不启动数据库、Worker、控制端或判题沙箱，不代表完整 OJ 可用。应用容器使用非 root、只读根文件系统；Nginx 临时文件只写入内存目录。当前默认服务没有持久化业务数据或 volume；后续增加数据库必须同步配置持久化与迁移步骤，不使用 `down -v` 清理数据。HTTPS、正式部署与离线制品交付尚未配置。

Dockerfile 的 Go、Node、pnpm 与现有配置保持一致，基础镜像固定多架构索引摘要；依赖仍使用 `go.sum` 和 `pnpm-lock.yaml`。`.dockerignore` 限定构建上下文，排除本地 Git、缓存、环境文件和常见凭据文件。更换工具链时需同步 Dockerfile 和对应锁定入口，并重新验证容器检查及启动流程。

## 显式启用普通会话 API

API 镜像的 `/usr/local/bin/oj-api` 在非空 `OJ_SESSION_CONFIG_FILE` 下切换为独立会话 HTTPS 服务；同时必须显式设置 `OJ_API_ADDR`（例如容器内 `:8443`），不能与 `OJ_EXPERIMENT_TOKEN_FILE` 同用。配置文件为绝对规范路径，完整字段及权限见[配置合同](docs/architecture/session-design.md#717-显式会话-api-入口)。未设置配置路径时仍运行现有 bootstrap/实验模式，不尝试连接会话数据库。

数据库及 Redis 的密码来源各自显式选择 `auth: "none"` 或 `"env"`。后者只读取固定的 `OJ_SESSION_DATABASE_PASSWORD` / `OJ_SESSION_REDIS_PASSWORD`，必须非空；前者拒绝对应非空残留环境。密码、登录 key、TLS 私钥均不写入配置 JSON、镜像或命令行。数据库不消费 `OJ_DATABASE_URL`，连接类 PG 环境变量须按运行时合同清理；服务不修改进程环境。

启动前须由受信维护流程完成 schema 5、恢复状态开门、共享密钥分发及匹配的限流 marker 初始化；命令只检查，不自动迁移、初始化或修复。配置与公开证书可只读挂载，登录密钥及 TLS 私钥必须满足 0400/0600、root/进程 euid 所有等约束。非 root 容器须能遍历并读取这些文件。不要把完整配置或秘密对象输出到日志。

direct 模式要求客户端直接 TLS；proxy 模式要求同一监听器的已验证客户端证书和允许的叶证书指纹，健康检查也须经 mTLS。现有 Compose/Nginx 的明文 backend 和浏览器页面尚未配置该入口；部署、生产阈值、真实证书/存储 TLS 和完整账号权限另行验收。回退先停服务并移除显式配置环境，不把旧明文路径接到会话 Handler。

可选的 `password_change` 对象另行启用 `POST /api/v1/account/password`，省略时保持 404。必须显式给定 `minimum_characters`、`blocklist_file` 及 `global`/`source`/`subject` 三个 `{count, window}` 额度；用户已确认至少 15 个 Unicode code point、允许空格、不强制字符组合、拒绝常见弱密码，保留原始字节且不 trim/归一化/截断。最低字符数可配置更高值，KDF 最大密码字节预算至少为最低字符数的四倍。规则文件是至多 1 MiB 的版本 1 JSON 整串 SHA-256 弱密码摘要表，采用与公开证书相同的受控文件读取约束；格式及边界见上述设计。生产字典仍须验证来源、许可与覆盖，不能将合成测试词条当作生产覆盖证明。

所有副本须预先显式初始化匹配额度和完整规则指纹的独立 marker；启动和请求均只核对，不修复丢失状态。规则/词典变化需要隔离旧副本并执行受控维护，不通过换 namespace 或重建计数绕过额度。改密成功推进持久化认证代次，清除两个浏览器 Cookie 后重新登录；503 可能已提交，不能自动重发。关闭该能力时移除 `password_change` 配置并重启；回退旧二进制也先移除该对象，已完成的密码更新仍保留，无 schema 迁移。

## 构建与测试（Docker Desktop）

开发前可执行 `python3 scripts/doctor.py`（结构化输出加 `--json`，警告也需阻断时加 `--strict`）。它只检查任务分支、缓存主线祖先关系、工作区、hooks、工具、Linux Docker/Compose、端口与资源；不联网 fetch、不修复环境、不启动容器。默认端口取 `OJ_WEB_PORT` 或 8080，可用 `--port` 指定。未提交改动、缓存主线落后或端口占用为警告；缺工具、主线/游离 HEAD、hooks 或 Docker 不可用为失败。缓存主线不证明远端最新，资源展示不是容量验收，端口探测后也可能被其他进程占用。

本机启动 Docker Desktop，使用 `desktop-linux` context。无需安装 Go、Node、pnpm、PostgreSQL 或 Make；宿主 Python 3 只编排测试。Linux CI 使用 Docker Engine。Makefile 已移除。

```sh
docker --context desktop-linux compose run --build --rm toolchain backend
docker --context desktop-linux compose run --build --rm frontend-check
docker --context desktop-linux compose run --build --rm toolchain sqlc
docker --context desktop-linux compose run --rm toolchain db
python3 scripts/repo_policy.py repository
python3 -B -m unittest discover -s tests/governance -v
python3 scripts/test-compose.py
```

后端包含格式、vet、golangci-lint、race、全部命令构建；前端包含 lint、类型、测试、构建。Go 静态分析覆盖主模块、sandbox 标签测试源码及独立会话实验模块，并先验证检测器能接受正确样例、拒绝四类错误；不执行 sandbox 选手代码。规则与边界见[代码质量检查](docs/code-quality.md)。SQL 生成检查在容器内比较生成前后内容；数据库检查使用容器内临时 PostgreSQL 14、迁移、并发租约与两个真实 API/mTLS 控制端。工具镜像不挂载 Docker socket，不访问用户数据库。测试每次运行，不以构建缓存替代测试。

测试专用会话 Store 成本采样使用 `python3 scripts/test-session-cost.py --module-cache <已校验缓存目录> --output <新JSON路径>`，只用已有本地镜像，独立私有 TCP 容器中比较启用相同授权守卫的路径；不是 API 跑分或容量准入。配置、采样窗口和持久化差异见[会话实验](tests/sessionexperiment/README.md#同等普通会话合同的-store-成本采样)，正确性仍执行原行为/race专项。

普通 Redis 协议专项（只执行合成账号数据）：

```sh
docker --context desktop-linux build --target toolchain --tag oj-session-protocol-toolchain .
docker --context desktop-linux pull redis:7.2.17@sha256:0312984fbfeb2ff1d4382ecef45b50e7d65478a6a58cf12ce28d6de7a4142e45
python3 -B scripts/test-session-protocol.py --toolchain-image oj-session-protocol-toolchain --build-cache-volume ''
```

脚本只使用已有本地镜像，检查 Linux/架构，创建带唯一 owner 标签的私有 internal 网络、Redis 和工具链容器；无宿主发布端口或 Docker socket。临时 PostgreSQL 只开放容器内 Unix socket，复制源码白名单并运行真实存储/race 专项。结束检查退出码/OOM 并仅清理自有容器和网络，保留缓存卷。可用 `--redis-image` 指定已核验本地镜像、`--module-cache` 挂载既有只读模块缓存；合并缓存后执行 `go mod verify`，测试使用 `-mod=readonly`。Linux CI 使用 `--context default`；此专项不是完整 AC23、容量或部署验收。

### 浏览器账号专项

账号页面与 API 必须由同一 HTTPS Origin 提供；当前默认 Compose/Vite HTTP 入口会阻止账号请求。不要通过关闭 Secure Cookie/来源检查或启用跨源代理来让密码表单工作。实际流程及限制见[浏览器账号](docs/account-browser.md)。

专用驱动要求本机已有 Node、Playwright 模块和匹配浏览器（仅用于自动化），应用构建仍使用 Docker；不自动安装或下载。先按权限准备当前锁文件对应的前端工具镜像及上面的 Go/Redis 镜像，再选择仓库外的全新产物目录：

```sh
docker --context desktop-linux build --target frontend-source --tag oj-browser-frontend-source .
python3 scripts/test-browser-account.py \
  --frontend-image oj-browser-frontend-source \
  --playwright-module /absolute/path/to/installed/playwright \
  --output-dir /absolute/new/path/browser-account
```

可选 `--chromium-executable /absolute/path/to/existing/chromium` 指定已有浏览器；`--toolchain-image`、`--module-cache`、`--build-cache-volume` 和 `--context` 的约束沿用现有专项。驱动只发布随机回环 HTTPS/HTTP 对照端口，建立隔离的合成账号/存储；真实 race API 使用固定 pin 的 mTLS，外层临时证书只在新测试 context 忽略。控制端点仅供本夹具，不能对外开放或接入真实数据。

产物包含日志、浏览器断言 JSON 与桌面/390px 截图；驱动分别检查浏览器和夹具结果，结束只清理本轮带 owner 标签的资源。`integration && browser` 夹具只运行 `TestBrowserFixture`，普通 PR CI 不自动执行此手动浏览器专项。生产证书、浏览器兼容矩阵及完整业务验收仍须另做。

真实 Worker 专项（当前固定 macOS arm64、Docker Desktop LinuxKit 7.0.14）：

```sh
python3 scripts/test-worker.py all
# 也可选择 sandbox / report / dual-worker / worker-fault / slot-recovery / network-fault / compile-lease / resource-bounds / http-loop
```

脚本用 Docker 编译宿主测试驱动与 `.local/bin/` 命令，构建固定 go-judge/GCC 镜像，再通过宿主 Docker CLI 操作两个禁网、无挂载、只读根文件系统的专用特权容器。独立无特权观察容器共享 Docker VM 的 PID namespace，核验停止后的进程确已消失。不会启动 Colima；Docker Desktop 的 VM 与其他项目共享，这与原专用 VM 隔离边界不同。只执行仓库合成测试，不作为生产不可信提交的部署方案。

首次下载需联网；重复运行复用镜像和停止的槽，拒绝接管其他来源或正在运行的同名容器。测试最后停止自己的槽，删除本次唯一命名的临时 PostgreSQL 容器（数据在 tmpfs），不删除其他容器、卷或旧 Colima 资源。异常隔离标记不自动清除。跨编译的宿主测试不含 race；Linux 容器内后端/数据库测试保留 race。

Docker 更新导致内核或镜像变化时会拒绝执行；须验证新环境并发布新材料版本，不能放宽断言。当前执行材料为 `m0-sum-v3`；v1 默认与 v2 原始清单继续保留，执行器拒绝旧版本。初始化夹具、提交 API、控制端应一致设置 `OJ_EXPERIMENT_MATERIAL_VERSION=m0-sum-v3`。

独立构建宿主实验命令：

```sh
docker --context desktop-linux build --target host-artifacts --output type=local,dest=.local/bin .
```

`.local/bin/worker` 需要 `OJ_SANDBOX_EXPERIMENT=1`、`OJ_WORKER_CONTROLLER_URL`、`OJ_WORKER_CA_FILE`、`OJ_WORKER_CERT_FILE`、`OJ_WORKER_KEY_FILE`；控制端 `.local/bin/worker-api` 需要 `OJ_WORKER_API_ADDR`、证书文件和 `OJ_DATABASE_URL`，材料目录为 `OJ_MATERIAL_ROOT`。缺失配置拒绝启动，测试脚本自动生成私有临时配置。详见 [M0 契约与历史证据](docs/m0-experiment.md)。

Worker 启动完成后，向该 Worker 进程发送 `SIGUSR1`（`kill -USR1 PID`，将 PID 替换为对应进程号）请求排空式下线：每轮领取前检查排空，已通过检查的至多一个在途领取继续完成下载、执行及回报；确认后不再领取。空队列等待可被唤醒；未知回报仍按原令牌/结果最多发送三次，失败保留原错误。`SIGINT`/`SIGTERM` 仍取消当前操作并等待停止确认。排空返回后同样停止沙箱、确认 PID 消失，成功才清正常占用标记；停止不确定继续隔离，不自动重启或解封。排空耗时受现有任务/网络预算影响，不承诺固定下线 SLA；需要中止时仍可发送取消信号。旧版本回退后不再使用新增信号，无数据库迁移。`python3 scripts/test-worker.py report` 包含执行中排空、保留后续排队任务及显式重启复用的真实验证。

### HTTP 到真实判题终态专项

`python3 scripts/test-worker.py http-loop` 显式设置 `OJ_SANDBOX_HTTP=1`，`all` 同样包含此项。驱动建立空的临时 PostgreSQL，实际运行迁移/固定材料初始化、两个提交 API 进程、mTLS 控制端及 Worker 命令；只提交仓库内固定 AC/WA 合成源码。先由 HTTP 确认 queued，再启动 Worker，通过 HTTP 观察 finished/AC 或 WA；数据库仅旁证原始源码、固定归属及每题唯一完成 attempt。

专项检查跨 API 原键重试/回执、异源冲突、缺失/错误凭据、Origin 拒绝无写入、闭合回执字段及无 Cookie/CORS、Worker 正常排空和槽停止。提交 API 用 `127.0.0.1:0`，仅从自身私有有界 JSON 日志取得回环地址，客户端禁代理/重定向；控制端沿用既有临时端口流程。私有 Bearer 留在服务端测试进程中。该入口不支持浏览器，不授予普通账号训练权限；最小页面及其失败路径仍待实现。无 API、迁移或预算变化，可单独回退专项及驱动选择器。

### M0 最小浏览器实验页

API 额外设置 `OJ_EXPERIMENT_BROWSER=1` 才提供 `/m0` 页面及[浏览器实验契约](api/m0-browser.openapi.yaml)。必须同时显式设置 `OJ_API_ADDR=127.0.0.1:<端口>`（可用0）、私有 `OJ_EXPERIMENT_TOKEN_FILE`、实验数据库及 `OJ_EXPERIMENT_MATERIAL_VERSION=m0-sum-v3`；不能混用 `OJ_SESSION_CONFIG_FILE`。其他开关值、主机名/通配监听或材料版本拒绝启动。仅供可信本机和独占合成数据库；从实际API启动日志取得回环地址访问 `/m0`，默认 Compose、账号网关和做题预览不开放此页。

页面只接受服务端固定AC/WA示例，不接收任意源码、主体、上下文或语言。独立实验归属与原Bearer提交隔离；Bearer不下放，随机nonce用于CSRF防护，不隔离同源脚本，也不代表账号身份。所有写请求严格检查Host、单一Origin、Fetch Metadata及nonce；数据GET也需nonce。不使用或设置会话Cookie、不开放CORS，页面含CSP/no-store/nosniff；HTTP回环实验不等于生产HTTPS或训练准入。

原键/预设和内容核验状态只保存于当前标签页的sessionStorage，刷新后重新bootstrap并只查询原键，不自动提交。未知回执/404/503不等于回滚；原样重试才确认内容归属，409既有结果与当前示例明确分开。停止查询不取消任务，自动查询次数/时长有界；只有收到终态后才可显式开始下一次实验。

使用已有Playwright和Chromium验收（不自动安装，不包含在 `all` 中）：

```sh
python3 scripts/test-worker.py browser-loop \
  --playwright-module /absolute/path/to/existing/playwright \
  --chromium-executable /absolute/path/to/existing/chromium \
  --output-dir /absolute/new/path/m0-browser
```

Chromium参数可省略以使用该Playwright已安装的浏览器。产物必须为仓库外新目录，含检查JSON、私有日志及桌面/390px截图；测试以真实POST提交成功后丢弃响应验证未知/原键恢复，在浏览器确认queued且数据库无attempt后才启动真实Worker。查询503另由浏览器路由注入，不冒充真实数据库故障。回退关闭开关或回退本模块即可，无迁移；已接受任务仍按原Worker协议处理。

### 沙箱资源预算专项

`python3 scripts/test-worker.py resource-bounds` 只在上述固定专用环境内编译/执行有界合成探针，保持现有材料、容器、运行和编译预算。普通 CI 仅编译和静态检查带 sandbox 标签的源码，不执行这些程序；驱动显式设置 `OJ_SANDBOX_RESOURCES=1`，`all` 也包含该专项。

- 进程：父进程最多尝试32次fork，子进程不递归创建、关闭管道写端后等待；核对15个存活子进程加父进程、cgroup `procPeak=16`、下一次EAGAIN，以及逐一wait后再次成功fork/wait。其他errno、首个fork即被seccomp拒绝或不能回收均失败。
- inode：最多8192个空文件，要求4094个文件加目录耗尽剩余inode、ENOSPC且字节仍有余量；删除/重建一个文件验证复用，保留满inode状态给下一次执行核对新目录/完整容量恢复。
- 字节：现有单文件 `RLIMIT_FSIZE=1MiB` 会先阻止大文件，探针按最多80个文件、每个最多1MiB分段真实写入。只在直接沙箱探针使用既有Compile的10秒CPU/15秒wall/512MiB/16进程数值，不改变普通Run；要求64MiB实际写入、ENOSPC、可用字节零且inode仍有余量。分别验证满盘后的执行隔离，以及关闭fd/删除后容量恢复并再次写入。
- 普通Run保持200ms CPU/2秒wall/32MiB/16进程：单文件有限写入验证1MiB后的SIGXFSZ/OLE；分文件写入验证内存先耗尽为MLE。每项后正常探针核对目录消失与容量完整恢复，最后删除唯一缓存制品。

限制由固定版本的[执行参数](https://github.com/criyle/go-judge/blob/v1.13.3/worker/worker.go)、[实际执行](https://github.com/criyle/go-judge/blob/v1.13.3/envexec/run_single.go)及实测共同核对；`copyOutMax` 不决定单文件RLIMIT_FSIZE。普通Run的SIGXFSZ/OLE现已由正式适配器精确支持，接受条件与未覆盖的Compile分支见[判题设计](docs/architecture/judging-design.md#53-统一结果分类与单点异常归因)。`python3 scripts/test-worker.py report` 另验同一Worker/LeasedRunner/容器生命周期的OLE→AC、未截断观测、真实持久化及双控制端重报，不能以直接资源探针代替该回报证据。本资源专项不验收跨主机、长期容量、宿主文件系统耗尽或完整M0；资源测试与驱动可单独回退，不更改预算。实测归[交付记录](docs/delivery-log.md)。

### Worker 异常后的受控解封

Worker 被 `SIGKILL` 等强制结束后，原槽的持久隔离标记会阻止普通启动。操作者须独占该槽及可信 Docker 控制面，**先确认旧 Worker 的全部宿主 CLI 和在途 Docker 控制操作已结束，不会迟到派发**，再使用**崩溃前已记录或可信历史记录确认的原容器完整 ID**执行：

```sh
OJ_SANDBOX_EXPERIMENT=1 OJ_WORKER_M0_SLOT=primary \
  .local/bin/worker recover <original-full-container-id>
```

将占位值替换为原容器的 64 位小写十六进制 ID；槽只接受 `primary`/`secondary`，未指定为 primary。不要把恢复时按名称查询到的当前 ID 当作历史身份依据；旧 ID 未知、容器已被替换或 Docker 控制面不再独占时，先人工调查。标记只记录活动状态，不自动证明历史容器身份。

命令不需要控制端地址、证书、数据库或邮件凭据。它持有与 Worker 相同的规范文件锁，先持久隔离，再核对固定槽名与原 ID、固定镜像和隔离配置；后续按 ID/启动时间绑定。运行容器沿用原停止/PID 消失检查，已停止容器依可信 Docker 的停止状态双次回读，不声称重建崩溃前 PID 清单。20 秒操作预算内确认停止后才清标记；错误、取消及停止未知均不报告成功。清空后的 Sync 失败会尽力重建隔离标记，文件系统同时拒绝恢复写入时不保证隔离持久性，须保持停用并调查。

成功只确认当前沙箱已停止并清空槽标记，不领取任务、不重启 Worker。新进程无法排空旧 Worker 的宿主控制请求，Worker 的进程内 25 秒计时器也会随 SIGKILL 消失；仅等待 25 秒、Worker 退出或当前容器停止不能替代控制操作排空的确认。无法确认时保持隔离，不运行解封或重启；命令不提供跨进程派发隔离机制。

在上述前提成立后，显式重启及重新核验固定环境才能运行新 Worker；旧任务仍由控制端自然租约到期及有限恢复处理，不编辑租约/结果。回退只需恢复旧 Worker 二进制并停用此子命令，无数据库迁移。真实专项 `python3 scripts/test-worker.py slot-recovery` 在冻结合成 Worker 后记录宿主子进程，并确认它们退出再解封，覆盖互斥/错误 ID 拒绝、自然接管及原槽重启复用；未覆盖人为延迟的孤儿 CLI 或任意 SIGKILL 后自动安全复用。普通 PR CI 不运行特权沙箱。

### Worker 控制链路网络故障专项

`python3 scripts/test-worker.py network-fault` 在同一组专用离线沙箱及临时 PostgreSQL 上，使用两个真实控制端进程和透明 TCP 中继。中继只转发原始字节，Worker 与控制端仍直接验证彼此的 mTLS；不终止 TLS、不解析 HTTP、不伪造响应，不修改系统路由或其他服务网络。只阻断本轮回环连接的一个或两个方向，恢复前关闭旧连接，避免把旧 TLS 字节重放到恢复后的流。

- 双向阻断：同一 Worker 先完成一次预热 AC，目标执行期间阻断流量；检查有界失败退出、沙箱/PID 停止、无结果及预先排队的下一题零尝试。另一节点经第二控制端，在自然租约到期后完成目标第 2 次尝试，代次保持，两个控制端拒绝旧令牌回报且结果不变。
- 下行确认丢失：短测试事务锁住尚未提交的目标尝试，启用下行阻断后立即回滚释放锁。真实控制端此后提交 AC，原 Worker 收不到确认而失败停止、不领下一题；显式恢复连接/沙箱和新 Worker 后继续排队任务。全过程目标只有一次尝试、首个载荷/摘要不变；两个控制端确认相同重报并拒绝冲突重报。

行锁仅用于给故障注入排序，不更改任务、租约、结果或业务参数；测试驱动不手调 Recover。字节阻断计数不用于识别 HTTP 请求或证明报告调用次数。该专项是本机 TCP 故障注入，不验收跨主机/路由器分区、长编译期间续租或执行端自动恢复。中继的纯回环字节、方向、恢复与并发关闭 race 测试不含 sandbox 标签，普通后端检查执行；真实 Worker 专项保留 sandbox 标签及显式开关，普通 CI 不执行特权代码。实际结果见[交付记录](docs/delivery-log.md)。

### 编译在途续租失权专项

`python3 scripts/test-worker.py compile-lease` 在专用合成沙箱内识别唯一 `cc1plus`，使用容器内 PID 发送 SIGSTOP，另从 `docker top` 核对唯一编译器的 VM PID 和 T 状态；两类 PID 不混用。短暂观察 Worker、容器和同一暂停编译器仍存活后，分别阻断真实控制链路或禁用临时节点。编译器保持暂停且不发送 SIGCONT，原有 15 秒编译 wall、5 秒续租周期及请求/停止预算保持。

测试在启动目标 Worker **之前**记录宿主单调时间 t0，要求 Worker 自行失败退出、容器停止及独立 VM PID 消失证据全部在 `t0+14s` 前取得。编译不可能早于 t0 开始，因此这个保守界限可排除原 15 秒编译 wall、20 秒 curl 等待上限；机器过慢判证据不足，不放宽界限，也不把失败清理引起的停止算作通过。这是编译在途的受控故障验证，不是编译性能测试或生产下线 SLA。

两种故障都检查当前尝试无结果、预先排队的下一题零尝试；随后另一节点经第二控制端完成第2次尝试 AC，判题/恢复代次保持1，旧身份/租约回报被两个控制端拒绝且新载荷不变。网络场景等待自然30秒租约到期，禁用场景可由撤销提前触发回收，不能描述为同一自然到期路径。测试未手调 Recover 或编辑租约时间。真实字节受阻、编译器T状态与提前停止结合既有续租代码支持失权中断判断，不表示解析加密请求或取得逐请求 renew 审计。

只有进程表解析边界测试进入普通 root race；实际暂停/信号、控制端、数据库与选手编译保持 sandbox 标签及 `OJ_SANDBOX_COMPILE_LEASE=1`，由上述驱动提供。不得把此流程用于普通宿主、共享容器或生产不可信提交。回退新增测试与驱动入口即可，无业务代码、协议、依赖、迁移、生产 SQL、材料版本或预算变化；完整 M0、跨主机/断电及执行端自动恢复仍待验，实际证据归[交付记录](docs/delivery-log.md)。

`GET /api/v1/healthz` 仅代表进程存活；普通 Compose 入口不启动完整 Worker 产品。真实编译/运行、回报、双 Worker 接管与撤销/材料故障分别由专项验证，普通 CI 不运行特权实验。

## 文档入口

完整目录与维护归属见[文档索引](docs/README.md)。已选方案不等于已引入或已验证；依赖以配置和锁文件为准，历史验证见[交付日志](docs/delivery-log.md)。
