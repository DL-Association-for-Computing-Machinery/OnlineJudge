# 私有草稿的受限参考程序验证

P4 的首个执行入口是维护者显式调用 `worker validate-reference`。它从完整私有 bundle 冻结参考源码和全部有序测试材料，通过独立文件缓存通路编译一次、逐点使用 go-judge 新隔离环境运行，再用 `lc-wcmp-tokens-v1` 比较实际输出。该命令不连接控制端、认领任务、读取 PG/Redis、写数据库验证状态或发布题目。命令与专用真实专项归 [README](../README.md#私有草稿参考验证)。

## 当前支持范围

只接收固定来源锁 SHA-256 `fe2722edade748228c9add9c54704c09861fa6be649b5cdf59e1f214b5ee9617`。`--source-id`默认A+B；当前版本十项254点均通过固定环境维护接纳，最终CI与合并状态见[交付记录](delivery-log.md#2026-11-04convolution_mod-十来源固定环境接纳)。路径均相对上游源目录：

| source-id | 固定参考源码 | 完整点数 |
| --- | --- | --- |
| `library-checker:aplusb` | `sample/aplusb/sol/correct.cpp` | 12 |
| `library-checker:unionfind` | `data_structure/unionfind/sol/correct.cpp` | 18 |
| `library-checker:zalgorithm` | `string/zalgorithm/sol/correct.cpp` | 29 |
| `library-checker:suffixarray` | `string/suffixarray/sol/correct.cpp` | 50 |
| `library-checker:primality_test` | `number_theory/primality_test/sol/correct.cpp`及固定4个头文件 | 13 |
| `library-checker:static_range_sum` | `data_structure/static_range_sum/sol/correct.cpp` | 11 |
| `library-checker:associative_array` | `data_structure/associative_array/sol/correct.cpp` | 20 |
| `library-checker:point_add_range_sum` | `data_structure/point_add_range_sum/sol/correct.cpp` | 21 |
| `library-checker:staticrmq` | `data_structure/staticrmq/sol/correct.cpp` | 27 |
| `library-checker:convolution_mod` | `convolution/convolution_mod/sol/correct.cpp` | 53 |

各来源绑定独立profile及固定槽。A+B沿用M0槽，unionfind使用small槽；zalgorithm、suffixarray、point_add_range_sum、primality_test、staticrmq、static_range_sum、associative_array共用medium槽及同一canonical锁。convolution_mod使用独立`oj-reference-convolution-sandbox`和`slot-reference-convolution.lock`，不扩大旧medium槽的8MiB上限。不接收参考程序替换、任意脚本或路径布局；来源锁变更需重新审查。十题维护参考验证通过不等于题目质量、生产预算或发布验收。

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

point_add_range_sum独立profile为`p4-lc-point-add-range-sum-reference-desktop-arm64-v1`，仅该固定来源准入input16 MiB、完整快照256 MiB，source64 KiB/answer4 MiB/64点保持。封闭`ReferenceMediumBulk`档位与Medium的Runtime逐字段相同，文件协议为`p4-gojudge-cached-files-medium-bulk-v1`、仅累计payload增至512 MiB，其余file/live/对象/操作数及诊断/检查器保持。完整21点最坏payload299443885B、133次逻辑操作，超过原256 MiB但低于512 MiB。所有旧七来源profile/plan摘要保持。

Medium与MediumBulk共用原容器及`slot-reference-medium.lock`；映射必须保留原tier，使Host与适配器分别绑定正确账本。两者Runtime相同，Verify不区分传输预算；CLI从同一source解析结果构造两端，误配由较小额度一端拒绝，不能据Runtime验证推导预算相同。回归分别验证两端旧256 MiB拒绝、新512 MiB额度及超限拒绝，实际132 MiB快照与16 MiB输入冻结/读回、完整集合超过256 MiB及input cap+1拒绝、共享额度和锁互斥；没有恰好256 MiB快照成功或任意材料组合容量证明。真实缓存stdin专项核验上传SHA-256回读和程序count/sum，在原8 MiB输出RLIMIT下读取8 MiB+1/16 MiB；阶段heap/cgroup/tmpfs样本不代表连续峰值或并发总RSS。

此前point_add_range_sum首次21点与大stdin通过，但旧suffixarray回归第11点SE、Stop为`kill/deadline`、`observed=0`且原容器仍运行，曾构成交付阻断。外部停止与受控恢复不追认首10点AC/后续NOT_RUN的失败报告；根因尚未定位。吸收已交付#123停止预算修复后，候选HEAD `0cd63e3ddacb3be00e3f693f1d0797a2f02d4e42`通过一次八来源174点及22个原子子测试的固定环境接纳，全部AC/Stop、完整旧Plan/Profile相等；支持该新版本准入，最终CI与合并状态见[PR #114](https://github.com/DL-Association-for-Computing-Machinery/OnlineJudge/pull/114)。历史失败不改写、不据此宣称旧根因已修复；[新接纳记录](delivery-log.md#2026-11-04point_add_range_sum-八来源固定环境接纳)与[原失败记录](delivery-log.md#2026-11-04point_add_range_sum-实现与停止故障阻断)分别保留。

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

staticrmq使用独立`ReferenceStaticRMQ`档位（枚举5）、profile `p4-lc-staticrmq-reference-desktop-arm64-v1`、Runtime `p4-staticrmq-gnu17-desktop-arm64-v1`和文件协议`p4-gojudge-cached-staticrmq-v1`。它仅扩大该来源完整快照至256 MiB、单input至16 MiB、累计payload至512 MiB；source64 KiB、answer/stdout/checker4 MiB、最多64点、原编译参数/执行资源/诊断/期限、48 MiB单file/64 MiB存活/4对象/512操作均保持。无私有编译依赖，沿用单文件入口；旧八来源摘要及预算不变，枚举3的point_add_range_sum已交付。复用原medium容器及同一锁，Host与Cached保留tier5且拒绝其他Runtime身份。原完整试验第9点SE；一次合入Host诊断后的[完整27点观测](reference-validation-investigation.md#staticrmq单次host边界观测)通过，但失败未复现、该次旧五来源回归未启动；吸收#123与已合并#114后，HEAD `e777ae2b985193ddaf1ec8d647d8697aa13c5964`通过一次九来源201点/24个子测试的固定环境接纳，完整旧Plan/Profile相等。新版本接纳见[九来源接纳记录](delivery-log.md#2026-11-04staticrmq-九来源固定环境接纳)，最终CI与合并状态见[PR #119](https://github.com/DL-Association-for-Computing-Machinery/OnlineJudge/pull/119)；不据此认定旧根因已修复或长期稳定性成立。

真实完整27点材料共200820493B（含catalog），需要超过旧128 MiB快照；最坏累计载荷`2*1890 + 2*158223359 + 27*(4194304+1) = 429696733`B，需要超过旧256 MiB传输额度。169为execution逻辑操作数，Host仅计自身文件操作，口径不等同。每次下载按请求cap计费，失败不退款；512 MiB精确边界测试通过账本注入证明，不是实际传输512 MiB。超过128 MiB冻结/末点读取使用合成集合；256 MiB材料再加catalog拒绝不等于精确256 MiB成功。真实完整集合另由CLI核验，最大参考答案距4 MiB仅3135B，不能用答案大小代替实际stdout/资源证据。

### static_range_sum参考验证

`ReferenceStaticRangeSum`为独立枚举6，profile为`p4-lc-static-range-sum-reference-desktop-arm64-v1`，Runtime为`p4-static-range-sum-gnu17-desktop-arm64-v1`，Files为`p4-gojudge-cached-static-range-sum-v1`。材料为source64 KiB/input16 MiB/answer8 MiB/总256 MiB含catalog、最多64点；raw stdout及checker每文件8 MiB，token/编译双流/运行stderr仍各4 KiB，payload512 MiB，其余Run资源/3分钟阶段墙钟/单file48 MiB/live64 MiB/4对象/512操作/各期限保持。Host与Cached保留档位6，物理复用原medium容器、Cmd及canonical锁，不替换槽；旧五profile保持；枚举3的point_add_range_sum已通过本轮固定环境接纳，枚举5的staticrmq亦已通过九来源固定环境接纳。

完整25对象149774419B已逐哈希核验：源码328B、两个catalog与11组input-answer，input总91745127B、answer总58026619B，最大分别11722894B/7636751B。固定源码仅两个标准头，无私有依赖。最坏累计载荷`2*328 + 2*91745127 + 11*(8388608+1) = 275765609`B，超过旧256 MiB，73次execution逻辑操作不等同Host计数。按请求上限收费，512 MiB边界用计数注入测试，不冒充实际搬运该总量；完整材料和16MiB/8MiB真实字节边界已在固定环境通过。

现有`-output-limit 8388608`是单文件大小约束，不是stdout/stderr合计限额。固定v1.13.3的[非pipe collector准备](https://github.com/criyle/go-judge/blob/v1.13.3/envexec/file_prepare.go)按Name打开各自普通文件，[Linux执行参数](https://github.com/criyle/go-judge/blob/v1.13.3/env/linuxcontainer/environment_linux.go)将Output映射到RLIMIT_FSIZE；此处分别通过官方GitHub只读API和源码页面核验。stdout collector现与8 MiB文件上限相等，可信程序继续写cap+1时预期先SIGXFSZ而保留8 MiB原始输出，不能沿用4 MiB collector超限的Exit0/CollectSizeExceeded断言；[结果转换](https://github.com/criyle/go-judge/blob/v1.13.3/envexec/run_single.go)与[收集逻辑](https://github.com/criyle/go-judge/blob/v1.13.3/envexec/file_collect.go)分别决定信号及正常退出后的collector错误。现有窄OLE条件完整保持，不仅凭Exit25归类。

专用驱动先核验完整16 MiB输入及stdout8 MiB/stderr4096的全部字节，再核对stdout cap+1完整窄SIGXFSZ证据和stderr4097的固定collector错误。预期拒绝必须恰为一条`validate/stdout/invalid_evidence`且未截断，任何stdout/input清理错误或附加context都失败；合成实际Cached适配器回归验证此门禁，不能把预期负例与清理失败的join当通过。首组及其Stop成功才进入11点CLI。首轮启动失败与Stop未确认仍保留；受控恢复只确认当时安全。用户批准localdev专项重启后，以原源码/制品/预算登记的新观测通过全部字节边界、11AC/Stop，随后独立原五来源串行回归也通过。该证据只支持当前固定环境维护入口，不解释旧启动根因或证明长期稳定；完整证据见[交付记录](delivery-log.md)。物理copy-out默认1 MiB保持，cached stdout独立拉取与原始字节保真已实测。

当前十题254点均完成固定环境维护参考验证，最终CI与合并状态见[交付记录](delivery-log.md#2026-11-04convolution_mod-十来源固定环境接纳)，历史失败保持。持久化申请和受信 Worker 报告见[参考报告接纳](reference-requests.md#参考报告接纳)；完整质量与非作者终审/发布仍未完成；草稿数据库仍为not-run、生产配置null、publication blocked，本入口不写入这些状态。

### associative_array参考验证

固定`library-checker:associative_array`接入独立`ReferenceAssociativeArray=7`，profile `p4-lc-associative-array-reference-desktop-arm64-v1`、Runtime `p4-associative-array-gnu17-desktop-arm64-v1`、Files `p4-gojudge-cached-associative-array-v1`。完整43对象451132962B（源码474B、两个catalog及20组input/answer）逐字节哈希核验；input总412885310B/最大39588095B，answer总38242614B/最大4691775B。固定源码只用cstdio/map，没有私有编译依赖；所有点和原始字节保留。

独立材料预算为source64KiB/input48MiB/answer8MiB/总512MiB含catalog，最多64点；raw stdout/checker8MiB及token/编译双流/运行stderr各4KiB。Runtime Run CPU5秒/wall10秒/memory128MiB/16进程，仅此来源改变CPU/wall，编译与原六来源保持。5秒依据已锁定`info.toml`的timelimit=5.0预选为独立参考预算，不等同生产参数或普遍Desktop性能证明。总阶段仍独立3分钟，包含完整快照、所有答案预检、编译、逐点I/O/执行/比较，可能在各点均未超限时总体失败；CLI4分钟/顶层测试5分钟不变。单次Run的上传/回读/执行/stdout下载共享wall+5秒即15秒，curl20秒/Host25秒不能作为额外时间相加；失败不自动加时或重试。

物理复用原medium完整配置及canonical锁，global单文件RLIMIT_FSIZE仍8MiB、默认copy-out1MiB，tmpfs/shm/cgroup/Stop与原六profile保持。Files仅此来源payload1GiB；原file48MiB/live64MiB/4对象/512操作不变。最坏累计载荷`2*474 + 2*412885310 + 20*(8388608+1) = 993543748`B，127次execution逻辑操作不等同Host请求数。input48MiB+stdout预留8MiB+1B+exe预留1MiB的逻辑峰值为57MiB+1B，低于64MiB；不等同multipart/缓存/工作目录的物理峰值，也不证明std::map百万节点低于128MiB。

普通测试覆盖含catalog的512MiB精确/+1、实际超过256MiB的匿名冻结、48MiB输入/8MiB答案精确/+1、共享快照quota释放、完整20点计费和1GiB注入边界、已知清理/档位互斥及旧六profile摘要。匿名文件只写源码和逐点材料，物理长度不包含两份已核验catalog；1GiB计数注入不冒充实际搬运。专用驱动先同次验证完整48MiB stdin→完整8MiB stdout/4096B stderr，记录存活资源样本并完成清理/Stop，再运行完整20点CLI；原48MiB→小输出和16MiB→8MiB不能替代这一组合。一次真实两组21.31秒通过，完整20AC/Stop；随后独立原六来源完整点集/边界故障回归通过，完整旧Plan/Profile保持。仅支持该固定环境维护入口，不更新数据库、生产配置或发布状态，不证明历史故障修复或长期稳定；证据归[交付记录](delivery-log.md)。

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

### convolution_mod参考验证

固定来源`library-checker:convolution_mod`的完整53点材料已逐哈希核对；输入合计211547577B、最大10485774B，答案合计215043482B、最大10369461B，参考源码3284B且仅使用标准头。`ReferenceConvolution=8`使用独立profile/runtime/files身份：含catalog完整材料512MiB、input/answer各16MiB、stdout/checker16MiB，source64KiB/64点及诊断/token4KiB保持。Run CPU5秒（固定info.toml的5.0秒）、wall10秒、memory128MiB/16进程；总阶段3分钟和每Run共享wall+5秒仍可先行拒绝，不代表53次最大wall均能完成。

单file48MiB/live64MiB/4对象/512操作不变，payload仅此档位为2GiB。按请求cap最坏累计`2*3284 + 2*211547577 + 53*(16777216+1) = 1312294223`B、325次execution逻辑操作。合法input16MiB+stdout预留16MiB+1+exe预留1MiB为33MiB+1；通用48MiB输入与该输出预留超过64MiB，应在运行前拒绝，不扩大存活预算。

该来源使用独立`oj-reference-convolution-sandbox`与`slot-reference-convolution.lock`，固定Cmd的output-limit为16MiB，其余镜像和隔离不变。旧medium槽的8MiB全局文件上限不能容纳此题，不修改其配置。真实专项先核验同次16MiB输入/16MiB原始输出（含非UTF-8字节）/stderr4096，再检查stdout cap+1的完整SIGXFSZ窄证据与stderr4097拒绝，任意附加清理/取消错误失败；停止确认后才运行完整53点。HEAD `f04dfdfe7d3f8fb3abdfcb61518e2fe176577291`的一次十来源254点/26子测试接纳已通过：同次16MiB输入/16MiB输出/stderr4096、stdout cap+1窄SIGXFSZ及stderr4097严格拒绝均成立，随后完整53点编译一次、全部AC/Stop；新Plan/Profile与独立未执行题源的准备基准相等、旧九来源完整Plan/Profile保持。证据及限制见[交付记录](delivery-log.md#2026-11-04convolution_mod-十来源固定环境接纳)；不改数据库not-run/生产null/publication blocked，最终CI/合并状态见交付记录，未部署。

### 卷积小规模独立对拍

维护者试验 `validate-convolution-differential` 固定为 `library-checker:convolution_mod`。源码根只读取锁定提交中的 `sol/naive.cpp` 和 `sol/correct.cpp`，在启动前核对原锁字节、文件类型、大小和 SHA-256，冻结两份内容，不接收源码替换或任意命令。朴素双循环累加与候选的蝶形变换独立；正式 53 点及其答案不参与对拍。

固定语料 v1 共 24 点，含模域边界、手算锚点和固定种子的有界用例。维度每边至多 16、输入及可比较输出各至多 512 字节。两边必须输出恰好 `n+m-1` 个模域内十进制非负整数。前导零和 ASCII 空白不改变系数；越界、截断、非整数和缺少/额外系数均不是合法差异。报告记录种子和有序输入 SHA-256，可从固定版本重建语料。

在同一共享宿主锁和卷积槽内先编译、运行朴素解并清理制品，三条手算锚点须一致；再编译候选解并按同序运行，首个合法差异中止。两个适配器共用 Host 传输预算；保持原卷积档隔离和文件上限，整个对拍执行期限至多三分钟，启动与独立 Stop 另有有界期限。朴素解或清理失败不得运行候选。取消、异常执行、停止不明或清除隔离失败不能通过。

私有目录权限为 0700，报告为 0600。报告包含两份源码身份、语料、Profile 与槽生命周期；仅在合法差异时保留实际反例输入、输入哈希和两端系数。开始后仍未能确认终态时保留 `INCOMPLETE`，有容器身份则保留可用的停止证据。专用测试中的固定错误解有独立报告 schema 和实际执行的合成源码哈希，不能当作上游候选的报告。

本试验只提供一个固定来源的 AC05 分项证据。维护命令不访问数据库、不自动重试；`publication` 始终为 `blocked`。小规模一致不能证明最大规模正确，也不代替题面、完整数据质量、非作者终审或原子发布。

### 已入库草稿的卷积对拍材料

内部 `FreezeDraftConvolution` 接收此前经作者权限读取的草稿快照和受信目录选定的导入身份。读取对象前重算完整导入修订，核对草稿 ID、归属、内容版本、来源锁以及两个解法在草稿引用和固定来源锁中的角色、大小与哈希。然后只读两个不可变对象，顺序关闭快照；任一对象损坏都拒绝整对材料。正式测试点和答案不会打开。

由这对材料生成的私有报告会带有 `draft.id`、`draft.version` 和 `draft.import_revision`，不会带上作者身份或解法源码。原源码根模式报告省略 `draft`。草稿绑定只描述材料出处：调用方仍须在使用时复核实时授权；此阶段未接作者入口、Worker 调度或可信报告接纳，草稿仍为 `not-run`。

## 执行失败诊断

2026-11-04的[停止故障调查](reference-validation-investigation.md)保留新主线独立50点、预注册前置序列未复现及旧stdout删除访问日志异常时间差的历史证据。旧根因仍未知，HTTP200不能证明旧客户端成功；后续吸收#123的新版本已完成[八来源接纳](delivery-log.md#2026-11-04point_add_range_sum-八来源固定环境接纳)，不追认旧失败。

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

范围只覆盖RoundTrip控制请求（GET元数据、POST/run、DELETE），不覆盖Host.Start/Stop/Verify或独立文件传输，也不能回溯已成功返回的输入回读内部耗时。Host成功返回、随后GoJudge JSON decode失败时没有Host trace，不通过成功响应侧通道补造证据。诊断字段本身不是故障修复、自动准入或重试授权；#114与#119原失败保持，具体版本的准入状态见[最新接纳记录](delivery-log.md#2026-11-04convolution_mod-十来源固定环境接纳)。如需继续调查，应另行预注册可检验假设。

## 槽所有权与停止

Stop总期限仍取调用方期限与10秒中的较早者。drain与首次容器生命周期核验成功后，进程枚举和kill分别使用独立子期限：最多2秒，且最多使用进入该阶段时总剩余时间的一半；每条本地命令返回并取消其子context后，才派发下一条命令。后续状态/PID确认仍受原总期限约束，不重试或增加总预算。此内部时间分配为终止及确认留出机会，可能使原本能在10秒内完成的慢命令提前失败；本地WaitDelay及调度仍可能耗尽预留时间，不保证信号送达或绝对墙钟上界。

首次生命周期核验失败不发信号；枚举失败时对已核验生命周期仍尝试kill，但缺少进程证据始终返回失败、保持隔离，不能仅因容器停止就清除标记。历史#114的kill/deadline没有本地启动分段证据，本改进不认定其旧根因；#114此前八来源接纳已交付；吸收前置后的#119版本另经九来源完整接纳通过，历史失败保持。

显式`OJ_SANDBOX_EXPERIMENT=1`和跨checkout文件锁仍是前置条件。默认A+B使用primary/secondary；unionfind使用固定small reference槽，zalgorithm、suffixarray、point_add_range_sum、primality_test、staticrmq、static_range_sum与associative_array共用固定medium reference槽及同一锁，均拒绝非空`OJ_WORKER_M0_SLOT`，不从M0 parser选择新槽。quarantine不自动恢复。命令先持锁并同步dirty标记，再接触沙箱。`--start-stopped-slot`只启动预存在、停止/PID0、固定owner及完整隔离配置匹配的槽，使用原full ID，不创建、替换、重启运行中容器或接管其他项目。返回Host绑定该ID与新StartedAt；运行中每次请求继续校验同一生命周期。

正常退出和失败均在独立context中停止并排空Host，核验容器停止及已观察PID消失；只有确认后才清理marker。启动回执未知时仍尝试确认原生命周期停止；拿不到绑定Host、停止未确认或marker同步失败则保留隔离。启动/验证报错后若已确认停止且清标记成功，可以解除marker，但本次报告仍失败。原[受控恢复](../README.md#worker-异常后的受控解封)步骤继续适用。取消发生在Run、制品清理或Stop期间都不得生成通过状态。不会为了保存报告恢复旧槽或再次执行未知操作。

独立槽异常时使用`OJ_SANDBOX_EXPERIMENT=1 worker recover-reference [<admitted-source-id>] <original-full-container-id>`，必须清空`OJ_WORKER_M0_SLOT`并先确认旧宿主进程及在途操作已排空。省略source保留原unionfind/small默认；显式`library-checker:zalgorithm`、`library-checker:suffixarray`、`library-checker:point_add_range_sum`、`library-checker:primality_test`、`library-checker:staticrmq`、`library-checker:static_range_sum`或`library-checker:associative_array`只选同一medium槽；显式`library-checker:convolution_mod`只选其独立16MiB输出槽。A+B或未知source拒绝reference恢复。它只接来源对应的固定槽，核验原ID/配置/停止生命周期，成功才清该锁；不启动槽、不认领任务、不追认失败报告，不能手工清marker。

此前一次staticrmq观测全AC且成功省略诊断，不能支持或反驳原失败位置假设；吸收已交付前置后的新版本已通过九来源独立接纳，该成功仍不解释旧失败根因。

Host的文件能力与原M0 RoundTrip分开：仅固定loopback文件路由、有限私有bytes reader、随机MIME boundary及有界sink；不接受任意URL、主机路径或流式Body提前交还。HTTP状态使用固定四字节trailer与二进制正文分开校验，子进程和全部IO结束后才释放active登记；取消不证明远端文件或进程已清除。M0默认入口、原JSON预算与Runtime保持。

Stop内部仍为10秒总预算，保持原调用顺序和零重试。枚举/解析失败仍尝试kill；同时失败时优先记录kill诊断，不能由此推定枚举成功。每次探针成功后才递增confirmed，首个失败立即结束。`pid_probe/pid_present`仅表示该时刻PID路径存在，可能是PID复用，不证明原进程仍活着；`proc_unavailable`表示无法建立procfs前提。探针唯有明确“不存在”才成功，表达式/命令异常仍失败；超时不能被当作PID存在或退出证明。宿主命令的既有128KiB＋1字节输出界限由显式组合缓冲执行，避免标准库ReadFrom绕过限长Write。

数据库中的草稿仍为`not-run`，既有来源准备报告不充当本站运行记录。十题维护参考验证已通过，持久化报告接纳见[参考申请合同](reference-requests.md#参考报告接纳)；后续须完成完整质量验证、非作者终审和原子发布。

首轮真实专项曾完整通过，但另两次实测出现停止确认未通过：包括额外12点全部AC后仍返回SE、保留隔离。后续受控recover仅确认当时安全，不追认原报告；旧报告没有新诊断，不能用当前代码反推其失败原因。后来[PR #114](https://github.com/DL-Association-for-Computing-Machinery/OnlineJudge/pull/114)的旧suffixarray回归又出现第11点SE、Stop kill/deadline且原容器仍运行；外部停止和受控恢复不改变原失败结论。当前已补停止步骤与执行失败阶段诊断；原根因尚未定位，后续新版本已通过十来源固定环境维护接纳，长期稳定性仍未知，不自动重试至绿或放宽Stop。历史与本次证据见[交付记录](delivery-log.md)。
