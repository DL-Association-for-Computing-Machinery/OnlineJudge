# OnlineJudge

面向东华理工大学计算机协会的训练与校赛平台。当前处于 M0 技术实验：已有提交持久化、幂等回执、任务租约及固定材料的 Worker 实验链路；当前使用 Docker Desktop 专用容器验证 GNU++17 沙箱。正式登录、训练产品及 M0 整体验收尚未交付。

## 当前交付与下一步

- [开发与交付计划](docs/phase-1-development-plan.md)：发布范围、组件引入时机、P0—P7 与里程碑。
- [M0 记录](docs/m0-experiment.md)：Worker、数据库与会话实验的证据及剩余缺口，是当前进度入口。
- [会话实验](tests/sessionexperiment/README.md)：独立测试模块的复现与限制，尚未接入正式登录。
- [密码记录核心](docs/architecture/session-design.md#71-同步密码记录核心)与[主体凭据](docs/architecture/session-design.md#72-主体凭据持久化与条件更新)：显式策略的 Argon2id、PostgreSQL 状态与条件更新，未开放账号 HTTP 或正式登录。

## 运行环境

Docker 工具链使用 Go 1.26.10、pnpm 11.25.0、Node 24.19.0，与 `go.mod`、[Node 版本](web/.node-version)及 [package.json](web/package.json)对应；依赖锁文件为 `go.sum` 和 `web/pnpm-lock.yaml`。宿主无需另外安装这些构建工具。

构建工具链由 Dockerfile 固定，应用依赖以 go.sum 和 pnpm-lock.yaml 为准。Docker Desktop 迁移的验证结果见交付日志；历史 Colima 证据不能替代本次验证。

## Docker 构建与一键启动（默认）

默认使用 Docker 构建和 Compose 启动。macOS 使用 Docker Desktop 的 `desktop-linux` context；Linux CI 使用 Docker Engine 与 Compose v2+，省略下面命令中的 `--context desktop-linux`。首次构建需联网获取 Dockerfile 中固定摘要的基础镜像和锁定依赖。

在仓库根目录执行：

```sh
docker --context desktop-linux compose up -d --build --wait
```

打开 [Docker 本机预览](http://127.0.0.1:8080/)，点击“检查 API 连接”。Nginx 提供 Vite 构建后的静态文件，将 `/api/` 转发到 Compose 内部的 `api:8080`；后端不发布宿主机端口。若 8080 被占用，执行 `OJ_WEB_PORT=18080 docker --context desktop-linux compose up -d --build --wait`，再访问 18080。后续源码修改需要重建，此入口不是热更新开发服务器。

```sh
docker --context desktop-linux compose build            # 构建默认应用镜像
docker --context desktop-linux compose ps               # 查看服务和健康状态
docker --context desktop-linux compose logs --tail=100   # 查看启动或请求错误
docker --context desktop-linux compose down             # 停止本项目容器、网络
```

当前只启动前端与基础 API，健康检查只证明进程/代理连通；不启动数据库、Worker、控制端或判题沙箱，不代表完整 OJ 可用。应用容器使用非 root、只读根文件系统；Nginx 临时文件只写入内存目录。当前默认服务没有持久化业务数据或 volume；后续增加数据库必须同步配置持久化与迁移步骤，不使用 `down -v` 清理数据。HTTPS、正式部署与离线制品交付尚未配置。

Dockerfile 的 Go、Node、pnpm 与现有配置保持一致，基础镜像固定多架构索引摘要；依赖仍使用 `go.sum` 和 `pnpm-lock.yaml`。`.dockerignore` 限定构建上下文，排除本地 Git、缓存、环境文件和常见凭据文件。更换工具链时需同步 Dockerfile 和对应锁定入口，并重新验证容器检查及启动流程。

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

真实 Worker 专项（当前固定 macOS arm64、Docker Desktop LinuxKit 7.0.14）：

```sh
python3 scripts/test-worker.py all
# 也可选择 sandbox / report / dual-worker / worker-fault
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

`GET /api/v1/healthz` 仅代表进程存活；普通 Compose 入口不启动完整 Worker 产品。真实编译/运行、回报、双 Worker 接管与撤销/材料故障分别由专项验证，普通 CI 不运行特权实验。

## 文档入口

完整目录与维护归属见[文档索引](docs/README.md)。已选方案不等于已引入或已验证；依赖以配置和锁文件为准，历史验证见[交付日志](docs/delivery-log.md)。
