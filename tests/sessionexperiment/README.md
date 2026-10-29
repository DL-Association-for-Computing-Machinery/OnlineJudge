# 会话名额与撤销实验

2026-10-29，分支 `codex/session-admission`；成功 `git fetch origin main` 后从 `99ca923` 创建。迁入旧工作区实验，原工作区保留；交付状态见[交付日志](../../docs/delivery-log.md)。

唯一目标：用 SCS 和真实 Redis 验证普通会话三名额，以及跨 API 撤销后迟到保存不能复活会话。所有适配器和 HTTP 处理器只存在于 `_test.go`，不接入应用，不提供可部署登录入口。独立 Go 模块避免把实验依赖引入正式程序；根目录 `go test ./...` **不会执行本模块**，当前 CI 未接入本实验。

## 已执行

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

时间判定使用 Redis TIME；SCS 生成绝对期限及有效载荷，适配器仅接受 12 小时内的普通会话。读请求手动 Load，不自动 Commit；本实验未验证有效活动采样、后台轮询策略或 SCS LoadAndSave 与 Gin 的生命周期。账号为固定、预认证测试主体；没有密码、账号数据库、当前权限与认证代次检查。

Lua 的串行性不等于运行时错误回滚；内存不足等中途写入失败、Redis 旧快照恢复、灾备代次、凭证轮换、比赛/管理会话、CSRF、PostgreSQL 对比、负载与独立进程恢复均未完成。不能将此适配器直接用于生产，也不代表 AC23 或完整 M0 通过。

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

驱动创建随机命名的 internal 网络及一次性 Redis，无宿主端口、无用户数据库或 Docker socket 挂载；有限轮询就绪，测试命令上限 300 秒。测试容器只读挂载实验源码/缓存，`GOPROXY=off`，先校验模块再运行 vet/race；正常完成或命令失败后清理本次测试容器、Redis 和网络，不删除 volume。强制终止宿主驱动后的残留清理未验收。

## 依赖依据

- [SCS 官方接口](https://github.com/alexedwards/scs)：MIT；本次下载的 v2.9.0 源码支持 Go 1.12+，无直接外部依赖。默认 Store Commit 为新增/覆盖语义，不能直接满足本实验的撤销要求。
- [go-redis 发布记录](https://github.com/redis/go-redis/releases)：v9.23.0，BSD-2-Clause，要求 Go 1.26.0；本机工具链 1.26.10 满足。传递依赖已锁定在本目录 go.mod/go.sum。
- [Redis 官方许可说明](https://redis.io/legal/licenses/)与[安全说明](https://github.com/redis/redis/security)：客户端许可不等于 Redis/Stack 服务端许可或安全准入。本次没有全面漏洞扫描、SBOM、镜像安全审计、二进制体积或生产版本选择；只对已有隔离夹具做行为实验。

回退只需移除本实验目录、测试驱动和相关文档；无数据库迁移、业务接口或主模块依赖变更。
