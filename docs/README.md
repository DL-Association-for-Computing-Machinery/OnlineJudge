# OnlineJudge 文档导航

| 你想做什么 | 从这里开始 |
| --- | --- |
| 启动项目、查可运行命令 | [项目 README](../README.md) · [本地 HTTPS 开发环境](operations/local-development.md) |
| 看现在做到哪个阶段 | [开发与交付计划](development/phase-1-development-plan.md) · [验收导航](acceptance/README.md) |
| 确认产品范围和验收条件 | [需求与验收](development/requirements.md) |
| 理解系统怎么工作 | [架构导航](architecture/README.md) |
| 配置环境、排查运行与恢复问题 | [运维导航](operations/README.md) |
| 查分支、PR 和 CI 怎么走 | [开发工作流](development/development-workflow.md) · [CI/CD](development/ci-cd.md) |

四类文档入口：[开发](development/README.md) · [架构](architecture/README.md) · [运维](operations/README.md) · [验收](acceptance/README.md)。

## 按功能查

| 功能 | 行为、配置与实现边界 | 接口契约 |
| --- | --- | --- |
| 账号、登录与会话 | [浏览器账号](operations/account-browser.md) · [会话设计](architecture/session-design.md) | [账号](../api/account.openapi.yaml) · [会话](../api/session.openapi.yaml) |
| 题源、私有草稿与审核 | [题源准备](architecture/problem-sources.md) · [草稿合同](architecture/problem-drafts.md) · [题包设计](architecture/problem-package-design.md) | [导入](../api/problem-draft-imports.openapi.yaml) · [草稿管理](../api/problem-draft-management.openapi.yaml) · [本人草稿](../api/problem-drafts.openapi.yaml) · [审核](../api/problem-review.openapi.yaml) |
| 参考验题与文本检查器 | [参考申请与调度](architecture/reference-requests.md) · [受限参考验证](architecture/reference-validation.md) · [检查器准入](architecture/checker-admission.md) | [参考申请](../api/problem-reference-requests.openapi.yaml) |
| 训练提交、本人历史与判题 | [提交队列与过载](architecture/submission-backlog.md) · [Worker 协议设计](architecture/api-worker-design.md) · [判题语义](architecture/judging-design.md) | [训练提交与历史](../api/training-history.openapi.yaml) · [Worker](../api/worker.openapi.yaml) |
| Web 页面、预览与渲染 | [Web 界面与预览](architecture/frontend-design.md) | 对应功能的 OpenAPI |
| 存储、备份与恢复 | [不可变材料存储](architecture/material-store.md) · [存储设计](architecture/storage-design.md) · [运维设计](operations/operations-design.md) | [健康检查](../api/openapi.yaml) |

## 开发与维护

- [根规则](../AGENTS.md)管授权和约束；[开发工作流](development/development-workflow.md)管实施、验收和交付步骤。
- [CI/CD](development/ci-cd.md)管检查何时运行；[代码质量](development/engineering-checks.md#代码质量)和[安全检查](development/engineering-checks.md#安全检查)解释门禁；[GitHub 治理](development/github-governance.md)管远端配置。
- [知识维护](development/knowledge-maintenance.md)说明如何同步仓库文档和项目知识库。

## 一条信息放在哪里

| 内容 | 维护位置 |
| --- | --- |
| 当前运行命令、工具链和环境入口 | [项目 README](../README.md)放快速启动；专项命令按[开发](development/README.md)、[运维](operations/README.md)、[验收](acceptance/README.md)查找；依赖版本以配置和锁文件为准 |
| 产品承诺和 AC 验收条款 | 需求文档；阶段安排和剩余工作归开发计划 |
| 设计约束、协议和取舍 | 对应主题设计；接口字段、响应和错误归 OpenAPI |
| 实际验收结果、失败与局限 | [交付证据](acceptance/README.md#阶段联验与单项复验)；按日期追溯看[交付日志](acceptance/delivery-log.md)；机器可读记录在 [evidence/](acceptance/evidence/) |

先改信息所属页面，其他地方用链接引用。历史记录保留原日期、版本和环境；查看当前操作时以对应指南为准。设计确认、实验通过、功能验收和部署分别记录。
