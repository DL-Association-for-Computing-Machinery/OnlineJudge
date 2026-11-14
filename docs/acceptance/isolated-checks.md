# 本机隔离验收专项

从仓库根目录运行本文命令。实际结果见[验收导航](README.md)；判题专项沿用各自的专用隔离环境。浏览器专项所需的 Go 和 Redis 工具镜像见[本机工程检查](../development/local-checks.md#构建与测试docker-desktop)。

### 浏览器账号专项

账号页面与 API 必须由同一 HTTPS Origin 提供；当前默认 Compose/Vite HTTP 入口会阻止账号请求。不要通过关闭 Secure Cookie/来源检查或启用跨源代理来让密码表单工作。实际流程及限制见[浏览器账号](../operations/account-browser.md)。

专用驱动要求本机已有 Node、Playwright 模块和匹配浏览器（仅用于自动化），应用构建仍使用 Docker；不自动安装或下载。先按权限准备当前锁文件对应的前端工具镜像及上面的 Go/Redis 镜像，再选择仓库外的全新产物目录：

```sh
docker --context desktop-linux build --target frontend-source --tag oj-browser-frontend-source .
python3 scripts/test-browser-account.py \
  --frontend-image oj-browser-frontend-source \
  --playwright-module /absolute/path/to/installed/playwright \
  --output-dir /absolute/new/path/browser-account
```

可选 `--chromium-executable /absolute/path/to/existing/chromium` 指定已有浏览器；`--statement-corpus /absolute/path/to/prepared-bundle/statements` 增加至多10份只读Markdown的浏览器渲染耗时/节点测量（响应替换，不冒充十题真实HTTP授权验收）；`--toolchain-image`、`--module-cache`、`--build-cache-volume` 和 `--context` 的约束沿用现有专项。驱动只发布随机回环 HTTPS/HTTP 对照端口，建立隔离的合成账号/存储；真实 race API 使用固定 pin 的 mTLS，外层临时证书只在新测试 context 忽略。控制端点仅供本夹具，不能对外开放或接入真实数据。

加 `--review-materials` 运行独立审核材料专项。夹具仅在临时库准备一项合成送审和非作者分配。验证 `/review/materials` 的桌面及 390px 分配列表、键盘选择、冻结题面和原始样例／标程、显式读取准入阻断及刷新清除，以及有审核资格的作者看不到本人的送审分配，也无法读取材料。另验证四项都勾选后才可确认、确认丢响应后原键回读，以及作者送审丢响应后更换当前参考申请仍能回读原送审 ID。批准专项在浏览器模拟准入通过及未知回执，核对真实 API 仍拒绝不满足条件的批准，且刷新后只用原键查询。该模式不执行参考 Worker，不与其他专项模式同时使用。

加 `--m1-login-load` 在同一隔离夹具运行集中登录。100 个合成账号在 60 秒内每 600 毫秒发起一次浏览器登录；每个浏览器上下文独立核对三步会话请求、账号身份和 Cookie。该夹具使用单 API、19 MiB / 2 轮 / 1 lane 的 Argon2id，证书例外只用于临时回环夹具。`browser-qa.json` 保留发起偏差、时延、状态和失败分类，不含邮箱或 Cookie。冷编译时可用 `--fixture-start-seconds` 将准备预算从默认 90 秒提高到最多 300 秒，不改变浏览器采样时限或通过标准。结果仅供本机复测，现场还须在实际配置、网络与正常证书验证下执行[试用验收](../development/phase-1-development-plan.md#试用容量和恢复默认值)。

同时指定 `--trial-nginx-image "$LOCAL_NGINX_IMAGE"` 可复用同一套集中登录采样，改由仓库试用模板渲染的本地 Nginx 服务正式前端，经 mTLS 转发真实 API。只发布一个随机回环 HTTPS 端口；后端和测试控制端口不对宿主发布。采样前使用测试 CA 严格核对就绪响应，浏览器采样仍沿用临时回环证书例外。该模式不使用现场配置。

Nginx 模式的每条会话响应观测增加第四列：夹具记录的单个上游 HTTP 状态，缺失或无法解析时为 `null`。探针先核对 API 返回的 200、405 和 Nginx 在未联系 API 时返回的 421。该标记仅在本地生成的配置中启用，不修改正式模板；上游状态可辅助定位，不能代替后端错误原因或证明请求已写入。

Nginx 报告另有每个样本的 `loginPhases`：从驱动发起点击到浏览器观察到请求及响应的相对毫秒数，以及 Nginx 记录的单次上游响应头耗时。没有上游或无法解析时 `upstreamMs` 为 `null`。上游耗时包括代理与后端之间的处理，差值还包括浏览器事件分发及自动化开销，不能当成纯前端渲染耗时。

夹具停止后，`fixture.log` 的 `BROWSER_LOGIN_FAILURE` 行汇总本轮 API 登录处理器记录的 503，字段仅有操作、阶段、错误类别和服务端耗时（毫秒）。这些事件可与本轮浏览器上游状态一起排查，不含逐账号关联标识。

再加 `--trial-firefox "$LOCAL_FIREFOX"` 可用已安装且与 Playwright 匹配的 Firefox 运行同一采样。全新配置先验证未知签发者会被拒绝，再在另一临时配置中信任测试 CA；不忽略 HTTPS 错误或改系统信任库。报告的 `browserTLS` 区分严格 Firefox 与回环例外两种模式。

严格 Firefox 在预备阶段检查按钮可操作，采样时发出真实鼠标点击。报告的 `clickClock` 为 `trusted-page-event` 时，`startLagMs` 以页面收到受信点击为准，`loginMs` 从该点击算到驱动观察到会话确认；缺少受信点击的样本不能核验通过。另保留 `dispatchLagMs`，用于比较驱动调度与页面接收时间；`loginPhases` 仍以驱动发起点击为时间零点。

严格模式把账号轮流交给四个独立 Firefox 进程，仍按同一条 600 毫秒采样时间线发起。报告保留 `browserProcesses`、`browserIndex`，以及首个登录 POST 被浏览器驱动观察到时相对计划的 `beginRequestLagMs`（即使没有响应也记录已观察到的发出时间）。该值有助于识别突发，但不是 API 收包时间。

加 `--training-history` 运行本人历史和详情专项。临时库写入合成记录，验证普通会话登录后的桌面和 390px 页面、同时间戳翻页、键盘打开本人源码、翻页清除详情、另一账号隔离、未验证账号拒绝，以及刷新后须显式重新读取。专项推进临时库中一条排队记录，验证真实状态请求不传输源码且页面更新到终态。夹具不发布题目、不接受正式提交、不启动 Worker。

要验证保留正常证书检查的冷启动，添加 `--cold-firefox /absolute/path/to/existing/playwright/firefox`，使用匹配的已有 Playwright Firefox；不同时使用 Chromium、语料或十题私有草稿包模式。先用不信任夹具 CA 的全新浏览器确认连接被拒绝，再在另一个临时浏览器配置导入公开测试 CA，不修改系统信任库。验证桌面和 390px 窄屏从同源加载题库与登录页；桌面完成真实登录、导入合成草稿，并创建、查询、准备和取消同一参考申请。刷新草稿后可用原键查到材料已准备，刷新与查询不会再次发出参考申请写请求。测试限制浏览器访问回环夹具，并记录页面资源来源；不代表整机断网、校园 DNS 或现场入口验收。

要验证真实十题预览，改用 `--authorized-draft-bundle /absolute/path/to/verified/bundle`，不能与 `--statement-corpus` 同用。它只接受当前锁内十题的完整私有草稿包，先复验完整性，再只读挂载；用合成账号经真实会话导入到临时 PostgreSQL 和材料根，通过本人列表与预览核对内容版本、题面、18 对样例与复制字节，并逐题检查匿名和另一作者的拒绝。该模式不执行题源程序，截图及报告留在私有输出目录。

产物包含日志、浏览器断言 JSON 与桌面/390px 截图；当前15组保留账号/预览与导入恢复覆盖，另验新浏览器档案/IndexedDB禁用时真实服务器分页发现及本人预览、迟到页隔离、列表不改变pending日志；实际Cookie换账号覆盖四条管理路由。独立journal测试入口仅由此驱动构建到临时assets，生产构建不引用它。驱动分别检查浏览器和夹具结果，结束只清理本轮带 owner 标签的资源。`integration && browser` 夹具只运行 `TestBrowserFixture`，普通 PR CI 不自动执行此手动浏览器专项。生产证书、浏览器兼容矩阵及完整业务验收仍须另做。

加 `--remembered-session` 在同一个临时 Chromium 资料中真实关闭并重开浏览器，对比默认会话、主动保持登录和退出后的表现。专项使用同源 HTTPS 夹具，不导入 Cookie 或 Playwright 存储快照。

加 `--session-devices` 用四个独立浏览器上下文验证同一账号的满额换机与会话撤销。默认用 Chromium；同时传 `--cold-firefox` 则用 Firefox，先验证未信任证书被拒绝，再在临时配置导入测试 CA。第四个上下文在窄屏上列出并替换一条旧会话，再从账号页撤销其他会话，保留当前会话。前端请求及状态回读均通过浏览器访问真实 HTTPS API。

## 真实 Worker 专项

真实 Worker 专项（当前固定 macOS arm64、Docker Desktop LinuxKit 7.0.14）：

```sh
python3 scripts/test-worker.py all
# 也可选择 sandbox / report / dual-worker / worker-fault / slot-recovery / network-fault / compile-lease / resource-bounds / http-loop / restore-loop
```

脚本用 Docker 编译宿主测试驱动与 `.local/bin/` 命令，构建固定 go-judge/GCC 镜像，再通过宿主 Docker CLI 操作两个禁网、无挂载、只读根文件系统的专用特权容器。独立无特权观察容器共享 Docker VM 的 PID namespace，核验停止后的进程确已消失。不会启动 Colima；Docker Desktop 的 VM 与其他项目共享，这与原专用 VM 隔离边界不同。只执行仓库合成测试，不作为生产不可信提交的部署方案。

首次下载需联网；重复运行复用镜像和停止的槽，拒绝接管其他来源或正在运行的同名容器。测试最后停止自己的槽，删除本次唯一命名的临时 PostgreSQL 容器（数据在 tmpfs），不删除其他容器、卷或旧 Colima 资源。异常隔离标记不自动清除。跨编译的宿主测试不含 race；Linux 容器内后端/数据库测试保留 race。

Docker 更新导致内核或镜像变化时会拒绝执行；须验证新环境并发布新材料版本，不能放宽断言。当前执行材料为 `m0-sum-v3`；v1 默认与 v2 原始清单继续保留，执行器拒绝旧版本。初始化夹具、提交 API、控制端应一致设置 `OJ_EXPERIMENT_MATERIAL_VERSION=m0-sum-v3`。

独立构建宿主实验命令：

```sh
docker --context desktop-linux build --target host-artifacts --output type=local,dest=.local/bin .
```

`.local/bin/worker` 需要 `OJ_SANDBOX_EXPERIMENT=1`、`OJ_WORKER_CONTROLLER_URL`、`OJ_WORKER_CA_FILE`、`OJ_WORKER_CERT_FILE`、`OJ_WORKER_KEY_FILE`；控制端 `.local/bin/worker-api` 需要 `OJ_WORKER_API_ADDR`、证书文件和 `OJ_DATABASE_URL`，材料目录为 `OJ_MATERIAL_ROOT`。缺失配置拒绝启动，测试脚本自动生成私有临时配置。详见 [M0 契约与历史证据](m0-experiment.md)。

#### Worker 排空下线

Worker 启动完成后，向该 Worker 进程发送 `SIGUSR1`（`kill -USR1 PID`，将 PID 替换为对应进程号）请求排空式下线：每轮领取前检查排空，已通过检查的至多一个在途领取继续完成下载、执行及回报；确认后不再领取。空队列等待可被唤醒；未知回报仍按原令牌/结果最多发送三次，失败保留原错误。`SIGINT`/`SIGTERM` 仍取消当前操作并等待停止确认。排空返回后同样停止沙箱、确认 PID 消失，成功才清正常占用标记；停止不确定继续隔离，不自动重启或解封。排空耗时受现有任务/网络预算影响，不承诺固定下线 SLA；需要中止时仍可发送取消信号。旧版本回退后不再使用新增信号，无数据库迁移。`python3 scripts/test-worker.py report` 包含执行中排空、保留后续排队任务及显式重启复用的真实验证。

### HTTP 到真实判题终态专项

受信维护流程须先使用 Docker 构建的 `.local/bin/migrate up` 升级 schema，再按材料版本执行 `.local/bin/migrate fixture`，最后显式执行 `.local/bin/migrate backlog <全局上限> <主体上限>`；命令共用私有 `OJ_DATABASE_URL`。不执行最后一步会拒绝新提交，API 不自动设值。HTTP 判题专项在独占临时库将两项上限设为 2 以触发过载，其他夹具可用较高上限。升级已有服务必须先停止全部旧版写入 API，详见[升级与回退](../architecture/submission-backlog.md#升级与回退)。

账号邮件队列有独立维护命令 `.local/bin/migrate mail-queue-up`。先完成应用 `migrate up`，停用邮件写入和发送后，以相同数据库和搜索路径执行。维护连接须直连 PostgreSQL 或使用会话级连接池，`idle_session_timeout` 须关闭或长于整个维护窗口。中断后确认上一迁移进程已退出，再重跑；如连接异常断开，应检查迁移版本后再恢复写入。禁用邮件功能或回退应用时保留 River 表和任务，本命令不提供删除队列的降级。默认 Compose 及 API/判题 Worker 启动均不执行此命令；它也不会启动发信。见[邮件队列设计](../architecture/session-design.md#101-river-开源核心验证路线与专用表备选)。

启用邮件发送前，维护角色使用同一 `OJ_DATABASE_URL` 执行 `.local/bin/migrate mail-send-rate <spacing-ms>`，按供应商额度指定全局发送许可间隔（100 至 60,000 毫秒）。重复配置不重置限速；缩短间隔不提前释放现有许可，延长间隔保守推迟下一次许可。Down22 后重新升级须再次显式配置。命令不启动发送。

邮件加密额度由独立数据库保存。维护角色将 `OJ_MAIL_BUDGET_DATABASE_URL` 指向独立额度库，执行 `.local/bin/migrate mail-budget-init <预算库名>`。仅对从未使用的密钥，用私有 `OJ_EMAIL_PAYLOAD_KEYS_FILE` 执行 `.local/bin/migrate mail-budget-provision <预算库名> <最大次数> --fresh-key`。不得为旧密钥重新登记丢失的计数。见[邮箱隔离联验](../architecture/session-design.md#126-邮箱隔离联验)。

会话 API 省略 `email_intake` 时，注册和找回申请返回 404。启用时同一配置必须包含 `email_limits`、`email_confirmation`，以及 `email_intake` 的 `capacity`、`workers`、`max_age`、绝对路径 `payload_key_file` 和 `budget_database`。额度库用与应用库 `database` 相同的字段，库名须不同；`auth: "env"` 读取专用 `OJ_SESSION_EMAIL_BUDGET_DATABASE_PASSWORD`。预置共享邮件限流 marker、River 迁移及额度账本后才能启动；入口不替代发送器或真实 SMTP 验收。停用时移除 `email_intake` 后重启，已入队邮件和账本保留。

账号邮件使用独立的 `.local/bin/mail-worker`，默认 Compose 不启动。完成应用迁移、River 迁移和共享许可配置后，使用同一 `OJ_DATABASE_URL`、私有 `OJ_EMAIL_PAYLOAD_KEYS_FILE`、可信 HTTPS `OJ_ACCOUNT_PUBLIC_ORIGIN`，以及 `OJ_SMTP_HOST`、`OJ_SMTP_PORT`、`OJ_SMTP_FROM`、`OJ_SMTP_TLS`（`implicit` 或 `starttls`）。供应商要求认证时，同时设 `OJ_SMTP_USER` 和仅所有者可读的 `OJ_SMTP_PASSWORD_FILE`，文件内容是原始密码。发送器不迁移或初始化额度，不向判题 Worker 分发这些凭据。不要在同一队列同时运行 River 的默认 worker。领取、救援或提交遇到存储错误时进程退出，由运行管理器在存储恢复后重启。

`python3 scripts/test-worker.py http-loop` 显式设置 `OJ_SANDBOX_HTTP=1`，`all` 同样包含此项。驱动使用临时 PostgreSQL、两个提交 API 进程、mTLS 控制端和真实 Worker。先以固定 AC/WA 源码填满队列；两个 API 都对第三个新键返回 `429 BACKLOG_LIMIT`，跨实例查询该键均为 404，已接收的原键仍可回读，数据库无新增提交、任务或尝试。Worker 将原两份判到终态后，测试显式按原键、原正文再提交，验证新增提交取得 AC。三份已接收提交各有一次完成尝试。

Desktop 档位还在同一容器生命周期内读回缓存条数、`/tmp` 和 `/dev/shm` 可用字节与 inode：判题时应观察到一个缓存可执行文件和共享内存卷占用，三笔任务到终态后、排空停止前应已清空缓存并回到基线。只输出计数；这是固定小题的占用样本，不能当成峰值或试用容量。

`python3 scripts/test-worker.py restore-loop` 验证[恢复后的真实判题](../architecture/storage-design.md#7-备份与验收)，`all` 也包含此项。它使用两个临时 PostgreSQL 容器，实际捕获和核验备份；停止源库后，在固定离线 Desktop 沙箱重判同一合成提交。不连接现有数据库。

已经按 [A+B 租约链路](../operations/problem-maintenance.md#ab-租约链路联验)导出私有材料后，可用现有、已停止的固定断网参考槽和本地已有的 `postgres:14-alpine` 镜像验证 A+B 历史重判：

```bash
python3 scripts/test-worker.py aplusb-restore-loop \
  --aplusb-materials "$PRIVATE_APLUSB_DIRECTORY" \
  --aplusb-manifest-sha256 "$TRUSTED_APLUSB_MANIFEST_SHA256"
```

使用封存前独立保管的运输清单摘要；私有材料目录置于仓库外。此项须显式执行，`all` 不读取 A+B 私有题包。已有当前提交的 `host-artifacts` 和 `restore-artifacts` 产物时，可加 `--prebuilt` 跳过构建。两次判题使用同一清单，第二次仅从核验后的恢复目录读取对象。详见[联验记录](delivery-evidence/aplusb-restored-rejudge.md)。

在[专用 Ubuntu VM](../architecture/checker-admission.md#ubuntu-独立判题实验) 上可显式运行同一个带 `sandbox` 标签的 `TestAPlusBRestoreJudgeLoop`，从 `tests/sandbox` 目录运行测试二进制。准备 Linux arm64 的 `.local/bin/{worker,worker-api,migrate,migrate-linux}`（客体内两份 migrate 可使用同一构建）、固定停止槽，以及两个专用回环空 PostgreSQL 14.24 容器。使用各自的 `oj-ubuntu-rejudge-db-<12 位小写十六进制>` 名称及 `owner=onlinejudge.ubuntu-rejudge-test` 标签，设 `OJ_RESTORE_SOURCE_CONTAINER`、`OJ_RESTORE_TARGET_CONTAINER` 和两库都存在的空库名 `OJ_RESTORE_DATABASE=test_<数字>`。另设 `OJ_SANDBOX_EXPERIMENT=1`、`OJ_SANDBOX_APLUSB_RESTORE=1` 和同上 Ubuntu 材料及档位环境。源库将在联验中被停止；测试只供专用客体内的本轮临时库。

`python3 scripts/test-worker.py rollback-loop` 单独运行[兼容回退专项](../operations/operations-design.md#兼容回退专项)。需要本地 Git 历史包含 `771ff3f`；构建该版本和当前工作树，使用同一个临时 PostgreSQL，在固定离线 Desktop 沙箱切换真实 Worker。该专项不包含在 `all` 中，不连接试用环境。

恢复后若需重判一条已完成的个人训练提交，使用[维护者重判命令](../operations/rejudge.md)。

仅在核验并隔离恢复库后，使用 `OJ_RESTORED_DATABASE_URL` 指向该副本，再执行 `.local/bin/migrate fence-restored-leases <恢复库名>`。命令拒绝使用常规 `OJ_DATABASE_URL`；隔离、结果读回和重新开放前的条件见[恢复提交租约](../operations/operations-design.md#恢复提交租约)。

专项检查跨 API 原键重试/回执、异源冲突、缺失/错误凭据、Origin 拒绝无写入、闭合回执字段及无 Cookie/CORS、Worker 正常排空和槽停止。提交 API 用 `127.0.0.1:0`，仅从自身私有有界 JSON 日志取得回环地址，客户端禁代理/重定向；控制端沿用既有临时端口流程。私有 Bearer 留在服务端测试进程中。该入口不支持浏览器，不授予普通账号训练权限；独立opt-in最小页面及其失败路径见下节。无 API、迁移或预算变化，可单独回退专项及驱动选择器。

### 专用 Ubuntu VM 判题实验

仅供无宿主共享目录的本地 Ubuntu 24.04 arm64 实验 VM。固定 Linux `6.8.0-106-generic`、Apple 虚拟化、go-judge/GCC 镜像及原生 Docker Unix socket。提交 API、控制器和 Worker 同时显式设置 `OJ_EXPERIMENT_MATERIAL_VERSION=m0-sum-ubuntu-arm64-v1`；运行和恢复只用 VM 内同一专用 root 账号的主槽。错误版本在领取前返回 412，不消耗尝试。禁止双槽、参考执行及浏览器模式；Desktop 默认仍用 `m0-sum-v3`。

从干净的受信提交构建 Linux 产物，并按 [固定 ID](../../scripts/offline_judge_bundle.py) 导出本机已有的两个镜像。导出按 ID 进行，不能加标签。以下要求 Python 3.11 及以上，输出使用新临时目录：

```sh
stage=$(mktemp -d)
docker --context desktop-linux build --target host-artifacts \
  --build-arg HOSTOS=linux --build-arg HOSTARCH=arm64 \
  --output "type=local,dest=$stage/bin" .
docker --context desktop-linux image save -o "$stage/images.tar" \
  sha256:cd40ffdb32619806a8d97e59fe03e890fe4066835f57efa6ae13f919ef3262c5 \
  sha256:4ea9e5ed06591da7ea23eb65465e8d3187fe79f4d5ec3ae976d29a33b013e77a
python3 -B scripts/offline_judge_bundle.py pack --binaries "$stage/bin" \
  --images "$stage/images.tar" --revision "$(git rev-parse HEAD)" "$stage/bundle"
```

通过本地 VM 通道复制 `bundle`，保留目录和文件权限，放入 root 控制的私有目录，由 root 持有整棵目录。另从受信提交复制 `test-ubuntu-worker.py` 和 `offline_judge_bundle.py` 到同一个受控脚本目录；它们不从待验证的包中运行。独立记录并传递 `pack` 输出的清单 SHA-256，不从接收包自行计算预期值。验证和运行期间目录保持静止。

```sh
# 只在已核验隔离的专用 Ubuntu VM 内运行。替换路径和清单摘要占位符。
sudo -n env OJ_SANDBOX_EXPERIMENT=1 python3 -B /path/to/trusted/test-ubuntu-worker.py \
  --manifest-sha256 TRUSTED_MANIFEST_SHA256 /path/to/private/bundle
```

[专项脚本](../../scripts/test-ubuntu-worker.py)先校验清单、每份文件和固定镜像，再导入归档并以 `--pull=never` 启动独占沙箱和回环临时 PostgreSQL。它执行真实 HTTP 判题与过载恢复，并确认原槽已停止、原客体 PID 消失。失败时保留已停止的自有沙箱供诊断；不要删除隔离标记或用旧版/Desktop 恢复命令处理 Ubuntu 槽。该包只用于实验判题，不覆盖完整试用离线发布。

复验空库时，在上述命令加 `--require-empty-image-store`。使用专用接收环境，并在运行前确认 Docker 和 containerd 均使用全新的独立数据目录；只换 Docker 数据目录仍可能共用原 containerd。脚本拒绝任何已有镜像，导入后要求镜像 ID 集合恰好等于包中两份固定镜像。无需清理或修改现有镜像库。

### M0 最小浏览器实验页

API 额外设置 `OJ_EXPERIMENT_BROWSER=1` 才提供 `/m0` 页面及[浏览器实验契约](../../api/m0-browser.openapi.yaml)。必须同时显式设置 `OJ_API_ADDR=127.0.0.1:<端口>`（可用0）、私有 `OJ_EXPERIMENT_TOKEN_FILE`、实验数据库及 `OJ_EXPERIMENT_MATERIAL_VERSION=m0-sum-v3`；不能混用 `OJ_SESSION_CONFIG_FILE`。其他开关值、主机名/通配监听或材料版本拒绝启动。仅供可信本机和独占合成数据库；从实际API启动日志取得回环地址访问 `/m0`，默认 Compose、账号网关和做题预览不开放此页。

页面只接受服务端固定AC/WA示例，不接收任意源码、主体、上下文或语言。独立实验归属与原Bearer提交隔离；Bearer不下放，随机nonce用于CSRF防护，不隔离同源脚本，也不代表账号身份。所有写请求严格检查Host、单一Origin、Fetch Metadata及nonce；数据GET也需nonce。不使用或设置会话Cookie、不开放CORS，页面含CSP/no-store/nosniff；HTTP回环实验不等于生产HTTPS或训练准入。

原键/预设和内容核验状态只保存于当前标签页的sessionStorage，刷新后重新bootstrap并只查询原键，不自动提交。未知回执/404/503不等于回滚；原样重试才确认内容归属，409既有结果与当前示例明确分开。停止查询不取消任务，自动查询次数/时长有界；只有收到终态后才可显式开始下一次实验。

使用已有Playwright和Chromium验收（不自动安装，不包含在 `all` 中）：

```sh
python3 scripts/test-worker.py browser-loop \
  --playwright-module /absolute/path/to/existing/playwright \
  --chromium-executable /absolute/path/to/existing/chromium \
  --output-dir /absolute/new/path/m0-browser
```

Chromium参数可省略以使用该Playwright已安装的浏览器。产物必须为仓库外新目录，含检查JSON、私有日志及桌面/390px截图；测试以真实POST提交成功后丢弃响应验证未知/原键恢复，在浏览器确认queued且数据库无attempt后才启动真实Worker。查询503及提交BACKLOG_LIMIT/ADMISSION_UNAVAILABLE展示另由浏览器路由注入，不冒充真实数据库故障；共享准入后端另有真实PostgreSQL/双API验证。回退页面可关闭开关；当前数据库版本按[训练队列合同](../architecture/submission-backlog.md#升级与回退)、历史schema8/7/6按草稿/积压合同升级与回退，不能直接混跑旧版进程；已接受提交仍按原Worker协议处理。

### 沙箱资源预算专项

`python3 scripts/test-worker.py resource-bounds` 只在上述固定专用环境内编译/执行有界合成探针，保持现有材料、容器、运行和编译预算。普通 CI 仅编译和静态检查带 sandbox 标签的源码，不执行这些程序；驱动显式设置 `OJ_SANDBOX_RESOURCES=1`，`all` 也包含该专项。

- 进程：父进程最多尝试32次fork，子进程不递归创建、关闭管道写端后等待；核对15个存活子进程加父进程、cgroup `procPeak=16`、下一次EAGAIN，以及逐一wait后再次成功fork/wait。其他errno、首个fork即被seccomp拒绝或不能回收均失败。
- inode：最多8192个空文件，要求4094个文件加目录耗尽剩余inode、ENOSPC且字节仍有余量；删除/重建一个文件验证复用，保留满inode状态给下一次执行核对新目录/完整容量恢复。
- 字节：现有单文件 `RLIMIT_FSIZE=1MiB` 会先阻止大文件，探针按最多80个文件、每个最多1MiB分段真实写入。只在直接沙箱探针使用既有Compile的10秒CPU/15秒wall/512MiB/16进程数值，不改变普通Run；要求64MiB实际写入、ENOSPC、可用字节零且inode仍有余量。分别验证满盘后的执行隔离，以及关闭fd/删除后容量恢复并再次写入。
- 普通Run保持200ms CPU/2秒wall/32MiB/16进程：单文件有限写入验证1MiB后的SIGXFSZ/OLE；分文件写入验证内存先耗尽为MLE。每项后正常探针核对目录消失与容量完整恢复，最后删除唯一缓存制品。

限制由固定版本的[执行参数](https://github.com/criyle/go-judge/blob/v1.13.3/worker/worker.go)、[实际执行](https://github.com/criyle/go-judge/blob/v1.13.3/envexec/run_single.go)及实测共同核对；`copyOutMax` 不决定单文件RLIMIT_FSIZE。普通Run的SIGXFSZ/OLE现已由正式适配器精确支持，接受条件与未覆盖的Compile分支见[判题设计](../architecture/judging-design.md#53-统一结果分类与单点异常归因)。`python3 scripts/test-worker.py report` 另验同一Worker/LeasedRunner/容器生命周期的OLE→AC、未截断观测、真实持久化及双控制端重报，不能以直接资源探针代替该回报证据。本资源专项不验收跨主机、长期容量、宿主文件系统耗尽或完整M0；资源测试与驱动可单独回退，不更改预算。实测归[交付记录](delivery-log.md)。

同一 `resource-bounds` 另含[固定g++编译文件上限对照](../../tests/sandbox/compile_file_size_test.go)：256KiB有限只读数据控制例AC，1MiB+128KiB示例只准编译，观察到结构化非零退出4、正式CE/零Run，再以同一Runner完成普通AC。固定编译器报告汇编器文件大小受限，诊断仅作辅助观测，不按错误文字判结果；未改Compile命令、LANG、预算或适配器，不代表未知Compile OLE分支已支持。

### Worker 异常后的受控解封

Worker 被 `SIGKILL` 等强制结束后，原槽的持久隔离标记会阻止普通启动。操作者须独占该槽及可信 Docker 控制面，**先确认旧 Worker 的全部宿主 CLI 和在途 Docker 控制操作已结束，不会迟到派发**，再使用**崩溃前已记录或可信历史记录确认的原容器完整 ID**执行：

```sh
OJ_SANDBOX_EXPERIMENT=1 OJ_WORKER_M0_SLOT=primary \
  .local/bin/worker recover <original-full-container-id>
```

将占位值替换为原容器的 64 位小写十六进制 ID；槽只接受 `primary`/`secondary`，未指定为 primary。不要把恢复时按名称查询到的当前 ID 当作历史身份依据；旧 ID 未知、容器已被替换或 Docker 控制面不再独占时，先人工调查。标记只记录活动状态，不自动证明历史容器身份。

判题 Worker、参考验证与恢复默认共用[同机执行槽](../architecture/api-worker-design.md#同机共享执行槽)。运行参考验证前先排空并退出判题 Worker；旧二进制和直连沙箱驱动也须先退出并排空。仅双 Worker 与槽恢复专项由测试驱动显式启用 `OJ_WORKER_M0_DUAL_EXPERIMENT=1`，不用于单机试用。

命令不需要控制端地址、证书、数据库或邮件凭据。它持有与 Worker 相同的规范文件锁，先持久隔离，再核对固定槽名与原 ID、固定镜像和隔离配置；后续按 ID/启动时间绑定。运行容器沿用原停止/PID 消失检查，已停止容器依可信 Docker 的停止状态双次回读，不声称重建崩溃前 PID 清单。20 秒操作预算内确认停止后才清标记；错误、取消及停止未知均不报告成功。清空后的 Sync 失败会尽力重建隔离标记，文件系统同时拒绝恢复写入时不保证隔离持久性，须保持停用并调查。

成功只确认当前沙箱已停止并清空槽标记，不领取任务、不重启 Worker。新进程无法排空旧 Worker 的宿主控制请求，Worker 的进程内 25 秒计时器也会随 SIGKILL 消失；仅等待 25 秒、Worker 退出或当前容器停止不能替代控制操作排空的确认。无法确认时保持隔离，不运行解封或重启；命令不提供跨进程派发隔离机制。

在上述前提成立后，显式重启及重新核验固定环境才能运行新 Worker；旧任务仍由控制端自然租约到期及有限恢复处理，不编辑租约/结果。回退只需恢复旧 Worker 二进制并停用此子命令，无数据库迁移。真实专项 `python3 scripts/test-worker.py slot-recovery` 在冻结合成 Worker 后记录宿主子进程，并确认它们退出再解封，覆盖互斥/错误 ID 拒绝、自然接管及原槽重启复用；未覆盖人为延迟的孤儿 CLI 或任意 SIGKILL 后自动安全复用。普通 PR CI 不运行特权沙箱。

### Worker 控制链路网络故障专项

`python3 scripts/test-worker.py network-fault` 在同一组专用离线沙箱及临时 PostgreSQL 上，使用两个真实控制端进程和透明 TCP 中继。中继只转发原始字节，Worker 与控制端仍直接验证彼此的 mTLS；不终止 TLS、不解析 HTTP、不伪造响应，不修改系统路由或其他服务网络。只阻断本轮回环连接的一个或两个方向，恢复前关闭旧连接，避免把旧 TLS 字节重放到恢复后的流。

- 双向阻断：同一 Worker 先完成一次预热 AC，目标执行期间阻断流量；检查有界失败退出、沙箱/PID 停止、无结果及预先排队的下一题零尝试。另一节点经第二控制端，在自然租约到期后完成目标第 2 次尝试，代次保持，两个控制端拒绝旧令牌回报且结果不变。
- 下行确认丢失：短测试事务锁住尚未提交的目标尝试，启用下行阻断后立即回滚释放锁。真实控制端此后提交 AC，原 Worker 收不到确认而失败停止、不领下一题；显式恢复连接/沙箱和新 Worker 后继续排队任务。全过程目标只有一次尝试、首个载荷/摘要不变；两个控制端确认相同重报并拒绝冲突重报。

行锁仅用于给故障注入排序，不更改任务、租约、结果或业务参数；测试驱动不手调 Recover。字节阻断计数不用于识别 HTTP 请求或证明报告调用次数。该专项是本机 TCP 故障注入，不验收跨主机/路由器分区、长编译期间续租或执行端自动恢复。中继的纯回环字节、方向、恢复与并发关闭 race 测试不含 sandbox 标签，普通后端检查执行；真实 Worker 专项保留 sandbox 标签及显式开关，普通 CI 不执行特权代码。实际结果见[交付记录](delivery-log.md)。

### 编译在途续租失权专项

`python3 scripts/test-worker.py compile-lease` 在专用合成沙箱内识别唯一 `cc1plus`，使用容器内 PID 发送 SIGSTOP，另从 `docker top` 核对唯一编译器的 VM PID 和 T 状态；两类 PID 不混用。短暂观察 Worker、容器和同一暂停编译器仍存活后，分别阻断真实控制链路或禁用临时节点。编译器保持暂停且不发送 SIGCONT，原有 15 秒编译 wall、5 秒续租周期及请求/停止预算保持。

测试在启动目标 Worker **之前**记录宿主单调时间 t0，要求 Worker 自行失败退出、容器停止及独立 VM PID 消失证据全部在 `t0+14s` 前取得。编译不可能早于 t0 开始，因此这个保守界限可排除原 15 秒编译 wall、20 秒 curl 等待上限；机器过慢判证据不足，不放宽界限，也不把失败清理引起的停止算作通过。这是编译在途的受控故障验证，不是编译性能测试或生产下线 SLA。

两种故障都检查当前尝试无结果、预先排队的下一题零尝试；随后另一节点经第二控制端完成第2次尝试 AC，判题/恢复代次保持1，旧身份/租约回报被两个控制端拒绝且新载荷不变。网络场景等待自然30秒租约到期，禁用场景可由撤销提前触发回收，不能描述为同一自然到期路径。测试未手调 Recover 或编辑租约时间。真实字节受阻、编译器T状态与提前停止结合既有续租代码支持失权中断判断，不表示解析加密请求或取得逐请求 renew 审计。

只有进程表解析边界测试进入普通 root race；实际暂停/信号、控制端、数据库与选手编译保持 sandbox 标签及 `OJ_SANDBOX_COMPILE_LEASE=1`，由上述驱动提供。不得把此流程用于普通宿主、共享容器或生产不可信提交。回退新增测试与驱动入口即可，无业务代码、协议、依赖、迁移、生产 SQL、材料版本或预算变化；完整 M0、跨主机/断电及执行端自动恢复仍待验，实际证据归[交付记录](delivery-log.md)。

`GET /api/v1/healthz` 仅代表进程存活；普通 Compose 入口不启动完整 Worker 产品。真实编译/运行、回报、双 Worker 接管与撤销/材料故障分别由专项验证，普通 CI 不运行特权实验。
