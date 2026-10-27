# 工程交付记录

## 2026-10-27：固定 GNU++17 材料的受限执行编排

唯一目标：P2、AC05/06/17/20 的固定 v2 受限编排。GitHub CLI 已核验 PR #4 合并为 `56b43361db91a98c1353c9342141e255f7ecd56f`，从更新主线创建独立工作区及 `codex/bounded-execution` 分支；原克隆 5 个文档改动保持。以下为本地验证时的未提交状态，后续提交及 CI 以对应 PR 的精确 head 为准。

- 新增 `internal/execution`：单槽 Runner、材料与环境预检、一次编译/按序逐点执行、首次失败停止、结果归因、编译独立/运行峰值统计、制品清理。go-judge REST 适配与编排分离；不新增依赖、SQL、公共 API 或后台任务服务。
- 基线相关实验/文本/材料单测通过。最终 `GOPROXY=off make go-check go-build` 通过格式/vet/race/四命令构建，Make 参数复用原克隆 `.local/go-mod`、`.local/go-build`；macOS arm64、Go 1.26.10。未改数据库/前端，本轮未重跑本地数据库/前端检查；未修改 hooks/CI。
- `OJ_SANDBOX_EXPERIMENT=1 GOPROXY=off make sandbox-test` 通过新增 10 项真实编排子测试与既有 12 项隔离回归；固定 Linux arm64、GCC 15.3.0、go-judge v1.13.3。逐例核验编译次数、首/末点停止和缓存清空，覆盖正常/错误/资源超限、非法 UTF-8 和临时文件隔离。
- 实测首先发现 OLE 响应保留 4097 字节并附 `CollectSizeExceeded`；新增失败回归，核对固定版本源码后修复适配，再通过真实测试。仅接受结构化输出超限证据，文件/未知错误不伪装为选手错误。
- 注入验证材料损坏、环境不匹配、协议缺字段/矛盾、响应超限、取消、容量与清理失败。远端状态不明时 Runner 停用，HTTP 取消不视为 kill 确认。测试驱动使用已有专用无网络/无挂载 VM，尚无生产执行传输或宿主检查器。
- README、M0 记录、首期计划同步。完整 Worker、租约中止确认、真实回报和 M0 整体验收仍未完成；无部署或独立人工审查。后续先做租约中止，再做真实回报。

收尾自审：仓库策略 120 文件及 `git diff --check` 通过，相对 `origin/main` 的改动仅包含本功能及必要测试/文档。专用容器与 VM 已停止，`colima list` 回读 Stopped，Docker context 仍为 desktop-linux。知识回流和远端 CI 另按实际结果记录，不用本地成功代替远端状态。

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

- 指定 vault：`/Users/huangqijun/Documents/OJ_Knowledge`；事务 `oj-engineering-baseline-20261025` 已完成。
- 新增 `wiki/decisions/go-gin-self-development.md`、`wiki/decisions/engineering-workflow.md`、`wiki/sources/engineering-baseline-20261025.md`；同步项目实体、推进、热点、索引、日志、来源/断言账本及托管地址清单。
- 新增三份只增不改的 raw 来源，共 71,866 字节：必要用户原文、15 文件工程快照、实际验证摘录。AI 快照与验证摘要均标为 synthetic；用户意图与工程效果分开。
- 事务预检曾拒绝不符合规范的来源 ID 和合成内容分类；按核心校验契约修正后预检、应用均成功。未用直接写文件绕过校验。
- 回写后 `lint --strict --as-of 2026-10-25 --format json`：19 页、98 链接、0 问题，来源账本无错误。
- 回读核对全部 15 个事务目标哈希、6 份已登记本地来源哈希；现有来源保持一致。另行语义复核了选型状态、已实施/未实施边界及用户摘录，未把结构 lint 当作语义证明。
- 恢复草稿与应用结果位于被 Git 忽略的 `.local/engineering-vault-transaction.json` 和 `.local/engineering-vault-result.json`；不要将其当作源代码提交。
- 本轮仅建立工程基线，没有 OJ 应用实现，也没有用户仓库提交、推送或远端配置。首次应用实现仍需真实应用 CI；远端保护与部署需后续授权和环境信息。
