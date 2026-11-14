# 已发布题训练联验

在本机隔离环境使用合成账号和临时依赖。实际判题、审核与发布证据见[验收导航](README.md)。

### 本机已发布 A+B 浏览器联验

在[专用 Ubuntu VM](isolated-checks.md#专用-ubuntu-vm-判题实验)内使用一次性 PostgreSQL、Redis 和 P4 的[本地执行准入](delivery-evidence/p4-trial-publication.md#mac-ubuntu-vm-本地完整流程)。普通 API 的 `local_training_intake: true` 要求同时开启 `training_history`、`training_submission_detail`、本地执行及题库；缺项拒绝启动。此开关默认关闭，仅用于回环 HTTPS 和本地 PostgreSQL Unix socket。

可在同一套 P4 浏览器夹具中加 `OJ_BROWSER_LOCAL_TRAINING_INTAKE=1`。以已核验的 Ubuntu 运输清单启动对应的 Worker 与私有 mTLS 控制端，指向夹具的临时数据库。完成 P4 批准并用 `problem-review publish-aplusb-local` 发布后，在 Mac 执行：

```sh
OJ_BROWSER_QA_INFO="$FIXTURE_READY_JSON" OJ_BROWSER_QA_OUTPUT="$QA_DIRECTORY" \
  OJ_PLAYWRIGHT_MODULE="$LOCAL_PLAYWRIGHT_ENTRY" OJ_CHROMIUM_EXECUTABLE="$LOCAL_CHROMIUM" \
  node tests/browser/published-training-qa.mjs
```

该脚本通过浏览器正常账号登录和选题，只上传并提交固定 A+B 源码。它验证文件限制、替换确认、刷新后原键回执、真实判定及账户隔离；不会启动 Worker 或替代隔离检查。验证边界见[交付证据](delivery-evidence/p5-published-training.md)。

同一夹具、参数和已发布题可将脚本替换为 `tests/browser/published-training-drafts-qa.mjs`，验证本地草稿、冲突及退出。该专项不发判题请求，不占用 Worker 槽。

### 本机文本题验题与提交

这个档位支持源锁校验通过、有单文件 GNU++17 标程、使用 UTF-8 文本 token 比较的完整题包：最多 64 个正式点，单输入最多 4 MiB，单答案最多 256 KiB，测试数据合计最多 48 MiB。并查集是已联验的代表题。

按 [Ubuntu 隔离流程](../operations/problem-maintenance.md#ab-租约链路联验)准备同一个固定断网槽。在槽停止时执行下列命令，自动检查冻结题包及全部答案、编译标程并运行每个正式点。只有全部 AC 且确认槽停止后，报告才能用于准入。可用新的报告目录追加 `--wrong-solution "$PRIVATE_CPP" --expected-verdict WA` 核对已有错解；退出码体现预期是否满足，错解报告不用于准入。

```sh
OJ_SANDBOX_EXPERIMENT=1 worker validate-checker-candidate --local-text \
  --source-id "$SOURCE_ID" --bundle "$PRIVATE_BUNDLE" --source-lock "$SOURCE_LOCK" \
  --output "$COMPLETED_REPORT" --start-stopped-slot
umask 077
checker-candidate export-local-text --source-id "$SOURCE_ID" --bundle "$PRIVATE_BUNDLE" \
  --source-lock "$SOURCE_LOCK" --report-dir "$COMPLETED_REPORT" \
  --expected-report "$REPORT_SHA256" --review-profile "$REVIEW_PROFILE_SHA256" \
  --export "$NEW_PRIVATE_DIRECTORY" > "$PRIVATE_ADMISSION_CONFIG"
```

从可信运行取得 `report.json` 的 SHA-256。沿用已有 P4 题目绑定时，审核档位摘要来自该题的正式参考记录。全新来源使用下面的题包审核方式。冻结题包、报告和清单均须位于可信本机目录，报告目录不得有 `INCOMPLETE`。执行验题不授予发布权限。

核对导出清单摘要；登记材料、控制端和 Worker 均使用 `OJ_SANDBOX_EXPERIMENT=1`、`OJ_LOCAL_TEXT_MANIFEST_FILE` 和 `OJ_LOCAL_TEXT_MANIFEST_SHA256`，不得与 `OJ_APLUSB_*` 材料配置混用。`migrate fixture` 与控制端另需 `OJ_MATERIAL_ROOT` 指向导出目录。Worker 只在固定 Ubuntu 槽使用 `OJ_APLUSB_START_STOPPED=1`；这两个版本不得共占槽。

回环会话 API 的 `private_drafts.local_text_execution` 使用上一步输出，配置冻结题包、源锁及对应的唯一来源，并开启本机训练提交、历史与详情。`migrate fixture` 后，用 `problem-review register-text-local --bundle "$PRIVATE_BUNDLE" --source-lock "$SOURCE_LOCK" --admission-file "$PRIVATE_ADMISSION_CONFIG" --database-socket "$PRIVATE_SOCKET" --database-user "$LOCAL_USER"` 校验并登记本地执行准入，再由作者、审核员与发布员走现有分工。发布使用 `problem-review publish-text-local`，参数与 `publish-aplusb-local` 一致，另传 `--bundle` 和 `--source-lock`。浏览器专项见 `tests/browser/local-text-training-qa.mjs`，沿用 A+B 专项的 Playwright 参数；夹具另设 `OJ_BROWSER_LOCAL_TEXT=1`。

该路径限定本机回环 HTTPS、Unix socket 数据库、固定断网 Worker 槽和运维显式 pin。未开放生产、混合题队列或任意题包。[P5 联验证据](delivery-evidence/p5-published-training.md)记录实际运行和未覆盖项。

#### 全新同档位题的题包审核

使用独立配置的可信源锁和完整冻结题包；无需在 Go 代码登记题名。题包内的 `quality/` 文件都须在源锁中。准备 `quality/plan.json`、单文件 `validator.cpp`、独立算法 `oracle.cpp`、`coverage.cpp`、`inputs/<名称>.in` 和 `wrong/<名称>.cpp`。计划包含 `controls`（一项接受和三项拒绝）、`probes`（10–32 个独立小用例）、`coverage`（条件名到有序的非样例点号）及 `wrong`（四种不同的错误模型）。输入校验器返回 0 或 1；覆盖程序输出符合当前输入的条件名。自动检查每个条件既有非样例命中，也有不命中，四个错解须各有不同的非样例拒绝集合。

在专用 Ubuntu VM 运行：

```sh
OJ_SANDBOX_EXPERIMENT=1 worker validate-checker-candidate --local-text --package-review \
  --source-id "$SOURCE_ID" --bundle "$PRIVATE_BUNDLE" --source-lock "$SOURCE_LOCK" \
  --output "$COMPLETED_REPORT" --start-stopped-slot
OJ_SANDBOX_EXPERIMENT=1 worker validate-package-quality \
  --source-id "$SOURCE_ID" --bundle "$PRIVATE_BUNDLE" --source-lock "$SOURCE_LOCK" \
  --output "$QUALITY_DIR" --start-stopped-slot
```

在独立回环预览实例中导入相同题包，用 `tests/browser/local-package-rendering-qa.mjs` 生成桌面及 390 px 题面证据（另设 `OJ_BROWSER_RENDERING_FILE`）。导出命令追加 `--package-review --quality-dir "$QUALITY_DIR" --expected-quality "$QUALITY_SHA256" --rendering-file "$RENDER_REPORT" --expected-rendering "$RENDER_SHA256"`；该模式自动选固定审核档位，不传 `--review-profile`。运营固定三个报告原文的摘要，准入绑定同一导入修订、完整正式计划和运行档位。

在启用了此准入的本机实例，作者按现有流程导入、申请正式参考验证并准备计划。参考控制端及参考 Worker 显式设置 `OJ_LOCAL_REFERENCE_SOURCE`、`OJ_LOCAL_REFERENCE_LOCK_SHA256` 和 `OJ_SANDBOX_EXPERIMENT=1`，参考控制端保留独立 mTLS 节点、Unix socket 数据库和私有材料根；用 `worker run-reference <申请ID> <来源ID> <计划摘要>` 消费真实参考租约。作者确认标程、非作者审核人确认题意并批准、独立发布人执行发布。批准事务和发布事务各自重查完整门槛。验证完参考执行并确认槽停止后，再用同一个 Ubuntu 槽启动训练 Worker。

合成验收题包可用 `python3 tests/fixtures/local-text/build-span-fixture.py "$NEW_PRIVATE_DIRECTORY"` 生成。它只供隔离测试，源锁沿用现有包格式，不代表上游发布。审核与训练浏览器脚本见 `tests/browser/local-package-review-qa.mjs` 和 `tests/browser/local-text-training-qa.mjs`。
