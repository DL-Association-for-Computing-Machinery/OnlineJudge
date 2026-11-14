# P7 Java 21 本机验收

本机已准入并发布的文本题可选择 Java 21，粘贴或上传单个 UTF-8 `.java` 文件（最多 64 KiB）。程序入口为无包名的 `Main` 类；允许该文件内的其他类。源码、独立语言草稿、提交回执、本人历史和详情沿用原流程。

在专用 Ubuntu arm64 VM 沿用原断网、只读、无宿主挂载及 Docker/cgroup/seccomp 自检。镜像从既有固定 GCC 执行制品和 [Eclipse Temurin 官方镜像](https://hub.docker.com/_/eclipse-temurin) 的 arm64 摘要构建，见 [Dockerfile](../../../deploy/experiments/go-judge-java.Dockerfile)。构建前核验 `oj-m0-sandbox:local` 的固定镜像 ID；本机镜像从审核过的制品导入，不用可变标签代替 Worker 固定的最终镜像摘要。运行时仅允许镜像里的固定脚本：`javac --release 21 -proc:none` 将 `Main.java` 编译后打成一个有大小上限的 JAR，每个测试点启动新的解释执行 JVM，堆上限 64 MiB。JAR 直接输出到约定文件，沿用原沙箱规则。Java 运行档位 CPU 1.5 秒、墙钟 5 秒、总内存 192 MiB、进程/线程上限 32。

题目材料摘要继续核对冻结测试、文本检查器及原参考验题档位。启用 JDK 的 Worker 使用另一固定执行镜像：C/C++ 保留编译命令和预算，Java 使用独立预算。该镜像下每笔判题的证据摘要额外绑定实际执行档位（含镜像和编译/运行配置），无 JDK 的旧 Worker 证据格式不变。

启用顺序：排空并停止旧 API、Worker 和维护写入，迁移到 schema 32；按原 Ubuntu 槽配置准备使用新镜像的停止容器。启动 Worker 时加 `OJ_LOCAL_JAVA21=1` 并核对就绪，然后在本机普通会话配置启用 `local_java21: true`（要求已启用本机提交）并用 `VITE_OJ_LOCAL_JAVA21=1` 构建本机前端。浏览器夹具用 `OJ_BROWSER_LOCAL_JAVA21=1`。同一队列的 Worker 必须一致配置，旧 Worker 不可与 Java 队列混跑。停用先关闭 Java 入口、排空已接收任务；历史仍可读。已有 Java 记录时数据库拒绝 Down32，并保留历史。

2026-11-15 用临时 PostgreSQL、Redis、回环 HTTPS 和本机专用 Ubuntu arm64 VM（内核 6.8.0-106-generic）重现已发布 A+B 版本 `fff51a78441424bbd2e646e839f81490826c63ad588e3b98edc19eb8d679d4c1`。Chrome 普通账号实际上传 `.java` 文件；真实 mTLS Worker 使用 Temurin 21.0.13、go-judge v1.13.3 判得 Java AC、WA、CE。Java 正解使用同一源文件里的 record，检验同时打包多个 class。C17 和 GNU++20 回归 AC、WA、CE，GNU++17 正解 AC，且拒绝同一份 GNU++20 concept 正解（CE）。十一笔均核对终态、语言、本人原始源码及一次真实 Worker 尝试，另一账号不能读详情或回执。夹具和沙箱均已关闭。

报告 SHA-256：Java `8cca7e9bb820aefd732288d40b77d0725688051f7ff1e2593d5760a207577327`，C17 `74259af46379400ed61a3a9945c7841878609a8a6d27798033201dd4b401c453`，GNU++17 `aacd2113512386fdd5bc315d1dda2ac8735ba7f21c4f1182e3511d2901e62b26`，GNU++20 `f1b82875fa3159a9c043756f2ee81e90531fa947d1a9691bdd8c8f13dd487e68`；数据库核对 `ab007c2bcc650f29e157f7d59f9396b01450ec9c6cb57919252fb4370db8fbaa`。

## Java 内存边界、接管、重判与双 Worker

2026-11-15 在同一专用 Ubuntu VM 用临时数据库复现同一 A+B 发布版本，两个终端以独立证书和执行槽启动真实 Worker。仅补 Java 特有的两种内存用例；本轮未重跑通用 CPU、输出超限矩阵。

| 项目 | 实测 |
| --- | --- |
| JVM 堆耗尽 | 固定堆上限为 64 MiB，申请一个 80 MiB 数组。独立执行器诊断报告 `java.lang.OutOfMemoryError: Java heap space`、`Nonzero Exit Status`、退出码 1，内存为 17,915,904 B，低于沙箱的 192 MiB。真实 Worker 判 RE。 |
| 沙箱总内存超限 | Java 标准库压缩器持有的堆外缓冲使总内存超过 192 MiB。相同源码和固定档位在独立探针报告 `Memory Limit Exceeded`、退出码 9，内存为 201,342,976 B。真实 Worker 判 MLE。两个判定均由本人读回；判题逻辑未按 stderr 字样猜测 MLE。 |
| 同机双 Worker | 同一查询观察到两个不同节点的有效租约，与两笔相同 Java 源码的浏览器提交对应。两节点第一代各完成一次 AC。 |
| 运行中退出 | 确认选手 JVM 已运行后给持租约的 Worker 发 SIGTERM。沙箱停止、PID 归零，首次尝试无结果。未编辑队列或租约；租约自然到期后另一节点在同一判题代次以尝试 2 判 AC，本人读回。 |
| 历史重判 | 独立维护角色的错材料摘要拒绝且无审计；正确摘要入队、原键回执一致。原提交获得第二代 AC，源码、语言、材料摘要和第一代结果及结果摘要逐字节不变，本人再次从浏览器读回。 |

数据库对账共五笔独立提交，接管和重判均未新建提交。记录 SHA-256：原始内存诊断 `0b77ea2f57ec16bf157dcb5ae2a324268004ca277951247075240c796ae9172d`，浏览器内存报告 `2c07f8397debf7a76d8f0bd3a4532b02835579f3553124b8839de892511135d4`，同时有效租约 `010bfa5b4d5e7870f9389f7f65ffdc0600a3ef46d5320f36ed3a5b41b958e6ea`，运行中退出 `712730790c62e4f8e1cfbff71ce7262fdef8e09da95cee2612c6838c5d6cbd7c`，重判回执 `9b2e761cba5cfb6f7a02f7e6559f7b3187fe95672178617e7f0025d9c57c8ece`，最终对账 `4a6f8d8087eaf29d0792a59baac6621074791104ab6c5b8943fd135b1bf98abd`。

复现使用 [README 的双 Worker 流程](../published-training-checks.md#本机已发布-ab-浏览器联验)，两 Worker 均启用同一 Java 镜像。浏览器脚本设 `OJ_BROWSER_TRAINING_LANGUAGE=java21`，沿用 `OJ_C17_PHASE` 与临时 JSON 路径。故障验证覆盖同机 SIGTERM 接管，未做 SIGKILL、跨主机故障或归档恢复。两槽和临时夹具均已关闭，未部署、未访问试用数据。
