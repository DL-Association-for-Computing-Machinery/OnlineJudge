# M0 实验实施与验证记录

日期：2026-10-25。状态：**进行中，未通过 M0 整体验收**。在 `codex/engineering-baseline` 实现并验证持久化与租约核心，另在独立 Linux VM 完成沙箱分项实验。成果现已整理为原子提交，PR/远端 CI 状态见[交付日志](delivery-log.md)。没有部署、真实用户数据或独立人工审查；本次提交前复跑快速检查和真实数据库测试，未重启特权沙箱。

## 已实现与边界

| 路径 | 实际能力 | 未覆盖部分 |
| --- | --- | --- |
| `internal/submission`、`postgres` | 源码/任务同事务、主体＋上下文幂等、接收回执/所有权查询；固定合成材料版本与哈希核验 | 真实用户、题目发布授权、全局接收额度、公平调度 |
| `postgres/leases.go` | 节点记录逐次授权、单次认领、容量限制、续租、结果条件更新、重复/冲突回报、有限恢复 | HTTP mTLS、材料下载授权、节点运维入口、自动恢复循环、真正 Worker 进程 |
| `db/migrations`、`cmd/migrate` | Goose 独立迁移与 advisory lock，API 只读检查模式版本，显式夹具注册 | 生产升级、模式内容哈希账本、断连及非事务迁移恢复 |
| `internal/judging` | 有界文本 token 比较，ASCII 大小写规则与显式敏感选项 | 浮点、SPJ、完整结果归因/编排 |
| `tests/sandbox` | 真实 GNU++17 编译运行、答案比较、超限和隔离分项实验 | 从提交到真实结果的闭环、双 Worker、租约中止执行、系统故障注入 |

API 的健康检查仍仅代表进程存活；未启用实验时行为保持原样。实验入口仅在设置 `OJ_EXPERIMENT_TOKEN_FILE` 后注册，启动时强制使用字面量 loopback 地址，凭据文件必须为 owner-private 普通文件。一个静态测试主体 `m0-operator`、一个固定上下文 `m0-training`，拒绝带 Origin 的请求；它不是正式登录、Cookie 或 CSRF 实现，也不是对同主机管理员的安全边界。现有浏览器预览页不接入此凭据。

实验契约见 [OpenAPI](../api/experiment.openapi.yaml)。POST 返回 202 只代表已持久化；同键同内容返回 200；不同内容返回 409。503 或网络错误不证明事务未提交，应使用原键查询 `/submission-receipt`，即使暂时 404 也不能推断另一个在途请求一定失败。所有回执禁止缓存，不包含源码、答案、节点或租约凭据。

## 数据模型、边界与预算

提交模块拥有 `submissions`、`judge_tasks`、`judge_attempts`、`worker_nodes` 和实验材料注册表。HTTP 只调用提交用例；SQL 由 sqlc 生成到模块内部。题库模块尚未建立，`submission_materials` 仅用于 M0 固定夹具，不是正式题库发布接口。`fixture` 是显式命令，API 启动不会写材料或执行迁移。

提交固定源码、语言、版本与材料清单哈希。JSON 字段顺序不影响幂等摘要，源码空格/换行影响摘要并原样持久化。单文件源码上限 64 KiB；请求 JSON 上限 394240 字节，允许 JSON 字符转义的体积膨胀。空源码、NUL、非法 UTF-8、未知字段与尾随 JSON 被拒绝。

任务状态与判定分开：`queued/running/finished/failed`；只有 `finished` 带判定，系统故障或节点失联耗尽重试后为 `failed`、`verdict=null`，不冒充选手 WA/RE。重判代次、恢复代次与尝试编号分开。每个尝试保存节点凭据版本、随机租约标识、截止时间及已接受结果摘要/载荷；旧代次和旧尝试不能覆盖有效结果。同代次已接受的历史重复回报可确认而不覆盖新尝试。重判及灾备恢复管理用例尚未实现，测试直接修改代次只证明更新条件的隔离能力。

M0 实验参数：每 API 连接池上限 4；连接/查询及操作超时 5 秒，锁等待 3 秒；每次仅领取 1 个任务，节点容量由管理员记录控制（默认 1，最多 8）；租约 30 秒，最多尝试 3 次，每次恢复最多 64 条且总超时 5 秒。恢复须由调用方定期触发，尚无后台扫描进程；不能宣称运行中的节点失联已自动恢复。上述值是有界实验配置，**不是用户确认的吞吐、RPO/RTO 或生产容量目标**。

M0 领取暂用 FIFO；M1 前仍须实现既定参与者公平调度、用户/全局共享额度及资源能力匹配。持久化方法供受信任控制端使用，不能把 `NodeIdentity` 当成来自客户端的认证结果。当前 Worker 骨架仍不持有数据库凭据、不领取任务。

索引：提交主键用于 ID 查询；`(subject_id, context_id, idempotency_key)` 唯一索引用于并发去重/回执定位；任务主键支撑一对一关联；材料联合唯一约束固定版本/哈希；尝试唯一约束防止同代次重复编号。活动节点、到期尝试、排队任务使用部分索引。代价是每次插入及状态变更维护索引、额外存储和 vacuum 压力；正式调度及大样本负载下需再测。回执的 `EXPLAIN (ANALYZE, BUFFERS)` 验证索引路径可用，测试为小夹具并显式关闭顺序扫描，不作为容量或延迟承诺。租约的真实并发测试验证行锁/条件更新；大队列查询计划与负载尚未验收。事务只覆盖数据库操作，编译执行不持有数据库锁。

## 本机数据库复现

前置：已安装 Go、Python 3、PostgreSQL 的 `initdb/pg_ctl/createdb`；固定 Go 模块已缓存或网络可用。不是对系统 PostgreSQL 服务进行初始化。

```sh
make db-test
```

命令先构建 API/Worker/migrate，在随机 `/tmp/oj-postgres.*` 建立权限 0700 的独立数据库目录，仅开启私有 Unix socket（无 TCP 监听）。运行后停止数据库，保留该次脱敏诊断；测试 API 子进程自动结束，随机凭据文件自动清理。`--auth-local=trust` 仅适用于这个私有临时目录，不能复制为部署配置。权限受限宿主可能需要允许 PostgreSQL 共享内存与本机监听。

`go test -race -count=1 -tags=integration ./tests/integration -v` 必须明确提供指向空白临时库的 `OJ_TEST_DATABASE_URL`；缺少配置或 public schema 非空会失败，不静默跳过。套件会注入故障/未来模式版本，不得用于真实数据库。

独立命令已在临时数据库实际执行：`.local/bin/migrate up`、`fixture` 及重复运行；由 `OJ_DATABASE_URL` 注入连接配置，不回显连接串。`down` 仅允许尚无提交的数据模式，已接受数据时数据库事务拒绝回退；真实系统应向前修复，不恢复旧数据库来覆盖新提交。API 模式版本不匹配时拒绝启动实验服务，默认健康服务不自动升级数据库。

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

- `make check`：含新增文本检查器的 Go 格式/vet/race/构建、前端 6 项测试及 lint/类型/构建、23 项治理回归通过。
- `make db-test`：真实 PostgreSQL 14.19，空库/并发/重复迁移、锁等待超时与恢复、空库回退/重升、含数据回退拒绝、同键并发/冲突、事务失败、未知回执、材料异常、所有权/上下文和未来模式拒绝通过。两个真实 API 子进程的 24 个并发请求只创建 1 个提交，停止并重启其中一个后回执可恢复。
- 租约：两个数据库控制端实例、两个合成节点身份竞争认领；续租、容量、失联、撤销/轮换、重复/冲突、迟到、三次恢复上限、代次隔离通过。结果是合成夹具，不冒充真实双 Worker 测试。
- 独立沙箱 12 项子测试通过：GNU++17 编译一次后按序测试两个输入并比较答案；WA、CE/RE、TLE/MLE/OLE、缺失执行程序的 SE 证据；缓存/节点文件不可读、两次执行的临时文件清理、网络隔离与缓存清理。沙箱 `Accepted` 仅表示程序运行成功，AC 仍需答案比较。
- `govulncheck ./...`：0 可达/已导入包漏洞；仍有 1 个未导入模块漏洞，不能据此宣称镜像或所有依赖无漏洞。镜像完整 OS/工具链 CVE 扫描和许可证清单仍为发布准入前置。

可复现证据：测试源码、固定镜像摘要；本机原始输出为忽略的 `.local/m0-db-test.log`、`.local/m0-sandbox-test.log`。同机多进程不证明主机高可用；数据库本机测试不等同灾备恢复。

## 组件准入与下一步

pgx v5.11.0、Goose v3.29.0、sqlc v1.31.1 已核对官方版本/本地模块许可证（MIT），与 Go 1.26.10 构建和真实 PostgreSQL 测试相容。Goose 库只使用 pgx 驱动，其他数据库 CLI 驱动未链接；其最低 Go 1.26 与 pgx 依赖约束导致必要的传递模块版本变化，见锁文件。sqlc 本机开发二进制约 75 MiB；正式 API/migrate 构建分别约 25/14 MiB，数值随构建而变。PostgreSQL 14.19 仅复用本机实验环境，生产版本与升级窗口未确认。

go-judge v1.13.3 官方 arm64 镜像约 58 MB，GCC 15 镜像约 532 MB；实际 GCC 为 15.3.0。固定 tag 的 MIT 许可证已核对并保存 `.local/go-judge-v1.13.3-LICENSE`，SHA-256 为 `a84a0427c3a1c25851a4689410a54283962a759a02558058a107470993b80c5a`；GCC/基础发行版各组件许可及安全清单尚未完整核验，只准入隔离实验，不准入生产或对外分发。[pgx 发布](https://github.com/jackc/pgx/releases/tag/v5.11.0)、[Goose 发布](https://github.com/pressly/goose/releases/tag/v3.29.0)、[sqlc 发布](https://github.com/sqlc-dev/sqlc/releases/tag/v1.31.1)、[go-judge 版本](https://github.com/criyle/go-judge/releases/tag/v1.13.3)、[隔离与 API](https://docs.goj.ac/api)为核验入口。

下一个可审查批次：连接 Worker mTLS/逐次授权、材料下载与哈希校验、受限编排、租约续租/中止与结果回报；把双 API＋双 Worker 跑成端到端，并完成回执不明、节点失联、撤销、材料异常和会话存储关键实验。工具链配置必须随真实尝试固定，当前 `m0-sum-v1` 清单明确 `execution_environment=unvalidated`，不能直接给已有排队提交贴上已固定运行环境的标签，应创建新材料版本。

M0 仍待完整链路与会话实验；M1/M2 仍需试用范围、题库授权、SMTP/HTTPS、负载目标、RPO/RTO 与独立备份位置。不能把本页分项通过视作首期平台可用。
