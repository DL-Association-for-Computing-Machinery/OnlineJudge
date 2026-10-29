# 会话名额、撤销、API 重启、Redis 超时与认证代次实验

2026-10-29，分支 `codex/session-admission`；成功 `git fetch origin main` 后从 `99ca923` 创建。迁入旧工作区实验，原工作区保留；交付状态见[交付日志](../../docs/delivery-log.md)。

实验目标：用 SCS 和真实 Redis 验证普通会话三名额，以及跨 API 撤销后迟到保存不能复活会话；后续子项补齐独立 API 进程与进程重启证据。所有适配器和 HTTP 处理器只存在于 `_test.go`，不接入应用，不提供可部署登录入口。独立 Go 模块避免把实验依赖引入正式程序；根目录 `go test ./...` **不会执行本模块**。后端 CI 已单独纳入本模块静态分析，真实 Redis 行为测试仍按下文专项执行。

## PostgreSQL 账号认证代次（2026-10-29）

`codex/session-auth-generation` 从成功 fetch 后的 `662cb8f` 创建。测试模块复用主模块锁定的 pgx v5.11.0 及其传递依赖，两个同进程 TLS API 使用独立 PostgreSQL 连接池；账号表仅为一次性数据库中的预认证夹具，不是正式账号迁移。

- 每次读取会话核对当前账号启用状态及认证代次，缺失/禁用/旧代次返回 401；数据库连接池关闭返回 503，均不发 Cookie。另一副本仍能读取有效会话。此失败注入不是数据库服务端宕机。
- 保存前重新核对代次，并持有账号行共享锁到 Redis 操作返回；批量失效更新与保存有明确顺序。真实数据库 `lock_timeout` 用例证明更新在共享锁内被阻塞，释放后可成功；过时代次不能进入保存回调。这是关键 SQL 的行为/锁证据，不是容量测试。
- 三个旧会话失效后，两副本拒绝旧 Cookie，新代次可以重新接纳三个会话；禁用及再启用都递增代次，旧会话不会复活。新建连接池读取已提交代次，不依赖 API 内存；未重启数据库。
- 屏障确认旧会话或登录代次已加载，另一副本先完成失效更新，再释放保存：均返回 401 且无 Cookie，不占新代次名额。

名额索引按主体与代次分区，旧记录保留至原期限；反复批量失效会增加暂存元数据，容量待测。主键 `(namespace, subject)` 支撑定点读/锁/更新，代价为夹具写入时的索引维护；没有新增生产索引。共享锁会在 Redis 延迟期间阻塞账号更新，由请求超时限制等待；不能称为 PostgreSQL/Redis 分布式事务，超时写入仍可能结果不明。已经通过授权检查的在途读取可能完成，未验证任意业务副作用与撤权的原子性或即时撤销 SLA。

Docker Desktop Linux arm64、Go 1.26.10、Redis 7.2.4、PostgreSQL 14.24：原十项 race 基线通过（18.621 秒）；最终十四项行为/race 通过（19.032 秒，另含两个子用例），gofmt、go mod verify、vet 通过，项目 golangci-lint 零告警。PostgreSQL 使用本地工具镜像内已有版本，驱动在测试容器内创建私有 Unix socket 集群，无 TCP 监听，退出时停止并随一次性容器清理。

只验证账号认证代次；全局灾备代次、Redis 重启/旧快照恢复、数据库备份回退、正式权限/密码校验、Gin/CSRF、独立 API 进程下的代次及完整 AC23/M0 仍未验收。下一项推进全局代次与旧快照恢复。回退本项测试、驱动和独立模块依赖即可，无正式 API 或生产迁移。

## Redis 服务端超时与恢复（2026-10-29）

`codex/session-redis-timeout` 从成功 fetch 后的 `f672cf4` 创建。新增三个串行用例，只对驱动创建的私有 Redis 执行有期限的 `CLIENT PAUSE 5000 ALL`，不关闭客户端池来替代服务端故障。暂停影响整个夹具，禁止这些用例并行运行；即使断言失败也会在暂停期限后恢复。

- 两个同进程 TLS API 并发读取/退出，在 4 秒验证预算内返回 503，均不发送 Cookie；恢复后原会话仍有效，显式退出后两端均拒绝旧 Cookie，并可重新接纳恰好三个会话。
- 屏障确认会话已加载后暂停 Redis，再释放保存：返回 503 且无 Cookie；恢复后原会话仍有效、绝对期限不变，重复保存不增加名额，跨 API 撤销仍有效。
- 新登录保存超时返回 503 且无 Cookie；恢复后检查未送达登录占用零或一个名额，再验证总占位不超过三个。本次实测占用零个，但不将它解释为超时必然回滚。正式登录仍需单独设计结果核验和遗留名额处置。

Docker Desktop Linux arm64、Go 1.26.10、Redis 7.2.4：原七项 race 基线通过（3.393 秒）；最终十项行为/race 通过（18.562 秒），gofmt、go mod verify、vet 同时通过。使用下方原复现命令，没有新增依赖、驱动参数、正式入口或迁移；回退新增测试与文档即可。

本项是命令处理暂停后恢复，未重启 Redis、加载旧快照或丢失数据；不是网络分区、内存不足、跨主机或独立 API 进程下的 Redis 故障实验。认证代次、旧快照恢复及完整 M0 仍待验证。排队/恢复语义见 [Redis 官方说明](https://redis.io/docs/latest/commands/client-pause/)。

## 独立 API 进程与重启（2026-10-29）

`codex/session-process-recovery` 从成功 fetch 后的主线 `e424392` 创建，唯一目标是补齐进程边界验证。复用原有测试处理器和存储适配器，通过同一 race 测试二进制启动两个子进程，各有一个 TLS HTTP server、SCS manager 和 Redis 连接池；PID 与父进程、另一 API 均不同。父进程只信任子进程经管道传回的测试证书，没有关闭 TLS 校验；管道屏障不作为 HTTP 端点暴露。

- 24 个并发请求分发到两个进程，恰好 3 个成功、21 个冲突；失败不发送 Cookie，成功会话在两个进程均可读取。
- 管道确认 A 已加载旧会话，再由 B 完成撤销，随后释放 A 保存：返回 401 且不发 Cookie。
- 对执行撤销的 B 发送 SIGKILL 并等待退出，断言退出信号；A 继续拒绝旧会话、接受有效会话。使用相同 Redis 命名空间启动新的 B 后，两进程仍拒绝旧会话、接受另外两个有效会话；恰好能补一个名额，之后两端均拒绝超额登录。
- 正常子进程关闭控制管道后退出并回收；子进程设置 race 首错立即退出，并检查退出状态。强制结束只用于上述故障注入。

Docker Desktop Linux arm64、Go 1.26.10、Redis 7.2.4 下，既有六项基线通过（1.306 秒）；新增后完整七项行为测试及 race 通过（3.405 秒），其中进程用例 2.08 秒，gofmt、go mod verify、vet 同时通过。`TestSessionAPIProcess` 是子进程入口，父进程扫描时跳过，实际由进程用例执行。命令仍为下方 `scripts/test-session.py`，无需新依赖或镜像。

Redis 在整个用例中持续存活，未恢复快照；这只证明 API 进程丢失内存不影响 Redis 中的会话状态，不证明 Redis 宕机/持久化、旧快照恢复、认证代次、跨主机高可用或生产准入。下一项是认证代次与 Redis 故障/旧快照恢复验证，完整 AC23/M0 仍未通过。

## 原名额与撤销子项（同进程基线）

- SCS v2.9.0、go-redis v9.23.0、Go 1.26.10；真实 Redis 7.2.4，Docker Desktop Linux arm64。
- 两个独立 SCS manager、Redis 连接池及 TLS HTTP server，共处一个 Go 测试进程。不是两个 API 进程、跨主机或 Gin 中间件验证。
- 24 个并发请求分发到两个 API，恰好 3 个成功、21 个冲突；失败不发送 Cookie。两个 API 都能读取成功会话，撤销后都拒绝旧 Cookie，并释放一个名额。
- 屏障确保 API A 已加载旧会话，API B 撤销完成后才允许 A 保存；A 被拒绝，不能复活旧凭证或重新占位。
- 撤销凭证即使改换主体也不能保存；失败不占用原主体或新主体的名额，两者仍各可接纳三个会话。
- 空闲过期拒绝迟到续期并释放名额；同一凭证重复保存仍只占一个名额；绝对期限不可修改，过期绝对期限不能创建会话。
- 关闭一个 Redis 客户端连接池模拟访问不可用：返回 503，不清除 Cookie，不冒充服务端退出成功；另一个 API 仍能访问原会话。不是 Redis 服务宕机、网络超时或内存不足演练。
- Cookie 验证 Secure、HttpOnly、SameSite=Strict；Redis 键只含凭证 SHA-256 摘要。公开 HTTP 面、日志扫描和浏览器行为未验收。

本次 `gofmt`、`go mod verify`、`go vet`、六项 `go test -race -count=1` 全通过（测试进程报告 1.311 秒），迁入后的五项基线也通过（1.307 秒）。旧实验曾发现 Redis Cmd 接口误用与客户端 Timeout 并发修改，迁入版已修正；不将旧失败算作本次复现。

## 机制与限制

Redis Lua 将名额检查与保存串行执行；按主体的有序集合索引释放已到期名额，无全库扫描。撤销仅删除载荷，不立即删除记录，记录保留至绝对期限，拒绝后续保存；若直接删除整条记录，存储无法区分迟到保存与新会话。代价是保留撤销/过期元数据直到最长 12 小时；本次未进行内存容量测试。

时间判定使用 Redis TIME；SCS 生成绝对期限及有效载荷，适配器仅接受 12 小时内的普通会话。读请求手动 Load，不自动 Commit；本实验未验证有效活动采样、后台轮询策略或 SCS LoadAndSave 与 Gin 的生命周期。账号为固定、预认证测试主体；原有用例没有账号检查，新增代次用例通过可选 PostgreSQL 夹具核验，仍无密码、正式账号数据库或当前资源权限。

Lua 的串行性不等于运行时错误回滚；内存不足等中途写入失败、Redis 旧快照恢复、灾备代次、凭证轮换、比赛/管理会话、CSRF、PostgreSQL 对比和负载均未完成。独立 API 进程重启的有限证据见上节，不能将此适配器直接用于生产，也不代表 AC23 或完整 M0 通过。

## 复现

测试驱动仅使用本机已存在的工具镜像与 Redis 夹具镜像，不自动拉取：

- `oj-toolchain:local`：本仓库 Dockerfile 的 `toolchain` target；可用 `docker --context desktop-linux build --target toolchain -t oj-toolchain:local .` 构建，首次需正常网络/证书信任。
- 固定 Redis 镜像 ID `sha256:92dbfb0bc408385074c83eb3a868b5f273d425f079f2af808403d22dcd94b667`，本机 `redis/redis-stack-server:7.2.0-v9` 缓存。只启动其中 redis-server，不加载 Stack 模块。这是本机旧夹具，不是可移植镜像分发或生产版本准入。

其他机器可通过 `--redis-image <已有镜像引用>` 指定已准备的 Redis 镜像；驱动先检查本地存在，绝不自动拉取。更换版本需记录版本并重新验证，不能沿用本页结论。

容器内访问 Go 代理出现 `x509: certificate signed by unknown authority`。此次用宿主正常 TLS 与 `sum.golang.org` 校验下载到独立临时缓存，构建/测试仍在 Docker 中；没有关闭证书或模块校验。具备宿主 Go 下载能力时，在本目录执行：

```sh
GOPROXY=https://proxy.golang.org GOSUMDB=sum.golang.org GOMODCACHE=/private/tmp/oj-session-mod go mod download all
```

回到仓库根目录执行实际验证入口：

```sh
python3 scripts/test-session.py --module-cache /private/tmp/oj-session-mod
```

驱动创建随机命名的 internal 网络及一次性 Redis，无宿主端口、无用户数据库或 Docker socket 挂载；有限轮询就绪，测试命令上限 300 秒。测试容器只读挂载实验源码/缓存，`GOPROXY=off`，`run.sh` 创建私有 PostgreSQL 后校验模块并运行 vet/race；正常完成或命令失败后清理本次测试容器、Redis 和网络，不删除 volume。强制终止宿主驱动后的残留清理未验收。

## 依赖依据

- [SCS 官方接口](https://github.com/alexedwards/scs)：MIT；本次下载的 v2.9.0 源码支持 Go 1.12+，无直接外部依赖。默认 Store Commit 为新增/覆盖语义，不能直接满足本实验的撤销要求。
- [go-redis 发布记录](https://github.com/redis/go-redis/releases)：v9.23.0，BSD-2-Clause，要求 Go 1.26.0；本机工具链 1.26.10 满足。传递依赖已锁定在本目录 go.mod/go.sum。
- [Redis 官方许可说明](https://redis.io/legal/licenses/)与[安全说明](https://github.com/redis/redis/security)：客户端许可不等于 Redis/Stack 服务端许可或安全准入。本次没有全面漏洞扫描、SBOM、镜像安全审计、二进制体积或生产版本选择；只对已有隔离夹具做行为实验。

回退只需移除本实验目录、测试驱动和相关文档；无数据库迁移、业务接口或主模块依赖变更。
