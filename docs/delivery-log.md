# 工程交付记录

## 2026-10-29：阻止强制提交本地资料

唯一目标：补齐既有 pre-commit 对本地个人绑定、日志、SQLite/数据库（含 WAL/SHM）及备份文件的路径检查；即使 git add -f 也拒绝，SQL 迁移/查询、锁文件和环境示例仍可提交。测试使用临时 Git 仓库实际安装 hooks，覆盖 14 类拒绝路径、输出不泄漏内容及合法提交。没有新增 hook 类型、联网或自动删除行为；回退恢复策略函数及相应测试/说明即可。

本地完整治理 46 项、仓库策略及 diff 检查通过；首次沙箱端口检查的权限失败已通过正常审批完整复验，未跳过断言。应用代码和 CI 不变，未重跑无关应用/Worker 实验。远端 CI 与合并结果以对应 PR/head 为准。

前置规则整理 PR #29 已合并为 `1ecdf7d`（head `920f839`）；CI 40890331393 四个必需门禁和 security 成功，应用按文档范围跳过。完整差异自审无阻断，无独立人工批准；此分支从成功 fetch 后的新主线创建。

## 2026-10-29：根规则精简与合并后清理流程

唯一目标：减少根规则重复并明确合并后本地清理的交付责任。根文件从含清理草案的 142 行缩至 54 行；知识回写与文档同步细则迁至 `docs/knowledge-maintenance.md`，根文件保留必读入口和授权边界。工作流补充 fetch/PR head/合并提交核验、工作区占用和本地资料保护、主线仅快进更新及清理后回读。不新增删除型 hook，不修改应用、CI 或远端保护。

验证：仓库治理、diff 和本地文档引用检查通过；人工逐项核对持续授权、安全边界、事务细则及清理条件。此提交仅规则/文档变更，不重复运行无关应用测试。源码状态以本任务提交为准，PR/CI/合并结果通过 GitHub 与知识库交付索引回读。

## 2026-10-29：会话名额与撤销子实验

唯一目标：用 SCS/真实 Redis 验证普通三会话名额与撤销后的迟到保存拒绝，支撑 P3/M0、AC23 子项。成功 fetch 后从 `99ca923` 创建 `codex/session-admission`，迁入旧实验工作区的测试模块并保留原工作区；不接入正式登录或改变产品规则。独立模块与测试适配器不进入正式构建，无数据库迁移，回退移除实验、驱动及文档即可。本轮用户授权提交/推送/PR，合并由用户决定。

Docker Desktop Linux arm64、Go 1.26.10、Redis 7.2.4：`python3 scripts/test-session.py --module-cache /private/tmp/oj-session-mod` 的基线五项 race 通过（1.307 秒）；新增撤销凭证改换主体仍被拒绝、两主体名额不被失败写入占用的回归后，最终六项 race 通过（1.311 秒），gofmt、go mod verify、vet 同时通过。驱动补充本地镜像参数、有限就绪等待与测试容器清理；使用已有镜像和只读模块缓存，不自动下载、不发布端口。最终代码为本节记录时的未提交工作区；PR/CI 以对应 head 为准。

24 并发请求恰好三次成功，跨 API 撤销后旧凭证/迟到保存拒绝；验证空闲过期、重复保存、绝对期限及客户端连接池失败返回 503。不等同于独立 API 进程、Redis 宕机/旧快照恢复、Gin/CSRF/账号权限、生产组件准入或完整 M0。根模块测试和当前 CI 不执行此嵌套模块；未重跑 Worker 特权实验。详情见[实验 README](../tests/sessionexperiment/README.md)。

## 2026-10-29：Go 模块漏洞处置

唯一目标：将 x/net v0.61.0 升级到官方修复版本 v0.62.0，消除 GO-2026-6866 与 GO-2026-6860；不修改业务接口、其他依赖或扫描门禁。从成功 fetch 后的主线 `13c1078` 创建 `codex/go-dependency-security`。回退可恢复本次 go.mod/go.sum，但会重新引入两个模块告警；无数据库迁移。

Docker Desktop、Go 1.26.10 实测：升级前 verbose 扫描为 0 可达/0 已导入包漏洞、3 个模块条目；升级后 0/0/1，两个 x/net 条目均消失。`go mod tidy` 仅改变 x/net 校验值，`go mod verify` 通过；默认和 sandbox 标签的 `go list -deps -test ./...` 均无 net/html 或 crypto/openpgp 包。GO-2026-5932 无上游修复版本，当前应用不适用，模块提示保留，未设置忽略；原因见[安全检查](security-checks.md)。

`docker --context desktop-linux compose -p oj-vuln-fix run --rm toolchain backend` 的格式、vet、race 测试和四个后端入口构建通过；同项目 `toolchain db` 的 PostgreSQL 集成、双 API 24 并发、重启/回执恢复和 v3 材料测试通过；`backend-audit` 复扫通过。镜像构建曾遇公共模块代理/校验服务连接拒绝，恢复后原配置构建成功，未关闭校验。仓库策略和 diff 检查通过。未重跑特权 Worker 实验，不宣称完整 M0 或部署；远端 CI 和合并以本 PR 最新 head 及交付回读为准。

原克隆 `.claude-obsidian.json` 为停止 Git 跟踪，文件仍在本地，`git check-ignore` 确认命中新增规则；不是文件丢失。本任务保留该既有暂存状态及 `.gitignore` 整理，不混入依赖修复 PR，也不覆盖其他既有改动。

## 2026-10-29：开发配置收敛与交付回读

本节为当前交付索引；下方各节保留其验证时点的快照，不能将“提交前/未运行”误读为当前状态。按顺序已核验并合并：

| PR | 唯一目标 | 合并提交 | 最新 head CI |
| --- | --- | --- | --- |
| [#21](https://github.com/DL-Association-for-Computing-Machinery/OnlineJudge/pull/21) | 分支起点与单目标规则对齐 | `daefe65` | [40876716530](https://github.com/DL-Association-for-Computing-Machinery/OnlineJudge/actions/runs/40876716530)，四门禁成功，应用按文档范围跳过 |
| [#22](https://github.com/DL-Association-for-Computing-Machinery/OnlineJudge/pull/22) | 容器结果接入必需门禁 | `b4be2f5` | [40877073240](https://github.com/DL-Association-for-Computing-Machinery/OnlineJudge/actions/runs/40877073240)，八项成功 |
| [#23](https://github.com/DL-Association-for-Computing-Machinery/OnlineJudge/pull/23) | 秘密与依赖扫描 | `fc95ada` | [40878643154](https://github.com/DL-Association-for-Computing-Machinery/OnlineJudge/actions/runs/40878643154)，九项成功 |
| [#24](https://github.com/DL-Association-for-Computing-Machinery/OnlineJudge/pull/24) | 只读开发自检 | `d790f65` | [40880041335](https://github.com/DL-Association-for-Computing-Machinery/OnlineJudge/actions/runs/40880041335)，九项成功 |

每项合并前核对本地/远端/PR head 一致、base 未变、完整差异自审、无现有 review/未解决讨论及 CLEAN/MERGEABLE，以 match-head-commit squash，合并后回读主线。没有独立人工批准、远端保护修改或部署。Go 三个未触达模块漏洞仍保留，未当作修复。

第五项唯一目标为当前文档入口去重：从成功 fetch 后的 `d790f65` 创建任务分支，纠正 README 的 v2/旧交付状态，移除 CI 文档中已被 Docker 迁移替代的当前命令，授权以根规则为唯一入口，操作和历史证据分别引用。旧原克隆五处改动仍保留。验证：仓库策略（172 文件）、diff 检查、配置/路径/引用人工核对通过；本项不改应用或流水线，提交时 CI 与知识库事务尚待完成，最终结果以本 PR 和知识库事务回读为准。

## 2026-10-29：仓库上传范围修正

唯一目标：减少个人配置和本地运行产物误提交，并去除当前文档中的个人 home 路径。基于最新 `origin/main` 的独立分支 `codex/repository-upload-hygiene`，不包含原克隆已有的其他文档改动。个人 `.claude-obsidian.json` 从版本控制移除，本机已有文件保留；忽略私钥/证书、日志、数据库文件及备份，SQL 迁移/查询、依赖锁文件和 `.env.example` 仍允许跟踪。

本页历史命令中的主克隆路径统一以 `${OJ_PRIMARY_CHECKOUT}` 脱敏，复现时设置为实际主克隆绝对路径；worktree 路径改用 `~/.codex/`，vault 路径相对主克隆。只替换路径，不改变历史验证结论。忽略规则不能阻止强制添加，也不会清除旧提交；本次不重写历史、不轮换凭据、不部署。

本地验证：`python3 scripts/repo_policy.py repository` 检查 171 文件通过，`git diff --check` 与 `git diff --cached --check` 通过；`git check-ignore` 验证 20 项本地文件路径均被忽略、5 项源文件/示例仍允许跟踪。回读确认个人绑定保留且不再跟踪，当前跟踪文件无个人 home 绝对路径。未改应用代码，未运行应用测试；远端 CI 以 PR 对应 head 为准。例行忽略规则维护无新增架构决策或故障知识，不重复写入知识库。

## 2026-10-29：只读开发环境自检

唯一目标：提供 `python3 scripts/doctor.py`（可选 `--json`、`--strict`、`--port`）检查工作区与 Docker 前置条件。禁用 Git 可选锁避免刷新 index，不 fetch、安装、切换分支或启动容器；工具错误不输出原始 stderr。缓存主线不能证明远端最新，资源展示不设未经压测的容量合格线。

验证：45 项治理测试通过，覆盖缺命令、主线/游离 HEAD、缺 hooks/主线引用、旧基线、脏工作区、Docker 超时/畸形响应、旧 Compose、真实占用端口及退出码。macOS Docker Desktop 实测 Linux Engine、Compose、hooks 和 8080 端口通过，工作区仅因本次待提交修改而警告；仓库策略及 diff 检查通过。首次沙箱内端口/socket 权限不足如实失败，经工具审批后在可访问环境复验通过，未跳过断言。

前置 PR #23 已合并为 `fc95ada`，九项 CI 成功；本分支从成功 fetch 后的该主线创建。自检不等于应用测试或 M0 验收，无部署。

## 2026-10-29：提交历史秘密与依赖漏洞检查

唯一目标：将可执行的秘密/依赖检查接入现有必需门禁；不升级应用依赖、不改远端保护或增加定时任务。前置 PR #22 已合并为 `b4be2f5`，八项 CI 成功；本任务从成功 fetch 后的该主线创建。

实际验证：Gitleaks v8.30.1 合成令牌拒绝及脱敏通过；基线 HEAD 的 22 个提交历史无命中。govulncheck v1.9.0 报告 0 可达、0 已导入包漏洞，但有 3 个未触达模块条目（详见 verbose 输出，不代表模块无漏洞）；pnpm 全依赖 audit 未发现已知漏洞。40 项治理回归、actionlint、仓库策略和 diff 检查通过。新提交自身的历史扫描及远端 CI 以 PR 最新 head 为准；无部署/独立人工批准。

合成拒绝自测首轮因 Gitleaks 默认不输出详细内容而未能验证脱敏；明确启用 verbose 后验证退出码 1、REDACTED 标记和原 token 不出现，未削弱断言。固定版本工具仅在容器内安装，扫描历史时禁网、无宿主挂载。工具准入、范围及告警处置见 [安全检查](security-checks.md)。

## 2026-10-29：容器验证接入必需门禁

唯一目标：容器冒烟失败必须阻止合并。实查远端 ruleset 25824684 仍要求 governance/backend/frontend/integration；修改后保留这些名称，由三个结果门禁依赖并核验 containers，不修改远端保护。原实现可在 backend-tests 成功、containers 失败时返回通过；新增回归覆盖前端/后端/混合/文档变化及失败、取消、缺失和意外跳过。

验证：39 项治理测试通过；仓库检查和 diff 检查通过。应用与 Compose 集成由本 PR CI 验证，提交时尚未完成远端 CI；没有部署。前置规则对齐 PR #21 已合并为 `daefe65`，本分支从成功 fetch 后的该主线创建。

## 2026-10-29：工作区与规则版本对齐

唯一目标：将旧初始化工作区尚未交付的分支起点与单 PR 范围约束移植到主线派生的任务分支。保留用户再次确认的持续交付、CI/review 通过后合并授权；不修改应用、CI 或远端保护。旧工作区的五处未提交改动原样保留，不将其历史状态全文覆盖到新主线。

新任务分支必须成功 fetch 后从 origin/main 创建，其他起点须用户明确指定；可独立验收/合并/回退的目标必须拆 PR，不以阶段名豁免。本次从 `d419df6` 创建 `codex/workspace-rules`，后续任务按同一规则推进。PR #1 已合并及旧分支清理的记录保留在原工作区，不重复作为当前状态发布。

验证：`python3 scripts/repo_policy.py repository`（168 文件）及 `git diff --check` 通过；完整差异自审核对了根规则、流程与模板的范围及授权一致性。本次为文档变更，未运行应用测试；提交时未运行远端 CI，合并结果以 PR 为准，无部署。

## 2026-10-29：Docker Desktop 工具链与 Worker 测试迁移（提交前快照）

本节保留提交前记录。后续 [PR #20](https://github.com/DL-Association-for-Computing-Machinery/OnlineJudge/pull/20) 已合并为 `d419df6`，CI 40873232906 八项成功；当前命令见 README，下文“未提交/未运行远端 CI”仅描述当时。

唯一目标：本机使用 Docker Desktop，构建/数据库工具容器化，移除 Makefile。基于本地 origin/main `436204c`，独立工作区 `~/.codex/worktrees/docker-tooling/OnlineJudge`，分支 `codex/docker-tooling`；原克隆五处既有改动未变。未提交、未推送、未建 PR、未运行远端 CI、未部署。

- Dockerfile/Compose 提供固定 Go、pnpm、sqlc v1.31.1、PostgreSQL 14 工具环境。CI 调用同一容器入口；保留稳定四门禁、影响范围和真实断言。Go 构建缓存留在 Docker volume；应用镜像不挂 Docker socket。
- `python3 scripts/test-worker.py all` 由 Docker 交叉编译宿主驱动与命令；宿主仅需 Python/Docker。选择固定 `desktop-linux`，两个带归属标签的禁网/无挂载/只读特权槽，拒绝接管外部来源或运行中的同名槽；跨工作区互斥。选手代码仅在 Linux 沙箱执行。
- Docker Desktop Linux arm64 / cgroup v2 / LinuxKit `7.0.14-linuxkit`，GCC 15.3.0 / go-judge v1.13.3。新执行配置为 `m0-gnu17-desktop-arm64-v1`，材料 v3；v1/v2 原始清单保留，v2 SHA-256 回归不变，执行端只接受 v3。配置/回退限制见 README 与 M0 文档。
- 停止确认从 Colima SSH 改为 Docker VM PID namespace 的短生命周期观察容器；无特权、无网络、无挂载、只读、cap-drop ALL。保持不可变容器 ID/启动时间、停止状态与已观察 PID 消失三层核验；任何不确定仍封锁旧执行器。
- 最终 `docker --context desktop-linux compose run --build --rm toolchain backend` 通过格式/vet/race/全部命令构建；`toolchain sqlc` 生成无差异；`toolchain db` 通过真实 PostgreSQL、迁移/租约/双控制端、双 API 24 并发/重启回执和 v3/v1 兼容。日志 `/tmp/oj-backend-verified.log`、`/tmp/oj-sqlc-verified.log`、`/tmp/oj-db-verified.log`。
- 前端容器 lint/typecheck/6 项测试/build 通过；最终 `python3 scripts/test-compose.py` 通过静态资源/SPA/API、权限、重建和停止恢复。日志 `/tmp/oj-frontend.log`、`/tmp/oj-compose-final.log`。
- 最终 Worker all 四套通过：受限编排与隔离、回报/单槽命令/信号退出、双 Worker 自然租约接管与旧回报拒绝、四类故障。故障专项 95.37 秒；日志 `/tmp/oj-worker-verified.log`。跨编译宿主专项不含 race，容器内后端/数据库保留 race。两个槽最后停止，临时数据库容器清理；没有启动/删除 Colima 或操作其他项目容器。
- 38 项治理回归、repository 检查与 diff 空白检查通过；新增拒绝接管已有资源回归。相对基线完整差异及新增文件已自审，无独立人工审查。历史 Make/Colima 结果仍作为历史证据保留，现行入口已更新。

自动审批曾拒绝向测试容器挂载 Docker socket，随后改用宿主驱动方案，没有绕过限制。Docker Desktop 与其他项目共享 VM，隔离边界不等同于原独立 Colima VM；仅限合成实验，不能据此宣称完整 M0 或生产准入。Docker 内核/镜像升级须重新核验并版本化，未验证其他架构。临时日志可能清理，重跑版本化命令获取新证据。

## 2026-10-28：撤销与材料故障联合验证

基于已合并 PR #17 的 `c3e3a48bfc0f39f760596458e9fb107311a3ee67`，独立 `codex/worker-fault-validation`。唯一目标为 P2、AC28/29 的四类故障联调；新增 `tests/sandbox/fault_test.go`、`make worker-fault-test` 及临时数据库入口。无应用行为、依赖、SQL、迁移、协议或 workflow 变化。以下为提交前实测快照，PR/CI/合并终态另行回读。

- 基线 `GOPROXY=off make go-check go-build GOMODCACHE=${OJ_PRIMARY_CHECKOUT}/.local/go-mod GOCACHE=/private/tmp/oj-lease-go-build` 通过格式、vet、race 和构建。日志 `/private/tmp/oj-fault-baseline.log`。
- `GOPROXY=off OJ_SANDBOX_EXPERIMENT=1 make worker-fault-test`（同上缓存参数）通过真实临时 PostgreSQL、双 API 24 并发及双控制端回归。四类故障专项 91.53 秒：禁用 11.02 秒、轮换 10.21 秒、缺失 34.46 秒、损坏 35.64 秒。日志 `/private/tmp/oj-fault-integration-final.log`；临时数据库 `/tmp/oj-postgres.n0TJG8` 已停止。环境为 macOS arm64、Go 1.26.10、既有 PostgreSQL 14.19 与专用 Linux arm64 VM/GCC 15.3.0/go-judge 1.13.3。
- 观察选手 PID 后禁用或轮换节点；材料用例破坏临时目录的最后一个答案文件。故障 Worker 退出码 1、整容器停止、已观察 PID 消失，没有结果载荷/verdict，也不领取下一题。修复临时材料并显式重启/复验沙箱后，实际控制端扫描回收任务；原题第 2 次尝试 AC，下一题也 AC。两个控制端拒绝旧回报，新结果字节不变。未修改租约时间或手调 Recover。
- 首次运行因新测试幂等键过短被既有验证拒绝，修正夹具后全套重跑通过；未放宽应用规则。首次失败日志 `/private/tmp/oj-fault-integration.log` 保留。`sh -n scripts/test-postgres.sh`、标签测试编译、仓库策略 160 文件与 diff 检查通过。没有修改应用代码，未额外重跑独立隔离套件或本地前端；普通远端 CI 按 Makefile 的共享范围执行全量套件。
- 两实验容器回读均为 `Running=false/Pid=0/Restarting=false`，本轮只使用 primary；VM 关闭且原 Docker context 保留。原克隆和前置任务工作区保持原状。文档同步 README、M0、开发/交付计划和 CI 分层；完整差异自审，无独立人工批准。

边界：5 秒续租周期内的短任务撤销可能在回报时拒绝，本项不承诺即时杀进程。材料不执行半套数据的直接计数由已有单元测试覆盖，本项不把未观察到 PID 当作从未执行的证据。沙箱/Worker 重启由测试驱动显式完成，不是执行端自动恢复；旧回报使用新结果作授权探针。会话、持久结果队列、SIGKILL/网络分区/跨主机故障与完整 M0 未通过。回退仅移除专项测试入口/用例，无迁移或部署。

知识事务 `oj-worker-fault-20261028` 已更新实测、推进、热点、索引和日志五页；严格 lint 32 页/394 链接/0 问题，五页哈希回读一致。保存测试源码哈希与证据边界，不修改 raw 或 accepted 账本。

提交前后完整自审补强：把后续题提前到 Worker 启动前入队，消除“题目来得晚所以未领取”的时序空隙。同一 `make worker-fault-test` 全套重跑通过，四故障 90.90 秒（10.34/10.18/34.63/35.55 秒）；日志 `/private/tmp/oj-fault-queued-regression.log`，临时数据库 `/tmp/oj-postgres.pUySCd` 已停止。首提交 `bb97e1f` 的 [PR #18 CI](https://github.com/DL-Association-for-Computing-Machinery/OnlineJudge/actions/runs/40711372352) 全部成功；补强提交仍须按最新 head 重新核验，不沿用此结果。

## 2026-10-28：Docker 构建与本机一键启动

用户确认后续默认使用 Docker 构建并一键启动，授权独立工作树与 `codex/docker-compose` 分支。最初基于 `c3e3a48`，交付时整合已合并 PR #18 的 `f13f4dc`，工作区 `~/.codex/worktrees/docker-compose/OnlineJudge`。下列为初次提交前的本地验证快照；原克隆五处既有改动保持原样，后续 PR/CI/合并状态以实际回读为准。

- 唯一目标：将当前前端与基础 API 的构建、启动、快速应用检查放入 Docker。新增固定基础镜像摘要的多阶段 Dockerfile、限定上下文的 `.dockerignore`、Compose 和 Nginx；默认仅本机 8080 入口，内部服务名代理，非 root、只读根文件系统及健康等待。检查服务使用独立 profile，默认启动不会运行测试容器。
- 在 macOS arm64 / Docker Engine 29.8.1 Linux arm64 / Compose 5.5.1 实测：`docker compose -p oj-docker-validation build` 成功；`docker compose -p oj-docker-validation run --rm --build --no-deps backend-check` 通过格式、vet、race 全部单元测试和全部 Go 命令编译；前端检查镜像构建后 `docker compose -p oj-docker-validation run --rm --no-deps frontend-check` 通过 lint、类型检查、6 项测试及 Vite 构建。Go 1.26.10、Node 24.19.0、pnpm 11.25.0 与现有配置一致。
- `python3 scripts/test-compose.py` 通过实际镜像构建、随机 loopback 端口启动、HTML/静态资源/SPA 回退、缺失资源 404、API 健康与 404/405 契约、非 root/只读/无特权/无 API 宿主端口，以及 API 重建、停止网关错误和重启恢复。唯一测试项目在 finally 中清理容器与网络，没有 volume 或业务数据。未删除镜像和构建缓存。
- 首轮后端容器缺少 `deploy/experiments/go-judge.Dockerfile`，原 `TestPinnedBuildRecipe` 正确失败；补齐构建上下文和 COPY 后完整重跑通过。首轮停止测试假定必为 502，诊断实测为连接超时 504；测试改为明确接受拒绝连接/超时两种网关失败，仍拒绝健康成功；未改代理掩盖错误。
- 新增 `containers` CI job，复用现有影响矩阵触发实际构建与冒烟；保持四个必需门禁和远端保护不变。新增 job 尚未在 GitHub 执行，不宣称远端通过。README、根规则与 CI 文档同步；仓库策略 164 文件、37 项治理回归及 `git diff --check` 通过，完整差异与新增文件自审，无独立人工审查。

日志为本机临时证据：`/private/tmp/oj-docker-build.log`、`/private/tmp/oj-docker-backend-check-final.log`、`/private/tmp/oj-docker-frontend-check.log`、`/private/tmp/oj-compose-smoke-final.log`，可能被系统清理。默认 Compose 不含 PostgreSQL、Worker、控制端或沙箱；专用 M0 适配器仍依赖现有宿主环境，数据库与沙箱专项本轮未重跑。只有 Linux arm64 本地实测，amd64/远端 CI 待验证；无 HTTPS、正式发布、离线制品或完整 M0 验收。回退仅需停止当前无状态 Compose 项目并恢复宿主机兼容入口，不涉及数据库迁移。

交付准备：用户授权整理原子提交并创建远端 PR。整合 `f13f4dc` 时仅有 CI/交付文档追加位置冲突，保留双方记录，无应用代码冲突；重新运行 Compose 全流程冒烟通过（`/private/tmp/oj-compose-pr-smoke.log`），165 文件仓库策略、37 项治理回归及完整目标分支差异检查通过。一个提交包含本目标的 10 个必要文件，无前置 PR 的重复改动；远端检查与合并结果以 PR 的当前 head 及知识库交付回读记录为准。

[PR #19](https://github.com/DL-Association-for-Computing-Machinery/OnlineJudge/pull/19) 首提交 `14c691d` 的 [CI 40714398607](https://github.com/DL-Association-for-Computing-Machinery/OnlineJudge/actions/runs/40714398607) 实际通过后端、前端与 PostgreSQL 集成套件，但新增 `containers` 失败，因此不能合并。日志显示 CI Compose 在恢复依赖后重建 web，随机宿主端口重新分配，测试仍访问旧 URL 而连接拒绝，容器健康检查本身已通过。本地先在恢复步骤强制重建复现同一失败（`/private/tmp/oj-compose-port-regression-before.log`），再改为重建后回读实际端口；保留强制重建作为回归场景和所有原断言。修复后完整冒烟通过（`/private/tmp/oj-compose-port-regression-after.log`），临时项目已清理，仓库策略及 diff 检查通过。修复以独立原子提交追加，不改已推送历史、不降低 CI 要求；最新 head 的最终结果另行核验。

## 2026-10-28：双固定槽 Worker 真实执行与退出接管

基于主线 `f1ece3f8d6e376045365024fbea73aedad018aef`，独立分支 `codex/dual-worker-recovery`，工作区 `~/.codex/worktrees/dual-worker-recovery/OnlineJudge`。唯一目标是双 Worker 的隔离运行与可清理退出后的真实任务接管。下列为提交前本地验证快照；后续按开发交付持续授权提交、推送、创建 PR 并核验对应 head 的 CI/审查，远端终态以实际回读为准。原克隆 5 处文档改动保持原状。

- `OJ_WORKER_M0_SLOT` 严格选择两个固定容器和对应锁，空值/primary 保持原容器及 `slot.lock`，secondary 使用独立容器/锁。初始名字解析后仍绑定完整 ID/启动时间，执行核心和控制端协议不变。无依赖、SQL、迁移、hooks 或 workflow 变更。
- 基线测试最初受沙箱 loopback 监听限制；按工具权限运行后通过。`GOPROXY=off make go-check go-build GOMODCACHE=${OJ_PRIMARY_CHECKOUT}/.local/go-mod GOCACHE=/private/tmp/oj-lease-go-build` 通过格式、vet、race 与四个二进制构建，日志 `/private/tmp/oj-dual-go.log`。
- `OJ_SANDBOX_EXPERIMENT=1 GOPROXY=off make dual-worker-test`（同上缓存参数）通过真实临时 PostgreSQL、双 API 24 并发/回执和双控制端回归；两个独立 Worker 在固定离线容器完成八份 AC，运行中 SIGTERM 后核验容器停止和 PID 消失，实际控制端等自然租约到期重新排队，另一节点第 2 次尝试完成 AC。旧令牌回报拒绝且新证据不变，存活节点继续完成新题。未修改租约时间或手动调用 Recover。首次日志 `/private/tmp/oj-dual-integration.log`；加强同时持有两份有效租约的断言后，最终日志 `/private/tmp/oj-dual-integration-final.log`。
- `OJ_SANDBOX_EXPERIMENT=1 GOPROXY=off make report-test`（同上缓存参数）通过原默认槽连续两题/丢失回执重报、AC/WA/CE/RE/TLE/MLE/OLE/失租回报，以及真实命令 AC/WA、重复启动拒绝、SIGINT/SIGTERM 停止确认；日志 `/private/tmp/oj-dual-report.log`。未重跑本地前端和完整独立隔离套件。
- 文档同步 README、M0、开发/交付计划、协议架构和 CI 分层；仓库策略检查 159 文件及 `git diff --check` 通过。自审覆盖从基线到工作区的完整差异与 4 个新增文件，无独立人工审查。知识回写保存可定位证据和范围，结果以事务 lint/哈希回读为准。

边界：同一 VM 内双容器，不代表跨主机高可用或容量验收；SIGTERM 可清理退出不等价于 SIGKILL、主机故障或网络分区。旧回报探针使用新尝试结果配旧令牌，不冒充中断节点已冻结结果。普通重试增加尝试编号，判题/灾备恢复代次不变。下一项执行中撤销/材料故障联合验证，然后会话；执行端自动重启/解封、磁盘结果队列与完整 M0 尚未完成，无部署。

最终双 Worker 用例 42.92 秒通过，临时数据库 `/tmp/oj-postgres.itdI7m` 已停止。两容器回读 `Running=false/Pid=0/Restarting=false`，`oj-m0` VM 已停止，原 Docker context 仍为 `desktop-linux`；未删除镜像、容器、锁文件或数据。

知识事务 `oj-dual-worker-20261028` 已更新 M0 实测、开发推进、热点、索引和日志 5 页；严格 lint 32 页/391 链接/0 问题，5 页哈希回读一致。保留未提交源码哈希和证据边界，未修改 raw 或 accepted 账本。

## 2026-10-28：控制端任务自动恢复

基于主线 `8448641` 的独立 `codex/worker-recovery`，唯一目标为 P1/P2、AC28/29 的控制端恢复扫描。新增扫描器复用既有 `Store.Recover`：启动即扫描、单轮最多 64 条/5 秒、完成后等待 1 秒、失败限速重试、取消排空后关闭池。仅记录安全计数，数据库级扫描跨材料版本；无 SQL、迁移、依赖、HTTP 契约或 CI 配置变化。原克隆 5 处未提交文档改动保留。

- 先新增回归再实现：旧控制端的撤销/轮换、到期和三次上限用例均在 8 秒后仍为 `running`，日志 `/tmp/oj-worker-recovery-before.log`。后续依赖用例的失败来自遗留未恢复任务，不把它们当作独立缺陷。
- macOS arm64 / Go 1.26.10 / PostgreSQL 14.19：`GOPROXY=off make go-check db-test GOMODCACHE=${OJ_PRIMARY_CHECKOUT}/.local/go-mod GOCACHE=/tmp/oj-worker-recovery-go-build` 通过格式、vet、race、四个二进制构建、真实数据库和双 API/控制端回归。最终日志 `/tmp/oj-worker-recovery-final-tests.log`，一次性数据库 `/tmp/oj-postgres.xMLTRB` 已停止。
- 新增双控制端启动前 65 条过期积压、仍有效租约不回收、每题唯一重领；撤销/轮换、旧回报拒绝和三次尝试后 `failed`/无 verdict 通过。涉及自动恢复的材料/领取测试改为等待实际扫描，避免手动调用掩盖调度缺口。扫描失败重试、日志脱敏和取消排空以单元注入验证。
- README、开发计划、M0 实验与 CI 说明同步。`python3 scripts/repo_policy.py repository`（155 文件）和 `git diff --check` 通过；完整差异自审和交付状态在最终回读中确认。本条为提交前验证快照，远端 CI 不沿用旧 head。
- 合成回报只证明控制端协议/存储，不是双 Worker 沙箱接管；本次未运行特权沙箱或前端套件。下一项双 Worker 真实执行与故障接管，执行端自动重启/解封、磁盘结果队列、会话和完整 M0 仍未完成。回退控制端二进制可停止扫描，历史数据保留；无部署或独立人工批准。

## 2026-10-28：按影响范围执行 CI

远端全量路径已核验：[PR #14](https://github.com/DL-Association-for-Computing-Machinery/OnlineJudge/pull/14) 的 head `cce2f6812826b250bffe3a5e063b952c4ff22b02` 在 [CI 40700532954](https://github.com/DL-Association-for-Computing-Machinery/OnlineJudge/actions/runs/40700532954) 中识别 9 个变更路径，选择三个应用套件并全部执行成功，四个稳定必需门禁全部 success。合并前 SHA/base 一致、完整差异自审无未解决问题、远端 reviews/comments/threads 为空、GitHub CLEAN/MERGEABLE；按持续授权 squash 合并为 `1f8a95127190f850ab40f7b47d6da891591f7a86`，主线树已回读一致。无独立人工批准或部署。

本次仅补齐此交付记录的 `codex/ci-docs-validation` 分支用于真实验证纯文档路径：预期 governance 与三个结果门禁成功，三个 `*-tests` 均 skipped；不能将其写为应用测试通过。该文档分支运行结果以对应 PR/CI 和知识库最终回读为准，下列为实现提交前的验证快照。

用户要求避免小改动无条件重跑所有 CI。基于已合并 PR #13（`2e11806a84020c5b5ff23f06ec85b1a9ef8496b4`）创建 `codex/ci-impact` 独立任务，唯一目标为 CI 分流和稳定必需门禁，不修改应用、依赖、远端 ruleset 或部署。

- `scripts/ci_scope.py` 使用完整 PR merge-base 差异或完整 push 差异，文档/治理、前端、后端/数据库按明确路径选择，混合取并集，未知路径/缺失历史/人工触发全量。重命名按删除与新增处理，不受 API 文件列表分页截断影响。
- `governance` 始终检查仓库、标题、治理回归并输出范围；三个应用 jobs 保留原测试。原 `backend`/`frontend`/`integration` 名称成为 `always()` 结果门禁，仅接受“明确需要且成功”或“明确不需要且跳过”；依赖失败、取消、缺输出均阻断。文档不安装 Go/Node 依赖或启动 PostgreSQL。
- 本地原 25 项治理基线通过；新增后共 37 项测试通过，涵盖真实 Git 多提交/分叉/删除/重命名、异常路径、push、缺失历史、状态组合、CLI 退出码和 workflow 接线。`actionlint`、仓库策略和 `git diff --check` 通过。自审覆盖相对主线完整差异，无独立人工批准。
- README、CI/CD、开发流程、GitHub 治理说明同步；本条为提交前记录，远端当前 head 全量 CI 与合并结果另行回读。CI 自身变化必须全量验证；后续纯文档交付记录可验证跳过路径。本机不重复运行未修改的应用/特权沙箱测试。
- 保留 PR `edited` 事件对完整差异的重验（代码 PR 修改描述仍会跑其受影响套件），不复用历史成功状态，不裁剪套件内测试。回退可通过 PR 恢复原无条件应用 jobs，四个远端检查名称不变；无需数据库迁移。

## 2026-10-28：CI 与 review 通过后的合并授权

用户明确要求“下次ci没问题，review pr没问题你就自己合并，写进根规则里”。本任务唯一目标为记录后续 PR 的条件合并持续授权，替代旧的禁止 Codex 合并条款。同步根规则、开发工作流、PR 模板、README 与 GitHub 治理说明；不修改应用、hooks、CI 或远端保护配置。

门禁要求最新 head/目标分支复核、CI 全部通过、完整差异审查、既有反馈无未解决问题及 GitHub 可合并；使用匹配 head 的 squash 合并，禁管理员绕过与提前启用平台自动合并。缺少独立人工审查如实说明，发布仍需独立授权。此次规则 PR 同样先完成审查与 CI，再按新授权合并。基线为已合并 PR #12 的 `d560e93fc86f5eb0eedc0df7d2d65b49a7b95c33`；原工作区保持不变。

本地验证：`python3 scripts/repo_policy.py repository`（150 文件）、`python3 -B -m unittest discover -s tests/governance -v`（25 项）及 `git diff --check` 全部通过。完整 6 文件差异已自审，未发现未解决问题；无独立人工批准，不把规则文字当作自动门禁。纯规则/文档变更，未重跑本地应用/沙箱；远端当前 head 的 CI、PR 审查反馈和最终合并结果在交付时回读。

## 2026-10-28：单槽 Worker 命令入口与独占运行

- 唯一目标：P2、AC28/29 的 `cmd/worker` 接线与进程所有权。在已合并 PR #11 的 `786611519e8c7d3347837b1a269f5aeb6eb3e43b` 上建立独立 `codex/worker-entry`；原克隆 5 处未提交改动保留。实现阶段为未提交工作区，最终提交/PR/CI 以交付回读为准。
- 配置与模块边界：显式 loopback HTTPS、节点证书/私钥、实验开关；入口装配既有客户端/生命周期/执行/宿主。固定同账号所有权文件锁及持久隔离标记，领取前宿主/协议预检，SIGINT/SIGTERM 取消排空并独立确认整容器停止。停止结果复用，失败不清隔离。无新增依赖、SQL、迁移或协议变化。
- 基线：Worker/execution/workerhost 测试通过；workerclient 首次受沙箱本机 bind 权限限制，获准后原测试全部通过，未修改断言。
- macOS arm64 / Go 1.26.10：`GOPROXY=off make GOMODCACHE=${OJ_PRIMARY_CHECKOUT}/.local/go-mod GOCACHE=${OJ_PRIMARY_CHECKOUT}/.local/go-build go-check go-build` 通过。新增配置、预检/停止错误、真实子进程锁与 SIGKILL 隔离测试通过。
- `OJ_SANDBOX_EXPERIMENT=1 GOPROXY=off make GOMODCACHE=${OJ_PRIMARY_CHECKOUT}/.local/go-mod GOCACHE=${OJ_PRIMARY_CHECKOUT}/.local/go-build report-test` 通过，临时数据库自动停止（`/tmp/oj-postgres.7jcI31`）。现有数据库/双 API/双 mTLS 控制端回归、单槽生命周期与 8 项回报通过；新增真实 Worker 子进程 AC/WA 落库、重复启动锁拒绝、空闲 SIGINT、在途选手 SIGTERM、容器/PID 消失及无中断结果发布通过。原始输出保存在任务工作区忽略的 `.local/worker-entry-report.log`。
- 新命令联合测试的控制端 handlers 在测试进程内；数据库回归另有真实双控制端子进程。本轮未重跑前端和完整 12 项隔离测试；不宣称双 Worker、自动恢复、磁盘回报队列、平滑下线、会话、生产准入或完整 M0 通过。无合并、部署或独立人工审查。
- README、M0 记录、开发计划、API/Worker 架构和 CI 验证边界同步。回退无迁移，须先核验旧 Worker/容器终止及隔离标记；禁止仅删除锁文件解除未知状态。
- 收尾：仓库策略检查通过（150 文件），`git diff --check` 通过；专用容器/VM 已停止，入口隔离文件回读为空，原克隆保持原状。知识库与最终远端交付结果另行回读。

## 2026-10-28：可信 M0 宿主适配器

唯一目标：P2、AC28/29 的可信宿主检查、容器内传输及停止确认前置功能。PR #10 已通过 gh 核验合并为 `9c926f3cb08be2df3f3382f20639c39be5a6dc9a`，从该主线建立 `codex/worker-host` 独立工作区，原克隆 5 处改动保留。此条为提交前验证快照，远端交付以最终 head 核验为准。非目标：cmd/worker 接线、跨进程锁、双 Worker、恢复、会话、生产部署。

- 新增 `internal/workerhost`，宿主细节留在适配层，执行与生命周期核心不变；固定专用 VM/容器、不可变 ID/启动时间核验、有限 REST 子集、取消排空、停止/PID 确认和永久封锁。工程选择与代价见[M0 宿主适配器](m0-experiment.md#可信宿主适配器)。无新增依赖、SQL、迁移、公共协议或环境变量。
- macOS arm64 / Go 1.26.10：直接使用已缓存工具链，`GOPROXY=off`、原克隆 `.local/go-mod` 和 `/tmp/oj-worker-host-go-build`；`go vet ./...`、`go test -race ./...`、`go build ./cmd/...` 通过。初次 TLS 基线被沙箱禁止临时监听端口，经审批后通过；未放宽断言或下载依赖。
- 既有无挂载专用 oj-m0 Linux VM/固定镜像：`OJ_SANDBOX_EXPERIMENT=1 go test -count=1 -race -tags=sandbox ./tests/sandbox -run 'TestBoundedExecution|TestWorkerHostExecution|TestLeasedExecutionConfirmsHostStop|^TestSandbox$' -v` 通过（沿用上述工具链/缓存）。新宿主真实 AC/WA、制品清理与运行中取消停止通过，原 10 项编排、12 项隔离回归通过；停止专项独立观察选手 PID 消失、后续点 NOT_RUN、旧实例拒绝。
- 本地未重跑 PostgreSQL/report-test 和前端；它们未改动，不能将历史结果记作本次通过。普通 CI 不执行特权沙箱；没有修改 hooks/CI。cmd/worker 仍为骨架，M0 未通过，无独立人工审查、合并或部署。

同步 README、M0、首期计划、协议架构与 CI 说明。仓库策略 143 文件与 diff 检查通过；AI 自审覆盖相对主线完整差异及新增文件。知识事务 `oj-worker-host-20261028` 更新实测、推进、热点、索引、日志 5 页，严格 lint 32 页、381 链接、0 问题，目标哈希回读一致。实验容器和 VM 已停止并保留，无部署。

## 2026-10-28：单槽 Worker 生命周期

唯一目标：P2、AC28/29 的串行领取、材料准备、租约执行与可靠回报编排。gh 核验 PR #9 已合并为 `51d054a6d63d5e5501af3c465b65120ab74684cb`，独立 `codex/worker-lifecycle` 从该主线开始，原克隆 5 处文档改动保留。此条为提交前实测快照，提交/PR/CI 以最终 head 核验为准。非目标为命令入口、宿主驱动、双 Worker、磁盘队列、自动恢复、会话与部署。

- `internal/worker` 通过最小接口串联已有模块；空队列间隔 1 秒，未知回报最多 3 次且原样重放，异常停止，取消等待执行排空，实例单次使用。选择失败停止以限制本阶段恢复复杂度，代价为暂不自动恢复可用性、无崩溃持久性或平滑下线。无依赖/SQL/迁移/公共 HTTP 或配置变化。
- macOS arm64、Go 1.26.10、PostgreSQL 14.19：`GOPROXY=off make go-check db-test GOMODCACHE=${OJ_PRIMARY_CHECKOUT}/.local/go-mod GOCACHE=/private/tmp/oj-lifecycle-go-build` 通过格式、vet、race、四命令构建、临时数据库/双控制端、双 API 的 24 并发与重启回执。最初基线测试受沙箱 bind 限制，经工具审批重跑通过，未放宽断言。临时数据库 `/tmp/oj-postgres.8OyFu8` 已停止。
- `GOPROXY=off make report-test GOMODCACHE=${OJ_PRIMARY_CHECKOUT}/.local/go-mod GOCACHE=/private/tmp/oj-lifecycle-go-build OJ_SANDBOX_EXPERIMENT=1` 通过新增生命周期联调及原 8 项回报用例。一个 Worker 连续完成真实 AC/WA，丢失确认跨控制端原样重报，5 次沙箱调用证明无重复执行；回读两题持久结果。日志 `/private/tmp/oj-lifecycle-report.log`，临时库 `/tmp/oj-postgres.OkZ3XM` 已停止。
- 联调仍使用同进程 TLS handler 和测试宿主驱动；取消排空/停止错误通过单元注入，不冒充真实进程信号、数据库撤销到沙箱中止或双 Worker 验收。未修改/重跑本地前端、独立沙箱隔离/中止专项，未改 hooks/CI；M0 未通过，无独立人工审查或部署。
- 同步 README、首期计划、M0、协议架构和 CI 说明，下一项为可信宿主适配器与命令入口。恢复沿用已有离线专用 VM/镜像，未下载/重建，实验完成后停止容器/VM，原 Docker context 保持 desktop-linux。

收尾：仓库策略 137 文件、`git diff --check` 通过，AI 自审覆盖相对主线完整差异和新增文件。知识事务 `oj-worker-lifecycle-20261028` 更新实测、推进、热点、索引与日志 5 页，严格 lint 32 页、379 链接、0 问题，目标哈希回读一致；不新增 accepted 断言或覆盖 raw。

## 2026-10-28：Worker 领取与材料客户端

唯一目标：P2、AC22/28/29 的受控领取与材料准备。gh 核验 PR #8 已合并为 `64b99922de3ab0e01509d6b3a42cd248687424c9`，从该主线创建独立 `codex/claim-material-client` 工作区；原克隆 5 处未提交文档改动保留。本条为提交前验证快照，提交号、PR 和当前 head 的 CI 以实际交付结果为准。非目标为常驻 Worker、自动恢复/轮询、磁盘缓存、通用题包、双 Worker、会话与部署。

- 新增 `workerclient.Acquirer`，一次领取、回执未知不重试；限定固定 v2 的下载、完整性与预算校验，失败不返回部分材料。独立连接预算，调用方继续使用原租约和 `LeasedRunner` 前置续租；无依赖、SQL、迁移、HTTP 契约或配置变更。
- macOS arm64 / Go 1.26.10 / PostgreSQL 14.19，`GOPROXY=off make go-check report-test GOMODCACHE=${OJ_PRIMARY_CHECKOUT}/.local/go-mod GOCACHE=/private/tmp/oj-claim-go-build OJ_SANDBOX_EXPERIMENT=1` 全部通过：格式、vet、race、四命令构建、真实数据库/双控制端、双 API 24 并发及重启回执、8 项沙箱联合用例。日志 `/private/tmp/oj-claim-report.log`；临时数据库 `/tmp/oj-postgres.RUWQV5` 已自动停止。
- 19 项领取与 24 项材料边界子测试、真实双节点竞争领取和跨控制端材料下载通过；覆盖回执不明、错误身份/归属/哈希/长度/媒体类型、取消、缺失/损坏、撤销、过期与恢复后旧尝试拒绝。最初本地 TLS 基线因沙箱禁止监听失败，经审批后通过；首次真实集成暴露本次 ID 校验错误，修正 UUID 假设为持久化的 32 位十六进制串后重跑完整检查通过，未降低断言。
- `report-test` 已从直接领取/材料夹具切换到客户端，再接已有受限执行、续租及回报；真实 AC/WA/CE/RE/TLE/MLE/OLE 持久化，丢失确认跨控制端原样重报、冲突、跨节点、撤销、到期拒绝仍通过。使用既有专用离线 Linux VM/固定容器，无镜像下载或重建；实验容器/VM 收尾停止，Docker context 保持 desktop-linux。
- 两个 TLS handler 在同一测试进程；提交与沙箱生命周期仍由测试驱动管理。独立双控制端由数据库回归另外验证，不能宣称常驻双 Worker 或完整 M0 通过。未改/未重跑本地前端、未修改 hooks/CI、未重跑独立沙箱隔离/中止专项；没有独立人工审查或部署。
- README、M0、协议架构、CI 说明及首期计划已同步，下一步为 Worker 入口及生命周期。仓库策略 134 文件、`git diff --check` 通过；AI 自审覆盖相对主线完整差异和新增文件。知识回流按既有授权记录到 M0 实测、开发推进、热点、索引与日志，实际事务及终态核验随最终交付汇报。

## 2026-10-27：真实执行结果摘要与 mTLS 回报

唯一目标：P2、AC28/29 的真实结果归属与可靠确认子项。gh 回读 PR #7 已合并为 `b6b0e428380ae18978b9bb6312c475c7659e7120`，从该主线建立独立 `codex/result-reporting` 工作区，保留原克隆 5 处文档改动。此条为提交前验证快照，提交/PR/CI 以最终实际 head 为准。非目标为领取/材料客户端、常驻 Worker、自动恢复/重试、双 Worker、会话及部署。

- `LeasedRunner.JudgeReport` 在租约守护、执行及清理成功后生成绑定提交/尝试/代次、材料、结果及用量的摘要值；失败不产生可发送结果。`workerclient.Reporter` 使用有界 mTLS、禁止重定向、明确未知/失租/撤销/冲突；未知确认只能原样重报，不能重新执行改变结果。无新增依赖、SQL、迁移或公共 HTTP 契约。
- `GOPROXY=off make go-check db-test` 通过：格式、vet、race、四个命令构建、临时 PostgreSQL 14.19、双控制端及双 API 的 24 并发/重启回执。macOS arm64 / Go 1.26.10，复用原克隆 `.local/go-mod` 与 `.local/go-build` 缓存，无依赖下载；临时库 `/tmp/oj-postgres.ZyuPYK` 已停止。最初 TLS 基线被沙箱禁止监听端口，经工具审批后通过，未改变测试要求。
- `OJ_SANDBOX_EXPERIMENT=1 GOPROXY=off make report-test` 通过，Make 命令行指定上述缓存；日志为当前工作区忽略的 `.local/report-test.log`，临时库 `/tmp/oj-postgres.cO0Pw1` 已停止。7 种真实选手判定持久化，提交后丢失响应、跨控制端原样重报、摘要冲突、跨节点、撤销及到期拒绝均通过，共 8 项。使用现有 oj-m0 VM 与固定镜像，无下载/重建，选手代码仅在离线无挂载容器中执行。
- 联合测试的两个 TLS handler 在同一测试进程，领取/材料准备为直接夹具；独立控制端进程另由数据库回归验证。无详细证据归档、待回报磁盘队列、崩溃恢复或生产停止驱动；不能称为完整 Worker 或 M0 通过。README、M0、协议设计、CI 说明及首期计划已同步，下一项为领取/材料客户端。

收尾复验：原 10 项编排、12 项隔离及真实中止确认均通过 race 沙箱回归，日志 `.local/sandbox-regression.log`；回报联合测试在独立 `report-test` 中实际运行，普通沙箱回归中的显式 skip 不算它的通过证据。仓库策略 130 文件、`git diff --check` 通过；未修改前端或 hooks/CI，未重跑本地前端。专用容器/VM 已停止并保留，Docker context 仍为 desktop-linux。AI 自审覆盖完整任务差异及新增文件，没有独立人工审查或部署。

知识回流事务 `oj-result-reporting-20261027` 已完成，更新实测索引、开发推进、热点、索引与日志 5 页；严格 lint 32 页、375 链接、0 问题，5 页哈希回读一致。语义复核保留真实执行、直接夹具、同进程 TLS handler 与独立控制端测试的区别，不新增 accepted 断言或覆盖 raw。

## 2026-10-27：租约守护与独占沙箱中止确认

唯一目标：P2、AC28/29 的执行生命周期子项。GitHub CLI 回读 PR #6 已合并，主线 `b50f6a0`；从该主线建立独立 `codex/lease-execution` 工作区，原克隆 5 处文档改动保留。本记录为提交前验证快照，提交/PR/CI 以实际交付结果为准。非目标为领取/材料下载、真实结果回报、自动恢复、双 Worker、会话与部署。

- 新增 mTLS 续租客户端与租约守护，执行前续租、周期续租、保守单调时限、失败取消、独立停止确认、旧槽永久停用；无依赖、SQL、迁移、HTTP 契约或配置变化。职责和限制见[M0 记录](m0-experiment.md#租约守护与中止确认)。
- `GOPROXY=off make go-check db-test GOMODCACHE=${OJ_PRIMARY_CHECKOUT}/.local/go-mod GOCACHE=/private/tmp/oj-lease-go-build` 通过：格式、vet、Go race、四个命令构建、临时 PostgreSQL 14.19、双 mTLS 控制端与双 API 的 24 并发/重启回执回归。macOS arm64、Go 1.26.10；临时库 `/tmp/oj-postgres.k68fim` 已自动停止。最初默认沙箱检查因本机端口权限失败，经工具审批后正常验证，未降低测试。
- `OJ_SANDBOX_EXPERIMENT=1 GOPROXY=off GOMODCACHE=${OJ_PRIMARY_CHECKOUT}/.local/go-mod GOCACHE=/private/tmp/oj-lease-go-build go test -race -count=1 -tags=sandbox ./tests/sandbox -v` 通过新增真实中止确认及原 10 项编排、12 项隔离测试，日志 `/private/tmp/oj-lease-sandbox.log`。首次失败来自无宿主挂载时 Colima SSH 默认 cwd 不存在；固定测试探针目录 `/tmp` 后所有断言通过。实验容器/VM 已停止，无镜像下载或删除。
- README、M0、协议架构、首期计划和交付记录同步。未改前端、hooks 或 CI，未重跑本地前端；无独立人工审查。完整数据库撤销→真实沙箱联合实验、生产停止适配器和完整 Worker/M0 仍未完成。

收尾：仓库策略检查 125 文件、diff 检查通过；知识事务 `oj-lease-execution-20261027` 已更新 M0 实测、推进、热点、索引与日志 5 页，严格 lint 32 页、373 链接、0 问题，目标哈希回读一致。保留可恢复草稿 `/private/tmp/oj-lease-vault.json`；未新增 accepted 断言、未改 raw，无已绑定外部同步目标。远端交付状态以本任务 PR 的当前 head 检查为准。

## 2026-10-27：固定 GNU++17 材料的受限执行编排

唯一目标：P2、AC05/06/17/20 的固定 v2 受限编排。GitHub CLI 已核验 PR #4 合并为 `56b43361db91a98c1353c9342141e255f7ecd56f`，从更新主线创建独立工作区及 `codex/bounded-execution` 分支；原克隆 5 个文档改动保持。以下为本地验证时的未提交状态，后续提交及 CI 以对应 PR 的精确 head 为准。

- 新增 `internal/execution`：单槽 Runner、材料与环境预检、一次编译/按序逐点执行、首次失败停止、结果归因、编译独立/运行峰值统计、制品清理。go-judge REST 适配与编排分离；不新增依赖、SQL、公共 API 或后台任务服务。
- 基线相关实验/文本/材料单测通过。最终 `GOPROXY=off make go-check go-build` 通过格式/vet/race/四命令构建，Make 参数复用原克隆 `.local/go-mod`、`.local/go-build`；macOS arm64、Go 1.26.10。未改数据库/前端，本轮未重跑本地数据库/前端检查；未修改 hooks/CI。
- `OJ_SANDBOX_EXPERIMENT=1 GOPROXY=off make sandbox-test` 通过新增 10 项真实编排子测试与既有 12 项隔离回归；固定 Linux arm64、GCC 15.3.0、go-judge v1.13.3。逐例核验编译次数、首/末点停止和缓存清空，覆盖正常/错误/资源超限、非法 UTF-8 和临时文件隔离。
- 实测首先发现 OLE 响应保留 4097 字节并附 `CollectSizeExceeded`；新增失败回归，核对固定版本源码后修复适配，再通过真实测试。仅接受结构化输出超限证据，文件/未知错误不伪装为选手错误。
- 注入验证材料损坏、环境不匹配、协议缺字段/矛盾、响应超限、取消、容量与清理失败。远端状态不明时 Runner 停用，HTTP 取消不视为 kill 确认。测试驱动使用已有专用无网络/无挂载 VM，尚无生产执行传输或宿主检查器。
- README、M0 记录、首期计划同步。完整 Worker、租约中止确认、真实回报和 M0 整体验收仍未完成；无部署或独立人工审查。后续先做租约中止，再做真实回报。

收尾自审：仓库策略 120 文件及 `git diff --check` 通过，相对 `origin/main` 的改动仅包含本功能及必要测试/文档。专用容器与 VM 已停止，`colima list` 回读 Stopped，Docker context 仍为 desktop-linux。知识回流和远端 CI 另按实际结果记录，不用本地成功代替远端状态。

集成期间主线合并 PR #5 为 `5834ac2`，导致交付日志同位置插入冲突；正常 merge 最新主线，保留两项独立记录，不重写已推送历史。应用实现和测试字节保持原功能提交 `65ad362`，远端 CI 须按合入主线后的新 head 验证。

## 2026-10-27：固定开发交付流程与持续授权

用户要求每次开发创建对应分支、整理原子提交、推送并创建 PR，一个 PR 对应一个功能或阶段，GitHub 操作使用 CLI。本次唯一目标是将该授权固化为仓库规则；从 `e69129d` 建立独立 `codex/delivery-workflow` 分支，不把规则改动追加到尚未合并的执行环境 PR #4，原克隆已有 5 处文档改动保留。

- 根 `AGENTS.md` 第 7 节明确持续授权与完整步骤，覆盖任务分支、恢复复用、原子提交、完整 diff、PR 范围、`gh` 操作和当前 SHA 的 CI 跟踪；替代此前默认不提交/不推送/不建 PR 的条款。单一阶段必须预先界定范围与验收，不能无限聚合其他任务。当前只读/不提交要求优先，不授权合并、自动合并、部署、强推或远端保护/秘密修改。
- 同步开发工作流、PR 模板及 README，清理长期沿用初始化分支与旧授权描述。保留 Git hooks 的机械检查职责，没有加入 hook 隐式联网推送或创建 PR，没有声称 hook 能判断功能语义。
- 实际本地验证：`python3 scripts/repo_policy.py repository`（110 文件）、`python3 -B -m unittest discover -s tests/governance -v`（25 项）和 `git diff --check` 通过。本次只改规则与文档，未修改应用、hooks 或 CI；未重跑本地应用/沙箱测试。PR CI 以该分支实际 head 为准，不能沿用 PR #4 结果。

按本次持续授权继续完成正常 hooks 提交、分支推送、CLI 创建 PR 与 CI 核验；最终提交号和检查状态以 Git 历史、对应 PR 及知识库交付记录为准。不自动合并或发布。

## 2026-10-27：固定 GNU++17 实验环境与材料 v2

唯一目标：P1/P2 的固定执行环境及新材料版本子项。已通过 GitHub 回读 PR #3 合并提交 `e69129d65334e5a7220cbf141b5cf33f5b8a6dc6` 并 fetch，从该主线新建独立工作区/分支 `codex/execution-environment`。以下记录本地验证阶段，当时尚未提交；随后用户授权提交、推送、创建 PR 并检查 CI，实际提交和远端结果以 Git 历史及对应 PR 检查为准。未授权合并或部署。原克隆 5 处文档改动保留。

- 复用并核验已有专用 VM 中的镜像 config ID、Linux arm64/内核、GCC/go-judge 版本、启动参数与资源预算。环境描述归 `internal/experiment`，`m0-sum-v2` 的规范清单包含完整描述，旧 v1 字节/hash 不变。API、控制端和夹具 CLI 共用显式版本选择，默认 v1；无依赖、SQL、索引或迁移变化。版本并存与回退规则见[M0 记录](m0-experiment.md#固定执行环境与新材料版本)。
- 新增真实数据库/mTLS 场景，验证 v2 显式注册、旧控制端不领取 v2、清单/源码精确字节摘要、材料下载、版本冲突和环境篡改拒绝。双 API 测试增加 v2 接收/幂等与 v1 历史回执；原 24 并发与重启恢复保留。
- 实际验证（macOS arm64、Go 1.26.10、PostgreSQL 14.19）：修改前 `GOPROXY=off make go-check db-test` 通过；默认沙箱首次因 httptest 本机监听权限失败，经工具审批后运行。新增集成首次捕获嵌套结构体 JSON 顺序与 JSONB 规范字节不一致，统一规范化后通过。最终相同命令通过格式/vet/race/四命令构建、真实数据库和进程回归；通过 Make 参数复用原克隆 `.local/go-mod`/`.local/go-build` 缓存，禁用下载。最终日志 `/tmp/oj-environment-final.log`；临时数据库 `/tmp/oj-postgres.gdEM5M` 自动停止。
- `OJ_SANDBOX_EXPERIMENT=1 GOPROXY=off make sandbox-test` 通过：执行前核验固定环境及运行隔离，12 项 GNU++17/文本结果、超限、文件/网络隔离和缓存清理全部通过。最终日志 `/tmp/oj-environment-sandbox-final.log`。本次未改/未重跑前端、未修改治理 hooks/CI，尚无本分支远端 CI 或独立人工审查。
- 同步 README、两份实验契约、M0、协议设计与首期计划。完整 Worker、租约中止、真实回报、跨节点分发、完整镜像许可证/CVE 和 M0 综合验收仍未完成；本机 image ID 不是可 pull 的 registry digest。下一项独立目标为受限执行编排。

收尾核验：仓库策略检查 114 文件及 `git diff --check` 通过，3 份 API YAML 解析通过（未运行完整 OpenAPI 校验器）；专用容器已停止，`colima list` 回读 oj-m0 为 Stopped，原 Docker context 仍是 desktop-linux。知识事务 `oj-execution-environment-20261027` 已 inspect/apply，更新实测索引、推进、热点、索引与日志 5 页，回读哈希一致；严格 lint 32 页、369 链接、0 问题，另行核对协议/独立沙箱与完整闭环的语义边界。可恢复草稿 `.local/execution-environment-vault-transaction.json`；未新增 accepted 账本断言、未改 raw，无已绑定外部飞书同步目标。

## 2026-10-27：有效租约限定的材料下载与哈希校验

唯一目标：完成 P1/P2、AC22/28/29 的任务范围材料读取子项。PR #2 已合并为 `4698b1f`，本次从该主线建立 `codex/worker-materials`，复用干净的托管工作区；原克隆的 5 个未提交规则文档保持原状。

- 新增 `assignment`/`material` 两个 mTLS POST 接口，按当前节点、凭据版本、任务尝试、租约和双代次读取固定源码/清单/输入/答案；不接受任意磁盘路径或哈希选择。源码与规范清单可核验 SHA-256，四个合成文件与既有 `m0-sum-v1` 哈希保持一致。
- `MaterialStore` 归提交模块；材料服务通过小型 `Reader` 接口依赖本地存储适配器，传输层不引用数据库实现。取得数据库锁后判断到期，文件读取不持锁，校验后再次授权；只发送已校验私有快照，拒绝链接/FIFO/目录/超限/损坏，1 MiB/文件、4 个活动快照/控制端。不新增依赖、迁移或索引。
- 可选 `OJ_MATERIAL_ROOT`：显式 `migrate fixture` 先初始化四个合成文件再注册数据库引用，重复校验、冲突不覆盖；控制端只读，未配置时材料接口 503。回退可取消配置并重启或退到 PR #2 二进制，保留数据，无需 Down。
- 实际验证：`GOPROXY=off make go-check db-test`（复用原克隆 Go 缓存）通过，含格式/vet/race/构建、私有 PostgreSQL 14.19、真实双 mTLS 控制端和双 API 的 24 并发/重启回执回归；新增 CLI 材料初始化重复执行通过。Go 1.26.10、macOS arm64。节点/尝试锁等待时失效、双代次、清单损坏及跨控制端读取实际验证；读取期间撤销/到期通过依赖注入测试。日志 `.local/worker-materials-check.log`；临时数据库 `/tmp/oj-postgres.LwzKch` 已停止，日志/目录可能随清理失效，优先重跑版本化测试。
- 文档同步：Worker OpenAPI 0.2、README、M0、存储/协议设计、CI 说明和首期下一步；OpenAPI YAML 解析与仓库策略/差异检查通过。未修改前端或治理配置，因此未重跑前端和治理回归；远端 CI 须按本 PR head 核验。

边界：执行端与缓存仍未接入，合成回报不是判题结果；本次未启动 Linux VM，未完成磁盘耗尽/断电一致性、生产发布回收、跨主机和容量验收；传输放行后已发送字节不能撤回。M0 未通过，无独立人工审查、合并或部署。下一项为固定执行环境及新材料版本，再推进受限编排、租约中止和真实回报。

收尾：sqlc 5 个生成文件复现一致。知识事务 `oj-worker-materials-20261027` 已 inspect/apply，5 页目标哈希回读一致；严格 lint 扫描 32 页、367 链接、0 问题，另行核对真实/模拟证据边界。恢复草稿 `.local/worker-materials-vault-transaction.json`，未新增 accepted 账本断言或覆盖 raw；无已绑定的外部飞书同步目标。

## 2026-10-27：Worker mTLS 租约入口

唯一目标：完成 P1/P2、AC28/29 的认证节点领取/续租/结果回报子项。基于已合并主线 `25ed31a`，在独立工作区的 `codex/worker-mtls` 开发；原 `codex/engineering-baseline` 工作区的 5 个规则文档改动保留，未混入此 PR。验收是证书身份绑定、逐次授权及跨实例租约约束；材料下载、真实判题、证书运维工具、恢复循环与会话是后续独立目标。

- 新增 `cmd/worker-api` 和 `internal/workerapi`：显式 loopback 配置、TLS 1.3 双向认证、唯一 URI SAN 节点身份、每次请求重验证书与原事务内节点授权，三个有界 JSON 接口。复用标准库 TLS/X.509，不新增或升级依赖；核对了官方 API 文档及本机 Go 1.26.10 源码。模块契约移至 `internal/submission`，HTTP 不依赖 PostgreSQL 类型，已有结果摘要序列化保持不变。
- 新增真实 TLS 边界测试与两个真实控制端子进程/两个节点客户端的 PostgreSQL 集成测试。证书每次临时生成，私钥不提交；合成结果只证明协议与持久化，不代表执行过选手程序。
- 同步 README、Worker OpenAPI、M0、协议设计、首期计划、需求子项证据和 CI 说明；无 SQL、迁移或流水线配置改动。退回原进程组合只需停止新增控制端，保留已领取任务并按既有恢复语义处理，不删除数据。

实际验证（macOS arm64、Go 1.26.10、PostgreSQL 14.19）：

- 修改前 `GOPROXY=off go test ./internal/...` 基线通过。新测试首轮通过；全量 vet 发现契约迁移后跨包结构体使用位置字段，改为命名字段后通过。
- `GOPROXY=off make go-check go-build`：格式、vet、全部 Go race 测试及四个命令构建通过；`make db-test`：临时 PostgreSQL、原迁移/租约回归、两个 mTLS 控制端进程竞争认领、同连接撤销/版本轮换、异节点/到期/旧尝试拒绝、重复/冲突回报通过，另有原两个提交 API 的 24 并发与重启回执检查。测试使用原克隆的 `.local/go-build` 与 `.local/go-mod` 缓存，通过 Make 变量显式指定，禁用网络下载。数据库已自动停止，诊断目录 `/tmp/oj-postgres.M9I8qM`。
- `python3 scripts/repo_policy.py repository`、25 项治理回归、`git diff --check` 通过；3 份 OpenAPI YAML 解析及本地 `$ref` 解析通过。未运行完整 OpenAPI 专用校验器。
- 本地未重跑未改动的前端，也未重启 Linux 沙箱；远端 CI 以此 PR head 独立核验。M0 整体未通过，未部署或合并，尚无独立人工审查。

收尾：知识事务 `oj-worker-mtls-20261027` inspect/apply 完成，更新 M0 实测、开发推进、热点、索引和日志共 5 页；目标哈希全部回读一致。严格 lint：32 页、368 链接、0 问题；另行核对了源码哈希、协议与真实判题的边界，不新增 accepted 账本断言、不覆盖 raw。可恢复草稿为本工作区忽略的 `.local/worker-mtls-vault-transaction.json`；外部飞书文档未发现已绑定同步目标。

## 2026-10-25：PR #1 回退竞态与 Unicode 源码修复

本条记录修复验证阶段：用户要求审查并修复 PR #1；基于 `a31e021`，沿用 `codex/engineering-baseline` 的未提交工作区。未安装 CodeRabbit，按用户选择由 Codex 直接审查与复现；该验证阶段没有提交、推送、修改远端审查状态或部署。随后用户授权按回退并发保护、Unicode 源码保真整理为两个原子提交并推送任务分支；提交标识以 Git 历史为准，推送与新 CI 结果须另行回读，不沿用旧提交状态。

- 两份实验 Down 迁移先获取 `submissions` 的 ACCESS EXCLUSIVE 锁，再检查空表。此前 advisory lock 不覆盖 API 接收事务，检查可能漏掉在途写入；旧代码实测在写入提交后仍完成回退，版本 1 删除源码与任务。新增真实 PostgreSQL 回归通过观察阻塞关系控制并发顺序，验证回退拒绝、模式版本和已接收内容保留。
- 实验提交入口在标准库 JSON 语法校验后、解码前检查 UTF-16 代理项配对，拒绝静默替换源码的输入；不新增依赖、不改变大小限制或幂等规则。14 个 HTTP 边界用例覆盖非法代理项、非法 UTF-8/转义、合法代理对、中文、反斜杠及原有 U+FFFD 字符。修复前 6 个非法代理项用例返回 202，修复后全部通过，非法请求不会进入 Store。
- 两份迁移只增强尚未发布的 Down，Up/模式版本保持不变；已有实验库无需迁移。需重建 migrate 才能获得回退保护，旧二进制不能继续用于回退；已有数据应前滚修复。表锁可能阻塞接收，实验调用方仍须按原键核验未知回执。
- 同步实验 OpenAPI、M0 记录与本交付条目。验证环境沿用 macOS arm64、Go 1.26.10、PostgreSQL 14.19；真实数据库测试使用独立私有临时集群并在结束后停止，没有连接用户已有数据库。专用 Linux 沙箱未重跑，M0 整体仍未通过。

验证：新增回归在旧实现上失败；修复后 `GIN_MODE=release GOPROXY=off make db-test` 通过，包含迁移/事务/租约及两个真实 API、24 并发请求与重启恢复。`GIN_MODE=release GOPROXY=off make check` 通过，含 Go 格式/vet/race/三命令构建、前端 lint/类型/6 项测试/构建、仓库检查和 25 项治理回归；`git diff --check` 通过。诊断日志保留在忽略的 `.local/pr1-before-db.log`、`.local/pr1-after-db.log`、`.local/pr1-after-check.log`。远端 CI 仅核验了原提交 `a31e021`，不代表本次未推送修复已通过远端检查。

收尾：仓库策略检查 89 文件通过；知识事务 `oj-pr1-review-fixes-20261025` inspect/apply 完成，更新 M0 实测、推进、热点、索引、日志共 5 页。回读 5 个目标及 11 份实现/测试/日志哈希一致；严格 lint：32 页、363 链接、0 问题。另行语义复核保留旧回退保证的局限、未提交状态和 M0 未完成边界。事务草稿在忽略的 `.local/pr1-vault-transaction.json`；无外部飞书同步目标、无推送或部署。

## 2026-10-25：原子提交、PR 与 GitHub 治理

用户授权将现有改动整理为原子提交、推送并创建 PR，配置本仓库；另行明确授权从空基点 `5d233d6` 创建 main，并确认仍由其维护、不强制非作者批准。保留既有三个工程基线提交，不改写共享历史；新增文本比较 `720ebf5`、独立沙箱实验 `5611cf5`、持久化/租约及契约测试文档 `934805f`，治理配置为后续独立提交。

- 已建立 main 并设为默认分支，启用无旁路的 `main-governance` ruleset（25824684）；PR、最新主线、4 项来源限定的 required checks、讨论解决、线性历史、禁止强推/删除。仅 squash，禁止自动合并，合并后删任务分支。仓库 Actions 只读、不能创建/批准 PR、限定 actions 来源并强制完整 SHA。安全扫描、推送保护及 Dependabot 安全更新已回读为 enabled。配置及控制缺口见 [GitHub 治理](github-governance.md)。
- `make check` 通过：Go 格式/vet/race/构建、前端 lint/类型/6 项测试/构建、治理检查；新增 PR 标题校验后 25 项治理回归通过。`make sqlc` 重新生成且无内容漂移。
- `make db-test` 通过：真实 PostgreSQL 事务/迁移/租约测试，两个 API 子进程的 24 个并发请求只创建 1 个提交，重启后回执可恢复。首次受限执行因共享内存被拒绝，按工具权限流程重跑通过，未降低测试。专用特权 VM 未重启，沙箱结果引用此前 M0 记录，不能记为本次重跑。
- 正常 pre-commit/commit-msg/pre-push hooks 均通过；没有关闭 hooks、强推或合并。[PR #1](https://github.com/DL-Association-for-Computing-Machinery/OnlineJudge/pull/1) 已创建并关联本次任务。
- 治理提交 `3c1b3c70cbe8e1b483163cc407f75703919104f4` 的[首轮远端 CI](https://github.com/DL-Association-for-Computing-Machinery/OnlineJudge/actions/runs/40204905120) 四项全部 success。Ubuntu 24.04 runner 执行治理、Go、前端和真实 PostgreSQL/sqlc 检查；后续文档同步提交仍须独立查看对应运行，不能沿用旧 head 的绿灯。
- 知识事务 `oj-github-governance-20261025` inspect/apply 完成，更新工程流程、推进、M0 历史说明、热点、索引和日志共 6 页；回读哈希全部一致。严格 lint 扫描 32 页、360 链接、0 问题；语义复核保留单人审查和 M0 未完成边界，不把历史沙箱结果当本次重跑。可恢复草稿为忽略的 `.local/github-governance/vault-transaction.json`。

配置前后快照保存在忽略的 `.local/github-governance/`；GitHub 插件的创建分支接口因集成权限返回 403，已使用经核验具备 admin 的 GitHub CLI 账号完成用户授权的初始化。单人维护无独立人工批准、生产 CD 与 M0 整体验收未完成。

## 2026-10-25：M0 持久化、任务租约与 Linux 沙箱分项

用户要求执行首期开发计划，并允许先用本机配置。沿用 `codex/engineering-baseline`，保留原有 README/交付基线/开发计划文档改动，无提交、推送或部署。

- 接入 pgx 5.11.0、Goose 3.29.0 与 sqlc 1.31.1，建立两份独立迁移、材料/源码/任务/尝试/节点模型。默认关闭的本机实验 API 支持原子接收、幂等冲突、回执和所有权查询；任务核心实现竞争认领、节点容量、续租、结果幂等、代次隔离和有界恢复。Worker HTTP/mTLS、材料授权下载与后台恢复尚未连接，不能把持久化方法称为运行中的判题服务。
- 新增文本检查器及规则测试；在新建的 Colima `oj-m0` 专用 VM 内运行固定摘要 go-judge 1.13.3＋GCC 15.3.0。2 CPU/2 GiB、Linux arm64/cgroup v2、无宿主挂载/SSH agent/暴露端口，原 Docker context 保持 `desktop-linux`。真实 GNU++17、错误/超限、文件/网络/临时目录隔离与缓存清理分项通过。
- 更新 README、实验 OpenAPI、M0 记录、开发计划/交付基线、需求子项状态及受影响架构/CI 文档。新增 PostgreSQL integration job；远端 CI 未运行，专用沙箱不进入普通 PR 流水线。

实际验证：`make check` 全部通过（Go 格式/vet/race/3 个命令构建，前端 6 项测试及 lint/类型/构建，23 项治理回归）；`make db-test` 真实 PostgreSQL 14.19 通过，含两个真实 API 子进程、24 请求唯一接收、重启回执恢复与全部事务/迁移/租约子项；`OJ_SANDBOX_EXPERIMENT=1 make sandbox-test` 的 12 个子项通过。sqlc 重新生成前后 4 个文件哈希一致；4 份 YAML 解析通过；`govulncheck ./...` 为 0 可达/已导入包漏洞，保留 1 个未导入模块告警。仓库检查、文档引用与 diff 检查在收尾再次执行，日志位于忽略的 `.local/m0-*.log`。

实际诊断：PostgreSQL initdb 在沙箱内被共享内存权限限制，经工具审批后在独立临时集群测试；Goose 无待迁移的 Up 不争锁，锁测试改为实际 Down，并保证失败也解锁；Colima 不挂载宿主路径时，`colima ssh` 从宿主项目 cwd 启动会失败，从 `/` 运行后只读核验成功。均未通过关闭检查、放宽隔离或伪造成功绕过。

M0 仍未通过：欠缺完整双 API/双 Worker 真实链路、mTLS/逐次材料授权、租约失效中止执行、环境随尝试固定、会话关键实验及综合故障恢复；M1/M2 未启动。当前 FIFO、静态实验主体和实验预算只用于 M0，不能替代正式公平调度、账号、容量目标或恢复验收。完整范围、复现方式、许可/镜像扫描缺口见[M0 记录](m0-experiment.md)。知识回写与实验资源停止状态在收尾回读核验。

收尾核验：本任务沙箱和 `oj-m0` VM 已停止，原 context 仍为 `desktop-linux`；没有删除镜像、volume 或用户文件。`claude-obsidian:save` 事务 `oj-m0-foundations-20261025` inspect/apply 完成，新增 M0 实测索引并更新实体/推进/索引/热点/日志，共 6 页；严格 lint（2026-10-25）检查 32 页、362 链接、0 问题，6 个写入目标及 14 份工作区证据哈希回读一致。事务草稿在 `.local/m0-vault-transaction.json`；结构检查不等同 M0 整体或生产验收。

## 2026-10-25：首期开发计划文档

用户要求根据需求和架构给出首期计划，并整理为开发文档。新增[首期开发计划](phase-1-development-plan.md)，按 0B 关键验证、1A 训练试用、1B 完整训练组织 P0—P7 开发包，记录依赖、模块边界、里程碑、完成条件、需求子项映射及首个实施批次；同步 README 和精简交付基线入口。原需求、架构选型和发布范围保持，硬件、试用账号入口、容量与恢复数值仍待确认，不将计划列为业务完成。

本次为 `codex/engineering-baseline` 未提交工作区的纯文档变更。已执行 `python3 scripts/repo_policy.py repository`（50 文件通过）及 `git diff --check`；静态检查核对计划与入口文档的本地链接、代码围栏、8 个开发包和 3 个里程碑，均通过。自审覆盖账号验证、题目审核、独立恢复和延期范围的一致性。未改代码、依赖、配置或运行命令，未重跑应用测试；没有安装、提交、推送、远端写入或部署。

已检查知识回写条件：本次整理既有决策和待实施任务，没有新增经实测验证的工程选择、故障或解决方案，故不重复写入 OJ_Knowledge，也不把计划升级为已验证断言。

## 2026-10-25：原子提交与任务分支推送准备

用户明确要求“整理为原子提交，然后推送到远端”。保留原有空基线 `5d233d6`，按依赖顺序组织三个逻辑提交：

1. `7712d59`：仓库治理与 Git hooks，包含用户既有暂存的 22 项治理回归及 PR 模板；对准确暂存快照在独立临时目录验证，22 项通过。
2. `4b58471`：可运行 API/Worker/前端骨架、当前接口契约、锁文件、Makefile、真实 CI 与对应第 23 项治理回归。暂存哈希与本轮 `make check` 通过的文件一致。
3. 项目规则、知识库相对路径绑定、完整需求/架构、精简交付基线及运行/验证文档；修订仅同步版本管理状态，不改变产品规则。该提交自身标识由 Git 历史提供，避免自引用哈希。

本轮 `make check` 实际通过：Go 格式/vet/race/构建，前端 lint/类型/6 项测试/构建，49 文件仓库策略与 23 项治理回归。提交使用正常 pre-commit/commit-msg hooks，不禁用检查；推送使用正常 pre-push hook。`.local/`、依赖、构建产物及临时截图不纳入提交。既有历史日志中的“未提交”描述各次交付当时状态。

推送目标为 `origin` 的 `codex/engineering-baseline`，不新建或直推 main/master、不强推、不创建 PR 或部署。推送前 `git ls-remote --heads origin` 成功返回空分支列表。本条写于最后一次本地提交前，不预报推送成功；实际结果由远端引用回读与知识库推进记录补充核验。当前 workflow 仅监听 PR、main/master 推送及手动触发，任务分支推送不表示远端 CI 已运行。

## 2026-10-25：精简交付基线 v1.0 与可运行工程骨架

用户要求审计技术栈和产品范围、给出新版文档，并明确“先初始化我的仓库，搭出项目骨架”。复用已有 Git 和 `codex/engineering-baseline`，保留既有暂存内容，无提交、推送、PR 或部署。

- 新增 `docs/delivery-plan.md`，将 0A 骨架、0B 关键验证、1A 训练试用、1B 完整训练、战队、正式比赛、辅助能力分开；同步需求 v0.23、总体 v0.30、专题、README 与开发/CI 入口。安全、一致性和已交付功能的验证不延期；多文件/ZIP、自测与辅助能力后移，完整需求保留。Redis 基线未替换；PostgreSQL 会话对比及邀请制试用仅为待验证/待确认方案。
- 建立 Go/Gin API、独立 Worker 检查入口、React/TypeScript/Vite 前端、pnpm 锁文件、Makefile 与当前 OpenAPI。仅健康检查和基础错误响应已实现，Worker 明确提示未接入后退出；无数据库、账号、题目、提交、判题或容器部署。
- `.github/workflows/governance.yml` 从空仓库阻断切换为治理、后端、前端三个实际任务；保留 `bootstrap-ci` 原有拒绝应用行为并新增配置回归。远端 CI/保护未运行或配置，本地结果不冒充远端通过。
- 依赖按官方 Go/Gin、npm registry 与 GitHub Actions 官方仓库元数据核对；Go 1.26.10、Gin 1.12.0、React 19.3.1、Vite 8.3.5、pnpm 11.25.0，TypeScript 固定 6.0.3 以满足 typescript-eslint 的 `<6.1.0` 同伴约束。精确依赖以 `go.mod`、`web/package.json` 和锁文件为准；不修改全局 Go 配置，不安装数据库/沙箱工具。

实际验证环境为 macOS arm64；Go 自动工具链 1.26.10；pnpm 子命令 Node 24.19.0（shell Node 22.17.1）。

| 验证 | 实际结果与边界 |
| --- | --- |
| `make go-check go-build` | 格式、vet、race 测试、API/Worker 构建通过；依赖修复后再次通过 |
| `make web-check` | ESLint、TypeScript、6 项 Vitest、生产构建通过 |
| `pnpm --dir web install --frozen-lockfile --offline --ignore-scripts --store-dir ../.local/pnpm-store` | 本机已缓存依赖的冻结锁文件安装通过；不代表完整离线部署 |
| `.local/bin/worker` | 提示未接入任务/沙箱，正常退出，无伪造判题结果 |
| 本机浏览器 | 真实健康响应、API 停止后失败提示、重启后恢复通过；390×844 与 1280×900 无横向溢出，截图位于 `/private/tmp/oj-bootstrap-preview.png` |
| API 进程停止 | 对本轮进程发送 SIGTERM 后输出 `API stopped gracefully`，退出码 0 |
| `pnpm --dir web audit --json --registry=https://registry.npmjs.org` | 当次数据库报告 0 已知漏洞，不等于全面安全保证 |
| `.local/bin/govulncheck -show verbose ./...`（Go 缓存指向 `.local/`） | 首扫发现 5 个可达漏洞，升级 x/net 0.60.0、quic-go 0.59.1 及必要传递依赖后复扫为 0 可达、0 已导入包漏洞；保留 1 个模块内未导入的 openpgp 告警 GO-2026-5932，该弃用包无修复版本，项目不使用它 |
| `python3 -B -m unittest discover -s tests/governance -v` | 23 项通过 |
| `python3 scripts/repo_policy.py repository`、Git 空白检查 | 通过；文档链接、锚点和 AC01—35 连续性另做静态核验 |

源码与旧文档对照基线为 `/private/tmp/oj-bootstrap-before.json`，差异为 `/private/tmp/oj-bootstrap-review.diff`。静态核验通过：14 份变更文档、160 个本地链接/锚点、AC01—35 唯一有序；`go mod verify` 通过。UI 验证不覆盖未来业务功能；Linux、Docker、数据库、真实判题、容量、恢复和远端 CI 尚未验证，无独立人工审查。后续先取得可用 Linux 隔离节点，再做 GNU++17 文本题的持久化/任务/租约/沙箱实验。

知识回写使用 `claude-obsidian:save`，事务 `oj-bootstrap-20261025` inspect/apply 完成：新增 `wiki/syntheses/bootstrap-delivery-20261025.md`，同步项目、架构基线、推进、索引、热点和日志，共 7 个页面；旧 raw/证据账本不覆盖、不新增 accepted 业务断言。7 个目标回读哈希、10 份仓库证据哈希一致；严格 lint（`--as-of 2026-10-25`）扫描 31 页、355 链接、0 问题。可恢复草稿位于忽略目录 `.local/bootstrap-vault-transaction.json`，持久事务记录在 vault `.vault-meta/transactions/oj-bootstrap-20261025/`。结构通过不代表业务、性能或隔离能力通过。

## 2026-10-25：架构需求收敛与文档、知识库统一整理

- 用户本轮明确授权更新相关文档并吸收进知识库；暂停逐轮写入期间的确认已整理为当前基线，不进入应用实现。需求 v0.22、总体 v0.29、判题 v0.6、题包 v0.5、存储 v0.5；同步 README、ADR、前端/沙箱、会话后续入口和 CI/CD，新增比赛、API/Worker、运维三份专题，需求验收扩展为 AC01—35。
- 原位替换过期口径：默认文本按后续 ICPC 对标接受 ASCII 大小写差异，显式敏感题固定配置；整分钟/20 分钟/CE-SE 不计罚及并列规则；正式重测候选审核生效；DOMjudge 虚拟排队思路替代早期严格轮转。首期明确不建设协会成员身份、退役状态及飞书导入，未来身份结束保留账号的要求仍保留。
- 已整理源码文件集/自测、权限与审核、报名/名单、题库/题单任务、封榜/气球/答疑/查重/奖项/开放、外部同步/成长/名额/AI 出题、API/Outbox/节点身份、公平队列、观测/备份/清理/离线发布及五阶段验证。不改变既有基础选型；优先验证不是正式准入，硬件参数和运行结果未补造。公开题库 SEO/分享预览范围仍单列待产品细化。
- 实际静态验证：`python3 scripts/repo_policy.py repository` 27 文件通过；`git diff --check`、`git diff --cached --check` 通过；`python3 /private/tmp/oj-architecture-check.py` 检查 14 份本次变更文档及仓库全部文档的空白、代码围栏、157 个本地路径/锚点、AC01—35 唯一顺序及过期后续入口通过，并生成编辑前后差异。前端/沙箱 v0.16、会话 v0.12 同步状态入口。本轮未改治理脚本/hooks，不重复运行治理单元测试。
- 知识写入使用 `claude-obsidian:save`，事务 `oj-architecture-consolidation-20261025`，14 个目标 inspect/apply 完成。新增必要用户确认和架构基线索引，更新既有决策/实体/进度/来源、热点、索引、日志及账本；不保存完整聊天，不覆盖 raw。预检先发现 UTC 时间格式及来源内容身份需迁移，修正时间格式、身份和证据引用后通过，未绕过校验。
- 实际知识验证：14 个目标回读哈希一致、35 份本地来源哈希一致，推进页记录的 6 份当前规范哈希一致；`lint --strict --as-of 2026-10-25 --format json` 扫描 30 页、345 链接、0 问题。语义复核将旧来源标为历史，将后续确认与技术实测分开；accepted 只描述用户选择，不支持运行、安全或容量效果。

当前工作区 `codex/engineering-baseline` 未提交，原治理暂存内容保留。没有应用代码、依赖安装、选手程序执行、Linux 隔离/兼容/容量/恢复演练、联网复核、远端写入或部署，也没有独立人工审查。基线和差异为 `/private/tmp/oj-architecture-consolidation-before.json`、`/private/tmp/oj-architecture-consolidation-review.diff`；知识事务草稿为 `/private/tmp/oj-architecture-consolidation-vault.json`，持久恢复记录位于 vault `.vault-meta/transactions/oj-architecture-consolidation-20261025/`。外部条件、具体参数与准入仍待后续验证，不把历史交付记录中的默认值当作当前规范。

## 2026-10-25：固定顺序、提交内串行与首次失败停止确认

- 用户确认首期固定顺序＋单份提交串行测试＋首次失败停止；发布时固定必需测试点有序清单与题目版本。需编译语言每尝试编译一次、逐点新隔离运行环境，解释型语言按固定配置准备；不同提交仍按 Worker 资源预算并行。首个选手失败停止且不跨点按错误种类改判，全部要求测试通过才 AC；系统故障中止本次尝试并有限恢复或人工处理，不计选手错误/罚时。剩余点标未执行，耗时/内存仅描述实际执行部分。
- 更新判题 v0.5 第 5.1 节、总体 v0.28、需求 v0.21、题包 v0.4、前端/沙箱 v0.15、README、ADR-001、会话后续入口和 CI/CD；关联 AC05/06/16/17/18/20。补充顺序/历史版本重判、首/中/末点失败停止、前点通过才启动下一点、编译复用及逐点隔离、系统故障/迟到回报、跨提交并行和未执行/统计边界验收。单测试点异常归因/完整映射、具体统计聚合、资源数值及编排/隔离/恢复仍待设计验证；不自动引入跨提交编译缓存、断点复用或全量诊断模式。根规则、存储与开发流程已检查，无需更新；下一步讨论统一判题结果分类与单点异常归因，尚未确认具体方案。
- 实际验证：`python3 scripts/repo_policy.py repository` 24 文件通过；`git diff --check`、`git diff --cached --check` 通过；`python3 /private/tmp/oj-frontend-support-check.py test-order` 检查 Markdown 空白、代码块及 131 个本地链接/锚点，生成编辑前后逐文件差异供审查。2026-10-25 只读核对 DOMjudge 提前停止说明作为取舍参考，不将其完整配置或优先级视为本站规则。
- 知识事务 `oj-test-order-20261025` inspect/apply 完成，新增必要用户原文与提案语境，同步来源、决策、实体、进度、热点、索引、日志及账本，共 10 个目标；旧 raw 不变。回读 10 个目标哈希及 34 份本地来源哈希一致；严格 lint（`--strict --as-of 2026-10-25 --format json`）28 页、335 链接、0 问题。accepted 断言只证明规则选择，不证明真实编排、隔离、恢复或容量收益已经通过。

当前 `codex/engineering-baseline` 工作区未提交，原有暂存内容保留。基线、差异及事务草稿为 `/private/tmp/oj-test-order-before.json`、`/private/tmp/oj-test-order-doc-review.diff`、`/private/tmp/oj-test-order-vault-transaction.json`；持久化恢复记录位于 vault `.vault-meta/transactions/oj-test-order-20261025/`。没有依赖安装、应用代码、程序执行、编排/隔离/恢复/数值/容量运行测试、提交推送或部署，无独立人工审查。

## 2026-10-25：浮点容差比较规则确认

- 用户确认每题显式绝对/相对容差，两者同时配置时取 OR；相对基准为标准值且包含误差边界。有限十进制/科学计数法、数值位置非法输出为 WA、混合文本区分大小写、严格整数混合使用 SPJ、非法配置/无效标准答案阻止发布、配置及比较实现版本绑定约定同步。
- 更新判题设计 v0.4 第 3 节、总体架构 v0.27、需求 v0.20、题包 v0.3、前端/沙箱 v0.14、README、ADR-001、会话后续入口和 CI/CD。关联 AC05/06/16/17/18/20，补充绝对/相对/OR、边界内/等于/外、仅满足一项、零/接近零/负数、科学计数法、非法数字、混合文本及发布阻断验收。逐题误差值、数值精度/解析/溢出/下溢、超长 token 与多节点/testlib 一致性仍待实现验证；根规则、存储和开发流程已检查，无需更新。下一步讨论测试点顺序、提前停止与最终结果汇总，尚未确认方案。
- 实际验证：`python3 scripts/repo_policy.py repository` 24 文件通过；`git diff --check`、`git diff --cached --check` 通过；`python3 /private/tmp/oj-frontend-support-check.py float-policy` 检查 Markdown 空白、代码块和 128 个本地链接/锚点，生成编辑前后差异供审查。2026-10-25 只读核对 ICPC 2025-09 浮点规则，不把引用规范等同于完整题包兼容或组件准入。
- 知识事务 `oj-float-policy-20261025` inspect/apply 完成：新增必要用户原文/提案语境来源页，同步既有来源、决策、实体、进度、热点、索引、日志和账本，共 10 个目标；旧 raw 不变。回读 10 个目标哈希与 33 份本地来源哈希一致；严格 lint（`--strict --as-of 2026-10-25 --format json`）27 页、324 链接、0 问题。accepted 断言仅证明用户规则选择，不证明数值实现或判题效果通过。

当前 `codex/engineering-baseline` 工作区未提交，原有暂存内容保留。审查基线、差异和事务草稿位于 `/private/tmp/oj-float-policy-before.json`、`/private/tmp/oj-float-policy-doc-review.diff`、`/private/tmp/oj-float-policy-vault-transaction.json`；持久化恢复记录位于 vault `.vault-meta/transactions/oj-float-policy-20261025/`。没有依赖安装、应用代码、检查器编译或程序执行、数值/判题/Linux 隔离/容量测试、提交推送或部署，无独立人工审查。

## 2026-10-25：SPJ 协议、testlib 验证路线与 GNU++17 确认

- 用户确认 ICPC 输出验证器协议、testlib 优先验证、首批 GNU++17 SPJ；输入/参考答案文件、选手输出标准输入，原生 42/43 正确/错误，其他退出（含 0）及异常为系统错误。固定 testlib/适配器/编译参数/制品哈希到题目版本，核验本站比较语义，诊断默认供裁判且选手反馈按授权过滤，依赖和隔离验证后准入。
- 更新判题设计 v0.3 第 4.1 节、总体架构 v0.26、需求 v0.19、题包 v0.2、前端/沙箱 v0.13、README、ADR-001、会话后续入口和 CI/CD；关联 AC05/06/16/17/18/20。原生协议与 testlib 原始结果分开解释，未知/缺失模式不猜测为通过，42 只表示单测试检查通过；完整原始结果映射、构建/分发、精确依赖和预算仍待验证。存储及根规则已检查，无需更新；下一步讨论浮点容差具体规则。
- 实际验证：`python3 scripts/repo_policy.py repository` 24 文件通过；`git diff --check`、`git diff --cached --check` 通过；`python3 /private/tmp/oj-frontend-support-check.py spj-protocol` 检查 Markdown 空白、代码块及 126 个本地链接/锚点，生成编辑前后逐文件差异供审查。已确认的协议/首批语言与未完成的 testlib 准入、适配和数值验证分开记录。
- 知识事务 `oj-spj-protocol-20261025` inspect/apply 完成，新增必要用户原文/语境来源页，同步既有来源、决策、实体、进度、热点、索引、日志和账本，共 10 个目标；旧 raw 不变。回读 10 个目标哈希与 32 份本地来源哈希一致；严格 lint（`--strict --as-of 2026-10-25 --format json`）26 页、313 链接、0 问题。新增 accepted 断言只表示用户协议/语言和验证路线及约定确认，不表示组件、适配、隔离或判题效果通过。

当前 `codex/engineering-baseline` 工作区未提交，原有暂存内容保留。审查基线、差异和事务草稿位于 `/private/tmp/oj-spj-protocol-before.json`、`/private/tmp/oj-spj-protocol-doc-review.diff`、`/private/tmp/oj-spj-protocol-vault-transaction.json`；持久化恢复记录位于 vault `.vault-meta/transactions/oj-spj-protocol-20261025/`。没有依赖安装、应用代码、检查器编译或程序执行、Linux/判题/数值/容量测试、提交推送或部署，无独立人工审查。

## 2026-10-25：统一题目模型与 2025-09 子集导入路线确认

- 用户确认统一内部题目模型＋优先验证 ICPC／Kattis 2025-09 受限子集 ZIP 导入，手动录入/导入共用私有草稿、受限验证、审核和不可变发布；保留原包/哈希/来源/使用依据，不支持内容报告并阻止发布，判定语义差异显式处理，API 不执行包内程序。
- 新增题包设计 v0.1，更新总体架构 v0.25、需求 v0.18、判题设计 v0.2、前端/沙箱 v0.12、存储 v0.4、README、ADR-001、会话后续入口及 CI/CD；关联 AC05/06/17/18/20/22。区分已确认模型/验证路线与未验证子集/组件，记录 KaTeX/MathJax 和来源大小写/浮点策略的映射边界；不自动确认完整兼容、任意脚本执行或 SPJ 组件。根规则已检查，无需更新，下一步讨论 SPJ 协议与检查器复用。
- 实际验证：`python3 scripts/repo_policy.py repository` 24 文件通过；`git diff --check`、`git diff --cached --check` 通过；`python3 /private/tmp/oj-frontend-support-check.py package-import` 检查 Markdown 空白、代码块和 118 个本地链接/锚点，以编辑前基线（新增文件按空内容）生成逐文件差异供审查。已同步题包路线、存储原包追溯和后续事项入口，字段/兼容矩阵/解析器/预算继续待定。
- 知识事务 `oj-package-import-20261025` inspect/apply 完成，新增必要用户原文及所指建议来源页，更新既有来源、决策、实体、进度、热点、索引、日志和账本，共 10 个目标；旧 raw 不变。回读 10 个目标哈希与 31 份本地来源哈希一致；严格 lint（`--strict --as-of 2026-10-25 --format json`）25 页、302 链接、0 问题。新增 accepted 断言仅表示模型/验证路线与流程确认，不证明格式兼容、解析安全或实际导入效果。

当前 `codex/engineering-baseline` 工作区未提交，原有暂存内容保留。审查基线、差异和事务草稿为 `/private/tmp/oj-package-import-before.json`、`/private/tmp/oj-package-import-doc-review.diff`、`/private/tmp/oj-package-import-vault-transaction.json`；持久化恢复记录位于 vault `.vault-meta/transactions/oj-package-import-20261025/`。没有依赖安装、真实题包导入、应用代码、检查器编译、程序执行、隔离/容量/恢复运行测试、提交推送或部署，无独立人工审查。

## 2026-10-25：首期题型与答案检查原则确认

- 用户确认首期标准输入输出＋文本比较、显式浮点容差和受限 SPJ，交互题/直接提交答案题后续扩展；文本默认区分大小写、忽略空白数量/换行差异，浮点按题显式配置，严格整数/浮点混合用自定义检查器。检查器及参数绑定题目版本，检查器失败归系统错误，全部要求测试通过才 AC，不引入部分分。
- 新增判题设计 v0.1，更新总体架构 v0.24、需求 v0.17、前端/沙箱 v0.11、README、ADR-001、会话后续入口及 CI/CD；关联 AC05/06/16/17/18/20。先文本闭环再补齐浮点/SPJ，完整首期仍须分别验证。浮点数值边界/误差组合、SPJ 协议/组件/语言与资源预算仍待细化；题目包格式尚未确认，下一步讨论其与导入方式。存储及根规则已检查，无需更新。
- 实际验证：`python3 scripts/repo_policy.py repository` 23 文件通过；`git diff --check`、`git diff --cached --check` 通过；`python3 /private/tmp/oj-frontend-support-check.py judging-scope` 检查 Markdown 空白、代码块及 99 个本地链接/锚点，以编辑前基线（新增文件按空内容）生成逐文件差异供审查。已确认范围不再保留为待确认或仅推荐，具体实现缺口继续明确记录。
- 知识事务 `oj-judging-scope-20261025` inspect/apply 完成，新增必要原文/所指判定建议来源页，更新既有来源、决策、实体、进度、热点、索引、日志和账本，共 10 个目标；旧 raw 不变。回读 10 个目标哈希与 30 份本地来源哈希一致；严格 lint（`--strict --as-of 2026-10-25 --format json`）24 页、291 链接、0 问题。新增 accepted 断言仅表示用户范围与判定原则确认，不证明数值实现、隔离或判题效果。

当前 `codex/engineering-baseline` 工作区未提交，原有暂存内容保留。审查基线、差异和事务草稿为 `/private/tmp/oj-judging-scope-before.json`、`/private/tmp/oj-judging-scope-doc-review.diff`、`/private/tmp/oj-judging-scope-vault-transaction.json`；持久化恢复记录位于 vault `.vault-meta/transactions/oj-judging-scope-20261025/`。无依赖安装、应用代码、检查器编译、选手程序执行、Linux/判题/数值/容量运行测试、提交推送或部署，无独立人工审查。

## 2026-10-25：题面 Markdown 与公式渲染确认

- 用户确认 react-markdown＋remark-gfm＋remark-math＋rehype-katex／KaTeX 及推荐渲染约定：预览/选手页面共用组件与配置、禁用原始 HTML、限制链接/图片地址、trust 为 false 及有限公式展开/尺寸、校内图片/CSS/字体、原始样例文本复制和公式局部错误反馈。
- 更新前端设计 v0.10 第 3.3 节、总体架构 v0.23、需求 v0.16、README、ADR-001、会话后续事项及 CI/CD；验收关联 AC06/17/18/20，不新增或改变既有 AC 编号语义。精确版本、许可/兼容性、公式资源预算及实际浏览器效果仍待核验；存储与根规则已检查，无需更新。下一步讨论首期题型与答案检查规则，尚未确认具体范围或检查器组件。
- 实际验证：`python3 scripts/repo_policy.py repository` 22 文件通过；`git diff --check`、`git diff --cached --check` 通过；`python3 /private/tmp/oj-frontend-support-check.py markdown-render` 检查仓库 Markdown 空白、代码块和 82 个本地链接/锚点，并以编辑前基线生成逐文件差异供审查。候选/待确认状态及下一步入口已同步。
- 知识事务 `oj-markdown-render-20261025` inspect/apply 完成，新增必要用户原文与所指建议来源页，更新既有来源、决策、实体、进度、热点、索引、日志和账本，共 10 个目标；旧 raw 不变。回读 10 个目标哈希与 29 份本地来源哈希一致；严格 lint（`--strict --as-of 2026-10-25 --format json`）23 页、280 链接、0 问题。新增 accepted 断言仅支持用户选型与行为约定，不把提案或库声明当作功能、安全或性能通过证据。

当前 `codex/engineering-baseline` 工作区未提交，原有暂存内容保留。审查基线、差异和事务草稿分别位于 `/private/tmp/oj-markdown-render-before.json`、`/private/tmp/oj-markdown-render-doc-review.diff`、`/private/tmp/oj-markdown-render-vault-transaction.json`；持久化恢复记录位于 vault `.vault-meta/transactions/oj-markdown-render-20261025/`。没有依赖安装、应用代码、真实浏览器/构建/性能测试、提交推送或部署，无独立人工审查。

## 2026-10-25：IndexedDB 本地草稿保存、恢复与清理确认

- 用户确认 IndexedDB＋idb：停输 1 秒后保存、持续输入目标每 5 秒保存；按账号/题目/比赛/语言隔离，事务成功且对应当前版本才显示已保存，失败提示并允许下载；多标签冲突保留双方，不按时间自动删除，提供导出/手动清理；主动退出默认勾选清除此账号草稿且可取消，会话自然过期保留供同账号重新登录恢复。
- 更新前端设计 v0.9 第 3.2 节、总体架构 v0.22、需求 v0.15、会话设计 v0.11、存储设计 v0.3、README、ADR-001 和 CI/CD。AC21/23 增补保存调度/版本、模式升级受阻、容量失败、多标签冲突与清理/迟到写入竞态；明确保存目标不是崩溃恢复保证，容量预算、精确版本与运行实现仍待定。根规则已检查，无需更新。下一步讨论题面 Markdown 与公式渲染，相关组件尚未选定。
- 实际验证：`python3 scripts/repo_policy.py repository` 22 文件通过；`git diff --check`、`git diff --cached --check` 通过；`python3 /private/tmp/oj-frontend-support-check.py idb-draft` 检查 Markdown 空白、代码块及 79 个本地链接/锚点，并生成编辑前后逐文件差异供审查。首轮批量补丁输出被截断导致格式校验失败且未应用，扩大输出预算并缩小差异上下文后成功；未绕过检查或覆盖用户既有改动。
- 知识事务 `oj-idb-draft-20261025` inspect/apply 完成，新增本轮必要原文与所指建议来源页，同步既有来源、决策、实体、进度、热点、索引、日志和账本，共 10 个目标；旧 raw 不变。回读 10 个目标哈希与 28 份本地来源哈希一致；严格 lint（`--strict --as-of 2026-10-25 --format json`）22 页、269 链接、0 问题。新增 accepted 断言仅支持用户确认，不证明浏览器持久性或功能效果。

当前 `codex/engineering-baseline` 工作区未提交，原有暂存内容保留。审查基线、差异和事务草稿为 `/private/tmp/oj-idb-draft-before.json`、`/private/tmp/oj-idb-draft-doc-review.diff`、`/private/tmp/oj-idb-draft-vault-transaction.json`；持久化恢复记录位于 vault `.vault-meta/transactions/oj-idb-draft-20261025/`。没有依赖安装、应用实现、真实浏览器/构建/容量测试、实际草稿清理、提交推送或部署，无独立人工审查。

## 2026-10-25：前端路由、请求、缓存与包管理器确认

- 用户确认 React Router（声明式模式）＋TanStack Query＋原生 fetch＋统一封装＋pnpm，并接受写操作不自动重试、提交超时按原幂等键核验接收状态、账号切换清理私有查询缓存、敏感查询不落盘及按需受控刷新。记录原请求取消不等于写入回滚、旧查询迟到响应隔离、任务终态停轮询和冻结锁文件构建等实施/验收约束；具体依赖、查询预算及刷新间隔仍待核验。
- 更新前端设计 v0.8 第 3.1 节、总体架构 v0.21、需求 v0.14、README、ADR-001、会话文档后续事项及 CI/CD；关联 AC18/19/20/21/23。没有配置虚构的应用命令。存储及根规则已检查，无需更新。下一步讨论本地草稿存储、保存/清理与多标签冲突；IndexedDB、localStorage 或相关封装尚未选择。
- 实际验证：`python3 scripts/repo_policy.py repository` 22 文件通过；`git diff --check`、`git diff --cached --check` 通过。`python3 /private/tmp/oj-frontend-support-check.py` 检查仓库 Markdown 空白、代码块及 74 个本地链接/锚点，并以编辑前基线生成逐文件差异审查；相关文档中的候选/待确认状态已同步。
- 知识事务 `oj-frontend-support-20261025` inspect/apply 完成，新增必要用户原文与提案语境来源页，并同步既有来源、决策、实体、推进、热点、索引、日志与账本，共 10 个目标，旧 raw 不变。回读 10 个目标哈希与 27 份本地来源哈希一致；严格 lint（`--strict --as-of 2026-10-25 --format json`）21 页、257 链接、0 问题。新增 accepted 断言只表示用户选型与规则确认，不将 AI 提案作为独立效果证据。

当前 `codex/engineering-baseline` 工作区未提交，原有暂存内容保留。审查基线、差异与事务草稿为 `/private/tmp/oj-frontend-support-before.json`、`/private/tmp/oj-frontend-support-doc-review.diff`、`/private/tmp/oj-frontend-support-vault-transaction.json`；持久化恢复记录位于 vault `.vault-meta/transactions/oj-frontend-support-20261025/`。没有依赖安装、应用实现、锁文件生成、浏览器/构建/容量测试、提交推送或部署，无独立人工审查。

## 2026-10-25：邮件令牌期限、旧链接保留与重试规则确认

- 用户确认推荐默认：注册验证链接 24 小时、找回密码链接 30 分钟、主动重发间隔至少 60 秒；每次主动重发新建令牌，旧链接按原期限保留，同账号同用途最多 3 个，满额暂缓新申请而不挤掉旧链接；任一成功完成操作后同用途全部失效。临时失败最多重试 5 次（含首次 6 次），每任务发送窗口最多 10 分钟且不超令牌到期，退避加随机抖动，永久失败停止。
- 同步会话设计 v0.10、总体架构 v0.20、需求 v0.13、README、ADR-001、前端后续事项与 CI/CD。AC25 增补多链接并发消费/重发、满额与到期释放、时间边界、迟到邮件及载荷清理不提前撤销链接；额外限流数值、供应商、密钥轮换和未验证账号保留期仍待定。存储及根规则已检查，无需更新；下一步讨论前端路由、请求层与包管理器，尚未确认新组件。
- 实际验证：`python3 scripts/repo_policy.py repository` 22 文件通过；`git diff --check`、`git diff --cached --check` 通过；当前全部仓库 Markdown 的 70 个本地链接/锚点、空白及代码块检查通过。未跟踪文档以编辑前内容逐文件生成差异审查，复核仅含本轮规则及状态同步。临时链接检查器首次错误地跨越相邻 Markdown 链接，修正提取规则后通过，未据此改动原文。
- 知识事务 `oj-token-policy-20261025` inspect/apply 完成，新增必要用户原文及所指提案语境来源页，更新既有来源、决策、实体、推进、热点、索引、日志和账本，共 10 个目标；旧 raw 不变。回读 10 个目标哈希与 26 份本地来源哈希一致；严格 lint（`--strict --as-of 2026-10-25 --format json`）20 页、246 链接、0 问题。用户选择可确认为需求，不将 AI 提案作为独立技术效果证据。

未提交工作区分支 `codex/engineering-baseline`，保留原有暂存内容。审查基线、差异和事务草稿分别位于 `/private/tmp/oj-token-policy-before.json`、`/private/tmp/oj-token-policy-doc-review.diff` 和 `/private/tmp/oj-token-policy-vault-transaction.json`；持久化事务记录位于 vault `.vault-meta/transactions/oj-token-policy-20261025/`。没有依赖安装、应用实现、真实发信、并发/恢复/容量测试、提交推送或部署，无独立人工审查。

## 2026-10-25：邮件载荷方案与不考虑付费约束（中断后完成）

- 已完成上次中断未应用的更新：业务侧短期加密载荷＋River 仅存业务 ID＋Go 标准库 AES-GCM＋独立密钥文件注入，以及不考虑付费方案或技术栈。同步会话设计 v0.9、总体架构 v0.19、需求 v0.12、ADR-001、README、CI/CD 和前端后续事项。River Pro/付费服务排除，历史比较保留但不再作为当前候选；免费条件和许可证仍需核验。
- AC25 补充 AAD、随机失败、跨副本密钥版本/总使用量、文件异常、轮换清理和恢复验收。未生成真实密钥、安装依赖或运行加密测试。下一步的 24 小时/30 分钟令牌、60 秒重发间隔及有限重试仅为建议，未记为已确认。存储和根规则已检查，无需更新。
- 实际核验：`python3 scripts/repo_policy.py repository` 22 文件通过；`git diff --check`、`git diff --cached --check` 通过；编辑前副本用于审查未跟踪文档，Markdown 空白、代码块、本地链接/锚点通过。上述仅为文档与治理验证。
- 知识事务 `oj-payload-free-20261025` inspect/apply 完成；11 个目标哈希、25 份本地来源哈希一致；严格 lint（`--strict --as-of 2026-10-25 --format json`）19 页、235 链接、0 问题，两条新增用户决定与原文一致。

保存 866 字节必要原文 `.raw/captured/2026-10-25-payload-free-user.md`，同步既有来源、决策、实体、推进、热点、索引、日志及账本，旧 raw 不变；恢复材料位于忽略目录 `.local/payload-vault-transaction.json`、`.local/payload-vault-result.json`。当前 `codex/engineering-baseline` 工作区未提交，原有暂存内容保留；无采购、真实发信、应用/迁移/恢复/压力测试、提交推送或部署，无独立人工审查。

## 2026-10-25：River 开源核心优先验证确认

- 用户确认优先验证 River 开源核心，保留小型专用任务表为备选。同步会话设计 v0.8、总体架构 v0.18、ADR-001、README、需求状态、前端后续事项及 CI/CD；需求保持 v0.11，AC25 保留同事务、迁移/恢复、开源功能和预算检查。正式准入、版本与部署均未验证。
- 下一步讨论业务侧短期加密邮件载荷、River 参数仅业务 ID 及密钥管理；AES-GCM、部署密钥文件和 Pro 均未由用户选择。只读查阅 Go 认证加密及 River Pro 文档；存储及根规则已检查，无需更新。
- 实际检查：`python3 scripts/repo_policy.py repository` 22 文件通过；`git diff --check`、`git diff --cached --check` 通过；用编辑前副本检查未跟踪文档差异，Markdown 空白、代码块、本地链接/锚点通过。治理检查不替代应用验证。
- 知识事务 `oj-river-validation-20261025` inspect/apply 完成，11 个目标哈希和 24 份本地来源哈希一致；严格 lint（`--strict --as-of 2026-10-25 --format json`）19 页、228 链接、0 问题；新增用户选择断言与原文一致。

保存 476 字节必要原文 `.raw/captured/2026-10-25-river-user.md`，更新既有来源、决策、实体、推进、热点、索引、日志与账本，旧 raw 不变；可恢复事务与结果在忽略目录 `.local/river-vault-transaction.json`、`.local/river-vault-result.json`。当前 `codex/engineering-baseline` 工作区未提交，原有暂存内容保留；无依赖安装、真实发信、应用/迁移/恢复/负载测试、Pro 采购、提交推送或部署，无独立人工审查。

## 2026-10-25：PostgreSQL 发信任务与独立受限执行器验证路线

- 用户确认优先验证 PostgreSQL 持久化发信任务＋独立受限发送执行器；更新会话设计 v0.7、总体架构 v0.17、ADR-001、README、需求状态、前端后续事项及 CI/CD。需求保持 v0.11，AC25 细化事务入队、竞争领取、崩溃恢复、迟到回报、重复投递、过期清理与积压恢复预算。
- 比较 River 开源核心和小型专用任务表，建议先验证 River，但用户尚未选择。记录 pgx 同事务、Goose 导出 SQL 迁移路线、MPL-2.0 与 Pro 功能边界、全局发送额度及秘密载荷的待设计项；没有宣称库已准入、资源开销已验证或费用已确定。只读查阅 River 官方文档，未安装、运行或购买。存储及根规则已检查，无需更新。

| 核验 | 实际结果 |
| --- | --- |
| `python3 scripts/repo_policy.py repository` | 22 个文件通过，仅仓库治理 |
| `git diff --check`、`git diff --cached --check` | 通过；编辑前副本用于审查未跟踪文档差异，Markdown 空白、代码块、本地链接及锚点检查通过 |
| vault 事务与回读 | `oj-mailtasks-validation-20261025` inspect/apply 完成；11 个目标哈希、23 份本地来源哈希一致 |
| vault 严格 lint | `lint --strict --as-of 2026-10-25 --format json`：19 页、222 链接、0 问题；新增断言与用户原文一致 |

知识回写保存 550 字节必要原文 `.raw/captured/2026-10-25-mailtasks-user.md`，更新既有来源、决策、实体、推进、热点、索引、日志和账本，旧 raw 不变；可恢复事务与结果位于忽略目录 `.local/mailtasks-vault-transaction.json`、`.local/mailtasks-vault-result.json`。当前 `codex/engineering-baseline` 工作区未提交，原有暂存内容保留；无依赖安装、应用实现、真实发信、迁移/故障/并发/恢复测试、压测、提交推送或部署，无独立人工审查。

## 2026-10-25：SMTP 接口与 go-mail 验证路线确认

- 用户确认采用可替换 SMTP 接口＋优先验证 go-mail，供应商等域名和学校现有服务明确后再定。更新会话设计 v0.6、总体架构 v0.16、ADR-001、README、需求状态、前端后续事项和 CI/CD；需求维持 v0.11，细化 AC25 的适配器替换、TLS、错误语义、连接预算及日志脱敏准入条件。没有把验证顺序写为库正式准入。
- 下一步比较进程内队列、PostgreSQL 发信任务和 Redis＋Asynq；PostgreSQL 路线及 River 等任务库评估仅为建议，未确认异步实现或改变 Redis 会话用途。只读查阅 PostgreSQL、Asynq 与 River 官方文档，未安装或执行组件。存储及根规则已检查，无需更新。

| 核验 | 实际结果 |
| --- | --- |
| `python3 scripts/repo_policy.py repository` | 22 个文件通过，仅仓库治理 |
| `git diff --check`、`git diff --cached --check` | 通过；编辑前副本用于审查未跟踪文档差异，Markdown 空白、代码块、本地链接及锚点检查通过 |
| vault 事务与回读 | `oj-smtp-choice-20261025` inspect/apply 完成；11 个目标哈希、22 份本地来源哈希一致 |
| vault 严格 lint | `lint --strict --as-of 2026-10-25 --format json`：19 页、215 链接、0 问题；新增断言与用户原文一致 |

知识回写保存 514 字节必要原文 `.raw/captured/2026-10-25-smtp-user.md`，更新既有来源、决策、实体、推进、热点、索引、日志及账本，旧 raw 不变；可恢复事务与结果位于忽略目录 `.local/smtp-vault-transaction.json`、`.local/smtp-vault-result.json`。当前 `codex/engineering-baseline` 工作区未提交，原有暂存内容保留；无依赖安装、应用实现、真实投递、故障/并发/恢复测试、压测、提交推送或部署，无独立人工审查。

## 2026-10-25：注册、找回与比赛发号流程确认

- 用户确认日常邮箱验证注册＋密码登录＋邮箱自助找回，正式比赛校内发号＋人工核验重置；补充域名/DNS 与可用 SMTP 暂不确定。更新会话设计 v0.5、总体架构 v0.15、需求 v0.11、ADR-001、README、前端后续事项及 CI/CD；新增 AC25，明确验证前权限、令牌单次消费、改密与撤销、限流防枚举、邮件故障及断外网边界。
- 邮件服务、Go 发信组件与异步投递仍为候选；只读核对阿里云邮件推送、Resend、go-mail 和 Go net/smtp 文档，记录可替换 SMTP 适配器建议及准入条件。未选择供应商、修改 DNS、采购或发送邮件。存储和根规则已检查，无需更新。

| 核验 | 实际结果 |
| --- | --- |
| `python3 scripts/repo_policy.py repository` | 22 个文件通过，仅仓库治理 |
| `git diff --check`、`git diff --cached --check` | 通过；编辑前副本用于审查未跟踪文档，Markdown 空白、代码块、本地链接及锚点检查通过 |
| vault 事务与回读 | `oj-email-flow-20261025` inspect/apply 完成；11 个目标哈希、21 份本地来源哈希一致 |
| vault 严格 lint | `lint --strict --as-of 2026-10-25 --format json`：19 页、209 链接、0 问题；新增用户选择断言与原文一致 |

知识回写保存 724 字节必要原文 `.raw/captured/2026-10-25-email-flow-user.md`，更新既有来源、决策、实体、推进、热点、索引、日志及账本，旧 raw 不变；可恢复事务与结果位于忽略目录 `.local/email-vault-transaction.json`、`.local/email-vault-result.json`。当前 `codex/engineering-baseline` 工作区未提交，原有暂存内容保留；无依赖安装、应用实现、实际投递、离线/并发/恢复测试、压测、提交推送或部署，无独立人工审查。

## 2026-10-25：Argon2id 密码保存路线确认

- 用户确认 Argon2id＋每个密码独立随机盐＋可升级参数记录，具体参数等硬件确定后验证。更新会话设计 v0.4、总体架构 v0.14、需求 v0.10、ADR-001、README、前端后续事项及 CI/CD；新增 AC24，记录严格解析、参数上限、旧参数升级、并发重置保护和多 API 密码计算预算。没有固定未经验证的成本参数或批准具体 Go 依赖。
- 下一步讨论注册、找回与账号发放；邮箱验证/自助重置等仅为建议，未记为已确认。会话期限和比赛账号规则不变；存储与根规则已检查，无需更新。

| 核验 | 实际结果 |
| --- | --- |
| `python3 scripts/repo_policy.py repository` | 22 个文件通过，仅仓库治理 |
| `git diff --check`、`git diff --cached --check` | 通过；以编辑前副本审查未跟踪文档差异，Markdown 空白、代码块、本地链接及锚点检查通过 |
| vault 事务与回读 | `oj-argon2id-choice-20261025` inspect/apply 完成；11 个目标哈希、20 份本地来源哈希一致 |
| vault 严格 lint | `lint --strict --as-of 2026-10-25 --format json`：19 页、202 链接、0 问题；新增用户选择断言与原文一致 |

知识回写保存 569 字节必要原文 `.raw/captured/2026-10-25-argon2id-user.md`，更新既有来源、决策、实体、推进、热点、索引、日志及账本，旧 raw 不变；可恢复事务与结果位于忽略目录 `.local/argon2-vault-transaction.json`、`.local/argon2-vault-result.json`。只读查阅 OWASP 找回密码和身份认证资料；未接入邮件、短信或第三方认证。当前 `codex/engineering-baseline` 工作区未提交，原有暂存内容保留；无依赖安装、应用实现、密码基准、并发/恢复测试、压测、提交推送或部署，无独立人工审查。

## 2026-10-25：SCS＋go-redis 优先验证路线确认

- 用户确认优先验证 SCS＋go-redis、保留 Gin sessions 为备选；更新会话设计 v0.3、总体架构 v0.13、ADR-001、README、需求状态、前端后续事项与 CI/CD。明确 Gin 适配、凭证摘要存储、期限和身份隔离、跨副本名额、撤销/迟到保存、连接预算及故障恢复为准入门槛，未报告正式采用或测试通过。
- 产品期限和比赛账号规则未变，AC23 不变；需求版本保持 v0.9，仅同步验证路线。下一步讨论密码保存与校验，Argon2id 等仅为建议；存储和根规则已检查，无需更新。

| 核验 | 实际结果 |
| --- | --- |
| `python3 scripts/repo_policy.py repository` | 22 个文件通过，仅仓库治理 |
| `git diff --check`、`git diff --cached --check` | 通过；以编辑前副本审查未跟踪文档差异，Markdown 空白、代码块、本地链接及锚点检查通过 |
| vault 事务与回读 | `oj-scs-validation-20261025` inspect/apply 完成；11 个目标哈希、19 份本地来源哈希一致 |
| vault 严格 lint | `lint --strict --as-of 2026-10-25 --format json`：19 页、195 链接、0 问题；新增断言与用户原文一致 |

知识回写保存 551 字节必要用户原文 `.raw/captured/2026-10-25-scs-user.md`，更新既有来源、决策、实体、推进、热点、索引、日志和账本，旧 raw 不变；可恢复事务与结果位于忽略目录 `.local/scs-vault-transaction.json`、`.local/scs-vault-result.json`。本轮只读核对 OWASP 密码存储及 Go Argon2/bcrypt 文档，未锁定密码算法/参数或引入依赖。当前 `codex/engineering-baseline` 工作区未提交，原有暂存内容保留；无安装、应用实现、SCS/Redis 运行验证、密码基准、压测、提交推送或部署，无独立人工审查。

## 2026-10-25：登录期限、多会话与比赛账号规则确认

- 用户接受推荐的分场景期限、普通三会话、正式线下比赛单参赛会话、拒绝二次登录/工作人员换机及三类比赛身份规则；普通 2 小时/12 小时、保持登录 24 小时/7 天、管理 30 分钟/8 小时为首期初值，赛后查看窗口以 30 分钟作为可配置初值。比赛结束立即停交，历史成员名单与成绩保留。
- 更新会话设计 v0.2、总体架构 v0.12、需求 v0.9、ADR-001、README、前端后续事项及 CI/CD，细化 AC23 的期限、并发名额、换机、延赛与赛后边界测试。存储与根规则已检查，无需更新。会话库/客户端进入比较：SCS＋go-redis 建议优先验证，Gin sessions 备选，均未获用户选定或正式准入。

| 核验 | 实际结果 |
| --- | --- |
| `python3 scripts/repo_policy.py repository` | 22 个文件通过，仅仓库治理 |
| `git diff --check`、`git diff --cached --check` | 通过；以编辑前副本审查未跟踪文档差异，Markdown 空白、代码块、本地链接及锚点检查通过 |
| vault 事务与回读 | `oj-session-policy-20261025` inspect/apply 完成；11 个目标哈希、18 份本地来源哈希一致 |
| vault 严格 lint | `lint --strict --as-of 2026-10-25 --format json`：19 页、189 链接、0 问题；用户规则断言与确认原文及已标明的合成语境一致 |

知识回写保存 1,627 字节必要确认及推荐语境 `.raw/captured/2026-10-25-session-policy-user.md`，更新既有来源、决策、实体、推进、热点、索引、日志及账本，旧 raw 不变；可恢复事务和结果在忽略目录 `.local/session-policy-vault-transaction.json`、`.local/session-policy-vault-result.json`。只读查阅 SCS、goredisstore、Gin sessions 和 go-redis 官方项目文档，具体版本、许可证/安全公告和兼容性仍须正式准入检查。当前 `codex/engineering-baseline` 工作区未提交，原有暂存内容保留；无依赖安装、应用实现、会话/Redis 运行测试、压测、提交推送或部署，无独立人工审查。

## 2026-10-25：Session 与 Redis 登录方案确认

- 用户指定 Redis，其余沿用推荐，确定服务端 Session＋Redis 共享会话＋HttpOnly Cookie。新增会话设计，记录跨副本共享、当前权限检查、续期与撤销竞态、Cookie/CSRF、Redis 故障与恢复防旧会话复活的要求；具体库、期限、多设备及部署参数仍待定。
- 同步总体架构 v0.11、需求 v0.8（新增 AC23）、存储 v0.2、README、ADR-001、前端后续事项和 CI/CD 规划；移除首期不引入 Redis 的旧建议。Redis 本轮仅确定会话用途，判题任务与业务事实仍持久化；PostgreSQL、sqlc＋pgx、go-judge 保持优先验证状态。根规则已检查，无需更新。

| 核验 | 实际结果 |
| --- | --- |
| `python3 scripts/repo_policy.py repository` | 22 个文件通过，仅仓库治理 |
| `git diff --check`、`git diff --cached --check` | 通过；以编辑前副本审查未跟踪文档差异，Markdown 空白、代码块、本地链接与标题锚点检查通过 |
| vault 事务与回读 | `oj-session-redis-20261025` inspect/apply 完成；11 个目标哈希、17 份本地来源哈希一致 |
| vault 严格 lint | `lint --strict --as-of 2026-10-25 --format json`：19 页、182 链接、0 问题；新增断言与用户原文及已标注合成的推荐语境一致 |

知识回写保存 807 字节必要原文与语境 `.raw/captured/2026-10-25-session-redis-user.md`，更新既有来源、决策、实体、推进、热点、索引、日志和账本，旧 raw 不变。可恢复事务与结果位于忽略目录 `.local/session-vault-transaction.json`、`.local/session-vault-result.json`。本轮查阅 Redis 持久化、复制、淘汰及安全官方资料；组件资料不能证明本项目运行效果。当前 `codex/engineering-baseline` 工作区未提交，原有暂存内容保留；未安装依赖、实施应用、运行登录/恢复/压测、提交推送或部署，无独立人工审查。

## 2026-10-25：Goose 数据库迁移方案确认

- 用户确认 Goose＋SQL 迁移文件＋独立迁移步骤＋显式迁移锁。总体架构更新为 v0.10，新增第 8.1.1 节，记录 SQL 历史与哈希、sqlc 共用模式来源、独立发布顺序、显式同锁标识/专用连接、事务内与事务外失败恢复、回退边界和离线验收。
- 同步 README、ADR-001、前端与沙箱文档的后续事项及 CI/CD 规划；需求验收与根规则已检查，无新增产品范围或规则修改。PostgreSQL、sqlc＋pgx 和 go-judge 仍为优先验证方向；登录会话为下一步讨论，未定版。

| 核验 | 实际结果 |
| --- | --- |
| `python3 scripts/repo_policy.py repository` | 21 个文件通过，仅仓库治理 |
| `git diff --check`、`git diff --cached --check` | 通过；以编辑前副本审查未跟踪文档差异，Markdown 空白、代码块和本地标题链接检查通过 |
| vault 事务与回读 | `oj-goose-choice-20261025` inspect/apply 完成；11 个目标哈希、16 份本地来源哈希一致 |
| vault 严格 lint | `lint --strict --as-of 2026-10-25 --format json`：19 页、175 链接、0 问题；新增断言与用户确认原文一致 |

知识回写保存 610 字节必要原文及语境 `.raw/captured/2026-10-25-goose-user.md`，更新既有来源、决策、实体、推进、热点、索引、日志及账本，旧 raw 不变。可恢复事务及结果在忽略目录 `.local/goose-vault-transaction.json`、`.local/goose-vault-result.json`。当前 `codex/engineering-baseline` 工作区未提交，原有暂存内容保留；未安装、生成代码、执行迁移、运行数据库/锁/恢复测试、提交推送或部署，无独立人工审查。

## 2026-10-25：工具链与数据库访问层确认

- 用户确认 GCC 15＋C17、GNU++17 默认和 GNU++20 可选、Temurin JDK 21、CPython 3.14；细化语言配置、环境版本追溯、赛时冻结、两种 C++ 模式及多节点/离线回归要求。补丁与构建版本、命令模板和资源限制尚未验证。
- 用户选择优先验证 sqlc＋pgx，GORM 保留为备选；记录显式 SQL 与生成流程的取舍、模块边界、事务/认领/断连测试和连接池总预算，迁移工具待选。PostgreSQL 与 go-judge 仍为优先验证方向。
- 同步总体架构 v0.9、前端与沙箱选型 v0.7、需求 v0.7、ADR-001、README 和 CI 规划；未修改根规则。当前为 `codex/engineering-baseline` 未提交工作区，原有暂存内容保留。

| 核验 | 实际结果 |
| --- | --- |
| `python3 scripts/repo_policy.py repository` | 21 个文件通过，仅仓库治理 |
| `git diff --check`、`git diff --cached --check` | 通过；以编辑前副本审查未跟踪文档差异，Markdown 空白、代码块与本轮锚点检查通过 |
| vault 事务与回读 | `oj-toolchain-data-access-20261025` inspect/apply 完成；11 个目标哈希、15 份本地来源哈希一致 |
| vault 严格 lint | `lint --strict --as-of 2026-10-25 --format json`：19 页、168 链接、0 问题；两条新增意图断言与用户原文一致 |

知识回写保存 827 字节必要用户原文及语境 `.raw/captured/2026-10-25-toolchain-data-access-user.md`，更新既有来源、决策、实体、进度、热点、索引、日志及账本。可恢复事务与结果为忽略目录 `.local/toolchain-vault-transaction.json`、`.local/toolchain-vault-result.json`；旧 raw 不变。只读核对 sqlc、pgx 和 GORM 官方资料；sqlc stable 入口发生重定向，引用具体文档页，不据此锁定依赖版本。无依赖安装、代码生成、应用实现、Linux/数据库运行测试、压测、提交推送或部署，无独立人工审查。下一步比较数据库迁移工具。

## 2026-10-25：文件存储与首批判题语言确认

- 用户确认数据库保存小数据、校内本地磁盘保存大文件、Worker 本地缓存，并预留对象存储接口；新增 `docs/architecture/storage-design.md`，记录数据归属、文件发布与数据库引用的一致性、缓存权限与清理、备份恢复及后续迁移边界。
- 首期 API 多副本共用同一可信主机的持久化目录，Worker 可跨主机通过授权接口取材；API 跨主机扩容前须统一文件访问。同机负载均衡不代表主机高可用，硬件和容量待验证。
- 首批语言确认为 C/C++、Java、Python 3，PyPy 后续评估；编译器、解释器、标准、版本和资源限制仍待验证。同步总体架构 v0.8、前端与沙箱选型 v0.6、需求 v0.6（新增 AC22）、ADR-001、README、CI 规划与根规则项目事实；PostgreSQL 和 go-judge 保持优先验证状态。

| 核验 | 实际结果 |
| --- | --- |
| `python3 scripts/repo_policy.py repository` | 21 个文件通过，仅仓库治理 |
| `git diff --check`、`git diff --cached --check` | 通过；编辑前副本与未跟踪文档差异已审查，空白及代码块闭合检查通过 |
| vault 事务与回读 | `oj-storage-languages-20261025` inspect/apply 完成；11 个目标、14 份本地来源哈希一致 |
| vault 严格 lint | `lint --strict --as-of 2026-10-25 --format json`：19 页、160 链接、0 问题；两条新增决策断言与用户原文一致 |

知识回写新增 822 字节必要用户原文及语境 `.raw/captured/2026-10-25-storage-languages-user.md`，更新既有来源、决策、实体、推进、热点、索引、日志及账本。恢复草稿与结果位于忽略目录 `.local/storage-vault-transaction.json`、`.local/storage-vault-result.json`。当前为 `codex/engineering-baseline` 未提交工作区，原有暂存内容保留；无应用实现、安装、提交推送或部署。文件一致性、备份恢复、沙箱隔离、语言兼容性与容量均未进行运行验证，无独立人工审查。

## 2026-10-25：首期浏览器本地代码草稿

- 用户确认暂时只支持浏览器本地保存代码草稿；同步总体架构 v0.7、选型 v0.5、需求 v0.5、ADR-001、README 和根规则项目事实。AC21 明确当前浏览器刷新恢复、保存失败可见与账号/题目/比赛隔离，首期无服务器草稿同步或跨设备恢复。
- 正式提交的服务端持久化不变，出题人题目草稿不受此决定影响；本地存储 API、保存频率、大小上限、退出清理和多标签冲突策略待设计。未修改权限或 Git 流程，未安装、实现应用、提交推送或部署。

| 核验 | 实际结果 |
| --- | --- |
| `python3 scripts/repo_policy.py repository` | 20 个文件通过，仅仓库治理 |
| `git diff --check`、`git diff --cached --check` | 通过；编辑前副本与未跟踪文档差异已审查，空白及代码块闭合检查通过 |
| vault 事务与回读 | `oj-local-draft-scope-20261025` inspect/apply 完成；11 个目标、13 份本地来源哈希一致 |
| vault 严格 lint | `lint --strict --as-of 2026-10-25 --format json`：19 页、150 链接、0 问题；新增首期范围断言与用户原文一致 |

知识回写新增 493 字节必要原文及语境 `.raw/captured/2026-10-25-local-draft-user.md`，更新现有来源、实体、决策、推进、热点、索引、日志及账本。原始资料只增不改；恢复草稿与结果位于忽略目录 `.local/draft-vault-transaction.json`、`.local/draft-vault-result.json`。应用功能、浏览器恢复与并发均未验证；无独立人工审查。

## 2026-10-25：CodeMirror 6 编辑器确认

- 用户在 CodeMirror 6 与 Monaco 比较后选择 CodeMirror；更新总体架构 v0.6、选型 v0.4、ADR-001、README、需求与根规则项目事实。基础编辑验收 AC21 不变，草稿保存、语言扩展、React 封装、包管理器及具体依赖版本未定。
- 保留 Monaco 比较背景，说明按需组合的理由及应用集成代价；不将选型视为依赖准入、包体积或性能验证。无安装、应用实现、浏览器测试、提交、推送或部署，原有暂存内容保留。

| 核验 | 实际结果 |
| --- | --- |
| `python3 scripts/repo_policy.py repository` | 20 个文件通过，仅仓库治理 |
| `git diff --check`、`git diff --cached --check` | 通过；另外以编辑前副本审查未跟踪文档差异，空白与代码块闭合检查通过 |
| vault 事务及回读 | `oj-codemirror-choice-20261025` inspect/apply 成功；11 个目标哈希与 12 份本地来源哈希一致 |
| vault 严格 lint | `lint --strict --as-of 2026-10-25 --format json`：19 页、140 链接、0 问题；新增用户选型断言与原文及紧邻比较语境一致 |

知识回写新增 422 字节必要原文及语境 `.raw/captured/2026-10-25-codemirror-user.md`，更新既有技术决策、来源、实体、推进、索引、热点、日志和账本；旧 raw 不变。可恢复事务与结果为忽略目录中的 `.local/codemirror-vault-transaction.json`、`.local/codemirror-vault-result.json`。尚无应用验证或独立人工审查。

## 2026-10-25：Tailwind CSS＋shadcn/ui 与基础编辑范围确认

- 用户选择 Tailwind CSS＋shadcn/ui；同步总体架构 v0.5、选型 v0.3、ADR-001、README 与根规则中的项目事实，原 Ant Design 推荐保留为比较背景，不作为首期主组件体系。
- 用户确认首版基础在线编辑＋文件上传，支持高亮、缩进、快捷键和草稿保存；需求 v0.4 新增 AC21。CodeMirror/Monaco、草稿保存位置与跨设备范围、包管理器和版本尚未决定。
- 本轮仅文档与知识同步；无应用实现、安装、浏览器测试、压测、提交或部署，既有暂存内容保留。根规则仅更新事实，不改变权限或流程。

| 核验 | 实际结果 |
| --- | --- |
| `python3 scripts/repo_policy.py repository` | 20 个文件通过，不代表应用验证 |
| `git diff --check`、`git diff --cached --check` | 通过；以编辑前副本审查未跟踪文档差异，空白及代码块闭合检查通过 |
| vault 事务与证据回读 | `oj-ui-editor-choice-20261025` inspect/apply 完成；11 个目标哈希、11 份本地来源哈希一致 |
| vault 严格 lint | `lint --strict --as-of 2026-10-25 --format json`：19 页、133 链接、0 问题；新增两条用户意图断言与原文核对一致 |

知识回写新增 628 字节必要用户原文 `.raw/captured/2026-10-25-ui-editor-user.md`，原位更新技术决策、产品来源、实体、推进、热点、索引、日志和账本。事务草稿与结果保留于忽略目录 `.local/ui-vault-transaction.json`、`.local/ui-vault-result.json`，旧 raw 未修改。

编辑器补充调研：CodeMirror 官网直接读取仍返回 403，GitHub README 确认其项目迁移入口，官方站点搜索索引可读到扩展配置摘要；Monaco README 描述其与 VS Code 的关系。以上均不是当前版本集成、完整功能或性能验证，未锁定编辑器。尚无独立人工审查。

## 2026-10-25：React＋TypeScript＋Vite 前端确认

- 用户确认采用 React＋TypeScript＋Vite；更新总体架构 v0.4、前端与沙箱选型 v0.2 及 ADR-001，保留 Next.js 比较依据，明确静态资源＋Go API 的职责和取舍。UI 组件库、编辑器、包管理器及版本仍待定；用户要求先进一步解释 UI 方案区别。
- 同步 README、需求、CI 规划与 AGENTS.md 的项目事实；未改变权限、Git 或验收规则。既有暂存内容保留，当前为 `codex/engineering-baseline` 未提交工作区。
- 无应用实现、依赖安装、应用构建、性能测试、提交、推送或部署；数据库任务表和 go-judge 仍只确认优先验证方向。

| 核验 | 实际结果 |
| --- | --- |
| `python3 scripts/repo_policy.py repository` | 20 个文件通过；仅仓库治理检查 |
| `git diff --check`、`git diff --cached --check` | 通过；另外以编辑前副本审查未跟踪文档差异，并检查行尾空白与代码块闭合 |
| 状态语义复核 | 当前前端路线改为已确认；历史比较保留，UI 候选未提升为决策，未把选型确认视为容量或离线验收通过 |
| vault 事务及回读 | `oj-react-vite-choice-20261025` inspect/apply 成功；11 个事务目标、10 份本地来源哈希一致 |
| vault 严格 lint | `lint --strict --as-of 2026-10-25 --format json`：19 页、124 链接、0 问题；另核对新增断言与用户原文一致 |

知识回写新增 434 字节必要用户原文 `.raw/captured/2026-10-25-react-vite-user.md`，更新现有技术决策、项目实体、产品来源、推进、热点、索引、日志及账本；原始资料只增不改，不新增重复规范页。恢复草稿与结果为被忽略的 `.local/frontend-vault-transaction.json`、`.local/frontend-vault-result.json`。未进行独立人工审查；应用运行与容量仍待实现后验证。

## 2026-10-25：总体结构确认与前端、沙箱比较

- 用户确认模块化单体＋独立判题 Worker，硬件后补；选择优先验证 go-judge，isolate 保留为备选；前端继续比较，未定版。
- 新增 `docs/architecture/frontend-sandbox-options.md`，基于协会官网与博客的固定提交比较 React＋Vite、Next.js 静态导出和服务端模式，记录资源、负载均衡、校内离线边界及沙箱验证计划。同步 README、ADR-001、总体架构 v0.3 与需求中的选型状态。
- 通过 GitHub 只读接口核对官网 `e6f8be13c6f76593ef852381df7935acf9606d28`、博客 `fd181804b14ab6f5cab869cde986aa3ccabd6b51` 的依赖、配置及说明；未运行它们的构建或将其视为 OJ 容量证明。
- 当前为 `codex/engineering-baseline` 未提交工作区，原有暂存内容保留；无应用实现、依赖安装、提交、推送或部署。

| 核验 | 实际结果 |
| --- | --- |
| `python3 scripts/repo_policy.py repository` | 20 个文件通过；不验证应用行为 |
| `git diff --check`、`git diff --cached --check` | 通过；另核对未跟踪文档空白、代码块闭合，并以编辑前副本审查修改 |
| 文档语义复核 | 前端待定、沙箱优先验证与正式采用分开；静态资源/API/判题容量分开；进程正常退出不等于答案正确 |
| vault 事务及回读 | `oj-structure-selection-20261025` inspect/apply 成功；11 个目标哈希、9 份已登记本地来源哈希一致 |
| vault 严格 lint | `lint --strict --as-of 2026-10-25 --format json`：19 页、119 链接、0 问题；另核对用户意图与原文 |

知识回写新增 1,153 字节必要原文 `.raw/captured/2026-10-25-structure-frontend-sandbox-user.md`，更新既有产品需求来源、技术决策、项目实体、推进、索引、热点、日志及账本。仅确认用户的结构选择与验证顺序，不将候选组件技术效果登记为已验证。可恢复事务和结果位于被忽略的 `.local/selection-vault-transaction.json`、`.local/selection-vault-result.json`。

未验证：前端原型、真实并发容量、Linux 沙箱权限/隔离/语言兼容性及故障恢复；无独立人工审查。下一步继续明确公开题库的搜索收录与首屏要求，决定前端路线，并在 Linux 条件具备后实施沙箱实验。

## 2026-10-25：负载均衡、高并发与资源预算补充

- 用户要求负载均衡、高并发与节约学校资源；资源可能有限为规划前提，实际规格待询问。未据此将微服务、代理组件、硬件规格或高可用方案标为已确认。
- 总体架构更新至 v0.2，增加 API 多副本分流与按节点容量领取判题任务、共享状态与幂等、连接/CPU/内存/磁盘预算、有限队列、过载降级和按瓶颈扩容；修订架构图以明确 API 副本和共享事实。
- 需求更新至 v0.3，新增 AC19 多副本与故障恢复、AC20 资源预算与过载验收；同步 README、CI/CD 验收要求。Nginx 与 PostgreSQL 官方说明仅支持组件能力，未作为本项目性能证明。
- 当前 `codex/engineering-baseline` 未提交工作区；原有暂存内容保留。仅文档变更，无依赖安装、应用实现、提交、推送或部署。

| 核验 | 实际结果 |
| --- | --- |
| `python3 scripts/repo_policy.py repository` | 19 个文件通过，含未跟踪文档与本地相对链接；不验证应用 |
| `git diff --check`、`git diff --cached --check` | 通过；另外用编辑前副本审查本轮未跟踪文档差异，空白与代码块闭合检查通过 |
| 文档语义复核 | 负载均衡不等于主机高可用；同机多副本不增加物理算力；资源受限与容量未测保持明确；新增 AC19/20 未降低 AC16 |
| vault 事务与回读 | `oj-resource-constraints-20261025` inspect/apply 成功；10 个目标哈希、8 份已登记本地来源哈希一致 |
| vault 严格 lint | `lint --strict --as-of 2026-10-25 --format json`：19 页、112 链接、0 问题；另核对需求断言与用户原文一致 |

知识回写新增一份 533 字节必要原文 `.raw/captured/2026-10-25-resource-requirements.md`，更新项目实体、产品需求来源、推进、索引、热点、日志和来源/断言账本。仅保存已确认需求，技术建议仍待讨论及验证。事务与结果保留于被忽略的 `.local/resource-vault-transaction.json`、`.local/resource-vault-result.json`。

未验证：应用吞吐、时延、过载恢复、节点资源限制、入口故障切换与实际学校容量；未开展独立人工审查。下一步取得硬件清单、定义资源预算与可接受指标，再执行单/多副本和混合负载实验。

## 2026-10-25：总体架构讨论稿与校内独立运行要求

- 新增 `docs/architecture/system-design.md` v0.1：比较模块化单体与微服务，明确模块数据归属、提交/租约/重判协议草案、派生视图、隔离边界、校内部署和分期验证门槛。
- 用户确认先训练判题、再完成校赛；正式校赛须在校内断外网运行；学校硬件条件有待询问。用户选择优先验证 PostgreSQL＋持久化任务表，不等于正式采用或已验证。总体业务结构仍待讨论。
- 需求更新为 v0.2，新增 AC18 断外网验收；同步 README、ADR-001 与 CI/CD 文档。当前处于 `codex/engineering-baseline` 未提交工作区，保留原有暂存内容；未修改应用或治理代码、安装依赖、提交、推送或部署。
- 查阅 PostgreSQL 约束/锁定查询、RabbitMQ 可靠性和 go-judge 官方说明，引用位于架构稿对应段落。资料仅用于候选设计依据；未完成依赖准入、Linux 验证或容量测试。

| 检查 | 实际结果与限制 |
| --- | --- |
| `python3 scripts/repo_policy.py repository` | 19 个文件通过；覆盖工作树中的未跟踪文档与相对文件链接，不代表应用验证 |
| `git diff --check`、`git diff --cached --check` | 通过；另对本轮未跟踪文档执行空白与代码块闭合检查，并与编辑前副本审查差异 |
| 文档语义复核 | 已确认、优先验证、建议与未知分开；新增 AC18；未将硬件、业务结构或组件建议写成已批准或已验证 |
| vault 回写后严格 lint | `lint --strict --as-of 2026-10-25 --format json`：19 页、106 链接、0 问题；结构通过不代替事实核验 |
| vault 回读与来源复核 | 11 个事务目标哈希一致、7 份已登记本地来源哈希一致；必要用户回复与三条新增意图断言逐项对照 |

知识库事务 `oj-architecture-discussion-20261025` 已应用：新增 `.raw/captured/2026-10-25-architecture-user-excerpts.md`，原位更新产品需求来源、项目实体、Go + Gin 决策、开发推进、索引、热点、日志及账本。旧原始来源未修改；仅将用户需求和优先验证方向登记为 accepted，不将技术效果或总体业务结构升级为已确认。

事务首次预检发现时间戳格式和来源 ID 不符合核心契约，修正后 inspect/apply 成功。可恢复草稿与结果保留于被忽略的 `.local/architecture-vault-transaction.json`、`.local/architecture-vault-result.json`。本轮无应用测试、压测、部署演练或独立人工审查；待讨论结构、取得硬件资料并确定语言/前端后，另行实施验证。

## 2026-10-25：技术决策与开发治理基线

- 用户确认 Go + Gin 自研，参考 Hydro / DOMjudge；记录设计借鉴和未决选型。
- 新增开发/Git、CI/CD 规范、PR 模板、治理脚本、三个 Git hooks 和治理流水线；更新根规则、README 及需求中的选型状态。
- 不含 OJ 应用实现；没有提交、推送、PR、远端保护或部署操作。初始用户文件均为未提交工作区状态。
- 实测环境：macOS 15.7.3 arm64、Python 3.13.7；本轮不安装依赖。

| 检查 | 实际结果 |
| --- | --- |
| `python3 -B -m unittest discover -s tests/governance -v` | 22 项通过；4.051 秒。临时仓库真实提交与临时本地裸仓库推送也在覆盖范围内，不访问项目远端 |
| `python3 scripts/repo_policy.py repository` | 18 个工作区文件通过；包括未跟踪文件 |
| `python3 scripts/repo_policy.py bootstrap-ci` | 通过；确认当前仅文档/治理，无应用验证声明 |
| 逐个 `sh -n` 检查安装器及三个 hooks | 全部通过 |
| Ruby 标准库解析 workflow YAML | 语法通过；不能替代 GitHub Actions 实际运行 |
| 未跟踪文件空白检查、`git diff --check` | 通过；仓库尚无提交，后者单独不能覆盖未跟踪内容 |
| `sh scripts/install-hooks.sh` 与回读配置 | 已安装，当前克隆 `core.hooksPath=.githooks`；不影响全局 |
| 在当前 main 调用 pre-commit | 按预期返回 1 并拒绝提交；没有执行项目提交 |
| vault 回写前严格 lint | 16 页、75 链接、0 问题；结构与来源账本检查通过 |

尚未验证：Linux runner、GitHub Actions 远端运行、远端分支保护、应用功能/性能/隔离、部署与恢复。

### 知识库与最终核验

- 指定 vault：`../OJ_Knowledge`；事务 `oj-engineering-baseline-20261025` 已完成。
- 新增 `wiki/decisions/go-gin-self-development.md`、`wiki/decisions/engineering-workflow.md`、`wiki/sources/engineering-baseline-20261025.md`；同步项目实体、推进、热点、索引、日志、来源/断言账本及托管地址清单。
- 新增三份只增不改的 raw 来源，共 71,866 字节：必要用户原文、15 文件工程快照、实际验证摘录。AI 快照与验证摘要均标为 synthetic；用户意图与工程效果分开。
- 事务预检曾拒绝不符合规范的来源 ID 和合成内容分类；按核心校验契约修正后预检、应用均成功。未用直接写文件绕过校验。
- 回写后 `lint --strict --as-of 2026-10-25 --format json`：19 页、98 链接、0 问题，来源账本无错误。
- 回读核对全部 15 个事务目标哈希、6 份已登记本地来源哈希；现有来源保持一致。另行语义复核了选型状态、已实施/未实施边界及用户摘录，未把结构 lint 当作语义证明。
- 恢复草稿与应用结果位于被 Git 忽略的 `.local/engineering-vault-transaction.json` 和 `.local/engineering-vault-result.json`；不要将其当作源代码提交。
- 本轮仅建立工程基线，没有 OJ 应用实现，也没有用户仓库提交、推送或远端配置。首次应用实现仍需真实应用 CI；远端保护与部署需后续授权和环境信息。
