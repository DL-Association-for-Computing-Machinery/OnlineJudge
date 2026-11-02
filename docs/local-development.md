# 本地 HTTPS 与账号开发环境

此入口把 HTTPS、真实 PostgreSQL/Redis 账号服务、模式迁移和本地测试账号初始化放在一个独立 Docker Compose 项目 `oj-local-dev` 内。宿主只需 Python 3、Docker 与 Compose v2；无需安装 Go、Node、pnpm、OpenSSL 或数据库。macOS 先启动 Docker Desktop：

```sh
python3 scripts/dev-up.py
```

Linux 使用当前 Docker context；也可显式指定，例如 `python3 scripts/dev-up.py --context default`。首次运行会构建固定工具链镜像，耗时取决于本地缓存。启动脚本只操作 `compose.dev.yaml` 的固定项目，不停止或重置原 `compose.yaml` 环境。

启动成功后打开 [本地登录页](https://localhost:8443/login)。点击“一键进入测试账号”，即可使用普通真实会话；仍可退出后通过邮箱/密码表单登录。该入口不包含注册、邮件找回、生产账号导入或完整训练与判题链路。

## 本地证书

首次初始化在 Docker 私有 volume 中生成本地 CA、`localhost` 服务证书和私钥。浏览器首次访问会提示证书不受信任；可在浏览器中确认继续，或自行把 `.local/dev-auth/ca.crt` 作为本地 CA 信任。证书匹配固定入口 `https://localhost:8443`，请使用该地址访问。

启动脚本只导出公开 CA，不自动修改系统或浏览器信任，不导出私钥。容器健康检查在自身 loopback 上跳过证书验证，仅验证 HTTPS 进程就绪，不代表浏览器已信任证书。`.local/` 被 Git 和 Docker 构建上下文忽略。

脚本还从宿主机使用导出的 CA 验证实际 HTTPS 入口，确认回环端口与证书链正确后才报告成功。服务证书有效期三个月；过期或材料不一致时拒绝启动，不自动更换信任材料。后续证书更新需单独维护，不能靠删除账号或 Redis 数据解决。

## 状态与重新启动

PostgreSQL、Redis AOF 和初始化状态分别保存在项目专用 volume。首次初始化创建数据库模式、随机账号、登录密钥及就绪标记；后续启动验证既有状态，无法证明状态一致时停止，不自动补发账号、替换密钥或清除限流状态。

初始化只接受空状态目录、空数据库和空 Redis。首次过程中断而留下部分状态时会失败关闭，需要保留现场后显式处理；重复执行不会盲目续写。已完成安装若丢失任一存储、marker 或材料，也不会自动重建来掩盖数据丢失。

再次运行 `python3 scripts/dev-up.py` 会重建应用并保留持久数据。账号凭据仅存储在私有状态 volume 的 `/state/account.json`，脚本和启动日志不会显示密码；如需手动输入，维护者可自行在本机检查该文件，勿粘贴到日志、聊天或提交。不要把该账号用于其他环境。

测试邮箱为 `developer@example.invalid`，密码随机生成。主动改密后，文件中的旧测试密码会被正常拒绝，一键入口也会失效；可使用新密码正常登录。重启不会覆盖改密结果。普通会话仍遵循原有空闲/绝对期限；此通道提供快速重新登录，不是延长会话或关闭鉴权。

只停止本环境并保留数据：

```sh
docker --context desktop-linux compose -p oj-local-dev -f compose.dev.yaml stop
```

查看状态：

```sh
docker --context desktop-linux compose -p oj-local-dev -f compose.dev.yaml ps -a
```

Linux 可省略 `--context desktop-linux`，或换成启动时选择的 context。脚本不执行 `down`、删除 volume 或重置数据库。停止前仍应保存浏览器中的未提交工作。

## 隔离与故障定位

只发布 `127.0.0.1:8443`；PostgreSQL、Redis 和初始化进程只在独立内部网络，数据服务无宿主端口。HTTPS 网关额外接入口 bridge 以便 Docker Desktop 正确发布端口，具备普通 bridge 出站能力，不声称完全断网。数据库 trust 与 Redis 无密码仅用于此专用隔离开发环境，不适用于生产或共享网络。服务证书私钥、随机登录密钥和账号文件由非 root 应用用户持有；短暂的卷准备容器仅调整状态目录归属，初始化与 HTTPS 进程以 UID 65532 运行。

若启动失败，先检查 Docker Engine/context、镜像构建、8443 端口占用及 `ps -a` 中的健康状态。脚本保留既有数据，不擅自删除或重新初始化；初始化拒绝不一致状态时，应先保留现场并排查原因。不要为消除错误直接删除就绪标记、Redis 数据或密钥文件。

此环境是本机开发入口，不代表生产 TLS、备份恢复、容量、完整 M0 或业务验收通过。默认 HTTP 骨架环境仍由 [README](../README.md) 中的原 Compose 命令启动。

## 自动验证

```sh
python3 scripts/test-local-development.py
```

该专项创建全新 UUID Compose 项目，只启动自有存储和初始化容器，不占用 8443、不接触 `oj-local-dev`。验证首次初始化、重复初始化材料保持一致、测试 Redis 丢失 marker 后拒绝启动且不补回数据；核对资源归属后只清理该轮自有测试卷、容器和网络。CI containers 门禁同时执行这一专项和原 Compose 冒烟。AOF 与 marker 不能单独证明旧快照/部分回滚后的完整连续性；生产恢复规则保持独立。
