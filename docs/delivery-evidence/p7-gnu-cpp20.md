# P7 GNU++20 本机验收

在已准入并发布的本机文本题中选择 GNU++20，沿用独立语言草稿、单文件 `.cpp/.cc/.cxx` 上传、本人提交详情和原键回执。未传语言时仍为 GNU++17；语言纳入原请求指纹，同键换语言返回 409。M0 合成入口仍只支持 GNU++17。

Worker 根据租约对应提交的语言，从固定运行环境中派生 `g++ -std=gnu++20 -O2 -pipe main.cpp -o main`。C17 与 GNU++17 档位、冻结测试材料、参考标程、检查器、执行预算和判题隔离保持原有约定。旧 Worker 不能处理 GNU++20，升级前应排空并停止旧 API、Worker 和维护写入。

schema 31 扩展语言约束。没有 GNU++20 提交时可回到 schema 30；已有 GNU++20 提交时，回退失败并保留数据。已有 C17 提交可回到 schema 30，仍不能继续回到 schema 29。

2026-11-15 在本机专用 Ubuntu arm64 VM（内核 6.8.0-106-generic）使用一次性 PostgreSQL、Redis 和回环 HTTPS。独立审核、发布角色把冻结的 A+B 发布到与前轮相同的版本 `fff51a78441424bbd2e646e839f81490826c63ad588e3b98edc19eb8d679d4c1`。普通账号在 Chrome 上传使用 `std::integral` concept 的源码，由真实 mTLS Worker（GCC 15.3.0、go-judge v1.13.3）判出 GNU++20 AC，错解 WA、非法源码 CE；同一份 concept 源码用 GNU++17 判为 CE。C17 回归 AC、WA、CE，普通 GNU++17 正解回归 AC；省略语言的旧请求重放返回同一 GNU++17 收件记录。

提交者逐笔读回原源码与判定，其他账号不能读取详情或原键回执。库中恰有对应八笔终态，均仅一次真实判题尝试。浏览器报告 SHA-256：GNU++20 `49e2c88a211051caeec792895de94c7bacbb7e227916d18945ac8ebde43c1d19`、C17 `b55a8f38b64e34a293ac7b3102381db111debf6448a5689764b864b16a6ce71a`、GNU++17 `afb303e5925cd0a3da557ecfed56d8da25a3ec3018b9646a836c91b8a7168deb`。数据库核对 `f3a06bbf91c187517a5d5dd614daf4089f06cfc8a8421ae734923f680c0b0280`。

本轮只验 GNU++20 发布题提交、编译档位、判定和本人回读；其超限、故障、历史重判与同机双 Worker 一致性仍待补齐。未部署、未访问试用数据。
