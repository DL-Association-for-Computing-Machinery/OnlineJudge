# OnlineJudge

面向东华理工大学计算机协会的训练与校赛平台。当前进入 M0 技术实验：已有提交持久化、幂等回执和任务租约核心，并在独立本机 Linux 虚拟机验证 GNU++17 沙箱分项。登录、完整 Worker 判题链路及训练产品尚未交付。

## 当前交付与下一步

- [精简交付基线 v1.0](docs/delivery-plan.md)：技术栈引入时机、训练试用/完整训练范围、分阶段验收和待补条件。
- [首期开发计划](docs/phase-1-development-plan.md)：0B → 1A → 1B 的任务依赖、P0—P7 开发包、里程碑与验收证据要求。
- [M0 实验记录与复现](docs/m0-experiment.md)：数据库、双 API 进程、任务租约及 Linux 沙箱的实际证据与未覆盖项；M0 整体验收尚未通过，健康接口不代表判题就绪。
- [Worker mTLS 契约](api/worker.openapi.yaml)：认证领取、续租、结果回报及有效租约限定的源码/材料下载与哈希校验；两个真实控制端进程和 TLS 客户端已验证。新增真实沙箱结果经 mTLS 回报至 PostgreSQL 的联合测试，见[真实结果回报](docs/m0-experiment.md#真实结果回报)。
- `cmd/worker` 已接入单槽领取、材料准备、租约执行与冻结结果回报，复用 `internal/worker`、`internal/execution`、`internal/workerclient` 和 `internal/workerhost`。入口提供跨进程独占、崩溃隔离标记、启动预检和信号停止；仅支持固定 `m0-sum-v2`，见[执行端命令入口](docs/m0-experiment.md#执行端命令入口)。下一步验证双 Worker 与故障恢复，再补会话实验。
- `cmd/worker-api` 自动扫描过期、撤销或凭据轮换后的任务租约，保留三次尝试上限及旧回报隔离；见[控制端任务自动恢复](docs/m0-experiment.md#控制端任务自动恢复)。这只恢复数据库任务，不重启 Worker 或解除沙箱隔离。
- 模块化单体＋独立判题 Worker；不创建空业务层，不把所有模块拆成服务。
- 既有 Redis 会话路线保留待验证，新增 PostgreSQL 会话对比；未做未经确认的替换。
- 已引入 PostgreSQL 实验、sqlc/pgx/Goose；go-judge 仅进入专用 Linux VM 的隔离实验。Redis 会话、River 与监控备份服务尚未接入。
- 多文件/ZIP 提交、自测与辅助能力不作为训练试用前置；完整语言、比赛及后续需求保留。
- 0A 和 M0 分项已按逻辑分组提交；main 已建立并保护，PR #1—#11 已合并；本次执行端入口通过独立分支交付，验证证据见交付日志。未部署，M0 整体仍未通过。

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

`make worker` 现在要求显式 M0 配置，缺失时非零退出。信号验收与运行建议直接使用 `make go-build` 生成的 `.local/bin/worker`，避免把信号发给 `go run` 包装进程。必须设置 `OJ_SANDBOX_EXPERIMENT=1`、`OJ_WORKER_CONTROLLER_URL`（字面 loopback HTTPS）、`OJ_WORKER_CA_FILE`、`OJ_WORKER_CERT_FILE` 和 `OJ_WORKER_KEY_FILE`；执行端不需要数据库凭据。专用容器与节点须由管理员预先准备，见[命令入口](docs/m0-experiment.md#执行端命令入口)。

API 可通过 `OJ_API_ADDR` 改监听地址；若改端口，同时调整 Vite 代理。默认没有业务接口；显式设置 `OJ_EXPERIMENT_TOKEN_FILE` 与 `OJ_DATABASE_URL` 可启用仅 loopback 的 M0 提交接口，见[实验契约](api/experiment.openapi.yaml)与[M0 配置边界](docs/m0-experiment.md)。凭据只放私有本地文件，不提交；未接入 .env 自动加载。

`make worker-api` 启动独立控制端 HTTPS 进程，也可运行 `make go-build` 生成的 `.local/bin/worker-api`。必须提供 `OJ_WORKER_API_ADDR`、`OJ_WORKER_CA_FILE`、`OJ_WORKER_CERT_FILE`、`OJ_WORKER_KEY_FILE` 和 `OJ_DATABASE_URL`，初始化及证书要求见 [M0 Worker 入口](docs/m0-experiment.md#worker-mtls-租约入口)。这是持有数据库连接的控制端，其服务端证书与执行端节点证书分别配置。缺少配置会拒绝启动，无默认开放端口。

控制端成功绑定监听地址后立即启动任务恢复扫描；每轮最多 64 条、5 秒预算，完成后等待 1 秒再扫描。扫描覆盖数据库中符合既有恢复条件的全部材料版本，不随该控制端的领取版本过滤。暂时失败会记录不含数据库错误正文的告警并继续限速重试；停止控制端会取消并等待扫描排空。无新增开关；回退旧二进制将停止自动扫描，数据库历史保留。

材料下载另需 `OJ_MATERIAL_ROOT` 指向专用持久化目录；先显式初始化合成文件，再让控制端只读使用同一目录。命令、预算和回退见[材料入口](docs/m0-experiment.md#有效租约限定的材料入口)。未配置时两个材料接口返回 503，原租约接口保持可用。

可选 `OJ_EXPERIMENT_MATERIAL_VERSION=m0-sum-v2` 为 GNU++17 实验固定执行环境；初始化夹具、提交 API 和 Worker 控制端须配置相同版本。未设置仍使用原 `m0-sum-v1`，未知值拒绝启动实验服务/注册夹具。执行端只接受 v2，不会执行 v1 或未知清单。新版本绑定已核验的本机 Linux arm64 镜像 ID、编译参数和预算，不重标旧提交；切换、并存和回退限制见 [固定执行环境](docs/m0-experiment.md#固定执行环境与新材料版本)。

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

CI 按完整变更范围执行：纯文档/根规则只跑治理；前端变更跑前端；Go/数据库变更跑后端与 PostgreSQL 集成；流水线、共享构建配置或未知路径跑全量。四个必需检查始终给出明确结果；不受影响的应用测试会标注未执行，分类/测试失败不能变成成功跳过。手动触发可运行全量检查，路径矩阵和限制见 [CI 分层](docs/ci-cd.md#按变更影响选择检查)。

已有 hooks 安装入口为 `sh scripts/install-hooks.sh`，仅改变当前克隆，遇到已有配置拒绝覆盖；本轮不重新安装或修改 hooks。持续授权的开发流程为“对应任务分支 → 实现与验证 → 原子提交 → 推送 → PR → 当前 head 的 CI 与 review → 条件满足后 squash 合并并回读”，GitHub 操作统一使用 `gh`。2026-10-28 新增后续 PR 的条件合并授权，无需逐次确认；不绕过保护，不启用平台自动合并，不授权部署。一个 PR 对应一个明确功能或预先界定、可独立验收的阶段；具体边界见[根规则第 7 节](AGENTS.md#7-git-与审查)与[开发工作流](docs/development-workflow.md)。

真实沙箱结果与临时数据库联合验证：在上述专用 VM/容器已启动时，执行 `OJ_SANDBOX_EXPERIMENT=1 make report-test`。该命令先运行数据库与双 API 回归，再在独立空库运行单槽 Worker 连续两题与丢失回执重报、原 8 项结果回报用例，以及真实 Worker 命令的 AC/WA、重复启动拒绝、SIGINT 空闲停止和 SIGTERM 在途停止。需要独占专用沙箱，入口隔离标记必须无未决状态；测试不自动清除旧隔离标记。普通 CI 不运行此特权实验，`sandbox-test` 不替代它。

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
