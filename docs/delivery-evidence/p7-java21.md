# P7 Java 21 本机验收

本机已准入并发布的文本题可选择 Java 21，粘贴或上传单个 UTF-8 `.java` 文件（最多 64 KiB）。程序入口为无包名的 `Main` 类；允许该文件内的其他类。源码、独立语言草稿、提交回执、本人历史和详情沿用原流程。

在专用 Ubuntu arm64 VM 沿用原断网、只读、无宿主挂载及 Docker/cgroup/seccomp 自检。镜像从既有固定 GCC 执行制品和 [Eclipse Temurin 官方镜像](https://hub.docker.com/_/eclipse-temurin) 的 arm64 摘要构建，见 [Dockerfile](../../deploy/experiments/go-judge-java.Dockerfile)。构建前核验 `oj-m0-sandbox:local` 的固定镜像 ID；本机镜像从审核过的制品导入，不用可变标签代替 Worker 固定的最终镜像摘要。运行时仅允许镜像里的固定脚本：`javac --release 21 -proc:none` 将 `Main.java` 编译后打成一个有大小上限的 JAR，每个测试点启动新的解释执行 JVM，堆上限 64 MiB。JAR 直接输出到约定文件，沿用原沙箱规则。Java 运行档位 CPU 1.5 秒、墙钟 5 秒、总内存 192 MiB、进程/线程上限 32。

题目材料摘要继续核对冻结测试、文本检查器及原参考验题档位。启用 JDK 的 Worker 使用另一固定执行镜像：C/C++ 保留编译命令和预算，Java 使用独立预算。该镜像下每笔判题的证据摘要额外绑定实际执行档位（含镜像和编译/运行配置），无 JDK 的旧 Worker 证据格式不变。

启用顺序：排空并停止旧 API、Worker 和维护写入，迁移到 schema 32；按原 Ubuntu 槽配置准备使用新镜像的停止容器。启动 Worker 时加 `OJ_LOCAL_JAVA21=1` 并核对就绪，然后在本机普通会话配置启用 `local_java21: true`（要求已启用本机提交）并用 `VITE_OJ_LOCAL_JAVA21=1` 构建本机前端。浏览器夹具用 `OJ_BROWSER_LOCAL_JAVA21=1`。同一队列的 Worker 必须一致配置，旧 Worker 不可与 Java 队列混跑。停用先关闭 Java 入口、排空已接收任务；历史仍可读。已有 Java 记录时数据库拒绝 Down32，并保留历史。

2026-11-15 用临时 PostgreSQL、Redis、回环 HTTPS 和本机专用 Ubuntu arm64 VM（内核 6.8.0-106-generic）重现已发布 A+B 版本 `fff51a78441424bbd2e646e839f81490826c63ad588e3b98edc19eb8d679d4c1`。Chrome 普通账号实际上传 `.java` 文件；真实 mTLS Worker 使用 Temurin 21.0.13、go-judge v1.13.3 判得 Java AC、WA、CE。Java 正解使用同一源文件里的 record，检验同时打包多个 class。C17 和 GNU++20 回归 AC、WA、CE，GNU++17 正解 AC，且拒绝同一份 GNU++20 concept 正解（CE）。十一笔均核对终态、语言、本人原始源码及一次真实 Worker 尝试，另一账号不能读详情或回执。夹具和沙箱均已关闭。

报告 SHA-256：Java `8cca7e9bb820aefd732288d40b77d0725688051f7ff1e2593d5760a207577327`，C17 `74259af46379400ed61a3a9945c7841878609a8a6d27798033201dd4b401c453`，GNU++17 `aacd2113512386fdd5bc315d1dda2ac8735ba7f21c4f1182e3511d2901e62b26`，GNU++20 `f1b82875fa3159a9c043756f2ee81e90531fa947d1a9691bdd8c8f13dd487e68`；数据库核对 `ab007c2bcc650f29e157f7d59f9396b01450ec9c6cb57919252fb4370db8fbaa`。

Java 的超限、故障恢复、历史重判与同机双 Worker 验收尚未执行。本轮使用一次性数据，未部署或访问试用环境。
