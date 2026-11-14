# 私有题库入库与管理

从仓库根目录运行本文命令。准备题源和执行离线验证见[题源与参考验题](problem-maintenance.md)，草稿、审核与权限边界见[私有草稿合同](../architecture/problem-drafts.md)。

### 转换为统一私有草稿

已有完整私有候选后，用 Python 3.11+ 在宿主进行静态适配；不执行题源程序、不联网或安装依赖：

```sh
python3 -B scripts/import-library-checker.py \
  --corpus /private/tmp/oj-library-checker-corpus/materials \
  --output /private/tmp/oj-library-checker-drafts
python3 -B scripts/import-library-checker.py \
  --verify-bundle /private/tmp/oj-library-checker-drafts/bundle
python3 -B -m unittest discover -s tests/governance -p test_problem_drafts.py -v
```

输入须为稳定私有快照；输出为仓库外、与输入不重叠的新目录。工具重新核对源码和全部数据哈希，生成自含的10题/254点/18样例草稿、英文Markdown题面、来源映射与许可对象。所有路径相对bundle，移到其他目录后可独立复验，不依赖原候选目录。失败保留 `INCOMPLETE` 和部分文件，重试用新目录；临时目录不是持久备份。格式、权限及发布阻断见[草稿合同](../architecture/problem-drafts.md)。已增加私有草稿入库服务；审核和生产判题配置仍未接通；已有按ID本人题面预览。

从已审查的 v6 题包派生边界修订时，另建私有单题包，保留原包供旧修订使用。已核验的用例及范围见[题目来源记录](../architecture/problem-sources.md#输入覆盖盘点)：

```sh
python3 -B scripts/supplement-zalgorithm.py \
  --base-bundle /private/tmp/oj-library-checker-drafts/bundle \
  --output /private/tmp/oj-zalgorithm-scale
python3 -B scripts/import-library-checker.py \
  --verify-bundle /private/tmp/oj-zalgorithm-scale/bundle
python3 -B scripts/supplement-zalgorithm-alternating.py \
  --base-bundle /private/tmp/oj-library-checker-drafts/bundle \
  --output /private/tmp/oj-zalgorithm-alternating
python3 -B scripts/supplement-associative-array.py \
  --base-bundle /private/tmp/oj-library-checker-drafts/bundle \
  --output /private/tmp/oj-associative-array-boundaries
python3 -B scripts/supplement-suffixarray.py \
  --base-bundle /private/tmp/oj-library-checker-drafts/bundle \
  --output /private/tmp/oj-suffixarray-structures
python3 -B scripts/supplement-primality-test.py \
  --base-bundle /private/tmp/oj-library-checker-drafts/bundle \
  --output /private/tmp/oj-primality-single-query
python3 -B scripts/supplement-convolution-mod.py \
  --base-bundle /private/tmp/oj-library-checker-drafts/bundle \
  --output /private/tmp/oj-convolution-coefficients
python3 -B scripts/supplement-aplusb.py \
  --base-bundle /private/tmp/oj-library-checker-drafts/bundle \
  --output /private/tmp/oj-aplusb-boundaries
python3 -B scripts/supplement-unionfind.py \
  --base-bundle /private/tmp/oj-library-checker-drafts/bundle \
  --output /private/tmp/oj-unionfind-singleton
python3 -B scripts/supplement-static-rmq.py \
  --base-bundle /private/tmp/oj-library-checker-drafts/bundle \
  --output /private/tmp/oj-static-rmq-boundary
python3 -B scripts/supplement-point-add-range-sum.py \
  --base-bundle /private/tmp/oj-library-checker-drafts/bundle \
  --output /private/tmp/oj-point-add-range-sum-boundaries
python3 -B scripts/supplement-static-range-sum.py \
  --base-bundle /private/tmp/oj-library-checker-drafts/bundle \
  --output /private/tmp/oj-static-range-sum-singleton
```

交替串命令生成[独立 32 点修订](../architecture/problem-sources.md#z-算法交替串修订)，不替换 29 点原包或 31 点规模修订。真实验收驱动 `tests/integration/live_reference.py` 使用 `--zalgorithm-alternating-data` 指定新修订，沿用已配置并停止的隔离槽和一次性 PostgreSQL 夹具；`--zalgorithm-local-data` 仍只接受 31 点。

```sh
OJ_SANDBOX_EXPERIMENT=1 python3 -B tests/integration/live_reference.py \
  --bundle /private/tmp/oj-zalgorithm-alternating/bundle \
  --toolchain-image "$REFERENCE_TOOLCHAIN_IMAGE" \
  --output /private/tmp/oj-zalgorithm-alternating-report \
  --start-stopped-slots --zalgorithm-alternating-data
```

离线盘点输入覆盖时，对各题包分别生成报告。只读取已核验题包，不执行题源程序；用私有输出路径保留报告：

```sh
umask 077
python3 -B scripts/audit-library-checker-coverage.py \
  --bundle /private/tmp/oj-library-checker-drafts/bundle >/private/tmp/oj-v6-input-coverage.json
python3 -B scripts/audit-library-checker-coverage.py \
  --bundle /private/tmp/oj-zalgorithm-scale/bundle >/private/tmp/oj-zalgorithm-input-coverage.json
python3 -B scripts/audit-library-checker-coverage.py \
  --bundle /private/tmp/oj-aplusb-boundaries/bundle >/private/tmp/oj-aplusb-input-coverage.json
```

判定口径、版本及已观测缺口见[输入覆盖盘点](../architecture/problem-sources.md#输入覆盖盘点)。

### 出题角色维护与草稿入库服务

出题角色始于schema 7，当前仍默认不给任何账号授予角色。受信维护者先独立迁移，再用 Docker 工具链构建的命令明确授予/撤销；`OJ_DATABASE_URL` 通过既有私有环境注入，不把真实连接信息写入命令或日志：

```sh
.local/bin/migrate up
.local/bin/problem-author inspect --subject SUBJECT_ID
.local/bin/problem-author set --subject SUBJECT_ID --expected-revision 0 \
  --enabled true --operator MAINTENANCE_OPERATOR --reason AUDIT_REASON
```

替换大写占位值；主体为已存在的32位小写十六进制ID。初次修订为0；之后必须使用inspect读出的实际修订。撤销用 `--enabled false`，不自动重试冲突或未知结果。`operator`仅为受信维护操作的审计归属，不是已验证的账号登录。单独镜像可通过 `docker --context desktop-linux build --target problem-author -t oj-problem-author:local .` 构建，入口为 `/usr/local/bin/oj-problem-author`，默认UID65532；数据库连接由维护流程单独配置，默认Compose不运行该工具。

`internal/problem.Service` 的创建/本人读取必须传真实普通会话凭据；授权后把选定草稿的全部材料写入私有Store，再读会话，并在同一PG事务核对恢复/账号/角色及写元数据。`LoadBundle`的expectedSourceLock来自受信配置原字节；材料根与bundle均由维护者预建。可选HTTP接入见下节；管理页面、审核和发布仍待完成，完整边界、幂等、迁移与实测归[草稿合同](../architecture/problem-drafts.md#私有草稿入库与本人读取)。

`internal/problemreference.Service` 提供[持久化参考验证申请](../architecture/reference-requests.md)：绑定实际会话及本人草稿，保存受信来源、完整导入修订、Profile 原字节和原授权快照，并支持原键回执与取消。参考申请默认没有准入 policy，须受信维护显式配置后才接受新申请。作者显式准备后，服务冻结对象并重验授权；固定卷积的新申请同时保存[独立对拍计划](../architecture/reference-requests.md#卷积独立对拍的预期计划)，供专用参考 Worker 在正式验证后执行。作者可回读本人终态摘要；草稿保持 `not-run`，发布仍阻断。精确升级与回退条件见[迁移与回退](../architecture/reference-requests.md#迁移与回退)。

参考领取为显式内部操作：控制端仅在设置 `OJ_WORKER_REFERENCE_NODE_ID` 后启用。节点须已由受信维护配置，持有 `spiffe://onlinejudge/reference-worker/{node_id}/{credential_version}` 客户端证书。沿用 Worker 的控制端地址、CA、证书和私钥配置，清空双槽和 M0 槽覆盖变量，以原回执中的申请 ID、固定来源及预期 Plan 摘要运行：

```sh
env -u OJ_WORKER_M0_SLOT -u OJ_WORKER_M0_DUAL_EXPERIMENT OJ_SANDBOX_EXPERIMENT=1 \
  .local/bin/worker claim-reference REQUEST_ID SOURCE_ID EXPECTED_PLAN_SHA256
```

命令获取同机槽后只发送一次领取，再在同一锁内下载并校验冻结计划的全部材料，关闭匿名快照后退出；数据库预留仍按原时钟在 30 秒后失效。控制端还须配置 `OJ_WORKER_REFERENCE_MATERIAL_ROOT`，使用草稿入库的已存在私有对象根（Linux、当前 UID、0700；上限 128 MiB/对象，同时一个快照），与 M0 的 `OJ_MATERIAL_ROOT` 含义不同。无题源执行、续租或回报；未知响应和下载失败不自动重试，低带宽下载大题可能超时。见[参考材料合同](../architecture/reference-requests.md#有效预留内的材料下发)。

同机受信执行另需控制端 `OJ_WORKER_REFERENCE_EXECUTION=1`，以及已预配且停止的固定隔离槽。使用同一参数运行 `.local/bin/worker run-reference REQUEST_ID SOURCE_ID EXPECTED_PLAN_SHA256`（同样要求 `OJ_SANDBOX_EXPERIMENT=1` 且清空双槽/M0 槽覆盖）。命令在原锁内续期、下载、执行并确认停止；不创建、恢复或替换容器。停止确认及清理后提交确定结果，控制端接纳不推进发布。停止无法确认时保留隔离，见[持槽执行合同](../architecture/reference-requests.md#同机持槽参考执行)。

另外显式设置控制端 `OJ_WORKER_REFERENCE_DISPATCH=1` 后，可使用 `.local/bin/worker run-next-reference SOURCE_ID [AFTER_ID]` 按来源选择并执行一个此前未领取的已准备申请。槽忙不发请求；空页可用输出的 `next_after` 显式继续。未知领取不自动重试，旧计划不会自动进入选择，见[选择合同](../architecture/reference-requests.md#按来源选择已准备申请)。

对有独立对拍计划的卷积申请，上述两个执行入口会在正式点通过后继续用同一槽对拍，确认停止后一次提交两份证据。合法首差异会保留为失败证据，命令返回失败。实物专项使用上方 Darwin arm64 构建的 `worker-validation.test`，设置 `OJ_CONVOLUTION_LEASED_SANDBOX_TEST=1`、`OJ_SANDBOX_EXPERIMENT=1`、`OJ_CONVOLUTION_BUNDLE`、`OJ_CONVOLUTION_SOURCE_ROOT` 和 `OJ_CONVOLUTION_SOURCE_LOCK`，清空两个槽覆盖并运行 `-test.run='^TestClaimedConvolutionOwnedSandbox$' -test.count=1 -test.timeout=4m`。确认原槽已停止后，可另设 `OJ_CONVOLUTION_LEASE_LOSS=1` 做续期失效对照。真实 mTLS／PostgreSQL 报告接纳与宿主执行证据分别记录。

### 十题真实报告接纳

在 macOS arm64 用既有 Docker Desktop 工具链和专用槽显式运行：

```sh
OJ_SANDBOX_EXPERIMENT=1 python3 -B tests/integration/live_reference.py \
  --start-stopped-slots --bundle /private/tmp/oj-library-checker-drafts/bundle \
  --toolchain-image YOUR_EXISTING_LOCAL_GO_TOOLCHAIN_IMAGE \
  --output /private/tmp/oj-live-reference-reports
```

题包父目录须归当前用户所有、权限为 0700，且只含题包；挂载整个父目录可保留 Linux 读到的题包所有者。工具链须已缓存依赖并提供 PostgreSQL 工具，以非 root 用户运行。输出须是已有父目录下的新目录，位于仓库与题包父目录之外。五个预配槽须已停止且无隔离标记；命令不创建、替换或恢复槽。临时控制端只向宿主回环发布，数据库使用容器内 Unix socket。执行仍在原断网槽，首个失败即停止批次，原始日志保留；成功才写 `summary.json`。见[报告接纳专项](../architecture/reference-requests.md#十题实物报告接纳专项)。

验证单题修订时，在同一命令追加 `--zalgorithm-local-data`（31 点）、`--aplusb-local-data`（17 点）、`--unionfind-local-data`（20 点）或 `--static-range-sum-local-data`（13 点），将 `--bundle` 指向对应的单题包，并为 `--output` 另选新目录。专项要求冻结和接纳同一份计划；并查集还核对仅合并用例的零字节答案，静态区间和核对两个整段查询的固定答案。A+B 还核对指定非作者读取覆盖、输入校验、独立小用例与历史渲染摘要，作者和未分配者被拒。夹具确认只解除题面缺项，撤回分配后重分配及退回会恢复该缺项；退回还恢复覆盖缺项。原 v6 的 A+B 保留原质量摘要且无覆盖摘要。

关联数组的同一专项使用 `--associative-array-local-data`（22 点），额外核对两个边界点的固定答案。

并查集 20 点专项还回读指定非作者的审核摘要：输入校验与独立小用例引用同一报告，覆盖摘要另含四种错解和本机 CPU 对照。专项核对当前审核预览与历史题面投影相同；夹具确认只解除题面缺项，重新分配须重新确认。材料退回恢复题面和覆盖缺项，保留历史摘要；生产检查器和目标环境仍保留缺项。

后缀数组使用 `--suffixarray-local-data`（52 点），额外核对两个新增点的完整后缀排序。

静态区间最小值使用 `--static-rmq-local-data`（28 点），额外核对只查询一次时的上界答案。

单点加与区间求和使用 `--point-add-range-sum-local-data`（23 点），额外核对加零及累加到 3×10⁹ 时的固定答案。

素性测试使用 `--primality-test-local-data`（15 点），额外核对两个单次查询的 `Yes` 与 `No` 答案。

卷积使用 `--convolution-local-data`（55 点），额外核对两条系数边界答案及绑定同一正式计划的 24 个独立对拍。

上述 Darwin 专项产物 `worker-validation.test` 可在专用固定离线槽运行 `-test.run '^TestClaimedReferenceOwnedSandbox$' -test.v`，显式设置 `OJ_REFERENCE_LEASED_SANDBOX_TEST=1 OJ_SANDBOX_EXPERIMENT=1` 并清空槽覆盖；须与其他宿主专项串行。只使用固定合成 A+B 和续期替身，分别验证真实执行与续期拒绝后的停止。


### 显式启用私有草稿 HTTP

在已有 `OJ_SESSION_CONFIG_FILE` 普通会话配置内增加下列对象（示例预算需按材料量实测调整）：

```json
{
  "private_drafts": {
    "material_root": "/store/private-materials",
    "bundle_directory": "/input/private-bundle",
    "source_lock_file": "/config/library-checker-source-lock.json",
    "source_ids": ["library-checker:aplusb"],
    "timeout": "30s",
    "max_in_flight": 1
  }
}
```

仅Linux；`material_root`必须是按[存储合同](../architecture/material-store.md)预建持久根，bundle为已准备的稳定私有快照，两者均由运行euid所有、目录0700/文件0600。来源锁从仓库受信的[固定来源锁](../../config/problem-sources/library-checker.json)单独配置，不得从请求或bundle自行选作信任根。三个路径须绝对规范，材料根与bundle不得互相包含，锁须在两者之外；部署者也须保证实际挂载/祖先没有把它们别名到同一位置。bundle/锁只读挂载，材料根可写，镜像默认UID65532须有对应访问权。服务不自动建目录、迁移、授予角色或修复文件。

`source_ids`为1—10个不重复候选，启动逐项加载元数据；大对象仍在授权后的导入中验证。`timeout`为正且至多1分钟，`server.write_timeout`不得小于它；`max_in_flight`为1—10，由本实例已授权的导入/预览材料阶段共享，无等待拒绝，不限制初次预检、普通元数据GET或HTTP输出，不是共享频率或磁盘总额。每对象128MiB、固定bundle上限及同题单份Prepared活动约束继续适用。缺省整个对象关闭能力，关闭后重启保留草稿和材料，无迁移；旧二进制回退前也先移除该对象。

已通过真实普通登录的出题账号可向`POST /api/v1/problem-drafts`发送`{"source_id":"library-checker:aplusb","request_key":"original-import-request-01"}`，需要同源证据及会话Cookie；最多1KiB JSON。成功/原回执均200，再以`GET /api/v1/problem-drafts/草稿ID`读取本人固定JSON元数据。断连/503可能已经提交，保留原键及精确候选快照核验，不自动重试或换键；配置/候选版本切换前须保留原版本供未知回执恢复。完整协议见[OpenAPI](../../api/problem-drafts.openapi.yaml)。默认Compose及本地测试账号均不自动开启或取得出题权。

`/author/drafts`已接来源选择、绑定导入及本机回执恢复，使用[主体绑定管理协议](../../api/problem-draft-management.openapi.yaml)：先`GET /api/v1/problem-draft-management/sources`取得当前授权候选的`source_id/title/version/import_revision`，再向`POST /api/v1/problem-draft-management/imports`提交严格三字段`source_id/request_key/expected_revision`，其中expected_revision原样取完整import_revision。所有管理路由都必须带当前检查的`X-OJ-Expected-Subject`；服务端只把它与实际会话比较，不作为身份来源，账号变化409且业务零访问。完整修订还绑定index/来源证据，不能用仅代表题目文档的version代替；修订变化在材料I/O前409拒绝。成功返回八字段最小回执，没有文档或材料引用。

若响应丢失或结果未知，保留原owner/source/key/revision，以`GET /api/v1/problem-draft-management/imports/原请求键`只读查询本人已提交回执，不必重传材料。该GET不依赖当前候选或材料活动槽，但整个private_drafts仍须有效配置并启动成功。404只是本次没有观察到提交，409也不证明旧请求不存在；保留原tuple，显式再次查询或用原tuple恢复，不自动POST、不换键/修订。页面先等本机IndexedDB严格事务提交才发起POST，最多32条/账号，不自动清除pending；刷新/恢复/账号变化只清显示。旧两字段及[先前绑定协议](../../api/problem-draft-imports.openapi.yaml)保持兼容，页面绝不回退它们，旧二进制404须停写。`/author/drafts/open`保留按已有ID选择入口；显式图片预览已接入，审核/发布待交付。本机恢复记录的保留、导出与存储丢失限制见[界面约定](../architecture/frontend-design.md#私有草稿导入管理)，细节和回退见[草稿合同](../architecture/problem-drafts.md#管理页导入协议)。

服务器本人列表为`GET /api/v1/problem-draft-management/drafts`，带同一预期主体header。首次无query，之后原样传单个`cursor=next_cursor`；每页最多20条，空next_cursor表示本次末页，无总数/他人统计。只读最小回执，不读题面或材料；网页即使本机记录已清或IndexedDB不可用也能手动发现已提交草稿并打开预览，不自动确认或清理pending。逐页视图不是跨请求快照，较早时间戳迟提交可能被旧边界漏过，并发时重查首页；空/末页不是取消或无在途证明。schema8索引的维护升级/回退见[本人列表合同](../architecture/problem-drafts.md#本人草稿列表)。

要启用本人参考申请，在 `private_drafts` 中显式设 `"reference_requests": true`，并由受信维护者设置独立队列额度（示例值，不代表容量建议）：

```sh
.local/bin/migrate reference-backlog 4 2
```

命令使用既有私有 `OJ_DATABASE_URL`，只写参考队列额度，要求当前 schema，不自动迁移。随后可在本人草稿预览提交、查原键回执、显式准备材料计划和取消。收到报告后可点击“查看验证结果”，查看正式判定和可用的独立对拍摘要；见[作者申请合同](../architecture/reference-requests.md#作者申请入口)。API 不执行题源，不更新草稿验证状态。

作者送审的 HTTP 入口可在上述配置内另设 `"review_intake": true`，默认关闭。入口保存本人草稿的完整导入修订和选中的已接纳报告，提供原键回执。作者也可按原键[核对并确认冻结标程](../architecture/problem-drafts.md#作者确认标程)。页面尚未接入送审及确认，草稿状态不变。

审核证据读取独立设 `"review_access": true`。指定审核人可以按送审 ID 读取固定验证摘要、[对拍详情](../architecture/problem-drafts.md#审核对拍详情)、[批准阻断](../architecture/problem-drafts.md#批准准入状态)和[题面及标程](../architecture/problem-drafts.md#审核材料读取)。维护者用 Docker 工具链构建的命令管理资格和分配，数据库连接由私有环境注入：

```sh
.local/bin/problem-review reviewer-set --subject REVIEWER_ID --expected-revision 0 --enabled true --operator MAINTAINER --reason 'confirmed non-author reviewer'
.local/bin/problem-review assign --review REVIEW_ID --reviewer REVIEWER_ID --reviewer-revision 1 --expected-revision 0 --operator MAINTAINER --reason 'assign exact review item'
.local/bin/problem-review reviewer-inspect --subject REVIEWER_ID
.local/bin/problem-review assignment-inspect --review REVIEW_ID
.local/bin/problem-review withdraw --review REVIEW_ID --expected-revision 1 --operator MAINTAINER --reason 'withdraw assignment'
```

用 32 位小写十六进制主体及送审 ID 替换占位值。修订 0 仅指首次记录；撤销资格用当前修订与 `--enabled false`。改派也以最新分配修订为前置条件。当前无页面入口、批准或发布。

如需允许指定审核人写退回决定，再设置 `"review_returns": true`；只开 `review_access` 仍为只读。作者能按原送审键查看理由；改用新键不能再次提交相同修订与证据。请求及原键恢复见[审核退回与重送](../architecture/problem-drafts.md#审核退回与重送)。

如需记录当前指定非作者对冻结题面的人工核对，启用 `"statement_confirmations": true`（需要 `review_access`，默认关闭）。写入与原键回执见[审核人确认题面语义](../architecture/problem-drafts.md#审核人确认题面语义)。

普通终审批准 HTTP 入口默认关闭；`"review_approvals": true` 需要 `review_access`。启用仍必须通过服务端准入检查，并不会发布题目；见[批准准入状态](../architecture/problem-drafts.md#批准准入状态)。

需要读取已发布题目时，在 `private_drafts` 中显式设置 `"public_catalog": true`。匿名选手访问 `/problems`，其同源 HTTPS API 只读发布账本和封存题面；示例题仍在首页。见[选手题库只读入口](../architecture/problem-drafts.md#选手题库只读入口)。

发布资格单独维护，默认无授权。使用相同的私有数据库连接及当前 schema：

```sh
.local/bin/problem-review publisher-inspect --subject PUBLISHER_ID
.local/bin/problem-review publisher-set --subject PUBLISHER_ID --expected-revision 0 --enabled true --operator MAINTAINER --reason AUDIT_REASON
```

首次授权用修订 0；以后先读当前修订。撤销使用 `--enabled false`，结果未知时先查回。该工具只管理[发布资格](../architecture/problem-drafts.md#发布资格)，不发布题目。

`GET /api/v1/problem-drafts/草稿ID/preview`返回本人已入库版本的`markdown`、有序`examples`（id/input/output）和可选`images`，不依赖当前候选版本；仍须有效普通会话和当前出题权。先核对全部引用与总预算，再逐份验证快照，读取后复核同一凭据和PG授权，失败不返回部分结果。原始markdown/input/output/显式原图合计至多1MiB，重复对象按返回次数计数；图片完整解码后限长重编码为静态PNG，输出PNG与原文/样例另限1MiB。图片角色/格式/像素/路径及回退限制见[图片合同](../architecture/problem-drafts.md#显式题面图片)。JSON文本转义至多六倍，图片base64另加元数据。文本严格UTF-8，保留空格、CRLF及末尾换行，不替换或截断。它是管理页数据，不提供一般材料下载或发布；正式测试与程序源码不进入预览。最终会话读取之后的退出及最终PG授权之后的撤权沿已接受在途合同，不能原子撤回已经开始发送的响应。关闭整个`private_drafts`能力即可停止入口，已入库资料保留。

浏览器可从 `/author/drafts` 的本人列表选择已提交草稿，或在 `/author/drafts/open` 输入ID，再显式检查会话并打开本人预览。Markdown/GFM/数学公式使用共享组件和本地KaTeX字体，样例原文复制；清单图片精确匹配后使用静态PNG数据，其他图片占位且无源地址请求，审核和发布尚未开放。账号操作、页面恢复/隐藏或期限到达会清除题面和图片，再次显示须重新检查；无持久缓存。预算与隐私边界见[前端设计](../architecture/frontend-design.md#共享题面与本人预览)。默认HTTP Compose及未启用private_drafts的账号环境不提供草稿能力。
