# 本机工程检查

从仓库根目录运行本文命令。检查范围和门禁见[工程检查](engineering-checks.md)与 [CI/CD](ci-cd.md)。

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

后端包含格式、vet、golangci-lint、race、全部命令构建；前端包含 lint、类型、测试、构建。Go 静态分析覆盖主模块、sandbox 标签测试源码及独立会话实验模块，并先验证检测器能接受正确样例、拒绝四类错误；不执行 sandbox 选手代码。规则与边界见[代码质量检查](engineering-checks.md#代码质量)。SQL 生成检查在容器内比较submission/identity/problem/problemreference四包生成前后内容，新增包为`internal/problemreference/postgres/referencedb`；数据库检查使用容器内临时 PostgreSQL 14，覆盖迁移、并发租约、持久化参考申请及两个真实 API/mTLS 控制端。申请服务组合测试使用合成会话观察，不代表该能力的真实Redis/HTTP、Worker消费或大材料验收。工具镜像不挂载 Docker socket，不访问用户数据库。测试每次运行，不以构建缓存替代测试。

测试专用会话 Store 成本采样使用 `python3 scripts/test-session-cost.py --module-cache <已校验缓存目录> --output <新JSON路径>`，只用已有本地镜像，独立私有 TCP 容器中比较启用相同授权守卫的路径；不是 API 跑分或容量准入。配置、采样窗口和持久化差异见[会话实验](../../tests/sessionexperiment/README.md#同等普通会话合同的-store-成本采样)，正确性仍执行原行为/race专项。

`toolchain db` 也运行[数据库与材料恢复回归](../architecture/storage-design.md#7-备份与验收)，使用两个容器内临时 PostgreSQL 集群和合成文件，无需访问现有数据库或运行选手程序。

查看 PR 的 CI 耗时、取消数和重试次数：`python3 -B scripts/measure-pr-ci.py <PR号>`。统计口径见 [CI/CD](ci-cd.md#衡量-pr-等待与重跑)。

训练队列的数据库负载基线另行手动运行：`docker --context desktop-linux compose run --build --rm toolchain training-load`。它在独立临时 PostgreSQL 上跑三轮合成负载，报告限额拒绝、领取顺序和延迟样本。使用已构建镜像时可用 `docker run --rm --network none --cpus=4 --memory=4g --memory-swap=4g <toolchain镜像> training-load` 固定容器预算；默认 Compose 命令不设置这些限额。该命令不启动业务服务或执行选手程序，见[采样口径](../architecture/submission-backlog.md#训练准入负载采样)。

普通 Redis 协议专项（只执行合成账号数据）：

```sh
docker --context desktop-linux build --target toolchain --tag oj-session-protocol-toolchain .
docker --context desktop-linux pull redis:7.2.17@sha256:0312984fbfeb2ff1d4382ecef45b50e7d65478a6a58cf12ce28d6de7a4142e45
python3 -B scripts/test-session-protocol.py --toolchain-image oj-session-protocol-toolchain --build-cache-volume ''
```

脚本只使用已有本地镜像，检查 Linux/架构，创建带唯一 owner 标签的私有 internal 网络、旧/替换/中断恢复 Redis 和工具链容器；无宿主发布端口或 Docker socket。临时 PostgreSQL 只开放容器内 Unix socket，复制源码白名单并运行真实存储/race 专项。结束检查退出码/OOM 并仅清理自有容器和网络，保留缓存卷。可用 `--redis-image` 指定已核验本地镜像、`--module-cache` 挂载既有只读模块缓存；合并缓存后执行 `go mod verify`，测试使用 `-mod=readonly`。Linux CI 使用 `--context default`；此专项不是完整 AC23、容量或部署验收。

在上述脚本命令末尾加 `--suite email-lifecycle` 可用另一套全新的临时 PostgreSQL 和 Redis 验证邮箱整链路。专项以配置文件启动两个 API 进程和独立发信进程，邮件只送往容器内回环 TLS 收件器，不发布宿主端口。测试驱动在 Go race 测试计时开始前构建所需入口，保持运行期超时不变。

`--suite archive-recovery` 单独运行同一归档的会话及邮件恢复回归，使用旧、替换及中断恢复 Redis 夹具。该用例也保留在普通会话套件中。

`--suite email-archive-recovery` 另起全新夹具，只还原应用库，保留独立发信额度库。真实邮件进程处理恢复后的队列：旧任务不消耗 SMTP 许可，新邮件到达回环 TLS 收件器后可在另一 API 完成激活。此命令不连接外部邮件服务。
