# PR 集成检查耗时基线

对照为 [PR #265](https://github.com/DL-Association-for-Computing-Machinery/OnlineJudge/pull/265) 的[最终成功运行](https://github.com/DL-Association-for-Computing-Machinery/OnlineJudge/actions/runs/43452466153)。从 GitHub Actions 查询，最终成功运行 737 秒，其中集成任务 692 秒、后端任务 411 秒。该分支在 PR 存续期间查询到 4 次运行（1 成功、3 取消、无手动重试），归属由同仓库同分支和时间窗口推断；不能据此判定取消原因。

复查入口：`python3 -B scripts/measure-pr-ci.py 265`。对照方法见 [CI 等待与重跑指标](../../development/ci-cd.md#衡量-pr-等待与重跑)。这里记录改动前的数据，不能据此宣称新流水线已加速。

改动后的本机验证：Docker Desktop Linux arm64、Go 1.26.11、现有离线工具链和已校验模块缓存。两套协议测试在同一个新建的 Go 编译缓存卷上并行运行，共约 2 分 28 秒，均通过。另用无外网容器依次验证四组 sqlc 输出、真实 PostgreSQL race 套件和双 API 进程，均通过。治理套件运行 141 项，1 项按现有条件跳过，工作流语法和仓库策略通过。本机环境与 GitHub runner 不同，耗时不能与 #265 相减。
