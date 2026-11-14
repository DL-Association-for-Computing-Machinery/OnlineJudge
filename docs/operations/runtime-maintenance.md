# 环境配置与恢复维护

从仓库根目录运行本文命令。先看[运维导航](README.md)核对环境归属和操作边界。

## 运行环境

Docker 工具链使用 Go 1.26.11、pnpm 11.25.0、Node 24.19.0，与 `go.mod`、[Node 版本](../../web/.node-version)及 [package.json](../../web/package.json)对应；依赖锁文件为 `go.sum` 和 `web/pnpm-lock.yaml`。宿主无需另外安装这些构建工具。

构建工具链由 Dockerfile 固定，应用依赖以 go.sum 和 pnpm-lock.yaml 为准。Docker Desktop 迁移的验证结果见交付日志；历史 Colima 证据不能替代本次验证。

### 试用服务器只读盘点

在实际 Linux 试用服务器上，用普通维护账号和 Python 3.9+ 执行：

```sh
python3 -B scripts/target-inventory.py \
  --materials-dir "$MATERIALS_DIR" --scratch-dir "$SCRATCH_DIR" --database-dir "$DATABASE_DIR"
```

三个参数是已有的材料、临时文件和数据库目录的绝对路径。脚本不创建目录或启动容器，也不输出路径或环境变量。报告分别记录当前进程 CPU 亲和性、系统内存读数、可见 cgroup v2 限额和各卷空间；缺失不等于无限。读数用于准备[单机验收](operations-design.md#当前单机试用资源约束)，不证明 Worker 已拥有这些资源或目标环境已经通过。

试用前可追加 `--storage-budget "$SCRATCH_PEAK_BYTES" "$SCRATCH_PEAK_INODES" "$RESERVE_BYTES" "$RESERVE_INODES"`。四个值均为正整数，分别表示从当前读数起预计新增的临时文件字节和 inode 峰值，以及峰值后每卷必须保留的字节和 inode。按并行槽位和实际材料预算预先登记数值；不要从通过结果倒推预算。同一设备上的卷均扣除峰值。读数缺失、临时文件或数据库卷只读、或预留不足时仍输出 JSON，并返回 2。通过只说明本次文件系统读数覆盖登记预算；设备号不同仍可能共享底层存储池，需另核对挂载、配额和实际峰值。预检不会清理文件或变更运行中的服务。

## 显式启用普通会话 API

平台管理员首次授权由受信维护入口管理。迁移后用 `.local/bin/migrate platform-admin inspect --subject <id>` 读取最新修订；用 `.local/bin/migrate platform-admin set --subject <id> --expected-revision <n> --enabled <true|false> --operator <name> --reason <reason>` 授权或撤销。初次修订填 0；结果未知时先 inspect。命令使用私有维护库连接 `OJ_DATABASE_URL`，不发放管理会话；维护操作者名不得以 `platform-admin:` 开头。

维护探针可请求普通会话 API 的 TLS 监听器：`GET /api/v1/readyz`，使用既有受信 CA 和配置的 Host；mTLS 模式经受信代理发起，沿用固定证书和已有转发头。响应 200 表示最近一次会话依赖检查通过，503 表示未就绪；`/api/v1/healthz` 继续只表示进程存活。默认 Compose 不启用会话依赖探针，见[就绪语义](operations-design.md#会话-api-就绪检查)。

API 镜像的 `/usr/local/bin/oj-api` 在非空 `OJ_SESSION_CONFIG_FILE` 下切换为独立会话 HTTPS 服务；同时必须显式设置 `OJ_API_ADDR`（例如容器内 `:8443`），不能与 `OJ_EXPERIMENT_TOKEN_FILE` 同用。配置文件为绝对规范路径，完整字段及权限见[配置合同](../architecture/session-design.md#717-显式会话-api-入口)。未设置配置路径时仍运行现有 bootstrap/实验模式，不尝试连接会话数据库。

数据库及 Redis 的密码来源各自显式选择 `auth: "none"` 或 `"env"`。后者只读取固定的 `OJ_SESSION_DATABASE_PASSWORD` / `OJ_SESSION_REDIS_PASSWORD`，必须非空；前者拒绝对应非空残留环境。密码、登录 key、TLS 私钥均不写入配置 JSON、镜像或命令行。数据库不消费 `OJ_DATABASE_URL`，连接类 PG 环境变量须按运行时合同清理；服务不修改进程环境。

启动前须由受信维护流程完成当前 schema 33（邮箱模型始于5、积压准入始于6、私有草稿始于7、列表索引始于8、参考申请始于9、预期计划始于10、参考领取始于11、参考报告始于12、参考调度始于13、卷积对拍预期计划始于14、作者送审始于15、审核资格与分配始于16、审核退回始于17、作者确认标程始于18、审核人确认题面始于19、训练虚拟时间始于20、邮件令牌始于21、邮件全局发送限速始于22、提交历史索引始于23、受控重判审计始于24、审核分配列表索引始于25、平台管理员历史始于26、普通终审批准账本始于27、发布资格历史始于28、发布账本与执行准入记录始于29、C17 提交约束始于30、GNU++20 始于31、Java 21 始于32、Python 3.14 始于33）、恢复状态开门、共享密钥分发及匹配的限流 marker 初始化；命令只检查，不自动迁移、初始化或修复。schema切换须在维护窗口排空/停止旧API、Worker及维护写入后执行，再启动匹配新版；运行中旧进程不会因版本变化自动停止，不混跑。精确回退条件见[邮件令牌迁移](../architecture/session-design.md#123-签发持久化与迁移)、[训练队列迁移](../architecture/submission-backlog.md#升级与回退)、[参考申请迁移](../architecture/reference-requests.md#迁移与回退)及[草稿历史迁移](../architecture/problem-drafts.md#入库版本与迁移)。本次没有迁移现有持久化localdev或部署服务器。配置与公开证书可只读挂载，登录密钥及 TLS 私钥必须满足 0400/0600、root/进程 euid 所有等约束。非 root 容器须能遍历并读取这些文件。不要把完整配置或秘密对象输出到日志。

direct 模式要求客户端直接 TLS；proxy 模式要求同一监听器的已验证客户端证书和允许的叶证书指纹，健康检查也须经 mTLS。现有 Compose/Nginx 的明文 backend 和浏览器页面尚未配置该入口；部署、生产阈值、真实证书/存储 TLS 和完整账号权限另行验收。回退先停服务并移除显式配置环境，不把旧明文路径接到会话 Handler。

可选 `session_devices: true` 启用 [会话管理及满额登录选择](../../api/session.openapi.yaml)。已登录用户读取不续期，撤销其他会话保留当前 Cookie。已验密的待确认登录在满额时可选择一个旧会话，确认后用原候选替换。移除配置并重启即可关闭，不改变会话数据。

可选 `session_rotation: true` 启用当前会话的同源凭据轮换 API。换发保留原期限和名额，消耗共享登录恢复限额；浏览器界面不自动触发。回执未知时先读取当前会话，失效则重新登录。

可选 `management_sessions: true` 启用[独立平台管理员会话](../../api/session.openapi.yaml)。管理员以本人密码登录，使用独立的非持久 Cookie，空闲 30 分钟、最长 8 小时；查询不续期，同源活动才续期。启用时共享现有登录限流、密码计算和连接预算；普通会话不能访问管理路由。再显式设置 `management_role_changes: true` 才开放平台角色写入，应用数据库角色须有该历史表的 INSERT 权限。每次写入需重新输入操作者本人密码；首次授权仍由受信维护命令完成，结果未知时先用 `inspect` 查目标修订。

可选的 `password_change` 对象另行启用 `POST /api/v1/account/password`，省略时保持 404。必须显式给定 `minimum_characters`、`blocklist_file` 及 `global`/`source`/`subject` 三个 `{count, window}` 额度；用户已确认至少 15 个 Unicode code point、允许空格、不强制字符组合、拒绝常见弱密码，保留原始字节且不 trim/归一化/截断。最低字符数可配置更高值，KDF 最大密码字节预算至少为最低字符数的四倍。规则文件是至多 1 MiB 的版本 1 JSON 整串 SHA-256 弱密码摘要表，采用与公开证书相同的受控文件读取约束；格式及边界见上述设计。生产字典仍须验证来源、许可与覆盖，不能将合成测试词条当作生产覆盖证明。

所有副本须预先显式初始化匹配额度和完整规则指纹的独立 marker；启动和请求均只核对，不修复丢失状态。规则/词典变化需要隔离旧副本并执行受控维护，不通过换 namespace 或重建计数绕过额度。改密成功推进持久化认证代次，清除两个浏览器 Cookie 后重新登录；503 可能已提交，不能自动重发。关闭该能力时移除 `password_change` 配置并重启；回退旧二进制也先移除该对象，已完成的密码更新仍保留，无 schema 迁移。

### 迁移前后回读 schema

将独立的 `OJ_SCHEMA_STATUS_DATABASE_URL` 指向已确认的端点。用只有账本读取权限的登录，显式传入库名和应用迁移账本所在的 schema（标准安装为 `public`）：

```sh
.local/bin/migrate schema-status "$EXPECTED_DATABASE" public
```

成功输出账本当前版本、此二进制要求的版本和 `matches_runtime`。较旧或较新的版本也会输出结果；切换前须核对值，不能只看退出码。库名不符、账本缺失、权限不足或超时均失败。命令只读，不执行迁移；库名匹配不等于端点已确认，账本读数也不能证明没有在途迁移、数据完整或所有进程已排空。

### 判题运维快照

值守人员可从维护工作站发起一次只读采样。由既有凭据分发渠道注入专用只读 `OJ_JUDGE_SNAPSHOT_DATABASE_URL`；连接角色和网络访问由现场维护者配置。命令显式接受期望数据库名和 1–60 分钟的接收窗口，例如：

```sh
migrate judge-snapshot <期望数据库名> 15
```

成功输出一行版本化 JSON。排队和运行计数覆盖当前积压；节点分别统计配置启用数、持有至少一个当前有效租约的节点数，以及没有当前有效租约的启用节点数。槽位统计是配置值。近期系统错误和恢复耗尽按提交的原接收时间分组。查询不依赖 API 或 Worker；它不会判定节点在线，也不应作为高频探针。库名不匹配、schema 不兼容、权限不足及超时均失败，见[口径和限制](operations-design.md#判题运维快照)。

### 会话恢复维护

会话恢复维护命令只用于已经隔离的恢复副本。先设置 `OJ_RESTORED_SESSION_CONFIG_FILE` 指向该副本专用的 API 配置，再按[恢复普通会话](operations-design.md#恢复普通会话)执行；不要在在线库使用。

```sh
.local/bin/migrate session-recovery status '恢复库名'
.local/bin/migrate session-recovery close '恢复库名'
# 核对并补回备份后的账号事实，将配置指向新的专用空 Redis 后再执行：
.local/bin/migrate session-recovery open '恢复库名' 'close 返回的代次' --accounts-reviewed
```

如配置了 [`email_limits`](../architecture/session-design.md#8-注册找回与账号发放)，`open` 会先在新的空 Redis 中准备邮箱限流标记，再开放本轮恢复代次。邮箱确认入口还须配置 `email_confirmation`；仅设置限流不会开放路由。若中途失败，按恢复说明读回状态，不在原实例上补写标记。

无法由作者本人处理的旧参考申请，可在恢复代次仍关闭时逐笔处理。确认 `OJ_RESTORED_DATABASE_URL` 指向同一个已隔离副本；命令不使用普通运行 DSN。见[恢复参考验证积压](operations-design.md#恢复参考验证积压)。

```sh
.local/bin/migrate restored-reference list '恢复库名' '关闭代次'
.local/bin/migrate restored-reference status '恢复库名' '关闭代次' '申请ID'
.local/bin/migrate restored-reference cancel '恢复库名' '关闭代次' '申请ID' --confirm-cancel
```

还原数据库和材料后、允许服务写入前，用同一个恢复 DSN 执行只读材料核验。两个根目录分别是按摘要命名的判题文件目录和私有对象目录；默认时限 10 分钟，可显式调整至最多 2 小时。范围与失败处理见[恢复材料核验](../architecture/storage-design.md#恢复材料核验)。

```sh
.local/bin/migrate verify-restored-materials -timeout 10m '恢复库名' '/绝对路径/判题材料' '/绝对路径/私有材料'
```

### 捕获逻辑备份

在受信 Linux 维护环境使用与服务器兼容的本地 `pg_dump`，显式指定源库 Unix socket、端口及备份角色；输出必须是不存在的绝对路径，父目录已存在且不在材料根内。保留源文件，暂停会删除或原地改写材料的操作。见[一致性备份](../architecture/storage-design.md#捕获逻辑备份)。

```sh
.local/bin/migrate capture-backup -socket '/绝对路径/套接字目录' -port 5432 -user '备份角色' -timeout 10m \
  '源库名' '/绝对路径/判题材料' '/绝对路径/私有材料' '/绝对路径/全新备份目录'
```

需要密码认证时，显式加 `-passfile '/绝对路径/备份角色凭据文件'`，其为当前用户所有、权限 `0600`、至多 32 KiB 的普通文件，且运行环境须提供 `/proc`。专用文件可用 `*` 匹配 socket 地址，仍应限定数据库、端口和角色。命令不回退到运行 DSN、默认密码文件或环境密码。

### 核验收到的逻辑备份

在源端封存后，用 `sha256sum` 计算 `manifest.json` 的 SHA-256，通过独立可信渠道保存或交给接收方。接收方在 `pg_restore` 前，用该原始摘要核验静止的备份目录；不要从收到的目录自行计算预期摘要。详见[接收核验](../architecture/storage-design.md#核验收到的逻辑备份)。

```sh
.local/bin/migrate verify-backup -manifest-sha256 '源端完成清单的原始摘要' -timeout 10m '/绝对路径/收到的备份目录'
```

### 核对 M1 恢复证据

独立恢复演练完成后，按[计时与证据约定](operations-design.md#试用恢复计时与证据)填入脱敏记录，并传入收到的原始清单和留存的五份观察文件：

```sh
python3 scripts/assess-m1-restore.py '/绝对路径/恢复观测.json' '/绝对路径/收到的备份目录/manifest.json' '/绝对路径/观察文件目录'
```

退出码 `0` 表示观察文件摘要匹配且记录满足计时与完整性条件、可供人工复核，`1` 表示记录有未通过项，`2` 表示输入无效、证据缺失或摘要不一致。命令不执行恢复、不验证备份目录内容，也不替代负责人对故障域与原始记录的复核。

P6 整阶段验收使用下列命令，并从演练前独立保存的记录取得拓扑摘要：

```sh
python3 -B scripts/assess-m1-restore.py "$RESTORE_RECORD" "$BACKUP_MANIFEST" "$RESTORE_OBSERVATIONS" \
  --topology "$RESTORE_TOPOLOGY" --topology-sha256 "$PREREGISTERED_TOPOLOGY_SHA256"
```

P6 格式要求七份观察，增加异地副本核验和复判结束后源端仍隔离的证据。沿用相同退出码，`local` 预演结果不能充当现场证据。

## 邮件密文维护

邮件密文维护：使用应用库的 `OJ_DATABASE_URL` 执行 `.local/bin/migrate mail-payload-sweep <1-512> [cursor]`。每次只检查一批，输出 JSON `removed` 和 `next_cursor`；非空游标供下一批使用，空游标表示本轮结束。单个维护任务按部署负载限制调用频率与批次数，每轮不传游标起步，以重访此前锁住或后来到期的行。命令只清理超过发送窗口的密文，不撤销有效链接，也不启动发送。
