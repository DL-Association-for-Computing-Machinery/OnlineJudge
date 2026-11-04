# 参考验证的停止故障调查

本页正文保留2026-11-04早期旧suffixarray及staticrmq故障调查的历史时点；其中“当前”“#114继续草稿”“#119继续草稿”等均指相应调查阶段。根因尚未确定，旧失败不追认。本页不是故障已修复、完整P4通过或生产稳定性证明。

后续状态：#114已合并为`b20900b2567f959879657d18a196dd6aba86b41a`；staticrmq版本HEAD `e777ae2b985193ddaf1ec8d647d8697aa13c5964`吸收该前置与#123后，通过一次九来源201点及24个原子子测试的固定环境接纳，全部AC/Stop、旧完整Plan/Profile相等。最终CI与合并状态见[PR #119](https://github.com/DL-Association-for-Computing-Machinery/OnlineJudge/pull/119)，新证据见[九来源接纳记录](delivery-log.md#2026-11-04staticrmq-九来源固定环境接纳)；当前合同见[参考验证](reference-validation.md)。新成功不解释旧阻塞根因，下面各历史阶段结论保持原时点。

## 原始故障与新运行

旧报告`/private/tmp/oj-point-add-suffixarray-regression-20261104/report.json` SHA-256为`19513bf823e8cd40bf5ba13fedcc5f2a3e9f6f073cc843246568ee2f2e14eb36`：前10点AC，第11点SE且无usage，其余39点NOT_RUN，Stop为kill/deadline、observed0/confirmed0。当时原容器仍运行，外部停止原完整CID及受控恢复不改变这个失败结论。此报告没有后续新增的执行/停止步骤诊断，不能反推其字段。

本次从成功fetch的`origin/main 61aaa4bb44889d1c0f5e8d20c7fda2048011f72f`建立独立调查分支。已有离线Docker Go1.26.10四包race基线通过，独立构建Darwin CLI/driver，核对精确测试清单、构建前后工作树及二进制哈希。固定medium原CID、镜像、owner、配置、停止/PID0、私有空canonical锁及无旧控制操作均先核验；真正运行仍由原owner持锁。没有修改应用代码、来源、预算、重试或Stop门禁。

| 运行 | 结果 | 范围与限制 |
| --- | --- | --- |
| 新主线独立suffixarray | 25.27秒，全50点AC、停止及清锁确认 | 一次新目录运行，未重复旧前置序列 |
| 预注册顺序前置 | 15.60秒，旧point_add完整21点及大stdin两组通过 | 原旧worker/driver成对匹配故障留存哈希，原工作区未修改；前置失败即终止协议 |
| 顺序目标suffixarray | 24.69秒，全50点AC、停止及清锁确认 | 前置进程结束至目标启动约0.058707秒（含安全核验）；未宣称与旧运行间隔相等 |

两次suffixarray均与旧报告的完整plan/profile逐字段一致，plan摘要`0ac8ba8b1fd52f7027ff401fdb495eb9f81fea9d2bbd3f1c0c752de24054b5c6`、profile摘要`0082bb2f961c1f1c082c0ed671881bca49cdaae72d9587bb76d8424ff5e661e7`保持。报告目录0700、文件0600、无INCOMPLETE，成功省略两类诊断，publication仍blocked；没有恢复后续跑、手清marker或增加尝试次数。新成功只说明各自预定条件下未复现，不能证明前置序列无影响或解除#114。

## 旧服务日志中的新证据

只读取原CID固定UTC窗口`2026-11-03T17:02:19Z`至`17:03:23Z`。每次采集限15秒、256KiB及1000行；保留结构化时间、状态、归一化路由与首次出现的对象序号，不保存原日志、缓存ID、参数、材料或原始错误。序号仅在该窗口关联对象，不能用作删除权限。旧窗口观察到72条HTTP访问记录，全部状态200；这不表示客户端收到完整响应或参考结果AC。

按固定调用顺序，第67—70条依次对应第11点的输入上传/回读、控制请求和输出读取。第71条DELETE与第70条GET指向同一对象，因此流程至少到达该点stdout对象的清理。由于[缓存适配器](../internal/execution/cached_gojudge.go)即使前面失败也执行deferred cleanup，仍不能把旧SE改写为“Run成功、仅清理失败”。

| 访问序号 | 已识别调用 | 服务端记录时间 UTC | Docker记录时间 UTC | 服务端handler latency |
| --- | --- | --- | --- | --- |
| 69 | POST /run | 17:02:25.975267 | 17:02:25.975389968 | 0.056723791秒 |
| 70 | GET stdout对象 | 17:02:26.040498 | 17:02:26.040581927 | 0.000988333秒 |
| 71 | DELETE同一stdout对象 | 17:02:26.114674 | 17:02:49.901981799 | 0.000252秒 |
| 72 | DELETE另一对象，角色未确认 | 17:02:49.943167 | 17:02:49.943318299 | 0.000070083秒 |

第71条两个记录时间相差约23.787307秒；其他旧记录差值约53—299微秒。旧CLI错误日志的时间为17:02:46.094267Z。缺少该点输入DELETE记录不证明未尝试；第72条也不能仅凭顺序断定为最终exe，更不能把较晚服务记录等同于客户端收到回执。

对两次新成功运行使用相同投影，各观察到307条HTTP记录，最大记录时间差分别约0.000446秒与0.000509秒。顺序目标窗口从其实际StartedAt开始，排除前置生命周期的尾部记录。另读更早A+B停止失败窗口得到空日志，仅表示未取得该窗口证据，不用于支持或反驳原因。0.5秒只用于列出时间差观察值，不是通过条件或预算。

从原停止容器只读复制的go-judge二进制构建信息核对为v1.13.3，依赖gin-contrib/zap v1.1.9及zap v1.28.0；日志中的Release为true。按对应[ginzap源码](https://raw.githubusercontent.com/gin-contrib/zap/v1.1.9/zap.go)，handler返回后才计算latency并调用logger；[zap生产编码](https://raw.githubusercontent.com/uber-go/zap/v1.28.0/config.go)以Unix秒记录ts、以秒记录Duration，[ioCore](https://raw.githubusercontent.com/uber-go/zap/v1.28.0/zapcore/core.go)同步调用输出Write。因此handler latency不包含随后日志调用及客户端等待的全部时间。构建信息中的vcs revision为`4ebd2ffe9474639c665fa21cfb438f68820fe77c`，本次未从该commit URL取得源码，不宣称重建了整个二进制。

## 能说明什么与下一步

当前证据把调查重点收窄到stdout清理附近、服务端handler计时之外的等待。23.787秒尚未分解为logger内等待、调度暂停、日志收集延迟或时钟变化；不能定性为日志背压、Docker daemon阻塞或VM暂停，也不能证明旧kill已派发。停止后的OOMKilled=false不能排除更广泛的瞬时资源压力。

#114相对旧main的四处生产差异新增Bulk预算/point_add来源及路由，suffixarray仍选择Medium，没有直接修改Cached I/O或Stop。结合plan/profile一致性，可排除所比较的材料计划和声明预算变化；不能排除旧二进制执行差异、瞬时环境或前置执行影响。两种新条件均未复现后停止实测，不以重复运行代替定位。

下一次有明确触发的调查预注册为：“stdout DELETE客户端完成失败，而该请求服务端handler已短时间返回。”使用新execution_diagnostic、Stop trace和同格式服务日志关联；需要补观测时，只记录有界固定角色/类别、单调begin/end与context状态，先独立复核设计，不关闭日志、改日志驱动或放宽期限。delete/stdout超时、短handler耗时与异常记录时间差同时出现，仅支持该狭窄位置假设；失败在此前阶段或handler自身长耗时则不支持；全部通过仍为未复现，不追加次数。前置序列不是唯一未决因素。

另一独立候选[草稿#119](https://github.com/DL-Association-for-Computing-Machinery/OnlineJudge/pull/119)的staticrmq完整27点单次验证，在第9点control失败且两个独立cleanup deadline，Stop已确认；原报告SHA `ee524a329fbfcc5d654e3ff761ce310dc0f1dee43b089dfa3bb5332a816d2d69`。该点input4035B，输入回读与后续run完成日志相隔约24.453秒，而handler计时19.172ms；service/Docker时间差最大约0.439ms，没有本页旧窗口的23.787秒形态。两次失败阶段不同，不能直接等同或排除共同上层原因，也不能把HTTP200追认为客户端完成。完整候选证据仍归其草稿交付日志；主线来源准入不扩大。

现有[Host分段失败证据](reference-validation.md#执行失败诊断)补充RoundTrip的admission/prepare/inspect/command/response_validation、最后完成位置、阶段及整体耗时、context/剩余期限与本地CLI启动证据。确定性单测区分命令未启动/启动后超时、阶段冻结与收尾等待，并验证跨层独立错误归属；不据此断定旧根因。本次只执行受信本地命令及一次既有A+B兼容回归，没有重跑suffixarray或staticrmq。后续若调查#119，应另行登记可检验假设与一次执行条件，再结合具体point/role/host_trace和同窗口脱敏服务日志判断，不把新增诊断或新通过当作自动解除草稿的依据。

## staticrmq单次Host边界观测

在主线Host诊断经[PR #120](https://github.com/DL-Association-for-Computing-Machinery/OnlineJudge/pull/120)交付后，复用#119原分支普通merge为`a8aa5fc135b345d35658ce88866ccad93b7a41fa`，没有改写推送历史或修改候选执行代码。旧19文件中的Go源码仅接受主线测试keyed literal转换，新增Host代码与main逐字节一致。完整离线backend/race/静态/七Linux命令、sandbox vet及独立Darwin构建通过；原失败制品和报告先逐哈希复核，完整57对象200820493B仍与旧材料身份一致。

本次先登记狭窄H1：“同类control/stdout/deadline有真实CLI启动证据，Host在command返回失败，未进入response_validation；进入该阶段采样时context active且剩余期限为正。”采样和实际调用/启动不是原子事件，`completed=false`不重建deadline到达瞬间；即使支持，也不证明daemon收件、沙箱开始执行或服务端根因。其他明确失败阶段/未启动不支持，未知或受限trace不猜测。全通过只记未复现，不追加次数、不解除旧草稿。

协议保留原完整顶层两组与所有预算/期限，精确test清单先验。外置编排复核发现日志创建在启动之后、收尾失败可返回0及旧Docker命令检查缺口，均在执行前修正；原v1登记未执行并保留，v2于`2026-11-03T20:42:32.637157Z`登记，SHA `1b5995a1274ea83762a45ec0df954d17ebb7759df1f8a410027309df343c2293`。必要新日志/结果文件在进程前独占创建，日志写失败继续drain，收尾或证据失败非零退出；一次性标记和新输出目录阻止重复启动。Astra针对性复核修正后无阻断，AI不替代人工审批。

固定原medium CID/镜像/owner/完整配置、三槽停止/PID0、私有空锁无占用、无旧native及固定槽Docker控制进程均通过前置核验。一次双组19.83秒通过：可信16MiB stdin2.57秒全字节/长度核验，完整27点CLI17.25秒全AC/Stop。新报告`/private/tmp/oj-staticrmq-host-observation-report-20261104/report.json` SHA `3d248ead0bd03e47853c7d53fae6a9ceb5d7fd4c73491349875c913fd012b55c`；完整plan/profile与旧失败报告逐字段相等，摘要仍为`0d2beed5a725d367ebcd464591e83690e74e1e2371b6702a1a03dafd4196adf7`及`f6e3875942ca1b4b57a17124ef758cb9ed922e715a624336b6d19191e8e6d946`。0700/0600、无INCOMPLETE、成功两类诊断省略，publication仍blocked。结束时三原槽停止/PID0、私有空锁无占用，无额外尝试、恢复或手清marker。

两生命周期分别投影日志：前置窗口从宿主运行开始UTC至目标StartedAt，前置实际StartedAt未知；目标使用收尾inspect的StartedAt `20:43:19.412924422Z`至FinishedAt `20:43:35.72820843Z`。`===RUN`时间只是宿主观察到行的时刻，不能当精确子组开始时间。每窗口一次15秒/256KiB/1000行封顶，实际前置6296B/13条HTTP、目标46096B/169条HTTP，均200且未达到采集上限；最大service/Docker时间差约0.293ms/0.335ms，目标最大handler计时0.211944秒。只保存归一化投影与流哈希，不保留raw日志/缓存ID；前置窗口不证明准确生命周期边界，HTTP200也不代替客户端结果。

本条件下没有失败条目，H1既未获得支持也未被反驳；成功省略Host trace符合合同。原`ee524a…`报告保持，旧失败没有被解释，新成功不构成故障修复或稳定性证明。#119与#114继续草稿，旧五来源真实回归未启动，没有新增准入；后续不为获得失败或绿色追加相同试验，有新的独立触发和可检验假设再登记。临时证据前缀`/private/tmp/oj-staticrmq-host-observation-`包含协议/前后置/运行/报告证明及两窗口投影，临时归档不是独立备份。

## 证据索引

- 独立报告`/private/tmp/oj-suffixarray-diagnostic-report-20261104/report.json`，SHA-256 `3671e312db6e664e5fe474469964e8b07abd3c3c252f8e32f937211090a4bb20`。
- 顺序前置报告`/private/tmp/oj-suffixarray-sequence-point-add-report-20261104/report.json`，SHA-256 `2e7bd3cb655d82161ecdf6b3ac616f25181d6336d36cc7394293b4a6c4eb5232`；目标报告`/private/tmp/oj-suffixarray-sequence-diagnostic-report-20261104/report.json`，SHA-256 `5cd41916524e8bfc26341744a473192882676139503fb1474ba5af89962466c6`。
- `/private/tmp/oj-suffixarray-investigation-{baseline-build,real}.log`、`oj-suffixarray-investigation-{preflight,run,postflight}.json`；顺序协议/结果为`oj-suffixarray-sequence-{protocol,results}.json`，两阶段日志为`oj-suffixarray-sequence-{prefix,target}.log`。
- 最终同格式投影为`oj-suffixarray-investigation-{old-suffixarray,current-suffixarray,sequence-target}-timeline.json`，汇总为`oj-suffixarray-investigation-timestamp-comparison{,-sequence}.json`；早期结构/宽窗口草稿不作为本页统计依据。
- 二进制构建信息`oj-suffixarray-investigation-gojudge-buildinfo.txt`，原旧18项证据哈希仍与`oj-point-add-evidence.json`相符。临时采集/编排脚本只在`/private/tmp`保存，不进入应用或产品入口。

临时证据不是独立备份。此调查未修改原始草稿not-run、生产null、publication blocked；未验收持久化验证任务、完整质量、非作者终审、题目发布或整体P4，未部署。当前限制继续归[维护合同](reference-validation.md#槽所有权与停止)。
