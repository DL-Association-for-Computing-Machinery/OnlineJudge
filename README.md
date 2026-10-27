# OnlineJudge

面向东华理工大学计算机协会的训练与校赛平台。当前进入 M0 技术实验：已有提交持久化、幂等回执和任务租约核心，并在独立本机 Linux 虚拟机验证 GNU++17 沙箱分项。登录、完整 Worker 判题链路及训练产品尚未交付。

## 当前交付与下一步

- [精简交付基线 v1.0](docs/delivery-plan.md)：技术栈引入时机、训练试用/完整训练范围、分阶段验收和待补条件。
- [首期开发计划](docs/phase-1-development-plan.md)：0B → 1A → 1B 的任务依赖、P0—P7 开发包、里程碑与验收证据要求。
- [M0 实验记录与复现](docs/m0-experiment.md)：数据库、双 API 进程、任务租约及 Linux 沙箱的实际证据与未覆盖项；M0 整体验收尚未通过，健康接口不代表判题就绪。
- [Worker mTLS 契约](api/worker.openapi.yaml)：认证领取、续租、结果回报及有效租约限定的源码/材料下载与哈希校验；两个真实控制端进程和 TLS 客户端已验证，回报仍为合成夹具。
- 模块化单体＋独立判题 Worker；不创建空业务层，不把所有模块拆成服务。
- 既有 Redis 会话路线保留待验证，新增 PostgreSQL 会话对比；未做未经确认的替换。
- 已引入 PostgreSQL 实验、sqlc/pgx/Goose；go-judge 仅进入专用 Linux VM 的隔离实验。Redis 会话、River 与监控备份服务尚未接入。
- 多文件/ZIP 提交、自测与辅助能力不作为训练试用前置；完整语言、比赛及后续需求保留。
- 0A 和 M0 分项已按逻辑分组提交；main 已建立并保护，PR #1、#2 和材料入口 [PR #3](https://github.com/DL-Association-for-Computing-Machinery/OnlineJudge/pull/3) 已合并；本次固定执行环境通过独立分支交付，验证证据见交付日志。未部署，M0 整体仍未通过。

## 运行环境

Go 工具链锁定为 `go.mod` 中的 1.26.10（本机可由 Go 自动下载到缓存）；前端使用 pnpm 11.25.0，建议 Node 24.19.0，见 [Node 版本](web/.node-version)。兼容范围见 [package.json](web/package.json)，锁文件为 `go.sum` 和 `web/pnpm-lock.yaml`。

本轮本地验证：macOS arm64，Go 1.26.10、PostgreSQL 14.19；pnpm 自带 Node 24.19.0。另建 Colima `oj-m0` Linux arm64 VM（2 CPU/2 GiB），实测 go-judge 1.13.3、GCC 15.3.0；该实验不证明生产部署或完整判题链路通过。首次依赖安装需要网络；Makefile 将 Go 工具链/模块/构建缓存和二进制放在忽略的 `.local/`。

## 安装与启动

从仓库根目录安装前端锁定依赖：

```sh
pnpm --dir web install --frozen-lockfile --ignore-scripts
```

两个终端分别运行：

```sh
make api
```

```sh
pnpm --dir web dev
```

打开 [本机预览](http://127.0.0.1:5173/)，点击“检查 API 连接”。Vite 将 `/api` 代理到 `127.0.0.1:8080`。默认仅监听本机；用 Ctrl+C 停止。首次 Go 命令可能下载固定工具链和模块；若本机 Go 代理不可达，可对本次命令设置 `GOPROXY=https://proxy.golang.org`，无需修改全局配置。

`GET /api/v1/healthz` 返回 `{"status":"ok","service":"api","scope":"process"}`，只说明 API 可响应，不表示数据库、Worker 或沙箱就绪。契约见 [OpenAPI](api/openapi.yaml)，未知接口返回 404，不支持的方法返回 405。

`make worker` 仍只提示任务/沙箱未接入后退出。API 可通过 `OJ_API_ADDR` 改监听地址；若改端口，同时调整 Vite 代理。默认没有业务接口；显式设置 `OJ_EXPERIMENT_TOKEN_FILE` 与 `OJ_DATABASE_URL` 可启用仅 loopback 的 M0 提交接口，见[实验契约](api/experiment.openapi.yaml)与[M0 配置边界](docs/m0-experiment.md)。凭据只放私有本地文件，不提交；未接入 .env 自动加载。

`make worker-api` 启动独立控制端 HTTPS 进程，也可运行 `make go-build` 生成的 `.local/bin/worker-api`。必须提供 `OJ_WORKER_API_ADDR`、`OJ_WORKER_CA_FILE`、`OJ_WORKER_CERT_FILE`、`OJ_WORKER_KEY_FILE` 和 `OJ_DATABASE_URL`，初始化及证书要求见 [M0 Worker 入口](docs/m0-experiment.md#worker-mtls-租约入口)。这是持有数据库连接的控制端，`cmd/worker` 执行端仍是骨架。缺少配置会拒绝启动，无默认开放端口。

材料下载另需 `OJ_MATERIAL_ROOT` 指向专用持久化目录；先显式初始化合成文件，再让控制端只读使用同一目录。命令、预算和回退见[材料入口](docs/m0-experiment.md#有效租约限定的材料入口)。未配置时两个材料接口返回 503，原租约接口保持可用。

可选 `OJ_EXPERIMENT_MATERIAL_VERSION=m0-sum-v2` 为本次 GNU++17 实验固定执行环境；初始化夹具、提交 API 和 Worker 控制端须配置相同版本。未设置仍使用原 `m0-sum-v1`，未知值拒绝启动实验服务/注册夹具。新版本绑定已核验的本机 Linux arm64 镜像 ID、编译参数和预算，不重标旧提交；切换、并存和回退限制见 [固定执行环境](docs/m0-experiment.md#固定执行环境与新材料版本)。执行端仍未交付。

## 检查与构建

```sh
make check
```

该命令运行 Go 格式检查、vet、race 测试、API/Worker 构建，以及前端 lint、类型检查、测试、生产构建和仓库治理回归。也可分别运行：

```sh
make go-check go-build
make web-check
python3 scripts/repo_policy.py repository
python3 -B -m unittest discover -s tests/governance -v
```

真实临时数据库、两个提交 API 子进程、两个 mTLS 控制端子进程及两个节点客户端测试：`make db-test`（需本机 `initdb/pg_ctl/createdb`、Python 3，自动建立独立私有目录并停止临时数据库）。专用 `oj-m0` VM 与沙箱启动后运行 `OJ_SANDBOX_EXPERIMENT=1 make sandbox-test`；不可在共享 Docker 主机或宿主直接运行选手代码，详细前置与清理见[M0 记录](docs/m0-experiment.md)。数据库和沙箱测试不在快速 `make check` 中静默跳过或冒充通过。

产物位于忽略的 `.local/bin/` 和 `web/dist/`。独立迁移程序为 `.local/bin/migrate`；实验数据库迁移/回退边界见 M0 记录。`pnpm --dir web preview` 仅预览静态构建；生产入口、HTTPS 与部署尚未配置，不用 Vite dev server 承担正式比赛。

GitHub Actions 的 `governance`、`backend`、`frontend` 和真实 PostgreSQL `integration` 首轮已全部通过，对应提交及运行链接见[交付日志](docs/delivery-log.md)；后续提交以对应 head 的检查为准。专用 Linux 沙箱不进入普通 PR 快速 CI。原 `bootstrap-ci` 命令保留用于检查空骨架，当前有应用时预期失败，已从 workflow 替换；未放宽其行为。

已有 hooks 安装入口为 `sh scripts/install-hooks.sh`，仅改变当前克隆，遇到已有配置拒绝覆盖；本轮不重新安装或修改 hooks。初始化不自动授权 Git 提交、推送或发布。

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
- [交付与验证记录](docs/delivery-log.md)
- [项目根规则](AGENTS.md)

已选方案不等于已引入或已验证，实际依赖以配置和锁文件为准；运行结果及限制记录在交付日志。
