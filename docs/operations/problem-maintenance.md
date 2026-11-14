# 题源与参考验题

从仓库根目录运行本文命令。设计见[题包与草稿](../architecture/problem-drafts.md)，实测见[验收导航](../acceptance/README.md)。

## Library Checker 首批题源准备

需要已有官方源码副本及本地 Docker 镜像（Python 3.11+、`g++` C++17、GNU tar、`/bin/sleep`、`/usr/bin/env`）。工具只使用已有镜像，不自动下载或安装依赖；镜像须无声明式 `VOLUME`。本次本机实测镜像和精确结果见[题源交付记录](../acceptance/delivery-log.md)，其他环境须重新验证。

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

镜像须无声明式 VOLUME，运行用户必须非 root 且可读取私有候选；在 root 环境须另配明确非 root UID/GID，不直接复制执行上述用户参数。所有写入仅在临时内存盘，编译/检查子进程各限120秒/5秒。模式 `lc-wcmp-tokens-v1` 保留 M0 规则，区别 VT/FF 并显式约束字节/token；[支持域与限制](../architecture/checker-admission.md)不等于完整题目验证或生产预算准入，草稿继续 `not-run`。

### 生产检查器候选预检

在已有 Go 工具链容器内，将已准备的私有题包和固定来源锁挂为只读；运行用户须满足题包的私有文件所有权要求。以下变量分别来自当前题包路径、已接纳的正式参考记录和本次待验证的显式限额，未设置默认值：

```sh
go run ./cmd/checker-candidate --bundle "$PRIVATE_BUNDLE" \
  --source-id library-checker:aplusb --source-lock config/problem-sources/library-checker.json \
  --expected-plan "$TRUSTED_FORMAL_PLAN_SHA256" --checker lc-wcmp-tokens-v1 \
  --bytes "$CANDIDATE_BYTES" --token-bytes "$CANDIDATE_TOKEN_BYTES"
```

成功时输出候选配置和摘要。命令只检查冻结答案，既不运行题源程序，也不采纳生产限额；见[预检契约](../architecture/checker-admission.md#生产检查器候选预检)。

### 候选检查器原始输出试跑

在已配置且停止的专用参考槽上，可用上一步生成、另行核验的候选摘要试跑 A+B、并查集或 Z 算法。A+B 是默认来源；并查集和 Z 算法在预检、试跑及离线复核均须分别指定 `--source-id library-checker:unionfind` 或 `--source-id library-checker:zalgorithm`。A+B 和并查集沿用小型参考槽（262144 字节）；Z 算法沿用下节配置的 medium 槽（4194304 字节）。候选的显式字节上限须等于该槽的采集上限，并显式指定 token 限额；这些值不作为生产默认值。沿用下节构建的宿主 `worker`，将来源锁、题包和输出目录写成绝对路径；输出目录必须新建且在题包之外。

```sh
env -u OJ_WORKER_M0_SLOT -u OJ_WORKER_M0_DUAL_EXPERIMENT OJ_SANDBOX_EXPERIMENT=1 \
  .local/validation-host/worker validate-checker-candidate --start-stopped-slot \
  --bundle "$PRIVATE_BUNDLE" --source-lock "$PWD/config/problem-sources/library-checker.json" \
  --expected-plan "$TRUSTED_FORMAL_PLAN_SHA256" --expected-candidate "$TRUSTED_CANDIDATE_SHA256" \
  --bytes "$CANDIDATE_BYTES" --token-bytes "$CANDIDATE_TOKEN_BYTES" --output "$NEW_TRIAL_REPORT"
```

报告只含候选、计划和档位绑定及运行结果，不含原始输出。运行或停止未确认时保留 `INCOMPLETE`，启动失败可能没有报告；如需恢复，只按现有参考槽恢复流程核对同一容器身份。见[试跑契约](../architecture/checker-admission.md#候选检查器原始输出试跑)。

归档目录不再被写入后，可复用前面的 `checker-candidate` 构建产物离线复核：

```sh
checker-candidate verify-trial \
  --bundle "$PRIVATE_BUNDLE" --source-lock "$PWD/config/problem-sources/library-checker.json" \
  --expected-plan "$TRUSTED_FORMAL_PLAN_SHA256" --expected-candidate "$TRUSTED_CANDIDATE_SHA256" \
  --report-dir "$COMPLETED_TRIAL_REPORT" --expected-report "$TRUSTED_REPORT_SHA256" \
  --bytes "$CANDIDATE_BYTES" --token-bytes "$CANDIDATE_TOKEN_BYTES"
```

路径须是绝对路径；报告摘要须通过独立可信的记录获得，不从待复核的副本现算并自证。成功仅输出报告、候选、计划、档位摘要和通过点数。命令不运行程序、不查当前槽状态，也不授予生产准入。

### A+B 固定材料隔离判题

使用上述同一私有 A+B 题包、可信计划和候选摘要及限额，先通过 `checker-candidate aplusb-judge-manifest` 核对有序材料和固定运行档位。确认输出与可信的材料计划一致后，固定其 `sha256`；Worker 会从私有题包重新生成同一清单，摘要不一致时不会启动参考槽。

试用机 amd64 的离线准备可为此命令加 `--trial-amd64-candidate`；它仅支持已审核的 A+B 17 点修订，不能与 `--ubuntu-aplusb` 同时使用。该清单只用于候选材料准备，当前 Worker 不接受。见 [P4 候选材料记录](../acceptance/delivery-evidence/p4-trial-publication.md#ab-amd64-候选材料)。

```sh
checker-candidate aplusb-judge-manifest \
  --bundle "$PRIVATE_BUNDLE" --source-lock "$PWD/config/problem-sources/library-checker.json" \
  --expected-plan "$TRUSTED_FORMAL_PLAN_SHA256" --expected-candidate "$TRUSTED_CANDIDATE_SHA256" \
  --bytes "$CANDIDATE_BYTES" --token-bytes "$CANDIDATE_TOKEN_BYTES"

env -u OJ_WORKER_M0_SLOT -u OJ_WORKER_M0_DUAL_EXPERIMENT OJ_SANDBOX_EXPERIMENT=1 \
  .local/validation-host/worker judge-aplusb-isolated --start-stopped-slot \
  --bundle "$PRIVATE_BUNDLE" --source-lock "$PWD/config/problem-sources/library-checker.json" \
  --expected-plan "$TRUSTED_FORMAL_PLAN_SHA256" --expected-candidate "$TRUSTED_CANDIDATE_SHA256" \
  --expected-manifest "$TRUSTED_JUDGE_MANIFEST_SHA256" \
  --submission "$PRIVATE_CPP_FILE" --expected-submission "$TRUSTED_SUBMISSION_SHA256" \
  --bytes "$CANDIDATE_BYTES" --token-bytes "$CANDIDATE_TOKEN_BYTES" --output "$NEW_JUDGE_REPORT"
```

路径使用绝对路径。源码须是权限 0400 或 0600 的私有普通文件，最多 64 KiB，且为不含 NUL 的 UTF-8；摘要固定原字节。只用现有停止的断网小型参考槽，输出目录须新建且在题包外。退出码 0 表示已产生确定判定并完成清理，实际 AC、WA、CE 等查看私有报告。详见[隔离判题契约](../architecture/checker-admission.md#ab-固定材料隔离判题)。

专用本地 Ubuntu arm64 VM 使用同一组命令，在生成清单和执行判题时都加 `--ubuntu-aplusb`，并固定 Ubuntu 清单自己的摘要。Worker 使用客体内的 Linux arm64 构建，以客体 root 运行，并去掉 `OJ_EXPERIMENT_MATERIAL_VERSION`。准备固定停止槽、核验边界和恢复步骤见 [Ubuntu 实验](../architecture/checker-admission.md#ubuntu-独立判题实验)。

### A+B 租约链路联验

在 [A+B 固定材料清单命令](#ab-固定材料隔离判题)上加 `--expected-manifest "$TRUSTED_JUDGE_MANIFEST_SHA256" --export "$NEW_PRIVATE_DIRECTORY"`，导出 `manifest.json` 及私有对象。核对后固定该运输清单的原始字节 SHA-256。隔离链路的登记、控制端和 Worker 都设置 `OJ_SANDBOX_EXPERIMENT=1`、`OJ_APLUSB_MANIFEST_FILE`（绝对路径）、`OJ_APLUSB_MANIFEST_SHA256`；不同时设置 `OJ_EXPERIMENT_MATERIAL_VERSION`、M0 槽或双槽配置。

登记进程和控制端另设 `OJ_MATERIAL_ROOT` 为该导出目录。在已迁移的临时 PostgreSQL 上执行 `migrate fixture` 和显式的积压策略配置，再按现有私有 mTLS 方式启动 `worker-api`。Desktop arm64 `worker` 可使用已启动的固定断网小型参考槽，或设置 `OJ_APLUSB_START_STOPPED=1` 让它持槽锁启动已停止的参考槽。普通 API 不启用此版本。真实联验使用 sandbox 标签的 `TestAPlusBLeasedLoop`，须显式设置 `OJ_SANDBOX_APLUSB=1`、空的临时 `OJ_TEST_DATABASE_URL` 并准备同一 worktree 的 `.local/bin/{migrate,worker,worker-api}` 宿主构建产物；测试从停止的参考槽开始，自行登记材料和临时 mTLS 身份。AC 走已验证账号的训练存储入口与本人回读；WA、OLE 仍走固定实验入口。账号由测试夹具提供，真实 Redis 登录由普通会话专项另验。不得指向试用库或正在使用的参考槽。见[判题契约](../architecture/checker-admission.md#ab-租约链路联验)。

专用 Ubuntu VM 导出时另加 `--ubuntu-aplusb` 并使用 Ubuntu 判题清单摘要。登记、控制端和 Worker 均设置 `OJ_APLUSB_RUNTIME=ubuntu-arm64` 和 Ubuntu 运输清单及其摘要，Worker 必须设置 `OJ_APLUSB_START_STOPPED=1`。在客体内用 Linux arm64 构建和相同测试，另传 `OJ_TEST_APLUSB_DESKTOP_MANIFEST` 为原 Desktop 运输清单的绝对路径，用来验证零领取。恢复后重判 Ubuntu 提交时，维护命令也需要同一档位和清单。

### 核对 A+B 现场准入记录

已有[交接报告回执](../architecture/checker-admission.md#封存与复核报告回执)，且维护者已从独立渠道取得目标节点、Worker 制品及两份[现场观察记录](../architecture/checker-admission.md#生产检查器与目标节点记录复核)的绑定后，可在可信主机离线核对：

```sh
checker-candidate review-aplusb-admission \
  --receipt-directory "$PRIVATE_RECEIPT_DIRECTORY" --expected-receipt "$TRUSTED_RECEIPT_SHA256" \
  --expected-handoff "$TRUSTED_HANDOFF_SHA256" --expected-worker "$TRUSTED_WORKER_SHA256" \
  --target-observation "$PRIVATE_TARGET_JSON" --expected-target-observation "$TRUSTED_TARGET_SHA256" \
  --checker-observation "$PRIVATE_CHECKER_JSON" --expected-checker-observation "$TRUSTED_CHECKER_SHA256" \
  --node "$EXPECTED_NODE" --revision "$EXPECTED_REVISION" --runtime "$EXPECTED_RUNTIME" --slot "$EXPECTED_SLOT"
```

三处路径须为绝对路径；摘要用独立取得的值。成功输出 `review-input-only` 供维护者核对；不会启用生产检查器、确认目标环境通过或批准题目。

### 私有草稿参考验证

[#151 新题包](../acceptance/delivery-log.md#2026-11-11新版题包十题参考复验与卷积报告接纳)的十题254点均通过固定环境维护参考验证并确认停止。最初批次因素性测试宿主超时中止，受控恢复后以新报告完成余下六题；[原失败](../acceptance/delivery-log.md#2026-11-11新版题包维护参考复验中止)仍保留，根因未知。维护入口不写数据库验证状态或发布。需要已配置且停止的专用槽、已有固定镜像与 Docker Desktop、macOS arm64；命令不会自动创建、替换或恢复槽。使用已有 Go 1.26.11 工具链镜像构建独立宿主产物（不与 Linux 检查共用输出）：

`associative_array`使用512MiB完整材料总预算、input48MiB、stdout/checker8MiB、payload1GiB，独立CPU5秒/wall10秒，保留更严的整体3分钟和原medium物理槽/锁。同次48MiB输入→8MiB输出、完整20AC/Stop及随后独立原六来源回归已通过；[参考合同](../architecture/reference-validation.md#associative_array参考验证)与旧失败限制分开记录。

point_add_range_sum新版本已吸收主线停止预算修复，并通过一次八来源174点及全部原子组的固定环境接纳；旧suffixarray失败报告保留，根因仍未知，不据新通过追认旧失败或证明长期稳定。最终CI与合并状态见[PR #114](https://github.com/DL-Association-for-Computing-Machinery/OnlineJudge/pull/114)，见[八来源新版本接纳记录](../acceptance/delivery-log.md#2026-11-04point_add_range_sum-八来源固定环境接纳)。

staticrmq新版本已吸收主线停止修复及已合并#114，通过一次九来源201点/24个原子子测试的固定环境接纳；历史第9点失败与根因未知结论保持，不追认旧失败或证明长期稳定。最终CI与合并状态见[PR #119](https://github.com/DL-Association-for-Computing-Machinery/OnlineJudge/pull/119)，证据见[九来源接纳记录](../acceptance/delivery-log.md#2026-11-04staticrmq-九来源固定环境接纳)。

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

路径替换为已有完整私有bundle和新的报告目录。`--start-stopped-slot`在持锁/同步dirty后仅启动预存在、固定owner/配置匹配的停止槽；省略则要求槽已由维护者启动。启动回执未知时仍确认原生命周期停止；不能确认停止或清标记失败才保持quarantine，即使随后安全解封，本次报告仍失败。禁止自动重试、换槽掩盖失败或手工清标记。成功也会停止该槽。`report.json`私有保存逐点结果、完整plan/profile及容器生命周期，`publication`始终blocked；失败输出与支持域见[受限参考验证](../architecture/reference-validation.md)。

停止失败报告的可选`stop_diagnostic`提供固定阶段/错误类别、有界计数和最多六步的耗时记录，不含PID、命令参数或原始错误。`elapsed_ms`超过60000时截断并标记`elapsed_capped`，不是停止时限；原共享10秒Stop预算保持。kill步骤记录调用前context、调用尝试及本地CLI启动证据，`local_process_start`不证明Docker daemon收到kill或容器收到信号，`unknown`不表示未启动。`pid_present`仅表示探测时路径存在，可能发生PID复用；`deadline`不证明进程仍在或已退出。诊断不授予清理隔离标记的权限，旧失败报告不追认通过；字段与证据限制见[受限参考验证](../architecture/reference-validation.md)。

执行适配器失败时，可选`execution_diagnostic`按观察顺序保存最多八条固定operation/point/phase/role/code，保留主错误、清理错误和独立取消原因，超出标记`truncated`。角色来自本次缓存账本，不含材料、ID或原始错误。它可与停止诊断并存，不改变SE/隔离结论或追认旧失败；成功报告不增加字段。具体范围见[执行失败诊断](../architecture/reference-validation.md#执行失败诊断)。

停止诊断的受信命令单测随普通Docker backend运行；在Darwin核对本地执行器时，使用上面单独构建的workerhost测试包，先确认列表恰含下列三项，再选择执行。`cmd/worker`驱动不包含依赖包的单测，空匹配的PASS不能算验证。此处仅运行固定受信shell及测试替身，不启动沙箱槽或执行题源：

```sh
.local/validation-host/workerhost-stop.test \
  -test.list='^(TestStopTraceDistinguishesExpiredKillAttempt|TestLocalCommandStartEvidence|TestStopElapsedIsBoundedAndMarked)$'
.local/validation-host/workerhost-stop.test -test.v -test.count=1 \
  -test.run='^(TestStopTraceDistinguishesExpiredKillAttempt|TestLocalCommandStartEvidence|TestStopElapsedIsBoundedAndMarked)$' -test.timeout=1m
```

Host控制失败时，执行诊断条目可附带有界`host_trace`，区分入场、准备、inspect、command及响应校验；本地CLI已启动不代表daemon收件。字段范围、取整及缺失证据限制见[执行失败诊断](../architecture/reference-validation.md#执行失败诊断)。以下原生专项仅运行固定受信命令与替身；先核对清单恰含八项再执行，不能以空匹配PASS计为验证：

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

全部材料完整哈希、所有答案检查域先验后才调用execution Verify/Compile；槽启动与Host环境探测更早发生。快照并发/关闭、失败报告和独立槽`recover-reference <original-full-container-id>`的前置要求见[受限参考验证](../architecture/reference-validation.md)。

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

convolution_mod已通过独立16MiB输出槽的同次输入/输出边界与完整53点验证，预算与限制见[维护合同](../architecture/reference-validation.md#convolution_mod参考验证)，十来源254点/26子测试接纳及最终CI/合并状态见[交付记录](../acceptance/delivery-log.md#2026-11-04convolution_mod-十来源固定环境接纳)。原第四槽已创建一次并完成本次Stop；以下创建命令仅在确认目标不存在时使用，已有槽不得替换：

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

使用上方的 Darwin arm64 宿主产物。固定错误解负对照只在专用 `worker-validation.test` 中运行：显式设置 `OJ_CONVOLUTION_DIFFERENTIAL_TEST=1`、`OJ_SANDBOX_EXPERIMENT=1`、`OJ_CONVOLUTION_DIFFERENTIAL_SOURCE_ROOT`、`OJ_CONVOLUTION_DIFFERENTIAL_SOURCE_LOCK` 和新目录 `OJ_CONVOLUTION_DIFFERENTIAL_TEST_OUTPUT`，清空上述两个槽覆盖，选择 `-test.run='^TestConvolutionDifferentialNegativeSandbox$' -test.timeout=5m`。负对照应在报告中留下合法系数差异并确认停机。私有报告和限制见[卷积小规模独立对拍](../architecture/reference-validation.md#卷积小规模独立对拍)。

static_range_sum的维护命令选择`--source-id library-checker:static_range_sum`和新报告目录。以下显式专项先验证完整16MiB输入与8MiB stdout/4KiB stderr、stdout超限的窄SIGXFSZ证据及stderr4097拒绝，再运行完整11点CLI。预期负例只接受一条明确验证错误；任何额外清理/取消/截断错误或Stop失败均中止后组。首次启动失败与后续环境改变后的新通过分开记录，不沿用已消耗的协议重试；固定文件限制/collector区分见[参考合同](../architecture/reference-validation.md#static_range_sum参考验证)：

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

## 本地不可变材料存储

Linux维护工具使用预建持久根，根为运行euid所有且权限0700；维护者须先完成根/父目录持久化准备。默认镜像UID为65532，输入也须允许该用户读取。命令不初始化或修复目录，不提供用户授权、数据库草稿导入或发布；完整合同见[本地材料存储](../architecture/material-store.md)。

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
