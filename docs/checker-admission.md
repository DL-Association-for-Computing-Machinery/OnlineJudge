# 首批 Library Checker 文本检查器组件准入

本页记录 P4 的文本语义组件。`internal/judging.CompareLibraryCheckerText` 和冻结材料入口 `CompareLibraryCheckerTextAt` 使用同一版本 `lc-wcmp-tokens-v1`；现有 M0 `CompareText`、Worker 固定材料门禁和草稿格式不变。冻结材料入口供[受限参考验证](reference-validation.md)使用，尚未接入生产 Worker；不执行 Testlib，不解除任何发布阻断。

## 支持域与错误

固定[来源锁](../config/problem-sources/library-checker.json)的十份 `checker.cpp` 均为相同的大小写敏感 `wcmp`，并非整数数值比较。本站新模式仅以 SP/TAB/LF/CR 分隔 token，逐字节比较，不做数字或 Unicode 规范化。VT/FF 是 token 内容；M0 仍把它们当空白。`01` 与 `1`、`YES` 与 `yes` 均不等价。

调用方必须显式提供 `TextLimits{Bytes, TokenBytes}`，分别约束每份答案/输出和单 token。配置须满足 `1 <= TokenBytes <= min(Bytes, 65536)`、`Bytes <= 128 MiB`；零值、负值及超界为 `ErrTextLimits`，不隐式扩容或裁剪。完整参考答案先验证，再考虑输出错误或失配；非法 UTF-8、参考文件/参考 token 超限为 `ErrReference`，输出文件或有效 UTF-8 token 超限为 `ErrOutput`，非法 UTF-8 输出拒绝为失配。这些错误不能直接归为 WA，未来执行适配须分别处理参考异常与选手输出超限。

支持域有意比锁内 Testlib 窄：只接受有效 UTF-8，单 token 至多 64 KiB；上游可以接受部分非法编码和更长 token，且原始 `0xFF` 有 EOF 特殊语义。不得声明与上游任意字节全域等价。128 MiB 是组件硬上限，差分实验显式使用每文件 16 MiB/每 token 64 KiB；二者都不是生产选手输出、内存、运行时间配置。

切片入口仍接收已分配的 `[]byte`。冻结材料入口接收声明长度的只读 `ReaderAt`，调用者负责在所有扫描期间保有不可变材料。读取严格限定在声明区间；短读、底层故障或数据在两遍之间变得不可读属于系统错误 `ErrTextRead`，不能作为 WA。完整答案校验先于读取输出，输出先检查声明字节数，再扫描完整 UTF-8 和 token 限额，最后比较有序 token；辅助空间最多两个 16 KiB 读缓冲及两个有界 token。读前后检查取消，底层阻塞 I/O 仍须调用者提供期限。

参考 Runner 使用冻结文件上的受限答案视图，不再整份分配答案；视图受原快照关闭和上下文约束。输入上传、执行 stdout、存储缓存、进程隔离和总并发预算尚未迁移，不能用检查器辅助内存证明整条 Worker 链路或目标节点容量。

## 独立差分范围

显式 `checkercompat` 构建标签下的[测试](../tests/checkercompat/library_checker_test.go)从锁定 `hash.json` 独立核验全部输入/答案，对十份 checker 和库头文件核验锁内哈希；不用来源准备的 `validation.json` 作为本站运行通过证据。先全量检查 254 份答案均在声明支持域内，再在专用容器内编译锁定 `wcmp`，对每份答案执行自身相同、添加四种空白、额外 token、空输出四种对照。额外覆盖 VT/FF、大小写、数字拼写、Unicode、NUL 和已知域外差异；上游非 0/1 返回码不冒充 AC/WA。

只为同源 checker 家族复用一次编译，若十份源码不再相同则拒绝，必须重新拆分准入。套件先核对固定提交；锁摘要、检查器版本、覆盖数量、最大答案、超 M0 预算数量和显式限额写入测试日志。每组同时比较切片入口、真实文件的冻结入口和锁定原 checker。单元回归另覆盖字节/token/UTF-8 跨缓冲、短读、取消、完整参考优先级及原 M0 行为。

完整运行命令归 [README](../README.md#首批文本检查器差分)，只允许显式本地隔离 Docker 专项；普通 CI 不执行第三方程序。固定工具链、实际测量及本次结果归[交付记录](delivery-log.md)。固定测试集合的一致不证明所有支持域输入等价，也不构成恶意题源隔离认证。

## 生产检查器候选预检

离线维护命令 `checker-candidate` 接受固定来源锁、私有题包、独立取得的正式计划 SHA-256，以及显式的检查器版本和两项限额。计划摘要从已接纳的正式参考记录取得，不能以待检查题包自报的值作为信任依据。命令重新校验并冻结有序材料，匹配计划后逐点验证答案的字节数、UTF-8 和 token 限额。答案异常、读取失败、计划不匹配或取消时拒绝生成候选。读取并核对题包元数据之后，冻结与答案预检有两分钟期限。

成功输出 `candidate` 和 `sha256`。候选绑定检查器版本、两项限额、来源、内容版本、导入修订、来源锁、完整有序计划和答案数；不含私有材料。摘要是 `oj.checker-candidate.v1` 候选对象按结构字段顺序编码成的紧凑 Go JSON 的 SHA-256，不含外层对象或换行。相同绑定与限额可重现同一摘要；改变任一限额或材料须重新预检。摘要用于固定后续验收所用的配置，不是签名，也不代表已经准入。

## 候选检查器原始输出试跑

独立维护入口 `validate-checker-candidate` 从受信计划、候选摘要和固定来源锁重新校验 A+B、并查集或 Z 算法草稿。预检和执行共用有界、私有的冻结快照；不匹配在启动前拒绝。A+B 保留原内存快照和档位；并查集与 Z 算法分别复用既有磁盘材料预算及小型、medium 参考槽，采集上限分别为 256 KiB 与 4 MiB。Z 算法与其他 medium 来源使用同一把宿主锁。冻结阶段超时与冻结后的生命周期分开；执行者负责在退出时释放快照。原始缓存 stdout 只走独立参考槽，运行时和检查器的字节限额必须相等。档位摘要覆盖执行环境、文件预算、材料预算和检查器限额；不接受调用者换宿主、镜像、执行档位或隐式扩大预算。

完整正常退出的原始输出按冻结答案比较，得到 AC 或 WA；完整且在字节上限内的输出若有效 token 超限则为 OLE。字节超限只接受已核验的采集回执（精确状态和同名错误、正常退出、唯一 stdout 采集错误、原始 cap+1 字节、完整角色和用量），或符合固定协议的文件大小终止回执。截断前缀不参与比较。原始输出超长但执行器声称正常、未知回执、答案异常、取消或清理失败均不作为选手错误。M0 与原维护参考验证的判定规则不变。

独立的 `oj.checker-trial.v1` 私有报告记录候选、计划和运行档位摘要、逐点结果、停止确认以及完成状态。运行、制品清理、独立 Stop 和锁标记清理都无错误才记完成；完成的 WA/OLE 等结果不代表候选通过。启动失败可能仅留下 `INCOMPLETE`。报告不会进入草稿审核或发布记录。操作入口见 [README](../README.md#候选检查器原始输出试跑)。

### 归档报告离线复核

`checker-candidate verify-trial` 从固定锁和题包重新冻结同一来源的候选、正式计划及试跑档位。命令要求独立可信的原始报告、候选和计划 SHA-256，显式提供原候选的两项限额，只读已完成且无人写入的归档目录。目录中仍有 `INCOMPLETE`、报告不是普通文件、超过 64 KiB、字节摘要或写入格式不符时拒绝。

只有固定槽、无异常诊断、完成且记录停止确认，并且全部正式点按计划顺序 AC、编译与运行用量均存在且在各自限额内时，才输出 `oj.checker-trial-verification.v1` 摘要。它确认的是归档报告与独立绑定一致；摘要本身不是签名，报告也不能证明当前槽已停止或目标环境已验收。命令不改报告、不调用 Docker、不写数据库或审核状态。

并查集 20 点修订已在本机固定断网参考槽通过原始输出试跑并离线回读，见[交付记录](delivery-log.md#2026-11-13并查集候选检查器试跑)。不改变该题的发布门禁。

Z 算法 32 点修订也已通过 medium 槽试跑和[离线复核](delivery-log.md#2026-11-14z-算法候选检查器试跑)。仅提供固定本机档位证据，不改变审核准入或发布状态。

## A+B 固定材料隔离判题

本阶段把 A+B 有序正式计划转成 `oj.aplusb-isolated-judge.v1` 清单。清单绑定候选及其限额、内容版本、完整导入修订、来源锁、正式计划、运行档位，以及每个点输入/答案的 SHA-256 和长度；只输出元数据。Worker 接受独立核对的清单和提交字节摘要，从私有包重新冻结并核对全部材料和答案，随后用现有固定断网参考槽编译指定 GNU++17 源码。前置绑定失败不创建报告目录、不启动槽。按正式顺序首错停止，剩余点为 `NOT_RUN`。

独立 `oj.aplusb-isolated-judge-report.v1` 报告绑定清单、提交、候选、计划和运行档位，不含源码、答案或程序输出。仅在判定明确、制品清理、停止确认及锁清理都通过时完成。旧的标程试跑报告与离线复核保持原协议，不能将隔离判题报告用作标程验证。回退移除两个维护入口即可，无数据库或运行配置迁移。复现见 [README](../README.md#ab-固定材料隔离判题)，联验见[交付证据](delivery-evidence/aplusb-isolated-judging.md)。

### Ubuntu 独立判题实验

`--ubuntu-aplusb` 显式选择独立的 Ubuntu 运行档位。只用于无宿主共享的本地 Apple 虚拟机、Linux arm64、内核 `6.8.0-106-generic` 和客体的固定 cgroup v2 Engine。清单含不同的档位摘要，不能与 Desktop 互换。判题前核对独立的 `oj-aplusb-ubuntu-sandbox` 名称、所有者、固定镜像、入口及隔离；启动后核对有效 CPU、内存、swap 和进程上限。控制与文件传输均走客体的固定 Unix socket。只在专用客体确认无其他槽运行或隔离后，使用已存在的镜像预置停止槽，不下载镜像：

```sh
/usr/bin/docker --host=unix:///var/run/docker.sock create --pull=never \
  --name oj-aplusb-ubuntu-sandbox --label owner=onlinejudge.ubuntu-aplusb-test \
  --privileged --network none --read-only --ipc private --cgroupns private \
  --restart no --tmpfs /tmp:rw,nosuid,size=128m --shm-size 128m \
  --memory 1g --memory-swap 1g --cpus 1 --pids-limit 128 \
  sha256:cd40ffdb32619806a8d97e59fe03e890fe4066835f57efa6ae13f919ef3262c5
```

保存返回的原始完整容器 ID。Ubuntu A+B 与小型 Desktop 参考槽共享宿主锁名，Ubuntu 的隔离标记不能用 Desktop 恢复命令清除。仅在确认旧 Worker 及在途控制操作全部排空、仍持有可信原始 ID 的前提下，由原客体 root 执行 `OJ_SANDBOX_EXPERIMENT=1 worker recover-aplusb-ubuntu <原始完整容器ID>`，保持 M0 槽和材料环境变量未设置。恢复不启动槽，不接受替换容器；已停止路径仍需按原 ID 回读。回退可停用专用命令；有未确认停止或隔离标记时先用支持该档位的二进制恢复，旧版本不清除此标记。

独立判题不登记租约；同一档位的运输清单可按[下节](#ab-租约链路联验)另行导出。旧归档复核、目标节点交接和生产门禁保持原契约。见 [本地实测](delivery-evidence/aplusb-isolated-judging.md#ubuntu-arm64-专用客体)。

### A+B 归档报告复核

`checker-candidate verify-aplusb-archive` 专用于已归档的 A+B 17 点本机 AC、WA、OLE 三份报告。用独立固定的来源锁、正式计划、候选及上限，从私有题包重建判题清单。按角色核对固定报告字节和提交摘要、实际档位、点序、用量、首错停止及清理记录。错误归档、互换、改写或有未完成标记均拒绝。复核只读取证据，不启动容器，也不能证明现场节点运行过。

```sh
.local/bin/checker-candidate verify-aplusb-archive \
  --bundle "$PRIVATE_BUNDLE" --source-lock "$TRUSTED_SOURCE_LOCK" \
  --ac "$AC_ARCHIVE" --wa "$WA_ARCHIVE" --ole "$OLE_ARCHIVE"
```

路径使用绝对路径；输出仅有固定摘要、点数和本机范围。审核人可从现有授权的[批准准入](problem-drafts.md#批准准入状态)读取同一份摘要。生产检查器与目标节点门禁仍须各自的受信证据。

## A+B 租约链路联验

现有的 A+B 17 点清单可以导出为独立隔离配置。运输清单绑定原判题清单摘要、检查器语义、单 token 限额、执行环境及缓存预算摘要，并按正式顺序声明输入和答案对象。维护命令在新私有目录写入内容寻址对象，最后写清单；只含判题输入和答案，不导出标程。登记前复核所有对象，成功后才向 PostgreSQL 写入版本和队列时钟；相同配置可重复登记，同版本不同清单拒绝覆盖。

控制端与 Worker 均须显式设置同一份 `OJ_APLUSB_MANIFEST_FILE`、`OJ_APLUSB_MANIFEST_SHA256` 与 `OJ_SANDBOX_EXPERIMENT=1`。配置与 M0 档位互斥；`OJ_APLUSB_RUNTIME` 未设置时使用原 macOS arm64 Desktop 断网小型参考槽。Ubuntu 档位在登记、控制端及 Worker 三处均须明确设置 `OJ_APLUSB_RUNTIME=ubuntu-arm64`，并与其运输清单的固定运行摘要吻合。Ubuntu Worker 必须使用[客体停止槽](#ubuntu-独立判题实验)和 `OJ_APLUSB_START_STOPPED=1`。控制端限定回环 mTLS 和登记的一套队列版本，Worker 的领取请求须声明该版本；Ubuntu 请求还须提供 `fixture_manifest_sha256`，任一不符均在创建租约前拒绝。租约、描述、源码及材料字节逐层核对。每次尝试新建缓存账本；租约监督、首错停止、制品清理与未知状态整槽停止沿用原有边界。轮询和回报没有新增公开入口。

此阶段在临时数据库通过受信存储直接排入合成任务，以验证控制端和 Worker，不开放普通用户提交。回退先排空并停止隔离 Worker 和控制端，再撤下 A+B 配置；已登记材料和历史结果保留，不要求迁移，也不将隔离版本改写成 M0 版本。复现步骤见 [README](../README.md#ab-租约链路联验)，结果见[交付证据](delivery-evidence/aplusb-leased-judging.md)。

`OJ_APLUSB_START_STOPPED=1` 仅允许显式 A+B Worker 在持有主机及参考槽所有权锁时启动已停止的固定槽。租约开始前先写隔离标记；启动回执不确定时只尝试停止已绑定的容器，无法确认则保留隔离标记。Ubuntu 仍使用独立的槽恢复命令；恢复联验的外层脚本不启停参考槽。[历史重判](rejudge.md)保留 Desktop 清单行为，Ubuntu 必须明确配置原运输清单及档位。

## 后续接入条件

### A+B 目标节点交接

目标节点由维护者独立确认；以下工具只交接材料及核对返回报告，不连接或操作目标节点。同一 A+B 17 点材料可以使用下列两套固定档位，槽名和档位摘要须成对提交。每套只接受自己的判题及运输清单。不同内核、镜像或隔离参数需要单独的执行适配、清单和实测。交接目录不包含 Worker 二进制、原私有题包、标程或证书；只能由有权限的维护者通过私有通道传递。

| 固定环境 | 槽 | `--runtime`（报告档位摘要） |
| --- | --- | --- |
| Desktop arm64 | `oj-reference-sandbox` | `1dacc3c77e44446d598490dd0d808ff32e1bcd1114e715af521406fcc7804ef6` |
| 专用 Ubuntu arm64 | `oj-aplusb-ubuntu-sandbox` | `8cf3b8386627d1a92b7566521c78dd030bf00fdd2acd5485dcc9bc4357e492b8` |

```sh
# 使用绝对路径。节点、候选 Git SHA、运行档位摘要和槽名来自独立确认。
checker-candidate aplusb-target-handoff --mode pack \
  --source "$PRIVATE_APLUSB_EXPORT" --directory "$NEW_HANDOFF" \
  --node "$EXPECTED_NODE" --revision "$EXPECTED_REVISION" \
  --runtime "$EXPECTED_RUNTIME" --slot "$EXPECTED_SLOT"

checker-candidate aplusb-target-handoff --mode verify \
  --directory "$RECEIVED_HANDOFF" --expected-handoff "$TRUSTED_HANDOFF_SHA256" \
  --node "$EXPECTED_NODE" --revision "$EXPECTED_REVISION" \
  --runtime "$EXPECTED_RUNTIME" --slot "$EXPECTED_SLOT"
```

单独传递 pack 输出的摘要；不要从收到的目录重建信任。目录须为接收者控制、无并发写入，文件权限保留为 0600。核验要求固定清单和所有摘要对象齐全；多余对象、缺文件或错配都拒绝。打包失败应另用新目录。

有现场授权后，节点操作员须独立确认 Worker 二进制来源、Git 候选版本和节点身份，使用[固定隔离命令](../README.md#ab-固定材料隔离判题)和另行授权提供的同一私有题包运行三个固定合成对照，再确认原槽退出且 PID 为零。Ubuntu 判题须加 `--ubuntu-aplusb` 并使用其独立判题摘要。通过独立通道交回每份报告的字节摘要与实际节点、版本证明；用 `--mode reports` 替换 verify 模式，并附 `--ac`、`--wa`、`--ole` 三个绝对归档目录以及对应的 `--ac-sha256`、`--wa-sha256`、`--ole-sha256`。

回读检查完整、停止、点序、用量、首错停止、三份固定合成源码摘要、材料和执行档位。内置的既有 Desktop 和 Ubuntu 对照归档摘要不接受作新证据。输出固定标注 `report-readback-only`：报告本身没有可信运行时间、节点身份或 Git 构建证明，不能将通过回读当作新鲜性或现场证明。回读不会写审核、发布或生产配置。独立确认目标节点、生产检查器与人工终审后才可推进相应门禁。见[本机交接演练](delivery-evidence/aplusb-target-handoff.md)。

#### 封存与复核报告回执

有权限核验原始材料和报告的一方，可在 `--mode reports` 的原命令上追加 `--receipt-out "$NEW_RECEIPT"`。工具在三份报告、完成标记和交接材料均通过检查后，在接收者控制的新目录写入 0600 的 `receipt.json`，同步文件和目录后输出该文件的 SHA-256。已有目录不会覆盖；失败后若已留下目录，不应自行从目录计算信任摘要，应使用新目录重新核验。

```sh
checker-candidate aplusb-target-handoff --mode receipt \
  --directory "$RECEIVED_RECEIPT" --expected-receipt "$TRUSTED_RECEIPT_SHA256" \
  --expected-handoff "$TRUSTED_HANDOFF_SHA256" \
  --node "$EXPECTED_NODE" --revision "$EXPECTED_REVISION" \
  --runtime "$EXPECTED_RUNTIME" --slot "$EXPECTED_SLOT"
```

通过可信渠道分别核对两份摘要和预期目标；不要从收到的回执自取预期值。回执固定来源锁、内容版本、完整导入修订、正式参考档位、正式计划、检查器候选、判题与运输清单、运行档位及三份报告摘要，不包含题包、源码、报告原文或测试点标识。回读只校验回执字节和绑定，无须持有原报告；它没有签名、实际运行时间、节点或构建证明，不会移除 `checker_admission_missing` 或 `target_runtime_missing`。既有 `reports` 命令未设 `--receipt-out` 时仍返回原摘要 JSON。

现有草稿仍为 `configured-not-validated` / `not-run`，`production_profile` 和 `limits` 仍为 null。目标机器实测还须验证显式预算下的真实参考/错误程序、受限编译执行、输出采集及故障分类，然后由有权限的验证、非作者审核及原子发布流程使用准确的候选摘要。离线预检只验证冻结答案，没有连接 Worker 或数据库，也不解除发布阻断。
