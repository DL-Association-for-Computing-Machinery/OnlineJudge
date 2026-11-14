# P4 题库与发布验收

本页对应[开发计划 P4](../phase-1-development-plan.md#p4-题库与发布)。剩余功能在 `codex/p4-problem-publication` 上按功能独立提交，测试和文档随对应提交交付。整阶段验收完成后统一创建 P4 PR。

| 验收项 | 主线已有证据 | 待完成 |
| --- | --- | --- |
| 冻结题目及证据 | 首批私有题源已适配，A+B 17 点修订已有锁定材料、输入覆盖、错解区分、参考结果和本地 Worker 联验。 | 确认试用题集、来源使用依据与各题的完整导入修订；每个拟发布版本按其判定语义核齐证据，旧修订的报告不能自动沿用。 |
| 试用节点及生产检查器 | [A+B 交接演练](aplusb-target-handoff.md)和离线复核已验证材料、回执、节点与制品的绑定；已完成下方试用机的只读盘点。 | 新增并验证 amd64 受限运行档位，落实管理员负责的专用槽，再取得实际节点运行与独立生产检查器证据。 |
| 普通终审 | 作者送审、标程确认、审核人分配、题面语义确认、退回及只读准入已上线到代码主线。 | 在同一事务内复核当前角色、分配和全部准入证据，持久化不可变批准及原键回执；用真实会话验证拒绝路径、并发和未知写入结果，取得实际非作者的终审。 |
| 独立发布 | [发布材料准备与读回](../architecture/problem-package-design.md#42-发布材料准备)已封存展示制品并验证正式材料。 | 由另有发布权的操作者在独立事务中复核准入、有效批准及准确的执行版本，原子发布可见引用；覆盖批准后撤权、改派、同人重授或重新分配均须重新终审，以及失败发布、历史版本和恢复。 |
| 试用题目读取 | 私有预览已有统一题面渲染和静态图片能力。 | 交付可见题目列表、检索、详情和附件，验证草稿、私有证据、答案、标程及未授权标识不会通过直接读取、筛选或计数泄漏。交给 P5 的是可核对的发布版本与执行材料绑定。 |

A+B 是首条验收链路，不能代替其余试用题。隔离夹具用于验证流程和拒绝路径；现场准入、实际非作者决定和正式发布分别留存证据。现场题目发布按项目的独立发布授权执行。

## 试用节点只读盘点（2026-11-14）

通过 SSH 公钥和严格主机密钥校验，以普通 `guest` 用户在目标节点运行 `uname`、Docker 客户端版本检查及 [target-inventory.py](../../scripts/target-inventory.py)。脚本由 SSH 标准输入送入 Python，无远端文件落盘。目标为 Ubuntu 24.04 x86_64，内核 `6.8.0-124-generic`，可见 CPU 亲和性为 4，系统总内存约 3.64 GiB，交换空间约 1.94 GiB；`/tmp` 所在文件系统当时约有 31.2 GiB 可用。登录会话可见 cgroup v2，祖先之一报告进程上限 9613；这些数据只描述盘点进程及文件系统，不是 Worker 容器配额。材料、数据库卷尚未指定，未取样。

Docker 29.6.1 是已观察到的客户端版本。标准 Engine socket 对此用户不可用（缺失或不可写），未确认 Engine 版本或容器隔离。现有 [Ubuntu 本地档位](../checker-admission.md#ubuntu-独立判题实验) 固定 arm64、`6.8.0-106-generic`、原生专用 Engine 和固定镜像；它与本机报告不能作为试用节点的档位或证据。盘点没有安装依赖、改权限、创建或启动容器，也未写入题目材料。

现场准备顺序：管理员先确认可执行受控实验的专用节点或隔离客体、容器运行权限与材料/临时路径；在本地锁定 amd64 工具链、执行镜像、内核和运行参数，为固定 17 点材料生成新档位的交接制品，并验证旧档位、错对象或错制品均不能启动。现场仅用已核对的制品执行固定 AC、WA、OLE 对照，保存原容器停止证据及独立检查器观察，再按同一修订复核。不能用共享主机上的普通 Docker 权限代替专用隔离；检查器准入、普通终审与发布仍独立处理。

## A+B amd64 候选镜像

[候选 Dockerfile](../../deploy/experiments/go-judge-aplusb-amd64.Dockerfile) 从原实验 Dockerfile 锁定的两个多架构索引中各选取唯一 `linux/amd64` 子清单，并保留执行器入口参数。原 arm64 镜像和运行档位保持不变。

```sh
docker --context desktop-linux buildx build --platform linux/amd64 --load \
  --provenance=false --network none \
  -f deploy/experiments/go-judge-aplusb-amd64.Dockerfile \
  -t oj-p4-aplusb-amd64-candidate:local .
docker --context desktop-linux image inspect --format '{{.Id}} {{.Os}}/{{.Architecture}}' \
  oj-p4-aplusb-amd64-candidate:local
docker --context desktop-linux run --rm --pull never --platform linux/amd64 \
  --network none --read-only --cap-drop ALL --security-opt no-new-privileges \
  --entrypoint /usr/local/bin/g++ oj-p4-aplusb-amd64-candidate:local -dumpfullversion
```

本地构建报告的镜像清单摘要为 `sha256:bfb42a2daf3f86c8636e3db883179852f385df1a31fd2b9f4a42d01e7e9f502c`，配置摘要为 `sha256:fe1a97991b090d9232fb8b086139925debe96dce834fe16def8c7f3373a2f4f9`；Docker 回读 `linux/amd64`，GCC 回读 `15.3.0`。这是本机候选镜像构建和版本读回，未验证独立重建、原生节点上的 go-judge 执行或隔离。尚未将候选镜像登记为 Worker 档位，也未传到试用服务器。

## A+B amd64 候选材料

对固定 A+B 17 点私有题包运行 [离线清单命令](../../README.md#ab-固定材料隔离判题)，增加 `--trial-amd64-candidate`。该分支只接受已审核的完整导入修订、正式 Plan、检查器候选与 `262144` 字节／每 token `12` 字节上限。生成的判题清单摘要为 `8fc0161f7229ea8c8882098d61973fa6f1b81c3bda4fcccc0c6d285fb1c94817`，其中检查器试验档位摘要为 `0127bfe7d7703cd5eb382f33d71dc81bdb8860d1c69abe20a5c2f51dc45c3365`。导出需单独传入已核对的该清单摘要以及新的私有目录。

现场候选档位的内核来自只读盘点，镜像来自上方构建。材料仍是同一 17 点，运输清单会绑定候选档位；现有执行配置和交接回执验证器均不接纳它。后续须验证专用节点、原生 Engine、镜像身份、有效容器限额及生命周期，再登记可执行档位。

本机无网络 Docker 中，以固定私有题包两次导出，32 个对象与运输清单逐字节一致。运输清单摘要为 `d136160e862b2553e6bb0c87637052635ec2871d72f7efcb51840454479e89b4`。传入旧 Ubuntu arm64 判题清单摘要或同时选择两个档位均被拒绝，未留下输出目录。该测试未运行判题程序或访问试用机。

## A+B amd64 候选交接

沿用 [A+B 交接命令](../checker-admission.md#ab-目标节点交接)，以 `--runtime 0127bfe7d7703cd5eb382f33d71dc81bdb8860d1c69abe20a5c2f51dc45c3365` 和 `--slot oj-aplusb-trial-amd64-sandbox` 封装或核对这个候选包。封装输出仅含 32 个固定对象、运输清单和交接清单。接收方必须从独立渠道获取预期交接摘要、节点、候选 Git 修订及档位，不从收到的包推导这些预期值。

候选包使用 `oj.aplusb-trial-candidate-handoff.v1`，只支持 `pack` 和 `verify`；报告核验及回执生成均拒绝该协议。成功核验只证明候选字节与约定绑定一致；不表示指定节点真实运行过，也不解除生产检查器、目标运行时或终审的阻断。

本机无网络复验：用测试节点标签和测试 Git 修订两次打包，34 个文件逐字节一致；接收方按独立摘要回读成功，修改节点标签或一个对象字节均失败。该包只用于演练；现场包应按获准节点及正式构建修订重新封装。
