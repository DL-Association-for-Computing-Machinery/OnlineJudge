# A+B 固定材料租约联验

使用[上一阶段冻结的 17 点判题清单](aplusb-isolated-judging.md)，在 macOS arm64、Docker Desktop 的固定断网小型参考槽以及一次性回环 PostgreSQL 14 上执行 `TestAPlusBLeasedLoop`。

| 联验输入 | SHA-256 |
| --- | --- |
| 原判题清单 | `a79d761e04d33da6714c987ab0512e3deec4a28531c839f00c95347442437f91` |
| 运输清单 | `d896b91d12727acc290e98f6cd8b0944d590efa905efaa1c84ef7cbe4a2f4a13` |
| 执行档位 | `dfe2ee521a3102051494301c829524105991c030732f15afb910803723461725` |

两次导出逐字节一致；17 点引用的 32 个不同私有对象均与文件名摘要一致。空对象目录的登记失败且未留下记录；完整材料重复登记成功。mTLS 控制端拒绝旧 Worker 的版本声明，拒绝时没有创建尝试。单个 Worker 依次处理三个合成任务，回报 AC、WA、OLE（超过 12 字节的单 token）；每份成功回报均为第一次尝试，数据库保留相同版本和清单摘要。测试在 10.09 秒完成，Worker 排空后该槽恢复至原始的 `exited`、PID 0 和 `none` 网络，临时数据库已清理。

本次仅证明临时数据库和固定本地沙箱间的链路，任务由联验直接经受信存储排入。目标节点、公开提交、发布审批和跨环境历史重判仍需各自验证。

## Ubuntu arm64 专用客体

2026-11-14，在本地 Apple Ubuntu VM（Linux `6.8.0-106-generic`、无宿主共享、原生 cgroup v2 Engine）和一次性回环 PostgreSQL 14 实测同一方案：

| 联验输入 | SHA-256 |
| --- | --- |
| Ubuntu 判题清单 | `a2aac16ff12078b51106b6a26ed374aa89dbe386152742d6c1aeee2f189983bb` |
| Ubuntu 运输清单 | `337c7a52705e4a8ff77c8d8ee03388563c3b54e80ed1b34b1d2a72e0863e4640` |
| Ubuntu 执行及缓存档位 | `ab95db1dc40ed24c464ea108f7bcb6a8df3f83e68ddbc552703b8d5c2e883256` |

两次导出完全一致，32 个对象均与 Desktop 同名对象逐字节相同。原 Desktop 运输摘要不变；使用 Desktop 判题摘要导出 Ubuntu 运输材料在创建目录前失败。临时库验证缺失对象和 Ubuntu 清单配 Desktop 默认运行设置均不能登记；合法登记可重试。真实回环 mTLS 控制端拒绝旧 M0、真实 Desktop A+B 版本及版本相同但运输摘要不同的 Ubuntu 声明，数据库在三次拒绝后为零尝试。

同一个受管 Worker 回报 AC、WA、OLE，均为第一次完成尝试且绑定同一版本和运输摘要；`TestAPlusBLeasedLoop` 用时 5.40 秒。Worker 排空后，原容器确认 `exited`、PID 0、`none` 网络，隔离锁为空；本轮自有沙箱和临时数据库均已移除。复现见 [README](../../README.md#ab-租约链路联验)。本次未在真实租约丢失下中断 Ubuntu 编译，没有执行 Ubuntu 历史重判或接触试用服务、公开 API 和发布流程。

同一实现另在 Docker Desktop 用原运输摘要回归 `TestAPlusBLeasedLoop`，10.22 秒通过；三组结果仍各一次完成，原槽停止、PID 为 0、锁为空，临时数据库已移除。
