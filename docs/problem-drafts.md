# 统一私有题目草稿与 Library Checker 适配

首个来源适配器将[固定十题候选](problem-sources.md)转换为自含的 `oj.private-draft-bundle.v1`。只接受仓库固定锁对应的已准备材料，不接受任意目录、ICPC ZIP或用户上传源码。离线工具只生成bundle；后续[私有草稿应用服务](#私有草稿入库与本人读取)接通材料与数据库，不执行程序、不发布题目。运行与复验命令统一归[README](../README.md#转换为统一私有草稿)。

## 目录与合同

成功产物在输出目录的 `bundle/`，其中文件0600、目录0700；全体为私有存储，样例角色不是当前公开授权。`index.json` 列出各草稿的路径、版本及全部其他文件的大小和SHA-256；它用于完整性检查，不是可信签名、来源执行证明或发布许可。

| 内容 | 路径与职责 |
| --- | --- |
| 内部草稿 | `drafts/<来源题号>.json`，schema为`oj.problem-draft.v1`，state固定`private-draft`；文件名只便于定位，不是本站稳定题目ID |
| 转换题面 | `statements/<来源题号>.md`，英语原文、公式和替换参数，样例嵌入原位置；中文标题是既有目录别名，不声称全文翻译 |
| 材料对象 | `objects/sha256/<哈希>`；独立复制而非硬链接，去重只发生在本次新bundle内；原始源码、许可、全部输入/答案都在其中 |
| 来源证据 | `provenance/source-lock.json`、`source-catalog.json`、`source-validation.json`保留原始字节；`material-map.json`将原始相对文件名映射到对象 |

内部草稿包括题名、带语言/哈希/大小的题面引用、顺序固定的`test_cases`、有序`examples`、判题配置、来源映射、适配器语义版本及验证状态。所有功能性文件引用相对bundle根，含`path/sha256/size`；无原输入绝对路径依赖。测试点内部ID为`case-0001`等，原分组/序号仅保留在来源映射；不按目录排序重新排列。只有原`example.in`分组的18点标为sample，其他236点、原始题包、checker、validator和标程保持私有；未来服务必须按逻辑角色和权限读取，猜中对象哈希不授予权限。

`version`是移除该字段后整个草稿的规范JSON（UTF-8、键排序、紧凑分隔符、末尾换行、不接受非有限值）的SHA-256。绑定题面、有序测试列表、样例角色、判题语义及该题和共用原始源码/许可哈希。运行时间、输出绝对路径、源报告的耗时不进入题目版本；来源报告原字节另受bundle清单保护。此版本是草稿内容标识，尚不是数据库发布版本或审核记录。变更生成新输出，不覆盖已存在草稿；CLI拒绝已有、重叠或仓库内输出。

## 模板与语义映射

固定模板词汇仅支持成对`lang.en/ja/end`、五种`keyword`、整数`param`及公开`example`引用。保留英文分支；即使在未选择的日语分支，未知/畸形指令也拒绝。参数缺失、语言块未闭合/嵌套、重复/遗漏样例或引用正式数据均拒绝。样例在Markdown中使用足够长的代码围栏；无末尾换行时仅为展示闭合添加换行，独立样例对象始终保留原始字节。

题面转换不证明Markdown/KaTeX渲染通过，不执行模板表达式或把原始HTML认作已获准公开内容。原始`info.toml`、`task.md`和其他锁定源码完整保留；来源的solutions/forum/生成配置只作为来源信息，不悄悄成为本站执行配置。

十题配置为大小写敏感的精确token字节比较，明确关闭数字等价（`01`与`1`不同）；不继承M0默认忽略ASCII大小写。`compatibility=configured-not-validated`，来源checker与本站检查器的空白/编码/预算和故障语义仍需专项对照。上游timelimit只记录在来源字段，本站`limits`与`production_profile`保持null，不能用题源准备容器的预算充当选手限制。

## 校验、失败与下一步

先把锁内源码复制到私有已核验元数据快照，再推导测试集合和顺序；每个实际流式复制的数据文件在同一安全打开的描述符内计算哈希。拒绝路径别名、目录/文件链接、FIFO、未知/遗漏文件、来源锁或报告不匹配，保持每文件128MiB、整体2GiB预算。输入须是受信维护者控制的稳定私有快照；不是针对同UID恶意修改者的通用上传服务。

原子创建新输出后先写`INCOMPLETE`，只在全部文件、草稿版本和引用独立复验通过后，将`bundle.partial`改名为`bundle`并移除标记。磁盘错误、中断或校验失败保留标记及部分成果，不修改输入；不保证掉电持久性或自动清理部分文件。命令行复验拒绝带未完成标记的输出，可独立检查搬迁后的bundle；校验成功始终返回publication blocked。

源报告仅记录题源准备阶段已执行的验证，不将其当作本站运行证据。草稿的本站验证固定`not-run`，发布继续阻断于完整题面质量、生产检查器与资源准入、本站独立受限验证、获授权非作者终审及原子发布。下方应用服务及可选HTTP已接入[本地材料存储](material-store.md)、数据库固定引用、最小出题权限及导入管理页；后续仍须完成图片授权、受限验证和审核发布。P3/P5接入真实训练授权及页面，当前Worker的固定M0材料门禁保持。

## 私有草稿入库与本人读取

`internal/problem.Service`提供`Create`和`GetOwn`，`Importer`在相同授权预检后从冻结候选目录选择Prepared。创建接收普通`redisstore.Credential`、owner范围的16—128位ASCII请求键及已准备草稿；不接收客户端指定的主体、认证代次、恢复代次或权限。登录和账号启用不自动赋予出题权；identity拥有独立默认缺省的`problem_author`状态、修订与维护审计。现有/本地开发账号均不自动授予，维护CAS及命令归README。作者撤权后，读取本人旧草稿及恢复原回执也拒绝；其他作者与未知ID均为同一`ErrNotFound`。

可选[HTTP合同](../api/problem-drafts.openapi.yaml)提供导入、本人元数据及题面/样例预览，并增加下节的管理页绑定导入协议；启用方式归[README](../README.md#显式启用私有草稿-http)。沿用普通会话TLS/Host、固定单跳mTLS代理、严格Cookie及POST正向同源检查，所有响应no-store/nosniff，不续期或写Cookie。既有`POST /problem-drafts`仍只接受source_id/request_key两个精确字段，最多1024字节，拒绝重复/未知/null/大小写别名/坏Unicode/尾随数据；不接受路径、客户端材料或主体。它保留选择当前候选的兼容行为，不绑定客户端选定修订；管理页必须使用下节新端点，不能在失败时回退旧POST。候选存在性只在会话和角色预检通过后可观察。旧端点返回独立JSON DTO，document为JSON对象而非base64；逻辑引用不是材料下载授权。

每实例显式材料活动预算由导入和预览共享，无等待，作用于初次授权后的材料/最终授权阶段；不限制初次预检、普通元数据GET或HTTP输出，不代替共享频率/磁盘额度。材料异常、容量不足、未知提交、取消和存储失败统一503；不能将Prepared文件异常的ErrInvalid变为400或选手错误。服务调用成功后仍检查请求取消。未知回执须用原键和精确候选快照核验；已占用原键的不同导入内容为409，旧两字段端点不会替客户端锁定修订，维护变更须保留旧快照处理未确认操作。Runtime跟踪整个同步调用，排空后才关闭全部Prepared、Local及PG/Redis；启动部分失败也释放已打开资源。HTTP关闭不删除已有数据；不新增schema或依赖。

### 管理页导入协议

[绑定导入合同](../api/problem-draft-imports.openapi.yaml)补齐选题与未知回执恢复的数据边界；[管理页](frontend-design.md#私有草稿导入管理)使用下述主体绑定命名空间，并提供独立的本人列表。`GET /api/v1/problem-draft-sources`在当前作者预检后返回按source_id排序的1—10份`source_id/title/version/import_revision`，末尾再次观察同一凭据、比较完整actor并检查PG角色/取消；失败不返回部分目录。摘要在启动时准入并固定，不访问大材料，不是材料健康检查，也不暴露路径/材料引用。

`version`只标识题目文档；`import_revision`为`Prepared.RequestSHA256`，覆盖文档摘要和全部排序逻辑引用，包含index及来源报告。证据或其他引用变化时，题目version可以不变，但完整导入修订必须改变。因此`POST /api/v1/problem-draft-imports`严格要求`source_id/request_key/expected_revision`三字段JSON，仍至多1024字节；`expected_revision`为选中候选的64位小写hex完整修订，不是document version。授权后先比较修订，再占用原导入/预览材料槽和执行任何材料I/O；不匹配为409 `DRAFT_SOURCE_CHANGED`，同键不同已存内容仍为409 `DRAFT_REQUEST_CONFLICT`。后续材料、最终会话与PG幂等事务完全复用原路径，不新增执行或发布能力。

成功只返回八字段回执：`draft_id/request_key/source_id/version/import_revision/state/validation_state/created_at`。POST和`GET /api/v1/problem-draft-imports/{request_key}`均使用PG已存储的request_sha256，不从当前候选或客户端输入拼装修订。GET经实际会话后，在同一PG短事务内锁定当前恢复/账号/作者权限，再按owner/key读取；不访问材料对象或材料关联表，不占材料槽。未知key与其他作者同key同为404。当前成功启用的实例即使配置了新候选也能读旧已提交回执；整个能力仍须有效配置和成功预加载才能启动，关闭private_drafts后入口404，不能把“回执不依赖候选”解释为绕过启动门禁。

断连、超时、取消和503可能已提交，保留原source/key/revision，先手动只读查回执；没有自动POST重试。404只表示本次未观察到提交，不锁住不存在的键或取消在途POST；409修订变化也不证明旧请求没有提交。未确认期间不得换key、更新expected_revision或回退旧POST，继续原tuple核验；候选切换后未提交旧请求的重试需要维护者恢复原受信快照，已提交则只读查询即可。新协议拒绝query及转义路径别名，不接受主体、材料、路径或版本选择器。

回执复用现有`DraftByKeyForShare`及owner/key唯一索引，无schema、新SQL或索引变化；原2048行EXPLAIN ANALYZE继续验证相同查询。材料槽范围不扩成整个HTTP或这些只读请求的限额；最终Redis观察之后的退出、最终PG检查之后的撤权仍按已接受在途边界，不宣称跨库原子撤回。回退旧API二进制后新端点404，数据库/材料不变；管理页保留未确认tuple，不能自动降级调用旧接口。

[主体绑定管理合同](../api/problem-draft-management.openapi.yaml)将上述三种操作映射到独立的`GET /api/v1/problem-draft-management/sources`、`POST /api/v1/problem-draft-management/imports`及`GET /api/v1/problem-draft-management/imports/{request_key}`。必须有且仅有一个规范32位小写hex的`X-OJ-Expected-Subject`，否则400；它仅是对实际会话主体的前置比较，不构造actor或授予权限。首次真实Read之后、角色/候选/容量/材料/回执访问前不一致则409`DRAFT_SUBJECT_CHANGED`，存储/未知/取消仍优先503。后续完整会话/PG授权及所有原协议DTO/幂等边界复用同一调用链。独立路径使旧实例缺能力时404，避免旧二进制忽略新header而把旧账号意图写入新Cookie账号；客户端无旧路径fallback。已经按旧凭据接受的在途工作不因此变成可原子取消。

### 本人草稿列表

`GET /api/v1/problem-draft-management/drafts`使用相同的单个`X-OJ-Expected-Subject`，实际会话Read后先比较主体，再在同一5秒内PG事务持有恢复→账号→当前作者权限锁并查询owner；完成提交才返回完整页。默认拒绝、TLS/pinned代理及no-store/nosniff/不返回Set-Cookie或续期与原协议一致；新可选能力缺失404。只投影每行既有八字段导入回执，不读/解析document，不查材料关联/对象、现候选或占材料槽，不查询他人计数。旧草稿的来源无需仍在现目录中，但整个private_drafts须有效配置且成功启用，不能绕过启动门禁。

固定每页20条，`created_at DESC,id COLLATE "C" DESC`；读21条判定续页，next_cursor取第20条，空字符串表示本次无下一页。first/after两个sqlc查询避免可选边界OR；比较、排序和索引使用同一C排序。仅列表允许一个规范cursor query，空值/重复/未知字段/百分号别名/裸问号拒绝400；其余管理路径继续拒绝query。游标是规范raw-base64url的版本/owner/十进制Unix微秒/ID，校验长度、规范编码、可JSON表示时间和当前actor的owner；它只提供seek边界，不授予权限，不需签名或查询边界ID是否存在。`DRAFT_CURSOR_INVALID`400与内部ErrInvalid/存储/未知/取消503分开，无部分页。

分页只保证静态已提交数据按边界可完整遍历，并不持有跨请求快照。created_at在提交前产生，较早时间戳的在途导入可能在游标越过后才提交，沿旧边界将漏过，需重新检查第一页；空页或末页不证明没有在途请求。浏览器只保留当前页，显式下一页/回首页，每次检查会话且清除迟到/过期/换账号结果；不依赖本机IndexedDB，不自动POST或修改pending。列表提供发现及本人预览入口，不授予一般材料访问、编辑、审核或发布。

### 本人题面与样例预览

浏览器页面 `/author/drafts/:id` 已复用此接口和[共享 Markdown/数学渲染](frontend-design.md#共享题面与本人预览)，显式核验会话后展示本人固定版本，并可原文复制样例；`/author/drafts`接入导入与恢复。图片资源授权、审核与发布另行交付；页面不会修改 `not-run` 或自动清除任何发布阻断。

`Importer.Preview`和`GET /api/v1/problem-drafts/{id}/preview`只读取已入库固定版本。初次新鲜actor及同事务owner/当前角色检查成功后取得材料槽；解析PG固定文档，核对ID/owner/source/version/state、draft.json原文关联及无重复逻辑名，再将statement与所有sample=true点的input/answer逐一匹配文档和存储引用。examples顺序必须与固定测试清单相符。客户端不能选择路径、hash或任意测试点，正式数据、原始程序和来源报告不读取；该题在现候选中换版或不再选中时，已启用服务仍读取旧存储版本。入口启动的受信bundle/配置条件保持。

全部所选引用及原始markdown/input/output累计1MiB预算先核验，再执行第一次Open；同对象在多个字段出现也重复计费，不截断超限资料。逐份Open得到已完整哈希验证的匿名快照，限长读完并Close后才读下一份，支持Local活动预算为1。只允许严格UTF-8，保留CRLF、空格、空样例、BOM/NUL及末尾换行；不作替换、trim或归一化。JSON转义后文本可占六倍另加字段/元数据，响应不是可信HTML或渲染验收。

读取完成后再次读同一实际Credential并比较完整actor，再通过GetOwn重新核对当前owner/账号/恢复/角色及固定文档、source/version和引用。材料I/O不持PG锁，任何失败返回空预览；缺失/损坏、非法UTF-8、关联/预算异常、容量不足和取消为503，明确会话拒绝401、撤权403、未知/他人草稿404。最终Redis读取后的单token退出及最终PG授权后的撤权沿已接受在途合同，不承诺跨库原子撤回HTTP。返回专用JSON DTO，仅包含draft_id/version/title/language/markdown/有序examples；无Set-Cookie或公开下载。页面、Markdown/KaTeX渲染、本站受限验证、审核发布和训练授权仍需后续交付。

### 入库、版本与迁移

创建顺序为：新鲜会话读取→短出题授权预检→逐项持久化材料→再次读取同一实际凭据→同一READ COMMITTED事务核对恢复、账号及当前角色并写入草稿。全流程锁序为恢复→账号→角色→草稿，identity公开窄事务适配器，problem不读取identity私有SQL。材料复制/哈希和Redis读取均不在PG锁内。最终Read之前退出、过期或存储故障拒绝写入；Read之后的单token退出遵循普通会话“已接受在途工作不追溯撤销”合同，不宣称Redis与PG跨库原子退出。

`LoadBundle`接受维护者控制的稳定私有目录及**受信配置的来源锁原始字节**，不把bundle内的锁自行当作信任根。Go读取器核对完整文件集合、0700/0600及euid所有权、精确JSON字段/必填值/规范字节、版本、来源锁、选题映射、全部原始文件及样例关系；拒绝未完成标记、路径别名、链接、FIFO、公开权限、未列文件、未知schema和生产预算。预算为1MiB元数据、每对象128MiB、bundle2GiB、最多8192文件；每份Prepared只允许一个在途导入，Close拒绝活动导入。小元数据被冻结，大材料在授权后由Put核验原始字节；不是针对恶意同UID并发改写者的上传隔离。

每题保存题面、选中题目的输入/答案、原始源码/许可、草稿文档及四份来源证据和bundle索引。逻辑名绑定hash/size，不存宿主绝对路径，不按对象哈希授予下载权。bundle索引中其他题目的条目只作来源证据，本草稿仅拥有自己列出的材料引用。内容版本继续绑定原始草稿语义；请求摘要另绑定文档及有序全部引用（含证据/index），因而来源报告字节变化也会改变请求内容。

PG事务同一次提交owner、稳定ID、不可变文档/版本、全部引用及请求回执。同owner同键同内容返回原记录，不同内容为`ErrConflict`。`ErrCommitUnknown`后保持原键/原bundle核验；不得用新键制造重复记录。Put失败/未知不提交引用，已成功的无引用对象保留，当前没有全局磁盘额度或孤儿GC。业务读取限本人元数据/题面样例、授权配置候选摘要、本人导入回执和分页列表；不提供改写、删除、转让、共享或一般材料下载，状态结构性固定`private-draft/not-run`。

角色/审计及草稿/材料关联表始于schema7，不为历史账号授予权限；当前精确schema8新增`problem_drafts_owner_created_id_idx(owner_subject_id,created_at DESC,id COLLATE "C" DESC)`，使首屏和深页owner+seek查询不扫描/排序全部历史。每次新草稿多维护一个B-tree条目及相应WAL/磁盘，实际占用依数据分布而变；12000行合成查询计划及索引大小见交付记录，不将该样本当生产容量估算。原owner/key唯一索引支持幂等定位，主键支持本人ID读取，材料复合主键支持按草稿读取；不为列表附带大document或冗余覆盖字段。

升级在维护窗口排空并停止旧版API/Worker及维护写入，独立`migrate up`至8后启动同版二进制；API不自动迁移。普通CREATE INDEX在事务中建索引，需要协调写入，不使用CONCURRENTLY。新二进制拒绝7/未来schema；旧二进制只在启动检查时拒绝8，迁移不会自动停止已运行的旧进程，不混跑。回退先停新版，独立Down8仅删索引并保留全部草稿/权限，再启动要求7的旧版；重Up8保留资料。继续Down7至6仍仅允许角色、审计、草稿均空，存在数据则向前修复；Down6也保留原积压保护，不清表绕过。

真实PG专项覆盖默认拒绝、owner隔离、角色CAS/审计失败回滚、同键并发、引用失败回滚、三类撤销锁及连接失去后禁止另库提交。提交未知用真实PG的COMMIT/ReadyForQuery回包被测试传输消耗后断连验证，再按原键恢复；不是直接伪造成功。2048行填充的权威sqlc查询计划自然使用owner/key、草稿与材料主键索引。真实十题服务联调已提交785逻辑引用、622对象/1461321676字节，并在Store关闭后及新PG池读回；该语料专项的会话观察为stub，真实Redis身份边界由普通协议专项单独覆盖。具体运行证据归交付日志；不据此宣称完整P4、邮箱验证后的训练权限、审核、发布或部署完成。
