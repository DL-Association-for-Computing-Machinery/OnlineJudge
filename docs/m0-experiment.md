# M0 实验实施与验证记录

日期：2026-10-27。状态：**进行中，未通过 M0 整体验收**。已有持久化、租约、材料授权和固定环境，本次新增受限执行编排，并在独立 Linux VM 实测。PR/远端 CI 状态见[交付日志](delivery-log.md)。没有部署、真实用户数据或独立人工审查；完整 Worker 与会话仍未交付。

## 已实现与边界

| 路径 | 实际能力 | 未覆盖部分 |
| --- | --- | --- |
| `internal/submission`、`postgres` | 源码/任务同事务、主体＋上下文幂等、接收回执/所有权查询；固定合成材料版本与哈希核验 | 真实用户、题目发布授权、全局接收额度、公平调度 |
| `postgres/leases.go`、`materials.go` | 节点记录逐次授权、单次认领、容量限制、续租、结果条件更新、重复/冲突回报、有限恢复；有效租约读取源码/清单 | 节点运维入口、自动恢复循环、真正 Worker 进程 |
| `cmd/worker-api`、`internal/workerapi` | 独立 loopback mTLS 控制端；证书身份绑定、逐请求证书验证、租约及材料操作；真实双控制端进程测试 | 证书签发/告警工具、跨主机部署、执行客户端；回报仍为合成数据 |
| `internal/materials` | 任务范围下载、SHA-256 校验、受限本地读取和私有快照、读取后再次授权 | 生产题库发布、Worker 缓存、对象存储与容量/灾备验收 |
| `db/migrations`、`cmd/migrate` | Goose 独立迁移与 advisory lock，API 只读检查模式版本，显式夹具注册 | 生产升级、模式内容哈希账本、断连及非事务迁移恢复 |
| `internal/judging` | 有界文本 token 比较，ASCII 大小写规则与显式敏感选项 | 浮点、SPJ、完整结果归因/编排 |
| `internal/execution` | 固定 v2 材料与环境预检、单槽编译/按序执行、首次失败停止、结果分类、统计和制品清理 | 租约客户端、远程中止确认、真实回报、通用题目/语言及生产节点接入 |
| `tests/sandbox` | 真实 GNU++17 编译运行、答案比较、超限和隔离分项实验 | 从提交到真实结果的闭环、双 Worker、租约中止执行、系统故障注入 |

API 的健康检查仍仅代表进程存活；未启用实验时行为保持原样。实验入口仅在设置 `OJ_EXPERIMENT_TOKEN_FILE` 后注册，启动时强制使用字面量 loopback 地址，凭据文件必须为 owner-private 普通文件。一个静态测试主体 `m0-operator`、一个固定上下文 `m0-training`，拒绝带 Origin 的请求；它不是正式登录、Cookie 或 CSRF 实现，也不是对同主机管理员的安全边界。现有浏览器预览页不接入此凭据。

实验契约见 [OpenAPI](../api/experiment.openapi.yaml)。POST 返回 202 只代表已持久化；同键同内容返回 200；不同内容返回 409。503 或网络错误不证明事务未提交，应使用原键查询 `/submission-receipt`，即使暂时 404 也不能推断另一个在途请求一定失败。所有回执禁止缓存，不包含源码、答案、节点或租约凭据。

## 数据模型、边界与预算

提交模块拥有 `submissions`、`judge_tasks`、`judge_attempts`、`worker_nodes` 和实验材料注册表。HTTP 只调用提交用例；SQL 由 sqlc 生成到模块内部。题库模块尚未建立，`submission_materials` 仅用于 M0 固定夹具，不是正式题库发布接口。`fixture` 是显式命令，API 启动不会写材料或执行迁移。

提交固定源码、语言、版本与材料清单哈希。JSON 字段顺序不影响幂等摘要，源码空格/换行影响摘要并原样持久化。单文件源码上限 64 KiB；请求 JSON 上限 394240 字节，允许 JSON 字符转义的体积膨胀。空源码、NUL、非法 UTF-8、未配对的 JSON UTF-16 代理项、未知字段与尾随 JSON 被拒绝。解码前由标准库检查 JSON 语法，并单独检查代理项配对，避免 `encoding/json` 将非法代理项静默替换为 U+FFFD；合法代理对、字面量反斜杠及用户原本输入的 U+FFFD 仍允许。

任务状态与判定分开：`queued/running/finished/failed`；只有 `finished` 带判定，系统故障或节点失联耗尽重试后为 `failed`、`verdict=null`，不冒充选手 WA/RE。重判代次、恢复代次与尝试编号分开。每个尝试保存节点凭据版本、随机租约标识、截止时间及已接受结果摘要/载荷；旧代次和旧尝试不能覆盖有效结果。同代次已接受的历史重复回报可确认而不覆盖新尝试。重判及灾备恢复管理用例尚未实现，测试直接修改代次只证明更新条件的隔离能力。

M0 实验参数：每 API 连接池上限 4；连接/查询及操作超时 5 秒，锁等待 3 秒；每次仅领取 1 个任务，节点容量由管理员记录控制（默认 1，最多 8）；租约 30 秒，最多尝试 3 次，每次恢复最多 64 条且总超时 5 秒。恢复须由调用方定期触发，尚无后台扫描进程；不能宣称运行中的节点失联已自动恢复。上述值是有界实验配置，**不是用户确认的吞吐、RPO/RTO 或生产容量目标**。

M0 领取暂用 FIFO；M1 前仍须实现既定参与者公平调度、用户/全局共享额度及资源能力匹配。持久化方法供受信任控制端使用，不能把 `NodeIdentity` 当成来自客户端的认证结果。当前 Worker 骨架仍不持有数据库凭据、不领取任务。

索引：提交主键用于 ID 查询；`(subject_id, context_id, idempotency_key)` 唯一索引用于并发去重/回执定位；任务主键支撑一对一关联；材料联合唯一约束固定版本/哈希；尝试唯一约束防止同代次重复编号。活动节点、到期尝试、排队任务使用部分索引。代价是每次插入及状态变更维护索引、额外存储和 vacuum 压力；正式调度及大样本负载下需再测。回执的 `EXPLAIN (ANALYZE, BUFFERS)` 验证索引路径可用，测试为小夹具并显式关闭顺序扫描，不作为容量或延迟承诺。租约的真实并发测试验证行锁/条件更新；大队列查询计划与负载尚未验收。事务只覆盖数据库操作，编译执行不持有数据库锁。

## Worker mTLS 租约入口

2026-10-27 新增 P1/P2 的单一前置功能：通过直接 mTLS 访问领取、续租和回报接口。契约见 [worker.openapi.yaml](../api/worker.openapi.yaml)。`cmd/worker-api` 是控制端进程，持有自己的 PostgreSQL 连接池（最多 4 个连接），`cmd/worker` 仍不执行任务。它和浏览器/实验提交 API 分开监听，避免向已有明文端口注册节点接口；不增加常驻外部服务或第三方依赖。模块公开契约在 `internal/submission/lease.go`，传输层不依赖 PostgreSQL 类型；原结果序列化字段及摘要保持兼容，无迁移或 SQL 行为变更。

启动前须在独立实验库运行已有 `migrate up`、`fixture`，由受信任管理侧登记 `worker_nodes` 的节点 ID、正整数凭据版本和容量。程序不自动创建或启用节点、不迁移数据库。配置均通过环境变量注入，不打印值、不读取 `.env`：

| 配置 | 要求 |
| --- | --- |
| `OJ_WORKER_API_ADDR` | 必填字面量 loopback，例如 `127.0.0.1:8443`；不允许通配地址或主机名 |
| `OJ_WORKER_CA_FILE` | 专用于受控节点的 CA PEM 文件；不回退到系统 CA |
| `OJ_WORKER_CERT_FILE` / `OJ_WORKER_KEY_FILE` | 服务端证书/私钥文件，私钥须普通文件且组/其他用户无权限；客户端验证服务端 IP SAN |
| `OJ_DATABASE_URL` | 仅控制端使用的数据库连接配置，不提供给执行节点 |

`make worker-api` 或构建后的 `.local/bin/worker-api` 启动，SIGINT/SIGTERM 在 10 秒预算内排空连接。缺配置、无效证书或非 loopback 地址拒绝启动。TLS 最低 1.3、强制验证客户端证书链并禁用会话票据。客户端必须使用专用 CA 签发且用于 ClientAuth 的证书，唯一 URI SAN 为 `spiffe://onlinejudge/worker/<node_id>/<credential_version>`，节点 ID 仅 1—128 个字母、数字、下划线或连字符，版本为无前导零的正 int64。CN、请求体和代理头均不能提供节点身份。测试每次动态生成 CA/私钥，临时文件仅 owner 可读；仓库无静态证书或密钥。

每次请求重验证书链/有效期，每个数据库操作在原事务内锁定节点并校验启用状态和凭据版本。撤销提交后，原连接下一次操作也被拒绝；轮换版本后，旧证书及旧租约不复活。证书签发、续期和到期告警工具尚未交付；不实现 CRL/OCSP，当前即时撤销依据节点禁用/版本递增。专用 CA 变更需重启控制端，不能把这个实验称为生产 PKI。

三个 POST 入口 `/api/v1/worker/claim`、`renew`、`report` 分别接收 `{}`、`lease_token`、`lease_token + verdict + evidence_sha256`。JSON 上限 1024 字节，未知字段、null、尾随 JSON、无效令牌/结果拒绝；所有响应禁止缓存，租约令牌不放 URL。领取受管理员容量约束、无任务返回 204；续租和有效新结果仍受原租约到期与代次条件保护。相同已接受结果可确认，不同结果 409。503/断连不证明事务未提交：领取丢失响应可能占用容量，需到期后由受信任恢复调用回收；续租失败须停止执行，回报确认丢失沿用原租约原结果核验。自动恢复循环与执行中止逻辑仍待后续 PR。

验证：`make go-check db-test` 在 macOS arm64、Go 1.26.10、PostgreSQL 14.19 通过；集成测试实际构建并启动两个 `worker-api` 子进程及两个 mTLS 客户端，覆盖竞争认领、跨控制端续租、异节点拒绝、重复/冲突回报、同连接撤销、轮换、到期及旧尝试。单元测试通过真实 TLS 验证无证书、错误 CA/用途、过期证书及非法身份拒绝，并用可控时钟验证连接建立后的过期。结果均为合成夹具，没有运行选手代码或宣称双 Worker 闭环。

回退：停止新增控制端即可恢复原进程组合，无数据库迁移或历史结果重写；已领取任务保留，须按现有有界恢复方法处理，不能删除任务清理。不要对有真实数据的数据库运行实验测试。当前只支持同机协议实验，跨主机认证及固定执行环境须独立验证。

## 有效租约限定的材料入口

2026-10-27，在已合并 PR #2 的 `4698b1f` 上新增单一功能：节点取得自己当前尝试的源码、固定清单和测试输入/答案。`POST /api/v1/worker/assignment` 接收 `lease_token`，返回原样源码、源码 SHA-256、固定清单哈希及清单规范字节的 base64；`POST /api/v1/worker/material` 额外接收 `test_id` 和 `kind=input|answer`，返回原始字节、`Content-Length` 和 `X-Content-SHA256`。令牌不放 URL，沿用 1024 字节请求预算和 no-store；清单限 16 KiB、64 测试点，源码仍限 64 KiB。无任意路径/哈希读取入口，详情见 [OpenAPI](../api/worker.openapi.yaml)。

提交模块拥有源码/清单授权的 `MaterialStore` 接口，`postgres.Materials` 按原节点 → 任务/尝试锁序校验节点状态、版本、尝试及双代次；取得锁后用数据库 `clock_timestamp()` 检查截止时间。`internal/materials.Service` 依赖该接口和仅按摘要打开已验证快照的 `Reader`，不直接依赖 PostgreSQL。文件 I/O 不持有数据库锁，校验完再次取得授权；响应释放前也重验证书。已完成、过期、异节点、旧凭据/尝试/代次返回 403/409；不在有效清单中的测试点返回 404；磁盘异常、哈希不符或未配置返回 503，不泄露内部路径，不转成选手 WA。客户端须完整接收并对照清单哈希校验，截断不可使用。

本地适配器通过标准库 `os.Root` 限定根目录，仅以服务端选择的 64 位十六进制摘要为文件名，拒绝符号链接、FIFO、目录和非普通文件；不新增依赖。每文件最多 1 MiB，每控制端最多 4 个活动快照，超额立即 503。流式写入 0600 临时文件并立即 unlink，完整 SHA-256 校验后只发送该私有快照，原文件随后变化不影响已验证字节；成功/失败/取消/传输结束均释放句柄与额度。快照位于系统临时目录，最多约 4 MiB/进程，需可用磁盘；不是持久缓存。支持当前 Linux/macOS 环境，不能作为恶意宿主管理员或任意远程文件系统的安全边界。放行后传输不持续锁定授权记录，已经发送的字节不能撤回。

在独立实验数据库上完成 `migrate up` 后，明确指定已创建的私有材料目录，示例只用于合成夹具：

```sh
mkdir -p .local/materials
chmod 700 .local/materials
export OJ_MATERIAL_ROOT="$PWD/.local/materials"
.local/bin/migrate fixture
make worker-api
```

上述命令仍须已有 `OJ_DATABASE_URL` 和前节 TLS 配置。`fixture` 在配置根目录时先写四个合成文件，fsync 完整文件并通过不覆盖的原子 hard link 发布，再注册数据库引用；重复初始化核验已有文件，冲突不覆盖。文件/数据库不共享事务，失败允许留下未引用合成文件并重试；尚未验收断电一致性、磁盘耗尽和正式发布回收。未设置根目录时保留原数据库夹具初始化行为。控制端只打开已有目录，路径无效拒绝启动；未设置时材料接口 503，租约接口不变。同机副本必须使用同一持久化目录，不向执行节点暴露主控目录或数据库凭据。

验证：`make go-check db-test`（macOS arm64、Go 1.26.10、PostgreSQL 14.19）通过。真实双控制端/mTLS 客户端核对四个文件、Unicode 源码、清单精确字节及摘要；覆盖异节点、猜测令牌/测试点、撤销/轮换、完成、过期和旧尝试，文件缺失/损坏失败后恢复。真实数据库验证节点/尝试锁等待期间失效、双代次隔离与清单损坏；这是关键 SQL 的行为/并发证据，无新增索引，大规模查询计划和负载仍未验收。单元测试覆盖快照不受原文件修改影响、链接/FIFO/目录/超限拒绝、取消与额度释放、读取期间撤销/到期及清单异常（这些竞态用注入依赖验证）。迁移 CLI 的可选材料初始化与重复执行也纳入进程测试。

兼容/回退：无数据库迁移，`m0-sum-v1` 清单与既有哈希不变，仍是 `execution_environment=unvalidated`。取消 `OJ_MATERIAL_ROOT` 并重启控制端可关闭材料入口，或回退到 PR #2 的程序；保留数据库与目录，不运行 Down 删除数据。本次没有 Worker 缓存/受限执行编排、生产题库发布或完整 M0 验收。

## 固定执行环境与新材料版本

2026-10-27，基于 PR #3 合并提交 `e69129d` 的独立 `codex/execution-environment` 分支，本地验证时尚未提交。唯一目标是固定 GNU++17 实验环境并绑定新材料版本，属于 P1/P2 的前置子项；不交付 Worker 执行循环或完整 M0。

`internal/experiment/environment.go` 记录 `m0-gnu17-linux-arm64-v1`：本机 Linux arm64 镜像 config ID `sha256:fa4d3b785c8245ae856b3bda4687deeb74fbd9615a0de6df3c93ab6151717661`、构建配方 SHA-256、Linux `6.8.0-100-generic`、GCC `15.3.0`、go-judge `v1.13.3`、沙箱启动参数、编译/执行参数、环境变量及每阶段预算。复用既有隔离实验制品，没有下载或升级依赖。镜像 ID 是本地内容寻址标识，**不是 registry manifest digest，不能拿它作为远端 pull 地址**；另一架构、内核、重新构建或升级必须独立验证并创建新配置/材料版本，不宣称跨节点可复现或可分发。

新 `m0-sum-v2` 将完整环境对象纳入规范清单摘要，输入/答案、文本比较器和顺序保持原样。原 `m0-sum-v1` 永久保留 `execution_environment=unvalidated`，不追改历史提交。规范清单 SHA-256：v1 为 `5825e71b429c1b4b41d853f714bd8e078f3f48adf06232f0903d5717d1703dea`，v2 为 `4699ed93798241b796c5c00701f7150e1d8c80fb7e3ba536b9344cf88fa3d68a`；回归固定两者。嵌套环境对象统一按 JSON 对象键排序，避免结构体字段顺序与 JSONB 回读规范化产生不同字节摘要。

显式选择新版本（仍需前文数据库、材料目录和 TLS 等配置）：

```sh
export OJ_EXPERIMENT_MATERIAL_VERSION=m0-sum-v2
.local/bin/migrate fixture
# 在各自终端中使用相同版本配置：
make api
make worker-api
```

三个命令共用严格选择器，只接受空值/v1/v2；空值默认 v1，未知值拒绝实验启动/注册。API 请求 `material_version` 必须等于该进程选择的版本。API 启动不注册材料，未注册的 v2 不可接收；注册重复执行校验同一清单，已存在版本不允许覆盖。四个文件沿用既有摘要，可共用材料目录；无 SQL、索引或数据库迁移变化。

控制端继续按现有 SQL 仅领取配置版本的任务，材料读取再核对该版本/清单，不能通过切换进程配置重新标记旧任务。需要同时处理 v1/v2 时，使用不同监听端口运行各自匹配的提交/控制端进程；不要在同一负载均衡入口混放不同版本。历史 GET 回执不受选择版本影响，旧提交的 POST 重试仍应回到旧版本入口。回退时先停止 v2 新接收并处理其在途租约，再取消配置使用 v1；v2 排队任务保留，旧二进制不会领取它们，恢复 v2 程序才能继续处理。不要通过 Down 或删除任务回退。

沙箱测试在任何程序执行前检查容器实际 image ID、平台、内核、编译器/执行器版本、启动参数、无网络、无挂载、只读根目录及外层 CPU/内存/进程/临时盘预算；已有 cgroup v2/seccomp 检查保持。编译/运行直接消费清单同源配置。本检查是专用 VM 的测试入口，不是已交付的生产 Worker 身份证明，不能抵抗恶意宿主管理员。

验证覆盖：新旧版本规范哈希与可变切片隔离、构建配方漂移、未知版本拒绝；真实 PostgreSQL 注册冲突、环境篡改拒绝；真实 mTLS 控制端只领取 v2、旧控制端不领取 v2、固定清单与材料下载；真实 API 子进程显式切到 v2、重复回执及 v1 历史查询。Linux 沙箱重新运行编译/两点 AC、WA/CE/RE/TLE/MLE/OLE、文件/网络隔离及清理共 12 项。实际命令与最终结果见[交付日志](delivery-log.md)。没有把数据库合成回报与独立沙箱拼接成真实 Worker 闭环。

## 本机数据库复现

前置：已安装 Go、Python 3、PostgreSQL 的 `initdb/pg_ctl/createdb`；固定 Go 模块已缓存或网络可用。不是对系统 PostgreSQL 服务进行初始化。

```sh
make db-test
```

命令先构建 API/Worker/migrate，在随机 `/tmp/oj-postgres.*` 建立权限 0700 的独立数据库目录，仅开启私有 Unix socket（无 TCP 监听）。运行后停止数据库，保留该次脱敏诊断；测试 API 子进程自动结束，随机凭据文件自动清理。`--auth-local=trust` 仅适用于这个私有临时目录，不能复制为部署配置。权限受限宿主可能需要允许 PostgreSQL 共享内存与本机监听。

`go test -race -count=1 -tags=integration ./tests/integration -v` 必须明确提供指向空白临时库的 `OJ_TEST_DATABASE_URL`；缺少配置或 public schema 非空会失败，不静默跳过。套件会注入故障/未来模式版本，不得用于真实数据库。

独立命令已在临时数据库实际执行：`.local/bin/migrate up`、`fixture` 及重复运行；由 `OJ_DATABASE_URL` 注入连接配置，不回显连接串。`down` 先在同一迁移事务内对 `submissions` 获取 ACCESS EXCLUSIVE 表锁，再检查是否为空，锁持有到事务结束。这样会等待在途接收事务，并阻止新读写穿过检查与删除之间的窗口；发现已提交记录则拒绝回退。迁移 advisory lock 只协调迁移进程，不能单独保护 API 写入。回退期间实验接收可能等待或超时，应使用原幂等键核验，不能把 503 当作未提交。真实系统应向前修复，不恢复旧数据库来覆盖新提交。API 模式版本不匹配时拒绝启动实验服务，默认健康服务不自动升级数据库。

PR #1 修复只修改尚未发布的两份实验迁移的 Down 保护，不改 Up、表结构或版本号；已有版本 2 实验库无需执行升级，但执行回退前必须重新构建并使用包含修复的 migrate 二进制。旧二进制仍存在竞态；不能只更新源码后继续使用旧程序，也不应通过回退已接收数据库来升级工具。

`make sqlc` 使用 `.local/bin/sqlc` v1.31.1 生成查询。二进制属于开发工具，不进入发布产物或仓库；PostgreSQL integration job 和生成差异检查已在 PR #1 首轮远端 CI 通过，运行与对应提交见[交付日志](delivery-log.md)。

## 本机 Linux 沙箱复现

用户允许先使用本机配置。本轮新建 Colima profile `oj-m0`：2 vCPU、2 GiB 内存、8 GiB 数据盘及 8 GiB 根盘；Linux arm64 6.8.0-100、cgroup v2。无宿主目录挂载、SSH agent 转发或宿主 SSH 配置写入，不激活新 Docker context；原 `desktop-linux` 上下文保持。不要将这些命令改为现有的共享 Docker 环境。

本轮已执行创建命令：

```sh
colima start oj-m0 --cpus 2 --memory 2 --disk 8 --root-disk 8 --vm-type vz --mount none --ssh-agent=false --ssh-config=false --activate=false --template=false --port-forwarder none
docker --context colima-oj-m0 build --network none -t oj-m0-sandbox:local -f deploy/experiments/go-judge.Dockerfile deploy/experiments
docker --context colima-oj-m0 run -d --name oj-m0-sandbox --privileged --network none --read-only --tmpfs /tmp:rw,nosuid,size=128m --shm-size=128m --memory=1g --cpus=1 --pids-limit=128 oj-m0-sandbox:local
OJ_SANDBOX_EXPERIMENT=1 make sandbox-test
```

首次启动/构建会下载镜像，已存在同名 VM/容器时应先检查状态，不能重复创建或清理用户资源。镜像来源与摘要固定在 [Dockerfile](../deploy/experiments/go-judge.Dockerfile)。go-judge 需要专用 VM 内的 privileged 权限建立嵌套隔离；容器没有网络、暴露端口或宿主挂载，REST 仅容器 loopback，通过管理侧 `docker exec` 驱动测试。无数据库、SMTP、私有题库或真实身份凭据进入该 VM。这个测试驱动不是正式 Worker 传输实现。

本轮验证后已执行 `docker --context colima-oj-m0 stop oj-m0-sandbox` 和 `colima stop oj-m0`；回读 profile 为 Stopped，原 context 为 `desktop-linux`。镜像、容器配置和 VM 保留用于恢复，没有删除 volume 或用户文件。

go-judge 使用 `-no-fallback`，实测 cgroup v2 的 CPU/memory/pids 控制器与 seccomp 启用。systemd dbus 不存在时选择当前容器 cgroup 嵌套，不是退化到无 cgroup。GNU++17 编译预算 10 秒 CPU / 15 秒墙钟 / 512 MiB；运行 200 毫秒 CPU / 2 秒墙钟 / 32 MiB，进程 16、输出管道各 4096 字节；临时文件系统每处 64 MiB；外层容器 1 CPU / 1 GiB / 128 进程。临时磁盘和进程耗尽的完整归因仍未验收。

文本比较按 ASCII 的空格、TAB、LF、VT、FF、CR 分词，只折叠 A-Z；非 ASCII 字符按 UTF-8 字节比较，非法参考文本为系统材料错误，非法选手 UTF-8 为不匹配。检查器独立上限 1 MiB、单 token 64 KiB；输出超限与参考超限分别返回错误。执行层 4096 字节输出上限更早生效。浮点、SPJ 不在本次范围。

## 验证结果

PR #1 审查修复（2026-10-25，验证时为基于 `a31e021` 的未提交工作区，后按修复拆分提交）：先运行新增测试，旧版本在两份 Down 的并发接收场景均错误放行，其中版本 1 删除了刚提交的源码/任务；Unicode 测试的 6 个非法代理项场景返回 202。修复后 `TestExperimentUnicodePreservation` 的 14 个边界用例通过，`make db-test` 覆盖两个 Down 等待在途写入后拒绝回退、版本/源码/任务保留，以及原有空库回退/重升和双 API 流程。回归源码为 `internal/httpapi/experiment_test.go`、`tests/integration/migrations_test.go`，日志为 `.local/pr1-before-db.log`、`.local/pr1-after-db.log`。这补充了旧测试未覆盖的并发窗口，不把此前顺序回退测试通过当作完整并发保证。

以下为首次分项实验的历史结果，后续修复与最新检查见[交付日志](delivery-log.md)：

- `make check`：含新增文本检查器的 Go 格式/vet/race/构建、前端 6 项测试及 lint/类型/构建、23 项治理回归通过。
- `make db-test`：真实 PostgreSQL 14.19，空库/并发/重复迁移、锁等待超时与恢复、空库回退/重升、含数据回退拒绝、同键并发/冲突、事务失败、未知回执、材料异常、所有权/上下文和未来模式拒绝通过。两个真实 API 子进程的 24 个并发请求只创建 1 个提交，停止并重启其中一个后回执可恢复。
- 租约：两个数据库控制端实例、两个合成节点身份竞争认领；续租、容量、失联、撤销/轮换、重复/冲突、迟到、三次恢复上限、代次隔离通过。结果是合成夹具，不冒充真实双 Worker 测试。
- 独立沙箱 12 项子测试通过：GNU++17 编译一次后按序测试两个输入并比较答案；WA、CE/RE、TLE/MLE/OLE、缺失执行程序的 SE 证据；缓存/节点文件不可读、两次执行的临时文件清理、网络隔离与缓存清理。沙箱 `Accepted` 仅表示程序运行成功，AC 仍需答案比较。
- `govulncheck ./...`：0 可达/已导入包漏洞；仍有 1 个未导入模块漏洞，不能据此宣称镜像或所有依赖无漏洞。镜像完整 OS/工具链 CVE 扫描和许可证清单仍为发布准入前置。

可复现证据：测试源码、固定镜像摘要；本机原始输出为忽略的 `.local/m0-db-test.log`、`.local/m0-sandbox-test.log`。同机多进程不证明主机高可用；数据库本机测试不等同灾备恢复。

## 受限执行编排

唯一目标为 P2、AC05/06/17/20 的固定 GNU++17 文本夹具编排子项。`execution.New(sandbox).Judge(ctx, descriptor, blobs)` 接收已由控制端授权的材料快照和按 SHA-256 索引的内容，再核验源码、规范清单和全部测试材料。只接受与内置 `m0-sum-v2` **逐字节一致**的清单及固定环境；v1、未知配置、缺失/损坏材料在编译前拒绝。该模块不代替有效租约授权，不自行领取或回报。

模块通过 `Sandbox` 小接口依赖执行设施，核心不依赖数据库或 Gin；`GoJudge` 负责 v1.13.3 REST 适配。每个 Runner 限一个活动尝试，忙时立即拒绝；没有后台无界排队。每次编译一次，编译制品仅在本次尝试内复用，每点独立 `/run`，只有前点 AC 才启动后点。参考答案留在编排侧，不传入编译器或选手运行环境。未运行点为 `NOT_RUN`、usage 为空；编译统计独立，汇总取实际运行点最大 CPU 和峰值内存，无运行点时汇总为空；输出截断明确标记。

适配器必须注入受信任宿主检查器和传输；先核验实际 image ID、内核、编译器、外层容器隔离，再检查 REST 版本、cgroup v2、cpu/memory/pids 和 seccomp。不存在默认网络传输。当前 `tests/sandbox` 通过既有固定 Docker context、`docker exec` 和容器 loopback REST 驱动真实编排；它是实验驱动，不是已部署 Worker 客户端。参考版本定义见 [go-judge v1.13.3 模型](https://github.com/criyle/go-judge/blob/v1.13.3/cmd/go-judge/model/model.go)。

编译非零退出或已证实的预算超限为 CE；编译器异常信号、文件/设施错误为 SE。运行资源结果映射为 TLE/MLE/OLE，非零退出、信号及受限系统调用为 RE；正常零退出才比较答案。未知、矛盾、缺字段和响应超限为 SE。REST 总响应限 128 KiB；输出超限必须有 `CollectSizeExceeded` 结构化证据，按[固定版本收集器](https://github.com/criyle/go-judge/blob/v1.13.3/envexec/file_collect.go)处理保留的上限 + 1 字节，普通文件错误仍为 SE。运行 stderr 单独超限的归因未纳入公布的 stdout 规则，保守返回 SE。JSON 可能把非法 UTF-8 字节变为 U+FFFD，因此解码文本预算最多为原字节预算三倍；固定数字答案不会与该替换字符匹配，实际 4096 个非法字节回归为 WA。这不是通用二进制保真输出协议，扩展题目之前须改用保真传输并另验。

编译/运行请求分别有固定墙钟预算加 5 秒传输余量，环境检查 10 秒，制品删除使用独立 5 秒上下文。成功、CE、运行失败和取消路径均尝试删除已知制品；清理失败改为 SE。开始编译后的传输、协议、取消或清理错误会将 Runner 标为不可复用，避免远端执行状态不明时释放容量后继续投递；必须由节点所有者确认恢复后重建 Runner。HTTP 取消不证明远端进程已终止，丢失响应可能留下未知制品，本轮不承诺远程 kill 确认或节点自动恢复。

验证：macOS arm64 / Go 1.26.10 的 `GOPROXY=off make go-check go-build` 通过（Make 参数指定已有 Go 缓存，无依赖下载）。`OJ_SANDBOX_EXPERIMENT=1 GOPROXY=off make sandbox-test` 在专用 `oj-m0` VM 通过新增 10 项真实编排验收及原 12 项隔离回归。新增覆盖 AC、首/末点 WA、CE、RE、TLE、MLE、OLE、非法 UTF-8 输出和逐点文件隔离，并逐例核验制品清空。异常材料、环境、协议、取消、容量与清理失败通过单元注入验证；不冒充真实租约中止或双 Worker 联调。

本轮不改变 API、数据库、依赖、默认配置和 `cmd/worker` 行为。回退本模块及实验测试无需迁移或重写历史任务；实际节点出现未知在途状态时须先停止接收并确认执行/制品清理，不能仅重建 Runner 宣称恢复。

## 组件准入与下一步

pgx v5.11.0、Goose v3.29.0、sqlc v1.31.1 已核对官方版本/本地模块许可证（MIT），与 Go 1.26.10 构建和真实 PostgreSQL 测试相容。Goose 库只使用 pgx 驱动，其他数据库 CLI 驱动未链接；其最低 Go 1.26 与 pgx 依赖约束导致必要的传递模块版本变化，见锁文件。sqlc 本机开发二进制约 75 MiB；正式 API/migrate 构建分别约 25/14 MiB，数值随构建而变。PostgreSQL 14.19 仅复用本机实验环境，生产版本与升级窗口未确认。

go-judge v1.13.3 官方 arm64 镜像约 58 MB，GCC 15 镜像约 532 MB；实际 GCC 为 15.3.0。固定 tag 的 MIT 许可证已核对并保存 `.local/go-judge-v1.13.3-LICENSE`，SHA-256 为 `a84a0427c3a1c25851a4689410a54283962a759a02558058a107470993b80c5a`；GCC/基础发行版各组件许可及安全清单尚未完整核验，只准入隔离实验，不准入生产或对外分发。[pgx 发布](https://github.com/jackc/pgx/releases/tag/v5.11.0)、[Goose 发布](https://github.com/pressly/goose/releases/tag/v3.29.0)、[sqlc 发布](https://github.com/sqlc-dev/sqlc/releases/tag/v1.31.1)、[go-judge 版本](https://github.com/criyle/go-judge/releases/tag/v1.13.3)、[隔离与 API](https://docs.goj.ac/api)为核验入口。

本次完成固定 v2 的受限执行编排；下一项是租约续期/失效与执行中止确认，随后接入真实回报，再贯通双 API＋双 Worker 及会话关键实验。既有 mTLS、材料、环境及编排子项不能替代完整链路；`m0-sum-v1` 继续保持 unvalidated。生产节点检查器、传输、恢复及材料下载客户端仍须独立验证。

M0 仍待完整链路与会话实验；M1/M2 仍需试用范围、题库授权、SMTP/HTTPS、负载目标、RPO/RTO 与独立备份位置。不能把本页分项通过视作首期平台可用。
