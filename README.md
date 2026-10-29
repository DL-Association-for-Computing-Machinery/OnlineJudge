# OnlineJudge

面向东华理工大学计算机协会的训练与校赛平台。当前进入 M0 技术实验：已有提交持久化、幂等回执和任务租约核心，并在独立本机 Linux 虚拟机验证 GNU++17 沙箱分项。登录、完整 Worker 判题链路及训练产品尚未交付。

## 当前交付与下一步

- [精简交付基线 v1.0](docs/delivery-plan.md)：技术栈引入时机、训练试用/完整训练范围、分阶段验收和待补条件。
- [首期开发计划](docs/phase-1-development-plan.md)：0B → 1A → 1B 的任务依赖、P0—P7 开发包、里程碑与验收证据要求。
- [M0 实验记录与复现](docs/m0-experiment.md)：数据库、双 API 进程、任务租约及 Linux 沙箱的实际证据与未覆盖项；M0 整体验收尚未通过，健康接口不代表判题就绪。
- [Worker mTLS 契约](api/worker.openapi.yaml)：认证领取、续租、结果回报及有效租约限定的源码/材料下载与哈希校验；两个真实控制端进程和 TLS 客户端已验证。新增真实沙箱结果经 mTLS 回报至 PostgreSQL 的联合测试，见[真实结果回报](docs/m0-experiment.md#真实结果回报)。
- `cmd/worker` 已接入单槽领取、材料准备、租约执行与冻结结果回报，复用 `internal/worker`、`internal/execution`、`internal/workerclient` 和 `internal/workerhost`。入口提供按固定槽独占、崩溃隔离标记、启动预检和信号停止；仅支持固定 `m0-sum-v2`，见[执行端命令入口](docs/m0-experiment.md#执行端命令入口)。两个独立 Worker/控制端的真实竞争执行与 SIGTERM 后接管已验证，见[双 Worker 实验](docs/m0-experiment.md#双-worker-真实执行与接管)。撤销/材料故障的专项命令与证据边界见[故障联调](docs/m0-experiment.md#撤销与材料故障联调)；会话实验及 M0 整体验收尚未通过。
- `cmd/worker-api` 自动扫描过期、撤销或凭据轮换后的任务租约，保留三次尝试上限及旧回报隔离；见[控制端任务自动恢复](docs/m0-experiment.md#控制端任务自动恢复)。这只恢复数据库任务，不重启 Worker 或解除沙箱隔离。
- 模块化单体＋独立判题 Worker；不创建空业务层，不把所有模块拆成服务。
- 既有 Redis 会话路线保留待验证，新增 PostgreSQL 会话对比；未做未经确认的替换。
- 已引入 PostgreSQL 实验、sqlc/pgx/Goose；go-judge 用于 Docker Desktop 专用容器实验。Redis 会话、River 与监控备份服务尚未接入。
- 多文件/ZIP 提交、自测与辅助能力不作为训练试用前置；完整语言、比赛及后续需求保留。
- 0A 和 M0 分项已按逻辑分组提交；main 已建立并保护，PR #1—#11 已合并；本次执行端入口通过独立分支交付，验证证据见交付日志。未部署，M0 整体仍未通过。

## 运行环境

Go 工具链锁定为 `go.mod` 中的 1.26.10（本机可由 Go 自动下载到缓存）；前端使用 pnpm 11.25.0，建议 Node 24.19.0，见 [Node 版本](web/.node-version)。兼容范围见 [package.json](web/package.json)，锁文件为 `go.sum` 和 `web/pnpm-lock.yaml`。

构建工具链由 Dockerfile 固定，应用依赖以 go.sum 和 pnpm-lock.yaml 为准。Docker Desktop 迁移的验证结果见交付日志；历史 Colima 证据不能替代本次验证。

## Docker 构建与一键启动（默认）

后续默认使用 Docker 构建和 Compose 启动。安装并启动支持 Linux 容器的 Docker Engine（macOS 需 Linux VM）与 Compose v2+；默认入口不需要宿主机安装 Go、Node、pnpm 或 Make。首次构建需联网获取 Dockerfile 中固定摘要的基础镜像和锁定依赖。

在仓库根目录执行：

```sh
docker --context desktop-linux compose up -d --build --wait
```

打开 [Docker 本机预览](http://127.0.0.1:8080/)，点击“检查 API 连接”。Nginx 提供 Vite 构建后的静态文件，将 `/api/` 转发到 Compose 内部的 `api:8080`；后端不发布宿主机端口。若 8080 被占用，执行 `OJ_WEB_PORT=18080 docker --context desktop-linux compose up -d --build --wait`，再访问 18080。后续源码修改需要重建，此入口不是热更新开发服务器。

```sh
docker compose build                 # 仅构建默认应用镜像
docker compose ps                    # 查看服务和健康状态
docker compose logs --tail=100        # 查看启动或请求错误
docker compose down                  # 停止并移除本 Compose 项目的容器、网络
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

后端包含格式、vet、race、全部命令构建；前端包含 lint、类型、测试、构建。SQL 生成检查在容器内比较生成前后内容；数据库检查使用容器内临时 PostgreSQL 14、迁移、并发租约与两个真实 API/mTLS 控制端。工具镜像不挂载 Docker socket，不访问用户数据库。测试每次运行，不以构建缓存替代测试。

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

`GET /api/v1/healthz` 仅代表进程存活；普通 Compose 入口不启动完整 Worker 产品。真实编译/运行、回报、双 Worker 接管与撤销/材料故障分别由专项验证，普通 CI 不运行特权实验。

## 文档入口

- [需求与验收](docs/requirements.md)
- [总体架构](docs/architecture/system-design.md)
- [技术决策 ADR-001](docs/architecture/001-go-gin.md)
- [前端与沙箱](docs/architecture/frontend-sandbox-options.md)
- [判题与提交](docs/architecture/judging-design.md)
- [题包导入](docs/architecture/problem-package-design.md)
- [存储与缓存](docs/architecture/storage-design.md)
- [会话与账号](docs/architecture/session-design.md)
- [比赛与赛务](docs/architecture/contest-design.md)
- [API 与 Worker 协议](docs/architecture/api-worker-design.md)
- [运维与实施阶段](docs/architecture/operations-design.md)
- [开发工作流](docs/development-workflow.md)
- [GitHub 仓库治理与配置](docs/github-governance.md)
- [CI/CD](docs/ci-cd.md)
- [开发安全检查与告警处置](docs/security-checks.md)
- [交付与验证记录](docs/delivery-log.md)
- [项目根规则](AGENTS.md)

已选方案不等于已引入或已验证，实际依赖以配置和锁文件为准；运行结果及限制记录在交付日志。
