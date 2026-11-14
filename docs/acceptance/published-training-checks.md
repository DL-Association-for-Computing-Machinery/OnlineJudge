# 已发布题训练联验

在本机隔离环境使用合成账号和临时依赖。实际判题、审核与发布证据见[验收导航](README.md)。

### 单 API 多题训练

先通过现有维护流程分别发布题目，并把封存题面保存到同一个受信对象根。独立训练实例使用 `published_training`，不配置 `private_drafts` 或旧的 `local_training_intake`；同时开启 `training_history` 和 `training_submission_detail`。仅接受回环监听、回环 HTTPS origin 和本机 Unix socket 数据库。示例中的版本后缀和 SHA-256 换成受信执行清单的值：

```json
{
  "published_training": {
    "material_root": "/absolute/path/to/sealed-statements",
    "max_in_flight": 2,
    "manifest_pins": [
      { "version": "aplusb-<judge-sha256>", "sha256": "<transport-manifest-sha256>" },
      { "version": "local-text-<judge-sha256>", "sha256": "<transport-manifest-sha256>" }
    ]
  },
  "training_history": true,
  "training_submission_detail": true
}
```

两种本机提交模式默认接受 GNU++17、C17 和 GNU++20。Java 21 和 Python 3.14 继续分别由 `local_java21` 和 `local_python3` 开启；先在对应队列上准备支持这些语言的 Worker，并同步前端语言开关。

上述片段并入正常 API 配置。材料 pin 最多十项；只能为已批准、已发布且版本和摘要完全匹配的题目提交。两个队列可在同一 API 上同时接收；各控制端和 Worker 继续使用各自的显式材料配置。若只有一个独占 Ubuntu 槽，可排空首个 Worker 并确认槽停止后启动另一个，API 不需要重启。浏览器专项 [multi-publication-training-qa.mjs](../../tests/browser/multi-publication-training-qa.mjs) 从同一账号提交 A+B 和并查集，回读原源码和两笔判定；使用现有 Playwright 参数，另传 `OJ_BROWSER_UNION_FIND_SOURCE`。夹具模式通过 `OJ_BROWSER_PUBLISHED_TRAINING_ROOT` 指定新建的题面根，读取 `/workspace/publication-pins.json`，并从 `/local-evidence` 和 `/local-text-evidence` 读取各自已验题证据；后者的准入文件应指向相应的挂载路径。

2026-11-15 在本机 Ubuntu VM 用临时 PostgreSQL 和 Redis 联验：同一普通账号在浏览器提交 A+B 与并查集，两笔 GNU++17 提交先同时进入各自的材料队列，再由各自的真实 Worker 依次使用同一个断网沙箱槽，均判为 AC。核对了两条完成记录的 Worker 节点、材料版本和摘要；API 的 PID 和启动时间始终一致。本人可回读判定和原源码，其他账号无法读取；同题同键重试返回原回执，跨题复用请求键和提交过期发布版本均被拒绝。此轮复用既有验题证据，未重跑正式参考验证或并行 Worker。

### 本机已发布 A+B 浏览器联验

在[专用 Ubuntu VM](isolated-checks.md#专用-ubuntu-vm-判题实验)内使用一次性 PostgreSQL、Redis 和 P4 的[本地执行准入](delivery-evidence/p4-trial-publication.md#mac-ubuntu-vm-本地完整流程)。普通 API 的 `local_training_intake: true` 要求同时开启 `training_history`、`training_submission_detail`、本地执行及题库；缺项拒绝启动。此开关默认关闭，仅用于回环 HTTPS 和本地 PostgreSQL Unix socket。

可在同一套 P4 浏览器夹具中加 `OJ_BROWSER_LOCAL_TRAINING_INTAKE=1`。以已核验的 Ubuntu 运输清单启动对应的 Worker 与私有 mTLS 控制端，指向夹具的临时数据库。完成 P4 批准并用 `problem-review publish-aplusb-local` 发布后，在 Mac 执行：

```sh
OJ_BROWSER_QA_INFO="$FIXTURE_READY_JSON" OJ_BROWSER_QA_OUTPUT="$QA_DIRECTORY" \
  OJ_PLAYWRIGHT_MODULE="$LOCAL_PLAYWRIGHT_ENTRY" OJ_CHROMIUM_EXECUTABLE="$LOCAL_CHROMIUM" \
  node tests/browser/published-training-qa.mjs
```

该脚本通过浏览器正常账号登录和选题，只上传并提交固定 A+B 源码。它验证文件限制、替换确认、刷新后原键回执、真实判定及账户隔离；不会启动 Worker 或替代隔离检查。验证边界见[交付证据](delivery-evidence/p5-published-training.md)。

在同一夹具中设置 `OJ_BROWSER_TRAINING_LANGUAGE=c17` 或 `OJ_BROWSER_TRAINING_LANGUAGE=gnu++20`，验证对应的源码上传、AC、WA、编译错误和本人回读；省略变量回归 GNU++17。须先排空并停止旧 API、Worker 及维护写入，迁移到 schema 33，再用同一修订启动。旧 Worker 不能处理新版语言任务。证据和回退说明见 [P7 C17](delivery-evidence/p7-c17.md) 与 [P7 GNU++20](delivery-evidence/p7-gnu-cpp20.md)。

双 Worker 仅用于本机专用 Ubuntu VM。按原 `oj-aplusb-ubuntu-sandbox` 的固定配置另建停止的 `oj-aplusb-ubuntu-sandbox-secondary`，保持断网、只读、无挂载、固定镜像和每槽配额。给两节点分别发放证书；两终端沿用同一材料、控制端及启动参数，分别多设 `OJ_UBUNTU_TEXT_DUAL_SLOT=primary` 和 `secondary`。默认仍为原独占槽；不要与 M0 双槽或参考验题混跑。使用 `tests/browser/c17-resilience-qa.mjs` 和上文环境变量，`OJ_C17_PHASE=limits|overlap|fault|read` 选择提交超限、并发、待接管或按保存的 JSON 本人回读；`OJ_BROWSER_QA_OUTPUT` 为该 JSON 路径。此专项默认 C17；`OJ_BROWSER_TRAINING_LANGUAGE` 也可选 `gnu++20`、`java21` 或 `python3`。Java 或 Python 专项须在两个槽启用相同的已准入镜像和语言设置。

正常退出用原排空流程。若进程意外退出并留隔离标记，先排空另一个 Worker，确认旧宿主控制操作全部结束，再用该槽的 `OJ_UBUNTU_TEXT_DUAL_SLOT` 和历史完整容器 ID 执行 `worker recover-aplusb-ubuntu`。恢复命令独占本机执行锁；原容器 ID 或停止状态无法确认时保留隔离。

同一夹具、参数和已发布题可将脚本替换为 `tests/browser/published-training-drafts-qa.mjs`，验证本地草稿、冲突及退出。该专项不发判题请求，不占用 Worker 槽。

### 本机文本题验题与提交

Java 21 使用另一个[本机判题制品和启用步骤](delivery-evidence/p7-java21.md)。浏览器 A+B 专项设置 `OJ_BROWSER_TRAINING_LANGUAGE=java21`；需启用 Java 的本机 API、前端及 Worker。

Python 3.14 使用[本机组合判题镜像](delivery-evidence/p7-python3.md)。浏览器 A+B 专项设置 `OJ_BROWSER_TRAINING_LANGUAGE=python3`；需分别启用 Python 的本机 API、前端及 Worker。镜像保留 GCC 和 Java，Java 入口仍独立启用。

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

本机浮点题在完整包审核中用 `quality/decimal.json` 显式开启，不与 SPJ 混用。配置为单行 JSON 加换行，键按字母排序，例如 `{"absolute":"0.01","relative":"0.001","text":["RANGE"]}`。容差是十进制字符串，至少一项；`text` 是排序后的标准答案文字标签，可省略，`case_sensitive:true` 启用 ASCII 大小写敏感。参考试跑和独立标程对照仍要求文本一致，四类错解用实际浮点规则检查。用合成题包脚本的 `--decimal` 可复测；比较规则见[判题设计](../architecture/judging-design.md#3-显式浮点容差)。

本机 Testlib SPJ 在同一审核流程中增加检查器证据和制品 pin。带 `quality/checker.cpp` 的题包必须附 SPJ 报告才可准入；专用 Worker 此时使用组合镜像和 `OJ_LOCAL_SPJ_CHECKER_FILE`。用上文的浏览器专项复测数列极差时，设 `OJ_BROWSER_SPJ_SPAN=1`、`OJ_BROWSER_TRAINING_LANGUAGE=gnu++17`；另可用 `OJ_C17_PHASE=budget` 验证单个 Worker 连续判四份完整提交。预算与故障接管实测见 [P7 Testlib SPJ](delivery-evidence/p7-testlib-spj.md)。
