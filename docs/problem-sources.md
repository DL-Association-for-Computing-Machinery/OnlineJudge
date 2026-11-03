# 首批题源与私有候选材料

用户于 2026-11-03 确认：Library Checker 起步，官方 ICPC 赛事包扩充，爬虫作为补充。首个录入方向为外部材料适配到统一内部模型；已实现离线题源准备及[统一私有草稿适配](problem-drafts.md)，尚未开放题库接口、发布或训练提交。正式业务导入继续满足[题包设计](architecture/problem-package-design.md)的权限、受限验证、审核和不可变版本要求。

## 固定来源与范围

官方仓库 [Library Checker Problems](https://github.com/yosupo06/library-checker-problems)，固定提交 `73876f1cbcf082d2b22722924b924d3d65a2f00e`。[锁文件](../config/problem-sources/library-checker.json)记录 162 份必要原始文件的 SHA-256 和大小；未锁定的文件不进入执行快照。中文标题仅为本站目录别名，原题面模板和英文标题保留。

| 来源题号 | 中文目录名 | 完整测试点数 |
| --- | --- | --- |
| aplusb | A + B | 12 |
| unionfind | 并查集 | 18 |
| associative_array | 关联数组 | 20 |
| static_range_sum | 静态区间和 | 11 |
| point_add_range_sum | 单点加与区间求和 | 21 |
| staticrmq | 静态区间最小值 | 27 |
| zalgorithm | Z 算法 | 29 |
| suffixarray | 后缀数组 | 50 |
| primality_test | 素性测试 | 13 |
| convolution_mod | 卷积 | 53 |

合计 10 题、254 点。测试点顺序取原 `info.toml` 的分组顺序和序号；仅 `example.in` 分组标为公开样例，其他静态 `.in` 仍是正式数据。来源标识为 `library-checker:<题号>`，不作为覆盖本站题目或授权发布的凭据。

仓库根许可为 Apache-2.0，但打包必须同时保留 `LICENSE`、checker 源码内 MIT notice 和 `common/testlib.h` 原始 notice。该提交内 Testlib 是 `0.9.40-SNAPSHOT`；这不等于项目生产 SPJ 库版本已选定或准入。十个 checker 均为大小写敏感 token 比较，不能直接替换为 M0 默认大小写不敏感比较器。

## 输出与验证边界

候选目录为 0700、材料文件为 0600，包含 `source/` 原始材料与许可、`problems/<题号>/{in,out}/` 完整生成数据、`validation.json` 执行记录和 `catalog.json` 目录及来源信息。原始模板含 `@{...}` 指令，尚未转换、中文翻译或验证 Markdown/公式显示。

生成、标程、输入 validator 和原 checker 只在固定本地镜像的非特权容器执行：断网、UID 65532、只读根文件系统、无 capabilities、禁止新增权限、只读输入及驱动挂载；无 Docker socket 或宿主输出写挂载。CPU 2、内存 4 GiB（含 tmpfs）、进程 128，工作 tmpfs 3 GiB、临时 tmpfs 128 MiB；子进程 120 秒、单文件 128 MiB、整轮生成验证 3600 秒，导出 180 秒/2 GiB 数据预算。这些是题源准备工具预算，不是生产题目运行限制。

工具使用未修改的上游生成器，通过窄接口把 `toml.load(path)` 接到 Python 标准库 `tomllib`，不安装 Python 包。采用生成器 DEFAULT 模式并核对原始 `hash.json`，再显式对全部 254 点运行输入 validator 和 `checker(input, answer, answer)`；每题首个样例另用错误 token 验证返回 WA（退出 1）。不使用上游 TEST 模式的退出 0 冒充全部解答验证：该脚本对部分解答异常只记录日志。

生成完成后将 USTAR 归档传到宿主私有临时文件，先终止并确认清理本任务容器，再由宿主按锁内材料和 `hash.json` 独立检查精确文件集合、字节哈希、类型与预算。解包拒绝越界、链接、稀疏/PAX、未知成员、重复及大小写冲突；不调用 `extractall`。Docker 的 `cp` 在本次环境看不到容器 tmpfs，故使用受限 `docker exec tar`；不通过增加宿主可写挂载解决。

源码与数据哈希属于宿主独立验证；validator/checker 执行记录依赖固定源码审阅，不是对同 UID 恶意程序的独立证明。只支持本锁内已审阅来源，不是任意上传程序执行服务。候选固定为 `private-unpublished`，不会写业务数据库或自动发布。失败、取消、导出或清理失败保留 `INCOMPLETE`；部分文件不能视为可导入完成包，重试须指定新输出目录。

## 后续接入

1. P4 已增加[统一私有草稿适配](problem-drafts.md)：静态转换英文模板和18个样例，重新核对全部来源/254点哈希，自含对象和内容版本可搬迁复验。业务持久化、权限、渲染/公式兼容、本站受限验证、审核和不可变发布仍待完成；P3权限和生产检查器准入满足后才接训练页面。当前Worker的M0固定材料门禁保持。
2. 用 [NWERC 2024 官方 Packaged Problems](https://2024.nwerc.eu/) 和 [BAPC 2023 官方数据/解答](https://2023.bapc.eu/) 扩充赛事题源。先核对逐题许可、实际旧格式及不支持特性，再形成兼容矩阵；本轮未下载或执行这些包，不假定其采用 2025-09。
3. 中文基础题按目标站和授权增加爬虫适配器。题面/样例抓取与完整本地判题材料、远程评测是不同能力，共用内部模型而不绕过发布验证。

复现命令归 [README](../README.md#library-checker-首批题源准备)，实际结果和失败记录归[交付日志](delivery-log.md)。
