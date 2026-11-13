# A+B 固定材料隔离判题证据

2026-11-14，macOS arm64 Docker Desktop，既有断网小型参考槽。用同一 A+B 17 点私有修订、正式计划 `fe4e1968ae66dbfbe23afb96c03ed62dba8957d06e3b88602ebd653faf1c80f5` 和候选 `84a8b9d0e0a142d5efbe3bc1c410e449856d7fbc2abc598dbfb3178a377c673e` 生成两次清单，输出字节一致，清单摘要 `a79d761e04d33da6714c987ab0512e3deec4a28531c839f00c95347442437f91`。测试使用 262144 字节和 12 字节 token 上限。

| 合成提交 | 判定 | 私有报告字节 SHA-256 |
| --- | --- | --- |
| 读两个数并输出和 | 17 点 AC | `73fdad6330390c20704401af452dad39707bfea533bc47a73a1cfbc9f3aeab1c` |
| 固定输出零 | 首点 WA，16 点 NOT_RUN | `f991170e7f456abca1f33956d8d2c56478e925e800706154ec0f405ff4c84502` |
| 输出一个 13 字节数字 token | 首点 OLE，16 点 NOT_RUN | `33b231280ac4bceb1ca31e90cae025ad575c4c87cd75609fc8cb03bf05600dc7` |

独立比较三份报告的清单摘要、提交字节摘要和完整点序；每轮核验原容器 `exited`、PID 0、网络模式 `none`，报告完整且无未完成标记。错误清单或源码摘要没有启动槽、没有创建报告目录。该证据仅覆盖上述固定本机档位，不证明生产预算、目标节点、真实非作者终审或题目发布。

复现使用 [README 的维护命令](../../README.md#ab-固定材料隔离判题)，逐份使用私有合成源码的 `shasum -a 256` 结果和新报告目录；停止核验读取同一 `oj-reference-sandbox` 的容器 ID、状态、PID 和网络模式。后端使用 Docker 工具链的 `scripts/toolchain.sh backend`；普通 CI 不启动这个断网参考槽。

随后用[离线归档复核](../checker-admission.md#ab-归档报告复核)从原私有题包复算上述绑定，三份报告全部通过；互换 AC 和 WA 或保留 `INCOMPLETE` 标记都被拒绝，且不输出摘要。输出 JSON 字节摘要为 `1d49f1f9e60a7481200278c9901d4493a5763b30b726be7a0639311d3915b750`。没有再次启动参考槽或改变原档案。

## Ubuntu arm64 专用客体

2026-11-14，在无宿主共享的本地 Ubuntu arm64 VM（Apple hypervisor、Linux `6.8.0-106-generic`、原生 cgroup v2 Engine）用[独立档位](../checker-admission.md#ubuntu-独立判题实验)复用上面的 17 点正文材料、计划、候选、三份合成源码及相同限额。两次生成的 Ubuntu 清单字节一致，清单摘要 `a2aac16ff12078b51106b6a26ed374aa89dbe386152742d6c1aeee2f189983bb`，档位摘要 `8cf3b8386627d1a92b7566521c78dd030bf00fdd2acd5485dcc9bc4357e492b8`。默认生成的 Desktop 清单摘要仍等于上面的固定值；两个方向的跨档位清单均在启动前被拒绝，未创建报告目录。

| 合成提交 | 判定 | 私有报告字节 SHA-256 |
| --- | --- | --- |
| 读两个数并输出和 | 17 点 AC | `2ba03e32c76500ff8b9186098a529107993d458723dd28463dd3091abffb30d0` |
| 固定输出零 | 首点 WA，16 点 NOT_RUN | `f0a22987537b3e7715b3c12672aede16bc0e07f3aac3c14051329259d4d2a7af` |
| 输出一个 13 字节数字 token | 首点 OLE，16 点 NOT_RUN | `69746d200ea2845b04993199e8194a0f69d36e0255a8563f952087b0208901e5` |

刻意将继承的 Docker host、context、配置、HOME 和 PATH 设为无效值，三轮仍可编译、执行及读回缓存。独立比对三份报告的提交摘要、完整点序和档位；每轮均报告完成并确认停止，同一容器为 `exited`、PID 为 0、网络模式仍为 `none`、锁为空。验证后移除本轮专用容器。材料首次传输因归属仍为宿主 UID 被前置拒绝；按客体当前所有者重新解包后通过，未放宽文件校验。没有访问试用服务器；未执行真实崩溃恢复、Ubuntu 租约链路或目标节点验收。
