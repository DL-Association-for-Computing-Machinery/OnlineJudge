# 试用制品与值守操作

从仓库根目录运行命令；实际试用环境的操作参见[值守手册](p6-operator-runbook.md)，须已有相应授权。

### 试用离线应用制品

从干净、已审查的提交构建一次。根据目标机明确选择 `linux/amd64` 或 `linux/arm64`，将输出写入新目录：

```sh
stage=$(mktemp -d)
docker --context desktop-linux buildx build --builder desktop-linux \
  --platform linux/arm64 --target trial-artifacts \
  --output "type=local,dest=$stage" .
```

输出包含 `bin/` 下的 API、Worker、控制器、内嵌迁移工具、材料工具和邮件执行器，`site/` 下的正式前端，以及 `templates/nginx.conf.example`。开发登录启动器、实验测试二进制、真实配置和证书不会导出。首次构建需获取已锁定的依赖；目标机接收成品，不重新编译。

在发送端用 Python 3.9+ 封存到一个尚不存在的同级目录。通过独立可信渠道传递打印出的清单 SHA-256。接收端保留文件权限、由接收账号持有整个目录，使用可信工作树中的脚本核验；两端核对期间都不修改目录。接收端按目标机架构填写 `--platform`。

```sh
python3 -B scripts/trial_app_bundle.py pack --export "$stage" \
  --revision "$(git rev-parse HEAD)" --platform linux/arm64 "${stage}-bundle"
python3 -B scripts/trial_app_bundle.py verify --manifest-sha256 "$TRUSTED_MANIFEST_SHA256" \
  --platform linux/arm64 "$RECEIVED_BUNDLE_DIR"
```

核验成功输出构建方声明的 Git 提交；不执行程序，也不替代构建来源审查。应用包 v2 要求同时封存 Nginx 模板；不含模板的 v1 旧包须重新导出封存后再做此联验。封存失败时，未写出清单的输出目录不可交付，复测使用新目录。完整[试用离线交付](operations-design.md#5-发布与回滚)还需运行依赖镜像、其他现场配置及安装恢复说明。密钥与真实配置单独交接。

### 核验试用运行镜像

发送端按已审查的镜像 ID 导出指定平台，不按可变标签导出。下面以 Linux arm64 为例；如果试用拓扑需要单独的判题镜像，在同一归档加入该镜像 ID，接收端同时传入 `--sandbox`。

```sh
docker --context desktop-linux image save --platform linux/arm64 -o "$RUNTIME_ARCHIVE" \
  "$NGINX_IMAGE_ID" "$POSTGRES_IMAGE_ID" "$REDIS_IMAGE_ID"
sha256sum "$RUNTIME_ARCHIVE"
```

通过独立可信渠道交接归档 SHA-256，以及各角色的**平台镜像配置摘要**（OCI 镜像清单的 `config.digest`）。镜像索引摘要、平台清单摘要和配置摘要不可互换。配置摘要须由维护者对照事先审查的镜像确认，不能从收到的归档自行认定。用可信工作树的脚本核验，要求 Python 3.11+ 和现代 Docker 保存的单平台、无标签 OCI 归档：

```sh
python3 -B scripts/verify_trial_images.py "$RUNTIME_ARCHIVE" --sha256 "$TRUSTED_ARCHIVE_SHA256" \
  --platform linux/arm64 --nginx "$NGINX_CONFIG_DIGEST" \
  --postgres "$POSTGRES_CONFIG_DIGEST" --redis "$REDIS_CONFIG_DIGEST"
```

命令只读核对归档、每个引用 blob、目标平台和可导入镜像集合，不解包、不调用 Docker。通过后仍需在受控的空镜像库验证导入、回读身份和禁拉取启动。可选判题镜像仍须单独满足目标节点的运行准入。

### 核对同一批试用制品

发送端从审查记录取应用包摘要、运行镜像归档摘要和各角色配置摘要，将应用、内置 Nginx 模板和运行镜像绑定在一份发布清单中。清单只创建新文件；如归档还包含判题镜像，封存时追加 `--sandbox "$SANDBOX_CONFIG_DIGEST"`。

```sh
python3 -B scripts/trial_release.py pack "$RELEASE_MANIFEST" \
  --revision "$RELEASE_COMMIT" --platform linux/arm64 \
  --app "$APP_BUNDLE_DIR" --app-sha256 "$APP_MANIFEST_SHA256" \
  --runtime "$RUNTIME_ARCHIVE" --runtime-sha256 "$RUNTIME_ARCHIVE_SHA256" \
  --nginx "$NGINX_CONFIG_DIGEST" --postgres "$POSTGRES_CONFIG_DIGEST" --redis "$REDIS_CONFIG_DIGEST"
```

接收端在导入或启动前，用独立可信渠道取得的发布清单 SHA-256 和可信工作树中的脚本重新核验两份制品。保持清单、应用目录和归档静止。

```sh
python3 -B scripts/trial_release.py verify "$RELEASE_MANIFEST" \
  --sha256 "$TRUSTED_RELEASE_SHA256" --app "$RECEIVED_BUNDLE_DIR" --runtime "$RUNTIME_ARCHIVE"
```

校验仅证明两份制品符合可信清单。现场配置、密钥、版本来源审查和试用验收仍分别保留。负载观测中的 `release_sha256` 使用这份发布清单的摘要。

### 从空引擎导入并联验试用制品

在专用原生 Linux 实验 VM 内，先准备无外网路由的网络空间、独立 Docker Unix socket、全新的 Docker 和 containerd 数据目录，并确认 containerd 内容库为空。支持 arm64 和 amd64；发布清单、应用二进制、运行镜像、客体及引擎须使用同一架构，架构不符在导入前拒绝。临时引擎根目录应为 `/tmp/oj-p6-*/engine.*` 下的 root 私有目录，socket 为其中的 `docker.sock`，Docker 数据目录为其 `data/`。应用包由 root 持有并保留权限，传输时不要附带 macOS 的 `._*` 元数据文件。用独立交接的发布摘要执行联合冷启动，应用和镜像使用同一清单：

```sh
bash scripts/smoke-cold-trial.sh --release "$COLD_DOCKER_SOCKET" "$COLD_ENGINE_DIR" \
  "$RECEIVED_BUNDLE_DIR" "$RELEASE_MANIFEST" "$TRUSTED_RELEASE_SHA256" "$RUNTIME_ARCHIVE"
```

验证、导入和启动期间保持制品及目录静止。成功时输出本次联验的发布摘要。也可先单独导入运行镜像，供组件或固定合成判题专项使用：

启动实验用 Docker 和 containerd 时沿用 `umask 022`，并显式将引擎根目录和制品接收目录设为 root 私有（`0700`）。在已测 Ubuntu 环境，若把接收文件时的 `umask 077` 传给这两个进程，导入仍可完成，但镜像中的命令可能无法执行。此时停止实验并检查自建引擎的启动环境，保留已封存制品。

```sh
python3 -B scripts/cold_trial_images.py "$RUNTIME_ARCHIVE" --sha256 "$TRUSTED_ARCHIVE_SHA256" \
  --socket "$COLD_DOCKER_SOCKET" --data-root "$COLD_DOCKER_ROOT" --platform linux/arm64 \
  --nginx "$NGINX_CONFIG_DIGEST" --postgres "$POSTGRES_CONFIG_DIGEST" --redis "$REDIS_CONFIG_DIGEST"
```

接收命令拒绝常用系统 socket、数据目录不匹配或已有镜像、容器、卷的引擎；不清空镜像库。导入后核对完整镜像集合，禁止拉取并验证 Nginx 程序、临时 PostgreSQL 查询及临时 Redis 写读。脚本不创建或停止 Docker 引擎，结束后由实验操作者停止自建引擎；镜像保留在该实验数据目录。此步骤不验证应用与镜像的联合冷启动，也不运行判题镜像。

在**同一隔离接收环境**做联合冷启动时，将前一条命令的 `--platform` 设置为接收引擎对应的架构，把标准输出保存为 `$IMAGE_REPORT`，并以 root 持有收到的应用包（保留权限）。临时引擎根目录应为 `/tmp/oj-p6-*/engine.*` 下的 root 私有目录，Unix socket 为该目录的 `docker.sock`，Docker 数据目录为其 `data/`。在启动独立引擎的同一私有挂载和网络空间中运行：

```sh
bash scripts/smoke-cold-trial.sh "$COLD_DOCKER_SOCKET" "$COLD_ENGINE_DIR" \
  "$RECEIVED_BUNDLE_DIR" "$TRUSTED_MANIFEST_SHA256" "$IMAGE_REPORT"
```

此专项在封闭的回环网络中，从核验过的应用包复制 Nginx 模板，配合临时 PostgreSQL、Redis、测试证书和合成配置验证空库迁移、会话门禁开启、HTTPS 就绪、前端和正常退出。`--release` 还使用包内工具和仅能读取账本的登录核对 schema，并以暂停状态启动入口，验证 HTTPS 读取及解除暂停后恢复转发。夹具不接收外部模板或真实配置，不创建账号、不启动判题。不要在现场引擎运行。

追加固定合成判题仍只支持专用 arm64 档位。先按下文[专用 Ubuntu VM 判题实验](../acceptance/isolated-checks.md#专用-ubuntu-vm-判题实验)用**同一 Git 提交**制作实验判题包。接收端须保证判题包及各级祖先目录由 root 控制。仍在同一个专用 VM 的私有挂载和无外网路由的网络空间、且只导入了试用运行镜像时执行：

```sh
bash scripts/smoke-cold-trial-judge.sh "$COLD_DOCKER_SOCKET" "$COLD_ENGINE_DIR" \
  "$RECEIVED_BUNDLE_DIR" "$TRUSTED_MANIFEST_SHA256" "$IMAGE_REPORT" \
  "$JUDGE_BUNDLE_DIR" "$TRUSTED_JUDGE_MANIFEST_SHA256"
```

脚本在启动服务前核对两份独立摘要及提交号，先验证正式前端和会话 API 的冷启动与退出，再在同一引擎导入固定判题镜像，使用独立临时库运行合成 HTTP 判题和过载恢复。判题入口是实验接口；没有把普通账号的训练提交接入这次验收，也没有用实际试用配置。

### 试用同源 HTTPS 入口

用[独立 Nginx 模板](../../deploy/trial/nginx.conf.example)准备试用入口。将 `oj.example.invalid` 的所有出现处替换为同一个公开域名；本模板对应公开端口 443，映射至 Nginx 的 8443。把已核验应用制品的 `site/` 以只读方式挂载至 `/srv/oj/site`；应让 Nginx 运行账号读取安装后的文件，同时保持原接收包不变。

- Nginx 与会话 API 共用回环网络空间，API 只监听 `127.0.0.1:9443`，不对客户端暴露。若采用不同网络空间或在 Nginx 前再加代理，先为实际拓扑重新核对隔离和来源地址。
- Nginx 使用公开证书和独立的代理客户端证书；后端 API 服务端证书的 DNS SAN 为 `oj-api.internal`。按模板挂载公开证书链、验证 API 的 CA 以及代理证书链与私钥，证书和私钥由维护者另行安装。
- 会话 API 设 `mode: "proxy"`，`origin` 为对应的公开 HTTPS Origin，配置验证代理客户端证书的 CA，并在 `proxy_fingerprints` 登记代理客户端叶证书 DER 的 SHA-256。不要用浏览器证书、CA 指纹或 Worker 证书代替。详见[单跳受信反代](../architecture/session-design.md#714-单跳受信反向代理)。

切换前先用安装后的配置执行 `nginx -t`，再从信任公开 CA 的干净客户端检查静态资源、`/api/v1/readyz`、登录及退出；错误公开 Host 和错误后端证书必须被拒绝。模板关闭访问日志以免记录敏感请求，现场的值守采样与告警另行配置。此模板不改变默认 Compose，也不配置校内 DNS、证书分发或发布服务。

#### 试用入口暂停写入

将专用维护目录只读挂载到 Nginx 的 `/run/oj-maintenance`，确保 Nginx 用户可遍历，由值守人独立控制。正常运行时不放标记；维护开始后，在宿主维护目录创建普通文件 `paused`，退出维护时仅移除此标记。挂载整个目录，切换不需重启或重载 Nginx。切换后从公开路径确认非 GET/HEAD 的 `/api/` 请求返回 `503`、`TRIAL_MAINTENANCE` 和 `Cache-Control: no-store`，现有查询、静态页面及就绪检查仍按原规则处理。

未挂载目录或 Nginx 无法读取标记时不能用此开关停写，必须以实际探针确认。标记不取消已转发请求，也不阻止绕开该入口的 API、数据库维护、邮件及 Worker 操作；[试用值守手册](p6-operator-runbook.md#发布排空和兼容回退)另行要求停止相关写进程并确认排空。

本地可复跑真实 API 与 Nginx 联验。准备普通会话专项的工具链、Redis 镜像和模块缓存，另用本地非 root Nginx 镜像及导出的正式 `site/`。宿主机需要已有的 `openssl`：

```sh
python3 -B scripts/test-session-protocol.py --suite trial-edge \
  --toolchain-image oj-session-protocol-toolchain --build-cache-volume '' \
  --nginx-image "$LOCAL_NGINX_IMAGE" --trial-site "$TRIAL_SITE"
```

专项用临时 CA、合成账号、真实 PostgreSQL／Redis 和独立 API 进程；Nginx 和 API 共用私有网络空间，不发布宿主端口。Go HTTPS 客户端验证信任链、静态入口、就绪、两步登录、转发头覆盖、Cookie、维护期间写入拒绝与会话可读，以及恢复后退出使旧会话失效。此专项不替代现场浏览器、DNS 或独立故障域验收。

### 核对 P6 混合训练记录

从主库独立核对收件全集：停止并排空本轮提交入口的在途写入后，用预登记的 UTC 起止时间运行以下命令。使用只读专用账号，通过 `OJ_JUDGE_SNAPSHOT_DATABASE_URL` 指定主库。

```sh
.local/bin/migrate training-acceptance "$EXPECTED_DATABASE" "$ROUND_FROM_UTC" "$ROUND_UNTIL_UTC" > "$ACCEPTANCE_OBSERVATION"
```

该命令只需要读取 `goose_db_version` 的 `id`、`version_id` 以及 `submissions` 的 `id`、`created_at`、`material_version`、`context_id`。它输出封闭时间窗 `[from, until)` 的全部个人训练收件，最多 20,000 条、跨度最多一小时；超限、窗口尚未结束、库名不符或处于恢复中的副本均拒绝。先检查命令退出码，再保存原始 JSON 与摘要。命令不接收客户端提交 ID 列表，不导出账号、原键、源码或凭据；不替代队列、终态和 HTTPS 回读证据。

将三轮脱敏逐请求观测按[混合训练记录约定](../acceptance/delivery-evidence/p6-trial-acceptance.md#混合训练记录核对)保存后运行：

```sh
python3 -B scripts/assess-p6-mixed-load.py "$MIXED_LOAD_RECORD" \
  --acceptance "$ROUND_1_ACCEPTANCE" "$ROUND_2_ACCEPTANCE" "$ROUND_3_ACCEPTANCE"
```

命令只读核对已记录的负载、时延和接收对账，不发请求。现场记录必须按轮次顺序提供三份原始主库清单；报告保留各清单摘要和对账差异数。本地旧记录可省略清单，报告显示 `independent_acceptance: null`。本地合成记录通过不代表试用容量达标。

### 核对 P6 过载恢复记录

本地可先运行 `python3 scripts/test-session-protocol.py --suite training-overload`。它在新建临时 PostgreSQL、Redis 和普通会话 HTTPS API 中用四个合成账号做三轮：全局额度包含一笔排队和一笔运行任务，新账号受控拒绝，原键保持同一回执；排空后重试被拒绝的原键并接收另一个新键，通过真实 HTTPS 读回四个终态。入队使用存储边界，终态由夹具受控上报；正式提交 HTTP、真实 Worker 及现场资源须另验。

按[过载恢复记录约定](../acceptance/delivery-evidence/p6-trial-acceptance.md#过载恢复记录核对)留存三轮的客户端记录和独立采集的服务端账本：

```sh
python3 -B scripts/assess-p6-overload.py "$OVERLOAD_CLIENT_RECORD" "$OVERLOAD_SERVER_LEDGER" \
  --acceptance "$ROUND_1_ACCEPTANCE" "$ROUND_2_ACCEPTANCE" "$ROUND_3_ACCEPTANCE"
```

现场核对须提供三份与过载轮次对应的原始主库收件清单。命令用提交 ID 和接收时间核对账本，再检查队列打满、拒绝未写入、原键重放及恢复接收。本地旧记录可省略原始清单，此时报告不含独立收件对账。命令不发送流量，也不替代对原始记录和现场环境的复核。
