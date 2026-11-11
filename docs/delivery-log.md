# 工程交付记录

## 2026-11-11：新版题包十题真实报告接纳

基于 `origin/main fb0e4ff0123f5124eea1f1a34ae80d98889ba62d`，新增[可复跑的十题实物专项](reference-requests.md#十题实物报告接纳专项)。断网构建真实 Darwin Worker，回环 mTLS 控制端使用临时 PostgreSQL 和私有材料根；合成作者逐题入库并准备申请。全局和作者队列均只允许一条待处理申请。

同一新版题包十题通过 254 个正式点，卷积另有 24 个独立对拍一致；总测试 148.65 秒。独立数据库连接回读十份终态报告，逐点 AC、停机确认、证据摘要及预留消费均成立。作者服务按原键回读十份正确摘要，另一有作者权的主体均无法读取。十份结果的内容版本、完整导入修订、正式 Plan 及 Profile 摘要与此前维护报告逐题一致。草稿均保持 `private-draft`／`not-run`。

夹具容器已清理，五个执行槽都已停止、无活跃 exec，宿主隔离标记为空。证据目录为 `/private/tmp/oj-p4-full-report-qa.cdGFLr/acceptance`，校验清单 SHA-256 为 `8809b469165bf066e4d6e7dc7d9ba8a6f73525e454d9a90e0b5c513a0a1bed8f`。末次新增题包父目录与输出路径检查另验了拒绝含无关文件或输出重叠，均在挂载前失败。108 项治理测试通过。本次不覆盖真实作者登录、完整数据质量、非作者终审、发布或部署；首轮素性测试历史失败保留。

## 2026-11-11：新版题包十题参考复验与卷积报告接纳

从 [#152 的失败记录](#2026-11-11新版题包维护参考复验中止)确认原 medium 容器身份，核对旧宿主控制进程已退出、Docker 已无活动 exec，且结束时间与保留的事件一致。原 Worker 的 `recover-reference` 确认该生命周期当前停止后清除隔离，未启动容器。随后复用同一 Worker 二进制、来源锁和 #151 题包索引，以新目录串行复验素性测试、静态区间和、关联数组、单点加区间和、静态区间最小值及卷积。分别 13、11、20、21、27、53 AC，共 145 点，全部确认停机。素性测试新旧两次 Plan/Profile 和容器 ID 相同，启动生命周期不同；原第 12 点 SE 及停机未知不追认，宿主超时根因仍未知。

结合首轮前四题的 109 点，新题包十题共 254 点均有独立的完整通过报告。重新核对十份报告的内容版本、完整导入修订、来源、正式点顺序和材料、Plan/Profile 摘要；254 点来自两批成功报告，不计入中止批次中素性测试的 11 个 AC。

另以临时 PostgreSQL、回环 mTLS 测试夹具及真实 Darwin Worker 从原断网卷积槽执行新版卷积申请。数据库回读确认 53 个正式 AC、24 个独立对拍一致、停止确认、`reported` 和预留失效；同一内容版本、导入修订和正式 Plan 与维护报告一致。草稿状态仍为 `private-draft`、验证状态仍为 `not-run`。本次夹具使用合成作者身份，不连接持久业务数据库；题包从只读父目录下读取以保持文件所有权，未修改题包。

验收后五个原专用槽均为 `exited`、PID 0、无活动 exec，宿主锁标记为空，临时 mTLS/PG 容器已清理。证据位于 `/private/tmp/oj-p4-reference-resume.pT5N5M`，校验清单 SHA-256 `7c651e3a3f1bc38f6d985079f4f8d2eea82d56abf5e52271117c1a537bb3350a`。其余九题未在此轮取得站内报告接纳；未完成完整质量、终审或发布，未迁移或部署。

## 2026-11-11：新版题包维护参考复验中止

以 `origin/main 342895601a6d50af7a8529e81bfbfecca719344a` 构建 Darwin arm64 Worker，在原有断网专用槽串行复验 [#151 新题包](#2026-11-11六题输入输出格式逐行排版)，索引 SHA-256 为 `3d52da88342d08c1fce6d2fa0bbeaa4784444a1f36b5dcbac15adf657b0ca537`。从题包重算五份报告的完整导入修订，核对来源锁、内容版本、固定源码和编译依赖、正式点顺序与材料、Plan/Profile 摘要。

| 来源 | 正式点结果 | Worker 确认停机 |
| --- | --- | --- |
| `aplusb`、`unionfind`、`zalgorithm`、`suffixarray` | 分别 12、18、29、50 AC，共 109 点 | 是 |
| `primality_test` | 11 AC；第 12 点 SE，第 13 点未运行 | 否 |
| `static_range_sum`、`associative_array`、`point_add_range_sum`、`staticrmq`、`convolution_mod` | 批次中止，未启动 | 不适用 |

素性测试第 12 点的宿主 inspect 成功，随后 Run 控制命令和清理超时；停机时排空及首次 inspect 成功，进程枚举和 kill 未在各自期限内返回。脱敏 Docker 事件显示第 12 点对应时段出现 `exec_start run`，kill 和容器退出事件晚于 Worker 失败回报。事后 inspect 为 `exited`、PID 0、未标记 OOM；这些观测不能补充当时缺失的进程确认，也不能确定超时根因。保留 medium 隔离标记；未换槽、清标记、重试或增加预算。

本机私有证据目录为 `/private/tmp/oj-p4-current-run.7UPWTq`；证据校验清单 SHA-256 为 `4fc01aae7a328b37d128ef17e2c1d582b71425c9a21c43943a535eb167999439`。前四题是此次修订的维护验证证据，历史 254 点仅适用于原试验及其绑定身份。此次未执行站内申请、报告接纳或卷积对拍，无数据库、部署或发布状态变化。再次运行前须按[槽所有权与停止](reference-validation.md#槽所有权与停止)单独核验恢复前置证据，原失败仍保留。

## 2026-11-11：六题输入输出格式逐行排版

基于 `origin/main 1243830`，[固定题源转换](problem-drafts.md#模板与语义映射)把六题九处输入输出格式的 LaTeX 字面量转成逐行显示公式。用同一已复验 v3 来源生成新私有题包，索引 SHA-256 为 `3d52da88342d08c1fce6d2fa0bbeaa4784444a1f36b5dcbac15adf657b0ca537`。逐项比较新旧清单，仅六份题面与对应草稿 JSON 改变，四题内容版本、18 对样例、254 对测试数据及来源证据保持。九处公式与独立按原行序构造的探针相符。

Chromium 151.0.7922.34 分别用两个真实会话夹具导入、预览新旧题包；旧包 73 处公式，新包 82 处（含九个格式块，浏览器 MathML 内的 TeX 摘要逐块匹配）。桌面／390px、18 对样例与 36 次复制、匿名／其他作者拒绝及清理均通过。新旧报告见 `/private/tmp/oj-format-math-browser-final` 与 `/private/tmp/oj-format-math-browser-legacy`。108 项治理测试通过。未覆盖其他题面语义或非作者终审，未迁移数据库、改写已有草稿或部署。

## 2026-11-11：十题真实授权预览与样例字节

基于 `origin/main 6f93558`，浏览器专项以只读 v3 私有题包在临时 PostgreSQL、Redis、材料根与真实 TLS／mTLS API 中导入十题。Chromium 151.0.7922.34 用真实会话从本人列表打开预览，所有响应均与锁定版本、题面摘要、18 对样例的顺序和摘要一致，36 次复制与原字节一致。1280×960／390×844 观测 73 处 KaTeX 公式、无公式错误、框架错误覆盖层、页面横向溢出或外部请求；两次登录前会话 401 是预期控制台诊断。匿名对十题均 401，另一名已授作者权的账号均 404，与不存在草稿同响应。

本次真实模式和原合成模式的 15 组检查、API 排空和资源清理均通过，报告分别位于 `/private/tmp/oj-authorized-preview-qa-final` 与 `/private/tmp/oj-authorized-preview-baseline`。真实模式显式使用夹具 45 秒导入预算和 46 秒 HTTP 写超时，未更改产品配置。另对照十题约束栏与固定校验器的数值、下标和字符范围，未发现冲突。六题的九处输入输出格式代码块仍显示上游 LaTeX 字面量（如卷积的 `$N$`）；这与代码块语义一致，但可读性仍待处理。没有题面改写、schema 变化、部署或非作者终审。

## 2026-11-11：三个固定错误解的正式数据区分度

基于 `origin/main 4a3627f`，[断网准备](problem-sources.md#输出与验证边界)在既有十题 254 点与 40 个输入对照之外，执行三份锁内错误解。在原 arm64 镜像（SHA-256 `fbfd84489b660f3cacaa8b211999116291589211abbacdf6f7d894f53f013afa`，GNU C++ 12.2.0、Python 3.11.2）中，A+B、静态区间和、素性测试首次正式 WA 分别在原序第 4、2、13 点，候选均正常退出，checker 均退出 1。容器阶段用时 104.643 秒。

v3 来源报告 SHA-256 为 `6d13ae8cf42543a407b6af46f202976c05e074e57386fa07980fb67762b87920`，运行日志为 `/private/tmp/oj-data-discrimination-real.log`。真实 v1/v2/v3 产物都通过草稿转换和独立复验，十题内容版本一致，报告原字节随各自导入修订保留。106 项本地治理测试通过。其余七题未做此对照；未验证低效解、完整题面或审核发布，也未迁移或部署。

## 2026-11-11：固定十题输入校验对照

基于 `origin/main ea7a827`，在[断网题源准备](problem-sources.md#输出与验证边界)中对每题增加合法边界、越界、截断末尾换行和尾随数据对照。真实 arm64 Docker 既有镜像（SHA-256 `fbfd84489b660f3cacaa8b211999116291589211abbacdf6f7d894f53f013afa`，GNU C++ 12.2.0、Python 3.11.2）重新生成并核验十题 254 个正式点，十个合法对照退出 0，三十个非法对照退出 3，原有 checker 对照通过。容器阶段生成与验证耗时 103.234 秒。

报告为 `oj.source-validation.v2`，SHA-256 `04cd50727ca84f8253b9e2c219d41ccd3818b35df30803f5e5f897574d88181b`。运行日志在 `/private/tmp/oj-quality-controls-real.log`，私有产物在 `/private/tmp/oj-quality-controls-corpus/materials`。实际草稿转换和独立复验保留原报告摘要，十题内容版本与原 v1 产物相同，报告及完整导入修订可随来源证据变化。103 项治理回归通过，覆盖旧报告兼容、缺失或错误证据和异常退出。无部署或数据库 schema 变化；题面质量、错误解区分度、审核和发布仍需各自证据。

## 2026-11-11：本人草稿回读参考验证结果

基于 `origin/main 15311be`，[本人预览](reference-requests.md#作者申请入口)按原申请键显式读取终态结果。普通会话和 PostgreSQL 核对当前作者与草稿身份；旧报告按原计划投影，独立对拍只返回一致数和首个生成差异，不公开正式数据、逐点证据或 Worker 资料。

两台本地 TLS API 联合临时 PostgreSQL 和 Redis 验证原键回读；专用 mTLS 入库专项以实际卷积题包和受控结果核对首差异投影。Chromium 用替身 HTTP 响应检查桌面和 390px 的显式读取、键盘展开、通过、旧报告和重新聚焦清除。对应日志在 `/private/tmp/oj-results-session-final.log`、`/private/tmp/oj-results-go-final.log` 和 `/private/tmp/oj-results-browser.log`。本阶段无 schema 变化或部署，草稿、审核与发布状态不变。

## 2026-11-11：持槽卷积对拍与组合报告接纳

基于 `origin/main d182db8`，[持槽运行](reference-requests.md#同机持槽参考执行)在同一次受权运行内顺序执行正式点、朴素解和候选解。已有对拍计划的首次正式 AC 必须携带对应的确定对拍证据；失败差异可审查，旧证据按原字节重放。无 schema 变化。

真实断网卷积槽通过 53 个正式 AC 和 24 个一致对拍，确认停止后才生成组合证据；独立对照在进入对拍时拒绝续期，确认停止且未发报告。最终原槽 `running=false`、PID 0、槽锁标记为空。原生宿主使用续期和报告替身；临时 PostgreSQL／真实 mTLS 专项另以受控结果验证首报缺字段、非法反例拒绝、合法差异落库及精确重放。完整离线后端和 PostgreSQL 集成通过。

另外以同一实现构建真实 Darwin Worker，通过仅发布到回环的临时 mTLS 控制端领取实际入库草稿，在原断网槽完成后直接上报。PostgreSQL 回读确认正式 53 AC、对拍 24 点一致、停止确认、证据已保存且预留已消费。日志：`/private/tmp/oj-convolution-live-worker.log`、`/private/tmp/oj-convolution-live-fixture.log`；分项日志见 `/private/tmp/oj-convolution-worker-evidence-native.log`、`/private/tmp/oj-convolution-worker-evidence-fault.log` 和 `/private/tmp/oj-convolution-worker-evidence-postgres.log`。

作者回读、终审与发布未接，草稿仍为 `not-run`；未迁移现有数据库或部署。

## 2026-11-11：有效预留内下发卷积对拍解法

基于 `origin/main 0ee6ba0`，[专用材料路由](reference-requests.md#有效预留内的卷积对拍材料)沿用正式预留与原授权，在读取前后复核两份计划。Worker 独立核对规范编码、固定语料和两份解法字节，失败时不返回部分材料。正式路由不扩大可读范围。

断网 Docker 中，实际卷积题包、临时 PostgreSQL、私有对象根和真实 mTLS 复现了两份解法的完整下发。测试验证普通证书、错令牌、越界对象、错摘要、非规范描述、第二份对象失败以及 I/O 期间计划移除和凭据轮换。日志：`/private/tmp/oj-convolution-worker-materials-postgres-final.log`。

没有执行题源、改动 schema、迁移持久库或部署；对拍执行、报告接纳和作者回读待接。最终 CI 与合并状态以 PR 回读为准。

## 2026-11-11：作者准备时持久化卷积对拍计划

基于 `origin/main de36dfb`，[作者准备](reference-requests.md#卷积独立对拍的预期计划)从固定来源锁的已入库卷积草稿核验两份解法，并与正式计划在原授权事务中一次提交。独立摘要绑定草稿修订、固定语料、两份解法、Profile 和正式计划摘要。schema 14 保留历史正式计划，不回填；存在对拍记录时拒绝 Down14。

断网 Docker 的临时 PostgreSQL 实物专项通过：旧计划回放与空对拍迁移可逆，候选读取失败无写入，真实 COMMIT 回执丢失后按原键恢复两份原值，规范但不同的对拍计划不得覆盖，半记录和错误摘要被数据库拒绝。两种计划读取的 `EXPLAIN ANALYZE` 均命中申请主键。日志：`/private/tmp/oj-convolution-expected-plan-postgres-final.log` 和 `/private/tmp/oj-convolution-expected-plan-postgres.log`。

本阶段没有执行对拍题源，也没有改变 Worker 正式点协议、草稿状态、报告接纳或发布。未迁移持久化 localdev 或目标服务器。最终 CI 与合并状态以 PR 回读为准。

## 2026-11-11：已入库卷积草稿的对拍材料

基于 `origin/main f5d9fa0`，[草稿材料冻结](reference-validation.md#已入库草稿的卷积对拍材料)用完整导入修订和固定来源锁绑定朴素解与候选解。材料绑定报告保留草稿 ID、内容版本和导入修订，维护命令旧格式保持；任何一份对象损坏都不返回部分材料。

离线 Docker 的定向竞态测试通过。已复验的实际卷积候选经正式导入服务写入临时 PostgreSQL 和私有对象根；关闭准备对象后，用独立数据库连接和重新打开的存储读回，核对实际只打开两个已审核的解法对象，错误归属及来源锁均在对象 I/O 前拒绝。实物专项 `TestPrivateDraftConfiguredConvolutionEvidence` 通过，日志见 `/private/tmp/oj-draft-convolution-evidence-postgres-final.log`。测试使用合成会话观察，未执行沙箱中的题源，不代表作者请求或 Worker 受信报告联通。

无 schema 或运行配置变化，草稿保持 `not-run`，未部署。最终 CI 及合并状态以 PR 回读为准。

## 2026-11-11：固定卷积小规模独立对拍

基于 `origin/main 4a30fa1`，[卷积小规模独立对拍](reference-validation.md#卷积小规模独立对拍)在现有断网卷积槽内先运行固定朴素解，清理后再运行固定候选。24 点语料包含模域、手算锚点和固定种子；首个合法差异保留有界输入、哈希和两端系数，启动不明时保留可用容器身份、停止证据及 `INCOMPLETE`。

Go 1.26.11 的离线 Docker 后端格式、vet、静态分析、竞态和构建通过，另核对 sandbox 标签 vet 与 Darwin arm64 构建。真实正例 24/24 一致；专用合成错误解在第 10 点保存预期 `[3,10,8]`、实际 `[4,10,8]` 及合成版本身份，前 9 点一致，两次分别确认 `running=false pid=0`，目录/报告权限为 0700/0600。篡改一字节后拒绝，槽未重新启动且无报告目录。证据：`/private/tmp/oj-convolution-differential-final-evidence-p7SfS7`。真实错误解只在显式 sandbox 专项运行，不可从普通 CLI 注入。独立静态审查发现的启动证据缺口已修复并复核。

这是单一来源 AC05 分项，未覆盖最大规模、其他来源的独立验证、作者接口、审核或发布。草稿保持 `not-run`，报告中 `publication=blocked`，无数据库迁移和部署。

## 2026-11-11：显式准备参考计划并按来源选择一次

基于 `origin/main a05cb18`，作者可按[原申请](reference-requests.md#作者申请入口)显式启动准备并只读观察持久计划。每进程最多一个三分钟活动任务，不排队，凭据只留在内存；冻结前后检查同一会话，最终事务复核原授权、版本和申请状态。专用 Worker 可在固定槽内[按来源选择](reference-requests.md#按来源选择已准备申请)一次，schema 13 把首领历史和预留原子提交，旧计划、预留过期或未知回执均不造成自动重跑。

离线 Docker backend、临时 PostgreSQL/双 API、四包 sqlc、OpenAPI、仓库规则及前端 lint/类型/测试/构建通过。2,001 条合成申请的实际查询计划用来源索引只读八条，旧排队计数仍命中原索引。真实 PG 验证并发首领、实际丢 COMMIT、预留覆盖后无重跑、旧计划隔离与 Down13 拒绝丢历史。真实专用 mTLS 验证同一 Plan 的选择、续期和报告接纳，普通证书拒绝。异步准备在取消、退出和关闭边界不写入。

真实 Redis/PG 双 API 覆盖直连 TLS 和代理 mTLS 启动准备并跨实例读到就绪；Chromium 完成显式准备、手动查状态和取消。真实 Darwin 固定离线槽完成合成 A+B、续期拒绝后的确认停止及占槽零选择，最终 `running=false pid=0`；真实宿主使用续期/报告替身，协议和宿主分别验证。Astra 静态复核无剩余阻断，已补有界任务、客户端未知和宿主锁回归。证据：`/private/tmp/oj-reference-prepare-dispatch-{backend-final,session-final,web,targeted-final,host,browser-final}.log`。

无常驻自动调度、持久会话委托、自动重跑、实际十题端到端或部署。schema 13 仅用于临时验证库；草稿仍为 `not-run`，题面/数据质量验收、非作者终审与发布待完成。

## 2026-11-11：本人参考验证申请管理

基于 `origin/main 63581d2`，[作者申请入口](reference-requests.md#作者申请入口)在普通会话和私有草稿能力上显式启用。当前作者可创建、按原键查回执和取消本人申请；主体不匹配在访问账本前拒绝。六字段摘要不暴露材料、计划或报告正文。额度由受信维护命令单独配置，schema 仍为 12。

离线 Docker 后端、临时 PostgreSQL/双 API、四包 sqlc、OpenAPI、仓库规则及前端 lint/类型/109 项测试/构建通过。真实 Redis、PG 和直连 TLS/代理 mTLS API 验证跨进程原键恢复、账号错配、越权、撤权、退出、额度拒绝和幂等取消。数据库回读确认维护命令只配置独立参考队列。真实 Chromium 在服务端接纳后丢弃浏览器响应，刷新无 POST，原键回执和取消通过，桌面及 390px 布局无横向溢出。Astra 静态复核无阻断；补充主体不匹配零账本调用及严格正文、未知结果边界回归。证据：`/private/tmp/oj-author-reference-{backend-final,session-final,web-final,browser}.log`、`/private/tmp/oj-author-reference-browser-qa`。

本机记录按账号和草稿保留当前原键，不提供跨设备发现；HTTP 不准备材料、不调度 Worker，未运行参考题源、迁移持久库或部署。草稿仍为 `not-run`，发布保持阻断。

## 2026-11-11：接纳停止已确认的参考验证报告

基于 `origin/main 35775cd`，[参考报告接纳](reference-requests.md#参考报告接纳)只允许显式启用的专用 mTLS Worker 提交。Worker 在原槽停止、工作排空、材料关闭和隔离标记清理后冻结确定证据；控制端按原授权、固定 Profile、冻结计划和有效预留复核，并在 schema 12 同一事务保存报告、终结申请、释放排队名额和原预留。丢确认时可原样重放，不改写原报告或新预留。

离线 Docker 后端（格式、vet、lint、race、构建）、临时 PostgreSQL/双 API、四包 sqlc、OpenAPI 和仓库规则通过。真实 PG/mTLS 验证首报竞争、有效期内接纳、跨期锁等待拒绝、旧授权或凭据拒绝、实际丢 COMMIT 回执及 Down12 等待并拒绝丢历史。2,001 条合成记录上的权威重放查询使用主键索引并返回一行。Darwin arm64 使用固定离线槽运行合成 A+B，确认停止后发送证据；模拟续期拒绝时确认停止且不发送，两次均释放锁，最终回读 `running=false pid=0`。Astra 复核发现的资源预算校验缺口已修复并复核通过。证据：`/private/tmp/oj-reference-report-{verified,host}.log`。

宿主专项用续期与报告替身，真实 PG/mTLS 专项与真实宿主分别验证。没有重跑实际十题、接入作者 HTTP、改动草稿 `not-run` 或推进发布；未迁移已有持久库或部署。

## 2026-11-11：持槽执行冻结参考材料并确认停止

基于 `origin/main 9aa310e`，[同机执行](reference-requests.md#同机持槽参考执行)显式启用后，在同一宿主与固定槽锁内完成原申请的领取、续期、材料校验、启动、执行和停止确认。续期只延长原活令牌，使用独立连接和单调监督预算；下载期限保持不变。未知续期取消执行，启动未知但有绑定容器时仍尝试停止，停止未确认或清标记失败时保持隔离。数据库过期不是停机证据；不支持跨宿主或节点迁移。

最终离线 Docker backend、临时 PostgreSQL/双 API、四包 sqlc、OpenAPI 和仓库规则通过。真实 PG/mTLS 验证续期 opt-in、普通判题证书拒绝、锁等待跨期及旧令牌拒绝、取消/撤权重授、节点禁用/凭据轮换、草稿漂移和实际丢 COMMIT 回执。续期 EXPLAIN ANALYZE 命中单行主键。race 覆盖迟到回执、快照取消不可复活、启动取消/回执不明及停止隔离。Darwin arm64 使用既有固定离线槽完成合成 A+B；再次启动后在编译前注入续期拒绝，均确认原槽停止且可重新取得锁，回读 `running=false pid=0`。Astra 静态复核无剩余代码阻断。日志：`/private/tmp/oj-reference-execution-{final,host}.log`。

控制端真实 PG/mTLS 和真实宿主停止分别验证；宿主专项使用合成材料与续期替身。没有运行实际十题、端到端部署或容量验收。schema 仍为 11，无已有持久库迁移；草稿状态不变，可信报告接纳及作者入口待后续交付。

## 2026-11-11：按验收目标划分 PR

根规则、[开发工作流](development-workflow.md#分支提交与合并)和 PR 模板明确：同一功能或阶段的必要步骤可以在同一 PR 中分成多个原子提交；具备独立交付价值、需要单独合并或回退，或风险明显不同且能独立验证时再拆 PR。仍审查完整差异，默认 squash，保留既有门禁。pre-push 在拒绝直推或删除主线时提示将相关提交推到任务分支，未增加提交数或语义门禁。

本地治理 101 项通过。真实 Git 夹具验证第二次提交的无效标题被拒绝、修正后两个原子提交完整到达临时远端，以及直推 main 被拒绝。仓库规则、shell 语法及 diff 检查通过。改动仅涉及交付治理，无应用、CI 配置或远端保护变更。

## 2026-11-11：有效参考预留内下发并校验材料

基于 `origin/main 3f74068`，[材料接口](reference-requests.md#有效预留内的材料下发)在显式配置私有对象根后，通过专用 mTLS 返回原冻结计划和计划内单个对象。对象读取前后分别以短事务重验节点、原授权、申请/草稿/固定配置、计划及预留；预留锁等待后重新检查数据库时钟。传输设置真实连接写期限。Worker 在同一宿主槽内完成领取和逐对象校验，以固定预算、完整测试目录及依赖闭包冻结匿名磁盘快照；没有全题内存缓存、自动重试或续期。schema 保持 11，没有新索引或持久写入。

已有镜像离线运行完整 Docker backend（格式/vet/lint/race/构建）、临时 PostgreSQL/双 API 集成和四包 sqlc 一致性，全部退出 0。真实 PG/mTLS 验证完整材料传输、普通证书隔离、计划外对象拒绝、原授权及预留失效、读取中取消/重授/到期/节点轮换、锁等待跨过期以及读取不续期。单行预留的两条查询有 EXPLAIN ANALYZE 证据；服务及 Worker 都顺序释放对象，MaxInFlight=1 用例通过。真实 TLS 慢读取连接被截止，客户端覆盖规范计划/摘要/预算/往返期限、损坏/截断/末对象失败与配额回收，十种合成来源及目录子集/顺序/依赖漂移覆盖通过。Darwin arm64 两项原生受信测试确认同一宿主锁贯穿下载错误路径及固定槽映射。Astra 完整差异复核无阻断，补强建议已验证；AI 复核不是人工审批。

首轮数据库准备缺少命令构建，损坏计划夹具未同步摘要而触发 schema 约束；随后修正并使用正式工具链重验。质量检查发现的测试导入名冲突、未检查的 Close 及冗余选择器已修正，未降低规则。证据：`/private/tmp/oj-reference-material-final.log`、`/private/tmp/oj-reference-material-catalog.log`、`/private/tmp/oj-reference-material-quality-native-build.log`、`/private/tmp/oj-reference-material-darwin.log`。测试均为合成材料，无题源执行。只验证冻结计划所绑定的材料，不重新计算完整导入来源；30 秒大题低带宽下载可能失败。命令结束关闭快照并释放槽，持续持槽执行、续期及可信报告未接入；未迁移既有持久库、部署或进行目标服务器容量验收。

## 2026-11-10：专用参考 Worker 受权领取

基于 `origin/main dcededd`，[专用节点领取](reference-requests.md#专用-worker-领取)通过默认关闭的 mTLS 路由登记一次 30 秒预留。命令先取得宿主和固定槽锁；最终事务复核节点、申请状态、原授权、草稿、当前固定配置及预期 Plan。活预留不重写，过期后才可用新令牌；未知回执不自动重试。schema 11 只保存一条预留，历史非空时拒绝回退。

离线 Docker 完整 backend、四包 sqlc 生成一致性、真实临时 PostgreSQL/双 API、OpenAPI YAML 解析与仓库规则通过。真实 mTLS 验证专用证书领取、普通证书与参考证书双向拒绝、无配置不开路由；数据库验证撤权重授、取消行锁交错、错误 Plan/配置、单预留竞争和过期换令牌、实际丢 COMMIT 回包及 Down11 等待后拒绝非空。2001 行合成分布中申请/Plan 锁查询命中主键索引；Darwin arm64 两项原生受信用例验证占槽零请求及十来源槽映射。首轮集成夹具编译问题修正后整套通过。Astra 复核无阻断。日志：`/private/tmp/oj-reference-worker-claim-final-{backend,db}.log`、`/private/tmp/oj-reference-worker-claim-darwin.log`。

领取命令退出后释放本地锁，不启动沙箱、下发材料、续租或接收报告。没有运行真实题源、改动既有持久数据库或部署。

## 2026-11-10：判题与参考验证默认共享宿主执行槽

基于 `origin/main 6712da3`，[宿主门禁](architecture/api-worker-design.md#同机共享执行槽)覆盖判题、参考验证（含 A+B）及恢复 CLI。默认先持有同账号宿主独占锁，再检查五个物理槽并取得目标锁；进程退出后仍以原槽持久标记阻断正常执行。默认恢复只清理已确认停止的目标。历史双 Worker / 槽恢复专项显式使用双槽模式；忙且空或未知标记不视为兼容。

已有工具镜像离线运行完整 Docker backend，退出 0；Docker 交叉构建后在 Darwin arm64 原生运行五项受信测试，全部通过。验证实际跨进程互斥、异常退出遗留标记、失败恢复仍封锁、逐槽恢复、双槽实验与参考互斥及异常锁文件拒绝。仓库规则通过，Astra 复核无阻断。日志：`/private/tmp/oj-shared-host-slot-{backend,darwin}.log`。本轮没有运行真实沙箱或历史双 Worker 专项。

无数据库、控制端协议或执行预算变化。升级/回退须先排空旧二进制与直连驱动；没有跨 UID、跨主机、断电或生产容量验收。持久化参考申请 Worker 领取与门禁接线、可信报告仍待实现，未部署。

## 2026-11-10：冻结后复核授权并持久化预期计划

基于 `origin/main d0bfd97`，[内部 PlanService](reference-requests.md#实时复核与预期计划写入) 冻结并关闭材料快照后重新观察同一会话，最终 PostgreSQL 事务锁住实时身份、角色、本人草稿和申请行，要求仍为 queued 且原授权快照及原配置未变。schema 10 保存原 Plan JSON 与摘要，原键只读恢复未知提交；相同计划幂等，不同计划拒绝。取消保留历史且不产生判定。

离线 Docker 参考申请包 race 基线、完整 backend、四包 sqlc 生成一致性、最终临时 PostgreSQL/双 API 集成和仓库规则均通过。测试使用真实对象根和合成会话观察，覆盖冻结后取消、撤权与重授、账号/恢复代次与 Profile 变化、同申请并发、真实 COMMIT 回复丢失、授权锁持有至提交、摘要/长度/外键约束和历史编码拒绝；实际 Down10 等待读写事务，空计划可回退并保留原账本，非空计划拒绝。2001 行合成分布的权威查询使用预期计划主键索引，不代表生产容量。Astra 静态复核无阻断。日志：`/private/tmp/oj-expected-plan-{baseline,backend,sqlc,db-final}.log`。

无公共接口、Worker 领取、共享执行槽或报告接纳。未运行真实十题，未迁移已有持久库或部署。

## 2026-11-10：从持久化对象存储冻结参考验证计划

基于 `origin/main ff0d4f9`，新增[内部冻结入口](reference-requests.md#从持久化对象冻结参考计划)。核对固定导入修订和原 Profile 字节后，沿用维护入口的来源布局、完整点目录和材料预算，逐对象验证并冻结参考源码、编译依赖及所有点材料。匿名快照与维护磁盘快照共用进程内单槽；关闭对象存储后仍可读取冻结材料。

现有 Docker 工具链离线运行四包 race 基线、最终 `toolchain backend` 与 `toolchain db`，均退出 0；仓库规则通过。十种合成来源的完整 Plan/摘要与维护入口一致，覆盖四个依赖的顺序和原始字节、单操作对象存储、共享快照槽、缺失/同长度损坏、末对象取消及失败后重新冻结。真实临时 PostgreSQL 和对象根验证写入后移除原 bundle，重开对象根、换连接读回申请与草稿，仍能冻结同一 Plan 并读出相同字节。首轮后端夹具目录被私有权限检查拒绝，改为显式创建 0700 子目录后全套通过；存储规则保持。Astra 只读复核无阻断。日志为 `/private/tmp/oj-freeze-persisted-baseline.log` 和 `/private/tmp/oj-freeze-persisted-verified-{backend,db}.log`。

未用真实十题大语料或运行题源；没有迁移、新接口或部署。控制端尚未持久化 expected Plan，实时授权复核、Worker 消费、跨任务执行槽、可信报告与发布仍待完成。

## 2026-11-10：参考验证申请绑定已入库草稿引用

从 `origin/main 1615d66` 建立独立任务分支。[内部绑定](reference-requests.md#有界排队与未完成边界)重新计算原始文档与完整引用的导入修订，按逻辑名的字节顺序排序副本，保持既有导入算法。草稿 ID、所有者、版本、来源锁或引用不符时拒绝；回执不代表当前授权或队列状态。

现有 Docker 工具链离线运行改前两个相关包的 race 基线、完整 `toolchain backend` 和 `toolchain db`，均退出 0。真实临时 PostgreSQL 验证回执和草稿重读后仍可匹配已关闭的导入准备；仅移除来源验证证据引用，保留版本与修订字段，绑定即拒绝。单测另覆盖引用乱序、别名、文档变更、引用增删替换、取消及规格损坏。仓库检查通过，Astra 针对性只读复核无阻断。日志为 `/private/tmp/oj-reference-draft-material-{baseline,backend,db}.log`。

本项不新增 SQL、迁移或 HTTP；持久化对象读取、冻结 Plan、受权消费和可信报告仍待实现，草稿验证状态与发布阻断保持，未部署。

## 2026-11-04：持久化参考验证申请

从成功fetch的`origin/main 26a9882d0aebc4b60392a8b93f143e9559704303`建立独立`codex/persistent-reference-requests`。新增[内部申请用例](reference-requests.md)及独立PG/sqlc包，固定本人草稿、完整导入/来源锁、Profile原始字节及原授权快照；提供原键回执和queued→cancelled幂等取消。schema9默认空policy，只有显式维护配置才可申请；恢复→账号→角色→草稿→policy→申请行锁，原键在目录/额度检查前恢复，policy锁后重查避免最后额度竞争误拒绝。读/取消不更新旧权限，未知写回执保持原键恢复；无HTTP、消费、Plan/attempt/判定字段，草稿not-run及发布阻断保持。

既有离线隔离工具链`sha256:4b016e9e3915f1cacb1d70c1ea0507d8a987822efc020c6d8ce17247d0936fec`完成完整backend格式/vet/静态正反例/race/七命令构建、四包sqlc生成一致性、真实临时PG及双API进程检查，最终退出0。日志`/private/tmp/oj-reference-requests-final-checks-20261104.log` SHA-256 `23c805cb9c6990080e9aa505e24fa1effad6b59f99f3d53370a21cca665e844f`。新增12顶层/27子测试通过，包含强制两调用实际等待policy后同键恢复/异草稿冲突、跨owner/全局额度、取消竞争、撤权与死事务、实际COMMIT回包丢失、缺policy/退役后原键恢复、重授后旧快照保持及新请求revision3、非空回退和查询计划。10,001行/30queued合成分布的部分索引16,384B，仅为该分布证据，不是生产容量。实际Service→PG在移除fixture材料后完成申请/回读/取消及撤权拒绝，会话观察明确stub。

首轮PG仅旧迁移测试失败：两处依赖Down次数的测试须改为显式DownTo7，保留角色/审计非空拒绝及旧schema门禁断言；修后整套通过。Astra复核发现并由真实PG复现Down9初稿的表锁交错：请求读取后/草稿锁前的旧例明确`40P01`，空policy授权后/请求读前的旧例仅确认ErrStore，不扩张具体SQLSTATE结论。修正为凭据→申请→草稿→policy表锁，包含DROP外键触发器父表需求；两个确定性交错分别让申请提交后拒绝删除历史、缺policy调用安全拒绝后空库Down到8/重Up9。前后日志`/private/tmp/oj-reference-request-races-{before,fixed}-20261104.log`保留；业务行锁与迁移表锁顺序不能混用，维护窗口要求保持。

治理首轮受普通沙箱禁止回环bind影响，两项环境检查失败；按正常本地测试权限重验101项全通过，日志`/private/tmp/oj-reference-requests-governance-fixed-20261104.log`，未改断言。主代理与Sol实现/自审，Astra针对性设计及完整高风险复核无剩余代码阻断；AI不代替人工批准。最后仅修正两处注释及文档，不改变已测逻辑。实际原十题bundle另经只读元数据审计：关闭全部Prepared后十来源仍匹配配置，Profile原字节最大1,484B，输出`/private/tmp/oj-reference-requests-ten-catalog-20261104.json` SHA-256 `2be7412c19f528a9bd356edcef9cc61381c3f217b256ebe470f0bd8d9508fc19`；未连接数据库、读取大点数据或执行题源，不冒充实际十题任务入库/消费。

永久撤权queued仍占额度，历史增长、跨判题/参考共享槽、持久化材料读取、可信报告、真实会话HTTP及完整P4尚未完成。现有localdev持久库及目标服务器未迁移、未部署；4核4GB/3Mbps同机容量与100人混合负载待实测。最新head CI/合并结果以交付回读为准。

## 2026-11-04：同事务出题授权修订快照

从成功fetch的`origin/main 9a09be17e8a1a889d86899e3f6fb7d13a972e80c`建立独立`codex/author-authorization-snapshot`。新增identity公开`ProblemAuthorSnapshot{Actor,RoleRevision}`及`ProblemAccess.LockAuthorSnapshot`，保留旧`LockAuthor`接口并委托同一路径；恢复→账号→角色共享锁仍属于调用方READ COMMITTED事务，全部检查及最终取消检查成功后才返回快照，所有失败返回零值。用途与限制归[私有草稿合同](problem-drafts.md#入库版本与迁移)：后续任务需重新锁定并比较完整快照，不能把当前enabled或旧快照当作权限；持久化任务、可信报告接纳及发布尚未接入。

现有隔离Docker工具链`sha256:4b016e9e3915f1cacb1d70c1ea0507d8a987822efc020c6d8ce17247d0936fec`以离线网络运行identity race基线，随后完整`toolchain backend`、`toolchain sqlc`和`toolchain db`均退出0；覆盖格式/vet/静态检测器正反例/全race/七命令构建、生成一致性、私有临时PG集成及双API实际进程，未连接既有数据库或执行题源。日志`/private/tmp/oj-author-snapshot-checks-20261104.log`。新增五顶层/15子测试通过：撤权拒绝/重授revision变化；credential-only rehash保持、改密/恢复代次隔离；11失败路径零快照；三行独立NOWAIT探针证明提交/回滚前持锁及之后释放；实际role等待后读取已提交撤权并拒绝。既有私有草稿授权/撤销并发覆盖继续通过。Astra设计及完整实现只读复核无阻断，不能代替真实人工批准。

本次无SQL、迁移、依赖或外部HTTP合同变更；尚未建立队列、租约或跨判题/参考任务的共享槽额度。四核不等于四槽，4GB同机容量仍须目标服务器实测，草稿`not-run`和发布阻断保持。最终head的CI与合并状态以交付回读为准。

交付回读：[PR #126](https://github.com/DL-Association-for-Computing-Machinery/OnlineJudge/pull/126)精确head`51d1f378dd98dbf945718e493ff9db3ba878bb3d`的[CI 41769325261](https://github.com/DL-Association-for-Computing-Machinery/OnlineJudge/actions/runs/41769325261)八项成功、frontend-tests按范围跳过，四必需门禁成功；无阻断讨论且普通ruleset满足后正常match-head squash为`26a9882d0aebc4b60392a8b93f143e9559704303`。树与最终head一致，成功fetch/prune后仅快进main；七份忽略制品及完整head bundle逐哈希归档于`/private/tmp/oj-pr126-delivery-archive-20261104`，manifest SHA-256 `8fbbb4f57699dd583c45d539986a6240abc648e5ad4b88cd4bce92a7ebf840c1`，完成的分支/worktree清理。五页知识事务及严格lint通过；本机临时归档不等于独立备份。

## 2026-11-04：convolution_mod 十来源固定环境接纳

冻结源码HEAD `f04dfdfe7d3f8fb3abdfcb61518e2fe176577291`已吸收服务器资源约定纯文档PR #124的主线提交`9b144b0cadedccb8f8e7759343801b2937d6c233`，全部Go/config与已完成离线检查的候选保持。完整backend/race/静态、sandbox vet及Darwin构建通过（`/private/tmp/oj-convolution-backend-20261104.log`）。新增独立tier8/profile/runtime/files和第四槽，旧九来源预算与摘要保持；16MiB input/answer/stdout、512MiB含catalog快照及2GiB累计传输只准入该固定来源，不扩张旧medium槽或生产预算。

原始草稿version `152d7b4c5817c9dfc8f58eaaaaf1e40a6707793ae9213345303d9888bb5f1682`，import revision `bcedb2583e67cba2f61d7d62fb377581012a506acbbb98467fccd8c89461e145`。新题133个原始引用对象426871481B，完整运行Plan材料426604858B（含source/catalog）；十来源全部对象按hash去重为597个、1460825722B。独立未执行题源的准备基准`.local/convolution-plan.json` SHA-256 `85e9b6ef6f241ca2e71d0cbaaeead860f427f96a7cdb699d3e0e6f68bd7af4ff`，plan `4077fcf727e1b6238fae27752b6627ccd16ab2adcaa96631097e6b0d2271aa26`、profile `7311e207d680b3be9276a7a57734785551be167aafa8a39b59148c61f7c9544d`。本次新报告与该非运行基准相等，旧九报告完整Plan/Profile逐字段相等；准备基准从未冒充成功运行报告。

一次协议`/private/tmp/oj-convolution-trial-20261104-protocol.json` SHA-256 `2316e399d1b8995fc4fcbc1634afd9e9f311d0321817990f661162077a924236`冻结最终HEAD、全部Go/config/制品、脚本/helpers、准备基准、槽创建回执、十题版本/来源/有序点集与材料。保持固定Desktop LinuxKit7.0.14/arm64、Engine29.8.1、go-judge v1.13.3/GCC15.3.0；共享独占令牌、原驱动canonical锁、每套`count=1/failfast/timeout5m`、原内部期限和Stop总预算，无重试或恢复。先新题两组，再staticrmq、point_add_range_sum、suffixarray、A+B、unionfind、zalgorithm、primality_test、static_range_sum、associative_array完整原suite；十来源254点AC/Stop、十顶层/26原子子测试RUN/PASS清单均精确匹配。

| 来源 | 点数 | 整套耗时（秒） | report.json SHA-256 |
| --- | --- | --- | --- |
| convolution_mod | 53 | 31.55 | `9a64657544ca14e9d7c590c60cf83243207b48f2e1ee03749866848d3a644fdc` |
| staticrmq | 27 | 16.03 | `a3f6e0b53aad1802b1133bb2a91f0c5d8d6e6c7109f8a05bf54bcf8b87f87932` |
| point_add_range_sum | 21 | 12.97 | `90b9b9790dc3f3f1c0acbc95d4bac20e299fad5ddf56af3257f84e71244aca55` |
| suffixarray | 50 | 20.53 | `b4a22ff8ca9e77a0dcbdceba2bf9cea6554a548490104e4fa773c75b3e13e965` |
| A+B | 12 | 18.96 | `a1a91b9466e5c06072a53e1758ce7204b08d8da9d8fdea0ec777cf6a9cc74071` |
| unionfind | 18 | 10.47 | `339c35554b220023777d3e86302812f84d40876a477d766c46ea52bc87b36520` |
| zalgorithm | 29 | 16.76 | `3e14871858d1663b9ace651f4ad63f148de929997c546dcb1357f2587df4fc6a` |
| primality_test | 13 | 9.70 | `881ee8c0bae897a9ddfc2999a9cccf43807ed9932dac3be720f8b3ba022f0c51` |
| static_range_sum | 11 | 10.11 | `f762f25e56744389dafbbec392963993cf0e3b9597dac41586bd0fd127d7ee42` |
| associative_array | 20 | 22.12 | `ffe7482af192d1dbf94ac45bd92fe0b95fb91558d90769dab212d604b02c687e` |

所有报告位于`/private/tmp/oj-convolution-trial-20261104-<source>-report/report.json`，日志、observer结果、独立postflight及验证记录同前缀。汇总`/private/tmp/oj-convolution-trial-20261104-summary.json` SHA-256 `515bf641c50b9e0b8391a8ca9b53d3bd2c6476e3a7c00490bfcc0133f68d1798`；报告目录0700/文件0600、无INCOMPLETE/成功诊断，publication blocked，observer无错误/截断。新题字节组同次16MiB输入/16MiB原始输出/stderr4096全字节核验、stdout cap+1完整窄SIGXFSZ证据及stderr4097严格拒绝均通过，再编译一次完成53点。完整CLI逐点CPU最大191141000ns、memory最大23576576B；CLI wait4-maxrss为129236992B，字节组outer memory.peak样本170569728B。这些计数口径不同，不能相加、充当连续总峰值或证明4核4GB同机服务器容量；本次没有服务器部署或资源实测。

第四槽仅创建一次，回执`/private/tmp/oj-convolution-slot-created-20261104.json` SHA-256 `09fd475857819fbe8f4398d8973153ab5a1910b897f9e7240b5927a8a60e01f5`保留；原full CID `7e6784da27313d56d02d778ee368597e3ef3a3411f5561a17a0422d33712691b`经驱动原有start/Stop生命周期完成，本次最终停止/PID0/私有空锁无占用，原三槽ID/固定配置保留且亦确认停止。本次四槽顺序检查均完成成功，不能将顺序采样宣称为原子快照。外置`save_new`未fsync；排他创建、一次标记及本次成功读回不构成断电恢复下的exactly-once保证或独立备份。冻结Go源码与制品未变，此后仅更新五页文档，不追加真实验收。

旧suffixarray/staticrmq失败及全部历史调查保持，根因和长期稳定仍未知，不追认原失败。当前十题254点只完成固定环境维护参考验证；持久化验证任务、权限下报告接纳、题面/数据质量、非作者终审与原子发布仍未完成，草稿not-run/生产null/publication blocked不变。最终CI与合并状态见下方交付回读，未部署。

交付回读：[PR #125](https://github.com/DL-Association-for-Computing-Machinery/OnlineJudge/pull/125)最终head`6e4c8cd470c7004a58e297db11e007447b262ef2`的[CI 41767312377](https://github.com/DL-Association-for-Computing-Machinery/OnlineJudge/actions/runs/41767312377)九项全成功，保护与讨论核验后正常match-head squash为`9a09be17e8a1a889d86899e3f6fb7d13a972e80c`。合并树与最终head一致，成功fetch后主工作区仅快进；66份证据/制品含11份忽略资料及完整head bundle逐哈希归档于`/private/tmp/oj-pr125-delivery-archive-20261104`（manifest SHA-256 `bd8e93512732fd1fdee737c354753cb53e16dcb7f884aa9f08ee01a6c99acc38`），完成的本地分支/worktree已清理。知识库六页事务及严格lint通过；归档仍为本机临时材料，不是独立备份。

## 2026-11-04：convolution_mod完整材料与独立槽候选

成功fetch后从`origin/main eb51ec9f36822a26b4cb1517566858dcdf733560`建立独立`codex/convolution-reference`。固定53点的133个有效对象引用共426871481B逐哈希核验，完整草稿SHA `7531580fa33906de971ff6b887c22c6ed5340ec4009fbbebb007cbadc0d18826`；源码3284B仅标准头，无编译依赖。统计及对象身份见`/private/tmp/oj-convolution-material-audit-20261104.json`，预算推导归[候选合同](reference-validation.md#convolution_mod参考验证)。材料统计不证明程序CPU/内存或整体3分钟足够。

Astra设计及完整20文件差异复核无阻断；独立tier8及16MiB输出槽保持原封闭Host/Cached/Runner和Stop门禁。通用48MiB输入与16MiB输出预留超过原64MiB live，回归核对零POST/run并清理已知input；合法16MiB输入/输出预留为33MiB+1。完整离线Docker backend格式/vet/静态/race/七Linux命令、sandbox vet及独立Darwin CLI/驱动构建通过，日志`/private/tmp/oj-convolution-backend-20261104.log`；旧九profile摘要、完整53点合成账本/325操作、材料512MiB含catalog精确/+1及双端2GiB边界均通过。AI复核不替代人工审批。

独立只读准备工具已实际冻结完整53点并检查全部答案的文本域，Plan摘要`4077fcf727e1b6238fae27752b6627ccd16ab2adcaa96631097e6b0d2271aa26`、Profile摘要`7311e207d680b3be9276a7a57734785551be167aafa8a39b59148c61f7c9544d`，证据`.local/convolution-plan.json` SHA `85e9b6ef6f241ca2e71d0cbaaeead860f427f96a7cdb699d3e0e6f68bd7af4ff`。辅助程序首次编译因LoadBundle锁参数类型错误失败，修为有界/无链接/常规文件读取及固定锁哈希校验，并补结束context/Sync后，原样材料预检通过（`oj-convolution-plan-preflight-fixed-20261104.log`）；未执行题源。真实字节边界、53点和旧九来源回归均待执行，尚未配置新槽。主线九来源及历史失败结论不因候选改变，数据库not-run/生产null/publication blocked保持，未部署。

## 2026-11-04：staticrmq 九来源固定环境接纳

候选HEAD `e777ae2b985193ddaf1ec8d647d8697aa13c5964`已吸收主线前置：停止预算修复#123提交`5411cebc283fc1d3f358f36c72be4058ae2c812d`，以及已正常合并#114的squash提交`b20900b2567f959879657d18a196dd6aba86b41a`。完整28文件差异已审查；完整backend、sandbox vet与Darwin构建通过，日志`/private/tmp/oj-staticrmq-readmission-final-backend-20261104.log`。静态/合成回归明确验证Medium与MediumBulk相同Runtime合法互验，同时StaticRMQ与Bulk/其他来源双向拒绝、单文件档位拒绝借用Primality依赖入口，未知档位为0/255。StaticRMQ原Plan/Profile及材料、运行、传输、清理与Stop预算保持；AI审查不代替人工审批。

一次协议`/private/tmp/oj-staticrmq-readmission-20261104-protocol.json` SHA-256 `0672c032000893190783ba9c35487413eaa9b2c5c3ed7c8fb67cf6a051da6573`冻结最终HEAD、全Go源码/配置/制品、九来源旧完整Plan/Profile及草稿版本/来源/有序点集与材料，核验两个交付SHA均为main和最终HEAD祖先。使用共享独占token、真实canonical锁与三原槽完整配置/停止/PID0/私有空锁前后检查；固定Desktop LinuxKit7.0.14/arm64、Engine29.8.1及原go-judge v1.13.3/GCC15.3.0不变。

顺序先staticrmq原两组（可信16MiB stdin、完整27点），再point_add_range_sum原两组及其余七来源完整原suite。每套只运行一次`count=1/failfast/timeout5m`；无重试、恢复或预算追加。全部201点AC/Stop，九顶层/24原子子测试RUN/PASS精确匹配；旧完整Plan/Profile逐字段相等，报告0700/0600、无INCOMPLETE、成功诊断省略、publication blocked。

| 来源 | 点数 | 整套耗时（秒） | report.json SHA-256 |
| --- | --- | --- | --- |
| staticrmq | 27 | 16.51 | `5ef7de916e34de90a92cebe7d1951b6e632a5527a2db4b272a4de0f35ae6fed0` |
| point_add_range_sum | 21 | 12.47 | `407bb08095b093f6eab33f9b78c18698d8455b8c714a2930af5210d8d7d5c08b` |
| suffixarray | 50 | 20.06 | `513a6c5e32e6ecf22ba87445387289e4e04b35be8c25a069fc100f6d6a3d2049` |
| A+B | 12 | 18.56 | `a77ae4fca7f6d4f4cb3629a6f5c41ab241c754bb47f0306553ae9d00f1d71c6e` |
| unionfind | 18 | 10.11 | `14ddbd0030d5f0e30817157f9d8a8a8681ae5aef1f9c7dcb2b52ded4fd4a0487` |
| zalgorithm | 29 | 16.49 | `b95e95e7fd178f694c5d2b56d7a091fa40479213822b774f2f5baab24fa80cd4` |
| primality_test | 13 | 9.25 | `eea8e1b09338a060c6a55d1edb9e672d6415a2fcc77b971c74852ff5fae1f752` |
| static_range_sum | 11 | 9.96 | `d6191ad232aacd7b59ec19c769f36252e937513a6e9f08e168b532e4eb7720f1` |
| associative_array | 20 | 20.97 | `5d5b9c5adb3ed1288c916ea044a6663eea47da55e94df3b3518d3425aedd1557` |

各报告位于`/private/tmp/oj-staticrmq-readmission-20261104-<source>-report/report.json`，日志、observer结果/错误、独立postflight及验证记录同前缀。汇总`/private/tmp/oj-staticrmq-readmission-20261104-summary.json` SHA-256 `cba63461d95c981f872c10debfaa2a1c584e5ada4881dbbfe0836d4b5820fc44`。全部observer无错误/截断，结束时三原槽停止/PID0、私有空锁无占用；冻结Go源码与制品未变，此后仅同步六页文档，未追加真实验收。

原staticrmq失败`ee524a329fbfcc5d654e3ff761ce310dc0f1dee43b089dfa3bb5332a816d2d69`、旧suffixarray失败与历史调查完整保留，根因及长期稳定性仍未知，不因新通过追认旧失败。当前版本九来源通过固定环境维护入口接纳，仅convolution_mod仍待独立准入；最终CI与合并状态见[PR #119](https://github.com/DL-Association-for-Computing-Machinery/OnlineJudge/pull/119)。不更新草稿not-run、生产null或publication blocked，不等于持久化验证任务/质量审核/题目发布/整体P4或M0通过，未部署；临时证据不是独立备份。

## 2026-11-04：staticrmq 吸收已交付八来源与停止修复（待验收）

[PR #114](https://github.com/DL-Association-for-Computing-Machinery/OnlineJudge/pull/114)最终head `86fe1311707b1a94569e3381f8eb6c86eaf8ad3c`的九项CI全部成功后，正常squash为main提交`b20900b2567f959879657d18a196dd6aba86b41a`；已fetch并核对squash树与最终head相等。#119随后普通合入该main，完整吸收八来源及[Stop修复#123](https://github.com/DL-Association-for-Computing-Machinery/OnlineJudge/pull/123)交付SHA `5411cebc283fc1d3f358f36c72be4058ae2c812d`，不重写历史。

staticrmq保持独立枚举5、原Plan/Profile、材料/传输/运行预算和原两组；不得因与Bulk共用medium物理槽或同为payload512MiB而归一化身份。未知档位仅保留0/255；明确验证Medium/Bulk的相同Runtime合法，同时保持staticrmq对Bulk及依赖编译入口的拒绝。完整28文件差异经主代理及Astra复核无阻断；离线Docker完整backend格式/vet/静态/race/七Linux构建、sandbox vet和Darwin CLI/驱动构建全部通过，日志`/private/tmp/oj-staticrmq-readmission-final-backend-20261104.log`。本轮九来源独立接纳尚未执行；旧`ee524a…`失败与单次未复现观测保持，不能提前宣称staticrmq通过、已交付或历史根因修复。

## 2026-11-04：staticrmq 候选整合七来源主线（未验收）

候选#119普通合入`origin/main 2499cc3`，保留主线已交付七来源与全部停止/执行/Host诊断，以及staticrmq枚举5的原Plan/Profile和预算；尚未引入#114的point_add_range_sum档位3。可信16MiB探针仅将Verify包装期限收紧为10秒，原两组顺序、Compile/Run/独立制品清理期限与所有字节断言保持。本轮完整backend首次因static_range_sum旧测试仍将staticrmq视为未准入而失败；修正后定向problemvalidation race、sandbox vet及七Linux命令构建通过（`/private/tmp/oj-staticrmq-readmission-integration-{backend,fix}-20261104.log`），仓库policy通过。未执行本轮真实题源验证，后续仍须吸收新的Stop修复与已交付前置；不得据整合、旧27AC或普通CI解除原失败阻断。

## 2026-11-04：staticrmq单次Host边界观测未复现（仍为草稿）

主线[PR #120](https://github.com/DL-Association-for-Computing-Machinery/OnlineJudge/pull/120)已交付Host失败诊断。本任务从成功fetch的`origin/main 8e5001063001d670e69e8ebb0a964b0adaab87bc`普通合入原#119分支为`a8aa5fc135b345d35658ce88866ccad93b7a41fa`，只解决README与本页相邻说明冲突，保留双方有效记录，不重写历史。旧候选生产代码未改，主线Host Go文件逐字节一致；完整离线backend模块/格式/vet/静态/root race/七Linux构建、sandbox vet及独立Darwin构建通过，精确顶层测试清单核验，policy688通过。日志`/private/tmp/oj-staticrmq-host-observation-merge-backend.log`；Astra窄整合复核无新增阻断。

原失败16工件/19源码与两宿主制品先哈希核验，旧Go仅接受主线测试keyed literal转换；完整57对象200820493B仍与旧材料证据一致。单次观测先登记H1与支持/不支持/未知条件：只区分Host失败边界和真实CLI启动，不推断daemon派发或根因；进入阶段context/期限不是实际调用瞬间保证。编排日志创建/失败退出和在途Docker命令检查在执行前修正，原v1未执行，修正后经Astra复核。v2协议SHA `1b5995a1274ea83762a45ec0df954d17ebb7759df1f8a410027309df343c2293`，绑定head、两制品及脚本/材料身份，所有预算/期限和原两组顺序保持；一次性标记、新报告目录与首组失败中止生效。

一次双组19.83秒通过：可信16MiB输入2.57秒逐字节/长度核验，CPU4548000ns/Memory1306624B；shm活跃16850944B后0、tmp观测0、outer memory.peak阶段最大111353856B，不等于连续总RSS。完整27点CLI17.25秒全部AC并确认Stop，编译CPU193032000ns/Memory77352960B、逐点最大CPU148162000ns/Memory44752896B、Darwin CLI wait4 maxrss100564992B。新报告`/private/tmp/oj-staticrmq-host-observation-report-20261104/report.json` SHA `3d248ead0bd03e47853c7d53fae6a9ceb5d7fd4c73491349875c913fd012b55c`，完整plan/profile与原失败报告逐字段一致，旧摘要保持；0700/0600、无INCOMPLETE、成功execution/stop诊断省略，publication仍blocked。结束时三原CID均停止/PID0、私有空锁无占用，无重试、恢复或手清marker。

原CID两个固定窗口的有界脱敏日志分别13/169条HTTP全200，无采集截断；最大service/Docker时间差约0.293/0.335ms。前置StartedAt未知，窗口只是从宿主运行开始到目标StartedAt的采集范围；目标使用收尾inspect生命周期，观察到`===RUN`的UTC不是精确子组起点。无失败条目，H1未获支持也未被反驳，仅本条件下未复现；旧报告/失败和根因未知保持。完整协议/窗口/证据限制见[调查](reference-validation-investigation.md#staticrmq单次host边界观测)，工件前缀`/private/tmp/oj-staticrmq-host-observation-`。

#119仍草稿，#114保持；旧五来源真实回归未启动，不追加相同试验或自动解除准入阻断。没有新依赖、SQL/schema、HTTP/UI、预算、数据库验证状态或发布变化；主线仍五来源，P4与开发目标未完成，未部署。此次新应用代码仅来自已交付主线；本地未重跑PG/Redis、前端/浏览器或完整M0，更新后精确head远端CI另核验。以下保留此前各阶段记录。

## 2026-11-04：staticrmq候选与第9点控制期限失败（未交付）

成功fetch后从`origin/main 31474f2a19280f7b9d53a64ca6987cb0ba5af6dc`建立`codex/staticrmq-reference`，目标为固定staticrmq完整27点，独立于草稿#114。本候选真实验证失败，保持草稿、不合并；主线仍仅五来源。没有新依赖、SQL/schema、HTTP/UI、生产预算、数据库验证状态或发布变化。

同一私有bundle的source/catalog/完整27组input-answer共57对象、200820493B逐字节哈希一致；证据`/private/tmp/oj-staticrmq-complete-material-evidence-20261104.json` SHA `55b3afa8c3d29c7e27d11aefb6c092a7f5ce984e7cc6d079ae633e7c093fae3d`。源码1890B SHA `a5586c48fa83887a773876dc5377ab6c5f7b59264051d6484ea7712ca5aafb3c`，仅三个标准头，无私有依赖。input总158223359/最大11723831B；answer总42589939/最大4191169B，距4MiB仅3135B，不能替代实际stdout证据。候选tier5独立Runtime/Files/profile身份，快照256MiB/input16MiB、累计payload512MiB；其他执行/外层/文件预算、单文件编译、3分钟阶段/10秒Run包装及5秒清理/原Stop期限保持。最坏429696733B/169 execution逻辑操作，Host操作口径独立；旧五来源及未准入3保持。

离线Docker Go1.26.10模块核验、六包race基线与定向race/vet、最终完整backend格式/vet/静态/root race/七Linux命令构建、sandbox vet及独立Darwin驱动构建均通过（`/private/tmp/oj-staticrmq-{baseline,targeted,backend}.log`）。新增合成集合实际冻结超过128MiB并读末点、256MiB再加catalog拒绝/共享额度释放、input16MiB和answer4MiB精确/+1、坏末点hash/UTF-8/token零Verify、旧五profile摘要、来源/档位隔离、512MiB账本注入边界/超限后删除及第六来源Stop矩阵；账本边界不等于实际搬运512MiB。Astra/high设计与静态实现审查未发现明确代码缺陷；真实准入仍因第9点失败阻断，不构成通过或人工审批。

原medium CID/镜像/owner/完整配置、停止PID0及私有空锁无占用先核验。一次真实双组共33.29秒：可信16MiB stdin2.28秒通过，沙箱逐块memcmp验证全字节并核对总长，CPU4997000ns/Memory524288B；tmp阶段均0、shm活跃16850944B/清理后0，outer cgroup memory.peak阶段最大112693248B，仅阶段观测、不证明连续峰值/总RSS。完整27点CLI31.02秒失败：首8点AC、第9点SE且无usage，后18点NOT_RUN。compile CPU215782000ns/Memory75915264B；已完成点最大CPU110782000ns/Memory41418752B，不能推断失败点资源。

原报告`/private/tmp/oj-staticrmq-reference-report-20261104/report.json` SHA `ee524a329fbfcc5d654e3ff761ce310dc0f1dee43b089dfa3bb5332a816d2d69`；plan `0d2beed5a725d367ebcd464591e83690e74e1e2371b6702a1a03dafd4196adf7`、profile `f6e3875942ca1b4b57a17124ef758cb9ed922e715a624336b6d19191e8e6d946`。0700/0600、无INCOMPLETE，status failed/publication blocked。execution诊断为第9点control/stdout/deadline、点内delete/input/deadline、最终delete/executable/deadline；两条unknown/deadline是finish附加context，不是另外两次I/O。第9点input4035B/answer8982B；输入上传+回读已完成，control角色stdout不证明输出生成/POST派发。两个cleanup各自独立Background+5秒，不能归因继承过期Run context。StopConfirmed为true、无stop诊断；原槽exited/PID0、私有空锁无占用，无恢复或手清marker。停止成功不追认此前DELETE完成。

随后只读采集原CID本轮启动至报告结束+2秒窗口（262144B/15秒/1000行封顶）：18030B、59条HTTP全200、未截断。脱敏投影`/private/tmp/oj-staticrmq-investigation-failed-staticrmq-timeline.json`只存时间/方法/封闭route/状态/latency/匿名对象序号，不存raw日志/缓存ID/材料；原流只保留SHA `850857971df2b4a0513d34f5e497f6d8d02b13666259a730f3efff1516e5c332`。第56条input回读19:30:28.640200969Z后，第57条POST/run直到19:30:53.093324883Z记录，相隔约24.453秒，但handler latency仅0.019172042秒；第58条另一对象DELETE先于第59条input DELETE。另一对象可条件推断为exe，未直接绑定角色。此次service/Docker时间差最大0.000439167秒，没有旧#114的23.787秒记录时间差形态，仍不能排除更上层共同原因。HTTP200不证明客户端完成；大间隔可能在回读返回、inspect、exec派发或宿主等待，不能定性VM暂停、日志背压或TLE。

后续旧五来源真实回归未启动，没有重跑/退款/提高期限；失败报告/制品保持。下一项独立工作应补Host固定inspect/command/response_validation阶段与最后完成位置、单调耗时/context/真实本地进程启动证据，以有界失败字段保留各独立错误且不含参数/ID/内容；先确定性本地验证，再另行预注册调查。当前没有已证实可直接修复的因果链。#114同样保持草稿，P4和开发目标未完成，未部署。

## 2026-11-04：point_add_range_sum 八来源固定环境接纳

候选HEAD `0cd63e3ddacb3be00e3f693f1d0797a2f02d4e42`已普通吸收[停止预算修复#123](https://github.com/DL-Association-for-Computing-Machinery/OnlineJudge/pull/123)的主线提交`5411cebc283fc1d3f358f36c72be4058ae2c812d`。该修复只在原10秒Stop总预算内为终止及确认预留时间，不宣称定位旧运行阻塞。point_add_range_sum仍为原MediumBulk枚举3和原Plan/Profile/预算；本轮只支持新版本在固定环境下准入，精确head CI及#114合并尚待核验，不记为已交付或部署。

离线backend整合发现旧`primality_test.go`仍将枚举3视为未知，修为仍未准入的5后，完整backend与必要静态/构建检查通过。缓存stdin驱动保留完整21点CLI后接原8MiB+1/16MiB两次输入的两组顺序，首组失败即停；Verify10秒、Compile wall+5、每次Run wall+5及独立Background5秒制品清理与Runner合同一致。上传SHA-256回读、程序count/sum和资源阶段样本保持；期限收紧不扩大预算，count/sum不单独证明顺序回显，资源样本不代表连续峰值。

一次协议`/private/tmp/oj-point-add-readmission-20261104-protocol.json` SHA-256 `3b2be2abd855cd51a7ca5679465d8c126d81cc16466165f2a42476031267bf26`冻结最终HEAD、全部Go源码/配置/两制品、完整八来源旧Plan/Profile、草稿版本/来源/有序点集及416个唯一材料对象925125315B。worker与driver SHA分别为`03fcaabfb0f557ae345e9899b67719a04d7ea78da33ecf4c2c2f32d87ca5818d`、`37821c6c671e1ac61693ca120c0c33238b8f5d8a5a59546b83aa5facb93cbd89`。显式核验Stop交付SHA为main与候选祖先，共享独占token、各真实canonical锁、三原槽完整配置/停止/PID0/私有空锁及无占用均通过前置核验。

固定Desktop LinuxKit7.0.14/arm64、Engine29.8.1、原go-judge v1.13.3/GCC15.3.0下，顺序point_add_range_sum、suffixarray、A+B、unionfind、zalgorithm、primality_test、static_range_sum、associative_array，各仅一次`count=1/failfast/timeout5m`，无重试、恢复或预算追加。全部174点AC/Stop，八个顶层及22个原子子测试的RUN/PASS清单精确匹配；每套完整Plan/Profile与历史报告逐字段相等，报告0700/0600、无INCOMPLETE、成功诊断省略、publication blocked。

| 来源 | 点数 | 整套耗时（秒） | report.json SHA-256 |
| --- | --- | --- | --- |
| point_add_range_sum | 21 | 13.25 | `790cbed00fc8eeacdc4ca6591b565dfcce676fb6443c2e92586b8b048bcf317b` |
| suffixarray | 50 | 20.10 | `c90f693abe1156d222e20d67de341e32fbe0c6ac9a0bdec8435c2d26a7a0bc8b` |
| A+B | 12 | 18.10 | `04554090e3b1280905d5bb0f419a6705ab27537ccd8ed081b9ca908569a48d5d` |
| unionfind | 18 | 10.09 | `dd038602004a852d8b1e2106ce062cd8f570b2083495ad18096dca04e743fdd6` |
| zalgorithm | 29 | 16.22 | `f18fbc43bcc912f4a6fd01598bb9b549f16b9f721ff26b6fc525670f4011a22c` |
| primality_test | 13 | 10.15 | `9863dac1b2414e8b565a4ab69d1df66e77b2ba844ae4e067e16cd5527c8932e6` |
| static_range_sum | 11 | 9.95 | `9da602d5e049d37250c75a3a23e05dd91ecc5fa1ec549afa47d18596c2833dce` |
| associative_array | 20 | 21.74 | `390ceb0b3f69ac6c3d7e9685c277aa1ea24e83e9c40b6601e611305106ebbd2c` |

各来源报告位于`/private/tmp/oj-point-add-readmission-20261104-<source>-report/report.json`；日志、observer结果/错误、独立postflight及验证记录同前缀。汇总`/private/tmp/oj-point-add-readmission-20261104-summary.json` SHA-256 `53659e0e515021b0fe0ac0827b28ec740eaf4cde16b03d902309c59742ff1d6b`；全部observer无错误/截断，最终源码/制品未变，三原槽停止/PID0/私有空锁且无占用。此后仅同步六页文档，不修改Go源码或重跑真实验收。

原suffixarray失败`19513bf823e8cd40bf5ba13fedcc5f2a3e9f6f073cc843246568ee2f2e14eb36`和既有调查保持，根因及长期稳定性未知，新接纳不追认历史失败。当前八来源通过固定环境维护入口接纳，其余两题仍待独立准入，#119仍待验收；不更新草稿not-run、生产null、publication blocked，不等于持久化验证任务/质量审核/题目发布/整体P4或M0通过。CI与合并状态后续按精确head核验；临时证据不构成独立备份。

## 2026-11-04：停止阶段预留终止与确认时间

原Stop枚举与kill共享整个10秒期限，枚举耗尽后可出现kill调用已尝试但本地进程未启动。独立修复仅调整内部时间分配：枚举与kill各最多2秒且不超过进入阶段时剩余总期限一半，串行返回后取消子context；drain/生命周期验证、总10秒、零重试、PID确认和失败隔离不变。枚举失败后kill成功仍不能通过停止验收。这是可复现的预算分配改进，不宣称定位旧#114/#119首次运行阻塞。

四包race回归通过：workerhost、execution、problemvalidation、cmd/worker；新增受信shell证明枚举子期限到期后kill仍实际启动，短父期限与独立取消、kill超时保持失败。原整体期限耗尽诊断继续覆盖本地收尾消耗所有剩余时间。完整backend格式/vet/静态/race/七Linux命令构建、sandbox vet和Darwin专项构建通过；Astra针对性实现/测试/协议复核无阻断，AI不替代人工审批。

原固定secondary槽、完整材料/源码/制品冻结并登记一次兼容协议后，A+B九组18.62秒全部通过（完整12点CLI、AC/WA/CE/RE/TLE/MLE/OLE/取消），每组停止确认；完整Plan/Profile保持。报告`/private/tmp/oj-stop-reserve-observation-20261104-report/report.json` SHA-256 `4f890571dc0e8a2f7ab706e4c918abba3a839d07e56d8496c188b77c38c33fae`，协议/完整日志/结果/后置/验证摘要同前缀；观察器无错误/截断，三原槽停止/PID0/私有空锁且无占用。未制造真实Docker故障或追加试验，未改服务配置、来源预算或原失败结论。最终CI/合并状态以远端精确head为准；两来源仍须吸收修复后各自完成新接纳，历史根因与长期稳定未知。

## 2026-11-04：point_add_range_sum 候选整合主线（未验收）

为恢复候选PR #114与现行主线的兼容，整合已交付七来源及停止/执行/Host诊断；保留枚举3的MediumBulk和原预算，不引入尚未合并的staticrmq档位5。缓存stdin驱动改为Verify10秒、Compile wall+5秒、每次Run wall+5秒及独立5秒制品清理，沿用原两组与8MiB+1/16MiB输入及断言。此处仅记录整合方向；本轮测试和真实专项待执行，不能据此解除下述suffixarray历史失败阻断或宣称已交付。

## 2026-11-04：point_add_range_sum 实现与停止故障阻断

成功fetch后从`origin/main b7f18f1d393abf7859a10321dbd628ff5910eb8b`建立`codex/point-add-reference`。新增固定point_add_range_sum完整21点与独立profile `p4-lc-point-add-range-sum-reference-desktop-arm64-v1`；仅该来源input16MiB、快照total256MiB，source64KiB/answer4MiB/64点保持。封闭`ReferenceMediumBulk`仅新增文件协议ID及payload512MiB，Runtime逐字段等于Medium；slot仍映射原medium容器/锁且保留原tier，Host与Cached两端分别计量。所有旧四profile golden保持；共享一份快照、实际锁互斥、132MiB匿名快照/16MiB输入读回与超限、坏末点零执行、来源借用拒绝、21点保守传输133操作及失败清理回归通过。512MiB边界使用预充值账本单测，不宣称实际传输512MiB或恰好256MiB快照成功。

固定来源锁不变；重新逐件哈希源码、catalog和全部input/answer。`data_structure/point_add_range_sum/sol/correct.cpp`为1382B、SHA-256 `3d57829f581030f3f97c7f282ffbb51a6a30247e4d28d540d663da139b9aacfe`，仅iostream/vector/cassert。input总105680358B/answer总29352240B，最大13501012B/3852936B；含source/catalog总135038029B，超过旧128MiB。按请求上限计费最坏299443885B/133逻辑操作，超过旧256MiB，在512MiB内。

固定Desktop LinuxKit7.0.14/arm64、go-judge v1.13.3/GCC15.3.0与原镜像下，新题首次两组15.40秒通过（`/private/tmp/oj-point-add-real.log`）：完整21点CLI12.86秒全AC并确认停止，最大执行CPU111100000ns/内存8388608B，编译CPU178704000ns/内存77594624B。报告`/private/tmp/oj-point-add-reference-report-20261104/report.json` SHA-256为`ece7b44eaec8ee29a18897eaf5a658eccd4b965f8cb6a7818a2af739bb753966`，plan `bb4f397db3d3e56d86d209c60ec0480f6d4e2314389728ef37e3a9539831e08b`，profile `5ab143ba3de72e53f361724db7c65ceca51d770dd81dd94be7495e2c286264c1`；0700/0600、无INCOMPLETE。

原8MiB输出RLIMIT下，缓存stdin8388609B/16777216B上传SHA-256回读成功，程序count/sum分别`8388609 1069547520`/`16777216 2139095040`，均AC；count/sum不单独证明字节顺序。资源分计：完整CLI wait4 Maxrss97697792B，不是Go heap或并发总RSS；16MiB输入回读时Go HeapAlloc46908648/HeapInuse48193536/Sys85727512B，shm已用16850944B、tmp0、容量各134217728B，outer memory.peak133808128B；清理后tmp/shm采样0。这些是阶段样本，不含全部宿主/沙箱并发内存，也不证明连续存储峰值。

随后旧suffixarray回归首次26.31秒失败（`/private/tmp/oj-point-add-suffixarray-real.log`）：首10点AC，第11点无usage的SE，其后NOT_RUN；Stop为`kill/deadline/observed=0/confirmed=0`且原medium容器仍运行，构成交付阻断。失败报告`/private/tmp/oj-point-add-suffixarray-regression-20261104/report.json` SHA-256 `19513bf823e8cd40bf5ba13fedcc5f2a3e9f6f073cc843246568ee2f2e14eb36`，旧plan/profile不变。至第11点完整成功路径载荷上界50352261B、含最终清理73操作，普通累计额度不足不能解释失败；不能据SE判断程序超时，或据kill/deadline推断kill已派发并运行10秒。现有报告无法区分上传/回读/运行/输出读取/清理，Stop又共用10秒并优先保留kill错误，根因尚未定位。

停止失败后立即停止后续实测，保留7字节marker；外部仅停止原full CID `f050f360e93733f021f5102fc52532ccedf2416301e0686d2ca8ad0377c16082`到stopped/PID0，OOMKilled=false、Dead=false、ExitCode0、无restart，不能由停止后状态还原失败时资源峰值。已知旧宿主/控制命令匹配0；固定owner/隔离配置及原ID回读后，使用既有`recover-reference library-checker:suffixarray <CID>`成功受控解封（`oj-point-add-recovery.log`）。故障窗口Docker events返回0，未提供派发时序证据；环境/二进制哈希记录在`oj-point-add-{suffixarray-failure-environment,failure-state}.json`。没有手清marker或重试失败题；恢复仅确认当时安全，不追认原报告。

恢复后只继续尚未运行的三个独立专项：zalgorithm三组19.54秒（29点CLI14.07秒、4MiB collector/8MiB文件边界）、unionfind两组12.52秒（18点CLI9.74秒、256KiB边界）、A+B九组22.10秒（12点CLI6.30秒、AC/WA/CE/RE/TLE/MLE/OLE/取消）均通过并确认停止。报告目录`/private/tmp/oj-point-add-{zalgorithm,unionfind,aplusb}-regression-20261104`，SHA-256依次`80f04bb0824c863cfeb137063b0c1b9f14027390b78692d2005f6d7a3dba895c`、`051bb1ddac106dad969d24d453b50498973116f28c880fd8ca50a36290ec4c15`、`2279bb186b32beb032cbf425019933ea42ee599654a05336179dcad76b8d9883`，旧摘要保持。这些独立通过不替代失败的suffixarray回归。

Docker离线Go1.26.10六相关包基线/定向race/vet、完整backend模块/格式/静态/root race/七命令构建与cmd sandbox标签lint通过（`oj-point-add-{baseline,targeted,backend}.log`）。无新依赖、SQL/schema、公共HTTP/UI变化；本地未重跑PG/Redis、浏览器或完整M0，远端精确head检查另核验。Astra/high实现复核无阻断，故障复核确认当前差异未改Cached执行、Host I/O或Stop，未发现明显旧配置回归，但不能证明运行等价；AI不代替人工审批。新增来源保持草稿、未交付，普通CI或新题通过不能抵消旧回归失败。下一步独立补齐脱敏执行阶段、Stop各步耗时及kill派发前deadline证据，保持原时限/停止门禁，再调查故障。原始私有材料、草稿not-run/生产null/publication blocked保持，未部署；临时日志/报告不构成独立备份。

## 2026-11-04：associative_array完整20点与旧六来源回归

候选实现提交`78fb42009f0d462d8354b0c18abb47f43268e76b`完成全部离线检查、Astra完整21文件/驱动/预算/外置协议复核及正常hooks；离线Gitleaks导出的候选HEAD历史119 commits通过。两份Darwin制品SHA分别为worker `88a089836877d4bf20d8e57c5112215f75fb9c84785cce173aeb3d89b067b1f2`、驱动 `946a02491b11b4c77b8f52097dd4dde49dede573696e95a4659ce9161c8a19fe`。最终工作区仅补五页文档，所有Go文件和制品仍匹配本次冻结状态；不重复无变化的应用检查，最终HEAD扫描/仓库门禁与远端CI另核验。

一次协议`/private/tmp/oj-associative-array-trial-20261104-protocol.json` SHA `76c03f7b19c983eab6e4879b920ae16dd495d90b9f8c070eee52a634f4cae36e`冻结21文件/43材料、制品/脚本及导入只读helper哈希；即时复核原Engine29.8.1/Kernel7.0.14-linuxkit/aarch64、原三槽完整配置/owner/停止/PID0/私有空锁、已批准localdev同生命周期健康。无新槽、持久服务变更、恢复、重试或预算追加；旧#114/#119及首次static_range_sum失败保持。

一次顶层21.31秒通过：首组2.37秒以真实单次48MiB完整stdin→8MiB完整stdout/4096B stderr，在同一共享15秒调用期限内逐字节验证，缓存/制品清理和Stop成功；然后独立生命周期的真实完整20点CLI18.90秒全部AC/Stop。raw stdout SHA `7d212b9c884f5c77896de960ae17cc341cda43b14d6a971f34ca29ebd4badf7f`；input存活shm-used50405376B、stdout存活58793984B、清理后0，outer memory-peak283099136B仅为此生命周期计数，不是每个文件系统瞬时峰值或生产总内存。真实题源编译CPU111471000ns/内存45821952B，逐点最大CPU649373000ns/内存65536000B；Darwin CLI wait4 maxrss238583808B不是Go heap或并发总RSS。

私有报告`/private/tmp/oj-associative-array-reference-report-20261104/report.json` SHA `77ed03e7126ef7e35653c6781653aee98600e471b3c0b4067ef5819f488ef3dd`，plan `5c5b515ff32168a8d76f94e594666e9148b650a48623e20bbc8b0f2d75155a17`、profile `abbc0c1081a07c239da97277a7ff8265a64205f847ba26ff606109d0750da519`。主代理独立核对源码、两catalog、版本/来源锁及20对input/answer身份和明确预算；Astra另核对摘要重算、完整日志/协议、全部点/usage和Stop。0700/0600、无INCOMPLETE，成功报告不含执行/停止失败诊断，完整流无截断或observer错误，postflight/result分别落盘；三原槽停止/PID0/私有空锁。

随后独立协议`/private/tmp/oj-associative-array-regression-20261104-protocol.json` SHA `0e34e26b06cc59211667bf136b42920df0a54be002b18725c786f7519daf6474`核对原六报告完整Plan/Profile和264唯一材料对象339670515B，以原六套完整suite串行各一次：A+B九组18.06秒、unionfind两组9.99秒、zalgorithm三组16.28秒、suffixarray一组20.85秒、primality两组9.52秒、static_range_sum两组9.94秒，完整12/18/29/50/13/11点及原故障/边界组全部通过。完整旧Plan/Profile逐项相等；原count1/failfast/5分钟、各内部预算和Stop保持，任一失败停止后组/来源，无恢复或重试。摘要/逐报告哈希归`/private/tmp/oj-associative-array-regression-20261104-summary.json`，最终三槽停止/PID0/私有空锁；不运行#114/#119候选，不解除历史失败或解释根因。

仅准入第七个固定环境维护来源，不更新数据库草稿not-run、生产null或publication blocked。其余三题、持久化验证任务/权限接纳、完整质量/非作者终审发布及整体P4未完成，未部署。以下为实现冻结、真实验证前的历史记录，不能替代本节最终实测及后续精确HEAD CI。

## 2026-11-04：associative_array完整20点候选（首次记录）

成功fetch后从`origin/main 7f009bd504a0541cff5b76ad12de4c376e9cc98c`建立独立`codex/associative-array-reference`，不依赖#114/#119。唯一目标为该固定来源的完整参考验证；43对象451132962B逐哈希核对，证据`/private/tmp/oj-associative-array-complete-material-evidence-20261104.json` SHA `4fc147dc30505ca7b5ef93bf4de6a46d6d11e4fe459c75201e04196a0e2ad196`。固定源码474B仅用标准头，input总412885310B/最大39588095B，answer总38242614B/最大4691775B；源码和20点不替换/采样。预算及独立整体期限归[参考合同](reference-validation.md#associative_array参考验证)。

新tier7/profile仅为此来源设置完整材料512MiB/input48MiB/raw stdout及checker8MiB/payload1GiB及RunCPU5s/wall10s/memory128MiB。CPU选择依据固定info.toml的5.0秒，不宣称本站生产参数或性能成立；阶段3分钟、单Run共享15秒、CLI4分钟/顶层5分钟为独立更严上限，可能先于单点预算耗尽。原六profile、编译、file/live/操作数、medium物理配置与canonical锁、Stop及零重试保持。

Astra设计及实现复核未见剩余代码阻断，指出单Run共享期限、3分钟整体限与48MiB+8MiB组合必须独立实测；主代理落实同次全字节探针/清理/Stop门禁，并补含catalog总预算精确512MiB/+1。现有离线Docker Go1.26.10模块核验、六包race基线/定向race-vet、额外problem/execution精确边界race、完整backend格式/vet/静态/root race/七Linux命令构建、sandbox vet及独立Darwin制品构建通过，日志`/private/tmp/oj-associative-array-{baseline,targeted,exact-and-build,backend}.log`；含catalog预算不等于匿名文件物理长度，1GiB账本注入不等于真实搬运。质量检测器中的四份故意缺陷夹具均被拒绝，其错误输出不代表应用检查失败。

当前未启动真实槽或题源，组合边界/完整20点和原六来源回归待执行，维护准入仍六题；不会自动启动#114/#119候选或解除其历史失败。无新依赖、SQL/schema、HTTP/UI、数据库验证或发布改动，草稿not-run/生产null/publication blocked保持，整体P4未完成，未部署。

## 2026-11-04：static_range_sum完整11点与原五来源回归

接续#121的干净head `cc23d6b97739d3bd1921c30d6de7d30e395d805d`，仅补足原候选的固定环境维护准入证据。用户在上一聊天明确批准只重启现有localdev，`00:01:52Z`一次重启返回0；同CID、镜像、端口和挂载指纹保持，PostgreSQL/Redis等兄弟容器生命周期未变，原公开CA校验HTTPS返回204，重启后PIDS单次样本22。已有离线镜像的受信`/bin/true`独立容器返回0。证据`/private/tmp/oj-localdev-approved-restart-20261104-{before,command,after,trusted-process}.json`。这些只证明环境条件改变与当时可用，不解释近五万PIDS或旧启动失败，不宣称永久修复；本轮没有再次重启或更改持久服务。

原失败26工件/22源码归档逐哈希保持。新预检核对原17个Go文件、两份Darwin制品、25对象149774419B、完整source lock、三原槽完整配置/owner/停止/PID0/私有空锁，Engine29.8.1及Kernel7.0.14-linuxkit/aarch64与原协议完全相等。新observer改为证据失败也等待原驱动并独立检查槽，先落postflight再独立写result；异常流只给前缀摘要，不冒充完整日志。Astra指出的收尾顺序及不完整流标注问题已修正，七类固定受信Python故障注入通过，包含2MiB日志写失败持续drain、启动失败、read异常/中断、槽检查失败和两份证据分别写失败；不运行题源或真实槽。脚本、材料与制品在执行前冻结，固定字段inspect不读取Config.Env。

一次新观测协议`/private/tmp/oj-static-range-sum-post-restart-protocol.json` SHA `9dc39d98b9c8e800567f518f5a3c69a8a5ce2464bec559c662a93202f535dd3f`；原协议/token及失败报告缺失保持，不沿用旧协议重试。新顶层9.88秒通过，先真实16MiB完整stdin、stdout8MiB/stderr4096逐字节校验，再核验cap+1返回完整8MiB/窄OLE/Exit25及stderr4097严格拒绝，已知缓存清理完成；边界组2.47秒、Stop确认后完整11点CLI7.37秒全部AC/Stop。输出原始SHA `7d212b9c884f5c77896de960ae17cc341cda43b14d6a971f34ca29ebd4badf7f`；探针输出存活shm-used25239552B、清理后0，外层memory-peak126369792B仅为该生命周期计数。真实题源编译CPU86798000ns/内存39399424B；逐点最大CPU90860000ns/内存12058624B，Darwin CLI wait4 maxrss111640576B不是并发总RSS或Go heap。

私有报告`/private/tmp/oj-static-range-sum-post-restart-report-20261104/report.json` SHA `f5bafb103a5b3731aef9490cd64faedc1d535c2fa81fe7b9ae5663435d2546f9`，plan `4b0480ec6f522182060acc6c064e9a90220b86548eded1dd859870f7989af846`、profile `501331a58bd9dfe76059e79701c80b7e7ba4d8096f1c93252f38c20d199cb028`，0700/0600、无INCOMPLETE、成功两类诊断省略。完整日志1971B且无截断或observer失败，postflight/result均落盘；三原槽停止/PID0/私有空锁无占用。新通过不追认首次启动或首次Stop成功。

随后另行登记五来源一次串行兼容回归，协议`/private/tmp/oj-static-range-sum-regression-20261104-protocol.json` SHA `287d950fc2c45b8b8bd83285649e9e1e5cd84caccf55c32eb523c0966a9d5bcd`，核对原五报告及239个唯一材料对象189896096B。顺序A+B九组17.54秒、unionfind两组9.77秒、zalgorithm三组15.77秒、suffixarray一组20.12秒、primality两组9.26秒，全部通过；完整12/18/29/50/13点均AC/Stop，完整Plan/Profile与原五报告逐项相等。原count=1/5分钟及各内部预算保持；`-test.failfast`确保任一subtest失败后不进入后组/来源，无自动恢复或重试。五份报告均0700/0600、无INCOMPLETE、成功诊断省略，摘要及逐报告哈希归`/private/tmp/oj-static-range-sum-regression-20261104-summary.json`。结束时三槽均停止/PID0/私有空锁，不涉及#114/#119候选执行。

Astra完整目标分支22文件及观测/回归协议复核未见剩余代码阻断，主代理另核验实际结果和完整diff；AI不代替真实审批。后续文档更新不改Go源码、profile或制品；本地仓库检查与最新head CI/PR状态单独核验。不把旧head CI当新head通过，也不重复无变化的应用检查。当前仅准入第六个固定环境维护来源；首次启动失败、未知根因及#114/#119的运行中旧失败限制保留，未证明长期稳定。数据库草稿not-run、生产null/publication blocked保持；其余四题、持久化验证任务/权限接纳、完整质量/终审发布及整体P4未完成，未部署。以下首次失败节为历史快照。

## 2026-11-04：static_range_sum候选与首次槽启动失败（未交付）

成功fetch后从`origin/main 8e5001063001d670e69e8ebb0a964b0adaab87bc`建立独立`codex/static-range-sum-reference`，不依赖仍为草稿的#114/#119。固定来源328B、仅两个标准头；source/catalog及完整11组材料25对象149774419B逐哈希一致，证据`/private/tmp/oj-static-range-sum-complete-material-evidence-20261104.json` SHA `3f96c296b7d9b04518897134bd8fd59b77d8239c57a91a11bcaf7a49ab3282bb`。input总91745127/最大11722894B，answer总58026619/最大7636751B；最大答案大小不替代实际stdout或资源证据。

唯一目标是该来源完整参考验证：独立tier6/Runtime/Files/profile，快照256MiB/input16MiB/answer及raw stdout/checker8MiB、payload512MiB，最坏275765609B/73 execution逻辑操作；旧五来源摘要、未准入3/5、执行CPU/wall/memory、文件/Stop期限及零重试保持。复用原medium完整配置、owner及canonical锁，不创建/替换槽。上游固定v1.13.3的Output→RLIMIT_FSIZE及非pipe独立文件collector已分别从官方源码页面/只读API核实；8MiB stdout+4KiB stderr不因合计超过8MiB自动超限，真实边界尚需验证，不据静态分析宣布容量成立。完整合同归[参考说明](reference-validation.md#static_range_sum参考验证)。

现有离线Docker Go1.26.10六包race基线/定向race-vet、完整backend模块/格式/vet/静态/root race/七Linux命令构建、sandbox vet及独立Darwin驱动构建通过。测试实际冻结超过128MiB、256MiB再加catalog拒绝、共享quota释放、input16MiB/answer8MiB精确/+1，验证11点最坏载荷/512MiB账本注入边界、25MiB+1字节峰值预留和逐点/最终清理。注入账本不等于真实搬运512MiB或实际内存峰值。旧五profile/来源及Runtime互斥、未知档位零派发、第六来源Stop/dirty矩阵保持。

新增负例初稿在裸observation要求额外空角色拒绝，但实际合同在更早Cached.Run wire ownership检查；Astra核对后将断言移至真实适配器入口，验证零stdout下载、已知input清理、后续fenced、已知exe可删及未知stdout预留保持，未修改生产分类。一次测试API签名编译错误修正后execution race/vet通过。驱动复核还要求预期stderr4097拒绝不得掩盖cleanup join；现仅接受一条未截断`validate/stdout/invalid_evidence`，注入stdout/input DELETE失败、取消和截断诊断均拒绝。对应普通race及sandbox标签合成回归通过，原失败日志保留，不计为通过。

主要日志`/private/tmp/oj-static-range-sum-{baseline,first-targeted,targeted,output-evidence,output-evidence-fixed,output-evidence-verified,sandbox-build-check,probe-cleanup-regression,backend}.log`区分失败与最终通过；Linux与Darwin制品分目录。policy689、完整diff/链接与离线Gitleaks通过，Astra针对性设计/实现/驱动门禁及最终文档复核无剩余阻断，AI不代替人工审批。候选实现提交为`852c43f46ca7c6f12ea53adc06a41f33d1b8187f`，验证前工作区干净，精确测试清单和22文件源码/两宿主制品哈希固定。

首轮协议于`2026-11-03T21:25:50.329091Z`登记，`/private/tmp/oj-static-range-sum-trial-protocol.json` SHA `1634b72c61f3118b6f6d6758f418acc278a02a090e672cff52583bae24708fce`；固定Kernel7.0.14-linuxkit/aarch64、Docker Engine29.8.1和原medium CID/镜像/owner/配置，三槽停止/PID0、私有空锁无占用、旧native/固定槽控制进程零。外置脚本绑定同一固定bundle/制品/源码/脚本/材料哈希，一次性标记与必要输出在Popen前创建；日志写失败继续drain，证据/收尾失败非零。未自动启动旧来源回归，未改期限或槽。

一次顶层测试0.109秒失败：首组在槽启动确认阶段返回command_exit，随后Stop为`inspect_before/evidence_unavailable`，未取得停止确认，dirty保留。未进入可信探针编译/运行或完整11点CLI，没有新报告目录，也没有执行报告中的Plan/Profile或判题usage。只读回读原CID仍exited/PID0、StartedAt/FinishedAt为原`20:43:19.412924422Z`/`20:43:35.72820843Z`，Docker状态ExitCode128，非paused/restarting；错误投影仅记录创建任务/启动shim/fork-exec/资源暂不可用固定片段及SHA `272f0fd5f6fc16511271b29c6bdd52afde421c668789dad33993a3216c61e425`，不保留原错误。旧生命周期一致不等同普通Stop所需的运行中进程与消失证据；本次Stop未进入枚举/kill，不能追认成功。

26工件和22源码逐哈希封存`/private/tmp/oj-static-range-sum-failed-start-archive-20261104/manifest.json`，SHA `29c67f384f15eabc6208c177abd883a3ec4cb790a29758aa3bbccc335f8a9388`，保留失败前后状态和原制品。Astra复核启动调用明确非零返回、控制进程退出、唯一控制者无并发操作、原CID两次同一旧生命周期停止/PID0满足本次外部排空前提后，使用既有`recover-reference library-checker:static_range_sum <原CID>`正常返回；CLI独占锁重新核验身份/配置/同一生命周期停止后清marker。三原槽再读均停止/PID0、私有空锁无占用。恢复只确认当前安全，不证明VM健康、不追认Stop或试验，不授权重试；没有再次启动或执行后续回归。

后续只读快照在`21:32:52Z`得到5个运行容器的PIDS计数49761/7/5/11/14；其中高计数对象后续归属元数据指向本项目持久`oj-local-dev`的localdev服务，不在本试验停启范围。PIDS包括进程及内核线程，字段含义按[Docker官方文档](https://docs.docker.com/reference/cli/docker/container/stats/)核对；Docker的8319504384B是总内存容量而非使用量。一次有界Docker top仅见一个可见oj-localdev进程，容器内固定status读取以退出128失败，未取得Threads值；这些不同口径和失败后的样本尚不证明线程泄漏、僵尸积累或shim失败根因。未读取环境/业务文件/参数，未停启该持久服务；源码与聚合投影归`/private/tmp/oj-static-range-sum-`工件，临时归档不是独立备份。

追加失败文档后的最终本地Gitleaks未取得成功，Docker run退出127；该检查结果单独保留，不记为扫描通过。候选实现提交前的完整扫描已通过，全部Go源码及两宿主制品仍与原试验哈希一致；最终head安全CI另核验，不能用旧扫描替代。

真实16MiB/8MiB边界和完整11点、旧五来源实际回归均未执行，候选保持草稿/未准入；精确head远端CI另核验。本地未重跑PG/Redis、前端/浏览器或完整M0。主线仍五来源，#114/#119旧失败/草稿、not-run/生产null/publication blocked保持，整体P4未完成，未部署。

## 2026-11-04：Host控制失败的阶段与本地启动证据

成功fetch后从`origin/main 31474f2a19280f7b9d53a64ca6987cb0ba5af6dc`建立独立`codex/host-control-evidence`。唯一目标为Host.RoundTrip失败证据：既有execution条目可选`host_trace`记录admission/prepare/inspect/command/response_validation固定前缀、失败/最后完成位置、单调耗时、进入时context与剩余期限，以及直接本地命令启动证据；原顺序、错误/取消、预算、清理、Stop及零重试语义保持。它不修改来源准入、Runtime/Profile/Plan、数据库或发布状态，也不修复/解释#114/#119旧故障。完整字段及缺失语义归[参考验证](reference-validation.md#执行失败诊断)。

共享纯值契约放`internal/experiment`但不进入Profile；Host局部固定5数组，不用共享LastError。每个失败返回点先冻结该阶段，整体在defer收尾后采样；原型把收尾时间计入末阶段的歧义经Astra指出后修正，25ms Body.Close定向回归核对整体与阶段之和差值。直接`commandFailure`才提供started/not_started，wrapped/nil/替身保持unknown；context/剩余期限非原子，0ms可能取整。前缀结构/数量/完成位置严格校验、数值0..60000及截断标记、枚举净化、未到达数组槽归零；无效或多操作歧义trace不借用。解耦Host/execution的typed getter按值返回，公开DTO/报告再分别复制。

GoJudge.request与cachedError重建固定脱敏错误前只复制元数据，不重新挂回已丢弃的transport cause；原Is/As（包括JSON SyntaxError与cached层原保留的context）保持。主control、点内input清理、最终exe清理分别经tag/FailureDetails/recordExecution携带自己的trace，finish附加context不复制。初稿provider终止分支另开类别遍历的缺口由Astra复核发现，现只读当前节点类别；计数型8个环provider零Unwrap、外层63/64链对应64/65槽，八条/第九条截断及poison Error方法断言通过。裸provider可unknown，槽预算不约束外来方法自身成本。Host成功而后JSON decode失败保持无Host trace，不引入成功响应侧通道。

现有离线Docker Go1.26.10模块核验、四包race基线及各阶段定向race/vet、最终完整backend格式/vet/静态/root race/七Linux命令构建、sandbox vet及独立Darwin驱动构建通过。第一次完整静态发现freeze返回值未检查，修正为保留原错误的赋值后全量通过；原失败日志保持。第五profile固定摘要及最后静态再验通过，成功JSON SHA `8bf9e56626c84d19e31a7050e36330866f798ab8c59b56866032ef7cfea4baa6`和旧五profile/Plan结构保持。主要日志`/private/tmp/oj-host-control-evidence-{baseline,host-targeted,host-reviewed,connected,projection,bounded-projection,backend,backend-fixed,final-quality-build}.log`；Linux与Darwin输出分目录。

Darwin原生测试先断言清单恰有11项（8项Host诊断及3项旧Stop），再一次执行，全部通过；包含真实受信本地进程未启动/启动后超时、过期inspect后仍保留原command尝试、wrapped/nil不伪造启动、inspect解码/隔离失败零command、阶段不吸收收尾、实际Host→execution接口、后续decode无trace。其余投影/并发副本/数字边界/多错误归属、旧cause及owner实际私有报告持久化/Stop成功失败marker决策由普通race测试验证。日志`oj-host-control-evidence-native.log`与精确清单`oj-host-control-evidence-native-list.txt`保留；这些仅执行固定受信命令和替身，不构成题源验收。

另一次必要的真实Host路径兼容回归使用原secondary CID/镜像/owner及停止/空锁前置：A+B九组23.43秒全部通过，完整12点AC/停止确认，AC/WA/CE/RE/TLE/MLE/OLE/取消保持。报告`/private/tmp/oj-host-control-aplusb-report-20261104/report.json` SHA `f4489bf3e618e2c701333d0833344fe20ef0d9aaf456204c35b44790847c2f08`；旧plan `12e87eb8d0771e34a8d187cb2bfc3b5cbf266a3d09768fc20194cfa4927f1325`、profile `658fef827daa872a726e6a728ad05c84a08103bd9857c71606fe76cff92c1437`保持，0700/0600、无INCOMPLETE、成功execution/stop诊断省略。结束时三个原槽均停止/PID0、私有空锁无占用，无恢复/手清marker。未重跑suffixarray、staticrmq或其余完整来源，亦未本地重跑PG/Redis、前端/浏览器或完整M0；精确head远端CI另核验。

没有新依赖、SQL/schema、HTTP/UI、实际期限或来源预算变更；#114与#119仍为原head草稿，旧失败/根因未知不解除，草稿not-run/生产null/publication blocked保持。下一次调查另行登记一次可检验假设和安全条件，不能以新增诊断或本轮A+B通过替代因果证据。其余五题、持久化验证任务/权限接纳、完整质量/终审发布与整体P4仍待完成，未部署。

## 2026-11-04：primality固定依赖与完整参考验证

成功fetch后从`origin/main b653bfcd9622204c9b5dd056d695aa0a99aa5473`建立`codex/primality-reference-dependencies`，独立于仍为草稿的#114。唯一目标为固定编译依赖闭包及完整13点；没有新依赖、SQL/schema、HTTP/UI、数据库验证状态、生产参数或发布行为变化。

同一可搬迁私有bundle的主源码、4头文件、2个catalog及13组input/answer共33对象逐字节复核，15381625B；证据`/private/tmp/oj-primality-complete-material-evidence-20261104.json` SHA-256 `32f397d6aa3b300d31ee0db92d282fdd671e53c118893a3e01ea486f78dd6cd9`。5份编译材料9285B，固定相对include链及`common/random.h`原始非ASCII字节保持，不做文本展开。匿名快照、plan路径/对象身份、64KiB聚合源/32MiB全材料、2MiB输入/512KiB答案和完整集合在执行前验证。新独立profile/tier4与6对象双端账本复用原medium槽锁；旧四profile摘要固定保持，未知3不准入。完整规则归[参考验证](reference-validation.md)。

Astra/high设计及完整实现复核未发现生产阻断；建议的真实驱动首组失败即终止/编译AC断言、五材料共享独立5秒context/父取消后仍尝试全部清理、旧suffix摘要及来源借用零Verify已补验。覆盖路径/大小写/前缀冲突零上传、64KiB精确边界、逐个上传未知回执隔离、最后头文件坏哈希、原始二进制/UTF-8及调用者切片分离、清理失败保留exe和八条诊断上限。AI复核不代替人工审批。

现有离线Docker Go1.26.10六包基线/定向race及vet、最终完整backend模块核验/格式/静态/root race/七命令Linux构建、sandbox标签vet、独立Darwin驱动构建通过。首次完整静态发现新测试Close错误未检查及路径表达式建议，修正后全量通过；复核测试主动取消父context后预期须保留新增取消证据，修正后两包race及最终静态通过。日志`/private/tmp/oj-primality-{baseline,first-targeted,targeted,verified-targeted,full-backend,full-backend-fixed,review-regression,review-regression-fixed,final-quality}.log`保留失败与成功，不把失败计为通过。Linux/Darwin产物分目录。

固定Desktop LinuxKit7.0.14/arm64、go-judge v1.13.3/GCC15.3.0和原medium CID下，一次双组11.84秒通过：可信5文件嵌套include2.77秒编译AC并运行输出核验；真实完整13点CLI9.07秒全部AC/停止确认/清锁。报告`/private/tmp/oj-primality-reference-report-20261104/report.json` SHA-256 `b35cb36b519a42e45b7197285ade3f9b265f9e600fd825f44c2ab3114de179b0`，plan `9ea3ea01f987edae4cf1b7433f9a282ed45d9d2abc0aed483c9c03d31d42e290`、profile `68f8cab1263e8c12897c6f0fc3a66ce915ba5296ee3304c03b2a6b5bd3419276`。编译CPU251511000ns/内存105353216B；逐点最大CPU370179000ns、沙箱内存786432B；Darwin CLI wait4 maxrss29261824B，非并发总RSS或Go heap。0700/0600、无INCOMPLETE、成功两类诊断省略，publication仍blocked。

原四来源一次串行回归全部通过（任一失败即终止，无重试）：A+B九组22.01秒，完整12点及AC/WA/CE/RE/TLE/MLE/OLE/取消；unionfind两组15.32秒，完整18点及输出边界；zalgorithm三组20.11秒，完整29点及4MiB collector/8MiB文件边界；suffixarray完整50点24.68秒。旧plan/profile保持，五份报告均全AC/Stop/0700/0600/无INCOMPLETE且成功诊断省略；报告哈希索引`/private/tmp/oj-primality-report-hashes-20261104.json`，四份旧来源报告位于`/private/tmp/oj-primality-{aplusb,unionfind,zalgorithm,suffixarray}-regression-20261104/report.json`，对应日志`oj-primality-*-real.log`。结束时三个原CID均exited/PID0、owner符合，canonical私有锁为空且无占用；无新槽、恢复或手清marker。本地没有重跑PG/Redis、前端/浏览器或完整M0；精确head远端CI另核验。

其余五题（含#114）、持久化验证任务/权限接纳、完整质量/终审发布及整体P4仍待完成。旧suffixarray失败与根因未知保持，不因本次新来源通过解除草稿；未部署。

## 2026-11-04：suffixarray 故障窗口与顺序对照调查

成功fetch后从`origin/main 61aaa4bb44889d1c0f5e8d20c7fda2048011f72f`建立独立调查分支。原medium完整CID/镜像/owner/配置、停止PID0、canonical私有空锁与无旧控制操作先核验；原固定镜像/Go1.26.10离线四包race基线与独立Darwin构建、精确测试清单通过。只新增[调查记录](reference-validation-investigation.md)并同步维护入口，没有应用代码、依赖、预算、来源或停止门禁变更。

新主线一次完整suffixarray50点25.27秒全AC并停止/清锁。随后预注册单次顺序对照：原#114 worker/driver与旧故障留存哈希成对匹配，原工作区未改；旧point_add完整21点及大stdin两组15.60秒通过，确认停止/清锁/排空后再运行新主线suffixarray，24.69秒全50点AC并停止。前置进程结束至目标启动约0.058707秒（含安全核验），未宣称与旧间隔相等。任一前置失败即终止、不恢复后续跑的协议保持；没有额外尝试、手清marker或解除#114。两目标报告与旧plan/profile逐字段一致，0700/0600、无INCOMPLETE、成功两诊断均省略；三个新报告SHA及全部证据见调查页。

原旧suffixarray故障窗口的有界脱敏服务日志提供新证据：72条HTTP记录；第11点stdout GET后的同对象DELETE，其handler latency为0.000252秒，但服务端ts与Docker记录时间相差23.787307秒。两次成功目标各307条记录，最大时间差约0.000446/0.000509秒。固定二进制依赖版本与官方ginzap/Zap源码核对后，明确handler计时不覆盖后续日志调用/客户端等待；不据HTTP200判AC，也不把时间差定性为日志背压、VM暂停或kill已派发。更早A+B窗口空日志不作原因证据。采集不保留原日志、缓存ID、参数/材料/原始错误。

Astra/high对方案、既有差异及证据判读复核：可把调查重点收窄到stdout清理附近/handler计时之外，不能确定旧主错误；完整新文档另复核。原`19513bf8…`失败报告及18项哈希保持，#114仍草稿。后续有明确触发时按预注册狭窄假设关联执行诊断/Stop trace和服务日志，不继续盲目重复。本地未重跑完整backend、PG/Redis、前端/浏览器或完整M0；本次文档差异按CI范围另核验，不把应用跳过称为通过。原草稿not-run/生产null/publication blocked保持，其余六题及持久化验证任务/权限接纳、完整质量/终审发布与整体P4仍待完成，未部署。

## 2026-11-04：执行阶段与独立清理失败诊断

成功fetch后从`origin/main baae5d5ec840f83db222e938c5142990b1f4fb93`建立`codex/reference-execution-evidence`。将此前九个自有WIP文件按哈希核对后适配当前Stop字段，不覆盖已合并实现。维护报告新增可选`execution_diagnostic`：可信缓存调用位置/角色账本提供phase/role/code，Runner添加operation与逐点序号。按观察顺序保留主错、各清理错误和独立父context，跨调用最多八条，遗漏标记truncated；已标记分支内同cause不重复展开。只记录封闭类别，不保留外来原错误文本/材料/ID；控制metadata与既有context unwrap语义分别验证，宿主文件context原因及Stop窄类别保持。

Astra/high预审发现WIP宽nil列表不计费以及已有tag掩盖独立context分支，均在本次实现修正。诊断遍历所有根/child槽（含nil）最多64项，宽/深/循环/空分支及64/65、8/9精确边界回归；另修正单值Unwrap返回nil的计槽。界限仅约束诊断投影，不约束外部方法或既有errors.Is。17种缓存阶段/角色故障、预算/准入/ErrBusy、控制transport/body/decode、宿主inspect/transfer错误接口、并发副本与保密、Runner跨调用/defer/点号/总量上限均有断言。owner实际调用链的取消+Stop失败同时保留两类诊断，经Finalize及私有报告持久化后仍为SE并保留quarantine。旧成功JSON SHA-256 `8bf9e56626c84d19e31a7050e36330866f798ab8c59b56866032ef7cfea4baa6`保持。

现有离线Docker Go1.26.10四包基线及定向race/vet、最终完整backend模块核验/格式/静态/root race/七命令构建、cmd sandbox标签vet通过（`/private/tmp/oj-execution-evidence-{baseline,first-targeted,targeted,final-targeted,final-backend}.log`）。首次baseline错误地使用镜像默认入口，未执行测试，不计验证；修正entrypoint后基线通过，原失败日志保留。Linux产物与Darwin宿主驱动分目录。本地未重跑PG/Redis、前端/浏览器或完整M0，精确head远端CI另核验。

固定Desktop LinuxKit7.0.14/arm64、go-judge v1.13.3/GCC15.3.0、原镜像/secondary槽下，一次A+B九组22.87秒通过，完整12点CLI6.98秒全AC并停止，AC/WA/CE/RE/TLE/MLE/OLE/取消保持（`oj-execution-evidence-real.log`）。报告`/private/tmp/oj-execution-evidence-aplusb-report-20261104/report.json` SHA-256 `f1729c67d4c5e29a167f04a050583f4b46cd9c6830f8fcc17f64f0a9ca415992`；旧plan `12e87eb8d0771e34a8d187cb2bfc3b5cbf266a3d09768fc20194cfa4927f1325`、profile `658fef827daa872a726e6a728ad05c84a08103bd9857c71606fe76cff92c1437`保持。0700/0600、无INCOMPLETE，成功两类诊断均省略。结束三个原固定槽均停止/PID0，canonical锁空且无占用，无恢复/手清marker。

Astra/high完整差异及证据复核无阻断，AI不代替人工审批。没有新依赖、SQL/schema、公共HTTP/UI、来源准入、重试或预算变化；原草稿not-run/生产null/publication blocked保持。本次仅补诊断，尚未重跑旧suffixarray故障；#114继续草稿，原`19513bf8…`失败与未定根因不被本次通过抵消。下一步在已验证诊断上做独立新目录调查，不追认旧报告或重试至绿；其余六题、持久化验证任务/权限接纳、完整质量/终审发布及整体P4仍未完成，未部署。临时产物不是独立备份。

## 2026-11-04：停止步骤与本地CLI启动证据

成功fetch后从`origin/main b7f18f1d393abf7859a10321dbd628ff5910eb8b`建立`codex/reference-failure-evidence`。本PR唯一目标为受限Stop失败证据：保留原stage/code/observed/confirmed/probe、原顺序、共享10秒预算、kill优先主错误、context原因、零重试及停止/dirty成功门禁，仅失败时附加整体和最多六个已到达步骤的毫秒记录。各耗时截断上限60000及elapsed_capped是证据边界，不是操作预算；整体含fence/锁等待/返回处理，不能用步骤之和或记录值证明时限严格兑现。PID探针只记合计次数，最多128；StopDetails及owner均复制列表。

kill记录调用前context与call_attempted；真实命令执行器仅用cmd.Process是否存在区分本地CLI not_started/started，nil error或无证据替身为unknown。active只是调用前采样，started不证明daemon接收kill或容器收到信号。确定性故障注入先耗尽caller期限，再沿原路径尝试kill：主诊断仍kill/deadline、observed0，但步骤可见enumerate/deadline及kill/context_before=deadline/call_attempted=true/local_process_start=not_started。另一固定受信shell实际发出启动信号后超时，记录started；这不复现或解释#114历史根因。

原Stop故障矩阵额外核对六步顺序/终止位置、PID合计次数、complete失败、枚举失败后kill成功仍unknown；验证耗时截断与owner完整复制ElapsedCapped/Count等字段。修改报告结构前固定成功JSON样本SHA-256 `8bf9e56626c84d19e31a7050e36330866f798ab8c59b56866032ef7cfea4baa6`，新增字段omitempty后保持同值。Docker离线Go1.26.10四包基线及三包定向race/vet、完整backend模块/格式/静态/root race/七命令构建、cmd sandbox标签lint通过（`/private/tmp/oj-failure-evidence-{baseline,original-json}.log`、`oj-stop-trace-{targeted,verified-build,backend}.log`）。

同一固定Desktop LinuxKit7.0.14/arm64、go-judge v1.13.3/GCC15.3.0与原secondary槽下，A+B九组22.72秒全部通过（`/private/tmp/oj-stop-trace-real.log`）：完整12点CLI7.01秒全AC并确认停止，AC/WA/CE/RE/TLE/MLE/OLE/取消保持。报告`/private/tmp/oj-stop-trace-aplusb-report-20261104/report.json` SHA-256 `3a8199be1af87e5e86a008fe445274caf7938eabdd3ab6327a7d4ca3f99bfe10`，旧plan `12e87eb8d0771e34a8d187cb2bfc3b5cbf266a3d09768fc20194cfa4927f1325`、profile `658fef827daa872a726e6a728ad05c84a08103bd9857c71606fe76cff92c1437`保持；0700/0600、无INCOMPLETE、成功未添加stop_diagnostic。最终三个固定槽均停止/PID0、canonical锁为空且无占用，没有恢复或手清marker。

初次将原生命令单测指向cmd/worker驱动，返回no tests to run，没有计入通过（`oj-stop-trace-native-command.log`）；改为独立构建workerhost测试包，先断言精确三项测试清单，再在Darwin执行，三项共约1.1秒通过（`oj-stop-trace-native-{package-build,command-verified}.log`）。只执行固定受信shell和测试替身，不启动沙箱槽或执行题源；README同步独立构建和列表核验命令。

Astra/high针对性设计/完整差异复核无阻断，补充其建议的矩阵与映射断言后重验通过；AI不代替人工审批。执行失败阶段与多清理错误诊断可独立验收，已按一个PR一个目标拆开，九个自有WIP文件及五个基线文件按哈希保留在`/private/tmp/oj-execution-evidence-wip-20261104`，尚未验证/交付，不混入本次差异。本次未故意制造真实Docker停止失败，也未重跑suffixarray；#114仍为草稿，原`19513bf8…`失败与交付阻断、未定根因保留，后续补执行阶段后再做独立诊断调查。无新依赖、SQL/schema、公共HTTP/UI或运行/传输预算变化；本地未重跑PG/Redis、前端/浏览器或完整M0，精确head CI另核验，原始草稿not-run/生产null/publication blocked保持，未部署。临时证据不构成独立备份。

## 2026-11-04：suffixarray 全50点参考验证

成功fetch后从`origin/main 934923ccefe0b42d7d50a018dbb98327bb7b1853`建立`codex/suffixarray-reference`。维护CLI新增固定来源`library-checker:suffixarray`，完整50点不采样。独立profile `p4-lc-suffixarray-reference-desktop-arm64-v1`仅将该来源完整快照总量准入128MiB；source64KiB/input1MiB/answer4MiB/64点、Medium Runtime/文件协议保持。zalgorithm仍64MiB，旧三profile/plan摘要均回归不变。所有磁盘来源共用匿名单文件与进程内一份额度；suffixarray与zalgorithm共用原medium槽、canonical锁与停止/dirty门禁，没有新增/替换容器或修改配置。

按固定LC锁复核`string/suffixarray/sol/correct.cpp`为3631B、SHA-256 `eadecedffa32796491d66c7450758ebee2ac52596784492b783989fb0154aeea`，仅标准库iostream/vector。完整50点input总13584593B、answer总91580275B，单点最大500001B/3388890B；总材料约105MB加source/catalog，超过旧64MiB。按请求上限计费最坏payload236891698B、307次逻辑操作，在256MiB/512次内，不退款或只计实际输出。回归实际写入并读取约68MiB匿名快照、旧来源拒绝相同集合、超过128MiB集合拒绝及跨来源共享额度；未声称恰好128MiB内容成功。另以50点各512KiB输入/4MiB输出验证更保守累计收费、307操作及每点/最终清理，坏末点UTF-8零执行与来源借用拒绝保持。

固定Desktop LinuxKit7.0.14/arm64、go-judge v1.13.3/GCC15.3.0和原镜像下，首轮完整50点24.67秒全部AC并确认停止（`/private/tmp/oj-suffixarray-real.log`）。最大执行CPU75731000ns/内存18567168B，编译CPU339128000ns/内存83959808B；CLI wait4 Maxrss43089920B只是Darwin OS计数，不当作Go heap或CLI/Docker/沙箱并发RSS总和。报告`/private/tmp/oj-suffixarray-reference-report-20261104/report.json` SHA-256为`8c33f50079f8eeb8c32befb5b31141b7e67212ef13c871480d1ec672c0b24cdf`，plan `0ac8ba8b1fd52f7027ff401fdb495eb9f81fea9d2bbd3f1c0c752de24054b5c6`，profile `0082bb2f961c1f1c082c0ed671881bca49cdaae72d9587bb76d8424ff5e661e7`；0700/0600、字段白名单及无INCOMPLETE已回读核验。

同构建复验zalgorithm三组19.42秒（全29点13.95秒、4MiB collector及8MiB文件边界）、unionfind两组12.05秒（全18点9.35秒、256KiB边界）、A+B九组21.78秒（全12点6.26秒、AC/WA/CE/RE/TLE/MLE/OLE/取消）全部通过并确认停止。报告目录分别为`/private/tmp/oj-suffixarray-{zalgorithm,unionfind,aplusb}-regression-20261104`，报告SHA-256依次`61716791784387435e14bf673caca4604643cf8c580e8d10926d280f59e34de9`、`571d0a521f13f99b5c2282156c15da2b8279fa79129e2c8d04c9ff76bcd4d993`、`eaa7bddb24cfc6073f6d70d91ae295e1cb2d7b18abebc089c72399257320600b`；日志为`oj-suffixarray-{zalgorithm,unionfind,aplusb}-real.log`。结束时三个原槽均停止/PID0，没有重试至绿、恢复或手清marker。旧历史停止故障本轮未复现，根因仍未知，不追认旧报告。

Docker离线Go1.26.10四相关包基线/定向race/vet、模块核验、完整backend格式/静态/root race/七命令构建及cmd sandbox标签lint均通过（`oj-suffixarray-{baseline,targeted,backend}.log`）。Astra/high针对性设计及完整差异复核无阻断；AI不替代人工审批。无新依赖、SQL/schema、公共HTTP或UI变化；本地未重跑PG/Redis、浏览器或完整M0，精确head远端CI另核验。原始私有材料、草稿not-run/生产null/publication blocked保持；其余六题、持久化验证任务/权限接纳、完整质量及非作者终审发布仍未完成，未部署。临时报告/日志/产物不构成独立备份。

## 2026-11-04：zalgorithm 全29点与封闭参考档位

成功fetch后从`origin/main f7b92c9477a2783603f6561fb550997a9e7f0604`建立`codex/zalgorithm-reference`。显式来源入口新增zalgorithm完整29点，不采样。固定来源表选择受信布局/材料预算，Runner在准备前及冻结后核对sourceID；所有来源仍先冻结/哈希完整集合、预检全部答案，再调用execution Verify/Compile。单匿名磁盘文件、进程级一份额度和按点加载共用；原A+B/unionfind API作为兼容入口保留，不借用新profile。按固定LC锁重核源码`string/zalgorithm/sol/correct.cpp`为969B、SHA-256 `9f7168d28138fb61d7706b5b899bb0ae21503a848428fc7d7551c5a8507ff8d7`，仅标准库iostream/vector，无私有依赖。

新增封闭Small/Medium档位解析及不可变配置副本，新factory和opaque tier slot拒0及未知值，无法回落M0；Host私有0仅表示原M0，构造/启动/恢复和每次控制/文件派发均先解析。Medium Runtime `p4-reference-medium-gnu17-desktop-arm64-v1`保持CPU1秒/wall5秒/memory128MiB、编译/诊断4KiB，raw stdout4MiB；zalgorithm完整快照input1MiB/answer4MiB/total64MiB。文件协议`p4-gojudge-cached-files-medium-v1`只升双端累计payload256MiB，其余48MiB单file/64MiB live/4对象/512操作保持；29点按请求上限计量已140258181B另加源码双传，不能沿用旧128MiB。新增29点累计跨旧上限、两端新cap/漂移、坏末点零执行、来源借用和未知档位零派发回归。

显式新建停止槽`oj-reference-medium-sandbox`，CID`f050f360e93733f021f5102fc52532ccedf2416301e0686d2ca8ad0377c16082`，固定owner、无网络/端口/挂载、private cgroup；旧槽不替换。使用同一固定镜像覆盖Entrypoint为`[/opt/go-judge]`并严格匹配固定Cmd，只升global output/RLIMIT8MiB、global copy-out仍1MiB，内层64MiB/外层1GiB及128MiB tmp/shm保持。新增`sandbox_cmd,omitempty`不改旧JSON，旧nil/空Cmd均接受，Medium非空参数必须精确相等；旧两profile golden摘要及真实回读一致。CLI不配置或创建槽；`recover-reference [source-id] CID`省略source仍只选原unionfind，显式zalgorithm才选Medium，拒A+B/未知source，独立停止/dirty门禁保持。

固定Desktop LinuxKit7.0.14/arm64、go-judge v1.13.3/GCC15.3.0与原镜像下，首轮三组20.22秒全部通过（`/private/tmp/oj-zalgorithm-real.log`）：完整29点CLI14.64秒全AC并停止确认，input总9311668B/answer总30611793B、最大499693B/3348946B；最大执行CPU22126000ns/内存6815744B。1MiB+1与4MiB全字节stdout保真AC，4MiB SHA-256为`2b07811057df887086f06a67edc6ebf911de8b6741156e7a2eb1416a4b8b1b2e`；4194305B为collector OLE，精确标签/状态/Exit0/用量presence比较全true。隔离内artifact写1048577B/8388608B成功并核对实际大小，8388609B为OLE/Exit25且无collector截断，区分全局RLIMIT与stdout预算。

资源证据分别计量：CLI wait4 Maxrss为42106880B，Darwin字节换算按[Go官方实现](https://go.dev/src/runtime/pprof/pprof_rusage.go)核实，不当作Go heap或CLI/Docker/沙箱并发RSS总和；4MiB缓存活跃时shm采样4272128B，host Go HeapAlloc7065384/HeapInuse8437760/Sys22792456B，均为阶段样本。collector与global probe的outer memory.peak分别40955904/43663360B，tmp/shm容量各134217728B，清理后采样均0；不含宿主堆、不构成连续存储峰值或任意材料组合容量证明。报告`/private/tmp/oj-zalgorithm-reference-report-20261104/report.json` SHA-256为`127ff2701e08ea21ad147412975b709030586a3ddad2e345a78b6f3ae935ef03`，plan `2fb3a00287701df144b0810ffc5dbbf4ae43e84402e20bc087829c475bdab40f`，profile `32aa64eb913108ccc089a0548a98cb777f019f37cd4bc349ab87ff0ddd1a515d`；0700/0600且字段只含身份/状态/用量。

同构建的旧unionfind两组11.99秒（18点CLI9.33秒）与A+B九组21.90秒（12点CLI6.30秒、AC/WA/CE/RE/TLE/MLE/OLE/取消）全部通过并确认停止（`oj-zalgorithm-unionfind-real.log`、`oj-zalgorithm-aplusb-real.log`）。报告SHA-256分别`88e2f295240becb4fe78d5d3233a195a4314c221a234eb50d982a4c4b20f1bac`与`12584c42ed4ecf91cae53148f0c520b2523d98f8ad90416a9d8f2964309a7aa5`，目录分别`oj-zalgorithm-unionfind-regression-20261104`/`oj-zalgorithm-aplusb-regression-20261104`，旧plan/profile摘要不变。没有重试至绿、恢复或手清标记；旧历史停止故障本轮未复现，根因仍未知，不追认旧报告。

Docker离线Go1.26.10基线/定向六包race/vet、模块/格式/三范围静态/完整root race/七命令构建和cmd sandbox标签检查通过（`oj-zalgorithm-{baseline,targeted,static-verified,backend}.log`）。首轮静态拒绝已无正常调用的私有lockReference包装，删除后原槽仍用同canonical锁名，未屏蔽unused规则。Astra/high分阶段设计/差异/真实驱动与证据复核无阻断，AI不代替人工审批。无新依赖、SQL/schema、公共HTTP或UI变化；本地未重跑PG/Redis、浏览器或完整M0，精确head远端CI另核验。原始私有材料、草稿版本/not-run/生产null/publication blocked保持，仍有七题及持久化验证任务/权限接纳、完整质量/非作者终审发布待完成，未部署。临时日志/报告/产物不是独立备份。

## 2026-11-03：unionfind 完整参考验证与磁盘快照

成功fetch后从`origin/main 0f77fec0f5f7c0b12bd32377b499a90c7450dda1`建立`codex/unionfind-reference`。维护CLI显式`--source-id library-checker:unionfind`接完整18点；默认A+B12点保持原profile。新增单个0600匿名磁盘快照，同fd完整哈希所有source/input/answer，完整catalog、逐点顺序/身份及总预算先核验；进程最多一份，Prepared占用至Close。全部答案UTF-8/token预检先于execution Verify/Compile，执行时只加载当前点，原材料随后修改不影响冻结值；CLI槽启动/Host探测仍先发生。失败关闭与取消、Read/Close竞争、完整集合超限、末点无效零执行、元数据不改内部span均有回归。磁盘准备失败可无完整plan/点列表；匿名unlink失败在写内容前拒绝，极端情况下只可能留空0600文件。

独立`p4-lc-unionfind-reference-desktop-arm64-v1`绑定64MiB磁盘总量、4MiB单input/256KiB单answer、最多64点、CPU1秒/wall5秒/memory128MiB、raw stdout256KiB；编译双流/运行stderr各4KiB并显式进入profile摘要。使用新固定`oj-reference-sandbox`和`slot-reference.lock`，新旧构造器及入口相互拒绝profile，M0 parser不接新槽。原固定镜像/内层64MiB临时盘、外层1GiB及128MiB tmp/shm、global输出/复制1MiB和文件48/64/128MiB/4对象/512操作保持；CLI不创建或替换槽。本轮显式创建独立停止槽CID`5900da241d91266df7302b50f0811c5cec32a3a6ab497834de9343833bea4d21`，固定owner、无网络/端口/挂载、private cgroup，既有M0槽保留。`recover-reference`只在旧宿主操作已排空及原ID/配置/停止确认下解封该锁，不启动或追认旧报告。

真实边界首先复现正常退出的cap+1输出被拒SE（`/private/tmp/oj-unionfind-real.log`及`oj-unionfind-output-diagnostic.log`）。固定v1.13.3官方[collector](https://github.com/criyle/go-judge/blob/v1.13.3/envexec/file_collect.go)、[结果转换](https://github.com/criyle/go-judge/blob/v1.13.3/envexec/run_single.go)与固定go-sandbox v0.14.2标签表证明：该OLE同时附带同名error。仅reference Run在精确status/error、Exit0、合法用量、唯一stdout/CollectSizeExceeded、raw恰cap+1、两个已验证流角色且无残余ID同时成立时归一化冗余标签；其他error/编译/M0仍SE。17类伪证据负例和完整缓存链通过，M0用其自身cap+1单独验证不吞该标签；未削弱断言或放大预算。

最终固定Desktop LinuxKit7.0.14/arm64、go-judge v1.13.3/GCC15.3.0、原镜像下，unionfind两组12.48秒通过（`oj-unionfind-release-real.log`）：完整18点CLI9.83秒全AC，input总34989222B/answer总1913930B，单点最大3126112B/200700B，实际最大CPU26175000ns/内存2097152B。64KiB/256KiB stdout原字节成功，262145B返回OLE；真实响应的固定标签/状态/Exit0/用量presence比较均true。probe清理后tmp/shm采样0、容量各134217728B，外层memory.peak40783872B，不含宿主堆、不是连续存储峰值。报告`/private/tmp/oj-unionfind-reference-release-20261103/report.json` SHA-256为`9da2d74fb0c039bb64182649682286bac530145713ee6923a2c45edb7a3829f6`，plan `9a5377ec9c56f8950e3b003337bceb2753d94d76ac5133814232e30804250e9b`，profile `a3f3b1fcea5d65e79003497ee98d713e4323131fb5446e08ea6482d9486c74c2`。

同最终产品构建的原A+B九组21.89秒通过（`oj-unionfind-aplusb-real.log`）：完整12点CLI6.27秒、AC/WA/CE/RE/TLE/MLE/OLE及取消，全部停止确认。报告`/private/tmp/oj-unionfind-aplusb-regression-20261103/report.json` SHA-256为`2ea259ba5c2d731629a1ff9b3c5e951dd1426cb1c93df5e2014b111568887311`，旧plan/profile摘要保持。两报告0700/0600，仅身份/用量/状态；新旧槽成功收尾，历史失败日志保留，没有手清marker或放宽Stop。旧两次停止故障本轮未复现，根因仍未知，不能追认旧报告。

Docker离线Go1.26.10模块验证、格式/vet、三范围静态/root race/七命令构建及cmd sandbox标签检查通过（`oj-unionfind-backend.log`），新增完整快照超限测试另作定向补验。Astra/high针对设计/完整差异/collector适配复核无阻断，AI不代替人工审批。初次基线挂载旧只读模块缓存缺当前版本而失败，改用镜像既有完整缓存后离线通过，未联网安装。无新依赖、SQL/schema、公共HTTP/UI或生产配置变化；本地未重跑PG/Redis、浏览器或完整M0，远端精确head CI另核验。文档与知识按交付同步；其余八题大输出/依赖、持久化验证任务/权限接纳、完整质量/非作者终审发布仍待完成，not-run、生产null、publication blocked保持，未部署。临时日志、报告及产物不是独立备份。

## 2026-11-03：P4 缓存文件的保真执行通路

成功fetch后从`origin/main 61ba081249a26711f354a27ec373c87c973aae8f`建立`codex/reference-byte-execution`。首份完整参考验证实际改用独立`CachedGoJudge`：源码/输入上传后回读SHA-256、缓存stdin、stdout原始字节拉取，答案留在宿主。M0默认构造器、内联白名单/预算、原样提取的结果校验及Runtime保持；新模式先严格验证角色/互斥表示/原始长度，再复用结构化证据校验。材料移除ASCII约束，参考答案仍全部在编译前按UTF-8/token域先验，坏编码不会触发沙箱。无新依赖、SQL/schema、公共HTTP或UI变化，草稿版本/not-run/production null不变。

新profile `p4-lc-reference-files-small-desktop-arm64-v1`绑定`p4-gojudge-cached-files-v1`和48MiB单文件、64MiB存活缓存、128MiB任务载荷、4活动对象/512操作；创建前预留exe/stdout未知大小，失败占额不退款，零字节操作/ID历史有界并预留DELETE额度。只接受本操作角色ID，未知回执不重传或猜测删除；每点独立清理input/stdout，最终清exe，失败封闭适配器并由Stop收束。Host使用有限reader/有界sink、标准库随机MIME、curl上传流及固定四字节状态尾，active覆盖完整IO/子进程。新增真实子进程stdin背压与注入宿主Stop排空回归。清理期间取消测试先复现返回nil（`oj-reference-bytes-cleanup-cancel-repro.log`），修复为独立清理后最终检查父context，回归通过；原Stop10秒及隔离规则不放宽。

Docker Go1.26.10/linux arm64相关基线、最终模块验证/格式/vet/三范围静态/root race/七命令构建和cmd sandbox标签检查全部通过（`/private/tmp/oj-reference-bytes-backend.log`、`oj-reference-bytes-targeted-verified.log`）。真实驱动追加MLE后仅重验受影响标签静态和驱动构建（`oj-reference-bytes-mle-driver-build.log`），产品代码未变，不重复无关套件。Astra针对核心差异及约束复核无阻断，AI复核不代替人工approval；治理/秘密/精确head远端门禁仍按交付流程核验。本地未重跑PG/Redis/浏览器或完整M0，不把新模式实测扩写为这些套件通过。

固定Desktop LinuxKit7.0.14/arm64、go-judge v1.13.3/GCC15.3.0、原镜像与私有cgroup中，字节专项5.53秒两子例通过：50331648字节上传/下载SHA-256为`dd3e5d8ff490678599d36f2c49f7ab607621c0bd4a9764f49896055277137ff7`，精确上限成功、上限+1拒绝；空/256/50331648字节stdin由受信memcmp程序逐块验全字节模式，输出含0—255及自身尾LF＋200仍保真。大stdin CPU4591000ns、进程内存524288字节，小输出269字节。`/tmp`和`/dev/shm`容量各134217728，阶段采样中shm最大观察值50409472、清理后0，不作为连续峰值；外层沙箱cgroup memory.peak为260976640，小于固定1073741824。tmp各采样时为0，不证明其历史峰值为0；外层计量不含宿主Worker堆，也不是全部存储域的硬峰值证明。记录在`oj-reference-bytes-real.log`。

新通路的真实12点CLI及原八组专项20.05秒全部通过（CLI6.75秒），另定向新增真实MLE2.06秒通过，返回MLE/后续NOT_RUN并停止确认（`oj-reference-bytes-reference-real.log`、`oj-reference-bytes-mle-real.log`）；没有把分开运行写作单轮九组。全部停止确认，最后同CID回读停止/PID0、marker为空，无恢复或重试至绿。报告`/private/tmp/oj-reference-bytes-20261103/report.json`的SHA-256为`741c1b9a8af3f6a0c3ac64e1f473cfa3e49b233b7c7711a54a0cdeefbf0db380`，profile为`658fef827daa872a726e6a728ad05c84a08103bd9857c71606fe76cff92c1437`，plan仍为`12e87eb8d0771e34a8d187cb2bfc3b5cbf266a3d09768fc20194cfa4927f1325`，0700/0600且字段白名单只含身份/用量/状态。

这证明固定环境的文件协议和本轮边界，不是其余九题或完整P4准入：完整材料仍限输入16KiB/总2MiB、Run stdout仍4KiB，较大输出/临时盘、私有头文件、大材料快照、持久化任务/权限接纳、完整质量/终审发布仍待完成。此前两次停止故障本轮未复现，根因继续未知，不追认旧报告，未部署。临时日志与制品不是独立备份。

## 2026-11-03：停止失败的脱敏诊断与真实输出界限

成功fetch后从`origin/main c9cf31c8c4f8f713bd79b8219237e79bcb6f3000`建立`codex/sandbox-stop-diagnostics`，承接上一阶段两次原因未知的停止确认失败。宿主命令错误只保留固定类别及context判等，原始命令/路径/stderr/Reader错误和退出码不外传；Stop各失败出口增加阶段及0—128进程计数，私有参考报告以可选stop_diagnostic投影，核心不依赖Docker适配器。枚举失败仍kill、多失败kill优先、首探针失败即停、最终context检查、10秒总预算及隔离/确认规则保持；清marker失败不能伪装成Stop失败或通过。

新增真实子进程输出回归首先失败：原limitedBuffer嵌入bytes.Buffer，提升的ReadFrom使io.Copy绕过限长Write，131074字节未拒绝。现改显式组合，实际131073字节成功/131074字节output_limit；原预算不放大。探针只把明确不存在当成功，proc不可用/目标路径存在使用不同非零码，异常表达式仍非零且不冒充PID存在；PID复用不能据此认定原进程存活。Astra复核发现初稿if-test把异常退出2误当不存在，已修正并补真实shell注入回归；二次复核无阻断。AI复核不代替人工approval。

已有离线Docker Go1.26.10/linux arm64基线通过。最终模块校验/格式/vet/三范围静态/root race/七命令构建、sandbox标签静态全部通过（`/private/tmp/oj-stop-diagnostics-backend-final.log`）；定向回归覆盖退出/启动/输入复制/输出/超时/取消分类、全部停止阶段、双失败仍kill、探针计数/首失败停止、末次context过期、procfs/存在/异常shell状态及报告脱敏。原输出缺陷首败保存在`oj-stop-diagnostics-output-repro.log`，没有削弱测试或延长预算。无新依赖、SQL/schema/API/UI变化；本地未重跑PG/Redis、浏览器或完整M0套件，通用远端回归仍按精确head CI核验。

所有重构建结束后仅运行一次既有真实专项：固定Desktop LinuxKit7.0.14/arm64、go-judge v1.13.3、GCC15.3.0与原镜像，八组98.71秒全部通过，含真实12点CLI22.35秒、合成AC/WA/CE/RE/TLE/OLE和取消；全部停止确认，无恢复/再跑至绿。私有报告`/private/tmp/oj-stop-diagnostics-real-20261103/report.json`的SHA-256为`7c73896caa55a96ea706a1f56f779bb76f26457aaac04d9b623b26bb5eca5125`，plan/profile与上一阶段相同，0700/0600；成功不含stop_diagnostic。日志`oj-stop-diagnostics-real.log`。本轮未复现旧停止问题，历史失败没有新字段，不能反推旧根因或宣称入口稳定；本次没有解决其余九题/持久化验证任务/完整质量/审核发布，也不改not-run或生产null配置，未部署。临时日志和制品不是备份。

## 2026-11-03：首份完整私有草稿的受限参考验证

从成功fetch的`origin/main 5f71597b5a7a74dd5ea517391402ee4591eed57c`建立`codex/private-draft-reference-validation`。新增维护命令`worker validate-reference`：只接收固定锁A+B完整12点，冻结独立plan/profile，编译一次、逐点运行及比较，记录资源和完整容器生命周期，制品清理、停止及marker同步均确认才通过。报告始终publication blocked，不连接控制端/PG/Redis，不写草稿not-run或生产配置；其余九题的大材料/依赖、持久化任务、完整质量、审核发布继续待完成。原M0分类、profile和Stop门禁不放宽；仅提升已锁定MIT许可go-toml/v2 v2.2.4为直接依赖，无版本/go.sum变化或新下载。

实际复现并修复两项准入缺陷：删点后重算合法草稿/index仍曾被接受，现独立核验锁定info.toml组序、完整点集、sample和hash.json精确配对，遗漏/顺序/重复键等在Compile前拒绝；macOS大小写别名曾绕过输出目录隔离，现按真实目录身份检查祖先，原生文件系统回归通过。固定快照与全部参考答案先验、错误/取消后NOT_RUN、启动owner/生命周期、Stop/清理失败不通过均有行为回归。针对性Astra设计及实现复核不代替人工approval。

Docker离线Go1.26.10/linux arm64的模块验证、格式、vet、三范围静态、完整root race及七命令构建通过（`/private/tmp/oj-reference-catalog-backend.log`）；目录修复后cmd/worker race及sandbox标签静态检查通过（`oj-reference-path-verified.log`），原生大小写回归通过（`oj-reference-case-verified.log`）。首轮冷并发构建时已有workerclient的300ms成功边界测试超时，未改断言/阈值，原样完整重跑通过。未本地重跑PG/Redis/浏览器或完整M0套件，无对应业务/schema/frontend改动；普通远端CI另按精确head核验。

固定Docker Desktop LinuxKit7.0.14/arm64、go-judge v1.13.3、GCC15.3.0镜像`sha256:cd40ffdb32619806a8d97e59fe03e890fe4066835f57efa6ae13f919ef3262c5`中，源集合修复后的完整真实专项97.95秒通过：真实12点CLI，以及合成AC/WA/CE/RE/TLE/OLE和取消共八组，全部停止确认。报告`/private/tmp/oj-reference-catalog-20261103/report.json`的SHA-256为`57ab672771a35ae732d865be8c582788c0c956c10f3c3a0ce74183933086d154`，plan为`12e87eb8d0771e34a8d187cb2bfc3b5cbf266a3d09768fc20194cfa4927f1325`，profile为`68557a3c0a3bdc8dc1afe488c1edaf9d4c0f08d425d962c2e45764c05d50fb13`；日志`oj-reference-catalog-real.log`。私有报告0700/0600，含全部身份和用量，不含源码/输入/答案/实际输出。

必须保留的失败：首轮驱动曾因cwd推导找不到宿主CLI，修正为独立产物路径；同轮WA停止确认失败并隔离，随后已有recover确认安全。最终目录修复后的额外12点CLI补跑21.60秒中全部点AC，但Stop未确认，最终报告正确为SE/failed、stop_confirmed=false并保留dirty（`oj-reference-shipping-real.log`及`oj-reference-shipping-20261103/report.json`）。之后同full ID的Docker回读为停止/PID0/Exit137，已有受控recover成功（`oj-reference-shipping-recovery.log`），没有手工清marker、重建容器或再跑至绿；后续恢复不追认旧报告通过。两次停止确认间歇失败的根因尚未定位，不能归因于负载，也不能以早先通过掩盖；这是当前实验入口的已知可用性限制，不作为生产或完整P4验收。临时证据不是备份。

## 2026-11-03：首批 Library Checker 文本语义组件

成功fetch后从`origin/main ebcca026ac71fc5dc13a9b9bc8445eb3623ac650`建立`codex/library-checker-text-admission`。P4独立受限验证前置新增`CompareLibraryCheckerText`与独立`lc-wcmp-tokens-v1`，显式每文件/每token预算，仅SP/TAB/LF/CR分隔、大小写/数字逐字节比较；配置错误、完整参考异常、输出超限分别返回。原M0空白、预算和错误优先级保留。支持域为UTF-8、单token至多64KiB、单文件配置上限128MiB；不是Testlib全域等价、已分配内存隔离或生产资源准入。草稿/Worker未接线，原版本、null生产配置、not-run及发布阻断不变。无依赖、SQL/schema或HTTP变化。

真实专项在已有镜像`sha256:fbfd84489b660f3cacaa8b211999116291589211abbacdf6f7d894f53f013afa`、Go1.26.10/GNU++17 g++12.2.0、linux arm64中执行，镜像无VOLUME；非root501:20、断网、只读根和两个只读挂载、4GiB内存/2CPU/128进程、1GiB临时执行盘、128MiB单文件、编译120秒/每次检查5秒及整体10分钟。十份checker和库头/hash.json均核验固定锁SHA-256，全部输入/答案独立核验上游hash.json，不用来源validation.json判定通过。先全量准入254份答案，再执行1026次正常/空白/额外/缺失token和十项边界差分，最终36.879秒通过。答案共509886584字节，最大10369461字节，103份超过M0的1MiB；本轮显式16MiB/64KiB实验预算不当生产限制。无效编码与65537字节token上游可AC但本站拒绝；`1\xffignored`的上游EOF哨兵AC而本站WA也实证，明确记录域外差异，未放宽输入来制造等价。

Docker完整离线模块验证/格式/vet/三范围静态/root race/七命令构建通过，最终补充回归与checkercompat标签静态检查通过。101治理12.439秒及repository通过；首轮治理因沙箱回环bind/环境读取限制失败，原样在授权环境重跑，无测试或阈值更改。针对性Astra设计/实现复核补充M0与LC错误优先级、非法编码先于token检查和EOF域外控制；AI复核不代替人工审批。源码、测试、支持域、计划、README及CI边界已同步，远端精确head CI/审查/合并另按门禁核验。原始日志在`/private/tmp/oj-checker-{compat-final,backend,final-targeted,governance-verified}.log`，临时日志不是备份；本轮没有本地PG/Redis/浏览器或完整真实Worker复验，不代表完整P4或部署。

## 2026-11-03：用户确认提交积压计量口径

用户明确答复采用P1训练积压建议：同一用户跨训练场景统计queued＋running，每份提交计一次；终态释放，超额只拒绝新增，同键重试保留原语义。具体限额继续要求显式配置，不预设生产值。本条取代#94交付时“尚未获用户确认”的状态，旧交付记录保留当时语境。

从成功fetch的`origin/main ebcca026ac71fc5dc13a9b9bc8445eb3623ac650`创建独立`codex/confirm-backlog-policy`，仅同步需求、积压规范和P1计划。已核对现有实现的主体/上下文计数、状态释放和原键优先行为与确认内容一致；本次没有代码、SQL、模式、配置值或部署变化，不重新宣称应用/容量验收。验证范围为完整文档差异、引用和仓库策略；远端门禁按文档影响范围核验，具体产品数值与其余P1要求继续待完成。

## 2026-11-03：本人草稿显式图片授权与静态预览

成功fetch后从`origin/main c7fd63db33d568c0c8022a762b4071e6aac4b938`创建`codex/private-draft-images`。本PR只补齐P4私有题面图片：可选非空`statement.images`逐项绑定该题来源锁原图及持久化逻辑引用，固定版本本人预览沿原会话/PG权限与材料槽读取，再最终复核。PNG/JPEG完整解码后限长重编码为静态PNG，原始文件动画、元数据和尾随载荷不进入响应；浏览器只显示清单精确路径，不请求原图URL，账号/页面失效清除。

原图/输出分别计费：最多16资产、每份原图/PNG各256KiB、每边4096、每张400万/合计800万像素；文本加原图、文本加输出PNG分别至多1MiB。全部引用/原始预算先验，无任何部分预览；浏览器另限前32次图片出现，未知/外链/HTML/SVG保持占位，坏图转说明。有限预算不是硬CPU隔离；不新增下载端点、依赖、SQL或schema。无图省略字段，现有十题文档/版本不变；旧API/前端不支持带清单/图片的新数据，回退会使其预览失败，恢复支持版本即可，无数据删除。当前固定十题无图，本轮可信合成PNG/JPEG证据不冒充真实图源质量或发布验收。

Docker Go1.26.10/linux arm64完整格式、模块验证、vet、三范围静态、root race与七命令构建通过；新增回归覆盖显式/未选角色、路径别名、锁内对象绑定、坏图/截断/伪SVG、尺寸/聚合像素、原图及真实JPEG扩张后的单张/聚合输出、末次授权与取消零结果。真实Redis/PG/双API协议49.578秒通过，带图预览仍拒绝读取中的退出/过期/撤权/禁用/恢复。前端依赖与现有镜像锁文件相同，lint/类型/105测试/构建通过；101治理、repository及diff检查通过。Astra完成设计与完整差异复核，无阻断发现，是AI复核而非人工approval。

首轮已有Playwright1.62.1/Chromium151.0.7922.71的15组真实浏览器与HTTPS→mTLS/Redis/PG夹具50.484秒通过：640×160授权PNG实际解码、1280×960和390×844无横溢出、零外链图片请求、页面失效清除。追加坏图占位验证后一次冷编译夹具超过90秒就绪窗，未进入UI，夹具及清理成功；未延长阈值或削弱断言，顺序重跑完整15组通过，真实夹具43.269秒，坏图占位与正常图片恢复通过，browser/fixture/container/Redis/cleanup均true，证据在`/private/tmp/oj-image-browser-shipping`。本次同步README、OpenAPI、草稿合同、前端设计、计划及M0当前入口。本站独立受限验证、完整题面质量、非作者终审和原子发布继续待完成，private-draft/not-run保持，未部署。

## 2026-11-03：服务器本人草稿列表与独立分页发现

成功fetch后从`origin/main 7bffd18e80a2f9f7f34d6dbb261deccbf658e868`建立`codex/own-draft-list`。本阶段补齐`/author/drafts`的服务器本人列表，使新浏览器、清空/禁用本机存储后仍能分页发现已提交草稿并进入真实本人预览；本机pending日志保持独立，不自动确认、替换、清理或POST。图片、编辑、独立受限验证、非作者审核、原子发布及训练提交继续待完成，不解除private-draft/not-run阻断，未部署。

新增`GET /api/v1/problem-draft-management/drafts`及独立可选能力；沿真实普通会话读取后比较单个规范`X-OJ-Expected-Subject`，再在同一短PG事务中持有恢复/账号/作者权限锁并按owner读取，commit成功才返回完整页。旧实例/缺能力404，无fallback。只投影既有八字段回执，不读/解析最多1MiB的document，不查材料/现候选或占材料槽，不提供他人计数。来源ID、创建时间、固定版本及草稿ID用于发现，标题留给真实预览，不加回填title列或跨页长事务。

固定20条，按created_at和id的C排序降序seek，first/after两个sqlc查询读取21条判定续页，游标取第20条。游标是规范raw-base64url的版本/owner/十进制Unix微秒/ID，仅seek不授予权限，真实actor/header匹配后才验owner；严格长度/编码/时间范围，前端不经Date/Number重建。仅列表允许单个cursor query，空值、重复、未知字段、编码别名和裸问号400，内部/未知/取消优先503，21条含哨兵都校验后才发布。不宣称跨页快照：created_at先于提交，晚提交旧时间戳可被旧边界漏过，须重查首屏，空/末页不是无在途证明。

schema8仅新增`problem_drafts_owner_created_id_idx(owner_subject_id,created_at DESC,id COLLATE C DESC)`；每次新草稿多一个B-tree条目和相应WAL/磁盘，无冗余document覆盖。普通事务CREATE INDEX需维护窗口排空/停止旧API/Worker/维护写入，独立迁移后启动匹配新版；旧进程不会因schema变化自动停止。新程序精确要求8，回退先停新版、Down8仅删索引保留草稿与权限后才能启动要求7的旧版；原Down7非空数据保护仍执行。README/会话与积压当前版本说明、API、计划、界面和CI约定同步，无新增依赖。

Docker Go1.26.10/linux arm64完整格式/vet/三范围静态/root race/七命令/sqlc通过。完整真实PG14.990秒通过：0/20/21/45、45条完全相同时间戳跨多页、微秒边界、owner隔离、三种撤销锁、死事务零结果、infinity/10000年拒绝、有数据8→7→8及精确schema门禁；实际事务屏障证明迟提交不可见/越界漏过及重新首屏发现，新时间提交不重复旧页。12,000行（两个owner各6000）权威SQL首/深页均为目标Index Scan、无Sort；深页Index Cond包含owner及ROW(created_at,id) seek，索引样本2383872字节，不当生产容量/性能承诺。计划原文保存在`/private/tmp/oj-own-list-postgres-verified.log`。首次root整合PG在CLI构建完成前过早启动，CLI验收失败，按build→PG顺序原样重跑通过，没有改产品或断言来通过；Terra早期定向输出未保存，不伪造原始日志，最终以root完整日志为准。

真实Redis/PG/direct TLS与pinned mTLS双API完整协议52.621秒通过：默认拒绝、同owner两已提交摘要、目录换版后旧列表、他人空列表、实际cookie主体错配、撤权/退出和正常排空。Docker前端类型/lint（含7项检测器）/103项测试/生产构建通过；独立控制器只留当前页，下一页/首屏都先Read，账户/页面/期限变化清除，迟到分页及只改时钟未跑timer的过期结果不复活；解码同时拒绝重复ID/请求键。100治理14.753秒、repository与OpenAPI本地引用验证通过。

最终已有Playwright1.64.1/Chromium141.0.7390.37的15组浏览器及真实race API/PG/Redis夹具42.239秒通过，浏览器/夹具/容器/Redis/清理均true。新档案禁用IndexedDB后通过真实API准备22条草稿，连同原2条共24条/两页，UI遍历逐ID与服务端相同且无重复、实际打开本人题面；发现流程仅GET、迟到下一页清除。服务器列表不会改写已提交但本机仍pending的原记录，实际A→B Cookie切换对四条管理路由409。1280×960/390×844首屏及全页截图已查看，无水平溢出、框架overlay或意外console/pageerror；Browser插件未列出，按skill使用已有Playwright，不安装新浏览器。证据位于`/private/tmp/oj-own-list-browser-qa-verified`、`oj-own-list-backend-full.log`、`oj-own-list-frontend-verified.log`、`oj-own-list-real-session.log`和`oj-own-list-governance.log`；临时证据不是备份，不扩写为题源十题页面或生产浏览器矩阵验收。

Astra设计及实现针对性复核无代码阻断，采纳非快照语义、微秒/C一致边界、最小DTO、重复请求键解码与精确迁移停启；补列表409 OpenAPI及不返回Set-Cookie的精确表述。AI复核不代替人工审批；完整候选/最终历史秘密扫描和精确head远端门禁按交付流程核验，未把阶段列表成功当P4/M1或部署通过。下一步推进图片授权，再做本站独立受限验证/审核发布。

## 2026-11-03：私有草稿导入管理与本机回执恢复

从成功fetch的`origin/main 1cf96c76934c6fe2cf735cfc46dc65ff42443353`建立`codex/draft-import-management`。唯一目标为`/author/drafts`来源选择、固定完整修订导入、本机原tuple恢复及接入本人预览，原按ID入口移至`/author/drafts/open`。没有服务端完整本人列表、图片、编辑、独立受限验证、审核/发布或训练提交，不解除private-draft/not-run阻断，不部署。

为避免已Read账号A但另一标签Cookie已变B的请求落入B，新增独立`/api/v1/problem-draft-management`三路及mandatory单个规范`X-OJ-Expected-Subject`。它只在实际会话Read之后、任何业务访问之前比较，不构造actor，保留最终真实会话/PG授权。独立namespace让旧二进制404而不是忽略header写入；旧两字段及先前绑定协议不变，新UI绝不fallback。基础设施/未知/取消优先503。无PG schema/SQL/索引及后端依赖变化。

引入idb8.0.3（ISC、无运行依赖，许可随站点分发）承载独立最小恢复数据库，仅owner/source/key/revision/pending或confirmed+draftId。readwrite同事务检查owner复合主键前缀及精确内容、活动source槽和32条上限，strict事务`done`成功后且记录仍匹配才POST；只新插入胜出者首次发送，pending无自动删除/驱逐/换键/换修订。确认/替换/清理CAS完整tuple，确认历史保留且旧操作不碰新pending。手动恢复先GET，404且原修订仍配置时才显式原tuplePOST；刷新、重新聚焦、通知没有自动POST。新修订先核验旧确认回执；本地confirmed不授予链接，必须本次真实回执匹配。退出保留恢复元数据，切账号清显示并仅读当前账号；不是AC21代码保存/退出清理或加密隔离。浏览器清库/回收/换设备可能丢失，可导出供人工核验，无自动回灌或跨设备保证。

Docker Node24.19/pnpm11.25类型/lint（含7项检测器）/98项前端测试、生产构建与pnpm audit全级别零已知漏洞通过；新增依赖版本/ISC/无依赖及LICENSE实读核验。`pnpm licenses list`因绑定安装的临时store索引已不在容器而失败，未将此命令记为通过；本次仅新增idb，已有许可图保持并补其许可证。Docker Go1.26.10/linux arm64格式/vet/三范围静态/root race/七命令/sqlc通过，真实PG全套13.800秒及双实际API回执专项通过。100治理11.401秒和repository通过；初次治理受沙箱回环bind限制，原样在授权环境重跑，没有改测试。

最终已有Playwright1.64.1/Chromium141.0.7390.37浏览器14组通过，1280×960/390×844截图已查看，无水平溢出或意外console/pageerror；真实HTTPS→pinned mTLS race API/PG/Redis夹具38.834秒通过，浏览器/夹具/容器/Redis/清理均true。实际导入先由服务端提交、再定向屏蔽成功回包，刷新只GET恢复原八字段回执，无第二POST；同修订只核验不再导入。实际A Read回包持有期间从HTTP将Cookie换为有作者权B，放行A响应后三条管理操作均409，B同键404、目录200且本机看不到A记录。真实IDB两连接并发仅一次创建、32条原子限额、旧确认/删除不碰新pending、损坏owner字段不漏检、配额失败零POST、未知schema不重建及清库后零内存重试通过；这不是两个完整管理标签并发验收。

针对性Astra审查推动修复“清库后内存重试”与“value.owner索引损坏漏检”，补原记录按key复核和owner主键前缀枚举；另保留合法BOM/来源字符。新控制器单测覆盖会话/页面清除、断开、只改时钟而timer未跑的迟到catalog/receipt，以及确认回执404/错ID/503不能创建新请求；追加非空记录在等待catalog/POST时到期的完整清除回归，最终Astra无剩余阻断，AI复核不代替人工审批。首轮浏览器最终B加载等待错误：账号空间变化会重建作用域，修正测试要求再次显式点击并验证没有自动GET，不改变产品满足测试。候选补丁秘密扫描无泄漏；合成检测器控制拒绝并脱敏，未放宽扫描规则。证据在仓库外`/private/tmp/oj-management-browser-qa-map-final`、`oj-management-frontend-map-verified.log`、`oj-management-backend-full.log`、`oj-management-postgres.log`、`oj-management-governance-verified.log`；临时日志不是备份。API/README/计划/架构与界面约定同步，最终head CI/审查及合并按门禁回读。

收尾自审发现合法请求键`propertyIsEnumerable`会命中普通对象的继承属性，使本地pending误显示已核验并生成undefined预览地址；不绕过服务端权限，但违反本次回执显示合同。追加修复使用ReadonlyMap，空缓存、复制写入和每条UI的get一致；载入/GET404无已核验或预览链接、零POST且原pending保留，只有完整匹配回执才确认，清除后Map为空。98项前端及14组真实浏览器重新通过，Astra对该修复最终无阻断；使用追加提交，不重写已推送历史，最终CI核验修复后的head。

## 2026-11-03：管理页的绑定导入与只读回执协议

成功fetch后从`origin/main 171bc6fefe9390bc5d53bf469c541107276c49a2`建立独立`codex/draft-import-protocol`。唯一目标是补齐管理页选题、完整修订绑定及未知回执恢复的[服务协议](problem-drafts.md#管理页导入协议)，随后再接页面；本阶段没有新增导入表单/本人列表、图片授权、受限执行、审核、发布或部署，不解除private-draft/not-run阻断。无依赖、schema、新SQL或索引变化，整个private_drafts仍默认关闭且须成功预加载启动。

增加三条能力：`GET /api/v1/problem-draft-sources`返回已授权且排序的配置摘要，前后真实会话/PG角色检查；`POST /api/v1/problem-draft-imports`严格source_id/request_key/expected_revision，授权后比较完整修订，再占原材料槽和复用原持久化事务；`GET /api/v1/problem-draft-imports/{key}`在当前权限和owner/key同一PG事务读取最小八字段回执，修订来自已存row.RequestSha256，不依赖现候选或材料槽/对象/关联查询。旧两字段POST及预览JSON保持兼容，不作为新客户端失败降级；新协议拒绝query/转义路径别名，沿同一TLS/pinned代理/Cookie/POST同源和no-store/nosniff/no-CORS/no-Cookie边界。

关键取舍是绑定Prepared.RequestSHA256而非仅Content.Version：同题面版本下index/来源报告仍可能变化，完整修订包含全部排序引用。修订变化409发生于任何材料I/O之前，且不证明旧请求没提交。404只表示本次没观察到回执，不锁不存在的键或取消在途POST；原source/key/revision必须保留，优先手动只读核验，不自动POST/换键/换修订。回执只在成功启用实例中脱离当前候选读取，不绕过配置或启动门禁；最终观察后的退出/撤权仍按已接受在途合同。

实测：已有离线Docker Go1.26.10/linux arm64基线及完整模块/格式/vet/三范围静态/root race/七命令/sqlc通过；真实PG全套13.214秒，新增回执owner/同键隔离、三种撤销锁、死事务不得改用其他连接、在途创建前404而后提交成功。原真实COMMIT回包消费后断连的未知提交，现先只读查得完整回执再原键重放；原2048行EXPLAIN ANALYZE继续自然使用owner/key唯一索引。单元覆盖同题目version但证据/index变化的旧修订拒绝且材料零I/O、满槽前拒绝、目录终验取消/失效不返回部分、返回切片隔离及回执不依赖候选/材料槽；新三字段与旧两字段严格分离。

真实Redis/PG协议50.852秒通过，双实际API的direct TLS/pinned mTLS覆盖未登录/无角色同样拒绝已知未知候选、源目录、成功响应体未消费便关闭后由另一API只读恢复、原tuple重放、换候选后旧回执仍可读、未提交旧修订409且无记录、外人同键统一404、撤权/退出与正常排空。成功体关闭的HTTP情况与实际PG COMMIT回包丢失分别证明，不把完整读取后忽略变量冒充网络丢失。100治理12.019秒及repository、OpenAPI YAML/全部本地引用验证通过。

实际十题专项在只读来源、全新容器私有材料根与临时PG中验证十份排序候选、完整修订、绑定导入/只读回执，254点/18样例及6449原文字节仍逐字节一致；子例10.16秒，完整PG22.763秒。785逻辑引用、622对象/1461321676字节在Store关闭后和新PG池重开复验。该大语料会话观察为stub，真实身份/HTTP由上述小型夹具单独验证，不宣称十题HTTP或管理页验收。日志为仓库外`/private/tmp/oj-import-protocol-backend-verified.log`、`oj-import-protocol-real-session-verified.log`、`oj-import-protocol-corpus.log`和`oj-import-protocol-governance.log`；本轮临时对象/数据库随容器退出清理，原来源/旧证据保持，临时日志不是备份。

验证过程中静态检查要求简化字段许可表达式，已修正且未关闭规则；首次真实API测试发现外层httpapi路由未转发新端点，改用共享公开路径谓词并扩展精确路由/代理拒绝回归，之后完整专项通过。Astra针对性复核指出完整修订须覆盖证据而非仅题面版本，采纳后最终无安全/一致性阻断，AI复核不替代人工审批。受影响README、API、CI/CD、计划和前端后续约定已同步，精确head远端CI及合并状态按交付门禁另行回读。

## 2026-11-03：共享题面渲染与本人私有预览页

成功fetch后从`origin/main 3c5b1a37457a1974f953625e03f78161ae862153`建立独立`codex/private-draft-rendering`。增加`/author/drafts`及`/author/drafts/:id`，显式检查普通会话后读取现有本人预览API；与示例做题页共用Markdown/GFM/数学组件和原文样例复制。只按已入库ID打开，不提供列表、导入表单、编辑、审核或发布；服务端当前角色/owner检查不变，无schema、API或后端授权规则变化。

渲染按路由懒加载，本地CSS/字体、跳过原始HTML、源图片零请求占位、受控链接及no-referrer；逐公式独立KaTeX配置和局部转义源码降级。固定KaTeX0.19.1及两个精确父依赖覆盖，避免上游旧0.16安全问题；生产依赖许可及字体许可保留随静态资源分发。题面128KiB、输入/输出AST20000节点/深度64、数学4096字节/128条/累计32768字节；包含math围栏。整题超限提供原文或有限节选，字体失败有可见原文入口。预算限制准入和输出，主线程解析不具有硬CPU隔离；完整图片/题面质量及生产低端设备性能另验。

私有数据仅在页面内存，7MiB流式响应/严格UTF-8及完整JSON字符串、精确字段/ID/内容版本/样例和1MiB原文预算，15秒超时且无自动重试。账号同步发布、凭据操作开始/finally、页面隐藏/恢复/focus及期限到达清除并取消，独立intent/generation防止会话检查与GET迟到复活，发布前再查deadline。跨标签只通知失效、不发送身份/正文、不自动重新读取；再次显示必须显式检查。BroadcastChannel为异步补充，不能瞬时撤回其他标签或已复制内容。

实测：Docker Node24.19/pnpm11.25的lint（含7项检测器控制）、类型、84项前端测试及构建通过；独立pnpm audit全部级别零漏洞，锁文件仅有KaTeX0.19.1。Docker Go1.26.10/linux arm64模块校验、格式/vet/三范围静态/root race/七命令和sqlc无漂移通过。100治理14.389秒、repository策略与完整候选补丁Gitleaks/合成拒绝脱敏通过。Astra针对性复核发现并修复延迟timer下到期成功发布、整题原文降级缺失和跨标签测试被焦点清理掩盖，最终无代码阻断；AI复核不代替人工审批。

最终浏览器专项在已有Playwright1.64.1/Chromium141.0.7390.37、1280×960及390×844运行：13组全部通过；真实HTTPS→pinned mTLS race API/PG/Redis夹具35.338秒通过，浏览器/夹具/容器/Redis/资源清理结果均为true。覆盖正常账号及未知回执原流程、本人工程草稿导入/GET、GFM/行内/块级/围栏数学、错误后正确公式、HTML/危险链接/外部图片零请求、冷context本地字体、逐字末尾换行复制、迟到响应、无焦点切换的跨标签退出清除且不自动Read/GET、字体阻断的可见转义原文、桌面/窄屏布局与无意外console/pageerror。

另对已准备的实际十题Markdown及六类恶意边界做renderer-only响应替换：16例最大打开耗时897ms（含Playwright操作/会话Read/模拟GET/页面提交，不是纯CPU），观测最大longtask76ms、题面DOM最多1410元素；十题均无公式错误。该语料不是十题真实API/身份/完整人工质量验收，真实owner/API由小型夹具独立证明。测量只代表本机Chromium，未宣称浏览器矩阵或硬时间上限。证据位于仓库外`/private/tmp/oj-render-browser-qa-final`（结构化结果、逐例测量、桌面/移动/字体失败截图）、`oj-render-frontend-verified.log`、`oj-render-backend-final.log`及`oj-render-governance-final.log`；临时产物不是备份。

环境修正：Playwright默认期望的Chromium1248未安装，明确选择已有1194/Chromium141，无下载；旧筛选断言补等待实际DOM提交，不降低内容断言；浏览器夹具增加新SPA路由回退，生产Nginx现有回退已支持；本轮生成pnpm缓存移到仓库外，不提交依赖/产物。P4仍需导入管理、图片授权、本站受限验证、非作者终审及原子发布；未部署、未解除not-run发布阻断，未声明P4/M1整体完成。

## 2026-11-03：本人草稿的固定版本题面与样例预览

成功fetch后从`origin/main b68a90f06a8be8104d5b5fc93d3b28104f9a1cd9`建立独立`codex/private-draft-preview`。唯一目标为P4管理页所需的[本人题面/样例数据入口](problem-drafts.md#本人题面与样例预览)：GET私有草稿`/{id}/preview`，沿普通TLS/pinned代理/Cookie/当前出题角色和owner边界；只选已入库statement及sample=true点，全部文档/逻辑引用与1MiB原文预算先验，再逐个匿名快照验证/读取/关闭，最后重新观察同一会话并同事务复核授权和固定元数据。只返回完整JSON，不替换/截断非法或超限材料，不执行或渲染HTML。

导入/预览共享已授权后材料活动槽，普通元数据GET、初次预检和HTTP输出不纳入该槽，不宣称全HTTP/跨API额度。预览依据PG固定版本，不跟随现候选版本；声明仍为private-draft/not-run。UTF-8保留CRLF/空白/空样例/BOM/NUL/末尾换行；JSON转义可能扩大六倍另加元数据。无依赖、SQL或schema变化，默认配置继续关闭；关闭整个private_drafts即可回退入口，资料保留。管理页面、Markdown/公式渲染、本站受限验证、审核发布及训练权限仍待后续，未部署。

实测：已有离线Docker Go1.26.10/linux arm64的模块/格式/vet/四检测器控制/三范围静态/root race/七命令通过，三包sqlc无漂移，普通真实PG全套12.465秒通过。最终真实Redis/PG协议50.486秒，其中双实际API直连TLS/mTLS代理4.52秒通过题面/样例读取、现候选换版后旧预览不变、owner隔离、撤权/退出；新增六组读取后会话边界0.51秒，实际退出/过期、角色/账号撤销和恢复换代均返回空预览。Local预算为1仍能逐份读取，多样例中间插正式点、后段引用错/预算超限零Open、实际缺失/损坏、非法UTF-8、取消、版本/source变化空结果及导入/预览双向活动竞争均有race回归。100治理14.202秒、repository及OpenAPI YAML/本地ref通过；Astra最终技术复核无阻断，不代替人工审批。

实际十题手动专项在只读来源挂载、全新容器私有材料根/临时PG运行：十题/254点/18样例预览共6449原文字节与原文件逐字节一致，子例11.32秒；完整PG套件23.515秒。785逻辑引用、622对象/1461321676字节在关闭Store后及新PG池完整复验。该语料用例会话观察为stub；真实会话/HTTP由上述小型夹具专项独立证明，不冒充十题HTTP或浏览器验收。容器退出清理本轮临时材料/数据库，保留原只读来源和宿主结果日志，不改前轮证据卷；临时日志不是独立备份。

首次新增混合正式点夹具因索引仍硬编码1点/1样例被生产校验正确拒绝，修正测试构造器按实际清单计数后通过。Astra指出最终元数据应同时比较SourceID，已补仅来源变化拒绝回归；短期过期夹具改为导入后独立namespace/新凭据，避免fsync耗时影响，不改产品期限或削弱断言。最终可达历史秘密扫描及精确head远端CI在交付门禁核验，知识库保存具体PR/合并索引。

## 2026-11-03：普通会话下的私有草稿 HTTP

成功fetch后从`origin/main b2245e452d4b85d3d8e7e796d700336491fd84d7`建立独立`codex/private-draft-http`。唯一目标为P4[私有草稿HTTP](../api/problem-drafts.openapi.yaml)：显式配置预置候选导入及本人元数据读取，复用同一普通会话TLS/Host、mTLS代理、Cookie/同源与PG授权边界。候选选择在当前角色预检之后，复制材料后再次读会话并沿既有同事务写入；不接收客户端路径/材料/主体。严格1KiB双字段JSON、JSON文档DTO、未知回执保留原键/精确版本、内容变化409、默认禁用、显式导入段并发及排空资源清理均已接通。无schema/依赖变更；管理页面、渲染、本站受限验证、审核/发布、训练前端继续后续，未部署。

Docker Desktop/现有离线Go1.26.10工具链：模块校验、格式、vet、四类检测器正反控制、三范围静态零问题、全部root race、七命令构建及三包sqlc无漂移通过。真实PG全套13.534秒，含既有迁移/授权/草稿提交未知与控制端回归；未增加SQL，不另造查询计划。真实Redis/PG协议全套49.786秒，其中新增双实际TLS API进程的直连/mTLS代理草稿场景4.53秒：默认拒绝、候选不可枚举、导入、跨API原键恢复同一回执、新候选内容409、本人读取/越权一致404、撤权、退出及正常退出连接回收通过。Linux专项确认首个Prepared加载后后续候选失败，以及Prepared/Local已打开后组装失败均无私有根FD残留。已有Runtime取消后实际调用返回再清理的race证据继续适用；本轮没有执行大语料HTTP、浏览器/管理页或Worker沙箱专项。

100项治理15.195秒、仓库策略/链接检查、OpenAPI YAML及全部本地ref复核通过。Astra针对设计、完整实现及最后guarded错误修正的技术复核无未解决代码阻断，不代替人工审批。已有Gitleaks对暂存补丁禁网脱敏扫描零命中，合成令牌拒绝控制通过；最终可达历史及精确head远端CI另在交付门禁核验，不以补丁扫描代替。

失败与修正：真实HTTP发现退出后Redis的明确ErrStale被identity guarded包装，初版映射误返回503；按原sessionDenied合同修正为明确拒绝401，存储/未知/取消及guarded conflict仍503，补单元回归并完整重跑通过。本地已有工具镜像缓存目录属于另一UID，离线派生镜像只校正缓存所有权；共享旧缓存另出现包加载失败，最终用独立容器临时缓存通过，未改依赖、断言或项目驱动。初次工具选择缺Go可执行文件，换用已存在的工具镜像；不把这些环境失败列为业务通过。

配置、契约、计划、题源与架构现状已同步；具体启用/移除能力及未知回执版本保留要求归README。`max_in_flight`仅限制已授权且命中候选后的材料导入段，预检/GET、跨API频率、磁盘总额、独立备份/容量及真实数据准入未在本阶段验收。远端状态以本变更PR的最终head CI及合并回读为准，知识库保存精确交付索引。

## 2026-11-03：获授权出题人的私有草稿入库与本人读取

成功fetch后从`origin/main f245055c8fa673e0106ea7b0604bf3ed9e78cdc6`建立独立`codex/private-problem-drafts`。唯一目标是让[统一私有草稿](problem-drafts.md#私有草稿入库与本人读取)成为实际材料/PG消费者：默认拒绝的最小出题角色、可信CAS/审计维护命令、真实普通Credential前后新鲜读取、全部选题材料Put成功后同一PG事务核对恢复/账号/角色与写入不可变文档/引用/原键回执，另提供当前角色下的本人读取。没有新增依赖、HTTP、管理页面、公开材料下载、审核、发布或生产判题配置。

schema7默认不给历史或开发账号授权；角色修订与审计同事务，业务锁序恢复→账号→角色→草稿，物料I/O在锁外。Down仅允许角色/审计/草稿均空，其他业务数据保留；旧schema6回归显式固定到自身迁移，保留原保护与锁竞态。请求内容摘要包含文档和所有证据/材料引用，未知提交保持原键；单token退出沿普通会话已接受在途边界，不宣称Redis/PG原子退出。

实测：离线Docker Go1.26.10/linux arm64模块校验、格式/vet/四检测器控制/三个静态scope、全部root race、七命令构建与三包sqlc一致性通过；真实PG/多API/控制端及维护CLI进程套件11.227秒通过。新专项涵盖默认拒绝/本人隔离、八路同键、引用故障完整回滚、初授及撤销审计故障回滚、三类实际撤销锁等待、授权连接被终止后不能换事务提交。真实服务器COMMIT/ReadyForQuery回包被测试传输消费后断连，实际`Commit`返回错误，`ErrCommitUnknown`及原键恢复单份完整记录通过。2048行权威查询自然使用owner/key、草稿和材料索引。100项治理15.583秒通过；普通会话真实Redis/PG专项45.647秒通过，其中新增六组草稿边界0.50秒，Put/hook位置断言确认复制后退出/过期/客户端故障/撤权拒绝写入。Astra复核的JSON大小写别名/缺false问题已补精确shape校验和回归，最后无代码阻断；AI技术复核不冒充人工审批。

实际Library Checker十题服务联调11.91秒：10题/254点/18样例，785逻辑关联、622不可变对象/1,461,321,676字节，在关闭Store及新PG池后完整读回；该大语料用例的会话观察为stub，身份效果由上述真实Redis专项独立证明。新专用卷`oj-private-drafts-evidence-20261103`独立回读622文件/字节/0700根/0600文件及UID501一致。维护镜像默认UID65532，缺DB配置拒绝且错误脱敏；实际CLI授予/检查/冲突/撤销/审计在隔离PG通过。schema7候选再跑真实HTTP→mTLS控制端→Worker→固定沙箱AC/WA闭环3.88秒通过，未执行来源程序；测试容器/数据库正常清理。

失败与修正：PG最初用UID501但镜像无对应passwd记录，随后冷缓存postgres验证发现历史迁移测试选错最新阶段及新锁观测误传空application_name；均按实际版本/连接名修正，没有放宽业务断言。完整进程测试需要先构建本分支Linux命令。最终专用离线工具镜像仅增加UID501记录，未下载依赖；真实Redis驱动继续用原postgres镜像，避免其预存缓存所有权不符。宿主治理首次因沙箱拒绝回环bind失败，获准本地端口后原100项全通过。维护目标的首次禁网构建缺原有锁定模块缓存，随后按原go.sum完成普通构建，无新增或升级依赖。跨平台产物顺序构建，七个Linux二进制先校验归档，避免覆盖混用。

远端首轮Gitleaks命中三处`generic-api-key`：一条查询计划测试的固定请求键及两条按文件路径作JSON键的SHA-256，均为合成夹具/源文件摘要，不是凭据。固定查询改为绑定参数，证据改为显式path/sha256记录，保留检测规则、拒绝控制及完整历史扫描；不加忽略项。用户按[安全规则](security-checks.md#告警与例外)明确授权仅替换本次任务提交，精确旧SHA的force-with-lease更新后实际全历史扫描零命中，原合成令牌自测仍拒绝且脱敏。随后CI定位到参数化修正多一级缩进，格式化并复验后正常追加修正提交，不再次改写历史。

源文件/日志哈希及范围归[机器证据](evidence/private-problem-drafts-20261103.json)。实际bundle、材料卷和本机原始日志不是独立备份。没有部署或授予真实账号；后续继续P4的HTTP/管理入口、本站受限验证、非作者审核和原子发布，完整训练权限/P5另行接通，不将本阶段记作P4/M1整体通过。

## 2026-11-03：本地不可变材料存储与可信维护命令

成功fetch后从 `origin/main eab59a975fb97e8541bfe994b57e774875174775` 建独立 `codex/immutable-material-store`。唯一目标为P4数据库草稿接入前置的[本地不可变材料存储](material-store.md)：预建私有Linux根、显式每对象/活动预算、流式长度/哈希、File.Sync→不覆盖Link→移除临时名→Directory.Sync、EEXIST独立校验/同步及未知写回执；读取先完整校验匿名快照，槽位持续至幂等Close，活动时拒绝Local.Close。新增独立维护镜像及put/verify命令，不自动创建或修复根，不引入依赖、数据库、网络接口、授权、发布或Worker接线。

Docker Desktop LinuxKit7.0.14/arm64、专用ext4命名卷 `oj-materialstore-evidence-20261103` 上，将十题草稿bundle的**597个去重对象/1,460,825,722字节**全部导入；全新容器不挂载来源，全部对象重开/完整哈希/长度复验通过，目录0700/文件0600及owner回读一致。维护镜像 `sha256:db3767069081d44714dc796b67295efba0b45dcd5c00d908718b18f3a38e9618`、默认非root65532，实测卷root预置UID501后使用同UID；没有运行题源程序。卷按owner标签保留为本地验证材料，不宣称独立备份或生产部署。

完整离线后端模块/格式/vet、四检测器正反例、三范围静态零问题、root race及六命令构建通过；存储包/CLI行为race分别3.077/1.007秒。双实例24写、双独立进程竞争、幂等不换inode、五个进程退出阶段、Sync/Link回执故障、EEXIST同步/取消、匿名快照、预算及Close均有行为证据。独立异主owner专项1.015秒通过；独占512KiB tmpfs真实写入ENOSPC、无最终对象及同实例后续写读恢复1.010秒通过。普通非root套件明确跳过两个显式专项，不把skip算作执行；完整治理100项12.057秒通过。来源、日志、命令实现及镜像/卷读数归[机器证据](evidence/material-store-20261103.json)。

失败与修正：最初测试/tmp缺exec、Go TempDir编号子目录按0777+umask创建导致根0755；分别修正可信测试挂载与显式夹具预建0700，未放松生产检查。Astra发现严格umask0777下新文件0000仍可能Put成功；独立子进程先复现重读permission denied，再改自身fd Chmod0600并验证通过。静态提示De Morgan条件已修正，完整重验；owner夹具首轮被内核DAC先拒，加独立容器DAC读取能力后确认应用可读仍拒异主。离线Docker镜像构建因原依赖层未缓存失败，正常构建只取得现有锁定依赖后成功，不新增或升级。AI针对性最终复核无阻断，不代替人工审批。

同步存储专题、草稿/计划、CI边界、README和文档索引。进程退出、tmpfs容量与ext4跨容器复验不证明断电恢复、共享配额、孤儿回收、业务发布或独立备份；后续仍需把草稿与材料按授权/验证合同落入数据库，当前M0门禁保持。远端交付以精确head的CI和合并回读为准。

## 2026-11-03：Library Checker统一私有草稿适配

成功fetch后从 `origin/main c5f003e94dd1f13465e9360de977dc7da3647b4b` 建立独立 `codex/library-checker-drafts`。唯一目标为[离线统一私有草稿](problem-drafts.md)：重新核对固定锁内162份来源及254点材料，将英文模板/参数/18样例转换成Markdown，保持来源测试顺序、显式精确大小写敏感token配置、内容版本及来源/许可对象。输入安全读取、同描述符流式哈希、自含内容寻址副本、0700/0600、完成前INCOMPLETE和独立搬迁复验均有实现；没有新依赖、网络、来源程序执行、数据库、公开接口或Worker门禁变化。

最终真实材料导入/搬迁复验 **4.675秒**，10题/254点/18样例；与此前独立目录导入版本一致，所有对象和引用哈希/权限通过。产物 `/private/tmp/oj-library-checker-draft-final-relocated-20261103` 可不依赖原候选目录复验，临时目录不是备份；机器证据见[索引](evidence/problem-drafts-20261103.json)。17项新增回归通过，完整治理 **100项、11.733秒**通过；首次受限运行的doctor回环绑定失败，按原断言授权重跑成功，没有降低检查。

针对性Astra复核发现并修正索引可遗漏/错误描述草稿、默认复验未拒INCOMPLETE，以及元数据先验后重读的竞态；相应行为回归通过。AI复核不代替人工审批。来源准备报告保留原字节并单独索引，本站验证仍not-run，生产限制/检查器兼容、题面渲染、权限/非作者审核和发布保持阻断。同步README、题源/题包设计、计划及文档索引；P4业务接线和训练闭环继续后续，未部署。远端状态以最终head的CI与合并回读为准。

## 2026-11-03：首批 Library Checker 离线题源准备

用户确认“Library Checker 起步＋官方 ICPC 赛事包扩充＋爬虫补充”。成功 fetch 后从 `origin/main 182b9700621d359f8a2b1e3a62ff81ab113f0285` 建立独立 `codex/library-checker-source`。唯一目标为[十题离线私有候选准备](problem-sources.md)：锁定官方提交 `73876f1cbcf082d2b22722924b924d3d65a2f00e` 的162份必要原始文件，以非root/断网/只读/有界Docker执行生成器、标程、validator和原checker，完整保留来源及Apache/MIT/Testlib原notice；不新增依赖、数据库、公开接口或任意上传执行服务。

最终脚本实测：现有本地镜像 `sha256:fbfd84489b660f3cacaa8b211999116291589211abbacdf6f7d894f53f013afa`、arm64、Python3.11.2、g++12.2.0，**10题/254点**原始生成数据哈希全部匹配、全部输入validator及参考答案checker通过，十个错误token控制均返回WA/1；生成与验证 **109.102秒**。静态归档独立精确集合/哈希检查通过，来源/数据及执行记录 **1,561,110,540字节**，另附宿主目录文件，输出 `/private/tmp/oj-library-checker-corpus-delivered-20261103/materials`（共672个文件），0700目录/0600文件，状态 `private-unpublished`。十题源内Testlib是 `0.9.40-SNAPSHOT`、checker大小写敏感，不替换本站默认比较器或视作生产SPJ准入。

最终17项来源/提取/驱动回归和完整83项治理（11.516秒）通过，覆盖来源损坏/遗漏/额外材料、路径/链接/重复/大小写冲突、文件与metadata读取预算、已有目录保护、实际日志文件上限、导出/清理失败和完成顺序。真实SIGINT专项退出-2，仅清理该轮容器，保留INCOMPLETE且没有完成包，另一轮实测继续运行；正常结束已回读无本轮容器残留。CI不运行外部题源程序；本轮未改变业务路径，应用/集成/容器与安全检查以PR精确head的普通CI结果为准。复现命令归README，日志、制品和运行脚本哈希归[机器证据](evidence/library-checker-20261103.json)。

失败与修正：首轮254点已通过，但Docker cp无法读取tmpfs的导出目录，两次小容器探针复现（含确保文件创建后的对照）。改为受限exec USTAR流，终止并回读容器后才在宿主校验，最后完成候选；无宿主输出写挂载或权限扩大。初次治理因本驱动生成的两份pyc残留失败，显式关闭本地字节码并仅清理本任务两份残留后原样重验通过，未放宽策略。针对性Astra复核要求的锁快照一致性、日志硬上限及完成前清理均已落实；AI复核不充当人工审批。

同步计划、题包设计、M0当前题源口径、CI边界和README。P4正式适配/私有草稿/权限/审核/发布、题面模板转换/渲染、生产检查器与资源准入、训练页面及NWERC/BAPC真实包矩阵继续后续；当前Worker固定M0门禁保持，未部署。远端状态以最新head的CI与合并回读为准。

## 2026-11-03：P1跨API共享未终结积压准入

成功fetch后从 `origin/main 37cf338021f5840843e6979fa013d6bacce0735f` 建立独立 `codex/shared-submission-backlog`。唯一目标是[共享积压准入](submission-backlog.md)：schema6单一显式策略、全局/主体跨上下文queued＋running权威计数，事务内锁后使用新语句快照，再同事务写提交/任务。满额或缺策略仍优先返回原回执/409，降低限额不删除已接受任务；没有冗余计数器、生产默认数值、公开管理接口或频率限流/公平调度变化。新增独立 `migrate backlog GLOBAL SUBJECT`，原fixture不暗设额度，升级必须停全部旧写API后迁移/配置/启动同版；已配置或含任何提交的schema6拒绝Down，须向前修复。跨上下文计量是已落实验证的工程选择，尚未作为用户确认的产品口径。

本地实测：Docker离线工具链完成模块校验、sqlc生成、格式/vet/lint、完整root单元/race及构建；真实PostgreSQL/race **180个含父级PASS条目、15.421秒**，包括最后名额竞争、主体跨场景、锁等待后的回执/计数新快照、策略提交/回滚、取消不冒充额度拒绝、终态释放、重复回报和Down并发保护。权威计数SQL实际EXPLAIN ANALYZE/BUFFERS在2048终态/3未终结样本选用unfinished部分索引；记录索引写/WAL/vacuum及扫描代价，不宣称固定查询成本或容量。两个真实API进程验证未配置503无写入、最后名额202/429、满额原键200/冲突409、原24并发幂等及重启/v3兼容。

固定Docker Desktop LinuxKit7.0.14 arm64、镜像 `sha256:cd40ffdb32619806a8d97e59fe03e890fe4066835f57efa6ae13f919ef3262c5` 上 `python3 scripts/test-worker.py all` 九个独立专项全部通过，106个含父级PASS条目；初始宽选的9个SKIP中，8项由all后续显式执行，浏览器另有独立实测，不将SKIP算作PASS。浏览器独立 **15.48秒/14项**，真实AC/WA、丢弃成功POST响应后的原键恢复、桌面/390px等通过；新增429/ADMISSION_UNAVAILABLE展示为明确标记的路由注入，不能冒充真实后端并发证据。五个原服务未重启，旧#89分支/工作区/三处未提交修改及HTTPS工作区保留，未部署。

失败及修正：首次PG基线仅未来schema常数6因新模式变更而失败，改为7；随后新增隔离夹具修改typed config再调用ConnString导致search_path丢失，只在本轮可丢弃数据库的public混用并触发重复数据失败。修正为stdlib.OpenDB/pgxpool.NewWithConfig深拷贝，实际核对迁移及两pool的current_schema与四表归属后，完整重跑通过；不是生产Store缺陷。前schema6的历史迁移测试改用四组严格合成白名单的test-only夹具，保留快照/竞争断言，无正式准入旁路。旧失败日志分别保留为 `oj-backlog-pg-baseline.log`、`oj-backlog-pg-full.log`；成功证据归 `/private/tmp/oj-backlog-{pg-isolation-fixed,backend-quality,worker-all,browser}.log`、`oj-backlog-browser-qa/result.json`，31份运行代码/测试及日志哈希归 `/private/tmp/oj-backlog-evidence.json`。普通doctor在受限沙箱无Docker权限，授权只读复核Engine可访问；8080由原服务占用，专项使用独立随机端口。

同步README、OpenAPI、架构、计划及M0当前进度；原#93验收JSON/旧日志哈希不追改，其“owning路径无变化”只适用于原审计快照。完整目标仍含频率限流、DOMjudge虚拟时间公平调度、真实训练权限/题库与生产验收；测试的10000/10000不作为试用阈值。针对性Astra工程复核未发现阻断，AI审查不充当人工批准；远端交付以精确head的CI及合并回读为准。

## 2026-11-03：固定合成环境M0技术证据收口

成功fetch后从 `origin/main da7ef2c` 建立独立 `codex/m0-acceptance-record`。唯一目标是按开发计划的显式M0条件收口证据、资源与限定组件准入，不新增业务功能、依赖、schema、预算、扫描规则或部署。新增[技术验收记录](m0-acceptance.md)及[机器索引](evidence/m0-20261103.json)，将条件→owning test/断言→实际命令/结果/日志哈希→当前源码适用范围串联；P1额度/公平调度继续按原文列为试用前gate，不把全部P1/P2或产品开放记为完成。

当前源码已含#92页面（`da7ef2c5739b76384225d507bab195c47d1d7fd1`），精确head `26614038e48139cbb9c9f26a7c9b78a9d73ca03c` 的九项CI成功且合并树等价。回读四份旧M0日志、九项Worker联合及浏览器/资源/Store采样原始哈希：从2b44a50到当前17个数据库/会话等owning路径无差异，从742d77f到当前16个执行/Worker等路径无差异；18份页面运行代码/测试/契约与11.95秒/12项的整合实测一致。Store原始42样本/86016请求的SHA和8份相关源码仍匹配。没有仅为统一SHA重复同一应用专项；CI、特权Worker/浏览器及独立会话实验分别记录，不相互冒充。

只读核对现有Docker29.8.1/LinuxKit7.0.14/arm64、当前10CPU/8319504384字节及三镜像身份；禁网只读容器未找到完整服务端许可原文，不据缺失推定许可。通过gh只读获取Redis7.2.17、PG14.24、GCC15.3.0的固定上游COPYING/COPYRIGHT、GPLv3/Runtime Exception3.1，并解析标签至不可变commit；另核对GCC与libstdc++源码notice。来源、blob及原文SHA进入索引，内容不执行、不安装或上传仓库。固定来源允许继续现有隔离合成实验，不推导完整镜像许可/二进制可复现、分发或生产准入；OS SBOM/漏洞扫描仍属既有独立后续范围。

资源记录区分配置预算、有限样本和故障探针：编译/Run预算保持，进程峰值16、4094空文件/inode、64MiB字节及1MiB单文件证据有明确范围；单次编译CE的CPU/内存不是容量基准。旧Store采样的tmpfs、关闭持久化、外围窗口与非HTTP限制保持。异常槽仍需可信管理员确认旧宿主操作结束后显式恢复；未知回执不等于回滚，旧备份不恢复其后的账号安全事实。

结论为固定合成环境内显式M0技术条件逐项满足，不记录用户产品验收或发布决定。P4题源仍未决，M1真实身份/权限/题目发布/完整训练、生产目标负载与独立备份继续待验；旧PR#89分支及用户HTTPS工作区保持。原始证据仅本地/private/tmp，机器索引不是原始日志附件，迁移环境前须另行保全。

470文件仓库策略、11项条件/11项证据关联、本地artifact与源码SHA、17/16组历史owning路径及238处Markdown路径/锚点检查通过；发布前固定禁网检测器的完整历史候选扫描零命中。Astra针对性完整复核无实质阻断，补明文本检查预算及原ID/本次StartedAt恢复语义；不构成人工审批。JSON证据文件是单份逻辑审计索引，保留同一文件以便逐项关联；既有CI对该新路径保守执行全部应用套件，不修改分类规则。无应用变更，本地不重复相同应用专项；最终head仍须通过全部远端门禁。

## 2026-11-03：M0 固定合成题最小浏览器闭环

成功fetch后从 `origin/main ff94039` 建立独立 `codex/m0-browser-loop` worktree。新增默认关闭的 `/m0` 固定示例页及三条小型JSON路径；API须另设 `OJ_EXPERIMENT_BROWSER=1`、私有Bearer实验配置、显式127.0.0.1监听和v3材料，实际随机监听后再构造严格Host边界；冲突配置在读取私有文件前拒绝。独立 `m0-browser-operator/m0-browser-training`，服务端仅选择仓库AC/WA源码，不接任意源码/身份/语言，不在API执行程序；原Bearer接口仍拒绝Origin。nonce用于CSRF而非账号身份，同源脚本不在隔离保证内；数据GET需nonce，POST另验单一Origin/Fetch Metadata及标准库防护。页面不使用/设置会话Cookie，不开放CORS，带CSP/no-store/nosniff。

页面沿用现有系统字体/橙色操作和浅色分隔，不新增框架/依赖。sessionStorage仅保存原键/预设/版本及unverified/confirmed/conflict状态；每次POST前先保存未确认，合法POST回执才确认，GET不升级绑定，刷新只重新bootstrap并核验原键。404/503/超时不等于回滚，不自动换键；409后既有结果不能归为当前示例，冲突/未知刷新后保留。终态后显式核验仍发GET，有限轮询可停止但不取消任务；BFCache返回重新加载，存储失败不发送POST。

修正候选的 `browser-loop` **10.42秒**通过（`/private/tmp/oj-m0-browser-scan-fix.log`、产物 `/private/tmp/oj-m0-browser-scan-fix`）：现有Playwright1.57.0、Node22.17.1、实际Chromium145.0.7632.6，桌面1280×960/390×844，12项交互/视觉断言。真实POST已接收后 `route.fetch` 再丢弃响应，页面未知；刷新仅GET，再以原键/原示例POST回原ID。浏览器确认queued后以原子marker通知，SQL旁证两任务/零attempt，才启动真实mTLS控制端和Worker。随后浏览器观察AC/WA；含原Bearer隔离夹具共三任务均finished、各一次completed attempt/双代次1，原始固定源码/独立归属未变，负面请求及重试不产生额外任务。Worker显式排空exit0及槽停止，未独立观察选手PID。

另验真实Host/Origin/额外source/nonce拒绝、已有Bearer行对浏览器404、终态再查询、409→查询→刷新区别、后退恢复、真实原键404、注入查询503恢复、存储写失败及无Bearer/Cookie/外部请求。5条资源错误仅出现在明确的丢回执/409/404/503测试路径，其他console/pageerror零；注入GET 503不冒充真实数据库故障。实际computed字号15/32/21px、正文/辅助字与按钮对比度均≥4.5、控件49px；桌面/窄屏截图经代理目视核对，无横向溢出。离线Impeccable检测器无法映射API绝对assets路径，字体/层级推断不完整；既有sidecar漂移未擅自修复，以实际CSS/渲染交叉核验。

原 `http-loop` 因共用启动/排空helper回归 **3.43秒**通过（`oj-m0-browser-http-regression.log`）。最终离线Docker后端格式/模块/vet、四检测器正负夹具、主模块/sandbox/独立会话模块静态各零、root race及五命令构建通过（`oj-m0-browser-backend-verified.log`）；19项配置、32项拒绝零Store调用及错误/nonce/静态隐私边界均覆盖。治理66项 **11.742秒**通过（`oj-m0-browser-governance.log`），467文件策略、OpenAPI YAML及53个引用核对通过。首次局部测试因未用导入失败、首次完整静态提示路由条件简化，均修正后完整重验，未降低门禁。

Astra设计/完整实现及最终增量复核发现并修复终态零请求、冲突归属丢失、BFCache中止器失效及marker半写竞态；不是独立人工审批。首次真实浏览器12.05秒，排版/中文状态后10.01秒，原子发布/内容绑定/诊断后10.55秒，新增computed视觉证据9.98秒，公开命名常量的变量改名候选取得上述10.42秒；不把较早运行冒充修正快照。契约、README、架构、前端边界、CI与M0进度已同步。无预算、材料版本、SQL/schema、迁移或依赖变更；关闭开关/回退模块即可，已接受任务不删除。普通CI及 `all` 不执行此依赖已有浏览器的特权专项；未重跑全部Worker、账号浏览器或其他浏览器矩阵。M0最小合成页面补证不代表M1训练、完整M0或生产准入通过，未部署。

初始PR #89的head `fed0592` 在CI秘密步骤失败：固定gitleaks8.30.1的generic-api-key规则将页面与测试中同一sessionStorage公开命名常量误报，共2项。禁网完整可达历史扫描复现并100%脱敏定位；该名称不是认证凭据，也未泄漏随机nonce/Bearer。修正只将变量改为明确的存储命名标识，保留实际存储字符串与数据兼容，不添加忽略或改扫描规则。隔离容器内从完整main历史加修正补丁构造候选，89提交扫描零命中（`oj-m0-browser-candidate-scan.log`），扫描器自测仍拒绝合成令牌并脱敏；旧head仍有历史命中，不能以追加修复或其他CI成功冒充安全门禁通过。最终交付须重新核验修正head的完整CI与Git授权边界。


在 #90/#91 合并后，将修正补丁整合到 `7e59f6a49425f2bf80d5d0eae4ca502c60d2cfb4` 的仓库外普通副本 `/private/tmp/oj-m0-browser-integrated-pg9234w7`；准备阶段没有建立替代项目分支或改写已推送历史。仅两份文档上下文需手工合并，18份运行代码/测试/契约与已复核修正候选逐字相同，所有其他主线文件及 #91 联合记录/历史段落保留。用户于2026-11-03明确选择“新分支替换PR，保留旧分支”，随后成功fetch并从同一最新main建立 `codex/m0-browser-delivery`，应用整合补丁后、追加本交付记录前，468份源码哈希全部匹配。旧 `codex/m0-browser-loop` 及工作区保留，替代PR建立后关闭 #89；不重写历史、不改变main或扫描规则。本地候选证据不代替替代PR精确head的远端CI与合并门禁。

整合后完整禁网Docker后端格式/模块/vet、检测器自测与三组静态零问题、root race/五命令构建通过（`/private/tmp/oj-m0-browser-integrated-backend.log`）。治理66项13.466秒通过（`oj-m0-browser-integrated-governance-verified.log`）；首次未提升工具权限时两项因沙箱禁止回环监听失败，获准原样复验，未改断言。468文件仓库策略及25文件差异/277个本地Markdown链接锚点通过。现有Playwright1.62.1、Node22.17.1与Chromium145.0.7632.6的真实专项 **11.95秒**、12项检查通过（`oj-m0-browser-integrated-browser.log`、产物 `/private/tmp/oj-m0-browser-integrated-qa`），桌面/390px的AC/WA及未知回执恢复继续通过；这是新工具版本的独立读数，不替换此前1.57.0的10.42秒证据。未重跑完整账号浏览器矩阵，未宣布完整M0通过。

最新main完整历史加整合补丁在禁网固定gitleaks镜像中零命中（`/private/tmp/oj-m0-browser-integrated-scan.log`）；隔离验证夹具的 `git rev-list` 为92个可达提交，检测器原始报告91个commits scanned，两项计数分别记录。此临时容器提交不是项目交付head；最终新分支还须接受普通CI的完整历史扫描及检测器自测，未添加ignore。

## 2026-11-03：M0 Worker联合复验及组件证据核对

独立文档目标从成功fetch后的 `origin/main f7f7871` 开始，不依赖尚未合并的浏览器候选。#90 的精确head `742d77f23c3d6120e3499f98943b875b4c9ec165` 经 [CI 41520823800](https://github.com/DL-Association-for-Computing-Machinery/OnlineJudge/actions/runs/41520823800) 8项成功，frontend-tests按后端范围明确跳过，完整差异/反馈/规则核对后squash合并为 `f7f7871b39568c1059e5e781bc63e6a68eea2aa2`；已证明树一致并归档5个任务二进制，清理仅该已结束worktree/本地分支。

合并前在同一精确head执行 `python3 scripts/test-worker.py all`，退出0，9个独立套件PASS、106个PASS条目（含父项）；基础阶段跳过的8个显式专项随后都实际通过。含新编译文件上限对照、正式Run SIGXFSZ/OLE、双Worker/材料/数据库/槽/网络/编译失权与双API HTTP终态；真实编译失权停止/PID证据9.856/6.856秒均小于14秒界限。准确父项耗时、日志哈希及历史会话/数据库证据的源码适用核对归[M0记录](m0-experiment.md#worker-联合复验与证据适用范围2026-11-03)，不能将含父条目的计数当独立场景数。

禁网只读提取当前镜像版本、8份直接Go模块许可原文、工具链/go-judge LICENSE及462条dpkg记录，不下载、安装或上传材料。当前PG实际14.24，与文中早期14.19区分；仅当前可变tag读数，不倒填旧日志。同步[M0组件准入](m0-experiment.md#组件准入与下一步)的原文哈希、限定实验结论和未知项；完整服务端/OS/工具链许可、传递依赖图、SBOM与OS漏洞检查仍未完成，未将客户端许可冒充服务端许可。当前CI的Go可达漏洞/前端依赖扫描范围也不能外推为完整镜像安全。

仅修改本日志及M0记录，历史命令/耗时不回写覆盖，无应用、依赖、预算、协议或扫描规则变化，不为纯文档重复应用构建。完整M0、生产准入/部署仍未通过；最小页面交付与最终限定技术验收仍待补齐。

## 2026-11-03：固定g++编译文件上限的真实分类证据

成功fetch后从 `origin/main ff94039` 创建独立 `codex/compile-file-size-evidence` worktree。本项不依赖未合并的浏览器功能，只核对P2剩余疑点：固定g++遭遇文件大小限制时是否实际进入未支持的Compile OLE分支。新增有限合成对照及现有resource-bounds调用，无业务、适配器、协议、材料、SQL/schema、依赖、预算或命令变化，LANG=C沿用原profile。

既有资源专项基线 **1.17秒**通过（`/private/tmp/oj-compile-size-baseline.log`）。最终专项 **2.62秒**、新增编译子项 **1.45秒**通过（`oj-compile-size-final.log`）：256KiB非零只读数据的A+B控制例AC；1MiB+128KiB示例仅编译，原始Nonzero Exit Status/exit4、CPU17350000ns、内存9953280B，零fileError/执行错误/制品，无输出截断。固定编译器报告汇编器受文件大小限制终止；诊断sha256为 `c895d39351950702898165eb6a70f7ef517b380b4c8ee1b4e79073577a0a101e`，文本仅作辅助定位，不作为CE分类依据。

正式Runner按既有非零退出规则得到CE，超限例一次Compile、零Run、两点NOT_RUN/无Usage或Max；随后同一Runner普通AC，合计3Compile/4Run。透明观察器只保存有界原响应，不重发或改写；超限例意外到达Run会直接错误而不执行，SE/适配错误早退保留停用合同。控制、CE及复用后缓存均为空。此固定路径没有落入无采集错误Compile OLE，不能外推该未知分支或所有编译异常已支持；未独立追踪汇编器PID/内核信号。

首轮及补诊断轮因测试先猜exit1与File too large文字而失败（`oj-compile-size-first.log`、`oj-compile-size-diagnostic.log`）；实际两轮均是exit4/正式CE/无适配错误。依据原始记录修正为固定工具的真实强断言，未放宽生产规则或忽略失败。完整离线Docker格式/模块/vet、四检测器正负夹具、主模块/sandbox/独立会话模块静态各零、root race及五命令构建通过（`oj-compile-size-backend.log`）；Astra设计及完整差异复核无阻断，不替代实际测试或独立人工审批。

README/CI/判题归因/M0同步；本轮未重跑其他Worker、会话或浏览器专项，不据此宣称完整P2/M0、生产准入或部署通过。回退仅移除测试及调用，无迁移。

## 2026-11-03：HTTP 到真实 Worker 终态联合验收

成功fetch后从 `origin/main 44b1417` 建立独立 `codex/http-judge-loop` worktree。新增 `http-loop` 专项（也加入all），仅测试/驱动和文档，无业务路由、授权、数据库、材料、依赖或预算变化。正式迁移/固定夹具命令初始化空的临时PG；两个真实提交API共享私有实验凭据，通过HTTP提交仓库固定AC/WA源码并确认queued，然后才启动实际mTLS控制端及Worker命令，最终经HTTP观察finished/正确判定，不以SQL代替请求或终态。

验证跨API原键重试/原键核验/ID查询、异源409冲突、缺失/错误Bearer与Origin拒绝、客户端subject拒绝且零提交/任务/attempt副作用；回执严格六字段、no-store、无Cookie/CORS，Location必须为自身相对路径。SQL只作原始源码/请求摘要、固定实验归属及两题各一次completed attempt/双代次1的旁证。Worker经SIGUSR1正常exit0，槽停止须显式断言；未观察选手PID，不扩写为本专项独立PID证据。

提交API用127.0.0.1:0，从自身私有有界完整JSON日志取得实际地址；请求禁代理和重定向，不跟随任意Location，不打印私有日志/凭据/响应正文。控制端复用原临时端口流程，不宣称所有监听均消除预留窗口。20项启动解析边界的Docker race **1.020秒**通过；原HTTP/experiment race基线分别1.024/1.026秒。首次真实专项 **3.83秒**通过，收尾检查细化后重跑 **3.67秒**通过（`/private/tmp/oj-http-loop-first.log`、`oj-http-loop-final.log`）。

最终源码真实专项 **3.40秒**通过（`/private/tmp/oj-http-loop-verified.log`）。完整离线Docker后端格式/模块/vet、四检测器正负夹具、主模块/sandbox/独立会话模块静态各零、root race及五命令构建通过（`oj-http-loop-backend-verified.log`）；治理64项 **12.343秒**通过（`oj-http-loop-governance.log`）。后端检查先发现超时收尾遗漏错误检查，显式空赋值又被staticcheck拒绝；最终分支将退出错误状态纳入明确失败报告，不忽略门禁或降低断言。456文件仓库策略及差异检查通过。Astra审查完整新增文件及驱动差异无阻断，不替代实际测试或独立人工审批。

本项仅受控合成HTTP链路，不包含接收回执丢失注入、浏览器页面、普通账号训练授权、全部Worker专项或生产容量复验。最小页面及其失败路径仍待实现；完整M0未验收，未部署。可单独回退专项/选择器，无数据库回退。

## 2026-11-03：普通Run的SIGXFSZ/OLE精确适配

成功fetch后从 `origin/main 79dbc5e` 创建独立 `codex/sigxfsz-verdict` worktree。本PR修复PR #86实测发现的分类缺口：原正式适配器将无采集错误的已确认SIGXFSZ/OLE当系统错误，停用旧执行器。GoJudge增加无fileError OLE的精确Run/exit25例外，要求完整非负资源、stdout/stderr为非null字符串、无执行错误/任何制品ID且输出有界；保持OutputTruncated=false。局部字符串指针保留null信息，仅新例外区分空串/null，其他分支仍按原空值解码行为处理。Compile仍拒绝该无采集错误分支，原collector截断及其他状态语义保持，不按错误文字或仅信号猜结果。

先新增边界回归并在旧代码真实复现三个合法正例被 `missing output overflow evidence` 拒绝（`/private/tmp/oj-sigxfsz-before.log`），再做最小实现。修复后最终执行包race **1.998秒**通过（`/private/tmp/oj-sigxfsz-unit-final.log`）：26个接受/拒绝子例（含两个显式null）、4个非OLE状态的exit25原语义、首点OLE/后点NOT_RUN、制品清理/同Runner再AC，以及未截断布尔值变化确实改变冻结摘要。UTF8替换扩张长度界限不是原始字节保真承诺；数据库只存结果和摘要哈希，不将其冒充可读取的截断字段。

最终 `python3 scripts/test-worker.py report` **16.17秒**通过，新增 `file-size-lifecycle` **0.80秒**（`/private/tmp/oj-sigxfsz-report-verified.log`）。有限17×64KiB单文件写触发原1MiB FSIZE，经过真实适配/租约/回报得到OLE；直接观测Status OLE/exit25/OutputTruncated=false。OLE确认后才入队AC，同一个Worker、LeasedRunner及容器ID/StartedAt/重启/运行状态完成两题，合计5次编译/运行调用、Stop回调零次，两题各一次attempt、双代次1；两个控制端重报均一致、冲突均拒绝，最初持久载荷逐字节不变，缓存清空。Worker在第二次确认后排空，未靠取消执行、停止/恢复或重建槽取得成功。

本项复用测试进程中的单个Worker对象和两个真实TLS handler，不能称为SIGXFSZ跨API进程或完整cmd/worker进程专项；同轮仍运行原有真实Worker命令、排空、AC/WA/CE/RE/TLE/MLE/stdout-OLE、过期/撤销和未知回执回归。首轮真实report也成功，加入直接Observation观测及null边界后重跑取得上述最终结果。Astra设计、完整实现及最终增量复核无阻断，未代替实际测试或独立人工审批。

完整离线Docker后端格式/模块/vet、四检测器正负夹具、主模块/sandbox/独立会话模块静态各零、root race及五命令构建通过（`/private/tmp/oj-sigxfsz-backend-final.log`）；治理64项 **12.379秒**通过（`oj-sigxfsz-governance.log`）。文档同步判题归因、README和M0当前缺口；无预算、材料、依赖、协议、SQL、迁移或摘要schema变化，不改旧结果。未重跑全部Worker/resource-bounds或浏览器，不将其算本轮通过。完整M0、最小HTTP/结果页面与生产验收继续待验，未部署。

## 2026-11-03：沙箱资源预算的独立行为证据

成功fetch后从 `origin/main 7427c94` 建立独立 `codex/sandbox-resource-bounds` worktree。本PR只补P2资源预算行为专项及其驱动/文档，新增sandbox-tag Go测试和嵌入的有界C++合成探针，扩展测试响应字段以保留cgroup峰值及执行/采集错误；无业务代码、预算、镜像配方、材料、协议、SQL、迁移或依赖变化。

最终 `python3 scripts/test-worker.py resource-bounds` **1.14秒**通过（`/private/tmp/oj-resource-bounds-final.log`，6个子例/8个含父PASS条目）。进程探针最多32次父进程fork且不递归：15个阻塞孩子+父进程，cgroup峰值16，下一次errno11/EAGAIN，NPROC软限大于32；全部wait后再次fork/wait成功。inode探针4094个空文件+目录，errno28/ENOSPC、free inode零且仍有67108864可用字节，删除/重建一个文件后再次满额。

总字节探针沿用Compile的10秒CPU/15秒wall/512MiB/16进程资源数值，最多80个文件、每个最多1MiB，每次64KiB无缓冲真实write；两例均写入67108864字节后ENOSPC、可用字节零、inode仍4029，内存67895296字节。分别保留满盘状态给下一次执行，以及先close再删除全部填充文件、核对容量恢复再写入4096字节。所有探针后正常执行核对目录不存在、字节/inode恢复原值，结束删除唯一缓存制品；不靠驱动收尾制造清理成功。

普通Run预算不变：单文件有限1MiB+64KiB写入由既有RLIMIT_FSIZE=1048576停止，原始OLE/exit25、Error空且FileErrors零，内存1572864；分文件写入先MLE，内存33570816。首轮将总容量当单文件写入，三例在FSIZE先失败（`/private/tmp/oj-resource-bounds-first.log`），不能记作总容量或MLE通过。核对固定v1.13.3的worker/envexec源码后区分全局OutputLimit、采集器4096和copyOutMax；不通过增加copyOutMax或修改预算解决。第二轮分文件方案1.07秒通过，再补实际fileError/ProcPeak/文件计数交叉证据取得上述最终结果。

这同时确认正式GoJudge适配器目前拒绝无采集错误的OLE，已知SIGXFSZ会升级为系统恢复/旧槽停用，属于下一独立修复目标；本PR不宣称此正式回报路径通过。Astra针对设计、实现与最终增量复核无阻断，未替代实际测试或独立人工审批。

完整离线Docker后端格式、模块校验、vet、四检测器正负夹具、主模块/sandbox/独立会话模块静态各零、root race与五命令构建通过（`/private/tmp/oj-resource-bounds-backend-final.log`）。首个临时复制遗漏deploy配方导致环境自检失败，补齐文件后完整重跑，不改测试/断言；失败日志 `oj-resource-bounds-backend.log` 保留。既有sandbox专项前后各49个PASS条目（含父条目）通过，最终编排5.28秒、宿主1.13秒、租约停止1.90秒、基础沙箱2.45秒（`oj-resource-bounds-sandbox-regression.log`）；显式opt-in的其他专项本轮不重复运行。治理64项 **9.651秒**通过（`oj-resource-bounds-governance-final.log`），首轮默认沙箱禁止回环socket而失败，按正常测试权限重跑，未改权限逻辑或降低断言。README/CI/M0归属同步，普通CI不执行合成资源探针；完整M0、最小HTTP/结果页面及生产准入仍待验，未部署。

## 2026-11-03：M0 当前证据核对与联合复验

成功 fetch 后从 `origin/main 2b44a50` 建立独立 `codex/m0-evidence-audit` worktree。本次唯一目标为把现行 M0 条件映射到当前 owning tests/真实复验及剩余工作；只改 M0 记录和本日志，无运行时代码、协议、SQL、迁移、依赖或预算变化，不删除原需求。Astra 按当前代码复核矩阵与边界，无阻断；AI 审查不冒充独立人工审批。

`python3 scripts/test-worker.py all` 退出0，七个独立子进程均 PASS（`/private/tmp/oj-m0-audit-worker-all.log`），共74个 PASS 条目（含父条目）。基础选择先跳过6个显式开关专项，随后六项各自实际执行并通过，不把这些初始 SKIP 计作通过或未跑专项。基础包含受限执行/宿主/停止及安全解析/中继；真实回报15.18秒、双 Worker43.96秒、材料/身份故障91.11秒、槽恢复45.77秒、网络故障53.45秒、编译失权43.05秒。编译失权两模式的全部停止/PID证据为9.661712708/6.709786209秒，仍在原14秒上限内；未放宽任何预算。专项使用同机两个真实控制端和专用离线执行槽，部分 Worker 先后启动，不冒充跨主机或每项同时并发。

独立会话专项使用现有 `oj-slot-recovery-offline` 镜像及本任务缓存，`python3 scripts/test-session.py --toolchain-image oj-slot-recovery-offline --module-cache /private/tmp/oj-m0-audit-module-cache` 退出0：格式、模块校验、vet、真实 Redis/临时 PostgreSQL/race **109项及279子项，71.743秒**通过（`/private/tmp/oj-m0-audit-session-final.log`）。3项顶层SKIP分别为成本准备、成本测量和子进程helper入口；独立API进程父测试实际通过，本轮没有重跑Store成本基准。真实旧RDB恢复握手完成，两个临时Redis及私有网络由唯一命名驱动清理；不是生产全库/PITR或账号安全事实恢复验收。

首轮旧宿主缓存缺少SCS，未进入行为测试；镜像已有锁定模块，导出当前go.mod声明的11个模块和161个已有元数据文件后成功。首次全依赖图查询还要求未缓存的旧依赖，改按实际声明清单与既有元数据导出；没有联网下载、升级、改锁文件或禁用校验。失败日志保留为 `oj-m0-audit-session.log`、`oj-m0-audit-cache-export.log`、`oj-m0-audit-session-verified.log`，不计作通过。

正式普通会话协议另以既有离线镜像执行 `python3 -B scripts/test-session-protocol.py --toolchain-image oj-slot-recovery-offline --build-cache-volume oj-slot-recovery-go-cache`：模块校验、集成静态分析、真实Redis/临时PostgreSQL/race **187个PASS条目（含父条目），33.183秒**，零SKIP；退出、数据库排空和驱动owner清理均成功（`/private/tmp/oj-m0-audit-protocol.log`）。不以测试专用SCS模块替代当前正式协议，也不以正式协议通过替代独立旧RDB恢复证据。

P1 在同一离线工具镜像、`--network none`、2CPU/2GiB容器内只读挂载当前worktree，选择性复制源码到容器临时目录后执行模块校验和 `sh scripts/toolchain.sh db`，避免Linux构建覆盖仍用于宿主专项的Darwin二进制。五命令构建、真实临时PostgreSQL/race **159个PASS条目（含父条目），9.615秒**、双真实API进程/24并发仅一个提交、重启回执及v3/历史v1兼容均通过（`/private/tmp/oj-m0-audit-database.log`），零SKIP。临时数据库关闭，原有五个开发服务保持healthy，两个专用执行槽均stopped/exit137/OOM=false/restarting=false；最后失权停止使用KILL，不能把137解释成数据库或驱动失败。

核对发现两个不能靠既有绿灯补写为通过的缺口：进程数与临时磁盘/inode耗尽缺少行为测试；示例页面提交禁用，现有 HTTP queued 专项与直接 Store 接收/查询的真实 Worker 专项不是同一次 HTTP→Worker→HTTP 终态/最小页面。M0 记录列明后续验收顺序。计划的额度/公平调度仍是 P1 试用子项，完整 P5、生产容量/独立备份域及部署仍按原计划节点处理；不擅自扩大或缩小 M0。用户要求先补齐 M0、题目来源未知，P4 录入入口保持未决。

仓库策略449文件、23个新增Markdown文件/锚点引用、空白与完整差异复核通过；最新PR head的CI/合并/知识事务结果依交付回读。未重跑前端、Store成本基准或完整root后端检查，不将这些写作本轮通过。完整M0仍待后续缺口与最终验收，未部署。

## 2026-11-03：编译在途续租失权的真实停止证据

成功 fetch 后从 `origin/main 7609202` 建立独立 `codex/compile-lease-faults` worktree。本 PR 唯一目标为 P2、AC28/29 的编译在途失权中断，包含控制链路未知与节点禁用两种既有失权输入。仅在专用合成容器内选择唯一 cc1plus，使用容器 PID 发送 STOP，并从 Docker top 独立核对唯一编译器的 VM PID/T状态。短暂停留确认 Worker/同编译器仍存活，再阻断原端到端 mTLS 字节或禁用本轮临时节点；不发送 CONT、不取消/强杀目标 Worker，不借测试清理制造停止。

宿主单调 t0 在目标 Worker 启动之前，固定要求自发非零退出、容器停止和独立 VM PID 消失证据均在 t0+14秒之前完成。编译只能晚于 t0 开始，因此排除原15秒 compile wall、20秒curl等自然上限；原5秒续租、3秒请求、停止/材料预算全部保持。该界限是证据条件，机器太慢应失败，不能放宽作通过或当生产 SLA。真实字节受阻后还观察同编译器 T；结合既有续租实现支持失权主动停止判断，没有解密请求或新增逐请求renew审计。

最终 `python3 scripts/test-worker.py compile-lease` **44.26 秒**通过（`/private/tmp/oj-compile-lease-sandbox-verified.log`）：network **34.01秒**，全部停止/PID证据在 **10.231530459秒**取得；disabled **9.98秒**，停止证据在 **6.804760875秒**取得，均早于14秒。原尝试无结果，预先排队的下一题零attempt；另一节点经第二真实控制端完成第2次尝试 AC，两代次仍1、旧attempt expired，两个控制端拒绝旧令牌/禁用身份且新载荷不变。network由自然租约到期回收，disabled可由撤销提前回收，未编辑时间戳或手调Recover。两个专用槽最终回读停止/无PID/未重启，临时数据库无残留。

完整离线Docker backend格式、模块校验、vet、四检测器正负夹具、主模块/sandbox/独立会话模块静态各零、全部root race及五命令构建通过（`/private/tmp/oj-compile-lease-backend.log`）。治理 **64项，12.705秒**和449文件策略/链接/空白通过。Astra指出 sh -ec 的选定后失败可能错误返回1而被当无候选重试，Docker CLI本身也可返回1；最终零候选正常exit0并返回固定no-compiler标记，只有成功命令+明确标记可轮询，所有非零退出直接失败。再次读取/名称校验/kill/printf失败显式exit2。新增16个进程表/暂停回执解析子例覆盖CLI失败即使附带标记/PID也不可轮询，不调用Docker/发信号；最终安全包race **1.091秒**、默认及sandbox静态各零（`oj-compile-lease-boundary-final.log`）及上述真实双模式通过。先前44.20/44.07秒的正向运行仍保留日志，不替代最终分类候选。

README、CI隔离说明及M0记录同步；无业务代码、协议、依赖、迁移、生产 SQL、材料版本或预算变化，回退新增测试/驱动入口即可。Astra设计、完整差异、PID/时序/失败分类复核不冒充人工审批，最终PR/CI/合并及知识事务依回读。这是受控暂停编译验证，不是长源码性能基准；跨主机/断电、生产容量、自动执行端恢复与完整M0仍待验，未部署。

## 2026-11-03：Worker 真实控制链路网络故障验收

成功 fetch 后从 `origin/main d8a7d6c` 建立独立 `codex/worker-network-faults` worktree，唯一目标为 P2、AC28/29 的网络故障证据。增加透明回环 TCP 中继，只转发原始字节、不持有 TLS 密钥、不解析/替换 HTTP；按方向阻断并记录有界缓冲中的字节，恢复前关闭旧连接。模式检查和有界写入共用屏障锁，生命周期锁独立，取消/关闭等待 Accept、Dial 及双向复制结束。无业务代码、依赖、协议、迁移、生产 SQL、材料版本或预算变化。操作与回退范围见 [README](../README.md#worker-控制链路网络故障专项)；撤回新增测试及驱动选项即可，无数据回退。

`python3 scripts/test-worker.py network-fault` 在 Docker Desktop/LinuxKit 7.0.14、固定 GNU++17/go-judge 材料、临时 PostgreSQL、两个真实控制端进程及先后运行的 Worker 下通过，**54.58 秒**（`/private/tmp/oj-worker-network-sandbox-final.log`）。双向阻断 **36.05 秒**：实际观察 main PID 后阻断，原 Worker 非零退出、沙箱/PID 停止，无结果且下一题仍 queued/零 attempt；第二节点经独立控制端在租约自然到期后完成第 2 次尝试 AC，原尝试 expired，判题/恢复代次仍 1，两个控制端拒绝旧令牌且新结果不变。上行阻断观察为3332字节，不能据此识别请求或报告调用次数；若执行跨过续租周期，也可能先因失权停止，本项不把退出事实等同于特定错误分支。

下行确认丢失 **18.25 秒**：同一 Worker 先完成预热，再执行目标；短测试事务锁定未提交尝试，启用 gate 后立即回滚，真实控制端此后提交 AC。下行观察3821字节被阻断，Worker失败停止、不领预先排队的下一题；新连接及显式重启/复验后继续 AC。捕获首个已提交 payload/hash，在恢复及两个控制端的相同结果确认/冲突拒绝后逐字节不变，目标始终仅1 attempt。测试协调只使用临时行锁，没有编辑时间戳、修改结果、手调 Recover 或伪造 HTTP 响应。最终两个固定槽回读停止/无 PID/未重启，临时数据库无残留。

完整离线 Docker 后端格式、vet、四检测器正负夹具、主模块/sandbox/独立会话模块静态各零、全部 root race 与五命令构建通过（`/private/tmp/oj-worker-network-backend-final.log`）；新增纯回环中继包 race **1.086 秒**。这些安全测试不带 sandbox 标签，现纳入普通 backend；真实 Worker 测试继续显式 opt-in，普通 CI 不执行特权程序。最后的提交前屏障和夹具键增量已通过 sandbox 静态零问题（`oj-worker-network-static-final.log`）及上述真实专项。治理 **64 项，10.551 秒**、仓库策略447文件/链接/空白通过。

首轮完整后端在默认构建下发现 endpoint 方法仅被 sandbox 调用而 unused，改为纯回环测试也通过同一端点接口连接，保留原检查。首个真实驱动在初始化即因夹具幂等键少于16字符被拒，尚未进入网络故障；按既有合同补前缀后通过，未改业务校验。Astra 设计/完整实现复核指出仅观测 PID 再启用 gate 有误判窗口，已用真实行锁把 gate 排在报告提交之前并复核释放逻辑；AI 复核不冒充人工审批。最终完整差异/文档及远端 PR、CI、合并、知识事务结果依回读。本机 TCP 故障注入不替代跨主机/路由器分区、长编译续租、自动执行端恢复、生产容量或完整 M0，未部署。

## 2026-11-03：Worker 异常槽受控解封

成功 fetch 后从 `origin/main 16deb73` 建立独立 `codex/worker-slot-recovery` worktree，唯一目标为 P2、AC28/29 的显式恢复子项。新增 `worker recover <original-full-container-id>`：不加载节点/控制端凭据，共用规范锁，先持久隔离；固定槽名匹配可信历史原 ID 后绑定 ID/启动时间及固定隔离配置。运行容器复用原严格停止/PID 消失检查，已停止容器单独双次回读；成功才清标记，不领取、不回报、不重启。普通启动/Stop 规则保持，清空后的 Sync 失败尽力重建隔离并始终报告失败。无依赖、协议、SQL、迁移或材料版本变化；操作及回退见 [README](../README.md#worker-异常后的受控解封)。

Docker Go 1.26.10 / golangci-lint 2.15.0 完整后端格式、模块校验、vet、四检测器正负夹具、主模块/sandbox 静态/嵌套会话模块静态各零问题、全部 root race 及五命令构建通过，日志 `/private/tmp/oj-slot-recovery-backend-final.log`。恢复覆盖参数/配置/ID/生命周期、互斥、inspect/top/kill/PID/取消、标记写入/清空错误及真实锁文件清空后 Sync 注入失败的隔离重建。64 项治理通过（11.158 秒），仓库策略/空白/链接及完整差异核对通过；最终文件数按交付前回读。

真实 `python3 scripts/test-worker.py slot-recovery` 通过（46.79 秒，`/private/tmp/oj-slot-recovery-sandbox.log`）：两控制端/两 Worker 与临时 PostgreSQL、固定离线 GNU++17 沙箱。运行选手时强杀 Worker，普通启动保留隔离；活跃 owner/错误 ID 拒绝，无控制端凭据解封确认停止，已停重复恢复保持停止。控制端自然回收原租约，另一 Worker 第 2 次尝试 AC；旧令牌迟报拒绝且结果字节不变，原槽显式重启并复验后再次 AC。顺序重跑 `report` 全部 11 子例通过（15.34 秒，`oj-slot-recovery-report-final.log`），包含真实 AC/WA/CE/RE/TLE/MLE/OLE、丢回执、普通启停/排空；既有 `dual-worker` SIGTERM 接管通过（44.22 秒，`oj-slot-recovery-dual.log`）。两个固定槽回读停止/无 PID/未重启，临时测试数据库无残留；日志均在 `/private/tmp`。

环境诊断：首次完整工具镜像构建因 proxy.golang.org 连接拒绝失败；旧质量镜像缺当前 cpuid 缓存，改为组合已成功构建且校验的当前依赖层与原固定质量工具，离线完整验证，未升级/关闭检查。首次真实沙箱启动在旧停止容器遇到 cgroup.subtree_control busy，未进入恢复用例；按精确 ID/owner/停止/无挂载条件仅重建两个专用临时槽，同配置复验成功。并行完整 backend 构建共写 `.local/bin` 导致初次 report 的 worker-drain 子例 exec format error，前十项通过仍不算整套成功；重新导出 macOS 产物后改顺序复验。治理初次受沙箱回环端口/只读 Docker 限制，使用允许条件重跑原 64 项通过，未放宽断言。

最终边界复核发现旧 Worker 的宿主 Docker CLI 不会随 SIGKILL 自动排空，旧 Go 进程计时器不再取消它们，新的 Host.active 也没有旧请求，docker exec 不携带生命周期 fence。因此恢复只确认当前停止，调用前操作者必须独立确认旧宿主 CLI/在途控制操作已排空；无法确认保持隔离，不以等待25秒或当前停止代替该前提。API 注释、成功提示及规范已同步。真实专项增加 SIGSTOP 后按精确 PID 回读 T 屏障、非空后代记录与逐一退出确认，最终 **46.41 秒**通过（`/private/tmp/oj-slot-recovery-sandbox-final.log`，实际观察 1 个宿主后代退出）；不是人为延迟孤儿 CLI 或普遍跨进程派发隔离的验收。补充后完整 backend 再次通过（`oj-slot-recovery-backend-drained.log`），最终 sandbox 静态零问题（`oj-slot-recovery-sandbox-static.log`）。

Astra 针对性设计、完整代码、Sync 与宿主子进程边界增量/文档复核无阻断；这是 AI 复核，不冒充人工批准。该命令依赖操作者独占且已排空的可信 Docker 控制面、已知历史原 ID；标记不记录原 ID，不重建历史 PID，不保证磁盘拒绝重写后的持久隔离。自动重启/解封、持久结果队列、网络分区/跨主机/断电和完整 M0 仍待验，未部署。PR/CI/合并与知识事务状态依最终回读。

## 2026-11-03：Testlib SPJ 纳入完整训练必交付范围

用户依据[洛谷 Special Judge 文档](https://help.luogu.com.cn/manual/luogu/problem/special-judge)要求支持 SPJ 库。成功 fetch 后从 `origin/main 8c6f612` 建立独立 `codex/testlib-spj-plan` worktree；本轮只修改计划及相关规范，不实施检查器。Testlib 从“优先验证”明确为 1B/M2 必交付，P7 拆为库/构建准入、三文件调用与结果适配、版本/受限执行、端到端验收四项；P2 文本闭环与 M1 顺序保持。

只读核对洛谷文档及其引用的官方固定头文件，首个待验证基线为 Testlib 0.9.45、提交 `2d20123984e9479b8a56ebe0d6a51e23ad7c35b3`。保留 GNU++17 与 ICPC 外部边界，明确 `checker input output answer`、`inf`/`ouf`/`ans`、正常结果与异常映射及部分分拒绝；版本/哈希/宏配置、隔离预算、材料保密、发布阻断、重判/多节点/断外网及真实提交验收随实现交付。需求、总体架构、判题、题包、工具链说明、ADR 与 CI 验收目录同步，不承诺完整洛谷题包导入或交互/Hack。

实际验证：`python3 scripts/repo_policy.py repository` 439 文件通过，`git diff --check` 通过，已核对完整目标分支差异和本地 Markdown 链接。Astra 针对性协议/故障/拆分复核无阻断，采纳正式准入版本措辞与宏退出码防碰撞要求；AI 复核不冒充人工审批。纯文档未运行应用测试、安装依赖、编译或执行 checker，不代表 SPJ、完整 M0 或部署完成；PR/CI/合并及知识事务结果依最终回读。

## 2026-11-02：本地 HTTPS、持久账号与一键测试入口

成功 fetch 后从 `origin/main 051f575` 建立独立 `codex/local-login` worktree。用户要求本地实际登录与一键启动，并明确选择“仅本地环境显示的一键进入测试账号”，继续使用真实会话。新增独立 `compose.dev.yaml`、`cmd/localdev`/`internal/localdev` 及 `scripts/dev-up.py`；固定 `https://localhost:8443/login`、回环发布，数据库和 Redis 仅私网，应用非 root、状态卷私有。生产 API 不导入 localdev，默认 HTTP Compose 保持；裸 Docker build 仍以原静态 web 为默认目标。没有新增依赖或迁移，初始化使用已有 schema 5 和身份/恢复/限流原语；合同归[本地开发指南](local-development.md)。

fresh 初始化只在三方均空时生成随机账号/密钥/本地 CA，迁移、账号初始化、首次恢复开门和 marker 完成并通过真实 runtime 检查后，fsync/rename 就绪记录。已有状态仅检查，不恢复丢失 marker、不重设密码或轮换密钥。证书私钥、登录 key 和账号为 0600 私有卷文件，CA 只导出公开副本，不修改宿主系统信任。快捷入口只克隆实际 TLS 请求并替换为标准 Begin 及私有 seed，保留 Host/来源/Cookie；浏览器复用原 Complete→Read 单飞和未知结果恢复，不裸发会话或延长期限。改密后旧 seed 正常拒绝；正常密码登录保持。

完整 Docker 后端格式/vet、四检测器正负夹具、三范围静态各 0、全部 root race 和五命令构建通过（`/private/tmp/oj-local-dev-backend-delivery.log`）；新增 localdev **8 项**真实 TLS/材料权限/拒绝/关闭测试，包 race **1.084 秒**。前端 lint（7配置自测）/类型/**56 项 Vitest**/生产构建通过（`oj-local-dev-frontend.log`），增加快捷链、未知不重试、原 pending 恢复断言。治理 **64 项，17.308 秒**和仓库策略/链接/空白通过；具体最终文件数按交付前 policy 回读，不把检查器的预期负例输出当作产品失败。

已有 Playwright1.64.1/Chromium151.0.7922.34 对本次真实持久开发环境的**六组**检查全部通过（`/private/tmp/oj-local-browser-evidence/result.json`、`/private/tmp/oj-local-browser-final.log`）：一键实际发放 Secure/HttpOnly Cookie、无 JS 存储；普通密码登录及确认退出；重复完整 dev-up 保持材料哈希和真实浏览器会话；Complete503 注入后刷新、快捷入口 pending409、原 Complete 恢复；错误来源/Host/方法/客户端输入拒绝且无凭据返回；真实改密后旧 seed 拒绝、新密码表单登录、显式改回后快捷入口恢复。改密测试已把随机初始化凭据恢复，未把固定合成新密码留作用户凭据。浏览器仅测试 context 忽略自签证书；用户系统信任未改变。

独立 UUID 容器专项 `python3 scripts/test-local-development.py` 通过 fresh/repeated 材料一致、仅自有测试 Redis FLUSHDB 后初始化非零/DBSIZE仍0/文件哈希不变及归属核验清理（`/private/tmp/oj-local-dev-isolation-final.log`）；不启动网关、不占8443、不触碰持久用户项目，已接入 CI containers，原门禁保留且 job 超时30分钟。宿主脚本另用公开 CA 严格验证入口，无 skip-verify 后才报告成功。Astra 设计、代码、关闭修复及网络/最终完整差异复核无剩余阻断；AI不冒充人工批准，CI/PR/合并状态依实际远端回读。

实际修正与边界：第一次启动被用户中断在镜像拉取前，未创建状态；恢复原命令后 fresh init 成功。本机 Docker Desktop 的 internal-only 网关实测未获 HostPort，增加独立入口 bridge，数据服务仍私网，并补精确回环端口和宿主 HTTPS 检查；不声称完全断网。Python严格TLS暴露初版证书缺 AKI，改用已编码再解析的 CA 作为签发者并补回归；自有初始证书已先备份再更新，账号/login-key哈希不变、PG/Redis未改，记录 `oj-local-cert-repair-2.log`。初次只读挂载质量扫描加载失败，改隔离可写白名单源码副本后完整原检查通过，未关闭规则。专项把一次性卷准备与长服务 --wait 分开后通过。服务关闭改为同一10秒预算并行 HTTP/runtime 排空，错误不能记成功；停止宽限15秒。证书期限、私网无认证/本地KDF与小型测试字典仅为开发配置，生产证书/阈值/恢复连续性/字典覆盖、注册/邮件、训练/判题和完整M0仍待验，未部署。

## 2026-11-01：简洁桌面首页、登录与做题预览替换

用户确认以专业刷题工具为方向、桌面 Web 优先，依次接受简洁题库、独立登录与左右分栏做题稿后明确要求替换。成功 fetch 的 `origin/main 11fe7ef` 为独立 `codex/homepage-redesign` worktree 起点。唯一目标是替换这组界面和必要导航；没有增加生产题库、判题/提交、注册、持久草稿、迁移或部署。当前行为归 [Web 界面](frontend-design.md)，颜色/控件/动效归根 [设计约定](../DESIGN.md)。参考 shadcn/ui 与 Radix，沿用原生 CSS，不引入组件/动画库；新增已选 Router 与路由懒加载 CodeMirror，精确 MIT 版本见锁文件，Docker Node 24.19.0 满足 Router 22.22 下限。

账号 controller 提升到应用生命周期，原服务协议和密码表单处理保持；站内导航不会清除改密未知标记。示例题目独立命名空间，编辑器在身份核对完成后才挂载；临时代码只存内存，按观察身份隔离，离开/刷新提示下载。上传用独立读取序号、编辑 revision 和卸载守卫，拒绝迟到成功/异常；原生文件输入退出 Tab 顺序，语言→上传→下载→代码均可键盘到达。没有把示例内存暂存称为 AC21 IndexedDB 草稿，也没有把禁用提交声称为真实判题。

最终 Docker 前端 lint（7 项配置自测）/类型/**53 项 Vitest**/构建通过，`pnpm audit --audit-level low` 无已知漏洞；主包约 290.83 kB、编辑器懒加载包约 429.44 kB（gzip 92.78/141.25 kB），不等于性能或容量验收。已有 Playwright 1.64.1/Chromium 151.0.7922.34 的 **12 组**浏览器检查通过：保留原真实 HTTPS/race API/临时 PG/Redis/pinned mTLS 账号路径，新增跨页改密未知保护、初始身份门、内存保留、卸载后读取、两文件正反完成顺序/旧异常、UTF-8/Unicode/CRLF 下载、无效文本拒绝、全局 beforeunload、分栏键盘/可见焦点、搜索筛选分页、减少动画和示例零 POST。fixture race **18.843 秒**、fixture_exit0、browser/container/Redis/owner 清理分别成功，证据 `/private/tmp/oj-homepage-browser-qa-4`。1440×1000 三页及 1280×900 做题尺寸、旧账号锚点跳转另验通过，稳定截图在 `/private/tmp/oj-homepage-final-captures`；390px 仅保留已有账号基本回归，不声称新的移动端设计验收。

补充最终候选：保留 UTF-8 BOM，原样测试和完整 12 组浏览器专项再次通过，fixture **18.515 秒**且全部结果/退出/清理为真（`/private/tmp/oj-homepage-browser-final`）；前端完整检查和 audit 见 `/private/tmp/oj-homepage-frontend-final.log`。占位文字由 4.16:1 调整为与正文辅助色一致的 4.97:1，代码行号和焦点也使用更清晰的已有 token；最终源码 lint/类型/53测试/构建见 `oj-homepage-frontend-contrast.log`。设计原稿、测量与未通过的像素门禁证据归档 `/private/tmp/oj-homepage-design-evidence`，大图不进入源码 Git。

治理 **64 项，10.428 秒**、独立 Compose 静态资源/SPA/API合同/权限/重建/启停检查通过。早期浏览器首轮因未挂既有模块缓存而 `GOPROXY=off` 失败，改用 `/private/tmp/oj-session-mod` 并保留校验；后两轮预览测试超时定位为 `getByLabel` 精确匹配包含 option 文本的 label，改用实际可访问角色/名称，并未削弱筛选断言。Astra 指出并已修复页面生命周期、迟到上传/读取顺序、不可见焦点、当前栏目和 17px 高度预算问题；复验后 AI 代码/视觉审查给出 ship，不冒充人工批准。设计像素工具总体约 88% match，但区域裁剪对位误报表格文字缺失，自动 hero 门禁仍未通过，不宣称像素级复刻或自动全阶段完成；人工看图与功能专项分开记录。最终 PR/CI/合并状态依远端回读，默认 HTTP/bootstrap、生产 HTTPS/字典/预算/正式训练及完整 M0 仍待验，未部署。

## 2026-11-01：浏览器账号登录、恢复与会话管理

前置 PR #77 最终 head `1de02f6` 经 CI `41314309363` 九项成功后 squash 为 `76ec453`；精确 head/合并树与祖先核验、主线快进及定向清理完成，知识事务 `oj-password-change-delivered-20261101` 五页字节回读一致/lint 0。重新成功 fetch 后从该主线建立独立 `codex/browser-account-lifecycle` worktree，唯一目标为已有账号的浏览器生命周期，不涉及注册/邮件、角色/资源权限、训练或部署。

本轮等待 CI 时，用户回复接受推荐密码规则：至少 15 个 Unicode code point、允许空格、无强制组合、拒绝常见弱密码，并保持原始字节不 trim/归一化/截断。实现及现有测试原已遵循这套规则，本次仅同步需求、设计、README、浏览器说明与当前验收记录，保留 PR #77 当时尚待确认的历史；生产字典覆盖/许可仍须验证。

沿用 React/fetch/CSS，不引入依赖或多页面框架。账号面板提供登录、原候选恢复、只读检查、显式续期、退出及可选改密；单飞覆盖 Begin→Complete→Read，本地代次隔离卸载/StrictMode 后的成功/错误/finally，密码不进入 React 状态或浏览器存储。请求在 HTTP 下先于序列化关闭，固定同源 URL/Cookie/no-store/无 referrer/禁止重定向，有限读取/严格操作响应/固定本地错误文案；不自动重试 POST。会话观察与操作回执分开，初访未登录为正常提示；完整合同归[浏览器账号](account-browser.md)，六会话路径补齐[OpenAPI](../api/session.openapi.yaml)。默认 Compose/Nginx/Vite 不启用会话 HTTPS，无依赖/SQL/迁移/生产参数变化，回退静态构建即可，已有账号和会话持久状态不撤销。

Astra 设计/代码复核要求 redirect:error 防止 307/308 重发密码、改密 401/404 不推导普通会话退出、Complete204/后续 Read 分别表述、单飞在首个 await 前占位，均已落实。复核发现 Abandon 可撤销原候选已发放的会话，已修正成功与未知 Abandon 的状态/文案并补回归；首次 Read403/405 不再停留“检查中”。改密未知标记只在当前 controller 实例保留，文档明确不能跨刷新/标签防止重提，表单始终提示先重新登录核验；未宣称控制浏览器密码管理器。

本机已有固定前端镜像中，首选旧质量镜像与当前安全修复锁文件不同，复制后触发 pnpm registry 重试但 network none 阻止联网；终止本轮自有容器，改用 package/lock 字节完全一致的已有镜像及原依赖路径，未下载或修改依赖。原基线 lint/类型/6 项测试/构建通过（`frontend-baseline-verified.log`）。首轮新增测试触发 mock any 的严格 lint，补真实 fetch 类型，不放宽规则。最终完整 Docker 前端 lint（含原检测器回归）/类型/**49 项 Vitest**/构建通过（`frontend-ready.log`），涵盖有限响应、无重试/跳转、操作状态、单飞与 cleanup/迟到回执。浏览器截图发现连接检查挤占主操作，已将账号面板提前并加页内入口，桌面/窄屏重新实测。

专用 `scripts/test-browser-account.py` 复用现有 owner 标签/镜像架构/容器状态检查，只用已有模块、浏览器及镜像，不 pull/install；构建 Docker 禁网，Node 只作宿主浏览器驱动。原 internal bridge 实测没有分配 published HostPort，改为独立带标签 bridge 并实际 inspect 只绑定 127.0.0.1 的随机两端口。临时 PG Unix socket、Redis 与真实 race API/pinned mTLS 保持隔离；双标签 Go 夹具只由精确 TestBrowserFixture 启动，不混入普通 CI。控制端点无认证，安全依赖该临时隔离，不能作为生产或通用代理。

最终已有 Playwright **1.64.1** / Chromium **151.0.7922.34**，**1280×960 / 390×844 十组浏览器断言**全过（`/private/tmp/oj-browser-account-final-qa`）：真实安全 Cookie/原候选刷新恢复/放弃、手动检查/活动/退出、改密拒绝与换代重登、确认密码仅本地、重复提交单飞、307 sink 零请求及无后续 POST、畸形202零确认、发放204后 Read503不重发 Complete、真实改密204后替换503且原始 Unicode/空格重新登录、HTTP零账号请求、本机快进3小时到期提醒零后台请求。无未捕获应用异常，14 个 HTTP 拒绝/注入网络诊断为预期；截图回读无横向溢出。最终 fixture race **9.723 秒**，browser/fixture/container/Redis/cleanup 各自为真、fixture_exit0，正常排空及资源清理通过，不把 mock 错误替代正常存储路径。

Astra 指出 route 异步回调异常不一定由外层捕获，已统一 safeRoute 固定失败标记/安全 abort，避免原 Playwright 网络诊断回显 Cookie。独立仓库外副本在 route.fetch 前抛带合成敏感哨兵的异常，实际 runner 非零、browser_ok=false/routeFailures1，所有日志无哨兵，同时 fixture_exit0、连接/容器/Redis/owner 清理成功（`route-negative-evidence`）；不把预期负例记为浏览器通过。缺失模块、既有输出目录和仓库内产物三类预检均在资源创建前拒绝。初始临时 driver 导入产生的唯一自有 bytecode 已删除，新驱动导入前禁止写 bytecode；初次治理另见受限回环 socket PermissionError，按工具权限重跑原 **64 项，10.047 秒全过**。未关闭 hooks、规则或断言。

两个 OpenAPI 用既有缓存 MIT YAML 库严格解析并核对路径/方法/响应形状/Cookie 与 **108＋10** 个本地引用（`openapi-verified.log`），不是完整 OpenAPI 语义验证；首次缓存合并因 root 丢弃能力后不能写 postgres 所有路径失败，改用镜像原非 root 用户，未放宽容器能力。403 文件策略/Markdown链接/空白和完整差异交付前核对；上述短日志均以前缀 `/private/tmp/oj-browser-account-` 定位。Astra 设计、产品与测试驱动针对性复核，AI 不代替人工审批。其它浏览器/关闭重开/多标签完整竞态、设备管理/保持登录、生产参数/证书/存储TLS/字典、注册/找回/权限/训练和完整 M0 仍待验，未部署；最终远端与知识回写依 PR 回读记录。

## 2026-11-01：已登录用户的原子改密与旧会话失效

前置 PR #76 已 squash 为 `5b9f541` 并完成工作区/知识回读；成功 fetch 后从该主线建立独立 `codex/authenticated-password-change` worktree。唯一目标为可选已登录改密的完整命令/API 路径，不涉及 SMTP/注册、管理权限或部署。新增身份用例/专用 SQL 与 sqlc 生成、独立 Redis 三维额度/规则指纹、同 owner/KDF capability、严格 HTTP 输入及可选配置；原 schema 5、密码格式、会话协议、旧配置和 login marker 保持，无新依赖/索引/迁移。接口和边界归[会话设计第 7.18 节](architecture/session-design.md#718-已登录用户修改密码)及[账号 OpenAPI](../api/account.openapi.yaml)。

Astra 设计复核要求不复用带 Legacy 预升级的验证用例、不把原 session fence 换成新状态、不升级账号共享锁为写锁、专用 UPDATE 额外要求 enabled、独立 quota/marker 及规则指纹不进 bucket key，均已落实。当时最低强度经异步询问尚无用户答复，采用已告知的 NIST 15 字符工程初值并要求显式配置，可提高，未记为用户确认/整体 NIST 合规；用户后续确认见本页浏览器交付记录。规则机制仅做原始 UTF-8 code point/字节预算与整串摘要精确查表，未下载字典，生产覆盖/许可/配置另验。

首轮身份/密码/PG unit race 通过，Redis 编译因共享 EVAL 提取漏 time import 失败；补回后各包 race 通过，静态指出字面 nil context 的测试写法，沿用已有表驱动上下文负例而未跳过检查。真实专项先发现故障 hook 签名和测试分支 QF1003，按既有 hook/显式 switch 修正；核心真实事务/额度专项 PASS（`protocol-core-verified.log`，27.403 秒）。随后完整接线首轮格式遗漏了新 owner 文件，已按 Git 完整新增/修改列表定向 gofmt；静态还指出新进程测试分支表达。接线编译暴露 apiapp 既有函数 password 与新包导入同名，改为显式 alias；原失败与二进制未构建结果保留（`backend.log`、`backend-fixed.log`、`protocol-http*.log`）。未关闭 lint、race、hooks 或放宽断言。

完整离线 Docker Desktop/Linux arm64 backend 最终通过格式/vet、检测器正负夹具、三范围实际静态 0、全部 race 和四命令构建（`backend-verified.log`；apiapp 1.052s、apiconfig 1.091s、HTTP 1.034s、redisstore 1.021s、runtime 1.031s）。Astra 发现当前密码超限被指认为新密码错误，改为 INVALID_PASSWORD_INPUT，并经“current 129 bytes/new 合法”的真实 HTTP 和 unit 回归核对。敏感对象合法格式/JSON 增量 unit/race 1.022s、包静态 0 及最终全树格式通过（`final-unit.log`）。

完整真实 Redis7.2.17/临时 PostgreSQL/race 首次接线后 **187 PASS（改密相关 34，HTTP 8，32.556s）**；补规则最大字节变化和运行中 marker 错配回归后最终再次 **187 PASS（含父条目，32.383s）**、集成静态 0、退出/OOM/自有清理通过（`protocol-http-verified.log`、`protocol-final.log`）。实证：Legacy 单次最终替换、双副本同 snapshot 仅一成功、原 AuthGeneration 不重标、disabled 当前匹配 revision 仍拒绝、验密 snapshot 后仅 CV 重哈希/恢复/禁用冲突；真实恢复共享锁与账号 UPDATE 串行、取消后精确 backend 退出、实际提交后注入丢回执恰一次、两修订溢出、实际 owning UPDATE 主键 EXPLAIN（不执行/不打印记录）。

真实 API 子进程证明双副本改密/清两 Cookie、旧会话和 pending 候选拒绝、旧密码拒绝/新密码重新登录；旧 Redis token 记录仍在，不依赖清理宣称撤销。弱/短/相同/错误/超长输入、CSRF/方法/缺 Cookie/自选主体拒绝不改凭据；缺失/错配规则或 marker 不监听且释放资源。实际 TLS→mTLS 代理的同来源跨副本限流、恶意转发/Connection 头覆盖、另一实际来源独立与成功改密通过。规则 min/max/词典差异构造及显式初始化失败关闭，已构造入口也逐请求检查 marker，不重建计数/刷新拒绝 TTL。

交付前再补有效 limited Login 前提下的 nil/零 capability、重复能力和跨 owner 拒绝，保持正确绑定的阳性构造；最终候选完整真实专项 **187 PASS，32.431s**、静态 0/退出/清理通过（`protocol-delivery.log`），不依靠无效 Login 使负例空泛通过。产品代码未变。

新增 OpenAPI 用已锁定 MIT YAML 库在隔离 Docker 严格解析，核对唯一路径/请求对象/Cookie/11 响应/全部本地引用（`openapi.log`）；不是完整 OpenAPI 语义或真实浏览器验收。389 文件 repository、Markdown 链接、空白/完整差异检查通过；Astra 最终产品/测试/文档复核无代码阻断，收窄了 rehash 在 snapshot 前后与提交后竞态的表述，AI 不代替人工审批。日志前缀 `/private/tmp/oj-password-change-`。生产字典/预算/证书/存储 TLS、浏览器、邮箱找回/注册、完整权限恢复及 AC01/23/24/25/M0 未完成，未部署；远端交付和知识回写依最终 PR 回读核验。

## 2026-11-01：显式配置的普通会话 API 进程入口

成功 fetch PR #75 主线 `847abe7` 后建立独立 `codex/session-api-entry` worktree。唯一目标是可运行的 opt-in 会话 API：严格版本化非秘密 JSON、固定密码环境、显式地址/模式互斥、TLS/mTLS 材料、同一 Runtime 的六路径及有界启动/退出。原登录密钥 fd 读取提取为 checkedfile，共用目录/所有者/单链接/快照检查；public 只放宽只读位，secret 仍仅 0400/0600。无依赖/SQL/迁移、自动初始化或默认 Compose 变化。合同、配置字段、回退和剩余边界归[会话设计第 7.17 节](architecture/session-design.md#717-显式会话-api-入口)及 README。

使用既有 Docker Desktop/Linux arm64 工具链与 checksum 缓存，全程不下载依赖、不挂载 socket/宿主端口、不访问业务数据。首轮相关 unit race 全通过，静态发现 error channel 接收未检查和测试 error 变量命名；修正后相关六包 race、静态 0（`unit-fixed.log`）。完整后端格式/vet/检测器正负夹具/三实际静态 0/全部 race/四入口构建退出 0（`backend.log`，apiapp 1.038 秒、apiconfig 1.060 秒、redisstore 1.024 秒）；Darwin arm64 共享读取/原 loader 测试及 API 交叉编译通过（`darwin.log`），不冒充 Darwin 运行实测。

真实专项首轮静态发现两处新测试 deferred Close 未检查返回，修正后原有用例和新进程功能通过，仅 timeout 后 remote backend 的原 3 秒观察失败（`protocol.log`、`protocol-fixed.log`）。该用例显式设置 PG statement/lock_timeout 5 秒，API 50ms 非零退出终止 driver 取消机会，不能据 3 秒断言认定泄漏。Astra 核对 PG14 连接检测/期限文档后，仅 timeout case 改为从实际 waiter 观察起按 DB.Timeout 加固定 1 秒余量检查精确 `(pid,backend_start)`，始终持有 blocker；正常 drain/startup cancel 的原严格 gone 不变。中间一次主代理补丁误匹配作用域导致 typecheck，已修正（`protocol-verified.log`）。未改产品超时、退出码、原断言或关闭 race。

最终真实 Redis7.2.17/临时 PostgreSQL/race 专项 **153 个 PASS 条目（含父条目，真实进程 11，27.281 秒）**、集成静态 0、退出/OOM/自有清理通过（`protocol-final.log`）。真实子进程验证默认 bootstrap/session404、双 direct TLS 的 Cookie/Legacy/活动/退出/禁用撤销、无 marker 不修复和 bind 失败资源释放、启动 PG 锁等待 SIGTERM 不监听及精确 backend 清理、正常活动请求排空期间 PG/Redis 保持存活、异常退出 code1/无 graceful。异常退出后确实观测 `active/Lock`，远端约 **5.001 秒**消失，符合服务端预算；进程退出不冒充远端即时回滚。mTLS 真实进程拒绝无证书/错误 CA 握手、同 CA 未 pin 或无规范转发头的请求；经 TLS ReverseProxy 完成登录/读/退出并覆写伪转发头。child race 默认保留，任何非 JSON/race 诊断都使测试失败，预期失败只接受 code1。

PR #76 首个 head `fe8026a` 的 CI `41309906824` 因治理测试仍预期旧源码复制白名单而失败，后续应用检查按门禁跳过，未将 skip 视为通过。将脚本额外复制范围收窄至构建必需的 `cmd/api`，同步精确白名单并保留其余安全断言。本机受限沙箱禁止 doctor 测试的临时端口绑定，原两项失败记录于 `governance.log`；按工具权限运行未修改的完整 **64 项治理测试（9.928 秒）**通过（`governance-verified.log`）。收窄复制后完整真实专项再次 **153 PASS（进程 11，27.634 秒）**、静态 0/退出/自有清理通过（`protocol-harness.log`），产品代码不变。

日志统一前缀 `/private/tmp/oj-session-entry-`。主代理完整差异/新增文件核对、367 文件 repository、链接与空白检查通过；Astra 产品、测试/文档及超时观察增量复核，AI 不代替人工审批。公开 OpenAPI/页面、真实存储 TLS/生产证书及代理、生产 KDF/限流/多 API 总预算、完整账号权限/恢复与 AC01/23/24/25/M0 仍待验，未部署。知识写回与远端最终状态在 PR 交付回读后按维护流程核验。

## 2026-11-01：会话敏感值的合法格式与配置 JSON 脱敏

前置运行时 PR #74 最终 head `8ddd80a` 的 CI `41305405642` 九项成功，四必需门禁均来自最终运行，旧同 head 取消运行仅保留终态历史；完整diff/反馈/讨论及GitHub CLEAN核验后 squash 为 `369d38b`，主线快进/树一致与祖先核验、定向清理完成，知识事务 `oj-session-runtime-delivered-20261101` 五页字节回读一致/lint 0。重新成功 fetch 后从该主线建立独立 `codex/session-secret-redaction`，唯一目标为既有敏感类型输出保护，未混入默认 API 接线。

先前 Docker 纯合成探针只输出布尔结果，已证明 Credential 数值格式包含 token、LoginConfig 数值格式/JSON 包含可恢复 key 数组；当前六类型回归进一步复现失败，日志 `/private/tmp/oj-session-redaction-before.log`、`before-all.log` 只记录类型/格式及通用断言，不打印材料。现有 String/GoString 只保护部分动词，不能把未出现原始 base64 子串当作未泄漏 key 数组的证据。

新 `redaction.go` 给 Credential/Attempt/Continuation/LoginSecrets/LoginConfig/Login 增加值 Formatter，直接值/指针16组合法格式固定既有标签，nil保持标准输出；LoginConfig增加值MarshalJSON，直接/标准容器输出固定字符串，其他opaque JSON仍为{}。全仓核对未发现LoginConfig JSON业务消费者，私有key文件和密封payload为独立schema；显式Value/Parse、实际AEAD恢复、candidate原tuple和key不变有回归。非法Go诊断/反射/重新包装/主动字段访问不提供保证，仍禁止秘密对象日志，合同归[会话设计](architecture/session-design.md#716-会话敏感对象的输出保护)。无依赖/SQL/迁移或行为授权扩展。

完整离线 Docker Desktop/Linux arm64、checksum缓存验证后，redisstore行为/race（1.022秒）、完整backend格式/vet/检测器正负夹具/三实际静态0/全部race（redisstore1.031秒）/四入口构建通过（`backend.log`）。真实 Redis7.2.17/临时PG/race完整专项 **142个PASS条目（含父条目，15.260秒）**、集成静态0、退出/OOM/自有资源清理通过（`protocol.log`）；上述短日志均以前缀`/private/tmp/oj-session-redaction-`定位。Astra设计与代码/测试复核无阻断，最终文档/差异/仓库策略交付前核验；AI不替代人工审批。默认命令/配置文件/TLS监听、真实存储TLS/浏览器/生产预算、完整权限恢复与AC01/23/24/25/M0仍待验，未部署。

## 2026-11-01：显式会话配置与资源生命周期装配

前置单跳代理 PR #73 的 final head `b23f0d2` 经 CI `41303412108` 九项成功后 squash 为 `08c48a8`；完成最终 head/合并树一致及祖先核验、主线快进和定向 worktree/分支清理，知识事务 `oj-session-proxy-delivered-20261101` 五页回读一致/lint 0。重新成功 fetch 该主线后建立独立 `codex/session-runtime`，唯一目标为 typed 配置的 Open/ServeHTTP/Close，不扩大到默认 API/文件解析/监听器。

装配一个 PG 身份权威/有界 pool、一个 Redis client、一个共享 KDF/准入及严格限流的 direct/proxy Handler；复用 owner 纯校验，固定普通期限，无裸客户端/Handler 导出。显式连接/执行预算、无任意 DSN/PG hooks，拒锁定 driver 读取的 ambient 配置并用官方常量模板后成组赋值/清 Fallbacks，防隐含凭据和明文降级；私网/VerifiedTLS 显式选择。启动只读 schema/open recovery/Redis条件/已有marker，不初始化/修复或迁移，失败释放自有资源；全程合作期限包含本地准备。关闭以同锁登记与停入口，实际 Handler/KDF 返回后恰一次清理，超时不提前关闭或重开。合同、配置、错误/格式化脱敏和回退归[会话设计](architecture/session-design.md#715-显式配置与运行时装配)。

首轮 Docker 测试暴露 PG 前缀拒绝过宽，容器仅回读名字确认 PGDATA/PG_MAJOR/PG_VERSION 是镜像元数据；未输出值。对照锁定 pgx v5.11.0 `pgconn.parseEnvSettings` 改为实际消费的 24 个连接键，增加元数据允许和恶意连接环境拒绝/保留原值的回归，未修改进程环境。首轮 unit/protocol 失败保留在 `/private/tmp/oj-session-runtime-backend.log`、`protocol.log`，未放宽断言或跳过检查。Astra 提醒整个 StartupTimeout 不能在文件读取后重置，已移至 Open 起点并在本地准备后/发布前复核 context；同步文件操作仍不提供硬中断。

完整离线 Docker Desktop/Linux arm64 backend：checksum 校验、格式/vet、检测器正负夹具、三实际静态范围各 0、全部 race（runtime 1.034 秒）与四入口构建通过（`backend-final.log`）。最终增加 PG VerifiedTLS 赋值/身份/RootCAs 克隆和无 fallback 直接断言后，仅重验全树格式、session 静态 0 和最终 runtime race **1.035 秒**通过（`unit-final.log`）。确定性阻塞 Handler 在请求取消后仍占活动名额，Close 超时/新请求 503/解除后 16 个等待者及恰一次 cleanup、64 请求与 8 Close 竞争、panic 后排空和零值拒绝均验证；未用随机长 KDF 模拟完成。

真实 Redis7.2.17/临时 PG/race 完整专项 **142 个 PASS 条目（含父条目，装配 11，14.933 秒）**及集成静态 0、退出/OOM/自有资源清理通过（`protocol-final.log`）。新增双 Runtime/共享 Cookie jar/Legacy 升级/活动/退出/关闭后拒绝、真实 TLS→mTLS 代理；缺失/不匹配 marker 快照不变、closed recovery 代次/状态不变、错 schema/Redis失败均拒绝并释放PG；Redis CLIENT LIST 前后计数相同，不关闭其他 owner。ACCESS EXCLUSIVE 表锁实际阻塞新恢复读取，观察唯一 waiter 后取消或等待 startup deadline，确认精确 backend 消失后再释放 blocker，且无 marker 变化。短日志名均以前缀 `/private/tmp/oj-session-runtime-` 定位。

PR #74 初始 head `86a2f1e` 等待 CI 期间，补充 `%d/%f` 负向探针实证 Stringer 被非常见动词绕过，输出断言失败但日志不输出合成凭据（`format-probe.log`）。为新 Config/DatabaseConfig 增加值接收者 Formatter，并对值/指针、16 种合法动词/flags 逐字断言固定脱敏文本；修复以正常追加提交交付，不沿用旧 head 绿灯。Astra 按锁定 fmt 源码指出非法 `%w` 诊断先于 Formatter 且可能反射原值，独立合成探针确实未保持固定脱敏（`format-invalid-probe.log`），故明确只保证合法受支持格式，禁止原配置日志/反射；该非法探针不充作支持保证。冻结修复后的完整 backend/全部静态/race/四入口重验通过（`backend-verified.log`，独立 runtime 1.032 秒、全后端 runtime 1.030 秒）；完整真实专项再次 **142 个 PASS 条目（装配 11，14.767 秒）**、静态 0/退出/自有清理通过（`protocol-verified.log`）。

主代理完整差异/新增文件与日志核对、345 文件策略/本地链接/空白检查通过；Astra 设计、核心、测试与文档复核按最终差异执行，AI 不替代人工审批。无依赖/SQL/迁移变化；默认命令/JSON配置/监听器、真实存储 TLS 握手、浏览器/生产代理、生产KDF/限流/多API总预算、账号权限/恢复与完整AC01/23/24/25/M0仍待验，未部署。

## 2026-11-01：单跳代理的会话信任与来源额度

成功 fetch 主线 `b51119f` 后建立独立 `codex/session-proxy` worktree，无开放 PR/未知改动。唯一目标为显式固定单跳代理入口，默认 API/生产配置另验。`NewProxiedLimitedLogin` 要求已有共享限流、复制的 1—16 个唯一非零叶证书 DER SHA-256；全部路由先核对真实 backend TLS、同叶 VerifiedChains、固定 public Host 和严格唯一 XFF/XFH/XFP，拒绝其他转发方言或多跳。来源局部显式传递，不修改 RemoteAddr/TLS 或 handler 共享状态。原直连入口行为保持，无依赖/SQL/迁移。合同与回退归[会话设计](architecture/session-design.md#714-单跳受信反向代理)。

基线旧镜像缺少锁定 cpuid 模块，禁网失败后合并本机既有 checksum 缓存，`go mod verify` 与原 HTTP race（1.028 秒）通过，未下载或变更锁文件。完整离线 Docker backend 通过格式/vet、检测器正负夹具、三实际静态范围各 0、全部 race（HTTP 1.035 秒）及四命令构建（`/private/tmp/oj-session-proxy-backend.log`）。随后仅测试/文档变更，最终 HTTP 静态 0、全树格式和 race 1.037 秒通过（`http-final.log`）。

首次真实协议专项原有用例及代理 Cookie 生命周期通过，两个新断言失败：`httptest.Server.Client()` 返回共享指针，后创建的 .3 来源/证书 transport 覆盖先前客户端。修正为复制 http.Client 后再 clone/config transport，生产实现不变，原失败保留（`protocol.log`）。Astra 发现 nil manager 使 pin 错误测试空泛，改用有效 limited manager 核验 empty/zero/duplicate/17 个不同非零 pin，并确认 16 个有效 pin 可构造、有效 unlimited manager 被拒绝；补跨副本 Complete/Abandon 共享恢复来源额度。未放宽检查或断言。

最终真实 Redis7.2.17/临时 PostgreSQL/race 专项 **131 个 PASS 条目（含父条目，新增代理 10，14.227 秒）**、集成静态 0、退出/OOM/自有资源清理通过（`protocol-final.log`）。短文件名均以前缀 `/private/tmp/oj-session-proxy-` 定位。两公开 TLS 代理经真实 RequireAndVerifyClientCert backend、同 CA 未列入 pin 拒绝和无证书握手失败有证据；两个真实 loopback socket 来源验证同源跨副本 Begin 限流、另一来源独立及恢复额度，认证入口探针确认 429 不到密码门禁。恶意 Forwarded/XFF/XFH/XFP/X-Real-IP/Connection 入站头经 Rewrite 删除覆写；共享 Go Cookie jar 登录/读/活动/CSRF/退出及撤销后拒绝通过。合成 TLS 状态单元测试另覆盖 nil/错链、映射地址和头边界，不冒充真实握手。

Astra 设计、完整实现及测试/文档增量复核无剩余阻断，主代理核对完整差异/新增文件和日志；337 文件 repository、diff 空白与本地链接检查通过，AI 不代替人工批准。按建议区分 Handler 的 403 与无证书 TLS 握手失败。默认 Gin/Compose/OpenAPI、真实浏览器、生产证书/反代/阈值/总预算、完整账号权限/恢复及 AC01/23/24/25/M0 仍待验收，未部署。受信代理自身可伪造来源，应用不独立证明公开 HTTPS；不提供多跳/CIDR/明文后端或自动证书轮换。

## 2026-11-01：共享登录请求额度与严格 HTTP 构造入口

前置私有密钥加载 PR #71（head `f8e2520`，CI `41277855976` 九项成功）已 squash 为 `135bf4e`，完成树内容一致/祖先核验、主工作区快进、对应 worktree/分支及四个自有构建产物清理，保留主克隆忽略资料。成功重新 fetch 后从该主线建立独立 `codex/login-rate-admission`。本轮同一 Login/Store 上提供显式五维预算、元数据初始化/只读构造和单次 Lua 联合准入，HTTP 严格入口要求配置存在，旧入口也自动执行已配置限流；默认 API 未挂载。合同、Redis key/TTL、配置不一致/恢复和边界归[会话设计](architecture/session-design.md#713-共享登录限流)。无新依赖/SQL/迁移，schema 5 与原会话记录不变。

复用锁定 go-redis/标准库 HMAC-SHA256，评估 redis_rate v10 单 key API 后用受限短 Lua 满足联合拒绝无写入；核对 Redis 官方 Lua flags/错误语义，不声称脚本错误回滚。显式 Initialize 不判断 namespace 空白或修复灾难，构造器只读且每次请求核验持久 marker，HMAC/配置不一致不可开新前缀避开全局桶。先全维校验再 429 或按全局优先 SET+PXAT，拒绝不建 key/续 TTL；恢复额度独立于 Begin 且 Complete/Abandon 共享。来源只用实际 socket IP，隐私 key 目的隔离，失败无 Cookie、退款或自动重试。

Astra 设计与核心复核要求处理 Redis 到期相等的物理存留，已以完整验证后的逻辑空桶实现；指出旧 session Cookie 需先廉价解析，已修复并补无 Redis 调用/计数不变的真实回归。初轮 internal race 通过（httptransport 1.078、redisstore 1.178 秒），静态曾报测试直接传 nil Context 的 SA1012，保持 nil/取消断言并改为表驱动，无屏蔽检查。第一轮真实专项 **119 个 PASS 条目（含父条目，限流 35，21.776 秒）**、集成静态 0 与退出/OOM/自有清理通过；日志 `/private/tmp/oj-login-rate-protocol.log`。后续补异构 Window、独立 recovery-source 额度与认证入口探针，再核验最终快照。

测试故障区分：双副本最后一额度、实际 TTL 过期、真实 Redis OOM 和 CLIENT PAUSE WRITE 使用真实服务；精确 `now==deadline`、首个 SET 后 runtime error、已准入回执丢失与 HTTP 预算后到期通过受控脚本/客户端 wrapper 注入，不声称自然网络或时钟故障。429/503 后认证 RecoveryState 入口计数不增，结合既有门禁调用顺序证实未进入密码流程；legacy credential_version 不变及零会话仅作补充结果证据。不同窗口全维拒绝、规范化邮箱/mapped IP、代理头忽略、HMAC 稳定封装轮换、损坏 metadata/counter 优先 503 均纳入最终回归。

最终验证：完整 Docker backend exit 0，格式/vet、检测器正负夹具、三实际静态范围各 0、全部 race（httptransport 1.196、redisstore 1.151 秒）与四入口构建通过（`/private/tmp/oj-login-rate-backend.log`）。补充异构窗口/认证入口探针后的专项121个PASS条目、16.750秒（`protocol-final.log`）通过；Astra随后发现异构夹具靠近秒/小时边界的时序风险，改为用真实Redis时间留足余量并严格确认初始三桶/不同截止，未改Lua或放松断言。冻结最终代码后完整专项 **121个PASS条目（含父条目，限流37，16.971秒）**、integration静态0、退出/OOM/自有清理再通过（`/private/tmp/oj-login-rate-protocol-verified.log`）。上述短日志均以 `/private/tmp/oj-login-rate-` 定位；333文件仓库策略、全部新文件/完整目标分支diff和Astra最后复核无剩余阻断。无SQL/治理变化，不重复它们的本地独立套件，远端必需检查保持。

固定窗口边界突发、校园 NAT/目标邮箱耗尽可用性、多 API 总 KDF/内存/连接预算与容量仍需实际参数/压测；不宣称防 DDoS、公平或完整防枚举。真实密钥分发/轮换、默认 Gin/Compose/公开 OpenAPI 接线、反代/浏览器及完整 AC01/23/24/25/M0 未验收，未部署。下个独立阶段再接入默认 API，不用挂载成功替代业务验收。

## 2026-11-01：登录生产密钥加载准备

成功 fetch `d989a56` 后建立独立 `codex/login-key-loading` worktree。新增私有文件加载与 `NewLoginFromSecrets`，复用原封装 key 校验及 Login owner；无默认 API 挂载或真实秘密生成/读取。Linux/Darwin 逐级 openat/O_NOFOLLOW/fstat、可信目录、0400/0600 单链接普通文件、4 KiB 上限与读前后元数据；严格 JSON/用途独立/固定脱敏，合同及部署限制归[会话设计](architecture/session-design.md#712-登录密钥文件加载)。仅把已锁定 `golang.org/x/sys` v0.49.0 提升为直接依赖，Go/其他版本/go.sum/SQL/schema 5 不变；核对固定版本官方 unix API 和 BSD-3-Clause 许可证。

改动前 Docker 禁网 race 基线通过（1.268 秒，`/private/tmp/oj-login-key-baseline.log`）；新增严格解析、文件边界、快照替换、轮换和脱敏测试，并让原双 TLS Cookie jar 联调用例分别通过合成文件加载共享 key。初轮单元 race 1.123 秒及会话静态 0 问题（`/private/tmp/oj-login-key-unit.log`）。Astra 发现指针专用 Stringer 会使解引用值格式化泄漏字段，改为值接收者并对值/指针逐字断言固定脱敏文本；nil 和 JSON 也覆盖。完整代码/测试/文档增量审查无剩余阻断，实际执行结果另核验，AI 不代替人工审批。

最终 Docker 完整 backend 通过格式/vet、检测器正负夹具、三个实际静态范围各 0、全部 race（redisstore 1.112 秒）与四入口构建（`/private/tmp/oj-login-key-backend.log`）；Darwin arm64 仅交叉编译通过，无 Darwin 运行声明。真实 Redis7.2.17/临时 PostgreSQL/race 专项 **84 个 PASS 条目（含父条目，17.600 秒）**、集成静态 0、退出/OOM/自有资源清理均通过（`/private/tmp/oj-login-key-protocol.log`）。323 文件仓库策略及完整差异/新增文件审查通过；没有改动 SQL/治理，不重复其本地独立套件，远端必需 CI 保持。

生产路径、真实密钥分发/备份/轮换、登录限流、默认 Gin/Compose/公开入口、浏览器/反代和完整 AC01/23/24/25/M0 仍待验，未部署。本轮只为后续限流装载独立稳定 HMAC 材料，不消耗额度或选择生产阈值；不把模式位检查当成 ACL/恶意可信写入者下的安全证明。

## 2026-11-01：跨请求登录候选与确认 Cookie 发放

成功 fetch 主线 `d2947fa` 后建立独立 `codex/cross-request-login` worktree。本轮单一目标是让普通邮箱密码认证产生的冻结候选跨 HTTP 请求恢复并在确认后发 Cookie；不开放默认 API、注册/邮件或资源权限。保留一个权威 Store：Prepare 在既有全局→主体门禁成功后封装候选但不写会话/名额；Complete 只恢复原 tuple，Retry 后独立 Check，失败不生成替代 Create/不清 Cookie；Abandon 仅作用于候选。窗口固定为 Redis created+显式 Window，实际 Lua 成功分支前检查；原 Create 显式传零，无记录/SQL/schema/依赖变更。协议、AAD/key 管理、限额及恢复/迟到效果边界归[会话设计](architecture/session-design.md#711-跨请求登录尝试与-cookie-发放)。

复用锁定 Go1.26.10 标准库 AES-256-GCM/随机 nonce API，核对本地 `go doc crypto/cipher.NewGCMWithRandomNonce` 的 nonce 前置/28-byte overhead/每 key 至多 `2^32` 次要求；聚合封装预算、共享 key 与轮换由调用方负责，本轮无真实 key 读取或生产配置。封装及 key 配置/manager 格式化脱敏，Redis 无 token 明文，密文无密码。HTTP 固定 direct TLS/Origin/Host、严格来源和唯一 Cookie；有效旧会话拒绝覆盖，确定失效允许重新认证，未知读取拒绝。登录 body 限 4096-byte、恰两个字符串字段、无重复/尾随/坏 Unicode；原 Unicode 检查无逻辑变化地移到 `internal/jsoninput` 供两个入口共用，避免私有跨模块访问或两份实现。

本地使用已有 Docker 质量镜像、Linux arm64/Go1.26.10、禁网 2 CPU/2 GiB 与 checksum 校验缓存。改动前会话包 race 基线通过（`/private/tmp/oj-cross-login-baseline.log`）；初版相关现有会话/HTTP 测试和静态通过（`first.log`）。新增密文往返、随机性、轮换/map 拷贝、配置隔离、畸形/篡改与固定窗口三项/16 子例、登录 JSON 一项/13 子例及原 HTTP 用例 race 通过，静态 0 问题（`unit.log`，redisstore 1.100 秒、httptransport 1.072 秒、httpapi 1.030 秒）。以上短文件名均以前缀 `/private/tmp/oj-cross-login-` 定位，输入在检查期间冻结。

首次真实专项在运行行为前被新增测试无用 import 的 typecheck 阻断（`protocol.log`），删除该 import，未放宽检查或断言。Astra 指出旧 HTTP 取消测试不覆盖新发放分支，补上 Retry/Check 实际成功后 HTTP 预算到期两用例，以及 6×6 组合错误优先 503/零 Cookie 的回归。最终 `python3 -B scripts/test-session-protocol.py --redis-image redis@sha256:0312984fbfeb2ff1d4382ecef45b50e7d65478a6a58cf12ce28d6de7a4142e45 --module-cache /private/tmp/oj-session-mod` 完整 **84 个 PASS 条目（含父条目，新增登录 40，14.549 秒）**、集成静态 0 问题及退出/OOM/自有资源清理核验通过（`protocol2.log`）。

真实证据：两个独立 Store/池/TLS 入口通过共享 Go Cookie jar，在另一重建 manager 恢复并发 Cookie；丢 Prepare 响应和无 Cookie 并发 Begin 均无会话写入。12 个并发 Complete 只出现一个 token/记录；重复恢复不改元数据与 deadline。真实 Retry/Check 后 wrapper 丢回执、实际操作后 HTTP 150ms 到期均 503/零 Cookie，另一 manager 恢复同 token/整记录/PEXPIRETIME；丢回执不是网络故障。三名额满后同候选等待原槽释放再准入，不刷新 created。旧 Cookie 确定 idle 失效后可重新认证，准备不提前清旧 Cookie；有效旧 Cookie 409。

`CLIENT PAUSE 650 WRITE` 使重试在 300ms 窗口后才执行，拒绝且无记录；Check 前屏障跨同一截止，保留已写 live 记录但不发凭据。五种畸形截止不降级为不限期。Abandon 在延迟 Retry 前建立 dead 阻复活；Check 成功后再 Abandon 的旧响应可携带迟到 Cookie，但后续 Read 必拒绝，未声称响应原子撤回。放弃不清另一个主体 Cookie；Rehash 后 Complete 拒绝但 Abandon 可清理，改密/禁用/换恢复代次旧候选拒绝。Prepare 的实际 PG backend 在 TIME 后被精确终止，整体门禁失败不发布封装且无 Redis 写入，复用既有唯一性和消失确认 helper。

完整离线 `go mod verify && sh scripts/toolchain.sh backend` 通过格式/vet、检测器正负夹具、三实际静态范围各 0 问题、全模块 race（httptransport 1.081 秒、redisstore 1.071 秒、原 httpapi 1.131 秒）及四命令构建（`backend.log`）；组合错误 36 个断言随此执行。317 文件仓库策略、全部新文件和完整目标分支差异审查通过；Astra 设计/产品/测试增量及完整复核无剩余代码阻断，并按其意见收紧清理边界和同步历史残留文案。未改 SQL 生成/迁移/治理脚本，不重复无改动的本地独立 DB 全套或 Worker 专项；CI 仍须交付回读。

默认 Gin/Compose/公开 OpenAPI 未挂载。无 OS 进程强杀、真实浏览器/关闭重开/反代、平台恢复、邮件/训练权限、生产限流/多 API 总预算/容量验收。保留 key/密文与权威状态连续才可恢复；候选遗失、窗口到期或提前丢钥可能失去恢复能力，未知已写记录按原 idle/absolute 占位；没有浏览器唯一 pending、最新意图顺序或恰好一次 Cookie 保证。完整 AC01/23/24/25/M0 未通过，未部署。最终 PR/head/CI/合并与知识回写按交付回读记录，AI 不替代人工审批。

## 2026-11-01：已有普通会话的 HTTPS 生命周期

成功 fetch 主线 `ad27b2a` 后建立独立 `codex/session-http-lifecycle` worktree。新增只适配已有普通会话的 HTTPS Handler：只读身份 GET、显式活动 POST、退出 POST 分别调用 Read/Touch/Revoke。固定可信 HTTPS origin、直接 TLS 和精确 Host，不读取转发头建立信任；POST 标准来源防护后要求唯一精确 Origin 或 same-origin Fetch Metadata 正证，任一出现头都须有效。标准库逐行严格解析 Cookie、跨行要求恰一个规范凭据；所有结果 no-store。Read/Touch 不发 Cookie，只有明确撤销成功且 context 有效才以安全 host scope 清 Cookie。公共响应不含凭据或认证/恢复代次，身份观察不提供资源权限。合同、SCS/反代边界与回退归[会话设计](architecture/session-design.md#710-已有普通会话的-https-适配)。

已有本地 Docker 质量工具链、Linux arm64/Go1.26.10、禁网 2 CPU/2 GiB 与已校验缓存运行基线，原会话/HTTP 包 race 通过（`/private/tmp/oj-session-http-baseline.log`）。首轮新 HTTP 行为/race 通过，但静态检查发现测试 Body.Close 未处理（`unit.log`）；已修复，未关闭规则。真实 Redis7.2.17/PostgreSQL 专项首次 9.391 秒复现三处退出/空闲过期误报 503（`protocol.log`）：协议明确 ErrStale 被通用 identity guard 包装，初版一律将 guarded 置为 503。结合实际 Lua 返回/写入次序与 Astra 复核，收窄为存储/恢复/未知/取消超时优先 503，明确协议拒绝可返回 401，即使 guarded；401 不确认撤销或回滚，所有错误无 Cookie/自动重试。原 Store/SQL/Lua 契约与实现均不变。

修复后 HTTP 八项/49 子例 race（1.087 秒）及新包静态 0 问题通过（`unit2.log`），补齐六种未知故障覆盖 guarded+stale、真实合作超时和取消后已执行效果。以上短日志名均以前缀 `/private/tmp/oj-session-http-` 定位；完整后端和真实专项最终结果随本节交付核验记录。所有 Go 输入在运行检查期间冻结。

第二轮完整真实专项 9.654 秒新增 HTTP 四子例全通过，但原 `protocol_test.go` 的绝对过期物理缺失断言读到 ErrStale 而非 ErrNotFound（`protocol2.log`）。[固定 Redis7.2.17 源码](https://github.com/redis/redis/blob/7.2.17/src/db.c)的 keyIsExpired 使用严格 `now > when`；现有等待只保证 TIME 大于等于目标，项目 Lua 在绝对期限小于等于当前时间时已拒绝。相等毫秒可出现尚未物理删除但已失效；只将该物理缺失断言前的等待延后一毫秒，保留 ErrNotFound 断言与所有产品期限，不改共用等待函数。Astra 定向复核认可该最小修正；这不代表延长会话有效期。

最终 `python3 -B scripts/test-session-protocol.py --module-cache /private/tmp/oj-session-mod` 完整专项 **44 个 PASS 条目（含父条目，10.438 秒）**、integration 静态 0 问题与容器退出/OOM/自有资源清理核验通过（`protocol3.log`）。两 TLS 入口实证原轮询/拒绝请求记录及绝对 TTL 不变、仅显式活动推进空闲期限且不改绝对期限、跨入口退出与迟到 Retry 拒绝、真实撤销后 wrapper 丢回执 503/无 Cookie、真实空闲过期/全局恢复/双入口禁用拒绝。丢回执为真实命令完成后的适配器注入，不是网络故障。完整离线 `go mod verify && sh scripts/toolchain.sh backend` 通过格式/vet、检测器正负夹具、三实际静态范围各 0 问题、全模块 race（新增 HTTP 1.094 秒）和四命令构建（`backend.log`）；之后仅修改上述 integration 到期等待行，最终专项已覆盖。305 文件仓库策略与完整差异/新增文件审查通过，Astra 设计/完整实现/测试增量和文档复核无剩余阻断；主代理实际运行检查，未重复无改动的 SQL 生成、原数据库独立全套或真实 Worker 专项。

默认 API/Gin、Compose、公开 OpenAPI 未挂载此 Handler，无登录、Cookie 发放或跨请求 Attempt；合成 Cookie 经受信 Create/Check 由测试注入，不代表真实浏览器 Cookie jar、关闭/重开或反代验收。无依赖/SQL/迁移/schema 变化，仍为 schema 5；不改变 Redis 基线，不在本阶段增加第二份状态。完整 SCS/Gin 生命周期、登录尝试恢复、邮箱/训练权限、活动采样/限流、恢复编排及生产容量另验，完整 AC01/23/24/25/M0 未通过，未部署。最新 head CI、反馈与合并条件须交付时回读，AI 审查不替代人工审批。

首个远端 head `2d8e780` 的[CI41272120843](https://github.com/DL-Association-for-Computing-Machinery/OnlineJudge/actions/runs/41272120843)在普通会话专项 8.048 秒失败：另一条“活动续期到绝对期限”原测试同样在恰好到期时断言物理缺失，读到 ErrStale 而非 ErrNotFound；本轮新增 HTTPS 子例仍全通过。PR #69 转草稿后补齐第二处 deadline+1ms 等待，逐一核对所有 waitTime/ErrNotFound 组合，专项内仅这两处验证物理缺失；IdleExpiresAt 的逻辑失效等待保持原样。未改产品代码、原断言、聚合超时或公共等待函数。日志 `/private/tmp/oj-session-http-ci-integration.log` 保留失败证据，后续追加修复提交而不重写已推送历史。

补齐后本地完整专项再次通过，44 个 PASS 条目（含父条目，10.518 秒）、静态 0 问题和自有容器/OOM/清理核验均成功（`protocol5.log`），Astra 增量复核无阻断。`protocol4.log` 仅记录默认 tag@digest 的本地 inspect 在创建容器前失败；只读回读确认同一 `redis@sha256:0312984fbfeb2ff1d4382ecef45b50e7d65478a6a58cf12ce28d6de7a4142e45` 仍存在，显式传驱动已有的 `--redis-image` 参数完成复验，没有下载或改版本；该本地名称解析差异原因未进一步确认。首轮远端除 integration 及其汇总外，其余七项检查成功；新 head 必须重新核验全部适用 CI。

## 2026-10-31：正式普通 Redis 会话协议

成功 fetch 主线 `7df407c2511342c950f1ee277f6d3d8974b44841` 后建立独立 `codex/ordinary-redis-protocol`，前置 PR67 主线 CI `41184439096` 已成功。新增 `internal/session/redisstore`，以同一权威 Authority 复用个人凭据认证及 PG 全局→主体门禁，落实普通空闲 2h/绝对 12h/三名额、冻结尝试的 Retry/Check/Abandon、Read/Touch/Revoke。合同归[会话设计](architecture/session-design.md#79-普通-redis-会话协议)。无 SQL/迁移变化，schema 5 保持；HTTP/SCS、Cookie/CSRF、跨请求持久化、权限、其他 profile/轮换及恢复编排不在本轮交付。

Create 冻结前失败无 Attempt，冻结后未知 Redis 或 PG Commit 失败均保留原 Attempt，独立 Check 成功才发 Credential。Retry 不回退已 Touch 的 idle，Check 原始回执冲突而当前 Read 可有效；Rehash 后 Read/Touch 保持，原尝试仍可 Abandon。令牌仅摘要入 Redis，十二字段/TTL/索引/长度/版本/绑定一致性校验；绑定摘要不作 MAC 或旧快照防护。启动核验 Redis 7.2.x standalone primary/noeviction，不证明后续配置或恢复安全。

锁定沿用实验的 go-redis/v9 v9.23.0（BSD-2-Clause），必须的 cpuid/v2 2.3.0→2.4.0 及其他传递依赖入锁。核对固定版本选项/许可证/依赖及历史 SETINFO 公告，显式池/超时、单次拨号、禁止隐式命令重试/额外 pipeline 池/维护通知并禁 SETINFO；完整漏洞扫描继续由 CI 执行。首轮既有 Redis Stack 镜像只运行裸 redis-server 7.2.4、无模块，属于隔离合成夹具；后续固定官方 Redis 7.2.17 多架构摘要 `0312984fbfeb2ff1d4382ecef45b50e7d65478a6a58cf12ce28d6de7a4142e45`，不把旧测试镜像当生产准入。

实际故障证据：双客户端并发恰好三名额、释放后重试原候选；Read/Touch/到期实际 Redis TIME 与 TTL、13 类坏字段/索引、账号变化/新恢复分组及 int64 上界。实际 PG 行锁等待取消前不写 Redis；定位后重新校验；精确 backend 终止后 Touch 的 Redis 迟到写仍可发生但不能返回 View；Create 已写 Redis、PG Commit 失败仍返 Attempt，后续 Check/Abandon 可确认清理。真实 CLIENT PAUSE 产生有界未知结果；已 OOM 的只读 Check/Read 成功、写脚本拒绝。丢回执/TIME 由客户端 hook 注入，部分写入由测试脚本 put 后主动 error，均不声称自然故障。TIME 失败 nil Attempt，之前实际提交的 Legacy 升级仍保留。

编排脚本仅创建带唯一 owner 标签的私有 internal 网络、自有 Redis/工具链容器，PG 仅 Unix socket；无发布端口、Docker socket 或选手执行。源码白名单复制、只读模块缓存合并后校验、readonly/race、显式仓库配置的 integration-tag 静态检查、退出/OOM 与 owner 清理都保留；不输出 PG 日志中的凭据记录。CI 在原 sqlc/PG 后执行同一专项，保持全部原检查和时限；README、计划、M0 与 CI/CD 同步。

本地实测使用现有 `oj-go-quality-toolchain`、Go1.26.10/Linux arm64、2 CPU/2 GiB 与既有校验缓存；除上述固定 Redis 镜像外不下载新工具或依赖。前置普通协议真实/race 10.119s；补三故障用例后 Redis7.2.17 专项 39 个 PASS 条目（含父条目）9.839s。完整 backend 首次指出测试 Close 未检查；integration-tag 静态补查指出 Close/Rollback，均修正资源清理，不弱化断言；Astra 指出临时目录需显式静态配置，已修复并加入治理断言。早期治理因沙箱端口限制 1 fail/1 error，获准原样执行后 64 项通过，最终完整治理 64 项 52.744s、新增4项回归及 actionlint 通过。

最终验证：`backend2-20261031.log` 中 go mod verify、完整 backend（格式/vet/检测器正负夹具/三范围静态各0/全部race/四入口构建）、两包 sqlc 无差异、真实 PG **158 子条目（20.255s）**及双 API 进程全通过，普通协议单元/race 1.107s。`integration5-20261031.log` 中仓库配置 integration-tag 静态0、真实 Redis7.2.17/PG/race **39 个 PASS 条目（含父条目，10.031s）**及正常退出/OOM/自有资源清理通过。日志统一前缀 `/private/tmp/oj-ordinary-redis-protocol-`；`verified-inputs.json` 记录核心/测试/脚本/锁文件/静态配置共22输入的 SHA-256，运行后逐字节哈希一致，模块校验不冒充源码快照。301 文件仓库策略、完整差异/新增文件及链接核验通过；无部署或生产容量结论。

Astra 完整核心/脚本/测试/编排及增量/文档定向复核无产品阻断，主代理实际测试和完整差异审查；AI 不冒充人工审批。最新 head CI/讨论/合并条件在交付时独立回读。100 同时在线已确认，峰值/延迟、生产参数、多 API 总预算和容量仍未验收；完整 AC01/23/24/25/M0 未通过，未部署。用户要求做完这一轮即停止，本轮收尾后不自动推进下一目标。

## 2026-10-31：个人凭据到双层门禁的实际认证路径

成功 fetch `b0f3ff0` 后建立独立 `codex/personal-credential-gate`；对应主线后续 CI `41183290661` 已成功。新增 PersonalCredentialGate，以一个组合存储 owner 和共享 BoundedHasher 内部构造既有 CredentialVerifier，避免误拼不同数据库的相同主体/版本；实现者仍须保证全部方法同一权威数据库/命名空间，接口不作物理证明。

执行路径固定为有效 open 恢复代次的一次读取→邮箱定位主体→真实密码验证/条件同密码升级/末次 State→以原代次和验证返回的完整 Revision 进入原同事务 global→account 门禁→至多一次受信存储回调。不使用邮箱查找所得旧 Revision，不补读/重贴恢复代次、不缓存候选、不重试或补偿。closed/missing 在查找和 KDF 前拒绝，坏元数据失败关闭；错误分类与未知副作用保持，合同归[会话设计](architecture/session-design.md#78-个人凭据到会话门禁的认证路径)。没有 SQL/迁移/索引/依赖变化，schema 5 保持；回退停用依赖流程后移除用例，不回退数据库。

已有 Docker 工具链 `oj-go-quality-toolchain`、Linux arm64/Go1.26.10、禁网 2 CPU/2 GiB 与既有缓存执行验证，无安装/下载、Docker socket 或选手程序。编辑前 `go mod verify` 与身份/密码/postgres race 基线通过（1.121/3.483/1.106 秒，`/private/tmp/oj-personal-credential-gate-baseline-20261031.log`）。首轮新增六项认证单元及原身份包/race、两包 sqlc、真实 PG/race **158 子条目（认证路径 20，17.694 秒）**及原双 API 通过（`test-20261031.log`；身份 1.112、密码 3.590、postgres 1.062 秒）。下列短日志名均用 `/private/tmp/oj-personal-credential-gate-` 前缀。

完整 backend 曾因 QF1008 要求去掉提升字段选择器而失败（`backend-20261031.log`）；将 `personal.Revision.Valid()` 等价改为 `personal.Valid()`，独立 ID/Email 校验保留，未修改断言/时限。最终原命令串行 `go mod verify && sh scripts/toolchain.sh backend && sh scripts/toolchain.sh sqlc && sh scripts/toolchain.sh db` 全通过（`verified-20261031.log`）：格式/vet、检测器正负夹具、三范围静态各 0、全部 race 与四入口构建；身份 1.126、密码 3.589、postgres 1.095 秒。生成内容不变，真实 PG/race **158 子条目（新 20，16.479 秒）**及双 API 进程通过。全部 Go/SQL 输入在运行期间冻结。

真实双池实证：有效恢复状态在邮箱/KDF 前捕获；大小写邮箱定位同主体，缺失/错密码/禁用/空密码不执行回调。恢复分别发生在查找后、真实 KDF 完成但 Verifier 末次 State 前、Verifier 成功后最终 guard 前，账号两版本不变，旧路径仍拒绝且恢复读取只一次；重新调用才重启完整验证。Legacy 升级后使用新 credential_version；升级前/升级后/最终门禁前×改密/禁用六交错均拒绝，保留变更后的完整凭据。屏障位于应用 Store 调用边界，不冒充数据库锁等待。

真实升级提交后 wrapper 注入回执错误并取消，保留 ErrStore/Canceled、无回调/重试/补偿；不声称真实网络注入。回调实际持有双层锁时 BeginRecovery 通过 pg_blocking_pids 确认等待，释放回调后才换代次；回调错误和回调内取消保持 ErrGuardedOperation/原 cause，Error 不回显私有内容，回调前取消则不执行。整个用例期限由调用方提供，合作取消和连接丢失仍不能撤销外部效果；不是 Redis/HTTP 或跨存储原子事务。

281 文件仓库策略、完整差异/新增文件与链接核验通过；README、设计、计划及 M0 同步。Astra 设计、完整实现/测试、等价增量及规范复核无阻断，主代理实测，AI 不替代人工审批；最新 head CI/讨论/合并条件交付时另行回读。EmailVerified 不冒充密码或训练权限；本次不决定未验证用户完整会话策略，不发 Cookie、不给训练授权。下一独立目标接入正式 Redis 协议；名额/条件写入/结果确认、HTTP/CSRF、邮件/令牌/权限、恢复编排、多 API 总预算/生产参数/100人容量及完整 AC01/23/24/25/M0 仍待交付，未部署。

## 2026-10-31：个人邮箱映射与原子初始化

成功 fetch `4bdf06b` 后建立独立 `codex/personal-email-identity`。落实用户确认的整邮箱忽略大小写、保留原始显示且不合并 dot/plus 别名；仅邮箱登录、昵称允许重复及比赛校内发号保持。新增私有字段 Email 值对象，复用锁定 Go 1.26.10 标准库 net/mail，再限制 ASCII 裸址、未引号 local、DNS 式标签与长度；没有隐式 trim、DNS/MX 或投递证明。Unicode 输入/IDNA 转换与 SMTPUTF8 暂不支持，不据此取消后续国际化需求。

迁移 5 增加主体 PK/FK、C 排序规则邮箱唯一键、原始显示与默认 false 的验证事实。InitializePersonal 在同一 READ COMMITTED 事务插入凭据与映射，KDF 在调用前完成，已有主体/邮箱均冲突且不覆盖、不为通用/比赛主体附加邮箱；第二步失败回滚凭据。两个读取只返回无密码时点元数据，Enabled 与 EmailVerified 分离。接口、未知提交结果及未完成注册/权限边界归[会话设计](architecture/session-design.md#77-个人邮箱映射与原子初始化)。无新模块依赖、公开 HTTP、邮箱验证或训练授权接口。

当前 API 精确要求 schema 5，旧要求 4 的二进制拒绝 5，无滚动混跑；停旧实例、独立迁移后启动新版，未部署。迁移不回填旧邮箱；Down 5 按凭据→邮箱固定锁序取得排他锁，仅邮箱表为空可移除，保留通用凭据/恢复/提交，任意非空映射（包括禁用/未验证）须前滚。多个 Down 不整体原子；旧迁移 4 用例固定版本 4 防止新保护掩盖旧断言，future fixture 为 6，迁移 3 原断言保持。

验证使用已有 `oj-go-quality-toolchain`、Linux arm64/Go1.26.10、禁网 2 CPU/2 GiB 与既有缓存，无安装/下载/socket/选手程序。起初 shell 参数被镜像既有 ENTRYPOINT 拒绝，两次均未运行测试；显式 `--entrypoint sh` 后未改主线的只读身份基线通过（身份 1.063 秒、密码 3.427 秒、postgres 1.026 秒），生成后旧包 race 通过。日志 `/private/tmp/oj-personal-email-identity-baseline2-20261031.log`、`generated2-20261031.log`；下列短名均用此前缀。

首轮 sqlc/邮箱单元与真实 PG/race 中，迁移、并发、故障回滚及计划通过，但新增运行用例复用既有恢复测试的 810/811/812 主体而失败（`test-20261031.log`，18.400 秒）；将其隔离为 1810—1851，没有修改产品行为或放宽断言。下一完整后端检查发现 QF1001，等价 switch 修复（`final-20261031.log`）。所有 Go/SQL 在检查运行期间冻结。

Astra 复核指出 Down 原先邮箱先锁，而 DROP 外键删除父表 RI triggers 时还会请求凭据排他锁，与初始化反序；[PG14 官方实现](https://raw.githubusercontent.com/postgres/postgres/REL_14_STABLE/src/backend/commands/trigger.c)的 RemoveTriggerById 确认该锁。新增真实初始化第一写后的屏障，先复现 `40P01`（`lock-repro-20261031.log`，15.900 秒，失败中断清理导致依赖/后续旧迁移保护错误亦保留）。修正为凭据→邮箱后，第一写/第二写后×提交/回滚四因果用例均通过：实际观察 Down 等待再释放，提交必须正常保护 `P0001`、不能把死锁算成功；回滚无孤儿。该结论针对初始化路径，不代表任意并发查询/DDL 无死锁，仍须维护期迁移。

最终原命令串行 `go mod verify && sh scripts/toolchain.sh backend && sh scripts/toolchain.sh sqlc && sh scripts/toolchain.sh db` 全部通过（`verified-20261031.log`）：格式/vet、检测器正负夹具、三个实际静态范围各 0 问题、全部 race 与四入口构建；身份 1.046 秒、密码 3.431 秒、postgres 1.077 秒。两包生成一致，真实 PG/race **138 子条目（邮箱相关 17，17.919 秒）**及原双 API 进程通过。覆盖原始显示/全址大小写、dot/plus 独立、两池竞争邮箱/主体且无孤儿、第二 INSERT 故障回滚、禁用/验证分离、数据库约束和坏元数据拒绝、4→5/空表回退旧数据不变、非空表拒绝；254 字节接受/255 拒绝边界亦通过。2000 合成映射的自然 EXPLAIN/BUFFERS 命中邮箱唯一/主体 PK 与凭据 PK，两个邮箱索引合计 311296 字节，不作生产负载/容量结论。

275 文件仓库策略及完整差异/新增文件审查通过，需求、README、设计、计划与 M0 同步。Astra 设计、完整实现/测试与锁序增量/文档复核无阻断，主代理实测，AI 不替代人工审批；最新 head CI、讨论和合并条件交付时另行回读。没有真实 Redis/邮件/Worker 专项复跑；未验证邮箱占位保留/清理、公开重复申请恢复/重发、防枚举、令牌/发信任务、激活/权限、登录捕获/会话协议、多 API 总预算/生产参数/100人容量和完整 AC01/23/24/25/M0 仍待交付，未部署。

## 2026-10-31：全局恢复状态与双层会话协调

成功 fetch `ee51db0` 后建立独立 `codex/session-recovery-authority`。迁移 4 增加唯一闭门占位行：generation 只允许空串或合法 32 位小写 hex，空串不能启用。RecoveryState 只读；Begin 在锁外生成随机 128 位代次，仅更新既有行并闭门；Finish 以预期代次和 closed 为条件开门，缺失行不自动补建。新 Session 两入口严格校验代次，在同一 READ COMMITTED 事务先全局 FOR SHARE、再主体 FOR SHARE，保留新候选两代次/已有会话仅认证代次的区别。旧纯主体入口签名保留，纠正文档为不可单独用于正式会话。接口及未知副作用合同归[会话设计](architecture/session-design.md#76-正式全局恢复状态与双层协调)。

当前 API 精确要求 schema 4，旧 schema-3 二进制拒绝 4，启用须停旧实例后独立迁移，无滚动兼容或实际部署。Down 4 锁全局→主体→submissions，要求确实唯一的空代次闭门占位且凭据/提交均空，初始化后前滚；缺失行不被视作未使用。旧迁移 3 回归显式固定 DownTo/UpTo 3 并断言版本/表存在性，随后恢复最新，避免新增保护掩盖旧断言；未来版本夹具改为 5，未放宽旧数据保护。

已有 Docker 工具链 `oj-go-quality-toolchain`、Linux arm64/Go1.26.10、禁网 2 CPU/2 GiB 与原编译缓存执行检查，无安装/下载、Docker socket 或选手程序。身份/密码包编辑前 race 基线通过（密码包 3.511 秒，`/private/tmp/oj-session-recovery-authority-baseline-20261031.log`）；生成后原 race 通过（`generated-20261031.log`，身份 1.115 秒、密码包 3.435 秒）。首轮新增 postgres 两项单元通过（1.050 秒），集成编译因未使用 import 失败，日志 `test-20261031.log` 保留；删除该 import 后原 sqlc/数据库/race/双 API 全套通过，119 子条目 24.552 秒（`test2-20261031.log`）。以上短日志名均以 `/private/tmp/oj-session-recovery-authority-` 为前缀。

Astra 建议将仅凭据、仅提交两种 Down 拒绝分支独立实证，已补齐完整行/数据不变断言。容器内原命令串行 `go mod verify && sh scripts/toolchain.sh backend && sh scripts/toolchain.sh sqlc && sh scripts/toolchain.sh db` 全通过（`/private/tmp/oj-session-recovery-authority-final-20261031.log`）：格式、vet、检测器正负夹具、三范围静态各 0 问题、全部 race 与四入口构建；身份 1.091 秒、密码包 3.448 秒、postgres 两项 1.017 秒；两包生成差异、真实 PostgreSQL/race 121 子条目（恢复相关 23，14.824 秒）及原双 API 子进程通过。Go/SQL 输入在每次检查运行时冻结，没有扩大时限或抑制检查。

实证包括完整凭据/源码/任务/租约 3→4 保留、占位/缺失/已初始化回退及闭门格式约束；实际 Begin 在 AFTER UPDATE advisory 屏障处持锁，Down 等待后分别因 Begin 回滚允许、提交拒绝。全局锁在等待主体时仍阻塞 Begin，双向等待后读取最新状态，过期/重复完成拒绝、并发完成一赢家；取消等待保持持锁方直到精确 backend 退出。真实密码用例未返回时换代次并开门、账号版本不变，旧候选仍拒绝；重新捕获 open 代次并重启完整验证才成功。单行 SELECT/UPDATE 的 EXPLAIN ANALYZE/BUFFERS 保留自然计划，不强制索引；新增单例主键服务唯一性，增加一份索引和全局更新/锁开销，无生产容量结论。按审查建议明确文档的有效 open 捕获要求，并在候选测试补上此前置断言后，原 sqlc/数据库/race/双 API 全套再次通过，121 子条目 15.750 秒（`/private/tmp/oj-session-recovery-authority-capture-20261031.log`）；产品代码未变。

266 文件仓库策略、全部新增文件和目标分支完整差异审查通过，README、计划、架构与 M0 同步；Astra 设计、完整实现/测试和增量/文档复核无阻断，主代理实测，AI 不替代人工审批。最新 head CI/反馈/合并条件在交付时另行回读。Begin/Finish 不是维护编排，随机代次不是外部单调 fence；接口不能证明捕获时机、旧实例隔离、Redis 清理或最新账号事实已校验。锁丢失后旧回调仍可能运行的限制保留，错误不自动重试或补偿。本地未重复 Redis/真实 Worker/备份恢复专项，无新模块依赖；真实 Redis/恢复流程、正式登录/会话协议、权限与生产预算、完整 AC23/AC24/M0 仍待交付，未部署。

## 2026-10-31：会话存储操作的主体状态协调

成功 fetch 主线 `b16d590` 后建立独立 `codex/identity-authority`。Store 复用调用方连接池，新增主键 `GetStateForShare` 与两个受信内部入口：WithCredentials 检查验证所得两代次，WithAccount 仅检查认证代次与启用状态，保持同密码重哈希不撤销已有会话。显式 READ COMMITTED/FOR SHARE 持锁协调同步且至多一次的短时会话存储回调；新候选不直接信任浏览器版本。合同、合作取消、锁丢失及后续协议边界归[会话设计](architecture/session-design.md#75-会话存储操作的主体状态协调)。

回调开始后错误统一带 ErrGuardedOperation，保留原 cause；Commit 数据库错误仍带 ErrStore，普通回调错误不伪装成数据库错误，Error 文本不回显私有内容。回调之前的冲突/取消不执行操作；之后的失败不证明外部效果未发生，无重试或补偿。五秒是合作 context，既不强制中断回调也不保证锁的硬时长；独立一秒尽力 rollback 覆盖取消/panic，panic 继续传播。没有恢复 schema、Redis 实现、HTTP/Cookie、权限或邮件，不能称完整生产会话授权/跨存储事务。

已有 Docker 工具链 `oj-go-quality-toolchain`、Linux arm64/Go1.26.10、禁网 2 CPU/2 GiB 与既有编译缓存执行检查，无安装/下载、Docker socket 或选手程序。编辑前身份/密码包 race 基线通过（密码包 3.382 秒，`/private/tmp/oj-identity-authority-baseline-20261031.log`）；生成新查询及初步原 race 通过（`generated-20261031.log`，身份 1.100 秒、密码包 3.448 秒）。所有 Go/SQL 输入在检查运行时冻结。

首轮两包 sqlc 生成一致性、真实 PostgreSQL/race 全套 98 子条目（门禁 19，14.826 秒）及原双 API 子进程通过（`/private/tmp/oj-identity-authority-postgres-20261031.log`）。完整 `go mod verify && sh scripts/toolchain.sh backend` 格式、vet、检测器正负夹具、三范围静态各 0 问题、全部 race 与四入口构建通过，身份 1.115 秒、密码包 3.434 秒（`/private/tmp/oj-identity-authority-backend-20261031.log`）。

Astra 指出 backend 定位可进一步要求恰好一个匹配事务，已去掉 LIMIT 并检查完整结果集，异常时不终止任何 backend；尚未观察选错实例的失败。该夹具加固后按原命令重新完整执行 sqlc 与数据库/race/双 API，98 子条目 18.106 秒通过（`/private/tmp/oj-identity-authority-verified-20261031.log`），没有扩大时限或改变产品逻辑。

实证包括两个 FOR SHARE 并行、通过 pg_blocking_pids 确认阻塞重哈希/改密/禁用、反向等待后读最新行，以及默认 REPEATABLE READ 的夹具池仍由入口显式覆盖为 READ COMMITTED。取消锁等待保持持锁方直到精确 backend 消失；故意忽略 context 的回调证明取消不假装停止。回调阻塞时终止唯一 backend，确认退出后另池先完成真实改密，再释放旧回调产生外部标记，包装器报未知结果且旧认证代次被拒。标记不是实际 Redis，不能撤回已执行效果。自然 LockRows/主键计划在已有 2000 条合成填充上通过，无强制禁用 Seq Scan、新索引/迁移/依赖或生产容量结论。

255 文件仓库策略、全部新增文件和目标分支完整差异审查通过；README、计划、架构与 M0 同步。Astra 设计、完整实现/测试及夹具增量/文档复核无阻断，主代理实际验证，AI 不替代人工审批；最新 head CI/反馈/合并条件在交付时另行回读。未重复本地 Redis/真实 Worker 专项；未来恢复门禁须同一事务全局行→主体行，旧备份版本重复、会话条件写入/名额/结果确认、正式账号/权限、CPU/RSS及100人容量与完整AC23/AC24/M0仍待验收，未部署。回退移除入口/查询，无数据迁移；未来调用方回退须同时停用依赖流程。

## 2026-10-31：密码验证与条件升级用例

成功 fetch 主线 `ed1475a` 后建立独立 `codex/credential-verification`。新增 `CredentialVerifier.VerifyPassword`，连接凭据读取、共享 BoundedHasher、Legacy 同密码 Current 创建、两代次 CAS 升级和末次 State 复核。Current 匹配只读，升级须返回精确预期版本且不溢出，失败不降级为成功、不盲重试或补偿。完整合同归[会话设计](architecture/session-design.md#74-密码验证与条件升级用例)。输入是未来邮箱映射已解析的主体 ID，不改变个人邮箱登录规则；Store/Hasher 须使用兼容批准策略。成功仅描述末次读取时点，不是会话或权限授权；升级已提交后仍可能因回执、末次读取、撤权或取消而返回零结果。

已有 Docker 工具链 `oj-go-quality-toolchain`、Linux arm64/Go1.26.10、禁网 2 CPU/2 GiB 与原编译缓存执行检查，无安装/下载、Docker socket 或选手程序。编辑前 `go mod verify && go test -race -count=1 -v ./internal/identity/...` 的原密码包 21 项/51 子例通过（3.536 秒），当时身份包无单元用例；日志 `/private/tmp/oj-credential-verification-baseline-20261031.log`。检查运行时冻结 Go/SQL 输入。

单元首轮因测试辅助误用不存在的 `slices.LastIndex` 编译失败，已换显式计数，失败日志 `unit-20261031.log` 保留。按原命令重验身份 9 项/33 子例 race 1.041 秒、密码包 21 项/51 子例 3.678 秒通过（`/private/tmp/oj-credential-verification-unit2-20261031.log`）。覆盖 Current 只读、同密码升级、拒绝路径、升级失败/溢出/返回合同、末次每字段不符、逐阶段取消与未知写入不重试/补偿。

`sh scripts/toolchain.sh sqlc && sh scripts/toolchain.sh db` 的两包生成差异、真实 PostgreSQL/race 完整 79 子条目（新增应用用例 11，18.141 秒）及原双 API 子进程通过（`/private/tmp/oj-credential-verification-postgres-20261031.log`）。真实 KDF 与两个 Store 竞争同一旧版本，仅一赢家；Current 校验后、Legacy 升级前/后分别交错改密或禁用六场景，零结果且完整新状态不被覆盖。屏障位于应用 Store 调用之前，不声称 PostgreSQL 锁等待。真实提交后返回合成 ErrStore+Canceled 的夹具只模拟回执错误，不冒称网络故障；真实新行仍保留。

全后端首轮静态检查指出嵌入字段可简化和故意空 context 输入的测试表达问题（`/private/tmp/oj-credential-verification-backend-20261031.log`）。改为同一嵌入方法简写及 typed absent context，保留原断言，无禁用/抑制检查或行为变化。最终 `go mod verify && sh scripts/toolchain.sh backend` 格式、vet、检测器正负夹具、三范围静态各 0 问题、全部 race 与四入口构建通过，身份 1.122 秒、密码包 3.570 秒（`/private/tmp/oj-credential-verification-backend2-20261031.log`）。

251 文件仓库策略、全部新增文件和目标分支完整差异审查通过，README、计划、会话设计与 M0 同步。Astra 设计、完整实现/测试及等价增量/文档复核无阻断，主代理负责实测和完整差异，AI 不替代人工审批；最新 head CI/反馈/合并条件在交付时另行回读。无新依赖、SQL/迁移、HTTP、邮件或 Redis 变更，本地未重复 Redis/真实 Worker 专项；合成参数不证明生产资源或 100 人登录容量。回退移除内部用例和测试，无数据迁移；未来调用方回退须同时停用依赖功能。邮箱映射、正式登录/会话原子门禁、激活/权限、多 API 总预算、生产参数及完整 AC24/AC23/M0 仍待验收，未部署。

## 2026-10-31：共享实例密码计算准入

PR #61 合并后成功 fetch，并从 `75e5b53` 建立独立 `codex/password-admission`。唯一目标为密码核心的同步计算准入；`NewBounded` 显式配置计算名额、等待人数与排队期限，Create/Verify 共享同一实例。输入与记录先做廉价校验；等待满额返回 Busy，自身超时与调用方取消/期限分开，等待位先于计算释放。计算开始后不以超时 goroutine 提前返回，实际派生/随机源调用返回后才释放名额；已观察的调用方取消优先并清零结果。没有账号接口、SQL/迁移、Redis 或跨进程协调，详细合同归[会话设计](architecture/session-design.md#73-密码计算准入)。

复用已锁定的 x/sync v0.24.0，仅改直接依赖标记，go.sum 不变；核对官方 semaphore 合同与固定版本实现/BSD-3-Clause。使用已有 `oj-go-quality-toolchain`、Linux arm64/Go1.26.10、禁网 2 CPU/2 GiB及既有编译缓存，无安装、下载、Docker socket 或选手程序。编辑前密码包基线 11 项/47 子例 race 1.225 秒通过，日志 `/private/tmp/oj-password-admission-baseline-20261031.log`，测试运行期间冻结 Go 输入。

首轮完整密码包 21 项/51 子例 race 3.472 秒通过（`/private/tmp/oj-password-admission-focused-20261031.log`）。`go mod verify && sh scripts/toolchain.sh backend` 的格式、vet、检测器正负夹具、主模块/sandbox 静态/独立会话模块三范围静态各 0 问题、全部 race 及四入口构建通过，密码包 3.465 秒（`/private/tmp/oj-password-admission-backend-20261031.log`）。Astra 复核发现 50ms deadline 子例要求捕获瞬时排队窗口的测试脆弱点，尚未观察实际失败；已移除该子例的窗口观察要求，仍检查 DeadlineExceeded、零派生/结果、原持有者许可和等待容量，cancel 子例继续用屏障证明真实排队。没有扩大时限或改产品逻辑。最终相同命令的密码包 race 21 项/51 子例 3.472 秒及包静态 0 问题通过（`/private/tmp/oj-password-admission-verified-20261031.log`）。

246 文件仓库策略、全部新增文件与目标分支完整差异审查通过，README、会话设计、计划、CI 与 M0 状态同步。Astra 设计/完整实现/测试增量与文档复核无未解决阻断；主代理负责完整差异和实际测试，AI 不代替人工审批。最新 head CI、反馈及合并条件在交付时另行回读。本地未重复数据库/Redis/真实 Worker 专项；本项测试使用合成小成本与受控派生屏障，不作生产 KDF 参数或 CPU/RSS/100 人容量证据。

这是每实例预算；所有相关流程须共享同一实例才可称进程额度，各 API 额度总和仍须结合目标硬件实测。没有账号/来源限流、公平性、防枚举或生产默认数值；用户已确认同时在线 100 人，登录峰值/延迟仍未定。回退移除包装及依赖直接标记即可，无数据迁移；未来生产准入必须确保所有计算路径均有预算，不能将无包装回退当作安全上线。正式账号与完整 AC24/AC23/M0 未验收，未部署。

## 2026-10-31：个人账号登录标识确认

PR #60 合并为 `8c13e79` 后成功 fetch，并从同一 SHA 建立独立 `codex/personal-email-login`。用户确认日常个人账号仅邮箱登录、昵称允许重复；需求、账号设计与计划同步，比赛专用账号继续按校内发号方案。昵称用于显示，不是唯一账号标识；邮箱规范化、重复注册、变更及其余注册字段规则仍待账号流程设计。邮箱验证、训练提交权限及找回规则保持。

本项为四份文档的产品决定同步，没有账号接口、数据库或运行行为变化；核对完整差异、相对链接、既有比赛/邮箱验证边界及仓库策略。应用检查按纯文档范围由 CI 分类，不将跳过算作应用实测；最新 head CI 与合并状态在交付时回读。首批同时在线 100 人已确认，登录峰值/延迟及生产硬件仍未定，正式账号与完整 M0 未验收，未部署。

## 2026-10-31：主体凭据持久化与条件更新

从成功 fetch 的 `effafc3` 建立独立 `codex/identity-credentials`，目标为身份模块的一行主体/密码状态与 CAS，不提前决定邮箱/登录名、注册和角色规则。迁移 3 新增 `identity_credentials`，sqlc/pgx 提供 Current 初始化、支持 Legacy 的完整读取、无密码状态读取、重哈希/替换/禁用的两版本条件更新。单 SQL 自动提交，不在锁内计算密码；版本按操作递增且不回绕、无删除/复用或覆盖初始化接口。数据库错误保留内部原因而不回显失败记录，写结果错误仍可能未知，不自动重新读取或重试。具体合同、信任与授权边界见[会话设计](architecture/session-design.md#72-主体凭据持久化与条件更新)。

开工基线首次在集成测试动态 `go build` 控制端期间编辑源文件，导致 schema 2 的测试进程混入要求 3 的新二进制，控制端未就绪；该基线无效，日志 `/private/tmp/oj-identity-credentials-baseline-20261031.log` 保留。随后用精确 `effafc3` Git archive 只读快照重跑：先遇嵌套 tmpfs 挂载点不存在，再因自建构建 tmpfs 缺 exec 而仅 Python 阶段失败；均未改产品/测试断言。补空挂载点并仅为本次可信构建输出指定 exec 后，原完整 48 子例与双 API 子进程基线成功，日志 `/private/tmp/oj-identity-credentials-baseline-frozen3-20261031.log`；早期 frozen/frozen2 日志保留，不追认为完整通过。

首轮新实现的核心/race 与两包生成差异通过，真实集成编译发现旧迁移测试改起点后在同一作用域重复 `:=`；修为赋值后原样重验，真实 PG/67 子例和双 API 进程通过（17.643 秒）。Astra 完整审查要求独立覆盖仅 auth 过期的替换，以及匹配 65 字节载荷的存储上限，均补齐，并验证 64 字节合法回滚。新增无密码 `State` 查询，使新调用方在旧策略退役后可取版本并尝试撤权；仍由 CAS 防读取后的竞争。首轮和中间成功日志分别为 `/private/tmp/oj-identity-credentials-test-20261031.log`、`/private/tmp/oj-identity-credentials-test2-20261031.log`。

首个完整候选冻结 Go/SQL 源码，在已有 `oj-go-quality-toolchain`、Linux arm64/Go1.26.10、禁网 2 CPU/2 GiB、既有 Go 缓存下串行执行：`go mod verify`、核心 `go test -race -count=1 -v ./internal/identity/...`（11 项/47 子例，1.375 秒）、两包 `toolchain sqlc`、完整 `toolchain backend` 和 `toolchain db` 全部通过。主模块、sandbox 标签和独立会话模块静态各 0 问题，检测器正负夹具仍有效，全部主模块 race 及四入口构建通过；真实 PG 集成 67 子例（身份新增 19，14.590 秒），双 API 进程的 24 并发、重启/回执与旧材料保持通过。未执行 sandbox 选手程序或完整 Redis 会话专项；日志 `/private/tmp/oj-identity-credentials-final-20261031.log`。

主代理最终自审再补三个更新方法各自“仅凭据版本不匹配/仅认证代次不匹配”的六种断言：主体仍启用、版本均正且记录有效，每次拒绝后完整行不变，不能被输入校验或启用门禁提前挡住。Astra 对该增量复核有效；未改产品 Go/SQL、原用例或时限。两包生成差异、原完整 PG/race 67 子例及 Python 双 API 再次通过（16.585 秒），该候选日志 `/private/tmp/oj-identity-credentials-revision-guards-20261031.log`；PR 以新增测试提交后的最新 head CI 为准，不复用首个 head 的检查。

远端 head `069a4ff` 的 CI `41175219123` 在取消用例失败：收到客户端 `context.Canceled` 后立即释放 holder，原完整行断言失败；其他身份用例及后端、安全、前端、容器检查通过，PR 转草稿。该日志只证明快照不同，不能追认它恰为完整禁用新行。读取固定 pgx v5.11.0 的默认 deadline watcher/asyncClose 和 PostgreSQL 14 取消协议，Astra 定向复核确认原测试误把客户端返回当作后台清理完成屏障；产品的 `ErrStore` 未知结果合同无需改变。失败日志 `/private/tmp/oj-identity-credentials-ci-integration-failed-20261031.log`。

修复测试因果顺序：捕获实际阻塞的 `pid + backend_start`，取消后保持 holder 锁，用独立 observer 自动提交查询确认精确 backend 消失，再解锁并保留完整原行断言。另建独立单连接测试池，保留默认 watcher/Store SQL，只在第二次 CancelRequest 拨号处用通道屏障延后取消；确认客户端错误后解锁、独立观察完整禁用新行提交，再放行取消并确认 backend 消失。未用“不再阻塞”或宽泛 idle 状态代替退出，也未把原断言放宽为任意旧/新结果。所有观察仍用原 2 秒预算，无睡眠碰概率，不改产品代码/SQL或请求时限。Astra 增量复核无阻断；两包生成差异、完整 PG/race 68 子例（身份 20，14.419 秒）及双 API 进程通过，日志 `/private/tmp/oj-identity-credentials-cancel-barrier-20261031.log`。最新 head CI 仍须重新核验后才能交付。

新用例包括双 Store/独立池的两重哈希及两替换仅一赢家、完整最终行核对，`pg_blocking_pids` 观测实际等待后由持锁夹具直接改变权威行并提交、旧 Store 写入拒绝（不声称双方均经 Store），以及调用方取消、AFTER UPDATE P0001 整条回滚、bigint 上界、策略退役后的无密码撤权。迁移保留旧源码/任务/租约，空库回退升级、并发初始化提交时 Down 拒绝/回滚时允许，以及原版本 1/2 的提交竞态断言保留。原 future-schema 夹具改为 4，不降低兼容性拒绝要求。

合成 2000 条填充下，真实读取与版本条件 UPDATE 等价形状自然使用 `identity_credentials_pkey`，未关闭 seqscan；样本主键索引 139264 字节。不按版本字段额外加索引；主键提供点查，但增加每行存储和写入维护，样本不作资源/容量结论。sqlc 两包共读权威 migrations，查询目录隔离，`omit_unused_structs` 只移除原生成包未引用的 33 行模型，未移动业务职责；锁定 [sqlc v1.31.1 配置](https://docs.sqlc.dev/en/v1.31.1/reference/config.html)已核对，生成差异检查覆盖两包，无依赖升级。额外在容器私有副本故意添加身份生成文件漂移，原检查按预期退出 1 且差异定位新包，负向探针通过（`/private/tmp/oj-identity-credentials-sqlc-negative-20261031.log`），未改仓库源文件或 CI 断言。

Astra 设计、完整实现和状态/测试增量复核无阻断；文档复核已校正回退范围及旧编码待办，主代理负责实测和完整差异审查，AI 不代替人工审批。治理 60 项通过（37.808 秒，`/private/tmp/oj-identity-credentials-governance-20261031.log`），243 文件仓库策略及完整差异通过。新 API 精确要求 schema 3，旧 API 不接受 3；启用需停止旧实例后独立迁移，不承诺滚动兼容。Down 仅 identity_credentials 与 submissions 均空时可用，保留已有主体或提交时须前滚修复，跨迁移 DownTo 不具全局原子性。初始化/重哈希/替换均为受信持久化原语，不能代替操作人授权、同密码核验、重置令牌消费、审计、发会话前复核或 Redis 撤权。100 同时在线不代表计算峰值；生产参数、计算准入、真实账号及完整 M0 未验收，未部署。

## 2026-10-31：正式 P3 同步密码记录核心

PR #58 合并为 `d10c9e3` 并完成证据归档后，成功 fetch 最新主线，从同一 SHA 建立独立 `codex/password-record-core`。目标是无数据库、网络和持久化副作用的密码记录核心；Create 仍读取系统密码学随机源。账号数据库、编码入口、登录/会话、邮件和计算准入不在本次实现范围，不把有限核心当作 AC24 全项通过。

新增 `internal/identity/password`，`New(Policy, Limits)` 校验并复制当前/旧完整参数 tuple 和显式预算；构造后策略不可变。固定记录格式 1/Argon2id 版本 0x13，逐项长度/成本/版本及 tuple 匹配均先于派生；拒绝库会规范化的内存布局，宽整数和平台 int 边界先校验。Create 使用密码学随机盐，随机失败返回空记录；Verify 只在旧策略匹配时返回迁移提示，无重哈希或写库。输入原始字节不规范化/截断，空值和超字节上限拒绝，零值失败关闭；同步调用无伪取消语义，不承诺总并发预算或完整内存擦除。详细合同归[会话设计](architecture/session-design.md#71-同步密码记录核心)。

依赖沿用 `x/crypto v0.58.0`，仅改直接依赖标记，go.sum 不变。核对该锁定版本官方文档、BSD-3-Clause、IDKey 源码及 CLI 生成向量；新增测试不重算期望摘要。已有 Docker 工具链 `oj-go-quality-toolchain`/Linux arm64/Go1.26.10，禁网、2 CPU/2 GiB、只挂本任务源码，无 Docker socket 或依赖下载。`go mod verify` 与 `go test -race -count=1 -v ./internal/identity/password` 的 10 项/47 子例通过（1.201 秒），日志 `/private/tmp/oj-password-core-focused-20261031.log`；覆盖随机失败零派生、未知版本/畸形/超限/未批准 tuple 派生前拒绝、字节边界、策略复制和并发调用。结构最小值/合成低成本不作为生产参数建议。

同一离线工具链及本仓库 Compose Go 编译缓存执行 `go mod verify; sh scripts/toolchain.sh backend` 完整通过：格式、vet、检测器正负夹具、主模块/sandbox 标签/独立会话模块静态各 0 问题、主模块全部 race 及四入口构建；密码包在整套中 1.222 秒通过。日志 `/private/tmp/oj-password-core-backend-20261031.log`，没有执行 sandbox 选手程序或将静态编译记作会话真实联调。231 文件仓库策略、全部新增文件及目标分支完整 diff 审查通过，README、计划、架构和 M0 状态同步；远端最新 head CI 与合并状态在交付时另行回读。

Astra 完整四文件及依赖差异复核无阻断，主代理负责实际测试与完整差异审查，AI 不代替人工审批。生产硬件、集中登录峰值与延迟目标未定；用户确认的 100 同时在线不能推导同时密码计算数。未来网络入口必须有跨 API 计算准入，超时后名额不能早于计算结束释放；重哈希须凭据版本 CAS、发会话前复核，均继续待实现。当前无调用方/SQL/迁移，回退移除核心与直接依赖标记即可，不涉及用户凭据数据；生产启用后的记录兼容回退另行验收。未部署，完整 M0/账号验收仍未通过。

## 2026-10-31：普通会话 Store 有限成本采样

从成功 fetch 后的 `143da27` 创建独立 `codex/session-storage-cost`。唯一目标为相同普通会话授权合同下的有限路径成本采样及决策记录，不替换 Redis，不是 HTTP/真实登录或生产容量验收。新增驱动只用已有固定镜像和离线缓存，每样本独立私有 TCP 网络及空 PostgreSQL，Redis 路径另加 Redis；两组客户端启用账号/恢复守卫，全部样本共用离线生成的原始 Gob、期限与 fixture SHA。读取、原样重复保存、独立主体准入、满额拒绝分别校验结果和数据状态；固定握手将资源结束采样放在语义验证/统计前。方法、固定场景和局限见[专项说明](../tests/sessionexperiment/README.md#同等普通会话合同的-store-成本采样)。

首轮构建非零，但初版未保存编译输出，不能追认其确切根因；已补 stdout/stderr 有界保留。第二轮实际复现锁定 go-redis v9.23.0 `ConfigGet` 只接受单模式，修为采样结束后逐项读取四个配置。第三轮构建成功后首个样本在 TCP 连接检查失败；独立空库探针明确复现 `no pg_hba.conf entry`，证明 initdb 的默认 host 规则未覆盖容器网段。仅在本次空库加入 `host postgres oj_cost samenet trust` 后同探针返回 `oj_cost`，前后均定向清理成功；仍依赖容器仅接入新建 internal 网络，无宿主端口，不是真实认证或生产配置。日志为 `/private/tmp/oj-session-cost-smoke{,2,3}-20261031.log` 与 `/private/tmp/oj-cost-tcp-probe-{before,after}.log`，对应 JSON 保留失败样本。

后续并行启动小样本构建与原完整 race 专项，两者均触发各自原有 300 秒驱动总预算：成本构建未产生样本，race 已完成 108 项行为及 279 子例、无断言失败，最后已有 Redis 暂停恢复项被外预算中止，均不能计完整通过。并行干扰尚未归因 CPU/内存/I/O，未增加原预算或降低断言；已改为串行复验，日志 `/private/tmp/oj-session-cost-smoke4-20261031.log`、`/private/tmp/oj-session-cost-race-20261031.log` 和 `/private/tmp/oj-session-cost-race-serial-20261031.log` 保留。

驱动单测 14 项及模块静态 0 问题通过。完整治理首次被宿主沙箱禁止本地监听/Docker 读取而失败，经工具审批按同一原命令重验 60 项通过（37.208 秒）；没有改断言。原完整行为/race 驱动在相同源码、预算和断言下独立串行重验，109 项行为/race、279 子例通过（91.652 秒），格式/模块校验/vet 随原驱动通过；另有三个既有/成本专用入口按合同跳过，日志 `/private/tmp/oj-session-cost-race-serial-20261031.log`。此结果不追认此前并行运行通过。

其后同一原 1 CPU/1 GiB builder、300 秒构建预算独立执行小样本，14 场景全部通过（每样本 8 次，不能作跑分），fixture SHA/配置/结果计数/原载荷与名额语义校验均通过。按结果中的精确 owner label 回读，无残留容器或网络；日志和结果为 `/private/tmp/oj-session-cost-smoke5-20261031.{log,json}`。因此未据并行超时扩大 builder 资源或提高验证时限。

最终以 `python3 scripts/test-session-cost.py --toolchain-image oj-go-quality-toolchain:latest --module-cache /private/tmp/oj-session-mod --rounds 3 --operations 2048 --output <新JSON文件>` 独立完成 42 样本、86016 次请求，61440 次成功、24576 次预期名额拒绝，其他结果类别为 0。独立核验完整场景矩阵、共同 fixture SHA、原始延迟重算分位数、语义与资源窗口，Astra 再次核对原始数据与决策边界无阻断；精确 owner label 回读无残留容器/网络。结果 `/private/tmp/oj-session-cost-full-20261031.json` SHA256 为 `0a86c18dd7cede46d58dc93d168ec6d03a963e9f99c63a937a1c8c3e31f14851`（2405876 字节），日志同名 `.log`；独立分析 `/private/tmp/oj-session-cost-analysis-20261031.{json,log}`，保留逐轮数据和对应代码哈希。最终模块静态 0 问题（`/private/tmp/oj-session-cost-lint-final-20261031.log`）。结果范围和继续 Redis/转正式 P3 决策统一见 [M0 记录](m0-experiment.md#普通会话-store-有限成本采样2026-10-31)，不把并行超时或小样本追认为正式结果。

Astra 设计/完整实现及 HBA 定向复核、主代理完整差异审查，AI 不代替人工审批；新增中断及临时目录清理失败回归确保未完成任务不能报告通过。两镜像实测均 linux/arm64；Docker VM 为 10 CPU、8319504384 字节、LinuxKit 7.0.14。PostgreSQL 为 tmpfs、Redis 关闭持久化，外围 CPU/内存与操作延迟窗口不同，不能据此作引擎排名、RSS/持久盘或生产 SLA 结论。无生产程序/依赖/迁移，完整 AC23/M0 未通过、未部署。

## 2026-10-31：PostgreSQL 绝对过期凭据有界清理

从成功 fetch 后的 `0337f1e` 创建独立 `codex/session-postgres-cleanup`。唯一目标为测试专用普通会话存储的显式过期清理：批量 1—128、派生 3 秒上下文、事务内一次数据库截止值、按 namespace/绝对期限/token 排序的带锁 CTE 删除；只取凭据锁，不反向取主体/账号/门禁锁。原有空闲过期或撤销墓碑在绝对期限内继续保留，所有主体/账号/门禁行均不删除。无新依赖、生产代码、迁移、后台调度或 Redis 替换，回退测试 helper/用例及测试 schema 索引即可。

基线为前一已交付主线的完整 100 项行为/race、272 子例通过（89.684 秒）；本项不再重复无变化基线。首轮 Docker vet/typecheck 发现新增表锁测试把 pgx 的 uint32 PID 传给既有 int32 helper，未运行行为专项；按既有模式改为 `SELECT pg_backend_pid()` 扫描 int32，失败日志保留于 `/private/tmp/oj-pg-cleanup-test.log` 和 `/private/tmp/oj-pg-cleanup-lint.log`。修正后按原命令、120 秒 Go 聚合 watchdog/300 秒测试容器预算及全部业务断言完整重验：Docker Desktop Linux arm64、Go1.26.10、私有 Redis7.2.4/PostgreSQL14.24，`python3 scripts/test-session.py --toolchain-image oj-go-quality-toolchain:latest --module-cache /private/tmp/oj-session-mod` 的 106 项行为/race、274 子例全部通过（96.894 秒，另一个既有子进程专用入口跳过）；格式、模块校验和 vet 随驱动通过。完整日志 `/private/tmp/oj-pg-cleanup-final.log`。

新增六项覆盖精确毫秒边界/有界多批/非法批量与 namespace 隔离、两清理事务交错提交或回滚、原 Gob 成功加载后真实到期→物理删除→两池迟到保存拒绝/新三名额、AFTER DELETE 语句级 P0001 整批回滚，以及 `pg_blocking_pids` 观察真实表锁等待后按调用方 1 秒期限失败/恢复。四表全部列快照只允许预期凭据删除。真实 `EXPLAIN ANALYZE` DELETE 在目标 4000 条未来＋32 条到期、其他 namespace 2000 条到期的合成数据上使用 `(namespace,deadline_ms,token)` 索引范围，实际上限 16 条、剩余 4016，随后回滚恢复 4032；共享夹具索引 1204224 字节，并非该 namespace 的独立容量或生产负载测量。

后续静态检查要求边界测试的 if 链改为等价 switch（QF1003），保留失败日志 `/private/tmp/oj-pg-cleanup-static.log`；修正后模块静态 0 问题（`/private/tmp/oj-pg-cleanup-static-final.log`），最终源码完整 106 项行为/race、274 子例再次通过（96.111 秒，`/private/tmp/oj-pg-cleanup-verified.log`），原格式/校验/vet及预算/断言全部保留。221 文件仓库策略与完整暂存 diff 通过，专项、M0 及会话设计状态入口已同步；最新 head CI 在交付时另行回读。

Astra 设计、完整实现及文档复核无阻断，主代理已自审并实测，AI 不代替人工审批。批量上限不保证固定扫描量，返回 0/不足上限可能仍有被锁到期记录；DELETE 不代表立即回收磁盘空间。索引增加写入维护和存储，主体行仍累积；数据库时钟回拨、VACUUM/膨胀、长期容量、自动调度、完整存储性能比较、正式账号与 AC23/M0 未验收，未部署。提交回执失败仍未知。

## 2026-10-31：PostgreSQL 会话与账号同库旧备份恢复

前置PR #56已按head`b76b809`的九项CI全部成功后合并为`40970b200a5a90b2a948a2268eec0773180834a4`。本次正常合入最新主线，保留两份交付记录解决唯一文档冲突，不重写历史；恢复Go源码与原审查`6fc175d`一致。按实际120秒聚合watchdog的原驱动完整100项行为/race、272子例通过（89.684秒，新增恢复用例0.64秒），格式/模块校验/vet随驱动执行；模块静态已零问题，218文件仓库策略及完整diff通过。最终head CI及合并状态另行回读，不沿用原head检查。

从成功fetch后的`7572ab9`创建独立`codex/session-postgres-restore`。仅测试模块：提取现有私有schema创建/绑定helper，保持原Redis侧逻辑回档验证行为；PostgreSQL夹具分离初始化与serve，恢复时新连接启动既有状态，不重建账号或门禁。新增私有schema真实pg_dump/pg_restore，用四表全部列的逻辑快照核对Gob/期限、主体、账号与门禁精确回档；备份后新增凭据/禁用账号/轮换门禁，再关闭排空全部旧API及池后还原。无API时observer直接证实旧凭据复活、后来凭据丢失；新随机恢复标识闭门清载荷、拒旧完成、双新API闭门503/重开拒旧与三名额，并保留备份后更新的独立schema sentinel。账号启用/认证代次仍回到备份状态，未重建最新安全事实，三名额仅为合成预认证主体；不是生产复服许可或整实例/PITR、自动恢复协调验收。

首次验证使用Docker Desktop Linux arm64、已有质量镜像/校验缓存，保持原Go套件90秒/测试容器执行阶段300秒和所有业务断言。原99项主线基线（容器内两待改文件SHA与main逐一相同）在90秒总超时中止：98项完成85.65秒，最后已有Redis暂停恢复用例4秒，无断言/race失败。新增100项中恢复用例通过0.58秒，99项完成84.86秒，最后同一已有用例5秒且已输出核验记录后仍被总超时中止；当时完整验收未通过，不把单项通过替代整套结果。行为阶段一次资源采样27.17%CPU、262.3MiB/7.748GiB、OOMKilled=false；这不是峰值或已定位性能根因。未改业务断言/时限、并行或分片，也不把SCS默认memstore清理协程当作已证实的变慢原因。

停止随机重跑后以独立PR #56校准聚合watchdog，并在交付后完整重验，结果见本节首段。Astra设计、完整源码差异及文档复核无实现阻断；失败日志保留于`/private/tmp/oj-pg-restore-baseline.log`、`/private/tmp/oj-pg-restore-test.log`，资源采样为`/private/tmp/oj-pg-restore-runtime-resources.log`，最终通过日志为`/private/tmp/oj-pg-restore-final.log`。无新依赖、生产程序或迁移，不替换Redis，完整存储比较、AC23/M0未通过、未部署。

## 2026-10-31：会话实验聚合 watchdog 校准

恢复草稿PR #55（head`6fc175d`）的局部恢复用例0.58秒通过，但原主线99项和新增100项都被整套90秒watchdog截断：前者98项完成85.65秒、最后已有Redis暂停恢复用例4秒；后者99项完成84.86秒、最后同一用例5秒且已输出初次核验记录。均无行为断言/race失败，完整验收不能计通过。失败日志保留于`/private/tmp/oj-pg-restore-baseline.log`及`/private/tmp/oj-pg-restore-test.log`；同次行为资源一次采样27.17%CPU、262.3MiB/7.748GiB、OOMKilled=false，不是峰值或已定位全部变慢根因。

成功fetch后从`7572ab9`独立创建`codex/session-suite-watchdog`，只把`run.sh`聚合Go watchdog从90秒校准为120秒并同步执行说明。保留-mod=readonly/race/count=1、全部测试/断言、请求与故障暂停/过期/恢复命令原时限；不分片、并行共享Redis、升级依赖或猜测性修补memstore清理。120秒是测试数量及有意等待累积后的有限执行预算，不是业务延迟/性能SLA；测试容器执行阶段300秒预算保持（不覆盖此前网络/Redis创建就绪及最终清理）。

Docker Desktop Linux arm64、已有Go1.26.10质量镜像/校验缓存、私有Redis7.2.4/PostgreSQL14.24，实际`python3 scripts/test-session.py --toolchain-image oj-go-quality-toolchain:latest --module-cache /private/tmp/oj-session-mod`完整原主线99项行为/race、272子例通过（91.629秒，另子进程专用入口按既有合同跳过）；格式、模块校验、vet随原驱动执行。临时负向探针从真实run.sh提取全部go test参数，禁网/1GiB/2CPU私有模块用select{}挂起；Go明确报`panic: test timed out after 2m0s`，包耗时120.111秒、非零退出，证明不是关闭watchdog。探针含编译总182.962秒，260秒外层保护未触发，唯一自建容器已清理；脚本`/private/tmp/oj-session-watchdog-probe.py`、负向日志/JSON及正向日志保留。该探针不属于持续CI，其断言摘要和实际命令供复现。

shell语法、仓库策略及完整diff检查通过，Astra方案和实际差异/探针定向复核无阻断；主代理实测，不冒充独立人工审批。未解释所有运行时波动或解决默认memstore清理协程生命周期；回退到90秒会恢复已复现的聚合预算不足风险。无业务/数据库迁移/部署变化，完整AC23/M0未通过。PR55继续保持草稿，须在本独立目标交付后正常合入主线，重跑完整100项再核验最终head CI。

## 2026-10-31：PostgreSQL 会话全局恢复门禁

PR #53 首轮head `f7c8e28` 的应用/容器检查成功，但security因新minimatch公告失败；独立修复PR #54已按最新head九项CI成功后合并为`01c6078e1d55321eb16fbb964cc92e8eb49e968e`。本次正常合入主线，保留两份交付记录解决唯一文档冲突，不重写历史；会话模块与已审查版本逐文件一致。合入复验首轮在90秒套件总超时中止：98项已完成耗时87.09秒，最后已有Redis暂停恢复用例仅运行3秒，新增8项及子例已通过，无行为断言失败；Astra日志复核未见死锁，未确定整体变慢根因。相同命令/预算/断言原样重跑后99项行为/race、272子例通过（90.514秒），217文件仓库策略及diff检查通过；整套测试耗时余量有限是待跟进限制，不能据此宣称环境/夹具稳定性已解决。最终head CI另行回读。

从成功 fetch 后的 `0c3ade7` 创建独立 `codex/session-postgres-recovery` worktree，唯一目标为 P3/M0、AC23 存储比较的全局恢复门禁子项。新增可选门禁只在测试模块，不替换 Redis，无生产程序、依赖或迁移。登录捕获随机恢复标识；保存按全局→账号→主体→凭据锁序同事务重查新旧载荷；读取从门禁 LEFT JOIN 会话/账号同快照核验。begin helper 同事务换标识/关闭/清本命名空间全部载荷，保留原期限墓碑；finish 仅按当前标识及闭门状态重开，无管理 HTTP 或自动恢复编排。

Docker Desktop Linux arm64、已有 Go1.26.10 质量镜像与校验过缓存、私有 Redis7.2.4/PostgreSQL14.24。原91项行为/race、261子例基线69.980秒通过；首轮新增7项后98项/267子例71.726秒通过。Astra指出HTTP前置门禁可能掩盖存储闭门及新JOIN分支，补直接Find和五种单条件授权子例，并核验成功恢复仅清data、其他全部字段保留。最终 `python3 scripts/test-session.py --toolchain-image oj-go-quality-toolchain:latest --module-cache /private/tmp/oj-session-mod` 的99项行为/race、272子例77.173秒通过（另一个专供子进程启动的入口按既有约定跳过）；格式、模块校验、vet和最终完整离线后端三范围静态、检测器正反例、根模块race、四命令构建通过。仓库策略216文件及diff空白检查通过，未改治理脚本。Astra设计/完整差异/增量及文档定向复核无阻断，主代理自审实测，不冒充独立人工批准。

双API覆盖缺失/关闭门禁503无Cookie，重开后旧凭据401、新三名额/第四409、旧删除不影响新记录；已加载登录/保存跨关闭重开补满仍拒绝。新旧Gob的缺失/错误类型/空/错标识拒绝且完整状态不变；新LEFT JOIN独立覆盖账号缺失/禁用/代次不匹配及未知/过期凭据。pg_blocking_pids观察两个方向真实锁等待，验证保存等恢复提交后重查、恢复等保存后清其刚提交载荷。AFTER UPDATE P0001使门禁与所有会话字段完整回滚；去故障后只清本命名空间，其他门禁/账号/会话不变。

门禁主键定点查询/锁的读收益伴随每次保存共享行锁及恢复写入；批量清载荷持有受影响行锁，实测并发/回滚/范围隔离是关键SQL等价证据，无大规模UPDATE计划、WAL/吞吐或清理预算。关闭前已开始读取可按旧快照完成，不能撤回已加载请求或业务副作用；COMMIT回执失败仍未知。随机标识非外部单调版本或恢复操作者锁；会话与账号同库旧备份恢复、跨进程/主机、完整策略及公平性能对照仍未完成，完整存储比较、AC23/M0未通过，未部署。回退本项测试/可选夹具即可；PR/head/CI及合并回读见远端和知识库，CI静态不代替本地专项。

## 2026-10-31：前端 minimatch 公告阻断修复

会话恢复门禁 PR #53 的 head `f7c8e28` 在 CI41160776238/security job124417655807 命中 GHSA-g3hv-75wg-m249：现有开发依赖minimatch10.2.6受影响，公告修复版11.0.1。主线未改依赖也可能因公告更新失败；本次保留PR53草稿，不重试或忽略漏洞。从成功fetch后的`0c3ade7`独立创建`codex/minimatch-advisory`，仅为三个精确父版本配置override到11.0.1，自动生成锁文件并把配置加入Docker安装COPY；直接依赖版本不变，新增必要间接依赖@isaacs/nonrecursive-glob-match1.0.3。

官方公告、上游修复commit77a0f5c4b80bdc641c3c930a1d9edf5ad2db57f8及changelog、npm元数据核验：11版移除makeRe/regexp/AST，set元素变Matcher，不能默认为兼容。实际三个调用方用match/partial/pattern/minimatch，未发现依赖被移除API；包许可BlueOak-1.0.0，minimatch引擎18||20||>=22，新间接包未声明Node引擎，解包体积分别420517/380111字节。npm确认11.0.1已发布（上游对应release API404，不以release缺失推断未发布）；未放宽TLS、完整性、供应链策略或审计门禁。

Docker Desktop Linux arm64、锁定Node24.19.0/pnpm11.25.0，实际`docker build --target frontend-source`的`pnpm install --frozen-lockfile --ignore-scripts`成功；该镜像内`pnpm lint && pnpm typecheck && pnpm test && pnpm build && pnpm audit --audit-level low`通过：7项lint自检、6项Vitest、构建成功，audit无已知漏洞。新增lintFiles('.')按文件系统独立枚举全部TS/TSX及vite配置，并检查dist/coverage/node_modules忽略；原镜像和修复镜像分别`pnpm exec eslint . --format json`，实际7文件集合完全相同且零错误/警告，不将跳过文件当作通过。

临时探针`/private/tmp/oj-minimatch-probe.mjs`在禁网/512MiB/单CPU/10秒容器内，沿三个真实父包路径分别解析minimatch/package.json确认11.0.1、CJS/ESM匹配、partial目录匹配，以及上游30段`**/../a/b`/optimizationLevel2/braceExpandMax64扩张恰64与两个正常匹配，全部通过；该探针未加入持续CI，断言摘要供复现。完整diff/空白及214文件仓库策略通过；治理46项通过（首轮因沙箱禁止本机bind失败，按原命令获工具审批后9.847s通过，未改断言）。Astra设计及完整差异复核无阻断，主代理实测自审，不代替独立人工审批。兼容结论限当前三个父版本和项目用法，不宣称所有平台或任意glob兼容；未来父版本支持补丁时撤去对应override并复验。无业务代码/数据库迁移/部署；PR53须在本独立目标交付后正常合入主线重验。

## 2026-10-31：PostgreSQL 账号撤权与同事务会话变更

PR #51 修复续接：原 head `cf12d06` 的 CI 失败来自固定 Go 工具安装的校验站 HTTP/2 错误，尚未进入应用检查；独立修复 PR #52 已合并为 `39eae1a`。本次将该主线正常合入原任务分支，保留双方交付记录解决唯一文档冲突，不重写已推送历史。合入后重新执行真实 Redis/PostgreSQL 专项，91 项行为/race、261 子例通过（72.507 秒）；完整离线后端格式/vet、检测器正反例、三个范围静态分析、根模块 race 及四命令构建通过，仓库策略 213 文件通过。首次后端验证误挂载仅会话模块缓存，在测试前失败，改用质量镜像完整缓存后通过。Astra 对完整账号差异和文档定向复核无阻断，CodeRabbit CLI 本机未安装，未运行新的 CodeRabbit 审查；最新 head 的远端 CI 与合并结果以 PR/知识库回读为准。

从成功 fetch 后的 `1d31678` 创建独立 `codex/session-postgres-account`，唯一目标为 P3/M0、AC23 存储比较的账号撤权一致性子项。两个测试 TLS API 可同时启用账号守卫，原未带守卫的普通会话基线保持；仍只在 `_test.go`，无生产代码/依赖/迁移、无密码/角色或管理HTTP入口，不替换Redis。账号表主键为namespace/主体摘要，保存按账号共享锁→主体锁→凭据锁，在同事务核验启用状态/正整数代次及原载荷代次；读取单JOIN同快照核验。撤权helper按同序独占账号，同事务推进代次/设启用状态/清载荷释放名额，保留期限墓碑。

Docker Desktop Linux arm64、Go1.26.10已有质量镜像/缓存、私有Redis7.2.4/PostgreSQL14.24。原84项行为/race、247子例基线通过（68.909秒）；首轮新增7项/14子例后91项通过（69.082秒）。Astra复核指出禁用子例同时带旧代次、墓碑可能掩盖账号JOIN门，补为保留正确代次与有效载荷、仅禁用或删账号，独立验证存储Commit拒绝和双API读取拒绝且状态不变；同时补再启用立即拒旧及其他主体整行不变。最终 `python3 scripts/test-session.py --toolchain-image oj-go-quality-toolchain:latest --module-cache /private/tmp/oj-session-mod` 的91项行为/race、261子例通过（69.239秒）；格式、模块校验、vet和模块静态分析零问题，仓库策略213文件通过。设计、完整差异、增量断言及文档经Astra复核，主代理自审/实测，无剩余阻断，不冒充独立人工批准。

跨API覆盖批量失效、禁用、再启用、旧Cookie拒绝与新代次三名额；已加载登录/保存跨完整禁用→再启用窗口，新代次补满后仍401/无Cookie，旧删除不改变新记录。非法/错放代次在读/保存时拒绝，不建账号或改会话。双方向通过pg_blocking_pids观察实际锁等待：撤权未提交时保存等待后重查拒绝；保存已持账号共享锁时撤权等待，保存提交后撤权继续清除其载荷。AFTER UPDATE触发器实证P0001，清会话失败使账号状态/代次及全部会话字段回滚；同一故障下HTTP活动保存503/无Cookie且完整状态不变，撤权helper本身不提供HTTP合同。撤去故障后只清目标主体，其他主体与命名空间记录不变。

账号主键支撑定点读取/锁定，增加账号索引写入成本；清载荷查询可利用原部分索引主体前缀，未测UPDATE执行计划。锁等待及目标范围隔离是关键SQL等价行为证据，不是负载/吞吐测量。COMMIT回执错误仍未知；已开始读取与业务副作用不能撤回，正整数代次不能自行防止账号和会话一起回档。PostgreSQL全局恢复/旧备份、清理、管理/长会话/轮换、跨进程/主机及公平性能比较仍未完成。回退本项测试/夹具即可，一次性数据库随驱动清理；完整存储比较、AC23/M0未通过，未部署。PR/head/CI及合并回读见远端和知识库；CI静态不代替本地专项。

## 2026-10-31：固定 Go 工具安装的局部传输缓解

PR #51 的账号撤权目标在本机91项行为/race、261子例及完整离线后端检查通过后，CI head `cf12d06` 两轮均在应用检查之前失败：运行41099633376首次backend job124214395987安装golangci-lint时，sum.golang.org校验exhaustive模块的tile发生HTTP/2 INTERNAL_ERROR；同head一次失败任务重试的job124216969218安装sqlc时，wazero模块的校验tile同类失败。没有关闭校验/门禁或继续盲目重跑，PR51保留草稿。为保持一个PR一个目标，成功fetch后从最新主线`1d31678`另建`codex/go-tool-install-http1`。

仅在Dockerfile的sqlc1.31.1、golangci-lint2.15.0两个RUN上加命令级`GODEBUG=http2client=0`，绕开已观察到的HTTP/2传输路径，不改固定版本、TLS、GOSUMDB、代理、运行时ENV或CI断言。Astra定向设计复核无安全阻断，要求真实安装成功、版本与最终镜像环境核验以及完整CI后才交付；此为缓解建议，不是已证明消除网络故障。

锁定Go1.26.10、已有Docker工具链在禁网容器内实际验证：默认HTTP/2→命令级开关HTTP/1.1→默认HTTP/2，三轮均有已验证TLS证书链，且拒绝不受信证书，进程exit0。临时探针及日志在本机`/private/tmp/oj-http1-probe.go`、`/private/tmp/oj-http1-probe.log`，不纳入仓库。本机首次及后续完整工具链尝试因Go代理/校验站/CDN的TCP拒绝或超时失败，GitHub API也曾间歇不可达；用户报告恢复后，API镜像实际构建成功，实际inspect无GODEBUG环境项，本机工具链下载仍不稳定。未关闭校验、改代理或降低门禁。

PR #52初始head `08c4d67` 的CI运行41103361476已完整成功。backend job124225937735日志确认两条修改安装命令真实执行、并非CACHED：golangci-lint@v2.15.0层103.7秒、sqlc@v1.31.1层106.2秒，最终toolchain构建及后端/数据库集成检查通过。版本按固定@v安装日志、精确产物流转和生成代码校验核对，未对候选产物另跑版本命令；toolchain无新增GODEBUG采用Dockerfile命令作用域/独立FROM阶段的结构证据，未另做该最终镜像inspect，API镜像环境另有实际检查。Astra复核确认该等价证据覆盖局部改动，仍须核验文档后续head的完整CI和审查条件，实际PR/head/合并结果见远端与知识库。

这只证明候选在该次runner构建成功，不证明上游HTTP/2根因或全部网络故障消失。回退两处命令级赋值即可，无生产代码、依赖版本或数据库迁移；完整M0未通过、未部署。本独立目标交付后，再同步主线到PR51并重验账号目标。

## 2026-10-31：PostgreSQL 普通会话名额与撤销对照

从成功 fetch 后的 `274368c` 创建独立 `codex/session-postgres-admission` worktree。唯一目标为 P3/M0、AC23 存储比较的普通会话正确性子项，新增三个 `_test.go`，不替换 Redis、不修改生产代码/依赖/迁移。两个同进程 TLS API 各用最多4条连接，SCS普通2小时空闲/12小时绝对期限与非持久 Cookie；测试表按 namespace/主体摘要行→凭据摘要行锁序，在全部锁之后读数据库时钟，名额与写入同事务。毫秒列保留原Gob载荷（含纳秒deadline），撤销/过期记录不提前删除。

Docker Desktop Linux arm64、已有 Go 1.26.10 质量镜像/缓存，私有 Redis7.2.4、PostgreSQL14.24。原74项行为/race基线通过（67.835秒）；首次仅缓存路径不存在，在创建容器前失败，改用既有缓存后正常执行。首轮新增8项连同原74项通过（68.389秒）；复核后补齐普通策略拒绝、错放载荷和过期前可读断言，84项通过（68.900秒）。最后将策略拒绝收紧为明确errStale后完整84项行为/race、247子例通过（68.878秒），命令 `python3 scripts/test-session.py --toolchain-image oj-go-quality-toolchain:latest --module-cache /private/tmp/oj-session-mod`。格式、模块校验、vet、模块静态分析零问题；最初静态提示改用tagged switch已修。仓库策略210文件通过，Astra设计/增量复核和主代理完整差异自审无剩余阻断，不替代独立人工审批。

24并发跨API登录恰3成功/21冲突；跨副本撤销与补位后，已加载保存401/无Cookie且不挤成第四名额。十二次保存只占一名额、原绝对期限不变；主体/期限变更、scope/保持登录/畸形期限拒绝且无行残留，错放载荷双API拒绝且完整快照不变。真实时钟先可读再验证idle/absolute过期、旧保存拒绝与三名额回收；`pg_blocking_pids`实际观察主体锁等待，撤销提交后保存重新核验并拒绝。AFTER INSERT触发器实证P0001，登记及会话行同事务回滚，HTTP503/无Cookie，移除故障后登录恢复；关闭一池时读/活动/退出/登录503/无Cookie，全记录/期限不变，另一池仍可读。

三条有效目标会话与2000个其他主体的 `EXPLAIN (ANALYZE,BUFFERS)` 使用 `session_pg_active_fixture` 部分索引，实际访问3条目标记录；索引节约墓碑计数访问，但增加存储、续期索引写和撤销移出成本。未做吞吐、WAL、CPU/RSS或公平Redis/PG负载比较。夹具没有清理任务、账号/恢复代次、旧备份恢复、管理/长会话/轮换、跨进程/主机；池关闭不是数据库宕机，INSERT异常不代表UPDATE/COMMIT失联。回退移除新增测试，一次性数据库随脚本销毁；完整存储比较、AC23/M0未通过，未部署。PR/head/CI/合并回读见远端及知识库，根模块CI不代替本地专项。

## 2026-10-31：Worker 显式排空式下线

从成功 fetch 后的 `bf041fc` 创建独立 `codex/worker-drain` worktree，唯一目标为 P2/M0、AC28/29 的单槽排空式下线。核心新增 `RunWithDrain`，原 `Run` 传 nil；同一单次门禁、每轮领取检查许可，检查后的至多一个在途领取完成整题及回报。空队列可唤醒，原样未知回报重试/错误传播和 context 取消优先不变。命令独立注册 SIGUSR1 请求排空，SIGINT/SIGTERM 仍取消；原一次性 Stop、独立停止确认与正常锁标记清理不变，不自动重启或清异常隔离。

Docker Desktop Linux arm64、Go 1.26.10：原 Worker/cmd race 基线通过，原真实 report 10 子例通过（9.57 秒）。最终完整后端格式/vet、四规则正反例及主模块/sandbox/独立会话模块静态检查、根模块 race 和四个命令构建通过。新增单元覆盖每个阶段排空、未知回报三次/终端拒绝一次、错误不被掩盖、空闲唤醒、共享单次门禁、同时取消/排空及命令停止失败继续隔离；Astra 定向复核与主代理完整差异自审无剩余阻断，不替代独立人工审批。

`python3 scripts/test-worker.py report` 最终 11 子例通过（18.01 秒），新增 worker-drain 5.02 秒：观察真实 main PID 后再入队第二题、发 SIGUSR1，首题 AC 持久化且仅一次尝试，第二题 queued/零尝试；进程0退出、容器/PID停止确认后，显式重启同一固定沙箱和新 Worker，第二题AC/仅一次尝试，再空闲排空。原报告与取消命令用例均继续通过。初轮新增用例在 Accept 前因夹具幂等键短于16字节被拒，未启动新Worker；修为符合既有契约的键后重验，没有放宽输入校验。最终两槽均停止、私有DB容器删除，规范锁均0字节，未清未知标记。

仅新单槽信号行为、必要测试与文档，无新依赖、HTTP、SQL/租约协议或迁移。宿主程序由 Docker 跨编译，实际沙箱专项不含 race；Linux 单元保留 race。排空检查与信号到达竞态可允许一个在途领取完成，下线时延受原有预算影响，无固定SLA；不是SIGKILL、断电、网络分区、自动重启/解封、跨主机或部署验收。回退旧程序后不使用新增信号，数据库保持；完整M0未通过、未部署。PR/head/CI/合并回读见远端及知识库。

## 2026-10-31：会话写请求来源防护与安全方法边界

从成功 fetch 后的 `428ea2f` 创建独立 `codex/session-origin-protection` worktree，唯一目标为 P3/M0、AC23 的测试 API 来源防护子项。复用锁定 Go 1.26.10 CrossOriginProtection，在路由/会话加载前加显式 HTTPS/Host 精确来源与头值检查；无 trusted origin/绕过/CORS 授权。两来源头缺失仍兼容非浏览器驱动，不代替认证。七条写探针在 Load 前统一限制 POST，失败 405/Allow: POST。

原 69 项行为/race 基线通过（67.162 秒）。接入前的负向完整运行实测非法来源登录返回 201/进入业务处理，GET `/late-save` 返回 201 并保存会话；后端关闭时非法来源得到503、混合活动中的非法来源得到201，均不能通过新增断言。修复后 20 种非法来源×两 API×三 profile×七写路径共 840 个 POST 拒绝403/no-store、无原始Set-Cookie、全载荷/索引/绝对期限不变，未进入屏障。四种合法来源模式生命周期、安全方法只读、后端故障前置拒绝对照和48个混合来源并发通过。

最终完整 74 项行为/race、234 个子例通过（67.796 秒），格式、模块校验、vet、已有质量镜像 golangci-lint 零问题，repository policy 205 文件通过；命令 `python3 scripts/test-session.py --toolchain-image oj-go-quality-toolchain:latest --module-cache /private/tmp/oj-session-mod`。Astra 定向复核及主代理完整差异自审无剩余阻断，不替代独立人工审批。403处理器只写状态、不输出内部错误正文；行为断言验证状态、缓存头、Cookie和逻辑状态，未把正文检查列作实测证据。实际 PR/head/CI/合并结果按交付回读及知识库记录。

仅直连同进程双 TLS 测试 API；精确来源可能保守拒绝Host大小写/默认端口表示不同的非浏览器请求，不实现公共origin/反代信任、浏览器兼容或正式Gin/CSRF。没有新依赖、生产程序、SQL、迁移或驱动改动；回退包装器及门禁即可。未验证XSS/点击劫持/真实管理授权，不宣称完整AC23/M0或生产准入，未部署。

## 2026-10-31：会话凭据原子轮换与原期限保持

从成功 fetch 后的 `bc0c138` 创建独立 `codex/session-token-rotation` worktree，唯一目标为 P3/M0、AC23 的凭据轮换子实验。SCS 默认 RenewToken 会先删除旧记录并重置期限；新增每请求存储包装器，暂存准确旧凭据的单次删除意图，生成新凭据后恢复原 deadline，双代次守卫下单 Lua 转移原名额、旧记录保留原期限墓碑。没有新增依赖、生产程序、SQL、迁移或驱动。

普通/保持登录/管理在三名额满时一换一，策略、原载荷和绝对期限保持；每种 profile 下 12 个已加载轮换与 24 个新登录同时竞争，恰一个轮换成功、新登录全部 409。跨 API 拒旧/接受新，旧保存不能复活；已加载轮换跨退出、账号/恢复代次、闭门和空闲/绝对到期均被拒绝。池失败和真实入口 OOM 无 Cookie/状态改写，恢复后旧会话可用。十五种畸形/碰撞/策略/重复条件零写拒绝；写后丢应用回执仍 503/无 Cookie，候选只在测试进程观察，不自动再发或重新生成。

Astra 指出初版索引最大分数无业务上界；私有 Redis 真实复现 score=`2^63`、索引过期=`2^63-1` 时最后 PEXPIREAT 越界、旧载荷已删除且候选已写入。新增个人 7 天/管理 8 小时命名空间上界，在首条写前拒绝，并用普通与保持登录混合索引验证不误拒合法兄弟会话。首轮完整测试另有 old-expiry 的 DUMP 字节比较失败；改为按类型比较完整逻辑内容和原始 int64 PEXPIRETIME，保留非法字段/期限而不依赖非规范序列化。并发屏障发送补上下文取消，失败时可清理。AI 复核及主代理实测/完整自审不替代独立人工审批。

Docker Desktop Linux arm64、Go 1.26.10、Redis 7.2.4、PostgreSQL 14.24：原 61 项基线通过（57.703 秒）；最终完整 69 项行为/race、95 个子例通过（68.412 秒），格式、模块校验、vet、已有质量镜像 golangci-lint 零问题，repository policy 204 文件通过。命令为 `python3 scripts/test-session.py --toolchain-image oj-go-quality-toolchain:latest --module-cache /private/tmp/oj-session-mod`。实测与 PR/head/CI/合并状态按交付回读及知识库记录。

仅同进程双 TLS API、预认证身份与测试探针；没有正式登录/权限提升或重认证、Gin/CSRF、跨进程/主机、持久化轮换尝试及正式结果核验/放弃协议。丢回执为应用适配器注入，不是 TCP 丢包；入口 OOM 零写不保证执行中 OOM 或运行错误回滚。回退新增适配器/探针即可；完整 AC23/M0 未通过、未部署。

## 2026-10-30：Redis 服务端空闲连接断开后的重连预算

从成功 fetch 后的 `1545a60` 创建独立 `codex/session-reconnect-budget` worktree，唯一目标为 P3/M0、AC23 的有限重连预算/状态恢复子项。只新增测试，沿用 PR #45 每 API 主池上限 8、共享 pipeline 和无自动重试配置；没有新依赖、SQL、迁移、驱动或生产程序变化。

原五十九项行为/race 基线通过（57.190 秒）。真实借满并归还 8+8 条连接后，observer 按精确随机 API 标签提取 ID，逐条 CLIENT KILL ID、每条返回 1，再确认两个 API 均归零。连续两轮各 32 个 worker/128 次只读请求，仅允许有界 200/503 且原始 Set-Cookie 头为空；最终双 API/双 scope 全部恢复 200，重新借满新 8+8 并确认旧 ID 不再出现。完整只读快照保持载荷、键集合、索引和绝对到期不变，恢复后活动/退出/三名额正确；个人和管理分别验证跨 API 撤销、断连重连后已加载保存仍 401/无 Cookie、不改撤销后状态。

Docker Desktop Linux arm64、Go 1.26.10、Redis 7.2.4、PostgreSQL 14.24：最终完整六十一项行为/race、五十个子用例通过（57.609 秒），两轮 256 次断连后读取均成功，283 次采样未见超预算；格式、模块校验、vet 和已有质量镜像 golangci-lint 零问题，repository policy 201 文件通过。Astra 针对性复核后补齐并发读取失败清理和恢复读原始 Cookie/超时断言，最终代码重新执行完整验证；主代理核验完整差异及新增文件，AI 复核不替代独立人工审批。实际命令为 `python3 scripts/test-session.py --toolchain-image oj-go-quality-toolchain:latest --module-cache /private/tmp/oj-session-mod`；远端 PR/head/CI/合并状态以交付回读和知识库为准。

这是同进程双 TLS API、Redis 持续存活时的已建立空闲连接断开；观察到的带标签连接计数不超过每 API 8，但离散采样不保证捕获所有瞬时或未命名连接。observer ID 和 Redis run_id 不变，不是 Redis 重启、网络分区、在途写入回执丢失、跨进程/主机或生产容量验收。回退新增测试即可；完整 M0 未通过、未部署。

## 2026-10-30：会话 Redis 连接预算、饱和与恢复

从成功 fetch 后的 `ff6b4a9` 创建独立 `codex/session-pool-budget` worktree，唯一目标为 P3/M0、AC23 的实验 Redis 连接预算与饱和恢复。每 API 共享客户端显式主池上限 8，禁独立 pipeline 池，保留 1 秒取池/网络超时、3 秒 HTTP 上下文及无自动重试。随机客户端标签只用于观测，不包含凭据；独立观察客户端另限 1 条，不计入 API 的 16 条。

原五十三项行为/race 基线通过（51.987 秒）。修复前负向用例实测主池占满时旧 pipeline 仍可额外取得连接、PING 成功，触发失败；同次运行另有两个既有短期限用例在分步 `KEYS → 内容 → PEXPIRETIME` 快照中遇自然过期、报期限缺失。没有忽略这些失败：必要观测辅助改为单次 `EvalRO`/只读 Lua、嵌套 RESP 保留二进制载荷，仍严格检查有效键期限/类型，并补自然过期的完整 hash/index 及非法记录回归。

Docker Desktop Linux arm64、Go 1.26.10、Redis 7.2.4、PostgreSQL 14.24：最终五十九项行为/race 通过（57.258 秒，另含四十八个子用例），格式、模块校验、vet、已有质量镜像静态检查通过。真实 CLIENT LIST 观测 API `[8,8]`、总 16；16 个跨副本/双 scope 读取、活动、退出、新登录并发均有界 503/无 Cookie/全记录及绝对期限不变，101 次实时连接采样未超预算。释放后读写、退出和名额恢复；单副本已加载保存失败时另一副本仍可读。仓库策略 200 文件及完整差异/新增文件自审通过，Astra 针对性复核无阻断；AI 审查不替代独立人工审批。

仅测试模块及必要文档，无新依赖、SQL、迁移、驱动或生产入口，回退本项夹具配置/观测即可。16 条只覆盖同进程两个 API 普通命令和共用主池 pipeline 路径，不含观察客户端、驱动、PubSub/AutoPipeline/维护连接等。原子快照仍是私有 standalone 的测试前缀扫描；跨进程/主机、生产容量、吞吐/RSS/CPU、数据库和重连峰值均未验收，M0 未通过、未部署。最终 PR/head/CI/合并回读见远端与知识库。

## 2026-10-30：会话连接池声明纠错

收尾核对锁定 go-redis v9.23.0 源码发现，PR #43 将 `PoolSize=8` 描述为“有界 Redis 连接池”过强：`PoolSize` 只是基础池大小，`MaxActiveConns=0` 不设最大连接数。实验实际只证明同一 API 的两个 scope 复用一个 Redis 客户端，未验证最大连接数、跨副本总预算或容量。修正实验 README 和下方表述，保留期限/隔离的行为结果及其限制。

仅文档纠错，不修改配置、依赖、运行行为或既有测试断言；仓库策略、链接/源码依据和完整 diff 已核对，无需重跑应用套件。完整 M0 未通过，未部署；连接预算仍是待验证项。

## 2026-10-30：管理会话期限与 scope 隔离

从成功 fetch 后的 `2485f9f` 创建独立 `codex/session-admin-lifetime` worktree，唯一目标为 P3/M0、AC23 的管理会话期限与个人 scope 隔离子实验。同一双 TLS API 各有普通/管理两个启动前 manager，共享本副本配置 `PoolSize=8` 的 Redis 客户端及账号/恢复代次，管理使用独立 Cookie 名、路径和 Redis 命名空间；30 分钟空闲/8 小时绝对期限，不接受个人保持登录。读写校验 scope/持久化策略，固定上界用全新 token 独立验证，防止仅因旧期限不一致而得到假阳性。

Docker Desktop Linux arm64、Go 1.26.10、Redis 7.2.4、PostgreSQL 14.24：原四十二项行为/race 基线通过（41.413 秒），最终五十三项通过（51.873 秒，另含四十四个子用例），格式、模块校验、vet 和已有质量镜像静态检查零问题；仓库策略 197 文件、完整差异及新增文件自审通过。缩短配置/真实 Redis 时钟覆盖轮询、活动、两种过期与迟到保存；另有跨副本退出、双向改名/复制载荷拒绝、共存 Cookie、共享双代次、24 混合并发及真实入口 OOM 的 503/无 Cookie/无状态改写。Astra 针对性复核无剩余阻断，主代理实际运行验证；AI 审查不冒充独立人工审批。

无新依赖、SQL、迁移、驱动或生产端点，回退本项夹具和测试即可。管理身份仍是预认证夹具，不验证真实管理资格/密码/敏感操作重认证；名额按 scope 独立，管理夹具三名额不是正式管理额度或跨 scope 总三名额承诺。没有实际等待 30 分钟/8 小时、浏览器/Gin/CSRF、凭据轮换、跨进程管理会话或容量验收；完整 M0 未通过、未部署。最终 PR/head/CI/合并回读见远端与知识库。

## 2026-10-30：主动保持登录的期限与共享名额

从成功 fetch 后的 `e796eed` 创建独立 `codex/session-remember-lifetime` worktree，唯一目标为 P3/M0、AC23 的保持登录子实验。默认关闭，主动选择后为 24 小时空闲/7 天绝对期限及持久化 Cookie；普通登录保持 2 小时/12 小时且 Cookie 不持久化。每请求复制配置，服务端载荷决定续期策略，活动参数不能改变选择、带 Cookie 再登录拒绝；两模式共用三名额。保存、结果核验及放弃共用期限上界，覆盖 7 天候选原样重试与原期限墓碑。

Docker Desktop Linux arm64、Go 1.26.10、Redis 7.2.4、PostgreSQL 14.24：原三十四项行为/race 基线通过（31.177 秒）；最终四十二项通过（41.465 秒，另含三十一个子用例），格式、模块校验、vet 通过。基础工具镜像没有 golangci-lint，使用已有 `oj-go-quality-toolchain` 执行，零问题；无安装或下载。仓库策略 195 文件与完整差异检查通过。Astra 针对性复核发现并补齐候选核验/放弃旧 12 小时上界及混合并发覆盖，主代理完成实际验证与完整自审；AI 审查不冒充独立人工审批。

默认期限以实际 Redis 字段验证，流逝用缩短期限和真实 Redis 时钟验证轮询、活动、空闲/绝对到期、迟到保存及名额释放；没有实际等待 24 小时/7 天。没有新依赖、SQL、迁移、驱动或生产端点，回退本项夹具及测试即可。仅同进程双 TLS API，浏览器关闭/重开、管理/参赛隔离、轮换、跨进程长会话及元数据容量仍待验收，完整 M0 未通过、未部署。最终 PR/head/CI/合并回读见远端与知识库。

## 2026-10-30：普通会话空闲与绝对期限

从成功 fetch 后的 `4a62b15` 创建独立 `codex/session-lifetime` worktree，唯一目标为 P3/M0、AC23 的普通会话期限子实验。测试处理器增加显式 POST 活动探针，原 `/me` 继续只读；默认 2 小时空闲/12 小时绝对期限不变。缩短配置后通过真实 Redis 时钟与两 TLS API 验证轮询不续期、活动跨越原空闲边界且准确截断到原绝对期限，以及已加载请求跨过空闲/绝对到期后保存拒绝、无 Cookie、无记录/期限改写。核对单名额分数、索引/记录期限、到期清理和准确三名额。

Docker Desktop Linux arm64、Go 1.26.10、Redis 7.2.4、PostgreSQL 14.24：原三十项行为/race 基线通过（20.828 秒）；最终三十四项通过（30.829 秒，另含二十六个子用例），格式、模块校验、vet 与静态检查零问题，仓库策略 194 文件通过。Astra 针对性复核及主代理完整差异自审/实测不冒充独立人工审批。既有恢复、OOM、结果不明回归继续通过。

无新依赖、SQL、驱动或生产接口，回退新增测试与探针即可。仅同进程双 API、缩短期限和受信探针，未定义浏览器有效活动识别、采样/限频、到期提醒或 Gin/CSRF；保持登录、管理/参赛配置、凭据轮换及跨进程期限仍未验收。完整 M0 未通过、未部署。最终 PR/head/CI/合并回读见远端与知识库。

## 2026-10-30：数据库回档后的会话恢复标识

从成功 fetch 后的 `2fba7cf` 创建独立 `codex/session-database-restore` worktree，唯一目标为 P3/M0、AC23 的数据库回档旧会话隔离子项。在驱动一次性 PostgreSQL 的随机专用 schema 中真实 pg_dump/pg_restore，先复现整数代次重复：已撤销 Cookie 返回 200、旧恢复完成操作误开放；再以 crypto/rand.Text 新标识替代测试全局整数，账号代次保持整数。

旧两个 TLS API/连接池停止后恢复；确认数据库回到备份、Redis 六个载荷/索引键和进程未变，再写新标识/关闭入口并重建两个 API。旧完成操作拒绝，三阶段旧 Cookie 均 401，恰好接纳三个新会话。Docker Desktop Linux arm64、Go 1.26.10、Redis 7.2.4、PostgreSQL/工具 14.24：原二十九项行为/race 基线通过（20.645 秒），最终三十项通过（20.582 秒，另含二十四个子用例），格式、模块校验、vet 与静态检查通过。首次空 PGSERVICE 被 libpq 当作服务名，改为清除子进程继承设置、显式参数连接后完成；未弱化断言。Astra 针对性复核和主代理实测/自审不冒充独立人工批准。

无生产接口、新依赖或生产迁移。测试全局字段 bigint→text，旧整数 Gob 被拒绝，旧夹具不原地迁移；复现使用全新私有集群。仅本次可信 schema 归档/恢复；不是全库/PITR、数据库/API OS 进程或跨主机恢复。外部独占维护和全部旧端点隔离仍必要，CAS 不能协调并发 restore，也不能修复回档的密码/封禁/权限。回退本项夹具和文档即可；完整 M0 未通过、未部署。最终 PR/head/CI/合并回读见远端与知识库。

## 2026-10-30：会话保存结果不明的核验与放弃

从成功 fetch 后的 `7253bb8` 建立独立 `codex/session-admission-outcome` worktree，唯一目标为 P3/M0、AC23 的保存结果不明原语。冻结 SCS 服务端候选，适配器层在真实 Redis 保存后丢弃回执或暂不执行保存，HTTP 均 503/无 Cookie；另一副本核验完整会话/名额，十次双副本并发原样重试不增加占位或延长期限。已有或尚未到达的保存都可用原绝对期限墓碑放弃，阻止迟到创建；共享身份解析与双代次锁序，夹具在启动前配置。

Docker Desktop Linux arm64、Go 1.26.10、Redis 7.2.4、PostgreSQL 14.24：原二十二项行为/race 基线通过（20.037 秒）；最终二十九项行为/race 通过（20.281 秒，另含二十二个子用例），格式、模块校验、vet、静态检查通过。覆盖缺失只表未知、索引/载荷/期限冲突、合法较长索引 TTL、活跃名额/过期残留、退出/代次变化、OOM/客户端关闭及过期原尝试。Astra 针对性复核与主代理自审/实测不冒充独立人工批准。

无生产入口、依赖、驱动或 SQL 迁移变化，回退本项测试与必要夹具配套即可。故障是适配器层受控注入，不是网络丢包或进程崩溃；候选仅在进程内持有，未交付正式跨请求协议、核验后发 Cookie、自动补偿/崩溃恢复、元数据容量或完整 AC23/M0，未部署。最终 PR/head/CI/合并回读见远端与知识库记录。

## 2026-10-30：Redis noeviction 超限会话写入

从成功 fetch 后的 `5666acc` 创建独立 `codex/session-redis-memory` worktree，唯一目标为 P3/M0、AC23 的入口内存超限与恢复子项。真实 Redis 临时限制为 1 字节，确认普通 SET 返回 OOM、无键驱逐；先复现原保存 Lua 仍让登录返回 201（满三名额时 409），再从首字节声明 `#!lua`，使入口超限统一 503 且无 Cookie。仅改测试适配器和新增故障回归，无生产入口、迁移、依赖或驱动改动。

Docker Desktop Linux arm64、Go 1.26.10、Redis 7.2.4、PostgreSQL 14.24：原十八项行为/race 基线通过（19.638 秒）；最终二十二项行为/race 通过（19.628 秒，另含九个子用例），格式、模块校验、vet 和静态检查通过。比较完整键集合、Hash 字段、ZSET 成员/分数与绝对到期，验证失败写入无占位/孤立载荷；超限期间读取/退出、双代次及恢复后三名额正确。配置查询初次与锁定客户端接口不符，在进入行为测试前修正；无跳过断言。Astra 针对性设计/代码复核与主代理完整差异自审不等同独立人工批准。

只验证入口已超限，未证明从阈值下开始的脚本永不越限、运行错误回滚、OOM kill、Cluster 或负载准入；当前只读 `/me` 不自动续期。正式登录结果不明处置、数据库备份回退、Gin/CSRF 等仍未验收，M0 未通过、未部署。回退声明、测试与文档即可。最终 PR/head/CI/合并回读另记于远端及知识库。

## 2026-10-30：全局会话代次与 Redis 旧快照恢复

从成功 fetch 后的 `3666874` 建立独立 `codex/session-snapshot-recovery` worktree。唯一目标为 P3/M0、AC23 的旧快照恢复子项：测试专用 PostgreSQL 全局代次/门禁、双代次读取与保存、条件完成防旧恢复误开门禁；独立恢复 Redis 显式 SAVE 后撤销会话，再 SIGKILL/重启加载旧 RDB。核对进程更换、旧载荷/索引确实返回及 TTL 未延长，双 API 拒绝旧会话，恢复后三名额成立；屏障与真实锁超时覆盖迟到操作及恢复顺序。无正式入口、生产迁移或新依赖。

Docker Desktop Linux arm64、Go 1.26.10、Redis 7.2.4、PostgreSQL 14.24：原十四项 race 基线通过（18.980 秒），最终十八项行为/race 通过（19.811 秒，另含四个子用例），格式、模块校验、vet 通过；46 项治理回归通过。首次 DUMP 字节比较失败，改为语义字段/索引及绝对到期时刻断言后通过；Astra 指出并修正夹具两次取时的毫秒边界。默认本地工具镜像缺少 golangci-lint，使用已有质量工具镜像执行项目静态检查，无下载或关闭验证。

已完成针对性 Astra 复核及完整差异自审；AI 复核不代表独立人工审批。数据库备份回退、故障自动检测/协调、AOF/Sentinel、内存不足、跨主机、完整期限/Gin/CSRF/正式登录结果不明处置和 PostgreSQL 会话对比仍未验收；M0 未通过，未部署。回退本项测试、驱动与文档即可。远端 PR、最新 head CI 与最终合并状态以本任务回读和知识库记录为准。

## 2026-10-29：文档合并与当前版本对齐

从成功 fetch 后的 `2401479` 建立独立任务分支。将交付基线的组件引入时机、发布阶段及验收归属并入开发与交付计划，删除原文件并更新引用；清除初始化授权、过时目录和重复进度/命令。README 保留运行入口，文档索引明确维护归属，M0 记录集中当前证据，旧实验保留原日期与版本。协议按代码的 `PinnedVersion` 修正为当前 v3，修正已存在 pnpm/锁文件/CI 和真实判题证据却仍被描述为未实现的条目。

仓库策略与 `git diff --check` 已通过，已审查合并内容与相关配置、代码及证据入口；本次仅改文档，未重跑应用或特权 Worker 实验，未部署，不改变需求或 M0 验收状态。远端 CI 与最终合并状态以本任务 PR 回读为准。

## 2026-10-29：PostgreSQL 账号认证代次实验

从成功 fetch 后的 `662cb8f` 创建 `codex/session-auth-generation`，唯一目标为 P3/M0、AC23 的账号认证代次子项。测试专用 PostgreSQL 账号夹具、同进程两个 TLS API/独立连接池验证逐次核验、批量失效/禁用/再启用、迟到登录/保存拒绝及新代次三名额。共享行锁协调 Redis 保存与代次更新，真实锁超时用例验证等待顺序；不声称跨存储原子事务或即时撤权 SLA。

Docker Desktop Linux arm64：`python3 scripts/test-session.py --module-cache /private/tmp/oj-session-mod` 的原十项基线通过（18.621 秒），最终十四项行为/race 通过（19.032 秒，另含两个子用例），格式、模块校验、vet 通过；本地已有 `oj-go-quality-toolchain` 内 golangci-lint 零告警。实验模块复用主模块 pgx v5.11.0 及依赖锁定版本，首次离线 tidy 因缺少校验和/传递模块缓存失败，随后按既有宿主 TLS/checksum 缓存准备流程完成；没有禁用验证。驱动增加容器内私有 PostgreSQL，未触及用户数据库。

没有正式入口或生产迁移；回退本项测试、驱动和独立模块依赖即可。数据库客户端池失败仅证明拒绝认证，不是数据库宕机；全局灾备代次、Redis 旧快照、数据库备份回退、正式账号/权限和完整 M0 未验收，未部署。远端 CI 不执行此独立模块行为套件，仅覆盖静态分析；最终 PR/head/CI/合并状态见远端及知识库索引。

## 2026-10-29：前端类型感知质量门禁

前置 Go 质量检查 [PR #33](https://github.com/DL-Association-for-Computing-Machinery/OnlineJudge/pull/33) 的 head `4101269` 九项 CI 全成功，完整差异/讨论核验后 squash 合并为 `9bc0cab`。从成功 fetch 后的该主线创建独立 `codex/frontend-quality`，唯一目标为启用已有 typescript-eslint 的类型感知推荐规则，覆盖 src TS/TSX 与 Vite 配置；无新增依赖、业务/UI/迁移变化。

Docker Desktop `frontend-check`：真实源码 lint 零告警、六项真实 ESLint 正反例（含 TSX/Vite）、TypeScript 检查、六项业务测试及生产构建全部通过。初次将 Node 自测命名为 `*.test.mjs` 导致被 Vitest 重复发现并失败，改为 `lint-selftest.mjs` 后完整复验通过，未修改测试发现规则或跳过断言。repository policy 和完整 diff 自审通过。

类型感知检查不替代运行时输入校验，void 不等于捕获拒绝；未引入覆盖率/fuzz 或外部质量平台。本项无 UI 视觉变化，未进行额外截图、未部署；最终 PR/head/CI/合并回读见远端与知识库。

## 2026-10-29：Go 静态质量门禁

从成功 fetch 后的 `3b4a96c` 创建 `codex/go-quality`，唯一目标为引入固定 golangci-lint v2.15.0 的 staticcheck/errcheck/ineffassign/unused。本地与 CI 共用后端入口，覆盖主模块、sandbox 源码及独立会话模块；运行镜像不包含检查工具。基线共 87 条告警已核对处理，主要为显式资源清理/故障注入返回值，另加强锁子进程退出和响应读取断言、简化等价表达式；没有全局排除或降低现有门禁。

Docker Desktop Linux arm64：`toolchain backend` 的格式、vet、四规则真实正反例、三范围静态扫描（各零告警）、主模块 race 与全部命令构建通过。`test-session.py --toolchain-image oj-go-quality-toolchain:latest --module-cache /private/tmp/oj-session-mod` 的十项真实 Redis 行为/race 通过（18.721 秒）。治理 46 项及 repository policy 通过；治理首次在受限沙箱因 loopback bind 权限失败，获得工具权限后完整通过。

未重跑真实 Worker 沙箱专项，本轮 sandbox 证据仅为静态分析；未增加覆盖率/fuzz、未部署、未完成 M0。回退本项配置/入口和对应基线改动即可。此为提交前验证快照，最终 PR/head/CI/合并状态见远端及知识库交付索引。

## 2026-10-29：Redis 服务端超时与恢复

唯一目标：补齐 P3/M0、AC23 的会话存储超时子项。从成功 fetch 后的 `f672cf4` 创建独立 `codex/session-redis-timeout`；新增三个 `_test.go` 用例，暂停一次性 Redis 的命令处理，验证读取、退出、已加载保存及登录返回有界 503 且无 Cookie，恢复后会话期限、撤销及三名额约束正确。无正式入口、依赖、驱动或迁移变化，回退新增测试及文档即可。

`python3 scripts/test-session.py --module-cache /private/tmp/oj-session-mod`：Docker Desktop Linux arm64、Go 1.26.10、Redis 7.2.4；原七项 race 基线通过（3.393 秒），最终十项行为/race 通过（18.562 秒），格式、模块校验、vet 同时通过。登录超时后本次观察到零个遗留占位；测试允许结果不明时零或一个，验证总名额不超三，不将超时等同回滚。正式登录仍需结果核验和遗留名额处置。

新增故障用例为同进程两个 API，Redis 暂停后恢复但不重启/恢复快照；认证代次、旧快照恢复、内存不足、跨主机及完整 AC23/M0 未验收，未部署。当前 CI 不运行嵌套会话模块，其行为证据来自本地专项。此段为提交前快照，最终 PR/head/CI/合并回读见远端交付与知识库索引。

## 2026-10-29：独立 API 进程的会话撤销与重启

唯一目标：补齐 P3/M0、AC23 会话实验的进程边界证据。从成功 fetch 后的 `e424392` 创建独立 worktree/分支 `codex/session-process-recovery`。只扩展 `_test.go` 夹具及文档，复用原有 SCS/Redis 适配器，无正式 API、依赖、迁移或产品规则变化；回退本项测试与文档即可。

Docker Desktop Linux arm64、Go 1.26.10、Redis 7.2.4：`python3 scripts/test-session.py --module-cache /private/tmp/oj-session-mod` 的原六项 race 基线通过（1.306 秒），新增后七项行为测试通过（3.405 秒），gofmt、go mod verify、vet 同时通过。两个独立 API 进程的 24 并发只接纳 3 个会话；屏障证明撤销后的迟到保存被拒绝；撤销方 SIGKILL 并重新启动后，旧会话仍无效、两个有效会话仍可读取且恰好补一个名额。子进程入口的父进程跳过不代表该场景未运行，实际由进程用例启动并校验 PID/退出状态。

Redis 全程存活；未证明 Redis 宕机/持久化、旧快照恢复、认证代次、跨主机高可用或完整 AC23/M0，未部署。当前 CI 仍不执行嵌套会话模块，本项专项证据来自上述本地 Docker 实测；未重跑特权 Worker 或无关应用套件。下一项为认证代次与 Redis 恢复验证。此段为提交前快照，远端交付以最终 PR/head/CI 回读为准。

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
