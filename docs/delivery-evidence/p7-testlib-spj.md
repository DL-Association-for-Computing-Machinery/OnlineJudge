# P7 Testlib 协议与隔离验收

首个验证基线为 [Testlib 0.9.45 固定提交](https://github.com/MikeMirzayanov/testlib/blob/2d20123984e9479b8a56ebe0d6a51e23ad7c35b3/testlib.h)。已有题源语料内的 0.9.40 快照仅用于既有文本比较对照。

本站区分原生 ICPC 与下游 Testlib 协议。两者均须接收限额内真实的进程正常退出；信号、超时、资源超限、文件采集错误和缺失回执均为检查器系统错误。原生 ICPC 的 42/43 与下游 Testlib 的 50/51–54 分开解释，退出 0 和未知码拒绝。诊断只用于排查，不参与结果映射。

[头文件准备脚本](../../scripts/prepare-testlib-spj.py)校验独立 SHA-256 及固定上游 Git blob，保留许可，标记修改。下游适配在额外输出检查之前把评分关闭：上游的部分分退出码可按参数计算，且可能先因额外输出被折成格式错误；单独调整基础退出码不足以隔开这些情况。只有适配过的版本和固定编译参数可配合本站解码器。

合成检查器分别覆盖非唯一答案和部分分。测试应以三个固定文件提供当前点输入、选手输出和参考答案，诊断预算独立于选手程序。上游文件不可用时，Go 合同测试只证明结果证据验证与模式映射，不构成 C++ 构建、沙箱兼容或发布验收。

复现探针时在专用 Ubuntu arm64 VM 中创建专用停止容器，沿用 [Python 组合镜像和 Ubuntu 槽](p7-python3.md)的断网、只读、无挂载及资源配置，所有者标签使用 `owner=onlinejudge.p7-testlib-probe`。启动后，以 VM 管理用户执行 `python3 -B scripts/probe-testlib-spj.py CONTAINER_ID ORIGINAL_HEADER EXPECTED_SHA256`。仅转移该脚本、准备脚本及本页的两个合成检查器到 VM；结束后确认专用容器停止并释放。此探针不接收任意检查器源码，不进入正式发布队列。
