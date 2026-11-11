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

## 后续接入条件

现有草稿仍为 `configured-not-validated` / `not-run`，`production_profile` 和 `limits` 仍为 null。目标机器实测还须验证显式预算下的真实参考/错误程序、受限编译执行、输出采集及故障分类，然后由有权限的验证、非作者审核及原子发布流程使用准确的候选摘要。离线预检只验证冻结答案，没有连接 Worker 或数据库，也不解除发布阻断。
