# CI/CD、环境与发布规范

本页维护现行 CI 行为与未交付的验收要求；命令统一见 [README](../README.md)，每次验证的 head/环境/结果见[交付记录](delivery-log.md)。历史 Make/Colima 命令仅保留于历史证据，不是现行入口。

## 当前状态

本人训练历史和详情随普通会话套件在真实 PostgreSQL 和 Redis 上核对同微秒翻页、最大源码原样返回、任务状态更新、主体及范围隔离、资格撤销、会话退出和关闭开关。可选的 [浏览器专项](../README.md#浏览器账号专项)验证合成记录在「我的提交」页面的桌面和窄屏流程，包括显式打开详情及翻页清除，不要求已发布题目或 Worker。

邮箱闭环在 `integration-tests` 下使用独立的临时 PostgreSQL、Redis 和回环 TLS 收件器验证；普通会话套件保留自己的全新夹具。邮箱专项不发布宿主端口，不启用文件配置中的公开申请，也不连接真实邮件服务。

题面语义确认的临时 PostgreSQL 专项覆盖原键并发、丢失 COMMIT 回执、分配重授、跨送审项退回竞态和 schema 19 空账本回退／非空保护；2,049 行夹具对现行两条 SQL 执行 `EXPLAIN ANALYZE`。v6 A+B 专项验证人工声明与可信渲染同时具备时只解除题面质量阻断。普通会话覆盖直连和 mTLS、关闭开关、坏对象、读完样例时撤销会话及未命中渲染证据。均使用测试审核人，不代表实际人工题意验收。

题面渲染证据专项在 `OJ_V6_STATEMENT_BROWSER_REPORT` 指定原 #151 `browser-qa.json` 时核对摘要、桌面／窄屏及十题内容版本和计数；`OJ_V6_INPUT_EVIDENCE_BUNDLE` 专项独立从实物重算 v6 修订和正式计划。临时 PostgreSQL 验证 A+B 能读证据且保留题面质量阻断；普通会话验证无可信绑定时省略字段。此轮复用相同内容版本的历史 Chromium 证据，没有运行新的浏览器或人工语义验收。

[独立正确性证据](problem-drafts.md#批准准入状态)验证精确绑定、旧 Profile、正式失败和卷积缺对拍／不一致。显式设置 `OJ_V6_INPUT_EVIDENCE_BUNDLE` 的专项复算十题计划、核对原报告中的 198 个独立小用例，并验证只有九道非卷积题的 192 个小用例可用于该项审核。临时 PostgreSQL 专项读取匹配 v6 的 A+B 送审，确认本站正式失败和覆盖缺项仍阻断。普通会话验证无证据时省略字段。

同一 v6 专项将十个错误解拒绝点及三个已记录的前序接受点与冻结计划的原序号、输入和答案摘要核对。PostgreSQL 专项验证指定审核人可读 A+B 对照且正式失败和覆盖阻断保留；普通会话验证未命中可信报告时省略字段。专项仅核对既有制品，不重新执行题源程序。

[输入校验证据](problem-drafts.md#批准准入状态)的普通测试逐项变更绑定、核对旧 Profile 和准入阻断。显式设置 `OJ_V6_INPUT_EVIDENCE_BUNDLE` 的离线 Go 专项从原 v6 题包重算全部十题完整修订和正式计划，并核对 254 个输入与 40 个对照的原报告。卷积 PostgreSQL／Worker mTLS 专项加载 v6 题包时验证命中后仍保留对拍差异阻断；用其他题包验证历史证据不继承。普通会话验证无证据时省略字段。验证不执行题源程序。

[作者确认标程](problem-drafts.md#作者确认标程)的普通会话套件验证本人读取、显式确认、摘要变化、坏对象、原键回读和准入只移除对应阻断；真实 PostgreSQL 集成验证另一作者拒绝、资格撤销重授、同绑定并发、COMMIT 回复丢失、索引查找和非空回退保护。使用受控报告，不执行题源。

[批准准入状态](problem-drafts.md#批准准入状态)的规则测试覆盖正式 AC、卷积对拍缺失／差异／通过、旧 Profile 和未知来源；普通会话套件验证指定审核权限、正式 AC 的阻断及退回后的变化。PostgreSQL 集成验证同材料重复项共享阻断；配置已核验卷积题包时另验证差异证据绑定与摘要错配拒绝。

[审核对拍详情](problem-drafts.md#审核对拍详情)随普通会话套件验证直接 HTTPS、单跳 mTLS 代理、当前分配、撤权和无对拍记录的结果。配置已核验卷积题包的临时 PostgreSQL／Worker mTLS 专项另验证从已接纳的差异回读原输入与两份输出、固定修订及摘要、证据绑定损坏时拒绝；未配置题包时该专项跳过。测试使用受控报告，不运行题源。

[审核退回与重送](problem-drafts.md#审核退回与重送)由普通 PostgreSQL 集成覆盖同材料并发决定、送审与退回锁等待、原键恢复、实际 COMMIT 回包丢失及回退保护。普通会话套件使用真实 Redis 和直接 HTTPS／单跳 mTLS 代理，验证写开关、指定审核人、作者反馈与新键拒绝；合成参考报告不构成批准或发布证据。

[审核人分配与证据摘要](problem-drafts.md#审核人分配与证据摘要)由普通 PostgreSQL 集成验证最新资格与分配、真实锁等待、回退保护和查询计划。普通会话套件在直接 HTTPS 及单跳 mTLS 代理入口使用真实 Redis 登录、临时 PostgreSQL 和合成报告，验证指定非作者的[审核材料](problem-drafts.md#审核材料读取)，并覆盖读取期间授权变化及坏对象。普通后端测试核对素性测试的固定依赖选择。没有运行题源或批准流程。

参考领取的普通验证使用受信文件锁及 mTLS 客户端；临时 PostgreSQL 集成覆盖节点凭据隔离、原授权与预期 Plan 核验、单预留竞争、真实 COMMIT 回复丢失以及 Down11 交错。没有运行参考题源或判题沙箱。

同机执行槽的普通后端测试使用私有临时目录、受信测试子进程和实际文件锁，验证判题/参考互斥、退出遗留隔离、所选槽恢复和双槽实验边界；不启动 Docker 沙箱。历史 `dual-worker`、`slot-recovery` 专项显式启用双槽模式，保留真实沙箱的独立验收边界。

[持久化参考验证申请](reference-requests.md)的可信目录、冻结配置副本、新鲜会话身份及取消/未知回执边界归普通backend单元测试；真实临时PostgreSQL用例归integration，覆盖owner隔离、同键恢复、并发最后额度、policy等待后的原键复查、取消竞争、撤权锁、事务/COMMIT回包故障、历史摘要完整性、权威 SQL 查询计划及 schema9 回退；schema10 验证冻结后二次会话观察、授权修订/取消竞争、计划幂等及未知回执、非空历史回退拒绝。服务与真实PG组合用例使用合成会话观察，不代表申请能力的真实Redis/HTTP、Worker消费或大材料验收；草稿not-run、生产配置null及发布阻断保持。执行结果以本次head的[交付记录](delivery-log.md)为准。

convolution_mod的完整材料/16MiB输入答案、共享快照、封闭tier8及双端2GiB账本、独立槽与错Cmd/跨档位拒绝归普通backend。`TestConvolutionReferenceSandbox`仅在sandbox标签、Darwin arm64、`OJ_CONVOLUTION_REFERENCE=1`与实验opt-in下运行同次16MiB输入/输出边界、窄OLE证据及完整53点；失败或Stop未确认即停止。一次十来源254点/26子测试、独立四槽顺序后置、预备新Plan/Profile与旧九完整Plan/Profile核对均通过；普通CI仍不创建槽或执行题源，不替代这次固定环境证据。最终CI/合并状态与证据限制见[交付记录](delivery-log.md#2026-11-04convolution_mod-十来源固定环境接纳)。

`tests/governance/test_problem_sources.py` 验证离线题源工具的来源哈希、完整集合、安全提取、输入对照、正式 WA、CPU 超限证据与输出/清理失败边界，随 governance 执行；不下载或执行外部题源程序。固定独立用例的手算锚点、严格比较和证据拒绝回归归 `tests/governance/test_library_checker_oracles.py`；七份本地候选的语义、输出形状、探测顺序及拒绝证据归 `tests/governance/test_library_checker_wrong_cases.py`。十题完整数据生成、真实 validator/checker、输入对照、十份固定错误解、关联数组低效解 CPU 对照及十题小规模对拍只由 README 的显式本地 Docker 命令运行，不进入普通 CI，也不构成生产 Worker/SPJ 或 P4 发布验收。十题真实授权预览另用 [浏览器账号专项](../README.md#浏览器账号专项) 的 `--authorized-draft-bundle`，真实 API 响应不替换；普通 CI 不启动 Chromium 或导入该私有题包。

[卷积小规模独立对拍](reference-validation.md#卷积小规模独立对拍)的固定语料、严格输出、首个差异、制品清理和停止不明回归归普通 backend，使用替身、不运行外部源码。正例 CLI 和 `TestConvolutionDifferentialNegativeSandbox` 仅在 README 所述专用断网卷积槽运行，负对照需 sandbox 标签、Darwin arm64 和双重显式启用，不进入普通 CI。

已入库卷积草稿的普通 backend 回归使用小型合成材料和真实对象存储，核对完整修订、锁定角色、单槽顺序读取和损坏拒绝。手动 `TestPrivateDraftConfiguredConvolutionEvidence` 在临时 PostgreSQL 中导入实际卷积草稿，再通过独立连接和重新打开的对象根确认私有证据绑定和读取范围；需提供已复验的 `OJ_PRIVATE_DRAFT_BUNDLE_DIR`，两种测试均不执行题源。

同一实物专项另验证新卷积申请的双计划原子提交、候选读取失败零计划、丢失真实 COMMIT 后原键恢复、历史回放及 Down14；未配置题包时普通集成会显式跳过该专项。普通 backend 验证历史对拍计划的规范编码及草稿、Profile、正式计划绑定；普通 PostgreSQL 集成对正式与卷积查询都运行 `EXPLAIN ANALYZE`，验证申请主键查找。

卷积材料下发的固定来源、语料、角色和同长度坏字节拒绝归普通 backend；实物专项使用真实专用 mTLS、临时 PostgreSQL 和私有对象根验证对拍材料下发、原路由隔离及对象 I/O 期间撤权。两者均不执行题源。

十题[实物报告接纳专项](reference-requests.md#十题实物报告接纳专项)只在显式 `live_reference` 集成标签及已配置的 macOS arm64 专用槽运行，普通 CI 不执行题源。命令归 README；合成作者会话与实际 mTLS/Worker/数据库证据分别描述。

组合报告的临时 PostgreSQL/mTLS 专项用受控证据验证正式 AC 缺少对拍或反例非法均拒绝、合法差异原子保存及逐字重放。另用固定断网槽上的 `TestClaimedConvolutionOwnedSandbox` 验证真实 53 正式点和 24 对拍点，以及在进入对拍时注入续期失效。宿主专项不进入普通 CI；两类结果不能互相代替。

`internal/judging`的独立Library Checker文本组件及M0回归随backend执行；`tests/checkercompat`仅在显式`checkercompat`标签和README专用断网/只读/非root Docker中运行锁定上游wcmp，不进入普通CI。组件差分不改草稿not-run或代替本站受限程序验证；支持域及预算见[准入约定](checker-admission.md)。

`internal/problemvalidation`及Worker维护命令的材料/顺序/失败/取消、启动所有权与清理回归归普通backend；`cmd/worker/validation_sandbox_test.go`仅在显式sandbox标签、独立宿主测试驱动及双重实验opt-in下执行。该真实专项验证A+B全点和故障，不进入普通CI，不创建数据库、不改默认M0 suite，也不更新草稿not-run。结果及限制见[受限参考验证](reference-validation.md)。

停止诊断的原故障矩阵继续核对主错误优先级、独立期限与dirty门禁，并验证六步顺序/到达位置、PID合计次数、耗时截断标记、返回副本及owner映射。可信固定本地shell分别证明过期context下kill调用尝试但CLI未启动、CLI已启动后超时；不执行题源或制造真实Docker停止故障，不据此断定历史根因。固定成功报告JSON golden验证新增字段在成功时完全省略。当前参考执行/传输预算与来源准入保持，历史#114失败不被诊断或普通CI改写；新版本吸收#123后另经一次八来源固定环境接纳通过，最终CI与合并状态见[PR #114](https://github.com/DL-Association-for-Computing-Machinery/OnlineJudge/pull/114)。

Host控制失败五阶段采样、冻结阶段/整体收尾耗时区分、直接本地启动证据、边界/枚举/结构净化、逐层脱敏复制及独立cleanup trace归属随backend执行；每次公开投影仍只检查64槽，裸provider不额外开启类别遍历，八条上限保持。原错误`Is/As`、五profile/成功JSON兼容、真实私有报告写入及Stop成功/失败隔离决策一并验证。Darwin专用workerhost驱动先核对精确清单再运行固定受信命令和替身，不等同沙箱或题源验收；字段及缺失限制见[参考验证](reference-validation.md#执行失败诊断)。#114历史失败保留，新版本已通过独立八来源接纳；#119新版本已通过九来源独立接纳，最终CI与合并状态见[PR #119](https://github.com/DL-Association-for-Computing-Machinery/OnlineJudge/pull/119)。

独立缓存字节通路的MIME/二进制响应、角色账本、配额/预留、未知回执/坏哈希、清理期间取消和真实本地子进程背压回归随backend执行，后者只运行受信任的固定shell命令。`reference_bytes_sandbox_test.go`的48MiB及缓存stdin/stdout实测仅在显式`OJ_REFERENCE_BYTES=1`与sandbox/宿主驱动下运行，普通CI不执行题源或大文件沙箱；M0默认内联路径及既有回归保持。

unionfind完整磁盘快照、Read/Close竞争/配额释放、坏末点零执行、独立profile互斥、诊断/输出分限及collector OLE负例随backend执行；`cmd/worker/unionfind_sandbox_test.go`仅在sandbox标签、darwin/arm64宿主驱动和`OJ_UNIONFIND_VALIDATION=1`下执行完整18点与256KiB真实边界，使用独立reference槽。普通CI不执行此专项，不自动配置/启动新槽，也不据此更新数据库状态或生产预算。

固定来源/封闭Small-Medium-MediumBulk-Primality-StaticRMQ-StaticRangeSum-AssociativeArray-Convolution档位、未知档位零派发、旧profile摘要与Cmd空值兼容、双端累计配额及十来源停止回归归backend；`cmd/worker/medium_reference_sandbox_test.go`只在sandbox标签、darwin/arm64及`OJ_MEDIUM_REFERENCE=1`下运行完整zalgorithm29点、4MiB collector和8MiB文件边界，使用固定medium槽。`reference_source_sandbox_test.go`共用完整CLI校验，并在`OJ_SUFFIXARRAY_REFERENCE=1`及同样宿主/实验前置条件下验证suffixarray全50点，复用同一槽锁，不创建槽。suffixarray超过64MiB实际磁盘冻结/超过128MiB拒绝、共享额度、坏末点零执行和50点最坏输出/累计计费清理归backend；旧四profile摘要保持。普通CI只检查代码，不执行这些真实专项或自动创建/替换任何槽。

primality固定依赖布局、同文件冻结/关闭竞争、聚合64KiB及路径预检、多文件逐个上传失败/未知回执、原始字节回读、六对象额度、共享独立5秒清理/保留exe及第五来源停止矩阵随backend执行；旧四profile摘要保持。`cmd/worker/primality_sandbox_test.go`仅在sandbox标签、darwin/arm64宿主驱动与`OJ_PRIMALITY_REFERENCE=1`及实验opt-in下运行，先验证可信嵌套include，再验证完整13点；首组失败即停止。它复用medium槽锁但保留独立档位，不进入普通CI或创建/替换槽，不代表数据库验证状态、生产预算或P4发布验收。

staticrmq的256MiB材料/16MiB输入上限、真实大于128MiB的合成冻结、共享额度、末点坏哈希/UTF-8/超长token零Verify、双端512MiB累计账本精确边界与清理、27点最坏传输、跨档位/来源拒绝及staticrmq停止矩阵随backend执行；旧八profile摘要保持。`cmd/worker/staticrmq_sandbox_test.go`仅在sandbox标签、darwin/arm64、`OJ_STATICRMQ_REFERENCE=1`和实验opt-in下运行可信16MiB stdin及完整27点，前组（含Stop cleanup）失败即中止。原第9点SE及早期未复现观测作为历史保留；吸收主线前置后，一次九来源201点/24个子测试、完整原Plan/Profile及全部Stop通过，支持新版本固定环境接纳；最终CI与合并状态见[PR #119](https://github.com/DL-Association-for-Computing-Machinery/OnlineJudge/pull/119)，不放宽期限或重试。失败与后续接纳见[交付记录](delivery-log.md)。

static_range_sum的256MiB上限/超过128MiB实际冻结与共享额度、16MiB input与8MiB answer边界、275765609B/73逻辑操作计费、25MiB+1字节存活预留、8MiB原始字节/SIGXFSZ窄证据、额外wire角色下载前拒绝/保留未知预留和第六来源Stop矩阵随backend执行。预期stderr拒绝不能掩盖stdout/input清理失败、取消或截断的合成回归亦归普通backend；旧五profile保持；档位3的point_add_range_sum已通过本轮固定环境接纳，5亦已通过九来源固定环境接纳。`cmd/worker/static_range_sum_sandbox_test.go`仅在sandbox标签、darwin/arm64宿主驱动与`OJ_STATIC_RANGE_SUM_REFERENCE=1`/实验opt-in下运行真实字节边界及完整11点，首组或Stop失败不进入CLI，不进入普通CI或改变槽。首轮槽启动/停止证据失败仍保留；已批准环境改变后的独立新观测与原五来源串行回归通过，不证明旧根因或长期稳定，普通CI不替代这些真实证据。

associative_array的含catalog512MiB精确/+1、超过256MiB实际匿名冻结、48MiB input/8MiB answer精确/+1、993543748B/127操作计费、57MiB+1预留、1GiB两端账本注入边界/已知清理、5s/10s wire参数和第七来源Stop矩阵归普通backend，旧六profile保持。`cmd/worker/associative_array_sandbox_test.go`仅在sandbox标签、darwin/arm64及`OJ_ASSOCIATIVE_ARRAY_REFERENCE=1`/实验opt-in下执行同次48MiB输入→8MiB输出及完整20点；首组含Stop失败则后组不运行。一次两组及后续旧六来源独立串行回归已通过；物理medium槽/锁不变，普通CI不执行该真实专项，不代替固定环境证据、生产容量或发布验收。

MediumBulk的共享槽/原tier保留、两端256/512MiB累计额度及失败清理、132MiB快照/16MiB输入读回与超限拒绝、133次逻辑操作和共享锁互斥归backend。`bulk_reference_sandbox_test.go`仅在sandbox标签、darwin/arm64、`OJ_POINT_ADD_REFERENCE=1`及实验opt-in下运行完整point_add_range_sum21点、8MiB+1/16MiB缓存stdin与阶段资源采样；Runtime、原槽配置和Stop门禁保持。吸收#123后，一次八来源174点、22个原子子测试及全部Stop通过，旧完整Plan/Profile相等；此证据支持新版本固定环境接纳，历史失败仍保留，最终CI与合并状态见[PR #114](https://github.com/DL-Association-for-Computing-Machinery/OnlineJudge/pull/114)。Verify10秒、Compile wall+5、各Run wall+5及独立5秒制品清理已按Runner合同收紧，不提高原预算。

`integration-tests` 在既有 sqlc/真实 PostgreSQL 检查后，构建同一 Docker 工具链并下载固定摘要 Redis 7.2.17，执行 `scripts/test-session-protocol.py --context default`。专项使用私有网络、临时 PG、双客户端/双连接池及 TLS 入口验证正式普通 Redis 协议、文件加载的合成密钥、两步登录与共享准入/限流故障，以及固定单跳 TLS→mTLS 反代的证书、来源和 Cookie 路径，失败传递到原 integration 必需门禁；不开放宿主端口，不持有生产凭据或运行选手代码。命令归 README，隔离和结果归 M0/交付记录；不代表公开 HTTP/SCS、真实浏览器/生产代理、生产阈值或完整恢复验收。

`containers` 在原 Compose 冒烟后执行 `scripts/test-local-development.py`：全新 UUID 项目验证本地账号首次初始化、重复初始化不改材料、Redis marker 丢失后拒绝启动且不修复，再核验并清理该轮自有资源。此专项不启动 HTTPS 网关或占用 8443，不触及用户的持久 `oj-local-dev`。加入第二套真实容器检查后，该 job 超时为 30 分钟，原必需门禁保持。

作者参考申请随真实会话专项覆盖直连 TLS 和代理 mTLS 下的显式启用、跨进程原键查询和取消、账号错配、越权、撤权、退出及额度释放。手动浏览器专项另验证服务端接纳后丢失浏览器响应、刷新不写、原键恢复与取消。验证边界见[参考申请合同](reference-requests.md#作者申请入口)。

作者显式准备与一次受控选择同样进入后端和集成门禁：真实 Redis 会话经双 API 启动准备并回读原键计划，真实 PostgreSQL 验证取消/退出或关闭时不写入、选择索引边界、并发首领、实际丢 COMMIT、历史隔离与非空回退拒绝。桌面固定槽测试与浏览器专项仍独立运行。

同一专项还构建真实 API 子进程，验证显式文件配置的 TLS/mTLS 入口、双副本 Cookie、启动只读门禁、PG 等待时信号取消、监听失败资源释放及活动请求的正常/超时退出。排空/并发 Close 的确定性测试归普通 backend race。默认 Compose 未启用会话模式，不把镜像的 PGDATA/PG_MAJOR/PG_VERSION 当成 pgx 连接环境，具体禁止项随锁定 driver 消费列表维护。子进程仅继承显式测试环境，使用合成材料与私有容器网络，不连接宿主或真实业务服务。

同一真实 API 专项还用 `pg_dump` / `pg_restore` 将会话权威还原到另一个私有 PostgreSQL 集群。刻意保留 Redis 旧记录和原密钥；闭门时 API 不监听，重新开放后的两个 TLS API 拒绝旧会话与未完成登录 Cookie，同一账号可重新登录。此回归验证恢复代次，不替代 Redis 重建或异机恢复演练。

同一恢复库还运行[普通会话维护命令](architecture/operations-design.md#恢复普通会话)：旧 Redis 拒绝、限流配置写入后 PostgreSQL 开门故障、部分准备实例不重用、另一空 Redis 成功初始化。测试补回备份之后的合成封禁，重开的双 API 拒绝旧会话和未完成登录，并跨副本接受新会话。账号对账是外部动作，临时实例不构成独立故障域。

重开的双 API 还检查[参考验证积压](architecture/operations-design.md#回档后的旧参考验证授权)：新会话查回原键、取消与跨实例重放、满额拒绝与取消后用新键提交。数据库核对旧请求未换绑、补回封禁的作者仍有排队记录；不运行参考 Worker。

可选已登录改密沿用同一专项：真实 PG 条件替换/双副本竞争、恢复→账号锁序/取消/未知回执及主键查询计划；独立三维 Redis 额度与完整规则指纹；双 API 进程的当前密码/弱密码/CSRF 拒绝、Cookie 清理、旧会话/候选拒绝、新密码重新登录，以及实际 TLS→mTLS 代理来源防伪。规则文件和弱词仅为合成夹具，不下载生产字典或验证浏览器/实际部署容量。

可选私有草稿HTTP沿用同一真实Redis/PG专项及API子进程：直连TLS和mTLS单跳代理下验证默认拒绝、受信候选导入、跨实例原键恢复、候选内容改变409、本人元数据/题面样例读取、旧预览不跟随新候选、越权统一404、撤权/退出及正常排空。真实协议另验材料读取后退出/过期、角色/账号撤销与恢复代次变化不返回部分预览；短期会话在导入完成后单独签发，避免fsync耗时冒充读取中到期。backend race另验部分启动失败后Prepared/Local文件描述符释放、严格正文、失败分类、完整预览预算/引用预检、UTF-8保真/正式点隔离及导入/预览共享材料并发。只执行小型合成bundle，不执行题源程序、不验收大语料HTTP耗时或管理页面；默认配置保持关闭。

同一专项现覆盖管理页绑定导入协议：作者候选目录、严格三字段及完整修订guard、成功体未消费便关闭后由另一真实API只读恢复回执、当前候选换版后仍读已存旧修订，以及owner/撤权/退出。普通PG套件验证回执与当前授权同事务、三类撤销锁和死事务不得换连接读取，真实COMMIT回包丢失后只读恢复；另用真实在途创建展示404之后仍可能提交。原owner/key查询计划继续检查，不增加schema/SQL。backend另验题目version相同但证据/index改变时旧revision在材料I/O/活动槽前拒绝，候选终验失败不返回部分摘要，回执不依赖候选或材料槽。实际十题目录/绑定导入/回执随已有显式私有bundle专项验证，会话观察stub，不冒充十题真实HTTP。

浏览器账号的请求/状态机 Vitest 随 frontend-tests 执行。另有显式手动 Playwright 专项：真实 race API/PG/Redis/pinned mTLS，固定回环发布端口、已有浏览器依赖、不自动安装；验证正常 Cookie/改密和注入未知回执/跳转。`integration && browser` 测试夹具不进入普通 CI；其无认证控制端点只允许隔离测试编排，Go 正常排空与浏览器断言结果分别核验。命令归 README，细节归[浏览器账号](account-browser.md)，不把九项 CI 通过当作浏览器专项已执行。

同一夹具的 `--cold-firefox` 模式要求未导入 CA 时导航以 `SEC_ERROR_UNKNOWN_ISSUER` 失败，再验证仅在临时 Firefox 配置导入夹具 CA 后，同一 HTTPS 端点冷加载成功。负对照直接检查导航错误，不依赖 Firefox 内部错误页的 DOM。专项还验证真实登录、合成草稿导入和参考申请按原键查询、准备与取消，区分准备受理与持久就绪，并验证刷新后只读恢复。普通 CI 不运行此手动模式。

同一手动浏览器驱动现为15组，保留管理页真实绑定导入、服务端已提交后屏蔽成功回包/刷新只读恢复、同修订只读核验、实际Cookie从A切换到已有作者权B时原三条管理路由及本人列表均409且B无草稿、B不见A本机记录。真实Chromium IndexedDB覆盖两连接首次插入仲裁、32条事务上限、旧确认/删除不碰新pending、owner主键内容损坏、配额失败、未知schema拒绝且不自动重建；UI清库后不能用内存重试。独立journal入口只由测试驱动构建到临时assets，生产App不引用，不是产品调试入口。backend覆盖mandatory header/缺cap404/主体比较零业务I/O；前端覆盖迟到响应、延迟到期timer和确认回执拒绝不换新请求。证据是小型合成bundle和两IDB连接，不扩写为十题页面、两个完整管理标签并发或生产浏览器矩阵验收。

本人服务器列表新增固定20条seek分页、同事务当前权限/owner只读摘要，真实PG验证0/20/21/45条、全相同时间戳跨多页、微秒边界、其他owner隔离、三种撤销锁、死事务零结果、infinity/不可JSON时间拒绝；12,000行权威first/deep SQL计划验证目标索引/owner与seek Index Cond、无Sort并记录索引大小。历史schema8索引的Up/Down/重Up保留非空草稿与权限，原7数据保护断言仍执行；当前启动拒绝不匹配的 schema，现行回退条件见[训练队列迁移](submission-backlog.md#升级与回退)。实际在途插入证明created_at早但迟提交可被旧边界漏过，重查首页可见，不宣称跨页快照。真实direct/proxy API验证默认拒绝、目录换版后仍读旧列表、owner与主体错配/撤权/退出；手动浏览器额外在新档案禁用IndexedDB后分页发现并预览真实草稿、丢弃迟到下一页、列表不改pending，全程发现操作无POST。原始材料/题源不在本专项执行。

| 控制 | 状态 |
| --- | --- |
| 根规则、开发文档、PR 模板 | 已维护；授权归根规则，步骤归开发工作流 |
| Git hooks 与治理脚本 | 本地实现；安装与测试结果见交付记录 |
| GitHub Actions `governance` | 已配置仓库权限及必需检查；具体远端运行见交付记录 |
| Go / 前端/数据库与容器检查 | Docker 工具链执行，真实 PostgreSQL integration 与 Compose 冒烟已有 CI 证据；完整 M0 未验收 |
| 秘密与依赖漏洞检查 | Gitleaks 历史扫描、govulncheck、pnpm audit；security 失败传递到必需门禁，范围见安全检查文档 |
| 分支保护、CODEOWNERS、独立审批 | main ruleset 已启用并回读；单人维护按用户选择暂不强制独立批准/CODEOWNERS |
| 测试/生产环境、制品仓库、CD | 未配置；不可变离线包/维护窗口/受控回滚已确认，Linux Docker Engine＋Compose 优先验证 |

远端为 GitHub 公开组织仓库，管理员权限和保护配置已核验，详见 [GitHub 治理](github-governance.md)。仓库设置与工作流执行结果分别验证。

## CI 分层

专用 Ubuntu VM 的固定材料判题链路另由 [Ubuntu 专项](../README.md#专用-ubuntu-vm-判题实验)执行。普通后端检查覆盖双向版本不符零领取、档位漂移、锁标记和原生 Docker 停止证据；普通 CI 不启动该特权槽。

停止阶段预算回归验证枚举子期限到期后实际受信本地命令仍可启动一次、父context不被子取消、短父期限/已过期期限不延长、kill子期限失败、枚举失败不能转为停止成功。既有drain/生命周期失败零信号、PID确认、owner隔离与原整体期限耗尽诊断继续执行。使用固定可信shell及替身，不制造真实Docker故障；真实正常停止兼容性另由专用沙箱验证。

前端 `frontend-check` 的 `pnpm lint` 包含类型感知 ESLint 及实际配置的六项正反例自测，失败阻断现有 frontend 门禁；未引入独立外部服务或改变保护规则。范围与成本见[代码质量检查](code-quality.md)。

普通 CI 包含治理、Go 静态分析/race/构建、前端、真实 PostgreSQL 集成、Compose 冒烟及安全扫描。Go 静态分析及检测器正反例由 `toolchain backend` 执行，覆盖独立会话模块和 sandbox 标签源码，范围见[代码质量检查](code-quality.md)。`containers` 在 backend 或 frontend 受影响时执行页面/代理、容器权限、重建与启停验证，结果传递到必需门禁。工具链与基础镜像由 Dockerfile/锁文件固定，宿主不需要 Go、Node 或 Make。

`internal/materialstore`和维护命令随普通backend做格式、静态、race及构建；文件同步故障注入、双实例/双进程及受控退出不运行题源程序。真实512KiB tmpfs写满需要`OJ_MATERIALSTORE_CAPACITY=1`及固定空挂载，异主owner专项需要独立root夹具；普通非root CI显式跳过二者，不能冒充专项通过。真实十题对象持久卷导入/无来源跨容器复验为手动专项，范围归[存储合同](material-store.md)。

特权判题实验不进入普通 PR CI。本机使用 `python3 scripts/test-worker.py all`（也可选择 sandbox/report/dual-worker/worker-fault/slot-recovery/network-fault/compile-lease/resource-bounds/http-loop/restore-loop），运行合成材料的隔离、回报、双 Worker 接管、异常槽解封、网络故障、编译在途失权、资源预算、HTTP 真实判题闭环及恢复后重判验证；不得在普通 runner 执行生产不可信提交。原专用 VM 与当前 Docker Desktop 的边界、失败诊断和历史结果见 [M0 实验记录](m0-experiment.md)及交付日志，不以普通 CI 代替专项验收。

`restore-loop` 另外导出 Linux 维护工具，由两个临时 PostgreSQL 容器执行捕获、接收核验和还原后检查；宿主仍运行 Darwin Worker 与控制端。普通 CI 对该沙箱源码做静态检查，真实备份到重判的组合验证由 Desktop 专项完成。

`rollback-loop` 是单独的 Desktop 专项：从固定 Git 历史构建旧版，与当前版本在同一个临时库上切换，验证原回执、已完成报告和排队提交。普通 CI 静态检查其源码，不构建历史版本或启动特权槽。

实验判题离线包的清单、文件和镜像拒绝行为由普通治理测试验证，不调用 Docker；包括非空库不导入、空库导入后缺失或额外镜像不通过。空库接收、导入和禁拉取的真实判题由[专用 Ubuntu 实验](../README.md#专用-ubuntu-vm-判题实验)验证。

`http_address_test.go` 的有界启动日志解析是普通 root race 用例，不启动进程或调用 Docker。双提交 API、mTLS 控制端及真实 Worker 的 HTTP 判题和过载恢复由 Desktop 和 Ubuntu 的手动 sandbox 专项验证。普通 CI 只编译/静态分析这些源码。准入及证据边界见 [README](../README.md#http-到真实判题终态专项)。

普通 PostgreSQL 集成测试从独立临时集群的逻辑备份还原在途租约，实跑[恢复提交租约](architecture/operations-design.md#恢复提交租约)命令，验证到期前拒绝旧租约、原任务重领、恢复前后尝试预算以及错误回滚；不访问试用库或执行选手程序。

同一套集成测试还从归档还原已准备的参考申请及活预留，验证[作者授权换代](architecture/operations-design.md#回档后的旧参考验证授权)前后真实 PostgreSQL 的授权差异；使用合成报告，不运行参考 Worker 沙箱。

[恢复参考验证积压](architecture/operations-design.md#恢复参考验证积压)专项在真实归档上运行维护命令，覆盖分页、错库与错代次拒绝、逐笔取消、原回执和保留证据、释放额度；数据库锁等待覆盖取消先取得门禁和重新开放先取得门禁两种顺序。

同一套件运行[恢复材料核验](architecture/storage-design.md#恢复材料核验)，检查合成判题材料、私有草稿和历史正式／对拍计划，注入缺文件、符号链接、损坏清单和对象及跨草稿错绑。记录小型归档的查询计划，不执行题源程序。

[捕获逻辑备份](architecture/storage-design.md#捕获逻辑备份)回归在真实 `pg_dump` 导入快照前提交一笔新材料版本，然后真实还原归档并核验两套材料。检查已封存文件的哈希、排除快照后的引用及缺文件不封存；使用临时 SCRAM 账号测试显式凭据文件和错误路径拒绝。

上述集成回归先把封存集合复制到另一目录，使用源端清单摘要通过[接收端核验](architecture/storage-design.md#核验收到的逻辑备份)，才还原 PostgreSQL。普通 Go 测试注入同长度损坏、缺失和多余文件、重复或异常清单、符号链接和 FIFO，核对失败无成功回执。

`internal/m0browser` 的配置/HTTP边界race随backend执行，固定预设、独立归属、拒绝零Store调用及nonce轮换不执行选手程序。`browser-loop` 是独立显式专项，需要已安装的宿主Node/Playwright/Chromium，普通CI和 `all` 均不运行；命令归[README](../README.md#m0-最小浏览器实验页)。它验证实际提交API、PG、mTLS控制端及Worker与桌面/390px浏览器，真实丢接收响应及原键恢复与注入查询503分别报告。现有前端预览和账号浏览器专项保持原入口，不因本页通过而扩写M1训练或账号权限验收。

`tests/sandbox/network_relay*_test.go` 没有 sandbox 标签，只测试标准库回环 TCP 的字节透传、双向/下行阻断、关闭旧流后恢复及并发关闭，已纳入普通 root `go test -race ./...`，不使用 Docker 或执行选手程序。`network_fault_test.go` 的真实 Worker/控制端/数据库及特权沙箱仍要求 sandbox 标签和 `OJ_SANDBOX_NETWORK=1`，由专用驱动提供；两类证据分开记录。

`compile_process_test.go` 的纯进程表解析边界也由普通 root race 执行，不发信号或调用 Docker；`compile_lease_test.go` 的实际编译器暂停、节点失权及提前停止证据仅由显式 `compile-lease` 专项执行。14 秒是早于原15秒编译 wall 的测试证据界限，不是业务预算或 SLA，不能为制造通过而延长。

`resource_bounds_test.go` 及其嵌入的有界 C++ 探针只由显式 `resource-bounds` 专项执行；普通后端检查只编译/静态分析，不创建子进程或填充文件系统。分别验证进程、inode、字节、单文件和内存边界及新执行恢复，保持固定预算；直接沙箱的Compile资源探针不冒充正式Run/Worker结果分类，具体限制见[README](../README.md#沙箱资源预算专项)。

该专项中的 `compile_file_size_test.go` 另走正式GoJudge/Runner，真实固定g++对照AC→文件上限CE→同Runner AC；透明观察器不重发或改写响应，超限例不转发Run。普通CI只编译/静态分析，不执行该合成编译程序；编译器诊断不作为生产分类规则。

SIGXFSZ/OLE的纯响应、分类和冻结摘要回归随普通root后端race执行；真实单文件写入、同一Worker/LeasedRunner及容器生命周期继续AC、持久结果和双控制端回放由显式 `report` 专项执行，普通CI不运行该沙箱程序。准确Run-only支持边界见[判题设计](architecture/judging-design.md#53-统一结果分类与单点异常归因)。

执行诊断的阶段/角色/安全类别、主错及多清理错误、独立context、64槽宽/深/循环/空分支、8/9条全局上限、并发副本和成功JSON兼容回归随backend运行。控制transport/body/decode与宿主inspect/file transfer分别验证既有context及错误接口契约；owner验证执行与停止诊断同时失败的Finalize、报告持久化和quarantine。专用 Worker 另经实际 owner 注入第一个点的输入上传超时，分别验证停机确认与不明的 JSON 诊断、脱敏和零报告。测试使用替身，不制造真实Docker停止失败；普通CI不据此解释历史故障。

### 当前治理流水线

对 PR、main/master 推送及人工触发保留工作流入口，不使用 workflow 级 `paths-ignore`，避免必需检查悬空。2026-10-28 起按完整变更范围选择应用测试，保留四个稳定必需检查名称，不修改远端保护。

1. 检出待检查版本，checkout 固定到核验过的完整提交 SHA，不保留推送凭据。
2. 用 Python 标准库执行仓库检查和 hook 回归测试。
3. `governance` 在仓库检查、标题检查和治理回归通过后，通过 `scripts/ci_scope.py plan` 输出各套件是否受影响。应用任务为 `backend-tests`、`frontend-tests`、`integration-tests`，执行原有完整检查，不按缺少 manifest 或测试失败跳过。
4. `backend`、`frontend`、`integration` 是轻量结果门禁，使用 `always()` 检查分类和对应应用任务结果。受影响时只接受 `success`；明确不受影响时只接受 `skipped` 并写入摘要“tests omitted”。分类失败、缺输出、取消、未知结果或应跑未跑均失败；不能把未执行测试描述为测试通过。旧 `bootstrap-ci` 初始化检测器仍在有应用时拒绝通过。

权限仅 `contents: read`，不注入生产秘密，不使用 `pull_request_target` 执行 PR 代码，不上传仓库到外部审查服务。PR 同分支新运行可取消旧检查；设定超时。`security` 执行固定版本 Gitleaks 历史扫描、Go 可达漏洞与 pnpm 全依赖审计，三个稳定结果门禁同时要求其成功；范围、联网行为和告警处置见[安全检查](security-checks.md)。文档变更仍执行安全检查，应用测试按下方矩阵分流。

### 按变更影响选择检查

| 完整差异中的路径 | 应用测试（治理检查始终执行） |
| --- | --- |
| 根 README/AGENTS/LICENSE、`docs/**/*.md`、PR 模板、`web/README.md` | 无应用测试；治理、安全检查和结果门禁仍执行 |
| `web/` 中其余文件（含依赖、锁文件、Node 和构建配置） | frontend |
| `cmd/`、`internal/`、`db/`、集成/沙箱测试目录、`go.mod`/`go.sum`、`sqlc.yaml`、`scripts/test-postgres.sh` | backend + integration |
| hooks、仓库策略/安装脚本、普通治理测试 | 无应用测试；治理回归、安全检查及结果门禁仍执行 |
| 分流脚本及其测试、workflow、Dockerfile/Compose/工具脚本、仓库配置、其他未识别路径 | 全量应用测试 |
| 手动触发、无法取得有效差异、空差异 | 全量应用测试 |

混合改动取并集。新增模块须先确定依赖影响再缩小默认全量范围；不通过后缀全局排除 Markdown（例如应用目录里的 Markdown 测试夹具仍触发应用测试）。本次不做包级测试裁剪，也不修改应用断言、部署或特权沙箱边界。

PR 使用事件中的不可变 base/head SHA 做 three-dot diff，覆盖整个 PR 而非最后一次提交；主线 push 使用 before/after 的完整 two-dot diff。checkout 获取完整历史，Git 的 NUL 分隔文件名支持空格/换行；禁用重命名折叠，以便同时检查删除源和新增目标。无 REST changed-files 分页或路径过滤文件数截断。应用测试仍检出 GitHub 合并候选，不仅测 head。

PR 的 `edited` 事件继续按完整差异执行，确保标题和目标分支变更被复核；仅编辑描述也会重验受影响套件。GitHub 会把被条件跳过的必需任务报告为成功，因此不为正文编辑跳过同名门禁。按[开发工作流](development-workflow.md#每个任务的闭环)集中定稿、推送和送审，减少无谓运行。

集成任务只构建一次 Docker 工具链，SQL 生成检查、真实 PostgreSQL 回归及两套协议测试复用本次任务的 Go 编译缓存。SQL 和 PostgreSQL 检查在无外网的容器中顺序执行；其后普通会话与邮箱生命周期并行，各自使用独立的 Docker 内部网络、PostgreSQL 和 Redis，邮箱使用本地 SMTP。两个驱动都必须正常退出；分别将日志写入 Actions，失败不能被另一套成功覆盖。缓存只保存 Go 构建产物；真实测试仍执行，未引入跨运行缓存。

本地验证：`python3 -B -m unittest discover -s tests/governance -v` 覆盖路径矩阵、真实 Git 多提交/分叉/删除/重命名/push、缺失历史及失败门禁；已有 `actionlint` 可检查 workflow 语法和表达式。远端实际启用和结果以对应 PR head 的运行及交付记录为准。

### 衡量 PR 等待与重跑

在仓库目录运行 `python3 -B scripts/measure-pr-ci.py <PR号>`，只读查询已认证的 GitHub CLI。它分别统计工作流运行数、手动重试次数、每个运行最新尝试的结论，以及最终 head 最近一次运行的状态和成功时的各任务耗时。首轮成功还报告从创建运行到最后任务完成的等待时间；手动重试只报告该次执行耗时。GitHub 有时不返回 PR 归属，脚本按来源仓库、分支和 PR 存续期推断并单独计数；保留分支复用或历史截断的误差。数据不能区分正文编辑、推送等具体触发原因，也不用于批准合并。

以 [#265 基线](delivery-evidence/ci-delivery-efficiency.md) 为对照，改动上线后连续采样 10 个实际跑集成套件的 PR。比较最终成功运行的中位耗时、集成任务耗时，以及每 PR 的取消数；应用测试未执行的 PR 不与全量运行混比。若集成时间仍是主要瓶颈，先检查冷构建和步骤耗时，再决定是否引入跨运行缓存或拆任务。

设计依据（2026-10-28 核对）：GitHub [必需检查与跳过任务](https://docs.github.com/en/pull-requests/how-tos/merge-and-close-pull-requests/troubleshooting-required-status-checks)、[工作流语法与 diff](https://docs.github.com/en/actions/reference/workflows-and-actions/workflow-syntax)、[并发控制](https://docs.github.com/en/actions/how-tos/write-workflows/choose-when-workflows-run/control-workflow-concurrency)。采用影响范围选择、稳定门禁和失败阻断，不表示已具备独立人工审批或完整企业发布体系。

### 已确认的验证与交付路线

会话名额、撤销、API 重启、Redis 超时及 PostgreSQL 账号认证代次的[独立实验模块](../tests/sessionexperiment/README.md)已有本机真实存储验证，但行为套件不属于根模块 `go test ./...` 或当前 CI；CI 覆盖该模块静态分析，专项 race 测试须按该页单独执行，具体数量与结果见实验记录。下方完整会话验收目录仍待完成，不能以 PR 的 backend 绿灯代替会话实验结果。

Go 标准 testing、真实 PostgreSQL、普通 Redis 协议专项与 Vitest 已配置，版本以依赖清单和 Dockerfile 为准，命令见 README。Playwright 账号路径已有本机手动验证；React Testing Library、Testcontainers 及其余完整 Redis/HTTP 会话验收仍待验证。专用可信 Linux 节点执行沙箱、故障、混合负载和断外网恢复演练，与快速 CI 分层；不可信 PR 不持有生产凭据或高权限执行节点。新增验收 AC26—35 与原 AC01—25 联合执行，不能用治理测试代替应用测试。

离线 Nginx/HTTPS/DNS/chrony、可观测性、pgBackRest/restic、一致性恢复集合与两阶段回收、不可变制品/短维护/兼容回滚规则见 [运维设计](architecture/operations-design.md)。当前无安装、部署或演练结果。

### 当前应用检查入口

当前 `backend-tests` / `frontend-tests` 分别运行容器内 `toolchain backend` 与 `frontend-check`。`integration-tests`使用同一工具镜像内的PostgreSQL14私有临时集群，运行sqlc v1.31.1四包（submission/identity/problem/problemreference）生成差异，新增`internal/problemreference/postgres/referencedb`；并执行事务/迁移/租约、私有草稿授权、持久化参考申请及双API/mTLS控制端测试，不连接现有数据库。普通会话专项另验真实Redis与草稿消费者的最终会话读取边界。实际十题大材料通过显式私有bundle/新材料根环境选择手动专项，普通CI只执行小型合成夹具，不能写成大语料验收通过。Docker Desktop真实沙箱通过`python3 scripts/test-worker.py all`独立验收，不进入普通PR CI。每次交付须核验对应head的结果，不借用旧PR的通过状态。

Dockerfile 的 sqlc 与 golangci-lint 两条固定版本安装命令局部使用 `GODEBUG=http2client=0`，将 Go 工具安装的 HTTPS 客户端切到 HTTP/1.1，缓解已观察到的校验站 HTTP/2 stream `INTERNAL_ERROR`。该开关受 [Go net/http 文档](https://pkg.go.dev/net/http#hdr-HTTP_2)支持，并已核对锁定 Go 1.26.10 的本地文档；它不是全局 ENV，不改变应用/运行镜像的 HTTP 配置，不关闭 TLS、Go checksum 或更换代理。传输切换不保证远端服务可达，不能解决 TCP 拒绝连接或视为已定位上游根因；实际构建与 CI 验收状态见[交付记录](delivery-log.md)。回退两处命令级赋值即可，无运行配置或数据库迁移。

### 随功能交付建立的验收目录

已实现 Worker/控制端的快速行为与真实 PostgreSQL/mTLS 测试由 backend-tests/integration-tests 维护；实际选手程序执行、PID 停止与故障恢复依赖独立 Worker 专项。各次引入时的测试数量和旧命令只在 [M0 记录](m0-experiment.md)与交付日志保存，不在本页并行维护另一套状态。

下列为完整功能验收目录，按 [交付阶段与 AC 子项](phase-1-development-plan.md#分阶段验收归属)启用，不是首次应用提交必须一次完成的清单。已交付功能的要求不能跳过；未交付条目保持待实现，不能标通过。实际可执行命令见 README。

- Go：锁定版本与依赖、格式检查、静态检查、单元测试、适用的 race 测试、构建和依赖漏洞检查。
- 数据库：用选定版本的真实临时数据库验证迁移、约束、并发和关键查询，不能只用 mock。
- 迁移：已有 Goose＋SQL 文件、`cmd/migrate` 与显式锁，`toolchain db` 执行现有迁移集成测试。完整验收仍须逐项覆盖空库初始化、旧版升级、重复运行、两个执行者争锁、锁超时/断连、事务失败及非事务部分成功恢复、允许的回退/重升；核对 SQL 哈希、执行顺序、sqlc 生成及真实数据库结构一致性，校内断外网也须可执行，不能仅凭现有测试通过宣称全项验收。
- 前端：已选 React＋TypeScript＋Vite、React Router（声明式）＋TanStack Query＋fetch＋pnpm；已引入 Router 与懒加载 CodeMirror 的示例界面，Query 和正式训练数据请求尚未引入。维护 packageManager 与 pnpm-lock.yaml，执行冻结安装、lint、类型检查、测试和生产构建。路由与预览验证见 [Web 界面](frontend-design.md)；随正式功能继续验证校内资源、版本切换、写操作不自动重试、提交超时按原幂等键核验接收状态、账号切换隔离迟到响应、敏感查询不落盘，以及刷新有界和终态停轮询，不将示例通过视为生产训练功能通过。
- 端到端：骨架验证前端到真实健康 API 的路径；账号、提交、真实判题和比赛榜单分别随功能加入。试用发布前贯通该版本全部必需路径，正式比赛发布前覆盖完整赛事流程。
- 题面渲染：react-markdown＋remark-gfm＋remark-math＋rehype-katex／KaTeX已引入，共享渲染/本人预览的GFM、公式、原文样例复制、HTML/危险链接/外图拒绝、展开/尺寸/AST预算、局部错误、窄屏与本地字体失败已有分项浏览器证据，见[界面约定](frontend-design.md#共享题面与本人预览)和交付记录。AC06/17/18/20仍需完整选手视图、图片授权、全部题面质量及真正断外网冷缓存演练，不把库安全声明或分项通过当整体验收。
- 登录会话：按 AC23 使用真实 Redis 和数据库及至少两个 API 副本验证 Session、当前权限、退出/封禁/过期、续期与撤销竞态、Cookie/CSRF；注入 Redis 超时、内存不足、重启及旧快照恢复，确保错误可见且旧会话不复活。普通存储协议已有专项命令，完整 HTTP/恢复流程及部署参数仍待验收。
- 会话规则：覆盖普通 2 小时/12 小时、保持登录 24 小时/7 天、管理 30 分钟/8 小时的边界和再次验证；用可控时钟验证，不能只等待真实过期。并发验证普通 3 会话上限、比赛单参赛会话拒绝新登录、换机与旧请求竞态；覆盖本地草稿恢复、轮询不无限保活、断外网账号重置、比赛结束立即停交、赛后窗口默认 30 分钟、延赛期限和名单历史归属。
- 本地代码草稿：已选 IndexedDB＋idb，按前端设计第 3.2 节和 AC21/23 用真实浏览器验证停输 1 秒/持续输入目标 5 秒调度、事务与编辑版本对应、乱序写入、刷新恢复、多标签版本冲突与双方保留、账号/题目/比赛/语言隔离；覆盖容量不足、存储禁用、模式升级受阻、导出/手动清理、主动退出默认清理及取消、跨标签清理与迟到保存竞态、会话过期后同账号恢复和断外网保存。首期不按时间自动删除，保存目标不视为崩溃持久性保证；代码草稿预算和实现仍待完成。idb 8.0.3已由管理页最小恢复日志引入/验证，但该日志不证明上述代码保存或退出清理通过。
- 会话库准入：先验证 SCS＋go-redis，Gin sessions 为备选；检查 Gin 中间件的请求上下文/响应提交/错误处理、凭证摘要存储及期限策略，禁止迟到保存复活已撤销会话。用两个以上 API 验证名额原子性、连接池总额、Redis 超时/重试和恢复。go-redis 已锁定并在普通存储协议专项运行；SCS/Gin 正式 HTTP 适配、完整资源与恢复准入仍待完成，不能以独立实验或存储专项代替。
- 密码：已选 Argon2id＋独立随机盐＋可升级参数记录，同步核心、主体凭据/CAS 与共享实例计算准入已有分项测试，见[会话设计](architecture/session-design.md#7-密码保存与校验)。完整 AC24 仍须验证传输编码严格解析、账号升级/改密/重置/迟到登录竞态及脱敏日志；硬件确定后测试多 API 的总计算额度、峰值 CPU/RSS、排队与集中登录恢复。HTTP 超时不等于计算已释放；复用锁定依赖，生产参数及负载基准尚未定版。
- 账号生命周期：按 AC25 验证邮箱注册激活、密码登录、自助找回、校内发号及人工核验重置；覆盖令牌过期/用途隔离/并发单次消费、限流、防枚举、旧会话撤销、邮件失败与断外网比赛。开发收件器或 mock 仅证明应用流程；实际供应商 TLS、配额、退信和目标邮箱收件情况须另行联调。可替换 SMTP 接口已确认、go-mail 优先验证；按会话设计第 9 节补充适配器替换、TLS/认证/超时/拒绝及受理后断连、连接预算与脱敏检查。发信任务优先验证 PostgreSQL 持久化＋独立受限执行器，按第 10 节验证事务回滚、竞争领取、进程终止/超时恢复、迟到回报、重复投递、过期丢弃和积压恢复的资源预算；River 开源核心优先验证、专用表备选；补测 pgx 同事务、Goose 迁移/恢复、开源功能边界与资源预算，未通过时再评估备选，不并行引入两套权威发信队列。域名、现有 SMTP、供应商和精确依赖尚未确定，无发信运行结果。
- OJ 回归：封榜泄漏、重复/乱序回调、重判旧结果、队伍唯一性、私有题目、节点掉线恢复；按需求 AC 编号跟踪。
- SPJ 协议与复用：已选 ICPC 输出验证器协议、必须支持 Testlib 及 GNU++17（1B/M2）；按判题第 4.1 节验证参数/标准输入、原生 42/43/0/未知码、信号/超时/超限与反馈矛盾、testlib 模式/原始结果适配不混用、固定版本/编译参数/制品哈希与多节点重判、本站比较语义、诊断权限和断外网运行。精确版本/许可与隔离验证后才能正式引入；当前无安装或执行，完整原始结果映射与预算待定，首期不支持部分分。
- 题包导入：统一内部模型＋2025-09 子集为已确认验证路线；按题包设计及 AC05/06/17/18/20/22 验证手动/ZIP 入口等价、原包/哈希/来源追溯、大小写/浮点差异显式处理、不支持特性阻断、安全解包/解析、私有材料权限、API 不执行包内程序、受限验证和审核后发布、并发/重复/中断/取消与存储失败、不可变历史版本和断外网恢复。形成真实题包兼容矩阵后才能声明对应支持；当前无解析组件、导入或运行测试。
- 测试执行与汇总：已确认固定顺序＋单份提交串行＋首次失败停止；按判题第 5.1 节及 AC05/06/16/17/18/20 验证版本有序清单与重判、需编译语言每尝试编译一次/解释型语言固定准备、逐点新环境、前一点通过才启动下一点、首/中/末点失败停止且不跨点改判、全通过才 AC。覆盖系统故障中止/有限恢复、不同提交资源有界并行、未执行点及耗时/内存统计范围、迟到/重复回报；固定 GNU++17 材料的编排/隔离已有分项验证，证据见 M0 记录；这不覆盖所有语言、题型和完整产品验收。
- 首期答案检查：已确认标准输入输出＋文本比较、按题显式浮点容差与受限 SPJ；按判题设计及 AC05/06/16/17/18/20 验证文本 token/大小写/空白/额外输出、浮点开启/关闭/每题参数/数值边界、严格整数混合输出约束、SPJ 多种合法与错误输出、崩溃/超时/非法协议映射为系统错误、固定版本重判和多节点一致。全部要求的测试通过才 AC，无部分分；交互题和直接提交答案题后续扩展。先文本闭环、再补齐浮点/SPJ，不将单项通过当作首期整体完成；SPJ 协议已按第 4.1 节确认，适配/准入与资源数值待验证，目前没有执行。浮点按第 3 节验证绝对/相对及 OR、只满足一项、边界等于/内/外、零/接近零/负数、科学计数法、有限值/NaN/无穷大/非法输出、混合文本沿用显式大小写配置（默认接受 ASCII 大小写差异）、非法参数/标准答案发布阻断；数值精度、溢出/下溢、超长 token 与多节点一致性尚无运行结果。
- 邮件载荷：已选业务侧短期 AES-GCM、River 仅存业务 ID 和独立密钥文件；按 AC25 补测密文往返、篡改/错误上下文拒绝、随机失败、跨副本密钥版本、密钥文件异常、轮换/清理/备份恢复及任务参数/日志无明文令牌。核验所选 Go 随机 nonce API 的版本与每密钥总使用量；缺钥暂停相关发信，不自动生成替代密钥、不影响已有登录和判题。当前未实现或执行。
- 邮件规则：注册验证 24 小时、找回 30 分钟、主动重发间隔至少 60 秒、同账号同用途最多 3 个有效链接；新链接不撤销旧链接，任一完成后同用途全部失效。真实 PostgreSQL 单次发送尝试专项验证同一任务复用原令牌、发送期限和已消费或过期载荷拒绝，SMTP 结果由测试适配器提供。仍须按会话设计第 12 节与 AC25 联验满额不挤掉旧链接、并发消费/重发、过期释放、载荷清理不撤销链接、最多 6 次发送尝试、永久失败停止和迟到邮件无效；主动重发才生成新令牌。
- 免费选型准入：不依赖付费功能完成验收；核对具体版本/传递依赖许可和免费服务长期条件。River Pro 排除，不将试用通过视为长期可用或采购授权。
- 存储：按 AC22 验证本地材料发布中断、并发与空间不足、同机 API 共享持久化、Worker 缓存完整性/权限及独立备份恢复；未来对象存储后端须另测迁移与回退，不以本地适配器通过替代。
- 判题语言：对 GCC 15 的 C17、GNU++17、GNU++20，以及 Temurin JDK 21、CPython 3.14 分别验证编译/解释执行、正确/错误/超限结果、资源统计与节点恢复；C++ 两种模式分别测试，核对各节点固定环境一致性和断外网可用性。PyPy 后续评估；工具链补丁、构建和参数未验证不得标为已支持，升级须回归并保留可追溯环境记录。
- 数据访问：sqlc＋pgx 已引入，`toolchain sqlc` 检查生成差异，`toolchain db` 使用真实 PostgreSQL 验证现有事务/租约行为。回滚/断连、超时取消与连接池总预算等完整验收仍逐项追踪；生成成功不能替代关键 SQL 计划和并发恢复测试。
- 运行环境：判题集成测试在受限 Linux 环境中执行；不在有生产凭据、真实题库或普通开发宿主权限的 runner 上运行不可信选手代码。

应用门禁使用稳定名称 `governance`、`backend`、`frontend`、`integration`；四项均已配置为 main 必需检查并限定 GitHub Actions 来源。失败、取消和未配置不能被当作通过。PR 标题编辑触发重验，标题通过 Conventional Commits 校验后作为 squash 标题。

## 合并门禁与单人阶段

三个稳定结果门禁同时依赖 `containers`：backend 或 frontend 被分类为需运行时，容器冒烟必须成功；两者明确不受影响时才允许容器任务跳过。分类输出缺失、容器任务失败/取消/意外跳过均使门禁失败，包括未选中的应用套件所对应的门禁。此依赖通过现有必需检查生效，不新增远端检查名称或修改保护规则。

远端目标配置：main 禁止直接更新、强推和删除；只允许经 PR 合并；必需 CI 全部通过，讨论已解决，目标分支变化后重新验证。管理员例外也需留痕。

- 当前只有用户一位维护者：按根规则的持续授权，由 Codex 审查并在 CI/讨论/可合并条件满足后合并，用户保留授权与发布决定；不能声称具备双人职责分离，不设置只有作者本人却要求非作者批准的死锁规则。
- 新增长期维护者后：至少一名非作者批准，关键模块设置真实 CODEOWNERS，更新提交后要求重新审查。
- 分支保护与 Environment 审批受套餐、仓库可见性和账号权限影响。不能配置时明确报告控制缺口，保留人工关口；不声称有等价自动强制力。

用户已授权并完成 main 保护、仅 squash 合并和 Actions 权限配置；单人维护的独立批准数量为 0。PR 模板只帮助记录，不能自动证明勾选内容真实。

## CD 与环境晋级

部署目标确定后采用：开发 → CI → 测试环境 → 人工验收 → 生产环境。

- 合并后只构建一次不可变制品，用提交 SHA、镜像 digest 和发布版本关联代码、测试及迁移。生产晋级同一份已验收制品，不重新从浮动分支构建。
- 测试环境可自动部署；生产必须由获授权的人发起并通过发布门禁。`workflow_dispatch` 只是触发方式，不能替代人员权限和审批。
- dev/staging/prod 分离数据库、存储、账号和凭据。测试不复制真实用户敏感数据，不持有生产写权限。
- 秘密交由环境秘密管理；可用时采用短期凭据。不得放入 Git、镜像层、日志或 PR。部署 runner 与 PR/判题 runner 分离。
- 部署操作串行化，生产发布不因新提交到来而中途自动取消。检查迁移、健康、冒烟和关键指标，记录操作者、制品 digest、开始/结束与结果。
- 本轮不提供伪 CD 脚本：服务器、网络、数据库、镜像仓库和凭据尚未确定，不能凭占位命令制造可部署假象。

## 迁移、恢复与比赛保障

- 使用 Goose 作为独立发布步骤，显式配置 PostgreSQL 会话级迁移锁；迁移入口共用锁标识，锁连接覆盖版本检查到执行及记录全过程，等待超时/断连必须停止并核对实际状态。锁不自动阻止业务流量，不替代兼容设计。详见 [迁移设计](architecture/system-design.md#811-数据库迁移)。
- 发布前核验数据库与备份恢复条件；迁移成功才发布新应用，失败停止发布。API 启动仅检查模式兼容性，不持有改表权限、不自动运行迁移；应用回退须单独验证兼容性。
- 已部署 SQL 不改写，修正新增版本；迁移与工具随固定制品分发，保存哈希和执行记录。事务外操作单独标记并设计部分成功恢复，不将多文件发布当成一个事务，不自动强改版本表后重试。
- 数据库变更优先兼容新旧应用的 expand/contract：先增加兼容结构，再迁移数据与调用方，最后另批删除旧结构。破坏性变更独立审批。
- 应用回退和数据库回退分别设计。旧镜像仅在数据结构兼容时可直接回退；不可逆迁移需前滚修复或经过验证的恢复流程。
- 备份覆盖数据库、源代码、题目数据/版本、必要配置和审计；备份完成不等于恢复可用，必须演练。RPO、RTO、保留周期和容量在硬件盘点后确定。
- Redis 会话与业务数据分别恢复；无法证明会话状态连续时，先暂停旧会话、更新持久化全局会话代次并重建活动会话空间，再开放重新登录。数据库恢复后同样防旧会话复活；持久化和自动切换方案未定，详见 [会话设计](architecture/session-design.md)。Redis 内存、连接及重新登录峰值计入容量验收，不把会话丢失等同于提交或成绩丢失。
- 正式比赛前固定编译器、判题镜像、题目包和计分规则版本；演练节点掉线、队列积压、封榜及滚榜；比赛期间冻结常规升级。
- 已确认正式校赛须能在校内断外网运行：按需求 AC18 演练新浏览器访问、本地登录、提交判题、榜单和赛务，并检查校内服务重启、页面资源、域名解析和时钟依赖；外部 AI 与同步停用不能阻断核心流程。当前尚未实施或验证。
- 发布后观察提交接收失败、排队等待、系统错误、节点健康与榜单延迟；阈值待压测确定，不能写未经验证的可用性数字。
- 负载均衡与资源验收对应 AC19/20：在固定总资源预算下比较单/多 API 副本，演练摘除恢复、跨副本一致性、突发排队和过载降级；同时报告接受/拒绝/超时及资源峰值。API 副本扩容同步审查数据库连接池总额，同机多副本不得作为主机容灾证据。

## 证据与变更控制

流水线、hooks、门禁脚本和规则也是代码：修改需解释目的、补成功与失败路径测试，不能借故障降低约束。远端审查权限与分支保护应防止未经审查地改掉检查本身。

本地 hook 可绕过，CI 可被有权限的人修改，AGENTS.md 是行为指导；它们不能代替平台权限、沙箱和独立审查。已启用 GitHub secret scanning、push protection 和 Dependabot 安全更新；SBOM/制品来源及完整漏洞响应仍待建立。

## 官方依据

- [GitHub 分支保护](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-protected-branches/about-protected-branches)
- [GitHub Actions 安全使用](https://docs.github.com/en/actions/reference/security/secure-use)
- [部署 Environment 的权限与套餐限制](https://docs.github.com/en/actions/how-tos/deploy/configure-and-manage-deployments/manage-environments)
- [Git hooks 可用入口与限制](https://git-scm.com/docs/githooks)

查阅日期：2026-10-25。CI 实际结果见交付日志；生产 CD 尚未配置。
