# OnlineJudge

面向东华理工大学计算机协会的训练与校赛平台。当前已有持久化/租约/固定材料 Worker 实验，以及显式 HTTPS 下普通账号的登录、会话、改密与浏览器面板；Docker Desktop 专用容器验证 GNU++17 沙箱。完整注册、权限、训练产品及 M0 整体验收仍待交付，未部署。

## 当前交付与下一步

- [开发与交付计划](docs/phase-1-development-plan.md)：发布范围、组件引入时机、P0—P7 与里程碑。
- [M0 记录](docs/m0-experiment.md)：Worker、数据库与会话实验的证据及剩余缺口，是当前进度入口。
- [共享提交积压准入](docs/submission-backlog.md)：schema 6、显式全局/主体策略、跨 API 原子检查及升级/回退边界；频率限流和公平调度待完成。
- [会话实验](tests/sessionexperiment/README.md)：独立测试模块的复现与限制，尚未接入正式登录。
- [密码记录核心](docs/architecture/session-design.md#71-同步密码记录核心)与[主体凭据](docs/architecture/session-design.md#72-主体凭据持久化与条件更新)：显式策略的 Argon2id、PostgreSQL 状态与条件更新，HTTP 接线见下述入口。
- [密码计算准入](docs/architecture/session-design.md#73-密码计算准入)：创建/验证共享实例额度、有限排队及取消后保留实际计算名额；多 API 总预算与生产容量仍待验证。
- [密码验证用例](docs/architecture/session-design.md#74-密码验证与条件升级用例)：同密码升级、双版本条件更新与末次状态复核；结果只描述时点，不发放会话或资源权限。
- [主体状态协调](docs/architecture/session-design.md#75-会话存储操作的主体状态协调)：共享行锁协调受信会话存储操作与账号变更；回调失败仍可能有外部效果，恢复编排待接入。
- [全局恢复状态](docs/architecture/session-design.md#76-正式全局恢复状态与双层协调)：schema 4 默认闭门、显式换代次与条件开门，同事务全局→主体双层检查；没有自动 Redis 清理或恢复编排。
- [个人邮箱映射](docs/architecture/session-design.md#77-个人邮箱映射与原子初始化)：schema 5 原子初始化凭据与未验证邮箱，整址忽略大小写、保留原始显示；当前限 ASCII 裸址，无公开注册或邮件验证接口。
- [个人凭据认证路径](docs/architecture/session-design.md#78-个人凭据到会话门禁的认证路径)：在邮箱查找与密码验证前捕获恢复代次，携带同一代次进入最终双层门禁。
- [普通 Redis 会话协议](docs/architecture/session-design.md#79-普通-redis-会话协议)：正式核心的三名额、固定创建尝试、结果确认、续期和墓碑撤销；真实 Redis/PostgreSQL 专项接入集成 CI，HTTP 发放由下述两步适配承担。
- [已有会话 HTTPS 适配](docs/architecture/session-design.md#710-已有普通会话的-https-适配)：只读身份、显式活动与确认退出，固定 HTTPS 来源和严格 Cookie；尚未挂载默认 API，也不发放登录 Cookie。
- [跨请求登录与 Cookie 发放](docs/architecture/session-design.md#711-跨请求登录尝试与-cookie-发放)：无会话写入的准备、密文候选恢复、实际执行点截止和确认后发放；[独立密钥文件加载](docs/architecture/session-design.md#712-登录密钥文件加载)，独立 HTTPS Handler，默认 API 未挂载。
- [共享登录限流](docs/architecture/session-design.md#713-共享登录限流)：跨副本全局/来源/邮箱联合额度、独立恢复额度与失败关闭；严格 Handler 构造入口供后续默认 API 使用，生产阈值待验。
- [单跳受信反代](docs/architecture/session-design.md#714-单跳受信反向代理)：显式 mTLS 证书指纹、固定公开来源与严格覆写头，独立代理入口要求登录限流；默认 Compose、生产代理配置和其余浏览器兼容仍待验。
- [会话运行时装配](docs/architecture/session-design.md#715-显式配置与运行时装配)：显式预算、单一身份权威、启动只读检查及关闭排空；typed owner 由下述显式 API 入口接线。
- [敏感对象输出保护](docs/architecture/session-design.md#716-会话敏感对象的输出保护)：六种会话敏感类型的直接值/指针合法格式保护，登录配置 JSON 隐藏密钥；不替代禁止秘密进入日志的规则。
- [显式会话 API 入口](docs/architecture/session-design.md#717-显式会话-api-入口)：`OJ_SESSION_CONFIG_FILE` 启用严格配置的 TLS/mTLS 命令入口，必须同时指定 `OJ_API_ADDR`；默认 Compose 未启用。
- [已登录改密](docs/architecture/session-design.md#718-已登录用户修改密码)：显式能力、独立三维额度和弱密码规则，核验当前密码后原子换代并撤销旧会话；[接口契约](api/account.openapi.yaml)。
- [浏览器账号面板](docs/account-browser.md)：已有账号登录、原尝试恢复、检查/主动续期/退出和改密；[六路径契约](api/session.openapi.yaml)。默认 HTTP 预览阻止密码操作，不代表注册、训练或完整权限已开放。
- [Web 界面](docs/frontend-design.md)：简洁题库首页、独立登录页和双栏做题预览；示例可搜索、筛选、编辑和导入导出，正式提交尚未开放。
- [首批题源](docs/problem-sources.md)：固定 Library Checker 10 题的断网数据准备与私有候选验证，尚未接入正式题库或发布。

## Library Checker 首批题源准备

需要已有官方源码副本及本地 Docker 镜像（Python 3.11+、`g++` C++17、GNU tar、`/bin/sleep`、`/usr/bin/env`）。工具只使用已有镜像，不自动下载或安装依赖；镜像须无声明式 `VOLUME`。本次本机实测镜像和精确结果见[题源交付记录](docs/delivery-log.md)，其他环境须重新验证。

首次准备源码时下载官方仓库并固定提交；目标目录须为新目录：

```sh
git clone https://github.com/yosupo06/library-checker-problems.git /private/tmp/library-checker-problems
git -C /private/tmp/library-checker-problems checkout --detach 73876f1cbcf082d2b22722924b924d3d65a2f00e
python3 -B scripts/prepare-library-checker.py \
  --source /private/tmp/library-checker-problems \
  --image YOUR_EXISTING_LOCAL_IMAGE \
  --output /private/tmp/oj-library-checker-corpus
```

`--output` 必须为仓库外的新目录。成功生成 `materials/catalog.json`、来源/许可、全部数据及 `oj.source-validation.v6` 报告。报告包含十题输入校验对照、每题一份固定错误解的正式 WA、关联数组低效解 CPU 对照和十题共 198 个独立小用例；失败保留 `INCOMPLETE`，重试使用新目录。私有候选不会自动进入数据库、Web 题库或 Worker。工具行为回归：

```sh
python3 -B -m unittest discover -s tests/governance -p test_problem_sources.py -v
python3 -B -m unittest discover -s tests/governance -p test_library_checker_oracles.py -v
python3 -B -m unittest discover -s tests/governance -p test_library_checker_wrong_cases.py -v
```

### 首批文本检查器差分

已有完整私有候选及同时含 Go 1.26.11、GNU++17 的本地镜像时，显式执行下列专用隔离实验。替换镜像为已审阅的本地不可变 ID；`CHECKER_CORPUS` 指向已有 `materials/` 目录。不下载/安装工具，不在宿主或普通 CI 执行题源 checker：

```sh
CHECKER_CORPUS=/private/tmp/oj-library-checker-corpus/materials
CHECKER_IMAGE=sha256:YOUR_EXISTING_REVIEWED_IMAGE_ID
docker --context desktop-linux run --rm --network none --read-only \
  --cap-drop ALL --security-opt no-new-privileges:true --user "$(id -u):$(id -g)" \
  --memory 4g --memory-swap 4g --cpus 2 --pids-limit 128 \
  --ulimit nofile=256:256 --ulimit fsize=134217728:134217728 --ulimit core=0:0 \
  --no-healthcheck --tmpfs /work:rw,exec,nosuid,nodev,size=1g,mode=1777 \
  --mount "type=bind,src=$PWD,dst=/src,readonly" \
  --mount "type=bind,src=$CHECKER_CORPUS,dst=/corpus,readonly" \
  --workdir /src --entrypoint /usr/bin/env "$CHECKER_IMAGE" \
  -i PATH=/usr/local/go/bin:/usr/bin:/bin HOME=/work TMPDIR=/work \
  GOCACHE=/work/go-cache GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off \
  OJ_CHECKER_COMPAT=isolated-docker \
  go test -tags=checkercompat -count=1 -timeout=10m -v ./tests/checkercompat
```

镜像须无声明式 VOLUME，运行用户必须非 root 且可读取私有候选；在 root 环境须另配明确非 root UID/GID，不直接复制执行上述用户参数。所有写入仅在临时内存盘，编译/检查子进程各限120秒/5秒。模式 `lc-wcmp-tokens-v1` 保留 M0 规则，区别 VT/FF 并显式约束字节/token；[支持域与限制](docs/checker-admission.md)不等于完整题目验证或生产预算准入，草稿继续 `not-run`。

### 生产检查器候选预检

在已有 Go 工具链容器内，将已准备的私有题包和固定来源锁挂为只读；运行用户须满足题包的私有文件所有权要求。以下变量分别来自当前题包路径、已接纳的正式参考记录和本次待验证的显式限额，未设置默认值：

```sh
go run ./cmd/checker-candidate --bundle "$PRIVATE_BUNDLE" \
  --source-id library-checker:aplusb --source-lock config/problem-sources/library-checker.json \
  --expected-plan "$TRUSTED_FORMAL_PLAN_SHA256" --checker lc-wcmp-tokens-v1 \
  --bytes "$CANDIDATE_BYTES" --token-bytes "$CANDIDATE_TOKEN_BYTES"
```

成功时输出候选配置和摘要。命令只检查冻结答案，既不运行题源程序，也不采纳生产限额；见[预检契约](docs/checker-admission.md#生产检查器候选预检)。

### 私有草稿参考验证

[#151 新题包](docs/delivery-log.md#2026-11-11新版题包十题参考复验与卷积报告接纳)的十题254点均通过固定环境维护参考验证并确认停止。最初批次因素性测试宿主超时中止，受控恢复后以新报告完成余下六题；[原失败](docs/delivery-log.md#2026-11-11新版题包维护参考复验中止)仍保留，根因未知。维护入口不写数据库验证状态或发布。需要已配置且停止的专用槽、已有固定镜像与 Docker Desktop、macOS arm64；命令不会自动创建、替换或恢复槽。使用已有 Go 1.26.11 工具链镜像构建独立宿主产物（不与 Linux 检查共用输出）：

`associative_array`使用512MiB完整材料总预算、input48MiB、stdout/checker8MiB、payload1GiB，独立CPU5秒/wall10秒，保留更严的整体3分钟和原medium物理槽/锁。同次48MiB输入→8MiB输出、完整20AC/Stop及随后独立原六来源回归已通过；[参考合同](docs/reference-validation.md#associative_array参考验证)与旧失败限制分开记录。

point_add_range_sum新版本已吸收主线停止预算修复，并通过一次八来源174点及全部原子组的固定环境接纳；旧suffixarray失败报告保留，根因仍未知，不据新通过追认旧失败或证明长期稳定。最终CI与合并状态见[PR #114](https://github.com/DL-Association-for-Computing-Machinery/OnlineJudge/pull/114)，见[八来源新版本接纳记录](docs/delivery-log.md#2026-11-04point_add_range_sum-八来源固定环境接纳)。

staticrmq新版本已吸收主线停止修复及已合并#114，通过一次九来源201点/24个原子子测试的固定环境接纳；历史第9点失败与根因未知结论保持，不追认旧失败或证明长期稳定。最终CI与合并状态见[PR #119](https://github.com/DL-Association-for-Computing-Machinery/OnlineJudge/pull/119)，证据见[九来源接纳记录](docs/delivery-log.md#2026-11-04staticrmq-九来源固定环境接纳)。

```sh
REFERENCE_TOOLCHAIN_IMAGE=YOUR_EXISTING_LOCAL_GO_TOOLCHAIN_IMAGE
docker --context desktop-linux run --rm --network none --cap-drop ALL \
  --security-opt no-new-privileges:true \
  --mount "type=bind,src=$PWD,dst=/src" --workdir /src \
  --entrypoint /bin/sh "$REFERENCE_TOOLCHAIN_IMAGE" -ec '
    export GOPROXY=off GOSUMDB=off CGO_ENABLED=0 GOOS=darwin GOARCH=arm64
    mkdir -p .local/validation-host
    go build -trimpath -o .local/validation-host/worker ./cmd/worker
    go test -c -tags=sandbox -o .local/validation-host/worker-validation.test ./cmd/worker
    go test -c -o .local/validation-host/workerhost-stop.test ./internal/workerhost
    go test -c -o .local/validation-host/workerhost-control.test ./internal/workerhost
  '
OJ_SANDBOX_EXPERIMENT=1 OJ_WORKER_M0_SLOT=secondary \
  .local/validation-host/worker validate-reference \
  --bundle /private/tmp/oj-library-checker-drafts/bundle \
  --source-lock "$PWD/config/problem-sources/library-checker.json" \
  --output /private/tmp/oj-reference-report \
  --start-stopped-slot
```

路径替换为已有完整私有bundle和新的报告目录。`--start-stopped-slot`在持锁/同步dirty后仅启动预存在、固定owner/配置匹配的停止槽；省略则要求槽已由维护者启动。启动回执未知时仍确认原生命周期停止；不能确认停止或清标记失败才保持quarantine，即使随后安全解封，本次报告仍失败。禁止自动重试、换槽掩盖失败或手工清标记。成功也会停止该槽。`report.json`私有保存逐点结果、完整plan/profile及容器生命周期，`publication`始终blocked；失败输出与支持域见[受限参考验证](docs/reference-validation.md)。

停止失败报告的可选`stop_diagnostic`提供固定阶段/错误类别、有界计数和最多六步的耗时记录，不含PID、命令参数或原始错误。`elapsed_ms`超过60000时截断并标记`elapsed_capped`，不是停止时限；原共享10秒Stop预算保持。kill步骤记录调用前context、调用尝试及本地CLI启动证据，`local_process_start`不证明Docker daemon收到kill或容器收到信号，`unknown`不表示未启动。`pid_present`仅表示探测时路径存在，可能发生PID复用；`deadline`不证明进程仍在或已退出。诊断不授予清理隔离标记的权限，旧失败报告不追认通过；字段与证据限制见[受限参考验证](docs/reference-validation.md)。

执行适配器失败时，可选`execution_diagnostic`按观察顺序保存最多八条固定operation/point/phase/role/code，保留主错误、清理错误和独立取消原因，超出标记`truncated`。角色来自本次缓存账本，不含材料、ID或原始错误。它可与停止诊断并存，不改变SE/隔离结论或追认旧失败；成功报告不增加字段。具体范围见[执行失败诊断](docs/reference-validation.md#执行失败诊断)。

停止诊断的受信命令单测随普通Docker backend运行；在Darwin核对本地执行器时，使用上面单独构建的workerhost测试包，先确认列表恰含下列三项，再选择执行。`cmd/worker`驱动不包含依赖包的单测，空匹配的PASS不能算验证。此处仅运行固定受信shell及测试替身，不启动沙箱槽或执行题源：

```sh
.local/validation-host/workerhost-stop.test \
  -test.list='^(TestStopTraceDistinguishesExpiredKillAttempt|TestLocalCommandStartEvidence|TestStopElapsedIsBoundedAndMarked)$'
.local/validation-host/workerhost-stop.test -test.v -test.count=1 \
  -test.run='^(TestStopTraceDistinguishesExpiredKillAttempt|TestLocalCommandStartEvidence|TestStopElapsedIsBoundedAndMarked)$' -test.timeout=1m
```

Host控制失败时，执行诊断条目可附带有界`host_trace`，区分入场、准备、inspect、command及响应校验；本地CLI已启动不代表daemon收件。字段范围、取整及缺失证据限制见[执行失败诊断](docs/reference-validation.md#执行失败诊断)。以下原生专项仅运行固定受信命令与替身；先核对清单恰含八项再执行，不能以空匹配PASS计为验证：

```sh
.local/validation-host/workerhost-control.test \
  -test.list='^(TestControlTrace.*|TestHostTraceCrossesExecutionTransportWithoutOldCause|TestSuccessfulHostThenDecodeFailureHasNoHostTrace)$'
.local/validation-host/workerhost-control.test -test.v -test.count=1 \
  -test.run='^(TestControlTrace.*|TestHostTraceCrossesExecutionTransportWithoutOldCause|TestSuccessfulHostThenDecodeFailureHasNoHostTrace)$' -test.timeout=1m
```

维护入口使用独立缓存文件通路：源码/输入上传后回读核对SHA-256，输出原始字节拉取，参考答案留在宿主。默认A+B保留2MiB材料/4KiB输出，unionfind使用64MiB匿名磁盘快照与256KiB运行stdout。zalgorithm使用64MiB快照，suffixarray独立准入128MiB快照；两者共用4MiB stdout及256MiB累计传输档位。primality_test独立冻结32MiB以内材料及固定4个头文件，stdout512KiB、6个缓存对象。point_add_range_sum使用input16MiB/完整快照256MiB、stdout4MiB及双端payload512MiB，Runtime保持Medium。staticrmq使用原256MiB完整材料、input16MiB、stdout4MiB与payload512MiB，原失败记录保持。static_range_sum使用256MiB完整快照、input16MiB、raw stdout/checker8MiB、payload512MiB。associative_array预算见上段。zalgorithm、suffixarray、point_add_range_sum、primality_test、staticrmq、static_range_sum、associative_array共用原medium槽锁；convolution_mod使用独立16MiB输出槽与独立锁，snapshot512MiB、input/answer及stdout/checker各16MiB、payload2GiB，所有磁盘来源共用进程内一份快照额度，两端账本均按档位计量。编译双流/运行stderr仍各4KiB；associative_array与convolution_mod独立使用CPU5秒/wall10秒，其余磁盘来源CPU1秒/wall5秒保持，memory128MiB不变。生产配置不变。

unionfind使用独立固定`oj-reference-sandbox`及`slot-reference.lock`，拒非空`OJ_WORKER_M0_SLOT`。维护者确认容器不存在后，可用已有固定镜像显式创建停止槽；已有槽不能替换，须核验owner/配置和停止状态：

```sh
docker --context desktop-linux create --pull never --name oj-reference-sandbox \
  --label owner=onlinejudge.desktop-test --privileged --network none --read-only \
  --tmpfs /tmp:rw,nosuid,size=128m --shm-size=128m --memory=1g --cpus=1 \
  --pids-limit=128 --cgroupns private \
  sha256:cd40ffdb32619806a8d97e59fe03e890fe4066835f57efa6ae13f919ef3262c5
env -u OJ_WORKER_M0_SLOT OJ_SANDBOX_EXPERIMENT=1 \
  .local/validation-host/worker validate-reference \
  --source-id library-checker:unionfind \
  --bundle /private/tmp/oj-library-checker-drafts/bundle \
  --source-lock "$PWD/config/problem-sources/library-checker.json" \
  --output /private/tmp/oj-unionfind-reference-report --start-stopped-slot
```

全部材料完整哈希、所有答案检查域先验后才调用execution Verify/Compile；槽启动与Host环境探测更早发生。快照并发/关闭、失败报告和独立槽`recover-reference <original-full-container-id>`的前置要求见[受限参考验证](docs/reference-validation.md)。

zalgorithm、suffixarray、point_add_range_sum、primality_test、staticrmq、static_range_sum与associative_array选择同一固定medium槽及`slot-reference-medium.lock`，串行使用，旧槽不改配置或替换。确认该容器不存在后，用同一已有镜像覆盖entrypoint并传精确固定Cmd，单文件output-limit8MiB、默认copy-out仍1MiB；命令自身不创建槽：

```sh
docker --context desktop-linux create --pull never --name oj-reference-medium-sandbox \
  --label owner=onlinejudge.desktop-test --privileged --network none --read-only \
  --tmpfs /tmp:rw,nosuid,size=128m --shm-size=128m --memory=1g --cpus=1 \
  --pids-limit=128 --cgroupns private --entrypoint /opt/go-judge \
  sha256:cd40ffdb32619806a8d97e59fe03e890fe4066835f57efa6ae13f919ef3262c5 \
  -http-addr 127.0.0.1:5050 -parallelism 1 -no-fallback -file-timeout 1m \
  -tmp-fs-param size=64m,nr_inodes=4k -output-limit 8388608 -copy-out-limit 1048576
env -u OJ_WORKER_M0_SLOT OJ_SANDBOX_EXPERIMENT=1 \
  .local/validation-host/worker validate-reference \
  --source-id library-checker:zalgorithm \
  --bundle /private/tmp/oj-library-checker-drafts/bundle \
  --source-lock "$PWD/config/problem-sources/library-checker.json" \
  --output /private/tmp/oj-zalgorithm-reference-report --start-stopped-slot
```

suffixarray使用同一命令，将`--source-id`改为`library-checker:suffixarray`并使用新的报告目录；不创建另一槽。其独立profile为`p4-lc-suffixarray-reference-desktop-arm64-v1`，仅完整材料总量提高至128MiB，单点input1MiB/answer4MiB、最多64点保持。medium异常恢复显式指定`worker recover-reference library-checker:zalgorithm <original-full-container-id>`或对应suffixarray/primality_test/staticrmq/static_range_sum/associative_array来源；省略source仍指原unionfind槽。均需实验opt-in、清空M0槽变量并先确认旧宿主在途操作排空；不能手工清marker。

point_add_range_sum同样只改`--source-id library-checker:point_add_range_sum`及新报告路径，不创建槽。独立profile为`p4-lc-point-add-range-sum-reference-desktop-arm64-v1`，使用封闭`ReferenceMediumBulk`档位；Runtime与Medium相同，仅文件协议ID和累计payload512MiB不同。恢复时指定该来源仍核验同一槽/锁；不能把档位归一化而丢弃Host端预算。

真实专项另覆盖固定12点CLI、合成正确/错误/编译错误/RE/TLE/MLE/OLE和取消后的停止确认。它不需要PG/Redis或控制端，不进入普通CI：

```sh
OJ_SANDBOX_EXPERIMENT=1 OJ_WORKER_M0_SLOT=secondary OJ_REFERENCE_VALIDATION=1 \
  OJ_REFERENCE_BUNDLE=/private/tmp/oj-library-checker-drafts/bundle \
  OJ_REFERENCE_OUTPUT=/private/tmp/oj-reference-suite-report \
  .local/validation-host/worker-validation.test \
  -test.v -test.count=1 -test.run='^TestPrivateDraftReferenceSandbox$' -test.timeout=5m
```

独立unionfind真实专项（两组：完整18点CLI与64KiB/256KiB/上限+1输出）使用新报告目录：

```sh
env -u OJ_WORKER_M0_SLOT OJ_SANDBOX_EXPERIMENT=1 OJ_UNIONFIND_VALIDATION=1 \
  OJ_REFERENCE_BUNDLE=/private/tmp/oj-library-checker-drafts/bundle \
  OJ_UNIONFIND_OUTPUT=/private/tmp/oj-unionfind-suite-report \
  .local/validation-host/worker-validation.test \
  -test.v -test.count=1 -test.run='^TestUnionFindReferenceSandbox$' -test.timeout=5m
```

medium真实专项覆盖完整29点CLI、4MiB全字节输出/cap+1 collector OLE及8MiB隔离内文件/cap+1 SIGXFSZ；资源样本区分CLI OS maxrss、Go heap阶段值和沙箱cgroup峰值，不进入普通CI：

```sh
env -u OJ_WORKER_M0_SLOT OJ_SANDBOX_EXPERIMENT=1 OJ_MEDIUM_REFERENCE=1 \
  OJ_REFERENCE_BUNDLE=/private/tmp/oj-library-checker-drafts/bundle \
  OJ_MEDIUM_OUTPUT=/private/tmp/oj-zalgorithm-suite-report \
  .local/validation-host/worker-validation.test \
  -test.v -test.count=1 -test.run='^TestMediumReferenceSandbox$' -test.timeout=6m
```

suffixarray完整50点专项复用medium槽，要求单独opt-in和新报告目录；完整集合冻结、所有答案先验、逐点AC、停止及槽标记清理均需成功：

```sh
env -u OJ_WORKER_M0_SLOT OJ_SANDBOX_EXPERIMENT=1 OJ_SUFFIXARRAY_REFERENCE=1 \
  OJ_REFERENCE_BUNDLE=/private/tmp/oj-library-checker-drafts/bundle \
  OJ_SUFFIXARRAY_OUTPUT=/private/tmp/oj-suffixarray-suite-report \
  .local/validation-host/worker-validation.test \
  -test.v -test.count=1 -test.run='^TestSuffixArrayReferenceSandbox$' -test.timeout=5m
```

point_add_range_sum专项覆盖完整21点CLI和8MiB+1/16MiB全字节缓存stdin，核验上传SHA-256回读、程序读取count/sum、清理及阶段资源样本；不将count/sum单独作为顺序回显证明：

```sh
env -u OJ_WORKER_M0_SLOT OJ_SANDBOX_EXPERIMENT=1 OJ_POINT_ADD_REFERENCE=1 \
  OJ_REFERENCE_BUNDLE=/private/tmp/oj-library-checker-drafts/bundle \
  OJ_POINT_ADD_OUTPUT=/private/tmp/oj-point-add-suite-report \
  .local/validation-host/worker-validation.test \
  -test.v -test.count=1 -test.run='^TestPointAddReferenceSandbox$' -test.timeout=5m
```

primality_test使用同一medium槽及锁、独立依赖档位，固定5份编译材料与完整13点。普通维护命令指定`--source-id library-checker:primality_test`及新报告目录；异常恢复显式指定该来源及原完整容器ID。专项先验证可信嵌套include布局，首组失败即停止，再执行真实完整CLI：

```sh
env -u OJ_WORKER_M0_SLOT OJ_SANDBOX_EXPERIMENT=1 OJ_PRIMALITY_REFERENCE=1 \
  OJ_REFERENCE_BUNDLE=/private/tmp/oj-library-checker-drafts/bundle \
  OJ_PRIMALITY_OUTPUT=/private/tmp/oj-primality-suite-report \
  .local/validation-host/worker-validation.test \
  -test.v -test.count=1 -test.run='^TestPrimalityReferenceSandbox$' -test.timeout=5m
```

staticrmq也复用同一槽；维护命令指定`--source-id library-checker:staticrmq`及新报告目录。以下为显式专项入口，原失败及早期单次未复现观测保留，新版本已通过九来源固定环境接纳；后续运行仍需独立明确目标和新目录。先验证可信16MiB缓存stdin全字节及长度，再运行完整27点CLI；前组或停止失败即中止，不扩大原执行/文件/Stop期限：

```sh
env -u OJ_WORKER_M0_SLOT OJ_SANDBOX_EXPERIMENT=1 OJ_STATICRMQ_REFERENCE=1 \
  OJ_REFERENCE_BUNDLE=/private/tmp/oj-library-checker-drafts/bundle \
  OJ_STATICRMQ_OUTPUT=/private/tmp/oj-staticrmq-suite-report \
  .local/validation-host/worker-validation.test \
  -test.v -test.count=1 -test.run='^TestStaticRMQReferenceSandbox$' -test.timeout=5m
```

convolution_mod已通过独立16MiB输出槽的同次输入/输出边界与完整53点验证，预算与限制见[维护合同](docs/reference-validation.md#convolution_mod参考验证)，十来源254点/26子测试接纳及最终CI/合并状态见[交付记录](docs/delivery-log.md#2026-11-04convolution_mod-十来源固定环境接纳)。原第四槽已创建一次并完成本次Stop；以下创建命令仅在确认目标不存在时使用，已有槽不得替换：

```sh
docker --context desktop-linux create --pull never --name oj-reference-convolution-sandbox \
  --label owner=onlinejudge.desktop-test --privileged --network none --read-only \
  --tmpfs /tmp:rw,nosuid,size=128m --shm-size=128m --memory=1g --cpus=1 \
  --pids-limit=128 --cgroupns private --entrypoint /opt/go-judge \
  sha256:cd40ffdb32619806a8d97e59fe03e890fe4066835f57efa6ae13f919ef3262c5 \
  -http-addr 127.0.0.1:5050 -parallelism 1 -no-fallback -file-timeout 1m \
  -tmp-fs-param size=64m,nr_inodes=4k -output-limit 16777216 -copy-out-limit 1048576
env -u OJ_WORKER_M0_SLOT OJ_SANDBOX_EXPERIMENT=1 OJ_CONVOLUTION_REFERENCE=1 \
  OJ_REFERENCE_BUNDLE=/private/tmp/oj-library-checker-drafts/bundle \
  OJ_CONVOLUTION_OUTPUT=/private/tmp/oj-convolution-suite-report \
  .local/validation-host/worker-validation.test \
  -test.v -test.count=1 -test.failfast -test.run='^TestConvolutionReferenceSandbox$' -test.timeout=5m
```

维护命令显式指定`--source-id library-checker:convolution_mod`；异常恢复指定同一来源及原full ID，遵循原在途控制排空、Stop/PID证据和dirty门禁。专项首组或Stop失败时不进入完整53点CLI，不重试到通过。

小规模独立对拍复用同一停止槽。令 `OJ_CONVOLUTION_SOURCE_ROOT` 指向本地已核验的固定上游源码根（含 `convolution/convolution_mod/sol/{naive,correct}.cpp`），报告目录必须是源码根外尚不存在的路径：

```sh
env -u OJ_WORKER_M0_SLOT -u OJ_WORKER_M0_DUAL_EXPERIMENT OJ_SANDBOX_EXPERIMENT=1 \
  .local/validation-host/worker validate-convolution-differential \
  --source-root "$OJ_CONVOLUTION_SOURCE_ROOT" \
  --source-lock "$PWD/config/problem-sources/library-checker.json" \
  --output /private/tmp/oj-convolution-differential-report --start-stopped-slot
```

使用上方的 Darwin arm64 宿主产物。固定错误解负对照只在专用 `worker-validation.test` 中运行：显式设置 `OJ_CONVOLUTION_DIFFERENTIAL_TEST=1`、`OJ_SANDBOX_EXPERIMENT=1`、`OJ_CONVOLUTION_DIFFERENTIAL_SOURCE_ROOT`、`OJ_CONVOLUTION_DIFFERENTIAL_SOURCE_LOCK` 和新目录 `OJ_CONVOLUTION_DIFFERENTIAL_TEST_OUTPUT`，清空上述两个槽覆盖，选择 `-test.run='^TestConvolutionDifferentialNegativeSandbox$' -test.timeout=5m`。负对照应在报告中留下合法系数差异并确认停机。私有报告和限制见[卷积小规模独立对拍](docs/reference-validation.md#卷积小规模独立对拍)。

static_range_sum的维护命令选择`--source-id library-checker:static_range_sum`和新报告目录。以下显式专项先验证完整16MiB输入与8MiB stdout/4KiB stderr、stdout超限的窄SIGXFSZ证据及stderr4097拒绝，再运行完整11点CLI。预期负例只接受一条明确验证错误；任何额外清理/取消/截断错误或Stop失败均中止后组。首次启动失败与后续环境改变后的新通过分开记录，不沿用已消耗的协议重试；固定文件限制/collector区分见[参考合同](docs/reference-validation.md#static_range_sum参考验证)：

```sh
env -u OJ_WORKER_M0_SLOT OJ_SANDBOX_EXPERIMENT=1 OJ_STATIC_RANGE_SUM_REFERENCE=1 \
  OJ_REFERENCE_BUNDLE=/private/tmp/oj-library-checker-drafts/bundle \
  OJ_STATIC_RANGE_SUM_OUTPUT=/private/tmp/oj-static-range-sum-suite-report \
  .local/validation-host/worker-validation.test \
  -test.v -test.count=1 -test.run='^TestStaticRangeSumReferenceSandbox$' -test.timeout=5m
```

associative_array命令选择`--source-id library-checker:associative_array`及新报告目录，不创建另一物理槽。专用驱动先验证同次48MiB完整输入与8MiB完整输出；任一清理/Stop失败则不进入20点CLI。更严整体期限和来源预算先固定，不能失败后自动追加：

```sh
env -u OJ_WORKER_M0_SLOT OJ_SANDBOX_EXPERIMENT=1 OJ_ASSOCIATIVE_ARRAY_REFERENCE=1 \
  OJ_REFERENCE_BUNDLE=/private/tmp/oj-library-checker-drafts/bundle \
  OJ_ASSOCIATIVE_ARRAY_OUTPUT=/private/tmp/oj-associative-array-suite-report \
  .local/validation-host/worker-validation.test \
  -test.v -test.count=1 -test.failfast -test.run='^TestAssociativeArrayReferenceSandbox$' -test.timeout=5m
```

原始字节与容量专项另外使用已停止的M0 secondary槽，覆盖48MiB边界/超限、空输入、全部字节值、缓存stdin→小输出、清理及数值资源采样；它不更新草稿状态或生产限额：

```sh
OJ_SANDBOX_EXPERIMENT=1 OJ_WORKER_M0_SLOT=secondary OJ_REFERENCE_BYTES=1 \
  .local/validation-host/worker-validation.test \
  -test.v -test.count=1 -test.run='^TestReferenceBytesSandbox$' -test.timeout=5m
```

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

输入须为稳定私有快照；输出为仓库外、与输入不重叠的新目录。工具重新核对源码和全部数据哈希，生成自含的10题/254点/18样例草稿、英文Markdown题面、来源映射与许可对象。所有路径相对bundle，移到其他目录后可独立复验，不依赖原候选目录。失败保留 `INCOMPLETE` 和部分文件，重试用新目录；临时目录不是持久备份。格式、权限及发布阻断见[草稿合同](docs/problem-drafts.md)。已增加私有草稿入库服务；审核和生产判题配置仍未接通；已有按ID本人题面预览。

从已审查的 v6 题包派生 [Z 算法](docs/problem-sources.md#z-算法规模边界修订)或 [A+B](docs/problem-sources.md#ab-值域边界修订)边界修订时，另建私有单题包，保留原包供旧修订使用：

```sh
python3 -B scripts/supplement-zalgorithm.py \
  --base-bundle /private/tmp/oj-library-checker-drafts/bundle \
  --output /private/tmp/oj-zalgorithm-scale
python3 -B scripts/import-library-checker.py \
  --verify-bundle /private/tmp/oj-zalgorithm-scale/bundle
python3 -B scripts/supplement-aplusb.py \
  --base-bundle /private/tmp/oj-library-checker-drafts/bundle \
  --output /private/tmp/oj-aplusb-boundaries
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

判定口径、版本及已观测缺口见[输入覆盖盘点](docs/problem-sources.md#输入覆盖盘点)。

### 出题角色维护与草稿入库服务

出题角色始于schema 7，当前 schema 19仍默认不给任何账号授予角色。受信维护者先独立迁移，再用 Docker 工具链构建的命令明确授予/撤销；`OJ_DATABASE_URL` 通过既有私有环境注入，不把真实连接信息写入命令或日志：

```sh
.local/bin/migrate up
.local/bin/problem-author inspect --subject SUBJECT_ID
.local/bin/problem-author set --subject SUBJECT_ID --expected-revision 0 \
  --enabled true --operator MAINTENANCE_OPERATOR --reason AUDIT_REASON
```

替换大写占位值；主体为已存在的32位小写十六进制ID。初次修订为0；之后必须使用inspect读出的实际修订。撤销用 `--enabled false`，不自动重试冲突或未知结果。`operator`仅为受信维护操作的审计归属，不是已验证的账号登录。单独镜像可通过 `docker --context desktop-linux build --target problem-author -t oj-problem-author:local .` 构建，入口为 `/usr/local/bin/oj-problem-author`，默认UID65532；数据库连接由维护流程单独配置，默认Compose不运行该工具。

`internal/problem.Service` 的创建/本人读取必须传真实普通会话凭据；授权后把选定草稿的全部材料写入私有Store，再读会话，并在同一PG事务核对恢复/账号/角色及写元数据。`LoadBundle`的expectedSourceLock来自受信配置原字节；材料根与bundle均由维护者预建。可选HTTP接入见下节；管理页面、审核和发布仍待完成，完整边界、幂等、迁移与实测归[草稿合同](docs/problem-drafts.md#私有草稿入库与本人读取)。

`internal/problemreference.Service` 提供[持久化参考验证申请](docs/reference-requests.md)：绑定实际会话及本人草稿，保存受信来源、完整导入修订、Profile 原字节和原授权快照，并支持原键回执与取消。参考申请默认没有准入 policy，须受信维护显式配置后才接受新申请。作者显式准备后，服务冻结对象并重验授权；固定卷积的新申请同时保存[独立对拍计划](docs/reference-requests.md#卷积独立对拍的预期计划)，供专用参考 Worker 在正式验证后执行。作者可回读本人终态摘要；草稿保持 `not-run`，发布仍阻断。精确升级与回退条件见[迁移与回退](docs/reference-requests.md#迁移与回退)。

参考领取为显式内部操作：控制端仅在设置 `OJ_WORKER_REFERENCE_NODE_ID` 后启用。节点须已由受信维护配置，持有 `spiffe://onlinejudge/reference-worker/{node_id}/{credential_version}` 客户端证书。沿用 Worker 的控制端地址、CA、证书和私钥配置，清空双槽和 M0 槽覆盖变量，以原回执中的申请 ID、固定来源及预期 Plan 摘要运行：

```sh
env -u OJ_WORKER_M0_SLOT -u OJ_WORKER_M0_DUAL_EXPERIMENT OJ_SANDBOX_EXPERIMENT=1 \
  .local/bin/worker claim-reference REQUEST_ID SOURCE_ID EXPECTED_PLAN_SHA256
```

命令获取同机槽后只发送一次领取，再在同一锁内下载并校验冻结计划的全部材料，关闭匿名快照后退出；数据库预留仍按原时钟在 30 秒后失效。控制端还须配置 `OJ_WORKER_REFERENCE_MATERIAL_ROOT`，使用草稿入库的已存在私有对象根（Linux、当前 UID、0700；上限 128 MiB/对象，同时一个快照），与 M0 的 `OJ_MATERIAL_ROOT` 含义不同。无题源执行、续租或回报；未知响应和下载失败不自动重试，低带宽下载大题可能超时。见[参考材料合同](docs/reference-requests.md#有效预留内的材料下发)。

同机受信执行另需控制端 `OJ_WORKER_REFERENCE_EXECUTION=1`，以及已预配且停止的固定隔离槽。使用同一参数运行 `.local/bin/worker run-reference REQUEST_ID SOURCE_ID EXPECTED_PLAN_SHA256`（同样要求 `OJ_SANDBOX_EXPERIMENT=1` 且清空双槽/M0 槽覆盖）。命令在原锁内续期、下载、执行并确认停止；不创建、恢复或替换容器。停止确认及清理后提交确定结果，控制端接纳不推进发布。停止无法确认时保留隔离，见[持槽执行合同](docs/reference-requests.md#同机持槽参考执行)。

另外显式设置控制端 `OJ_WORKER_REFERENCE_DISPATCH=1` 后，可使用 `.local/bin/worker run-next-reference SOURCE_ID [AFTER_ID]` 按来源选择并执行一个此前未领取的已准备申请。槽忙不发请求；空页可用输出的 `next_after` 显式继续。未知领取不自动重试，旧计划不会自动进入选择，见[选择合同](docs/reference-requests.md#按来源选择已准备申请)。

对有独立对拍计划的卷积申请，上述两个执行入口会在正式点通过后继续用同一槽对拍，确认停止后一次提交两份证据。合法首差异会保留为失败证据，命令返回失败。实物专项使用上方 Darwin arm64 构建的 `worker-validation.test`，设置 `OJ_CONVOLUTION_LEASED_SANDBOX_TEST=1`、`OJ_SANDBOX_EXPERIMENT=1`、`OJ_CONVOLUTION_BUNDLE`、`OJ_CONVOLUTION_SOURCE_ROOT` 和 `OJ_CONVOLUTION_SOURCE_LOCK`，清空两个槽覆盖并运行 `-test.run='^TestClaimedConvolutionOwnedSandbox$' -test.count=1 -test.timeout=4m`。确认原槽已停止后，可另设 `OJ_CONVOLUTION_LEASE_LOSS=1` 做续期失效对照。真实 mTLS／PostgreSQL 报告接纳与宿主执行证据分别记录。

### 十题真实报告接纳

在 macOS arm64 用既有 Docker Desktop 工具链和专用槽显式运行：

```sh
OJ_SANDBOX_EXPERIMENT=1 python3 -B tests/integration/live_reference.py \
  --start-stopped-slots --bundle /private/tmp/oj-library-checker-drafts/bundle \
  --toolchain-image YOUR_EXISTING_LOCAL_GO_TOOLCHAIN_IMAGE \
  --output /private/tmp/oj-live-reference-reports
```

题包父目录须归当前用户所有、权限为 0700，且只含题包；挂载整个父目录可保留 Linux 读到的题包所有者。工具链须已缓存依赖并提供 PostgreSQL 工具，以非 root 用户运行。输出须是已有父目录下的新目录，位于仓库与题包父目录之外。五个预配槽须已停止且无隔离标记；命令不创建、替换或恢复槽。临时控制端只向宿主回环发布，数据库使用容器内 Unix socket。执行仍在原断网槽，首个失败即停止批次，原始日志保留；成功才写 `summary.json`。见[报告接纳专项](docs/reference-requests.md#十题实物报告接纳专项)。

验证单题修订时，在同一命令追加 `--zalgorithm-local-data`（31 点）或 `--aplusb-local-data`（17 点），将 `--bundle` 指向对应的单题包，并为 `--output` 另选新目录。专项要求冻结和接纳同一份计划。A+B 还核对指定非作者读取覆盖、输入校验、独立小用例与历史渲染摘要，作者和未分配者被拒。夹具确认只解除题面缺项，撤回分配后重分配及退回会恢复该缺项；退回还恢复覆盖缺项。原 v6 的 A+B 保留原质量摘要且无覆盖摘要。

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

仅Linux；`material_root`必须是按[存储合同](docs/material-store.md)预建持久根，bundle为已准备的稳定私有快照，两者均由运行euid所有、目录0700/文件0600。来源锁从仓库受信的[固定来源锁](config/problem-sources/library-checker.json)单独配置，不得从请求或bundle自行选作信任根。三个路径须绝对规范，材料根与bundle不得互相包含，锁须在两者之外；部署者也须保证实际挂载/祖先没有把它们别名到同一位置。bundle/锁只读挂载，材料根可写，镜像默认UID65532须有对应访问权。服务不自动建目录、迁移、授予角色或修复文件。

`source_ids`为1—10个不重复候选，启动逐项加载元数据；大对象仍在授权后的导入中验证。`timeout`为正且至多1分钟，`server.write_timeout`不得小于它；`max_in_flight`为1—10，由本实例已授权的导入/预览材料阶段共享，无等待拒绝，不限制初次预检、普通元数据GET或HTTP输出，不是共享频率或磁盘总额。每对象128MiB、固定bundle上限及同题单份Prepared活动约束继续适用。缺省整个对象关闭能力，关闭后重启保留草稿和材料，无迁移；旧二进制回退前也先移除该对象。

已通过真实普通登录的出题账号可向`POST /api/v1/problem-drafts`发送`{"source_id":"library-checker:aplusb","request_key":"original-import-request-01"}`，需要同源证据及会话Cookie；最多1KiB JSON。成功/原回执均200，再以`GET /api/v1/problem-drafts/草稿ID`读取本人固定JSON元数据。断连/503可能已经提交，保留原键及精确候选快照核验，不自动重试或换键；配置/候选版本切换前须保留原版本供未知回执恢复。完整协议见[OpenAPI](api/problem-drafts.openapi.yaml)。默认Compose及本地测试账号均不自动开启或取得出题权。

`/author/drafts`已接来源选择、绑定导入及本机回执恢复，使用[主体绑定管理协议](api/problem-draft-management.openapi.yaml)：先`GET /api/v1/problem-draft-management/sources`取得当前授权候选的`source_id/title/version/import_revision`，再向`POST /api/v1/problem-draft-management/imports`提交严格三字段`source_id/request_key/expected_revision`，其中expected_revision原样取完整import_revision。所有管理路由都必须带当前检查的`X-OJ-Expected-Subject`；服务端只把它与实际会话比较，不作为身份来源，账号变化409且业务零访问。完整修订还绑定index/来源证据，不能用仅代表题目文档的version代替；修订变化在材料I/O前409拒绝。成功返回八字段最小回执，没有文档或材料引用。

若响应丢失或结果未知，保留原owner/source/key/revision，以`GET /api/v1/problem-draft-management/imports/原请求键`只读查询本人已提交回执，不必重传材料。该GET不依赖当前候选或材料活动槽，但整个private_drafts仍须有效配置并启动成功。404只是本次没有观察到提交，409也不证明旧请求不存在；保留原tuple，显式再次查询或用原tuple恢复，不自动POST、不换键/修订。页面先等本机IndexedDB严格事务提交才发起POST，最多32条/账号，不自动清除pending；刷新/恢复/账号变化只清显示。旧两字段及[先前绑定协议](api/problem-draft-imports.openapi.yaml)保持兼容，页面绝不回退它们，旧二进制404须停写。`/author/drafts/open`保留按已有ID选择入口；显式图片预览已接入，审核/发布待交付。本机恢复记录的保留、导出与存储丢失限制见[界面约定](docs/frontend-design.md#私有草稿导入管理)，细节和回退见[草稿合同](docs/problem-drafts.md#管理页导入协议)。

服务器本人列表为`GET /api/v1/problem-draft-management/drafts`，带同一预期主体header。首次无query，之后原样传单个`cursor=next_cursor`；每页最多20条，空next_cursor表示本次末页，无总数/他人统计。只读最小回执，不读题面或材料；网页即使本机记录已清或IndexedDB不可用也能手动发现已提交草稿并打开预览，不自动确认或清理pending。逐页视图不是跨请求快照，较早时间戳迟提交可能被旧边界漏过，并发时重查首页；空/末页不是取消或无在途证明。schema8索引的维护升级/回退见[本人列表合同](docs/problem-drafts.md#本人草稿列表)。

要启用本人参考申请，在 `private_drafts` 中显式设 `"reference_requests": true`，并由受信维护者设置独立队列额度（示例值，不代表容量建议）：

```sh
.local/bin/migrate reference-backlog 4 2
```

命令使用既有私有 `OJ_DATABASE_URL`，只写参考队列额度，要求当前 schema，不自动迁移。随后可在本人草稿预览提交、查原键回执、显式准备材料计划和取消。收到报告后可点击“查看验证结果”，查看正式判定和可用的独立对拍摘要；见[作者申请合同](docs/reference-requests.md#作者申请入口)。API 不执行题源，不更新草稿验证状态。

作者送审的 HTTP 入口可在上述配置内另设 `"review_intake": true`，默认关闭。入口保存本人草稿的完整导入修订和选中的已接纳报告，提供原键回执。作者也可按原键[核对并确认冻结标程](docs/problem-drafts.md#作者确认标程)。页面尚未接入送审及确认，草稿状态不变。

审核证据读取独立设 `"review_access": true`。指定审核人可以按送审 ID 读取固定验证摘要、[对拍详情](docs/problem-drafts.md#审核对拍详情)、[批准阻断](docs/problem-drafts.md#批准准入状态)和[题面及标程](docs/problem-drafts.md#审核材料读取)。维护者用 Docker 工具链构建的命令管理资格和分配，数据库连接由私有环境注入：

```sh
.local/bin/problem-review reviewer-set --subject REVIEWER_ID --expected-revision 0 --enabled true --operator MAINTAINER --reason 'confirmed non-author reviewer'
.local/bin/problem-review assign --review REVIEW_ID --reviewer REVIEWER_ID --reviewer-revision 1 --expected-revision 0 --operator MAINTAINER --reason 'assign exact review item'
.local/bin/problem-review reviewer-inspect --subject REVIEWER_ID
.local/bin/problem-review assignment-inspect --review REVIEW_ID
.local/bin/problem-review withdraw --review REVIEW_ID --expected-revision 1 --operator MAINTAINER --reason 'withdraw assignment'
```

用 32 位小写十六进制主体及送审 ID 替换占位值。修订 0 仅指首次记录；撤销资格用当前修订与 `--enabled false`。改派也以最新分配修订为前置条件。当前无页面入口、批准或发布。

如需允许指定审核人写退回决定，再设置 `"review_returns": true`；只开 `review_access` 仍为只读。作者能按原送审键查看理由；改用新键不能再次提交相同修订与证据。请求及原键恢复见[审核退回与重送](docs/problem-drafts.md#审核退回与重送)。

如需记录当前指定非作者对冻结题面的人工核对，启用 `"statement_confirmations": true`（需要 `review_access`，默认关闭）。写入与原键回执见[审核人确认题面语义](docs/problem-drafts.md#审核人确认题面语义)。

`GET /api/v1/problem-drafts/草稿ID/preview`返回本人已入库版本的`markdown`、有序`examples`（id/input/output）和可选`images`，不依赖当前候选版本；仍须有效普通会话和当前出题权。先核对全部引用与总预算，再逐份验证快照，读取后复核同一凭据和PG授权，失败不返回部分结果。原始markdown/input/output/显式原图合计至多1MiB，重复对象按返回次数计数；图片完整解码后限长重编码为静态PNG，输出PNG与原文/样例另限1MiB。图片角色/格式/像素/路径及回退限制见[图片合同](docs/problem-drafts.md#显式题面图片)。JSON文本转义至多六倍，图片base64另加元数据。文本严格UTF-8，保留空格、CRLF及末尾换行，不替换或截断。它是管理页数据，不提供一般材料下载或发布；正式测试与程序源码不进入预览。最终会话读取之后的退出及最终PG授权之后的撤权沿已接受在途合同，不能原子撤回已经开始发送的响应。关闭整个`private_drafts`能力即可停止入口，已入库资料保留。

浏览器可从 `/author/drafts` 的本人列表选择已提交草稿，或在 `/author/drafts/open` 输入ID，再显式检查会话并打开本人预览。Markdown/GFM/数学公式使用共享组件和本地KaTeX字体，样例原文复制；清单图片精确匹配后使用静态PNG数据，其他图片占位且无源地址请求，审核和发布尚未开放。账号操作、页面恢复/隐藏或期限到达会清除题面和图片，再次显示须重新检查；无持久缓存。预算与隐私边界见[前端设计](docs/frontend-design.md#共享题面与本人预览)。默认HTTP Compose及未启用private_drafts的账号环境不提供草稿能力。

## 本地不可变材料存储

Linux维护工具使用预建持久根，根为运行euid所有且权限0700；维护者须先完成根/父目录持久化准备。默认镜像UID为65532，输入也须允许该用户读取。命令不初始化或修复目录，不提供用户授权、数据库草稿导入或发布；完整合同见[本地材料存储](docs/material-store.md)。

```sh
docker --context desktop-linux build --target materialstore -t oj-materialstore:local .
docker --context desktop-linux run --rm --network none --read-only --cap-drop ALL \
  --security-opt no-new-privileges:true \
  --mount type=bind,src=/ABS/PREPROVISIONED_PRIVATE_ROOT,dst=/store \
  --mount type=bind,src=/ABS/READABLE_INPUT_DIRECTORY,dst=/input,readonly \
  oj-materialstore:local put --root /store --source /input/OBJECT_FILE \
  --sha256 SHA256_VALUE --size BYTE_LENGTH --max-object-bytes 134217728 --timeout 2m
docker --context desktop-linux run --rm --network none --read-only --cap-drop ALL \
  --security-opt no-new-privileges:true \
  --mount type=bind,src=/ABS/PREPROVISIONED_PRIVATE_ROOT,dst=/store \
  oj-materialstore:local verify --root /store --sha256 SHA256_VALUE \
  --size BYTE_LENGTH --max-object-bytes 134217728 --timeout 2m
```

替换全部大写占位值。128MiB/2分钟仅为调用示例，不是生产阈值；维护镜像只在Linux执行，与默认API/Compose分开。成功只输出对象hash/size；写入失败可能已保存，按原对象重试Put确认后才提交引用。verify校验读取完整性，不确认失败写入的持久化。输出与错误均不包含材料正文。

## 运行环境

Docker 工具链使用 Go 1.26.11、pnpm 11.25.0、Node 24.19.0，与 `go.mod`、[Node 版本](web/.node-version)及 [package.json](web/package.json)对应；依赖锁文件为 `go.sum` 和 `web/pnpm-lock.yaml`。宿主无需另外安装这些构建工具。

构建工具链由 Dockerfile 固定，应用依赖以 go.sum 和 pnpm-lock.yaml 为准。Docker Desktop 迁移的验证结果见交付日志；历史 Colima 证据不能替代本次验证。

## 本地 HTTPS 账号环境（一键）

需要在页面上实际登录时，在仓库根目录运行：

```sh
python3 scripts/dev-up.py
```

打开 [本地 HTTPS 登录页](https://localhost:8443/login)，点击“一键进入测试账号”，或使用私有初始化凭据走普通表单登录。首次访问需确认本地证书，或自行信任脚本导出的公开 `.local/dev-auth/ca.crt`；脚本不更改系统信任。账号、密钥、PostgreSQL 和 Redis 数据持久保留，重复运行不重设密码。完整边界、停止方式与验证见[本地开发指南](docs/local-development.md)。此独立项目不替代生产部署，也不影响下面的 HTTP 骨架预览。

## Docker 构建与一键启动（默认）

默认使用 Docker 构建和 Compose 启动。macOS 使用 Docker Desktop 的 `desktop-linux` context；Linux CI 使用 Docker Engine 与 Compose v2+，省略下面命令中的 `--context desktop-linux`。首次构建需联网获取 Dockerfile 中固定摘要的基础镜像和锁定依赖。

在仓库根目录执行：

```sh
docker --context desktop-linux compose up -d --build --wait
```

打开 [Docker 本机预览](http://127.0.0.1:8080/) 查看示例题库；连接诊断位于 `/account` 的折叠区域。Nginx 提供 Vite 构建后的静态文件，将 `/api/` 转发到 Compose 内部的 `api:8080`；后端不发布宿主机端口。若 8080 被占用，执行 `OJ_WEB_PORT=18080 docker --context desktop-linux compose up -d --build --wait`，再访问 18080。后续源码修改需要重建，此入口不是热更新开发服务器。

```sh
docker --context desktop-linux compose build            # 构建默认应用镜像
docker --context desktop-linux compose ps               # 查看服务和健康状态
docker --context desktop-linux compose logs --tail=100   # 查看启动或请求错误
docker --context desktop-linux compose down             # 停止本项目容器、网络
```

当前只启动前端与基础 API，健康检查只证明进程/代理连通；不启动数据库、Worker、控制端或判题沙箱，不代表完整 OJ 可用。应用容器使用非 root、只读根文件系统；Nginx 临时文件只写入内存目录。当前默认服务没有持久化业务数据或 volume；后续增加数据库必须同步配置持久化与迁移步骤，不使用 `down -v` 清理数据。HTTPS、正式部署与离线制品交付尚未配置。

Dockerfile 的 Go、Node、pnpm 与现有配置保持一致，基础镜像固定多架构索引摘要；依赖仍使用 `go.sum` 和 `pnpm-lock.yaml`。`.dockerignore` 限定构建上下文，排除本地 Git、缓存、环境文件和常见凭据文件。更换工具链时需同步 Dockerfile 和对应锁定入口，并重新验证容器检查及启动流程。

## 显式启用普通会话 API

API 镜像的 `/usr/local/bin/oj-api` 在非空 `OJ_SESSION_CONFIG_FILE` 下切换为独立会话 HTTPS 服务；同时必须显式设置 `OJ_API_ADDR`（例如容器内 `:8443`），不能与 `OJ_EXPERIMENT_TOKEN_FILE` 同用。配置文件为绝对规范路径，完整字段及权限见[配置合同](docs/architecture/session-design.md#717-显式会话-api-入口)。未设置配置路径时仍运行现有 bootstrap/实验模式，不尝试连接会话数据库。

数据库及 Redis 的密码来源各自显式选择 `auth: "none"` 或 `"env"`。后者只读取固定的 `OJ_SESSION_DATABASE_PASSWORD` / `OJ_SESSION_REDIS_PASSWORD`，必须非空；前者拒绝对应非空残留环境。密码、登录 key、TLS 私钥均不写入配置 JSON、镜像或命令行。数据库不消费 `OJ_DATABASE_URL`，连接类 PG 环境变量须按运行时合同清理；服务不修改进程环境。

启动前须由受信维护流程完成当前 schema 19（邮箱模型始于5、积压准入始于6、私有草稿始于7、列表索引始于8、参考申请始于9、预期计划始于10、参考领取始于11、参考报告始于12、参考调度始于13、卷积对拍预期计划始于14、作者送审始于15、审核资格与分配始于16、审核退回始于17、作者确认标程始于18、审核人确认题面始于19）、恢复状态开门、共享密钥分发及匹配的限流 marker 初始化；命令只检查，不自动迁移、初始化或修复。schema切换须在维护窗口排空/停止旧API、Worker及维护写入后执行，再启动匹配新版；运行中旧进程不会因版本变化自动停止，不混跑。精确回退条件见[参考申请迁移](docs/reference-requests.md#迁移与回退)及[草稿历史迁移](docs/problem-drafts.md#入库版本与迁移)。本次没有迁移现有持久化localdev或部署服务器。配置与公开证书可只读挂载，登录密钥及 TLS 私钥必须满足 0400/0600、root/进程 euid 所有等约束。非 root 容器须能遍历并读取这些文件。不要把完整配置或秘密对象输出到日志。

direct 模式要求客户端直接 TLS；proxy 模式要求同一监听器的已验证客户端证书和允许的叶证书指纹，健康检查也须经 mTLS。现有 Compose/Nginx 的明文 backend 和浏览器页面尚未配置该入口；部署、生产阈值、真实证书/存储 TLS 和完整账号权限另行验收。回退先停服务并移除显式配置环境，不把旧明文路径接到会话 Handler。

可选的 `password_change` 对象另行启用 `POST /api/v1/account/password`，省略时保持 404。必须显式给定 `minimum_characters`、`blocklist_file` 及 `global`/`source`/`subject` 三个 `{count, window}` 额度；用户已确认至少 15 个 Unicode code point、允许空格、不强制字符组合、拒绝常见弱密码，保留原始字节且不 trim/归一化/截断。最低字符数可配置更高值，KDF 最大密码字节预算至少为最低字符数的四倍。规则文件是至多 1 MiB 的版本 1 JSON 整串 SHA-256 弱密码摘要表，采用与公开证书相同的受控文件读取约束；格式及边界见上述设计。生产字典仍须验证来源、许可与覆盖，不能将合成测试词条当作生产覆盖证明。

所有副本须预先显式初始化匹配额度和完整规则指纹的独立 marker；启动和请求均只核对，不修复丢失状态。规则/词典变化需要隔离旧副本并执行受控维护，不通过换 namespace 或重建计数绕过额度。改密成功推进持久化认证代次，清除两个浏览器 Cookie 后重新登录；503 可能已提交，不能自动重发。关闭该能力时移除 `password_change` 配置并重启；回退旧二进制也先移除该对象，已完成的密码更新仍保留，无 schema 迁移。

## 构建与测试（Docker Desktop）

开发前可执行 `python3 scripts/doctor.py`（结构化输出加 `--json`，警告也需阻断时加 `--strict`）。它只检查任务分支、缓存主线祖先关系、工作区、hooks、工具、Linux Docker/Compose、端口与资源；不联网 fetch、不修复环境、不启动容器。默认端口取 `OJ_WEB_PORT` 或 8080，可用 `--port` 指定。未提交改动、缓存主线落后或端口占用为警告；缺工具、主线/游离 HEAD、hooks 或 Docker 不可用为失败。缓存主线不证明远端最新，资源展示不是容量验收，端口探测后也可能被其他进程占用。

本机启动 Docker Desktop，使用 `desktop-linux` context。无需安装 Go、Node、pnpm、PostgreSQL 或 Make；宿主 Python 3 只编排测试。Linux CI 使用 Docker Engine。Makefile 已移除。

```sh
docker --context desktop-linux compose run --build --rm toolchain backend
docker --context desktop-linux compose run --build --rm frontend-check
docker --context desktop-linux compose run --build --rm toolchain sqlc
docker --context desktop-linux compose run --rm toolchain db
python3 scripts/repo_policy.py repository
python3 -B -m unittest discover -s tests/governance -v
python3 scripts/test-compose.py
```

后端包含格式、vet、golangci-lint、race、全部命令构建；前端包含 lint、类型、测试、构建。Go 静态分析覆盖主模块、sandbox 标签测试源码及独立会话实验模块，并先验证检测器能接受正确样例、拒绝四类错误；不执行 sandbox 选手代码。规则与边界见[代码质量检查](docs/code-quality.md)。SQL 生成检查在容器内比较submission/identity/problem/problemreference四包生成前后内容，新增包为`internal/problemreference/postgres/referencedb`；数据库检查使用容器内临时 PostgreSQL 14，覆盖迁移、并发租约、持久化参考申请及两个真实 API/mTLS 控制端。申请服务组合测试使用合成会话观察，不代表该能力的真实Redis/HTTP、Worker消费或大材料验收。工具镜像不挂载 Docker socket，不访问用户数据库。测试每次运行，不以构建缓存替代测试。

测试专用会话 Store 成本采样使用 `python3 scripts/test-session-cost.py --module-cache <已校验缓存目录> --output <新JSON路径>`，只用已有本地镜像，独立私有 TCP 容器中比较启用相同授权守卫的路径；不是 API 跑分或容量准入。配置、采样窗口和持久化差异见[会话实验](tests/sessionexperiment/README.md#同等普通会话合同的-store-成本采样)，正确性仍执行原行为/race专项。

普通 Redis 协议专项（只执行合成账号数据）：

```sh
docker --context desktop-linux build --target toolchain --tag oj-session-protocol-toolchain .
docker --context desktop-linux pull redis:7.2.17@sha256:0312984fbfeb2ff1d4382ecef45b50e7d65478a6a58cf12ce28d6de7a4142e45
python3 -B scripts/test-session-protocol.py --toolchain-image oj-session-protocol-toolchain --build-cache-volume ''
```

脚本只使用已有本地镜像，检查 Linux/架构，创建带唯一 owner 标签的私有 internal 网络、Redis 和工具链容器；无宿主发布端口或 Docker socket。临时 PostgreSQL 只开放容器内 Unix socket，复制源码白名单并运行真实存储/race 专项。结束检查退出码/OOM 并仅清理自有容器和网络，保留缓存卷。可用 `--redis-image` 指定已核验本地镜像、`--module-cache` 挂载既有只读模块缓存；合并缓存后执行 `go mod verify`，测试使用 `-mod=readonly`。Linux CI 使用 `--context default`；此专项不是完整 AC23、容量或部署验收。

### 浏览器账号专项

账号页面与 API 必须由同一 HTTPS Origin 提供；当前默认 Compose/Vite HTTP 入口会阻止账号请求。不要通过关闭 Secure Cookie/来源检查或启用跨源代理来让密码表单工作。实际流程及限制见[浏览器账号](docs/account-browser.md)。

专用驱动要求本机已有 Node、Playwright 模块和匹配浏览器（仅用于自动化），应用构建仍使用 Docker；不自动安装或下载。先按权限准备当前锁文件对应的前端工具镜像及上面的 Go/Redis 镜像，再选择仓库外的全新产物目录：

```sh
docker --context desktop-linux build --target frontend-source --tag oj-browser-frontend-source .
python3 scripts/test-browser-account.py \
  --frontend-image oj-browser-frontend-source \
  --playwright-module /absolute/path/to/installed/playwright \
  --output-dir /absolute/new/path/browser-account
```

可选 `--chromium-executable /absolute/path/to/existing/chromium` 指定已有浏览器；`--statement-corpus /absolute/path/to/prepared-bundle/statements` 增加至多10份只读Markdown的浏览器渲染耗时/节点测量（响应替换，不冒充十题真实HTTP授权验收）；`--toolchain-image`、`--module-cache`、`--build-cache-volume` 和 `--context` 的约束沿用现有专项。驱动只发布随机回环 HTTPS/HTTP 对照端口，建立隔离的合成账号/存储；真实 race API 使用固定 pin 的 mTLS，外层临时证书只在新测试 context 忽略。控制端点仅供本夹具，不能对外开放或接入真实数据。

要验证真实十题预览，改用 `--authorized-draft-bundle /absolute/path/to/verified/bundle`，不能与 `--statement-corpus` 同用。它只接受当前锁内十题的完整私有草稿包，先复验完整性，再只读挂载；用合成账号经真实会话导入到临时 PostgreSQL 和材料根，通过本人列表与预览核对内容版本、题面、18 对样例与复制字节，并逐题检查匿名和另一作者的拒绝。该模式不执行题源程序，截图及报告留在私有输出目录。

产物包含日志、浏览器断言 JSON 与桌面/390px 截图；当前15组保留账号/预览与导入恢复覆盖，另验新浏览器档案/IndexedDB禁用时真实服务器分页发现及本人预览、迟到页隔离、列表不改变pending日志；实际Cookie换账号覆盖四条管理路由。独立journal测试入口仅由此驱动构建到临时assets，生产构建不引用它。驱动分别检查浏览器和夹具结果，结束只清理本轮带 owner 标签的资源。`integration && browser` 夹具只运行 `TestBrowserFixture`，普通 PR CI 不自动执行此手动浏览器专项。生产证书、浏览器兼容矩阵及完整业务验收仍须另做。

真实 Worker 专项（当前固定 macOS arm64、Docker Desktop LinuxKit 7.0.14）：

```sh
python3 scripts/test-worker.py all
# 也可选择 sandbox / report / dual-worker / worker-fault / slot-recovery / network-fault / compile-lease / resource-bounds / http-loop
```

脚本用 Docker 编译宿主测试驱动与 `.local/bin/` 命令，构建固定 go-judge/GCC 镜像，再通过宿主 Docker CLI 操作两个禁网、无挂载、只读根文件系统的专用特权容器。独立无特权观察容器共享 Docker VM 的 PID namespace，核验停止后的进程确已消失。不会启动 Colima；Docker Desktop 的 VM 与其他项目共享，这与原专用 VM 隔离边界不同。只执行仓库合成测试，不作为生产不可信提交的部署方案。

首次下载需联网；重复运行复用镜像和停止的槽，拒绝接管其他来源或正在运行的同名容器。测试最后停止自己的槽，删除本次唯一命名的临时 PostgreSQL 容器（数据在 tmpfs），不删除其他容器、卷或旧 Colima 资源。异常隔离标记不自动清除。跨编译的宿主测试不含 race；Linux 容器内后端/数据库测试保留 race。

Docker 更新导致内核或镜像变化时会拒绝执行；须验证新环境并发布新材料版本，不能放宽断言。当前执行材料为 `m0-sum-v3`；v1 默认与 v2 原始清单继续保留，执行器拒绝旧版本。初始化夹具、提交 API、控制端应一致设置 `OJ_EXPERIMENT_MATERIAL_VERSION=m0-sum-v3`。

独立构建宿主实验命令：

```sh
docker --context desktop-linux build --target host-artifacts --output type=local,dest=.local/bin .
```

`.local/bin/worker` 需要 `OJ_SANDBOX_EXPERIMENT=1`、`OJ_WORKER_CONTROLLER_URL`、`OJ_WORKER_CA_FILE`、`OJ_WORKER_CERT_FILE`、`OJ_WORKER_KEY_FILE`；控制端 `.local/bin/worker-api` 需要 `OJ_WORKER_API_ADDR`、证书文件和 `OJ_DATABASE_URL`，材料目录为 `OJ_MATERIAL_ROOT`。缺失配置拒绝启动，测试脚本自动生成私有临时配置。详见 [M0 契约与历史证据](docs/m0-experiment.md)。

Worker 启动完成后，向该 Worker 进程发送 `SIGUSR1`（`kill -USR1 PID`，将 PID 替换为对应进程号）请求排空式下线：每轮领取前检查排空，已通过检查的至多一个在途领取继续完成下载、执行及回报；确认后不再领取。空队列等待可被唤醒；未知回报仍按原令牌/结果最多发送三次，失败保留原错误。`SIGINT`/`SIGTERM` 仍取消当前操作并等待停止确认。排空返回后同样停止沙箱、确认 PID 消失，成功才清正常占用标记；停止不确定继续隔离，不自动重启或解封。排空耗时受现有任务/网络预算影响，不承诺固定下线 SLA；需要中止时仍可发送取消信号。旧版本回退后不再使用新增信号，无数据库迁移。`python3 scripts/test-worker.py report` 包含执行中排空、保留后续排队任务及显式重启复用的真实验证。

### HTTP 到真实判题终态专项

受信维护流程须先使用 Docker 构建的 `.local/bin/migrate up` 升级 schema，再按材料版本执行 `.local/bin/migrate fixture`，最后显式执行 `.local/bin/migrate backlog <全局上限> <主体上限>`；命令共用私有 `OJ_DATABASE_URL`。不执行最后一步会拒绝新提交，API 不自动设值。专项驱动在独占临时数据库配置的 `10000 10000` 仅为合成夹具，不是生产建议。升级已有服务必须先停止全部旧版写入 API，详见[升级与回退](docs/submission-backlog.md#升级与回退)。

`python3 scripts/test-worker.py http-loop` 显式设置 `OJ_SANDBOX_HTTP=1`，`all` 同样包含此项。驱动建立空的临时 PostgreSQL，实际运行迁移/固定材料初始化、两个提交 API 进程、mTLS 控制端及 Worker 命令；只提交仓库内固定 AC/WA 合成源码。先由 HTTP 确认 queued，再启动 Worker，通过 HTTP 观察 finished/AC 或 WA；数据库仅旁证原始源码、固定归属及每题唯一完成 attempt。

专项检查跨 API 原键重试/回执、异源冲突、缺失/错误凭据、Origin 拒绝无写入、闭合回执字段及无 Cookie/CORS、Worker 正常排空和槽停止。提交 API 用 `127.0.0.1:0`，仅从自身私有有界 JSON 日志取得回环地址，客户端禁代理/重定向；控制端沿用既有临时端口流程。私有 Bearer 留在服务端测试进程中。该入口不支持浏览器，不授予普通账号训练权限；独立opt-in最小页面及其失败路径见下节。无 API、迁移或预算变化，可单独回退专项及驱动选择器。

### M0 最小浏览器实验页

API 额外设置 `OJ_EXPERIMENT_BROWSER=1` 才提供 `/m0` 页面及[浏览器实验契约](api/m0-browser.openapi.yaml)。必须同时显式设置 `OJ_API_ADDR=127.0.0.1:<端口>`（可用0）、私有 `OJ_EXPERIMENT_TOKEN_FILE`、实验数据库及 `OJ_EXPERIMENT_MATERIAL_VERSION=m0-sum-v3`；不能混用 `OJ_SESSION_CONFIG_FILE`。其他开关值、主机名/通配监听或材料版本拒绝启动。仅供可信本机和独占合成数据库；从实际API启动日志取得回环地址访问 `/m0`，默认 Compose、账号网关和做题预览不开放此页。

页面只接受服务端固定AC/WA示例，不接收任意源码、主体、上下文或语言。独立实验归属与原Bearer提交隔离；Bearer不下放，随机nonce用于CSRF防护，不隔离同源脚本，也不代表账号身份。所有写请求严格检查Host、单一Origin、Fetch Metadata及nonce；数据GET也需nonce。不使用或设置会话Cookie、不开放CORS，页面含CSP/no-store/nosniff；HTTP回环实验不等于生产HTTPS或训练准入。

原键/预设和内容核验状态只保存于当前标签页的sessionStorage，刷新后重新bootstrap并只查询原键，不自动提交。未知回执/404/503不等于回滚；原样重试才确认内容归属，409既有结果与当前示例明确分开。停止查询不取消任务，自动查询次数/时长有界；只有收到终态后才可显式开始下一次实验。

使用已有Playwright和Chromium验收（不自动安装，不包含在 `all` 中）：

```sh
python3 scripts/test-worker.py browser-loop \
  --playwright-module /absolute/path/to/existing/playwright \
  --chromium-executable /absolute/path/to/existing/chromium \
  --output-dir /absolute/new/path/m0-browser
```

Chromium参数可省略以使用该Playwright已安装的浏览器。产物必须为仓库外新目录，含检查JSON、私有日志及桌面/390px截图；测试以真实POST提交成功后丢弃响应验证未知/原键恢复，在浏览器确认queued且数据库无attempt后才启动真实Worker。查询503及提交BACKLOG_LIMIT/ADMISSION_UNAVAILABLE展示另由浏览器路由注入，不冒充真实数据库故障；共享准入后端另有真实PostgreSQL/双API验证。回退页面可关闭开关；数据库当前 schema 19按[参考申请合同](docs/reference-requests.md#迁移与回退)、历史schema8/7/6按草稿/积压合同升级与回退，不能直接混跑旧版进程；已接受提交仍按原Worker协议处理。

### 沙箱资源预算专项

`python3 scripts/test-worker.py resource-bounds` 只在上述固定专用环境内编译/执行有界合成探针，保持现有材料、容器、运行和编译预算。普通 CI 仅编译和静态检查带 sandbox 标签的源码，不执行这些程序；驱动显式设置 `OJ_SANDBOX_RESOURCES=1`，`all` 也包含该专项。

- 进程：父进程最多尝试32次fork，子进程不递归创建、关闭管道写端后等待；核对15个存活子进程加父进程、cgroup `procPeak=16`、下一次EAGAIN，以及逐一wait后再次成功fork/wait。其他errno、首个fork即被seccomp拒绝或不能回收均失败。
- inode：最多8192个空文件，要求4094个文件加目录耗尽剩余inode、ENOSPC且字节仍有余量；删除/重建一个文件验证复用，保留满inode状态给下一次执行核对新目录/完整容量恢复。
- 字节：现有单文件 `RLIMIT_FSIZE=1MiB` 会先阻止大文件，探针按最多80个文件、每个最多1MiB分段真实写入。只在直接沙箱探针使用既有Compile的10秒CPU/15秒wall/512MiB/16进程数值，不改变普通Run；要求64MiB实际写入、ENOSPC、可用字节零且inode仍有余量。分别验证满盘后的执行隔离，以及关闭fd/删除后容量恢复并再次写入。
- 普通Run保持200ms CPU/2秒wall/32MiB/16进程：单文件有限写入验证1MiB后的SIGXFSZ/OLE；分文件写入验证内存先耗尽为MLE。每项后正常探针核对目录消失与容量完整恢复，最后删除唯一缓存制品。

限制由固定版本的[执行参数](https://github.com/criyle/go-judge/blob/v1.13.3/worker/worker.go)、[实际执行](https://github.com/criyle/go-judge/blob/v1.13.3/envexec/run_single.go)及实测共同核对；`copyOutMax` 不决定单文件RLIMIT_FSIZE。普通Run的SIGXFSZ/OLE现已由正式适配器精确支持，接受条件与未覆盖的Compile分支见[判题设计](docs/architecture/judging-design.md#53-统一结果分类与单点异常归因)。`python3 scripts/test-worker.py report` 另验同一Worker/LeasedRunner/容器生命周期的OLE→AC、未截断观测、真实持久化及双控制端重报，不能以直接资源探针代替该回报证据。本资源专项不验收跨主机、长期容量、宿主文件系统耗尽或完整M0；资源测试与驱动可单独回退，不更改预算。实测归[交付记录](docs/delivery-log.md)。

同一 `resource-bounds` 另含[固定g++编译文件上限对照](tests/sandbox/compile_file_size_test.go)：256KiB有限只读数据控制例AC，1MiB+128KiB示例只准编译，观察到结构化非零退出4、正式CE/零Run，再以同一Runner完成普通AC。固定编译器报告汇编器文件大小受限，诊断仅作辅助观测，不按错误文字判结果；未改Compile命令、LANG、预算或适配器，不代表未知Compile OLE分支已支持。

### Worker 异常后的受控解封

Worker 被 `SIGKILL` 等强制结束后，原槽的持久隔离标记会阻止普通启动。操作者须独占该槽及可信 Docker 控制面，**先确认旧 Worker 的全部宿主 CLI 和在途 Docker 控制操作已结束，不会迟到派发**，再使用**崩溃前已记录或可信历史记录确认的原容器完整 ID**执行：

```sh
OJ_SANDBOX_EXPERIMENT=1 OJ_WORKER_M0_SLOT=primary \
  .local/bin/worker recover <original-full-container-id>
```

将占位值替换为原容器的 64 位小写十六进制 ID；槽只接受 `primary`/`secondary`，未指定为 primary。不要把恢复时按名称查询到的当前 ID 当作历史身份依据；旧 ID 未知、容器已被替换或 Docker 控制面不再独占时，先人工调查。标记只记录活动状态，不自动证明历史容器身份。

判题 Worker、参考验证与恢复默认共用[同机执行槽](docs/architecture/api-worker-design.md#同机共享执行槽)。运行参考验证前先排空并退出判题 Worker；旧二进制和直连沙箱驱动也须先退出并排空。仅双 Worker 与槽恢复专项由测试驱动显式启用 `OJ_WORKER_M0_DUAL_EXPERIMENT=1`，不用于单机试用。

命令不需要控制端地址、证书、数据库或邮件凭据。它持有与 Worker 相同的规范文件锁，先持久隔离，再核对固定槽名与原 ID、固定镜像和隔离配置；后续按 ID/启动时间绑定。运行容器沿用原停止/PID 消失检查，已停止容器依可信 Docker 的停止状态双次回读，不声称重建崩溃前 PID 清单。20 秒操作预算内确认停止后才清标记；错误、取消及停止未知均不报告成功。清空后的 Sync 失败会尽力重建隔离标记，文件系统同时拒绝恢复写入时不保证隔离持久性，须保持停用并调查。

成功只确认当前沙箱已停止并清空槽标记，不领取任务、不重启 Worker。新进程无法排空旧 Worker 的宿主控制请求，Worker 的进程内 25 秒计时器也会随 SIGKILL 消失；仅等待 25 秒、Worker 退出或当前容器停止不能替代控制操作排空的确认。无法确认时保持隔离，不运行解封或重启；命令不提供跨进程派发隔离机制。

在上述前提成立后，显式重启及重新核验固定环境才能运行新 Worker；旧任务仍由控制端自然租约到期及有限恢复处理，不编辑租约/结果。回退只需恢复旧 Worker 二进制并停用此子命令，无数据库迁移。真实专项 `python3 scripts/test-worker.py slot-recovery` 在冻结合成 Worker 后记录宿主子进程，并确认它们退出再解封，覆盖互斥/错误 ID 拒绝、自然接管及原槽重启复用；未覆盖人为延迟的孤儿 CLI 或任意 SIGKILL 后自动安全复用。普通 PR CI 不运行特权沙箱。

### Worker 控制链路网络故障专项

`python3 scripts/test-worker.py network-fault` 在同一组专用离线沙箱及临时 PostgreSQL 上，使用两个真实控制端进程和透明 TCP 中继。中继只转发原始字节，Worker 与控制端仍直接验证彼此的 mTLS；不终止 TLS、不解析 HTTP、不伪造响应，不修改系统路由或其他服务网络。只阻断本轮回环连接的一个或两个方向，恢复前关闭旧连接，避免把旧 TLS 字节重放到恢复后的流。

- 双向阻断：同一 Worker 先完成一次预热 AC，目标执行期间阻断流量；检查有界失败退出、沙箱/PID 停止、无结果及预先排队的下一题零尝试。另一节点经第二控制端，在自然租约到期后完成目标第 2 次尝试，代次保持，两个控制端拒绝旧令牌回报且结果不变。
- 下行确认丢失：短测试事务锁住尚未提交的目标尝试，启用下行阻断后立即回滚释放锁。真实控制端此后提交 AC，原 Worker 收不到确认而失败停止、不领下一题；显式恢复连接/沙箱和新 Worker 后继续排队任务。全过程目标只有一次尝试、首个载荷/摘要不变；两个控制端确认相同重报并拒绝冲突重报。

行锁仅用于给故障注入排序，不更改任务、租约、结果或业务参数；测试驱动不手调 Recover。字节阻断计数不用于识别 HTTP 请求或证明报告调用次数。该专项是本机 TCP 故障注入，不验收跨主机/路由器分区、长编译期间续租或执行端自动恢复。中继的纯回环字节、方向、恢复与并发关闭 race 测试不含 sandbox 标签，普通后端检查执行；真实 Worker 专项保留 sandbox 标签及显式开关，普通 CI 不执行特权代码。实际结果见[交付记录](docs/delivery-log.md)。

### 编译在途续租失权专项

`python3 scripts/test-worker.py compile-lease` 在专用合成沙箱内识别唯一 `cc1plus`，使用容器内 PID 发送 SIGSTOP，另从 `docker top` 核对唯一编译器的 VM PID 和 T 状态；两类 PID 不混用。短暂观察 Worker、容器和同一暂停编译器仍存活后，分别阻断真实控制链路或禁用临时节点。编译器保持暂停且不发送 SIGCONT，原有 15 秒编译 wall、5 秒续租周期及请求/停止预算保持。

测试在启动目标 Worker **之前**记录宿主单调时间 t0，要求 Worker 自行失败退出、容器停止及独立 VM PID 消失证据全部在 `t0+14s` 前取得。编译不可能早于 t0 开始，因此这个保守界限可排除原 15 秒编译 wall、20 秒 curl 等待上限；机器过慢判证据不足，不放宽界限，也不把失败清理引起的停止算作通过。这是编译在途的受控故障验证，不是编译性能测试或生产下线 SLA。

两种故障都检查当前尝试无结果、预先排队的下一题零尝试；随后另一节点经第二控制端完成第2次尝试 AC，判题/恢复代次保持1，旧身份/租约回报被两个控制端拒绝且新载荷不变。网络场景等待自然30秒租约到期，禁用场景可由撤销提前触发回收，不能描述为同一自然到期路径。测试未手调 Recover 或编辑租约时间。真实字节受阻、编译器T状态与提前停止结合既有续租代码支持失权中断判断，不表示解析加密请求或取得逐请求 renew 审计。

只有进程表解析边界测试进入普通 root race；实际暂停/信号、控制端、数据库与选手编译保持 sandbox 标签及 `OJ_SANDBOX_COMPILE_LEASE=1`，由上述驱动提供。不得把此流程用于普通宿主、共享容器或生产不可信提交。回退新增测试与驱动入口即可，无业务代码、协议、依赖、迁移、生产 SQL、材料版本或预算变化；完整 M0、跨主机/断电及执行端自动恢复仍待验，实际证据归[交付记录](docs/delivery-log.md)。

`GET /api/v1/healthz` 仅代表进程存活；普通 Compose 入口不启动完整 Worker 产品。真实编译/运行、回报、双 Worker 接管与撤销/材料故障分别由专项验证，普通 CI 不运行特权实验。

## 文档入口

完整目录与维护归属见[文档索引](docs/README.md)。已选方案不等于已引入或已验证；依赖以配置和锁文件为准，历史验证见[交付日志](docs/delivery-log.md)。
