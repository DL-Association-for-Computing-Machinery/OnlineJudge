# P7 CPython 3.14 本机验收

本机已准入并发布的文本题可选择 Python 3.14，粘贴或上传单个 UTF-8 `.py` 文件（最多 64 KiB）。Worker 用固定 CPython 3.14.10 在沙箱内编译为 `main.pyc`，语法错误为 CE；每个测试点启动一个新解释器，使用 `-I -S -B`，未捕获的异常为 RE。源码不在编译阶段执行。字节码使用已有的有界缓存，运行复用同一文本题档位的限制和沙箱。题目冻结材料不变，结果证据另绑定实际执行档位、参数和镜像。

本机专用 Ubuntu arm64 VM 使用[组合镜像 Dockerfile](../../../deploy/experiments/go-judge-python.Dockerfile)，在已有 Java 21 和 GCC 镜像上加入[官方 CPython 镜像](https://hub.docker.com/_/python)的固定 arm64 制品。构建前核对 Java 基底为 [Java 21 证据](p7-java21.md)中的固定制品，以 `docker --context desktop-linux build --network none --provenance=false --platform linux/arm64 -f deploy/experiments/go-judge-python.Dockerfile -t oj-p7-python314:local .` 构建。保存后导入专用 VM；以 `docker image inspect` 验证最终镜像 ID 为 `sha256:e6b23d40935394adff7db3d21ac237a5921cc3b7e403acccc88158f759dfc2a3`。CPython 版本和常用标准库可在断网容器检查，实际选手代码仅在专用 VM 的固定 Worker 沙箱执行。

开启前排空并停止旧 API、Worker 及维护写入，迁移到 schema 33。按原 Ubuntu 槽配置准备使用该镜像的停止容器，Worker 设置 `OJ_LOCAL_PYTHON3=1`；回收该槽时同样设置此变量。Worker 就绪后才开启本机普通会话 API 的 `local_python3: true` 和前端构建变量 `VITE_OJ_LOCAL_PYTHON3=1`。同一队列用一致配置；组合镜像上的 Java 入口仍须独立开启。浏览器夹具用 `OJ_BROWSER_LOCAL_PYTHON3=1`，验收脚本用 `OJ_BROWSER_TRAINING_LANGUAGE=python3`，其余参数沿用 [A+B 流程](../published-training-checks.md#本机已发布-ab-浏览器联验)。

停用先关闭 Python 入口，再排空已接收任务；历史仍可读。没有 Python 提交时可回退 schema 32；已有记录时数据库拒绝 Down33，保留原提交。

2026-11-15 在专用 Ubuntu arm64 VM（内核 6.8.0-106-generic、go-judge v1.13.3）和临时 PostgreSQL、Redis 中复现已发布 A+B 版本 `fff51a78441424bbd2e646e839f81490826c63ad588e3b98edc19eb8d679d4c1`。Chrome 普通账号实际上传 `.py`，真实 mTLS Worker 判出 AC、WA、CE、RE；正解确认解释器为 3.14.10。同一镜像回归 C17、GNU++20 和 Java 21 的 AC、WA、CE，以及 GNU++17 的 AC 和对 GNU++20 concept 源码的 CE。15 笔全部核对浏览器回执、材料版本、终态及恰好一次完成的真实 Worker 尝试；本人能回读源码，另一账号读不到详情或回执。

Python 浏览器报告 SHA-256：`2d348bf652a29cb060ad385f951243c2a42148d50f53b3a5ebcbe663277c395b`；15 笔数据库对账 SHA-256：`f70ea738c1f41cc8299931408477f79eafbe04b443d4fefd15f17547ee5e16b9`。

## Python 超限、接管、重判与双 Worker

2026-11-15 使用同一发布版本和组合镜像，在专用 VM 的两个终端分别启动真实 Worker。每个节点使用独立 mTLS 证书和断网、只读、无宿主挂载的执行槽。Python 沿用文本题 CPU 1 秒、墙钟 5 秒、内存 128 MiB、标准输出 256 KiB 的限额。

| 项目 | 实测 |
| --- | --- |
| 超限 | 空循环判 TLE；申请并触及 144 MiB 的字节数组判 MLE；一次写出 400,000 字节判 OLE。三类终态均由本人从浏览器读回。 |
| 双 Worker | 对同一源码的两笔浏览器提交，同时观察到不同节点持有的未到期租约；两个节点在第一代各完成一次 AC。 |
| 故障接管 | 确认持租约节点正在运行 `main.pyc` 后发送 SIGTERM。专用沙箱停止、PID 归零，首次尝试无结果；没有改动任务或租约，租约自然到期后由另一节点在同一判题代次以尝试 2 判 AC，本人从浏览器读回。 |
| 历史重判 | 用独立维护角色提交错误材料摘要时拒绝且未写审计；正确摘要入队后原键回执相同。原提交第二代判 AC；语言、材料摘要、源码、第一代结果及结果摘要逐字节不变，本人读回。 |

对账覆盖 6 笔浏览器提交、8 次尝试（7 次完成、1 次过期）。SHA-256：超限浏览器回读 `2eb6cb901aae8482f899fc9a1bd546723a28b019fc03a3a076139d958ab6acfb`，同时有效租约 `62f9f609921eb6a66d68e6ce18c395f4faed7c79442741e5d05c9b3b69a3d52a`，运行中停止 `1959d646ea32e68677c2e4b35de35efe4e0e1013c82fd3ebb55d5f0f82622470`，重判回执 `c414b0c2c7884b70a9eb43a5105722df0cc799e74bb702dddc3bc45995b1216c`，最终对账 `f1400334017cee978d22f699e6556f1fa7c699a4d40de7975cdcd8356f0bcaf3`。

复现沿用 [README 双 Worker 流程](../published-training-checks.md#本机已发布-ab-浏览器联验)和[维护者重判](../../operations/rejudge.md)，两节点都使用组合镜像并启用 Python。故障范围是同机运行中 SIGTERM 与自然接管。两个执行槽和临时夹具均关闭；未部署、未连接试用数据。
