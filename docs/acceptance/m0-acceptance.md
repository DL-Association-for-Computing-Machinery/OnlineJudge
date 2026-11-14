# M0 限定环境技术验收记录

2026-11-03 对主线 `da7ef2c5739b76384225d507bab195c47d1d7fd1` 逐项核对：**开发计划明确要求的 M0 技术条件，在固定合成实验范围内满足**。这是技术核对结论；没有记录用户产品验收或发布决定，不表示完整 P1/P2、M1 训练试用、生产准入或部署通过。

范围为专用 Docker Desktop LinuxKit 7.0.14 / arm64、固定 GNU++17 文本题与受控主体，材料 `m0-sum-v3`。当前实际 Docker 29.8.1 分配10个CPU、8319504384字节内存；这是本次只读环境读数，不倒填旧测试条件，也不是目标生产硬件。版本和镜像身份、每项 owning test、原始日志哈希、源码适用范围及复验入口集中在[机器可读证据索引](evidence/m0-20261103.json)。原始日志仅存本机，未作为仓库或 CI 附件；环境迁移前须另行保全。

## 条件与证据

原验收条款保持在[开发计划](../development/phase-1-development-plan.md#m0-关键技术验证通过)，没有为本结论删减要求。以下编号对应索引中的 `requirements` / `evidence`，具体实现与历史仍归[M0实施记录](m0-experiment.md)。

| 技术条件 | 已核对证据 | 结论和边界 |
| --- | --- | --- |
| P1 接收、幂等及事务 | `P1-receipt-and-transaction`：原键同内容/冲突、未知回执核验、失败整事务回滚、无孤立任务 | M0子项满足；受控实验身份，不是训练授权 |
| P1 任务租约与结果 | `P1-lease-and-result`：竞争认领、双代次、重复/冲突/迟到回报、三次系统失败后failed且无选手verdict | M0子项满足；恢复时限不等于SLA |
| P1 SQL、连接与迁移 | `P1-query-pool-and-migration`：真实查询计划、有限连接池/领取、执行前释放事务、迁移并发锁/超时/回退 | M0子项满足；小夹具索引证据不代替大队列负载 |
| P2 判定与文本规则 | `P2-verdict-and-text`：AC/WA/CE/RE/TLE/MLE/OLE/SE、token/空白/ASCII大小写、未知证据失败关闭 | 满足固定文本题范围；不为未识别执行器分支猜判定 |
| P2 尝试顺序、隔离和预算 | `P2-ordered-attempt-and-cleanup`、`P2-budgets-and-isolation`：编译一次、逐点新环境、首失败停止、NOT_RUN/无虚构usage、资源越界/恢复 | 满足固定profile；GNU++20/其他语言/浮点/SPJ按后续阶段交付 |
| 双API/Worker、材料及节点故障 | `M0-dual-api-worker-and-faults`：真实竞争、自然租约到期、撤销、材料缺失/损坏、网络/回报丢回执、停止/PID及槽恢复 | 满足同机多实例实验；不是跨主机高可用或自动执行端重启 |
| P3 会话关键实验 | `M0-session-experiments`：独立API进程、跨实例撤销/名额、Redis/PG故障及旧备份门禁 | 满足关键实验；不能恢复备份后的账号安全事实或许可生产复服 |
| 真实请求与最小结果页面 | `M0-real-request-and-minimal-browser`：HTTP queued→真实Worker→终态、浏览器真实丢POST响应/原键恢复、桌面/390px | #92已合并；固定服务端AC/WA示例，nonce不是账号身份，查询503明确为注入 |
| 组件、资源及失败恢复记录 | `M0-admission-resources-and-recovery-record` 及下文 | 满足现有隔离合成实验的限定准入；不是镜像分发/生产准入 |

P1的共享用户/全局额度与公平调度仍未实现。原计划明确要求在**试用开放前**落实，索引将其列为 `later_trial_gate`，没有将整个P1标为完成。P4题源/录入入口未决；本次不替用户选择。普通账号训练、完整题库/历史/权限、生产目标负载、SMTP/HTTPS及独立备份仍按各自使用节点验收。

## 源码适用范围与实际验证

- `2b44a50` 的数据库、普通协议和独立会话日志哈希已回读；索引列明的17个 owning 目录/文件至当前主线无差异。`cmd/api` 浏览器包装另验，不混入“未变化”清单。
- `742d77f` 的九项Worker联合命令退出0，9个独立PASS、106个含父条目PASS，基础8个SKIP随后各专项实际通过；16个执行/判题/Worker/材料/数据库及资源测试 owning 路径至当前主线无差异。新增浏览器包装和HTTP测试helper通过下面的独立链路核对，不仅为统一SHA重跑相同套件。
- 浏览器11.95秒/12项实测的18份运行代码/测试/契约哈希与当前主线一致；Playwright1.62.1、Node22.17.1、Chromium145.0.7632.6。SQL只旁证固定源码/归属及唯一completed attempt；排空exit0/槽停止有断言，没有独立选手PID观察。完整后端、66项治理及277处文档目标检查均有本机证据。
- #92精确head `26614038e48139cbb9c9f26a7c9b78a9d73ca03c` 的[CI 41550599046](https://github.com/DL-Association-for-Computing-Machinery/OnlineJudge/actions/runs/41550599046) 九项全部成功，合并树与当前源码一致。CI包括真实PG/Redis普通协议，**不执行**特权Worker和浏览器实验；独立会话行为套件也不能由普通root测试冒充。

## 固定组件准入结论

现有隔离合成实验继续使用固定版本和预算。Go/Gin/pgx/Goose/sqlc、go-redis、SCS及go-judge的既有许可/构建证据见[M0组件记录](m0-experiment.md#组件准入与下一步)，本次补核服务端与工具链的固定上游原文。索引保存版本标签解析出的不可变commit、Git blob及SHA-256；没有仅按客户端许可推定服务端许可。

| 实际组件 | 固定原文与适用说明 | 原文SHA-256 |
| --- | --- | --- |
| Redis 7.2.17 | [该版本COPYING](https://github.com/redis/redis/blob/83937c51afb53e1c69661c8e1533e0e19b896fe1/COPYING)：三条款BSD；不外推其他Redis版本 | `97f0a15b7bbae580d2609dad2e11f1956ae167be296ab60f4691ab9c30ee9828` |
| PostgreSQL 14.24 | [REL_14_24的COPYRIGHT](https://github.com/postgres/postgres/blob/6b3806732b7c5df06bdd0ed150e8a07b4ad62315/COPYRIGHT)：PostgreSQL License；本机可变tag当前读数14.24，不倒填早期日志小版本 | `3d6af92ff8a4c2cdf69afb1cf44edea727922f5cd0cf8b5f72b11cdecac8fdfd` |
| GCC 15.3.0 | [COPYING3](https://github.com/gcc-mirror/gcc/blob/4db0e8df15bef836558857c291c323add11d035c/COPYING3)及对应C前端源码许可头为GPLv3或后续版本 | `8ceb4b9ee5adedde47b31e975c1d90c73ad27b6b165a1dcd80c7c545eb65b903` |
| GCC运行库例外 | [COPYING.RUNTIME](https://github.com/gcc-mirror/gcc/blob/4db0e8df15bef836558857c291c323add11d035c/COPYING.RUNTIME)：版本3.1；另核对该版本libstdc++文件notice | `9d6b43ce4d8de0c878bf16b54d8e7a10d9bd42b75178153e3af6a815bdc90f74` |

运行库例外依适用文件的notice及编译条件生效，不是对整个GCC镜像、任意第三方文件或任何产物的通用许可。固定镜像digest/版本读数与这些上游文件构成当前实验来源记录，不证明镜像全部二进制可复现构建、完整传递依赖/发行版许可或对外分发义务已核验。完整OS许可、SBOM与镜像OS漏洞扫描仍按[安全计划](../development/engineering-checks.md#工具准入与维护)独立推进，不声称本次覆盖，也不新增为全部M0实验前置。当前安全证据限实际CI的秘密、Go可达调用及前端依赖审计，不能推出“无漏洞”。

## 资源数值和恢复前提

| 项目 | 固定预算或实测 | 口径 |
| --- | --- | --- |
| 编译 / 普通Run | 编译CPU10秒/wall15秒/512MiB；Run CPU200ms/wall2秒/32MiB；各16进程 | 配置见 `internal/experiment/environment.go`，不是吞吐测量 |
| 文件和输出 | 临时文件系统64MiB/4k inode；执行单文件1MiB；输出管道各4096字节；编译制品1MiB | go-judge全局采集限制与单请求输出/制品预算分开；Compile总容量探针不冒充普通Run预算 |
| 文本检查 | 参考与选手输出每侧1MiB、单token64KiB | `internal/judging/text.go`固定预算；参考越界为系统材料错误，选手越界为输出预算错误，执行层4096字节限制可先触发 |
| 外层槽容器 | 1 CPU/1GiB/128进程，禁网、只读根、私有PID/IPC，受控tmpfs | 服务及嵌套执行预算，不能由当前Docker分配10CPU推导10并发槽 |
| 资源耗尽观测 | 父+15子峰值16/EAGAIN并回收；4094空文件耗尽inode；分文件写满67108864字节；1MiB单文件SIGXFSZ/OLE | 直接探针及正式Run适配分开；各自恢复后继续执行已有断言 |
| 固定编译超限样本 | 原始非零exit4→CE，CPU17350000ns、内存9953280字节；随后同Runner AC | 单次失败观测，不是容量/峰值基准；未知Compile OLE仍失败关闭 |
| Store有限成本采样 | 42样本/86016请求，61440成功、24576预期名额拒绝，非预期类别0 | 8个在途、PG tmpfs/Redis持久化关闭；8份相关源码哈希仍匹配。完整逐轮数值与窗口局限归[既有采样记录](m0-experiment.md#普通会话-store-有限成本采样2026-10-31)，不是密码/HTTP登录、100人规模或SLA验收 |

恢复时必须保留以下合同：

1. 未知接收/回报回执按原键、原内容核验，不把未知当回滚；有限失败恢复耗尽后为系统failed，不发布选手判定。
2. 失权/未知槽生命周期先停止并隔离；可信管理员确认旧宿主操作排空后，以受信历史记录中的原完整容器ID显式恢复自有槽。恢复流程可停止仍在运行的该容器并确认已停；StartedAt仅绑定本次恢复观察期间的生命周期，不证明崩溃前的StartedAt。重启成功不代替PID/停止证据，也不保证自动重启或跨主机恢复。
3. 会话旧备份还原期间停旧API并换新恢复标识、闭门清理后再开。旧备份不能恢复其后的封禁/改密事实，不能用实验的会话废止结果批准生产复服。

操作步骤仍归[README](../../README.md)、协议和会话专题；本记录不复制另一套命令。当前可继续后续开发，训练/生产开放仍须对应的身份、发布、额度、负载和恢复验收。
