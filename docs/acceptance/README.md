# 验收导航

[返回文档总览](../README.md) · [开发计划](../development/phase-1-development-plan.md)

| 证据类型 | 从这里查 |
| --- | --- |
| 怎么复验浏览器、Worker 和故障场景 | [本机隔离验收专项](isolated-checks.md) |
| 怎么复验已发布题的审核、提交与本人回读 | [已发布题训练联验](published-training-checks.md) |
| M0 技术验收及范围 | [限定环境验收](m0-acceptance.md) · [实验与缺口](m0-experiment.md) |
| 按日期追溯交付、失败和修正 | [交付日志](delivery-log.md) |
| 单项历史调查 | [参考验证停止故障](reference-validation-investigation.md) |
| 机器可读记录与负载样本 | [evidence/](evidence/) |

证据保留当时的环境、版本与结果。当前操作步骤见[运维导航](../operations/README.md)和[项目 README](../../README.md)。

## 阶段联验与单项复验

- 各阶段的验收结果和缺项，从[开发计划](../development/phase-1-development-plan.md)的对应阶段进入。
- A+B 的[隔离判题](delivery-evidence/aplusb-isolated-judging.md)、[租约链路](delivery-evidence/aplusb-leased-judging.md)、[恢复后重判](delivery-evidence/aplusb-restored-rejudge.md)和[目标环境交接](delivery-evidence/aplusb-target-handoff.md)分别留有证据。
- 阶段证据：[P4 发布](delivery-evidence/p4-trial-publication.md)、[P5 已发布训练](delivery-evidence/p5-published-training.md)、[本地代码草稿](delivery-evidence/local-code-drafts.md)、[P6 试用验收](delivery-evidence/p6-trial-acceptance.md)、[P7 完整训练](delivery-evidence/p7-stage.md)。
- 固定环境的 M0 证据见[机器索引](evidence/m0-20261103.json)；本机负载与容器资源见 [Ubuntu 单机混合负载](evidence/ubuntu-mixed-load-20261112.md)。
- CI 耗时与对比口径见[交付效率基线](delivery-evidence/ci-delivery-efficiency.md)。

需要长期引用的验收或实验，写清目标、环境、复现步骤、观察结果和未覆盖项，再从对应阶段、主题文档或 PR 链接，不在导航重复记录验收结果。
