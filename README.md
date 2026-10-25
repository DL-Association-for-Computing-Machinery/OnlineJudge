# OnlineJudge

面向东华理工大学计算机协会的训练与校赛平台。当前已建立可运行的工程骨架：API 进程健康检查、React 预览页和独立 Worker 命令入口；尚未实现登录、数据库、提交或判题业务。

## 当前交付与下一步

- [精简交付基线 v1.0](docs/delivery-plan.md)：技术栈引入时机、训练试用/完整训练范围、分阶段验收和待补条件。
- 本轮为 0A 工程骨架；下一步是 GNU++17 文本题的真实提交、持久化、隔离执行与故障恢复实验，不能将当前健康接口视为判题就绪。
- 模块化单体＋独立判题 Worker；不创建空业务层，不把所有模块拆成服务。
- 既有 Redis 会话路线保留待验证，新增 PostgreSQL 会话对比；未做未经确认的替换。
- 暂未引入 PostgreSQL、Redis、sqlc、Goose、River、go-judge 或监控备份服务；它们随实际功能和验证目标接入。
- 多文件/ZIP 提交、自测与辅助能力不作为训练试用前置；完整语言、比赛及后续需求保留。
- 0A 已完成本地验证，按治理、骨架与 CI、项目文档分组提交；远端同步状态以 Git 引用为准。未部署，远端 CI 和分支保护尚未验证。

## 运行环境

Go 工具链锁定为 `go.mod` 中的 1.26.10（本机可由 Go 自动下载到缓存）；前端使用 pnpm 11.25.0，建议 Node 24.19.0，见 [Node 版本](web/.node-version)。兼容范围见 [package.json](web/package.json)，锁文件为 `go.sum` 和 `web/pnpm-lock.yaml`。

本轮本地验证：macOS arm64，Go 1.26.10；pnpm 自带运行环境实际为 Node 24.19.0，本机 shell 的 Node 22.17.1 不等于 pnpm 子命令运行版本。未验证 Linux 判题、Docker 或离线安装。首次依赖安装需要网络；Makefile 将 Go 工具链/模块/构建缓存和二进制放在忽略的 `.local/`。

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

`make worker` 只执行骨架检查、明确提示任务/沙箱未接入后退出，不启动判题服务。API 可通过 `OJ_API_ADDR` 改监听地址；若改端口，同时调整本地 Vite 代理配置。当前无秘密配置，未接入 .env 自动加载。

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

产物位于忽略的 `.local/bin/` 和 `web/dist/`。`pnpm --dir web preview` 仅预览静态构建，不提供生产 API 反代；生产入口、HTTPS、容器、数据库迁移与部署命令尚未配置，不用 Vite dev server 承担正式比赛。

GitHub Actions 已配置 `governance`、`backend`、`frontend` 三个真实检查，远端尚未运行。原 `bootstrap-ci` 命令保留用于检查空骨架，当前有应用时预期失败，已从 workflow 替换；未放宽其行为。

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
- [CI/CD](docs/ci-cd.md)
- [交付与验证记录](docs/delivery-log.md)
- [项目根规则](AGENTS.md)

已选方案不等于已引入或已验证，实际依赖以配置和锁文件为准；运行结果及限制记录在交付日志。
