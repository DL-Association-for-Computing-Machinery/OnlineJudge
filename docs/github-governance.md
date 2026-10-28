# GitHub 仓库治理

2026-10-25，用户授权提交 PR、配置仓库，并单独确认从空基点 `5d233d6` 建立 `main`。配置范围仅为本仓库；不调整组织成员、收费套餐或其他仓库。以下配置已通过 GitHub API 回读验证，远端 CI 运行结果另见[交付日志](delivery-log.md)。

## 已生效控制

- 默认分支为 `main`；实现留在 `codex/engineering-baseline`，首次 PR 包含空基点之后的全部工程成果。
- `main-governance` ruleset（ID `25824684`）为 active，匹配 `refs/heads/main`，无 bypass actor。禁止删除、强推，要求线性历史和 PR；讨论必须解决。
- 必需检查为 `governance`、`backend`、`frontend`、`integration`，限定 GitHub Actions App `15368`。分支必须基于最新主线，创建匹配分支也不豁免检查。
- 2026-10-28 起应用测试按[完整变更范围](ci-cd.md#按变更影响选择检查)选择；四个必需名称和远端 ruleset 保持不变。后三项是结果门禁：明确不受影响才接受对应测试跳过，受影响则必须真正成功，分类失败/缺失也阻断。门禁通过不代表所有应用测试均执行。
- 仅允许 squash merge，以 PR 标题/正文生成提交信息；GitHub 平台自动合并未启用，合并后自动删除任务分支。PR 标题通过 Conventional Commits 校验，标题编辑会重新触发检查。2026-10-28 用户已授权 Codex 在当前 head 的 CI 与 review 均通过后主动执行普通 squash 合并，具体门禁见[开发工作流](development-workflow.md)；这不改变平台设置，不使用 `--auto` 或管理员绕过。
- Actions 默认令牌只读，不允许 Actions 创建或批准 PR；仅允许本组织及 GitHub 官方 actions，外部已验证发布者和自定义匹配列表关闭，所有 actions 必须固定完整 SHA。工作流仍显式 `contents: read`、不保留 checkout 凭据、不使用 `pull_request_target`，普通 PR 不进入专用特权沙箱。
- Dependabot alerts 已启用，安全更新、secret scanning 和 push protection 已启用。未启用向提供方验证秘密有效性的功能，未引入额外付费服务。扫描开启不代表代码或镜像已无漏洞。

用户确认目前仍由其维护，虽然有其他管理员，暂不强制独立批准：required approvals 为 0，CODEOWNERS 与最后推送者之外批准未启用。这个配置强制 PR/CI，但不提供双人职责分离；具备设置权限的管理员仍可修改规则。新增实际长期审查者后，另行确认至少 1 位非作者批准及真实 CODEOWNERS。

生产 Environment、CD、签名制品和独立审查尚未建立；不能把上述治理基线称作企业认证或生产就绪。

## 可审查配置与复核

期望配置保存在 [.github/repository](../.github/repository/) 的 JSON 中：`settings.json`、`main-ruleset.json`、`actions.json`、`selected-actions.json`、`workflow-permissions.json`。这些文件是配置依据，不会自动授予 CI 管理员凭据或自动改远端。

只读复核命令（需要 GitHub CLI 已登录）：

```sh
gh api repos/DL-Association-for-Computing-Machinery/OnlineJudge
gh api repos/DL-Association-for-Computing-Machinery/OnlineJudge/rulesets/25824684
gh api repos/DL-Association-for-Computing-Machinery/OnlineJudge/rules/branches/main
gh api repos/DL-Association-for-Computing-Machinery/OnlineJudge/actions/permissions
gh api repos/DL-Association-for-Computing-Machinery/OnlineJudge/actions/permissions/selected-actions
gh api repos/DL-Association-for-Computing-Machinery/OnlineJudge/actions/permissions/workflow
```

更新配置时先回读当前值、审查 JSON 差异，经授权后更新现有 ruleset ID，不能重复 POST 产生多套重叠规则。配置前后快照保存在本机忽略目录 `.local/github-governance/`；它们供诊断与恢复参考，不是可直接整包 PATCH 的请求体。恢复按受影响字段逐项处理，重新核验，不自动移除保护或覆盖主线。

首次主线为空，工作流与文档须经 PR 合并后才成为主线内容。已有 PR 的检查使用其合并候选；CI 通过并不自动证明 PR 中的治理脚本未被人为削弱，仍需人工审查权限和工作流变更。

依据：[GitHub rulesets](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-rulesets/available-rules-for-rulesets)、[Actions 权限 API](https://docs.github.com/en/rest/actions/permissions)、[Actions 安全使用](https://docs.github.com/en/actions/reference/security/secure-use)。
