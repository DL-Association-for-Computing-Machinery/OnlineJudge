# 架构导航

[返回文档总览](../README.md) · [总体架构](system-design.md)

| 要理解的边界 | 主文档 | 相关入口 |
| --- | --- | --- |
| 一次提交怎样进入队列、被 Worker 领取并回报 | [API 与 Worker](api-worker-design.md) | [积压准入](submission-backlog.md) |
| 答案怎么比较、结果怎样归类 | [判题设计](judging-design.md) | [检查器准入](checker-admission.md) |
| 题包怎样从来源进入草稿并准备发布 | [题包设计](problem-package-design.md) | [题源](problem-sources.md) · [私有草稿](problem-drafts.md) |
| 参考验题怎样申请、调度与执行 | [参考验证申请](reference-requests.md) | [受限参考验证](reference-validation.md) |
| 账号怎样登录、续期和撤销 | [会话设计](session-design.md) | [浏览器账号](../operations/account-browser.md) |
| 数据库与材料怎样存储、备份和恢复 | [存储设计](storage-design.md) | [材料存储](material-store.md) |
| 本机、离线交付和试用运行怎样划界 | [运维设计](../operations/operations-design.md) | [开发计划](../development/phase-1-development-plan.md) · [本机开发](../operations/local-development.md) |
| 前端与沙箱为什么选现有路线 | [前端与沙箱选型](frontend-sandbox-options.md) | [当前 Web 设计](frontend-design.md) · [界面设计约定](../../DESIGN.md) |
| 比赛报名、计分与成绩发布怎么设计 | [比赛设计](contest-design.md) | [需求与验收](../development/requirements.md) |

选型依据见 [Go + Gin 架构决策](001-go-gin.md)和 [Hydro / DOMjudge 参考分析](oj-reference.md)。文档描述的方案是否已经交付，以开发计划和[验收证据](../acceptance/README.md#阶段联验与单项复验)为准。
