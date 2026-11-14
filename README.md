# OnlineJudge

面向东华理工大学计算机协会的训练与校赛平台。

## 快速开始

### 本地 HTTPS 账号环境（一键）

需要在页面上实际登录时，在仓库根目录运行：

```sh
python3 scripts/dev-up.py
```

打开 [本地 HTTPS 登录页](https://localhost:8443/login)，点击“一键进入测试账号”，或使用私有初始化凭据走普通表单登录。首次访问需确认本地证书，或自行信任脚本导出的公开 `.local/dev-auth/ca.crt`；脚本不更改系统信任。账号、密钥、PostgreSQL 和 Redis 数据持久保留，重复运行不重设密码。完整边界、停止方式与验证见[本地开发指南](docs/operations/local-development.md)。此独立项目不替代生产部署，也不影响下面的 HTTP 骨架预览。

### Docker 构建与一键启动（默认）

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

当前只启动前端与基础 API，健康检查只证明进程/代理连通；不启动数据库、Worker、控制端或判题沙箱，不代表完整 OJ 可用。应用容器使用非 root、只读根文件系统；Nginx 临时文件只写入内存目录。当前默认服务没有持久化业务数据或 volume；后续增加数据库必须同步配置持久化与迁移步骤，不使用 `down -v` 清理数据。

Dockerfile 的 Go、Node、pnpm 与现有配置保持一致，基础镜像固定多架构索引摘要；依赖仍使用 `go.sum` 和 `pnpm-lock.yaml`。`.dockerignore` 限定构建上下文，排除本地 Git、缓存、环境文件和常见凭据文件。更换工具链时需同步 Dockerfile 和对应锁定入口，并重新验证容器检查及启动流程。

## 文档入口

- [文档总览](docs/README.md)：按任务和功能查找。
- [开发](docs/development/README.md)：需求、计划、分支、PR 与 CI。
- [架构](docs/architecture/README.md)：系统、题库、判题、会话和存储。
- [运维](docs/operations/README.md)：环境配置、题库维护、备份与恢复。
- [验收](docs/acceptance/README.md)：隔离专项、实测证据和历史交付。

阶段进度以[开发与交付计划](docs/development/phase-1-development-plan.md)为准，实际通过范围见[验收记录](docs/acceptance/README.md)。
