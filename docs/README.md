# 文档索引与维护归属

文档随同一 Git 提交修订，不另设独立的“最新版本号”。历史日期、实验环境和材料版本用于追溯，不能统一替换成当前版本。具体依赖版本以配置和锁文件为准。

| 要查或修改的内容 | 唯一维护入口 |
| --- | --- |
| 启动、工具链、当前可运行命令 | [项目 README](../README.md) |
| 产品需求与 AC 验收条款 | [需求与验收](requirements.md) |
| 发布范围、组件引入时机、P0—P7 与里程碑 | [开发与交付计划](phase-1-development-plan.md) |
| 当前 M0 证据与剩余缺口 | [M0 记录](m0-experiment.md) |
| 会话实验细节、夹具与专项复现 | [会话实验 README](../tests/sessionexperiment/README.md) |
| 跨模块架构、数据库迁移与资源预算 | [总体架构](architecture/system-design.md) |
| 初始选型的理由、备选与取舍 | [ADR-001](architecture/001-go-gin.md) |
| 前端、草稿与渲染 | [前端设计](architecture/frontend-sandbox-options.md) |
| 判题语义、浮点与 SPJ | [判题设计](architecture/judging-design.md) |
| 题包、存储、会话、比赛 | [题包](architecture/problem-package-design.md)、[存储](architecture/storage-design.md)、[会话](architecture/session-design.md)、[比赛](architecture/contest-design.md) |
| API / Worker 与运维设计 | [协议](architecture/api-worker-design.md)、[运维](architecture/operations-design.md) |
| 已实现接口的字段、响应和错误 | [健康接口](../api/openapi.yaml)、[实验接口](../api/experiment.openapi.yaml)、[Worker 接口](../api/worker.openapi.yaml) |
| 授权与稳定规则、执行步骤 | [根规则](../AGENTS.md)、[开发工作流](development-workflow.md) |
| CI 行为、质量和安全检查 | [CI/CD](ci-cd.md)、[代码质量](code-quality.md)、[安全检查](security-checks.md) |
| 远端治理、知识回写流程 | [GitHub 治理](github-governance.md)、[知识维护](knowledge-maintenance.md) |
| 按日期记录的交付、失败与验证 | [交付日志](delivery-log.md) |
| 上游 OJ 比较依据 | [参考分析](architecture/oj-reference.md) |

更新时先修改所属页面，其他页面链接到该处，不复制完整进度、命令或验收清单。设计确认、实验通过、产品验收与部署分别记录。旧实验结果保留原环境和日期；当前说明不得沿用旧实验的“下一步”或授权状态。

原交付基线已合并并删除，所有仓库内引用改指开发与交付计划。需求与架构专题保留各自职责，避免合并时丢失约束；历史记录不作为当前操作手册。
