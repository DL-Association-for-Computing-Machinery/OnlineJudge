# 私有草稿的受限参考程序验证

P4 的首个执行入口是维护者显式调用 `worker validate-reference`。它从完整私有 bundle 冻结参考源码和全部有序测试材料，通过独立文件缓存通路编译一次、逐点使用 go-judge 新隔离环境运行，再用 `lc-wcmp-tokens-v1` 比较实际输出。该命令不连接控制端、认领任务、读取 PG/Redis、写数据库验证状态或发布题目。命令与专用真实专项归 [README](../README.md#私有草稿参考验证)。

## 当前支持范围

只接收固定来源锁 SHA-256 `fe2722edade748228c9add9c54704c09861fa6be649b5cdf59e1f214b5ee9617`：默认选择 `library-checker:aplusb` 的 `sample/aplusb/sol/correct.cpp` 和完整12点；显式`--source-id library-checker:unionfind`选择`data_structure/unionfind/sol/correct.cpp`和完整18点，`library-checker:zalgorithm`选择`string/zalgorithm/sol/correct.cpp`和完整29点，`library-checker:suffixarray`选择`string/suffixarray/sol/correct.cpp`和完整50点，`library-checker:primality_test`选择`number_theory/primality_test/sol/correct.cpp`、固定4个头文件和完整13点。各来源绑定独立profile及固定槽；后三者共用medium槽及同一canonical锁。不接收参考程序替换、任意脚本或路径布局。来源锁变更需要重新审查准入。其余五题尚不支持这个执行入口，不截取样例或部分测试点来制造通过。

限制来自现有实验传输、执行环境和本轮验证配置，不是生产选手限制：

| 范围 | 显式预算 |
| --- | --- |
| 冻结材料 | 源码64 KiB、每点输入16 KiB、每点答案4 KiB、最多64点、总量2 MiB |
| 来源定义与哈希 | `info.toml` / `hash.json`各64 KiB以内，同时计入总量与plan身份 |
| 字节与检查域 | 源码/输入/输出按原始字节传输；所有参考答案在编译前按检查器UTF-8/token域先验 |
| 检查器 | `lc-wcmp-tokens-v1`，每文件及单token均4 KiB，大小写/数字精确比较 |
| 执行 | 原样绑定 `experiment.PinnedEnvironment()` 的镜像、内核、编译器、命令及资源；不改 M0 profile |
| 阶段墙钟 | 验证阶段2分钟；启动预检20秒；制品清理/停止另用独立5秒/15秒预算，不能以取消证明停止 |
| 文件协议 | `p4-gojudge-cached-files-v1`；单文件48 MiB、存活缓存64 MiB、任务载荷128 MiB、最多4个活动缓存对象/512次操作，文件IO并发1 |

上述小材料profile为`p4-lc-reference-files-small-desktop-arm64-v1`，绑定文件协议预算和原Runtime，区别历史内联profile。源码和输入上传后回读核对SHA-256；stdin使用缓存fileId，stdout缓存后按原始字节拉取，不经过JSON字符串转换。参考答案始终留在宿主，全部答案在任何execution适配器Verify/Compile前先验；CLI在此之前可做槽启动和Host环境探测，不运行参考程序。源码和输入允许原始字节。48 MiB协议cap不授予其他大题准入，不能据此宣称支持10.4 MB输出；生产配置仍为null。

unionfind独立profile为`p4-lc-unionfind-reference-desktop-arm64-v1`，Runtime为`p4-unionfind-gnu17-desktop-arm64-v1`，不放宽M0或小材料入口：

| 范围 | unionfind试验预算 |
| --- | --- |
| 磁盘快照 | source64 KiB、单input4 MiB、单answer256 KiB、总64 MiB含两个catalog、最多64点；完整来源集合仍独立核验 |
| 执行 | Run CPU1秒、wall5秒、memory128 MiB；Compile原10秒CPU/15秒wall/512 MiB；原镜像/工具链/命令及外层隔离保持 |
| 流与检查器 | 仅raw Run stdout256 KiB；Compile stdout/stderr及Run stderr各4 KiB，独立`diagnostic_bytes`进入profile摘要；checker每文件256 KiB/token4 KiB |
| 阶段与文件 | 验证阶段3分钟；原48/64/128 MiB文件协议、4对象/512操作、独立清理和Stop预算保持 |
| 固定槽 | `oj-reference-sandbox` / `slot-reference.lock`；新构造器只接此profile，拒M0配置，M0入口同样拒此配置 |

zalgorithm的独立profile为`p4-lc-zalgorithm-reference-desktop-arm64-v1`，suffixarray为`p4-lc-suffixarray-reference-desktop-arm64-v1`；固定`ReferenceMedium`档位仅准入这两个来源：

| 范围 | medium试验预算 |
| --- | --- |
| 完整快照 | source64 KiB、input1 MiB、answer4 MiB、最多64点；zalgorithm总64 MiB、suffixarray总128 MiB，均含catalog，分别完整29/50点不采样 |
| 执行与检查 | Run CPU1秒/wall5秒/memory128 MiB、raw stdout与checker每文件4 MiB；Compile/诊断/token及3分钟阶段墙钟沿用unionfind |
| 文件协议 | `p4-gojudge-cached-files-medium-v1`；累计payload256 MiB，单file48 MiB/live64 MiB/4对象/512操作/单文件IO并发1保持；Host与Cached账本从同一档位解析 |
| 固定槽 | `oj-reference-medium-sandbox` / `slot-reference-medium.lock`，旧两类槽不替换 |
| 沙箱全局 | 只将global output/RLIMIT从1 MiB提升到8 MiB，global copy-out保持1 MiB；Compile copyOutMax仍1 MiB，Run的stdout collector显式max4 MiB并拉取cap+1 |
| 容器形态 | 同一固定镜像，Entrypoint精确`[/opt/go-judge]`、Cmd精确固定参数；新`sandbox_cmd,omitempty`进入新Runtime摘要，旧profile不出现此字段，旧nil/空Cmd兼容保持 |

累计预算按每次raw下载请求上限收费，29点最坏stdout请求加双传input已有140258181B（另加源码双传），超过旧128 MiB；suffixarray完整50点加源码双传最坏236891698B，307次逻辑操作，仍在256 MiB/512操作内。不通过退款/只计实际输出规避。Medium只接封闭档位枚举，新factory/opaque slot拒零值和未知值，Host仅在其私有解析中明确零档位为原M0；错误不回落或派发Docker。CLI复用固定source→profile/tier解析，Runner在准备前与冻结后核对sourceID，不能借其他来源配置。旧A+B、unionfind、zalgorithm的plan/profile摘要均不变；medium沿用原reference collector OLE窄适配。

suffixarray仅扩大该固定来源的完整快照总量，Runtime、文件协议及槽配置沿用medium；不修改zalgorithm的64 MiB准入。回归实际冻结超过64 MiB的匿名磁盘内容、拒绝超过128 MiB的完整集合、跨来源共享一份额度，并在缓存通路完成50点最坏输出计费及清理；这不构成恰好128 MiB内容成功或宿主连续内存峰值证明。

`primality_test`使用独立`ReferencePrimality`档位（枚举4），profile为`p4-lc-primality-reference-desktop-arm64-v1`，Runtime为`p4-primality-gnu17-desktop-arm64-v1`。它复用medium容器及同一锁，但Host保留自己的档位和双端文件预算；不借用Medium编译入口。

| 范围 | primality试验预算 |
| --- | --- |
| 完整快照 | 主源码及4个依赖合计64 KiB；input2 MiB、answer512 KiB、总32 MiB含catalog、最多64点；来源目录要求完整13点 |
| 固定布局 | `sol/correct.cpp`、`lib/prime.hpp`、`lib/montgomery.hpp`、`lib/util.hpp`、`common/random.h`，逐个绑定来源对象身份和目标路径 |
| 编译 | 固定GNU++17/O2/pipe，入口`sol/correct.cpp`，增加`-Icommon`；源文件均以原始缓存字节copyIn，不展开或改写include |
| 执行与检查 | Run CPU1秒/wall5秒/memory128 MiB，raw stdout及checker每文件512 KiB；Compile/诊断/token及3分钟阶段墙钟保持 |
| 文件协议 | `p4-gojudge-cached-dependencies-v1`；6个对象容纳5份编译材料及预留exe；payload256 MiB、file48 MiB/live64 MiB/512操作/并发1保持 |

私有主源码和4个头文件必须与全部测试材料一起冻结；缺失、损坏或超限均在execution Verify前拒绝。plan新增可省略的`source_path`和`dependencies`并绑定摘要，旧四来源不出现新字段且摘要保持。Runner要求可选`CompilationSandbox`能力；缺少能力不调用Verify。编译适配器先校验整个有界文件集，再复制字节并上传；拒绝绝对/非规范路径、dot段、反斜杠、大小写别名、重复、文件与目录冲突和保留输出路径，不接收调用者缓存ID或宿主路径。旧M0/Small/Medium拒绝此能力。

各材料上传后回读哈希，最多5份源码与exe预留同时存活；所有已知材料共享原独立5秒清理期限。未知上传回执继续fence，清理失败保留已知exe制品供最终清理，并使验证失败。诊断只新增封闭`dependency`角色，不记录头文件名或内容。13点最坏累计载荷为32521421字节、97次逻辑操作（含全部材料双传、stdout按请求上限收费及删除），不以实际小输出退款。嵌套目标目录依据固定[go-judge v1.13.3 copyIn](https://github.com/criyle/go-judge/blob/v1.13.3/envexec/file_copyin.go)的`MkdirAll`语义，另以可信固定嵌套include专项验证。

其余五题仍需完整快照、累计传输或临时资源的独立准入；`point_add_range_sum`的#114仍为草稿，旧失败不因新来源通过而解除。生产限额保持未定。

文件入口cap结合固定tmpfs/cgroup边界，内部账本计量存活缓存、总传输载荷与操作数；没有独立multipart暂存或其峰值计数器，64 MiB存活缓存不等于全部存储/内存峰值。创建前为源码/输入预留实际大小，为未知大小的exe/stdout按上限预留；最多一个大输入。上传和校验回读分别计费，下载按请求上限保守计费，失败不退额度；固定MIME/trailer及控制JSON各自有界。累计操作数限制零字节操作与已删除ID历史，并为已存在对象保留DELETE额度。固定挂载、collector和cgroup的真实容量证据另见交付记录。

协议依据固定v1.13.3的[文件handler](https://github.com/criyle/go-judge/blob/v1.13.3/cmd/go-judge/rest_executor/file_handler.go)与[缓存输出转换](https://github.com/criyle/go-judge/blob/v1.13.3/worker/worker.go)：multipart字段file返回JSON ID、按ID取原始文件、copyOutCached返回fileIds。handler未提供本项目所需的总量边界，下载有整文件读取fallback；[缓存Get会刷新TTL](https://github.com/criyle/go-judge/blob/v1.13.3/filestore/timeout.go)，因此限额、对象所有权和显式清理由本适配器负责，不能以TTL当清理证明。

## 固定快照与失败报告

`Prepared.PrepareReference`沿用bundle私有权限、路径/库存和来源校验，先检查完整材料集合的大小，再从安全打开的同一描述符限长读取、校验大小/哈希并冻结私有字节。每次调用返回独立切片，不在执行期间重开原材料。版本、完整导入修订、来源锁、源码身份和有序逐点身份进入plan；plan摘要为不含原始字节的紧凑`encoding/json` SHA-256，profile摘要绑定完整配置。读取预算和检查器预算分别计量。

磁盘来源使用`Prepared.PrepareDiskReference`按自身固定sourceID选择受信布局/预算；原`PrepareUnionFindReference`保留仅unionfind的兼容入口。先按完整集合扣除总预算，再创建0600临时文件并立即unlink，在同一匿名文件依次存入全部source/input/answer，每个对象从同一安全打开描述符分块哈希并核验完整大小后才返回。所有来源共用进程内最多一份的额度，Prepared占用持续到幂等Close；错误/取消关闭并释放，Close失败阻断报告成功。外部plan为独立身份拷贝，内部span不可修改，执行时通过ReadAt只读当前点，所有答案预检不加载输入。原文件后续修改不影响已冻结字节；不宣称Go堆或文件系统硬时限。匿名unlink失败在写内容前拒绝，极端情况下可能留空0600文件；磁盘准备失败没有完成的plan和逐点列表，报告points可为空，不能解释为已执行零点后通过。

验证器独立读取锁定`info.toml.tests`，按组顺序与组内编号生成完整点集，严格核对草稿映射、sample角色和每点输入/答案SHA；`hash.json`必须恰为这些点的完整配对，无遗漏/额外/重复键。删点后重算合法版本和index仍会在Compile前拒绝，不只检查草稿自称的完整集合，也不按哈希字典排序推导来源顺序。仅复用已锁定的MIT许可`go-toml/v2 v2.2.4`，从间接依赖提升为直接使用；无版本升级、新下载或go.sum变化，不执行生成器/脚本。

报告schema为`oj.reference-validation.v1`，包含上述plan/profile及摘要、全部逐点状态、实际编译/执行CPU纳秒与内存字节、固定槽的完整容器ID/StartedAt和停止确认；不序列化源码、输入、答案、实际输出或编译诊断。材料读取失败时plan只保留已声明身份；缺少`plan_sha256`表示完整冻结尚未成功。首个参考错误停止后续点并保留`NOT_RUN`，未知证据归SE，制品清理失败不能通过。参考程序CE/WA/RE/TLE/OLE表示参考验证未通过，不是对选手提交的判题。

停止失败时提供可选`stop_diagnostic`，保留固定`stage`/`code`、已解析枚举及已确认进程数量`observed`/`confirmed`（0—128）和从1开始的失败探针序号`probe`（非探针失败省略）。枚举尚不可用时observed为0，不证明没有进程。阶段为`fence`、`drain`、`inspect_before`、`enumerate`、`kill`、`inspect_after`、`pid_probe`或`complete`；未知适配器错误为`unknown`，不推导阶段。命令错误区分`deadline`、`canceled`、`launch_failed`、`command_exit`、`output_limit`和`wait_delay`，另有固定状态/证据错误；不含原始错误、stderr、参数、退出码或PID。诊断不会改变报告接纳条件；停止成功而marker清理失败时，报告仍失败、stop_confirmed为true且没有stop_diagnostic。

本适配器失败诊断附加整体`elapsed_ms`及可选`steps`，最多按实际到达顺序记录`drain / inspect_before / enumerate / kill / inspect_after / pid_probe`六步；未到达步骤不生成零耗时/成功记录，PID探针只记录合计耗时与尝试`count`（最多128），不输出逐PID明细。各毫秒数取单调时间，超过60000截断并设置同级`elapsed_capped=true`；这是证据大小边界，不是新的操作限额。整体从进入Stop起算，包含fence/锁等待及返回前处理，各步之和不保证等于整体，不能据此证明10秒期限在实际调度中严格兑现。成功报告不添加诊断字段，返回与owner映射均复制步骤列表。

kill步骤的`context_before`为调用前采样的`active / canceled / deadline`，active只表示采样时未取消；`call_attempted=true`只证明进入既有调用路径。`local_process_start`为`unknown / not_started / started`：只有真实命令执行器的`cmd.Process`证据区分本地CLI是否启动；替身或nil error无此错误通道证据时保持unknown，不能解释为未启动。即使started，也不证明Docker daemon已接收kill或容器已收到信号。枚举可能先耗尽共享预算，随后kill在过期context上失败；既有kill优先主诊断仍保留，步骤记录补充此前枚举结果。原始失败报告不新增或反推这些字段。

只有制品清理、停止确认、槽标记清理和最终报告写入均成功，命令才可宣布`reference-validation-passed`。该状态仅证明本次固定参考程序与完整点集在指定试验预算下通过；`publication`始终为`blocked`。报告不是可信签名或非作者审核，也不是全部题面/数据质量、完整P4或生产资源准入证明。

缓存账本只接受本次操作创建的source/exe/input/stdout身份，拒绝外来、重复或角色冲突的ID。每点独立限时清理input/stdout，最终清理exe；未知上传回执不重传、不猜ID删除，任何协议/清理错误封闭适配器，由整槽Stop收束。源码/参考答案不会进入报告。清理期间父context失效也必须返回失败；全部原始参考答案仍在编译前先验。旧报告保留其原profile身份，不升级或追认。

unionfind运行输出的正常collector溢出按固定协议作窄适配：[v1.13.3收集逻辑](https://github.com/criyle/go-judge/blob/v1.13.3/envexec/file_collect.go)及[结果转换](https://github.com/criyle/go-judge/blob/v1.13.3/envexec/run_single.go)会附带同名`Output Limit Exceeded`错误标签。仅在精确status/error、Exit0、完整非负且不超预算用量、恰好cap+1原始stdout、存在有界stderr、唯一stdout/CollectSizeExceeded、已验证角色和无残余ID同时成立时，才归一化这个冗余标签并判OLE。其他error、缺字段、混合文件错误、编译或M0路径继续拒绝；控制JSON仍限128 KiB。

输出目录必须是新的绝对路径。解析已有父目录链接后，按目录实际身份检查祖先，拒绝输入bundle内的符号链接及大小写别名路径。目录0700、文件0600；先放置`INCOMPLETE`，最终报告使用排他创建并同步，再移除标记和同步目录。失败/中断保留已有成果，不覆盖、修复或自动删除；写入/同步失败属于未知报告完成状态，须检查命令退出结果和标记，不仅凭JSON中出现passed决定接纳。依赖可信稳定的父目录，不承诺文件系统硬时限、掉电恢复或独立备份。

## 执行失败诊断

2026-11-04的[停止故障调查](reference-validation-investigation.md)记录新主线独立50点及一次预注册前置序列均未复现，以及旧stdout删除访问日志的异常记录时间差。旧根因仍未确定，HTTP200/新通过不能证明旧客户端成功或解除#114；后续按该页的狭窄假设继续收集证据。

可选`execution_diagnostic`包含`entries`与可选`truncated`，跨所有调用最多八条；第九条或诊断遍历遗漏才标记截断。成功省略字段，原成功JSON、profile/plan及schema保持。每条有固定`operation`（verify、compile、run、cleanup、snapshot_close、validation或unknown）、`point`（run从1开始，非逐点为0并省略）、`phase`、`role`和`code`。

phase为verify、upload、readback、control、stdout_read、validate、delete或unknown；role为source、dependency、input、executable、stdout、environment或unknown，来自可信调用位置和私有对象账本，不根据错误文本猜测。delete映射到cleanup，保留触发清理的点号；最终制品清理point为0。code只接收deadline、canceled、budget、integrity、ownership、invalid_evidence、http_rejected、decode、command_exit、launch_failed、output_limit、wait_delay或unknown。材料、文件ID、命令、stdout/stderr和原错误均不进入诊断。

已标记分支内同一cause不重复展开；独立join分支保持主错误→各清理错误→最终父context的观察顺序，未标记分支使用unknown阶段/角色。投影最多检查64个根/child槽，包括nil项，限制宽列表、深链与循环；恰好64项且没有待检查项不标截断，每次返回新列表。界限不约束外部Unwrap/类别方法本身的成本或既有errors.Is等其他路径。GoJudge控制传输原本丢弃context unwrap的行为保持，metadata为deadline不表示errors.Is也成立；宿主文件通路继续保持原context原因，Stop类别未扩大。

覆盖缓存适配器Verify/Compile/Run/Remove、Runner调用/结果校验、快照关闭及最终取消；不承诺覆盖Host.Start、材料预检或所有操作系统错误。control仅表示控制调用失败，不证明服务端执行或daemon接收。执行与停止诊断可同时持久化，均不改变SE、停止/dirty门禁、资源预算、零重试或publication blocked；旧报告不反推新字段，也不提供恢复权限。

`Host.RoundTrip`失败时，对应条目可选`host_trace`保留以下固定阶段的前缀，最多五项：

| 阶段 | 范围 |
| --- | --- |
| `admission` | 请求形状/路由与Host活动入场 |
| `prepare` | 有界请求体、固定配置解析 |
| `inspect` | 本次容器inspect命令及身份/生命周期/隔离校验 |
| `command` | 本次固定Docker CLI控制调用 |
| `response_validation` | 命令返回后的context、HTTP状态与响应大小校验 |

整体`phase`为失败阶段，`last_completed`为上一个完整阶段或`none`；各step含`completed`、`elapsed_ms`/`elapsed_capped`、进入时`context_before`、`has_deadline`、`remaining_ms`/`remaining_capped`。耗时用单调时钟，负值归0、超过60000ms截断；剩余期限也非负且有界，不改变实际期限。context采样与剩余时间并非原子观察，0ms可来自取整或两次采样之间到期，不能单凭它判过期。失败阶段在返回点冻结，不吸收后续defer清理；整体耗时包含body关闭及Host活动收尾，不能把总量或阶段之和解释成纯沙箱执行时间/硬时限证明。

inspect/command的`call_attempted`只证明调用该函数；`local_process_start`仅接受本次直接`commandFailure`的`started`/`not_started`，成功返回、包装错误或无证据替身保持`unknown`。本地进程启动不证明Docker daemon收到请求。固定顺序、完成位置、计数与枚举先校验再复制，未到达数组槽清零，非法或歧义trace省略；不能从一个join分支借另一操作的trace。脱敏重建错误前只复制元数据，不重新挂回原传输错误；主control、点内input清理和最终exe清理各自保留归属，finish附加context不复制trace。

新增数据不进入Profile/Plan；成功JSON/schema和既有错误`Is/As`语义保持。元数据检索与公开投影分别有64槽界限；公开投影在provider终止节点只读其自身类别，裸provider未声明类别时可为unknown，不重开嵌套遍历。总条目仍最多八条，字段不含命令/路径/缓存ID/材料/原错误；外来getter、Unwrap或类别方法自身成本不由槽计数保证。缺少`host_trace`可表示能力不适用、数据无效/歧义或检索受限，不能证明没有Host调用。

范围只覆盖RoundTrip控制请求（GET元数据、POST/run、DELETE），不覆盖Host.Start/Stop/Verify或独立文件传输，也不能回溯已成功返回的输入回读内部耗时。Host成功返回、随后GoJudge JSON decode失败时没有Host trace，不通过成功响应侧通道补造证据。新增字段不是故障修复、题源准入或自动重试授权；#114及staticrmq草稿#119的原失败保持，后续须另行预注册有可检验假设的调查。

## 槽所有权与停止

显式`OJ_SANDBOX_EXPERIMENT=1`和跨checkout文件锁仍是前置条件。默认A+B使用primary/secondary；unionfind使用固定small reference槽，zalgorithm、suffixarray与primality_test共用固定medium reference槽及同一锁，均拒绝非空`OJ_WORKER_M0_SLOT`，不从M0 parser选择新槽。quarantine不自动恢复。命令先持锁并同步dirty标记，再接触沙箱。`--start-stopped-slot`只启动预存在、停止/PID0、固定owner及完整隔离配置匹配的槽，使用原full ID，不创建、替换、重启运行中容器或接管其他项目。返回Host绑定该ID与新StartedAt；运行中每次请求继续校验同一生命周期。

正常退出和失败均在独立context中停止并排空Host，核验容器停止及已观察PID消失；只有确认后才清理marker。启动回执未知时仍尝试确认原生命周期停止；拿不到绑定Host、停止未确认或marker同步失败则保留隔离。启动/验证报错后若已确认停止且清标记成功，可以解除marker，但本次报告仍失败。原[受控恢复](../README.md#worker-异常后的受控解封)步骤继续适用。取消发生在Run、制品清理或Stop期间都不得生成通过状态。不会为了保存报告恢复旧槽或再次执行未知操作。

独立槽异常时使用`OJ_SANDBOX_EXPERIMENT=1 worker recover-reference [<admitted-source-id>] <original-full-container-id>`，必须清空`OJ_WORKER_M0_SLOT`并先确认旧宿主进程及在途操作已排空。省略source保留原unionfind/small默认；显式`library-checker:zalgorithm`、`library-checker:suffixarray`或`library-checker:primality_test`只选同一medium槽，A+B或未知source拒绝reference恢复。它只接来源对应的固定槽，核验原ID/配置/停止生命周期，成功才清该锁；不启动槽、不认领任务、不追认失败报告，不能手工清marker。

Host的文件能力与原M0 RoundTrip分开：仅固定loopback文件路由、有限私有bytes reader、随机MIME boundary及有界sink；不接受任意URL、主机路径或流式Body提前交还。HTTP状态使用固定四字节trailer与二进制正文分开校验，子进程和全部IO结束后才释放active登记；取消不证明远端文件或进程已清除。M0默认入口、原JSON预算与Runtime保持。

Stop内部仍为10秒总预算，保持原调用顺序和零重试。枚举/解析失败仍尝试kill；同时失败时优先记录kill诊断，不能由此推定枚举成功。每次探针成功后才递增confirmed，首个失败立即结束。`pid_probe/pid_present`仅表示该时刻PID路径存在，可能是PID复用，不证明原进程仍活着；`proc_unavailable`表示无法建立procfs前提。探针唯有明确“不存在”才成功，表达式/命令异常仍失败；超时不能被当作PID存在或退出证明。宿主命令的既有128KiB＋1字节输出界限由显式组合缓冲执行，避免标准库ReadFrom绕过限长Write。

数据库中的草稿仍为`not-run`，既有来源准备报告不充当本站运行记录。后续须补齐其余题的大材料/依赖、持久化幂等验证任务及当前权限下的报告接纳、完整质量验证、非作者终审和原子发布。

首轮真实专项曾完整通过，但另两次实测出现停止确认未通过：包括额外12点全部AC后仍返回SE、保留隔离。后续受控recover仅确认当时安全，不追认原报告；旧报告没有新诊断，不能用当前代码反推其失败原因。后来[草稿PR #114](https://github.com/DL-Association-for-Computing-Machinery/OnlineJudge/pull/114)的旧suffixarray回归又出现第11点SE、Stop kill/deadline且原容器仍运行；外部停止和受控恢复不解除原交付阻断。当前已补停止步骤与执行失败阶段诊断；原根因尚未定位，维护入口仍有可用性限制，不自动重试至绿或放宽Stop。历史与本次证据见[交付记录](delivery-log.md)。
