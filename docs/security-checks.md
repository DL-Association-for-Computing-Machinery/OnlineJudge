# 开发安全检查

本页维护扫描范围与处置流程；扫描通过不等于完整安全审计。主线必需结果门禁要求 `security` 成功，失败、取消、超时和未知状态均阻断合并，不修改远端保护名称。

## 执行与边界

在仓库根目录执行：

```sh
python3 scripts/check-secrets.py --self-test
python3 scripts/check-secrets.py
docker --context desktop-linux compose run --build --rm backend-audit
docker --context desktop-linux compose run --build --rm frontend-audit
```

Linux CI 使用相同镜像与命令，省略 `--context desktop-linux`。PR、主线 push 和手动 CI 均执行；没有新增定时任务。

- 秘密：Gitleaks v8.30.1 扫描 HEAD 可达的完整提交历史，包含已删除内容；扫描前用合成令牌确认检测器确实拒绝泄漏且脱敏。通过 `git bundle` 导出，不挂载 checkout、Docker socket、用户配置或凭据，扫描容器禁网、非 root、只读根文件系统、无 capabilities。忽略文件、未跟踪/未提交内容、其他不可达分支不在扫描范围内；提交前仍须审查暂存差异。日志完整脱敏，不发布原始秘密报告。
- Go：官方 govulncheck v1.9.0 分析 `./...` 当前默认构建条件下的可达调用；可达漏洞与工具错误返回失败。verbose 输出保留模块级未触达条目，不能将这些条目说成依赖完全无漏洞；不同平台、build tags、动态调用与镜像 OS 包不由本检查完整覆盖。
- 前端：沿用项目固定 pnpm，`audit --audit-level low` 包括开发依赖；任何已知等级告警都阻断。不使用自动 fix、忽略不可修复漏洞或忽略 registry 错误的选项。

下载公共模块/镜像、查询 Go 漏洞库和 npm audit 接口需要联网。pnpm audit 会向包 registry 发送依赖名称与版本；不上传仓库源码或扫描出的秘密。服务不可用是未完成检查，不是安全通过。

## 工具准入与维护

2026-10-29 核对官方仓库：Gitleaks CLI 为 MIT，govulncheck 为 Go BSD 风格许可；两个版本的 Go 要求分别为 1.24.11 与 1.26.0，项目构建工具链为 1.26.10。固定版本通过 Go 模块校验安装到专用 Docker stage；不进入应用运行镜像，不修改应用依赖清单。代价是首次下载、工具镜像空间与分析时间。Gitleaks 官方公开 advisory 查询为空，仅表示该接口未返回公告，不是无漏洞证明。

使用 CLI 而非额外第三方 Action，沿用现有只读工作流权限、固定 checkout SHA 和 Docker 工具链。镜像 OS 包漏洞扫描及 SBOM 留作独立后续目标，不宣称本项覆盖。

2026-10-29 初始基线有三个未触达模块条目。本次将 `golang.org/x/net` 从 v0.61.0 升至 v0.62.0，处置 [GO-2026-6866](https://pkg.go.dev/vuln/GO-2026-6866)（HTML 渲染的祖先命名空间检查）和 [GO-2026-6860](https://pkg.go.dev/vuln/GO-2026-6860)（HTML 文本节点合并的二次复杂度）；仅更新该模块及校验值，不改业务接口或升级其他依赖。实际复扫与兼容性验证见[交付记录](delivery-log.md)。

[GO-2026-5932](https://pkg.go.dev/vuln/GO-2026-5932) 涉及 `golang.org/x/crypto/openpgp` 及其子包，所有版本均无已知修复。项目默认及 `sandbox` 标签的 `go list -deps -test` 包图均未导入这些包；这属于当前项目不适用，不能称为已修复上游模块。`x/crypto` 仍用于其他包，例如 `cmd/api → gin → binding → validator/v10 → x/crypto/sha3`，不能为消除模块提示删除必要依赖或引入无用途的 OpenPGP 替代库。verbose 告警继续保留，未配置忽略或降低门禁；调用、依赖、平台或标签变化后须重新评估。

## 告警与例外

当前维护者负责处理阻断；Codex 可在授权任务内定位和修复，但不得静默禁用检测器、增加忽略、降低等级或升级无关依赖。

1. 秘密疑似泄漏：日志只记录规则、路径和位置，先由凭据所有者撤销/轮换，再处理历史及外部副本；删除代码不是撤销。历史重写需单独授权。
2. 漏洞：记录 advisory ID、版本、触达条件、修复版本和验证。可达/前端阻断项在合并前解决；Go 未触达条目保留在 verbose 输出中，评估后单独修复，不能称为已修复。
3. 确需例外：由用户明确批准，记录精确规则/advisory、范围、原因、补偿措施、负责人和到期日期。当前没有例外配置；如将来引入，须同时实现到期失败检查，禁止永久宽泛排除。
4. 工具或网络故障：先诊断、恢复后重跑当前 head，不以旧报告或成功占位替代。工具版本升级独立验证合成拒绝案例与实际仓库，保留回退版本和兼容性证据。

官方依据：[Gitleaks](https://github.com/gitleaks/gitleaks/tree/v8.30.1)、[Go 漏洞管理](https://go.dev/doc/security/vuln/)、[govulncheck v1.9.0](https://github.com/golang/vuln/tree/v1.9.0)、[pnpm audit](https://pnpm.io/cli/audit)。
