# 首批 Library Checker 文本检查器组件准入

本页记录 P4 的文本语义组件。`internal/judging.CompareLibraryCheckerText` 使用独立版本 `lc-wcmp-tokens-v1`；现有 M0 `CompareText`、Worker 固定材料门禁和草稿格式不变。该函数已供[首份草稿的独立参考验证](reference-validation.md)使用，尚未接入生产Worker或数据库验证任务；不执行Testlib，不解除任何发布阻断。

## 支持域与错误

固定[来源锁](../config/problem-sources/library-checker.json)的十份 `checker.cpp` 均为相同的大小写敏感 `wcmp`，并非整数数值比较。本站新模式仅以 SP/TAB/LF/CR 分隔 token，逐字节比较，不做数字或 Unicode 规范化。VT/FF 是 token 内容；M0 仍把它们当空白。`01` 与 `1`、`YES` 与 `yes` 均不等价。

调用方必须显式提供 `TextLimits{Bytes, TokenBytes}`，分别约束每份答案/输出和单 token。配置须满足 `1 <= TokenBytes <= min(Bytes, 65536)`、`Bytes <= 128 MiB`；零值、负值及超界为 `ErrTextLimits`，不隐式扩容或裁剪。完整参考答案先验证，再考虑输出错误或失配；非法 UTF-8、参考文件/参考 token 超限为 `ErrReference`，输出文件或有效 UTF-8 token 超限为 `ErrOutput`，非法 UTF-8 输出拒绝为失配。这些错误不能直接归为 WA，未来执行适配须分别处理参考异常与选手输出超限。

支持域有意比锁内 Testlib 窄：只接受有效 UTF-8，单 token 至多 64 KiB；上游可以接受部分非法编码和更长 token，且原始 `0xFF` 有 EOF 特殊语义。不得声明与上游任意字节全域等价。128 MiB 是组件硬上限，差分实验显式使用每文件 16 MiB/每 token 64 KiB；二者都不是生产选手输出、内存、运行时间配置。

该入口接收已分配的 `[]byte`，只扫描并比较，不拷贝整份数据。调用方仍需在读取/分配前施加字节、活动并发与总内存限制；本次不声称解决大材料的流式读取、进程隔离或取消预算。生产参数仍须按实际题目与节点实测后配置。

## 独立差分范围

显式 `checkercompat` 构建标签下的[测试](../tests/checkercompat/library_checker_test.go)从锁定 `hash.json` 独立核验全部输入/答案，对十份 checker 和库头文件核验锁内哈希；不用来源准备的 `validation.json` 作为本站运行通过证据。先全量检查 254 份答案均在声明支持域内，再在专用容器内编译锁定 `wcmp`，对每份答案执行自身相同、添加四种空白、额外 token、空输出四种对照。额外覆盖 VT/FF、大小写、数字拼写、Unicode、NUL 和已知域外差异；上游非 0/1 返回码不冒充 AC/WA。

只为同源 checker 家族复用一次编译，若十份源码不再相同则拒绝，必须重新拆分准入。套件先核对固定提交；锁摘要、检查器版本、覆盖数量、最大答案、超 M0 预算数量和显式限额写入测试日志。单元回归另覆盖配置、字节/token 边界、完整参考优先级及原 M0 行为。

完整运行命令归 [README](../README.md#首批文本检查器差分)，只允许显式本地隔离 Docker 专项；普通 CI 不执行第三方程序。固定工具链、实际测量及本次结果归[交付记录](delivery-log.md)。固定测试集合的一致不证明所有支持域输入等价，也不构成恶意题源隔离认证。

## 后续接入条件

现有草稿仍为 `configured-not-validated` / `not-run`，`production_profile` 和 `limits` 仍为 null。后续须把明确的检查器版本和实测预算绑定不可变配置，验证有界材料读取、真实参考/错误程序的受限编译执行及故障分类，再接当前权限下的验证记录、非作者审核和原子发布。本页的组件证据不能替代这些要求；回退本 PR 只移除未接线的新入口和专项，不涉及数据库或既有草稿迁移。
