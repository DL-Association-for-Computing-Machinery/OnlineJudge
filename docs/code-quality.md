# 代码质量检查

命令沿用 README 的 `toolchain backend` 与 `frontend-check`，本机使用 Docker Desktop `desktop-linux`；CI 使用同一容器入口。静态分析不能替代行为测试、真实数据库/Worker 联调或业务验收。

## Go

golangci-lint 固定 v2.15.0，配置位于 `.golangci.yml`，仅启用 staticcheck、errcheck、ineffassign、unused。既有 gofmt、vet、race 和构建继续保留；不全量开启风格检查，不使用仅检查新增告警或全局排除来隐藏基线。

`scripts/check-go.sh` 依次检查主模块、sandbox 标签测试源码、`tests/sessionexperiment` 独立模块。主模块 `./...` 不会自动进入嵌套模块；sandbox 静态分析不运行判题程序。当前平台是容器 Linux，不宣称覆盖所有 OS/build tags。

每次执行先用临时独立模块运行真实检测器：正确代码必须成功，四类错误必须返回告警退出码且分别出现对应规则；工具错误不能冒充成功拒绝。测试仅使用标准库，不执行样例代码。告警数量不截断，模块只读检查避免悄悄修改依赖。

已有资源清理保留原有行为，并明确忽略理由；影响行为的返回值应检查，不能为了 lint 机械添加空白赋值。文件写入/同步、租约与授权错误继续由行为测试验证。测试故障注入中客户端主动中断的响应写入允许显式忽略返回值。

## 前端

前端沿用已有锁定的 ESLint/typescript-eslint，无新增依赖。`src/**/*.{ts,tsx}` 和 `vite.config.ts` 使用 `recommendedTypeChecked`、`projectService` 及固定配置目录，包含未处理 Promise、Promise 回调误用与不安全 any 数据流检查；JS 配置/工具脚本沿用普通 lint，不要求加入 TypeScript 项目。保留 React Hooks、refresh、零警告及原有 typecheck/Vitest/build。

`pnpm lint` 先扫描真实源码，再通过 Node 测试运行器使用实际 ESLint 配置进行六项正反例。内存样例使用已有 tsconfig 成员路径，不落盘、不进入构建；覆盖 TS、TSX 事件回调和 Vite 配置，解析错误不能冒充目标规则拒绝。自测脚本命名 `lint-selftest.mjs`，避免被 Vitest 的 `*.test.*` 自动发现而混用测试运行器。

类型感知分析增加 TypeScript 项目分析成本，不能替代运行时外部数据校验。`void promise` 只表示显式放弃等待，不处理拒绝；业务入口仍需 catch/错误状态。前端现有健康检查已将响应作为 unknown 并校验契约，事件处理器已有 try/catch，本次不改 UI 或业务源码。官方依据：[typed linting](https://typescript-eslint.io/getting-started/typed-linting/)。

## Go 工具准入

2026-10-29 核对 [v2.15.0](https://github.com/golangci/golangci-lint/releases/tag/v2.15.0)：要求 Go 1.26.0，使用项目 Go 1.26.10 构建；主项目许可证 GPL-3.0，作为独立开发 CLI 使用，不链接到应用，也不进入运行镜像。若对外分发开发镜像，应履行适用许可义务。公开 GitHub advisory 查询为空，不代表工具或传递依赖无漏洞。

工具通过固定 Go 模块版本和校验和下载，额外成本是首次构建下载、开发镜像空间及每次分析时间；不上传源码到第三方质量服务。后续升级须验证配置、有效/无效样例、源码基线和 CI，并重新核对许可、安全与兼容性。官方规则见 [linters](https://golangci-lint.run/docs/linters/)。
