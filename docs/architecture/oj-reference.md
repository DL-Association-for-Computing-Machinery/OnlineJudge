# Hydro 与 DOMjudge：自研项目的参考边界

分析日期：2026-10-25。性质：官方文档与部分源码的静态评估；没有运行验证。当前实施路线以 [ADR-001](001-go-gin.md) 为准，本页不建议将业务系统改为二次开发。

## 参考版本与方法

- Hydro：源码快照 `0dba6c4ac1870a96ef83dc2b0e514142181b7075`，核心包标记 5.0.7；开发分支能力不自动等于部署版本能力。
- DOMjudge：9.0.1 文档与源码。没有完整审计其全部路径。
- 对照 [本项目需求](../requirements.md)，区分已找到的实现、适配推断与待验证行为。前轮“Hydro 更适合做基础”的建议已由用户的 Go + Gin 自研决定替代。

## 值得吸收的设计

| 主题 | Hydro 参考 | DOMjudge 参考 | 本项目需要明确的契约 |
| --- | --- | --- | --- |
| 日常训练 | 题库、训练计划、作业、小组 | 比赛与题目组织 | 私有题单、队长可见范围和任务进度 |
| 参赛身份 | 组队邀请、比赛成员快照、个人提交归队 | 账号关联参赛队、名单导入 | 长期战队与逐场阵容分离，保留提交者 |
| 计分封榜 | 显示成绩与真实成绩的处理 | 按提交时间封榜、人工解封 | 全部公开 API 的保密和事件时间口径 |
| 重判 | 结果更新与比赛统计 | 先检查重判集，再应用或取消 | 结果版本、旧回调隔离、派生数据纠正 |
| 赛务 | 答疑、气球、现场工具包 | 裁判答疑、独立气球员角色 | 最小权限、撤销和人工纠错审计 |
| 扩展 | 插件及事件接口 | Contest API 对接外部工具 | 明确公开接口，避免共享私有实现 |
| 分布式判题 | 节点连接和任务分配 | domserver 与 judgehost 分离 | 领取租约、确认、重试、幂等与容量上限 |

## 不能直接沿用的假设

- Hydro 有队伍邀请，不等于满足“每人最多一个长期战队”和队长权限；这需要独立业务约束。
- 所查 Hydro 气球处理器使用比赛管理权限；本项目必须实现仅能处理气球的角色。
- Hydro 有 CDP 导出与外置滚榜入口，DOMjudge 可对接 Contest API；仍要验证最终榜一致、结果冻结、离线演示与奖项控制。
- 所查 Hydro ACM 计分含固定 20 分钟错误罚时；本项目以最终确认的比赛规则为准。
- 两者现成功能均不能证明本项目的 AI 出题审核、不可变题目版本、成长积分纠正或名额确认流程已完成。
- 300 人容量、异构机房机器的判题一致性、掉线不丢任务、气球重判纠正均需要实际实验。

## 可定位证据

- [Hydro 比赛与组队](https://github.com/hydro-dev/Hydro/blob/0dba6c4ac1870a96ef83dc2b0e514142181b7075/packages/hydrooj/src/handler/contest.ts)
- [Hydro 计分与结果](https://github.com/hydro-dev/Hydro/blob/0dba6c4ac1870a96ef83dc2b0e514142181b7075/packages/hydrooj/src/model/contest.ts)
- [Hydro 现场插件](https://github.com/hydro-dev/Hydro/blob/0dba6c4ac1870a96ef83dc2b0e514142181b7075/packages/onsite-toolkit/index.ts)
- [DOMjudge 配置](https://github.com/DOMjudge/domjudge/blob/9.0.1/doc/manual/config-basic.rst)、[赛务](https://github.com/DOMjudge/domjudge/blob/9.0.1/doc/manual/running.rst)、[重判](https://github.com/DOMjudge/domjudge/blob/9.0.1/doc/manual/judging.rst)
- [ICPC Resolver](https://tools.icpc.global/resolver)

没有将上游整段代码复制到本项目，也没有建立依赖或外部服务连接。
