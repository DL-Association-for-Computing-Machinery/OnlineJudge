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

固定 A+B、关联数组、静态区间和、后缀数组、素性测试和卷积的九处指定输入输出格式块转换为逐行显示公式。转换只识别相应源小节下的裸围栏，保留公式、行序和变量顺序；省略号显示为数学省略号，单独冒号行保留。接受的符号与命令封闭，最多 16 行、4096 字节，缺失、重复或不支持的格式拒绝导入。普通代码围栏和原始样例不参与转换。六题获得新内容版本，其他四题内容版本不变；完整导入修订覆盖题包索引，不能用内容版本代替。重新转换须写入新目录，旧题包和已入库草稿不被改写。

题面转换不证明Markdown/KaTeX渲染通过，不执行模板表达式或把原始HTML认作已获准公开内容。原始`info.toml`、`task.md`和其他锁定源码完整保留；来源的solutions/forum/生成配置只作为来源信息，不悄悄成为本站执行配置。

十题配置为大小写敏感的精确token字节比较，明确关闭数字等价（`01`与`1`不同）；不继承M0默认忽略ASCII大小写。[独立文本组件](checker-admission.md)已提供四种空白的`lc-wcmp-tokens-v1`及显式限额，区别M0的VT/FF规则，并接入[首份完整草稿的受限参考验证](reference-validation.md)。该维护报告不更新数据库验证状态或生产Worker；`compatibility=configured-not-validated`保持，生产配置和完整验证任务仍待接通。上游timelimit只记录在来源字段，本站`limits`与`production_profile`保持null，不能用题源准备容器的预算充当选手限制。

## 校验、失败与下一步

先把锁内源码复制到私有已核验元数据快照，再推导测试集合和顺序；每个实际流式复制的数据文件在同一安全打开的描述符内计算哈希。拒绝路径别名、目录/文件链接、FIFO、未知/遗漏文件、来源锁或报告不匹配，保持每文件128MiB、整体2GiB预算。输入须是受信维护者控制的稳定私有快照；不是针对同UID恶意修改者的通用上传服务。

原子创建新输出后先写`INCOMPLETE`，只在全部文件、草稿版本和引用独立复验通过后，将`bundle.partial`改名为`bundle`并移除标记。磁盘错误、中断或校验失败保留标记及部分成果，不修改输入；不保证掉电持久性或自动清理部分文件。命令行复验拒绝带未完成标记的输出，可独立检查搬迁后的bundle；校验成功始终返回publication blocked。

源报告仅记录题源准备阶段已执行的验证，不将其当作本站运行证据。草稿的本站验证固定`not-run`，发布继续阻断于完整题面质量、生产检查器与资源准入、完整本站验证、获授权非作者终审及原子发布。下方应用服务及可选 HTTP 已接入[本地材料存储](material-store.md)、数据库固定引用、最小出题权限、导入管理页和显式图片预览。[持久化参考验证申请](reference-requests.md)已接入作者管理、专用 Worker 持槽执行及报告接纳；作者可在私有预览按原键回读本人终态摘要。P3/P5 接入真实训练授权及页面，普通判题 Worker 的固定 M0 材料门禁保持。

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

浏览器页面 `/author/drafts/:id` 已复用此接口和[共享 Markdown/数学渲染](frontend-design.md#共享题面与本人预览)，显式核验会话后展示本人固定版本、清单图片，并可原文复制样例；`/author/drafts`接入导入与恢复。审核与发布另行交付；页面不会修改 `not-run` 或自动清除任何发布阻断。

`Importer.Preview`和`GET /api/v1/problem-drafts/{id}/preview`只读取已入库固定版本。初次新鲜actor及同事务owner/当前角色检查成功后取得材料槽；解析PG固定文档，核对ID/owner/source/version/state、draft.json原文关联及无重复逻辑名，再将statement与所有sample=true点的input/answer逐一匹配文档和存储引用。examples顺序必须与固定测试清单相符。客户端不能选择路径、hash或任意测试点，正式数据、原始程序和来源报告不读取；该题在现候选中换版或不再选中时，已启用服务仍读取旧存储版本。入口启动的受信bundle/配置条件保持。

全部所选引用及原始markdown/input/output/清单原图累计1MiB预算先核验，再执行第一次Open；同对象在多个字段出现也重复计费，不截断超限资料。逐份Open得到已完整哈希验证的匿名快照，限长读完并Close后才读下一份，支持Local活动预算为1。文本只允许严格UTF-8，保留CRLF、空格、空样例、BOM/NUL及末尾换行；不作替换、trim或归一化。JSON转义后文本可占六倍，图片另以base64表示，外加字段/元数据，响应不是可信HTML或渲染验收。

读取完成后再次读同一实际Credential并比较完整actor，再通过GetOwn重新核对当前owner/账号/恢复/角色及固定文档、source/version和引用。材料I/O不持PG锁，任何失败返回空预览；缺失/损坏、非法UTF-8、关联/预算异常、容量不足和取消为503，明确会话拒绝401、撤权403、未知/他人草稿404。最终Redis读取后的单token退出及最终PG授权后的撤权沿已接受在途合同，不承诺跨库原子撤回HTTP。返回专用JSON DTO，包含draft_id/version/title/language/markdown/有序examples及可选images；无Set-Cookie或公开下载。全部题面质量、本站受限验证、审核发布和训练授权仍需后续交付。

#### 显式题面图片

`statement.images`为可选非空映射，键是相对来源`task.md`的路径，值为既有`path/sha256/size`引用。每一项必须与该题`provenance.original_files["source/<source_path>/<键>"]`完全一致；LoadBundle继续将这些原文件与受信配置的来源锁逐项比对。不能把目录内所有图片、正式数据或共用源码自动赋予题面角色。路径至多240个ASCII字符，仅字母/数字/下划线/连字符/点/斜线，各段非空且不以点开头，后缀只允许小写`.png/.jpg/.jpeg`；不解析URL、解码百分号、补`./`或回退basename。

最多16份原图，每份1—256KiB；所有原图引用在第一次材料读取前纳入原1MiB预算并匹配`original/source/...`逻辑关联。每张先用标准库PNG/JPEG读取尺寸，单边至多4096、单张至多400万像素、合计800万；随后完整解码并核对实际尺寸和格式，逐张受限重编码为静态PNG。动画、元数据、尾随内容不进入响应；坏图或输出超限使整份预览失败。每份PNG至多256KiB，PNG输出与原文/样例合计仍至多1MiB。有限输入/像素和调用前后取消检查不是硬CPU超时；运行在现有材料活动槽内，不增加应用并发。返回`images`按source排序，每项仅source/media_type/data（规范base64）/width/height，固定MIME为`image/png`。

浏览器严格核对字段、尺寸、base64及PNG头预算，Markdown原图地址只与已授权键精确匹配；成功时使用上述静态PNG的data URL，不请求原图地址。外链、SVG、HTML、data源及未知路径仍占位；每篇题面前32次图片出现才可能显示，后续占位。解码失败保留说明，图片随页面/账号失效清除，既有预览页面仍按原协议检查实际会话。

当前固定十题没有图片，离线适配器不会自动推断新来源的图片角色；本次可信合成锁/PNG/JPEG用例只验证协议和授权链。无图文档继续省略清单，文档版本与六字段预览不变。增加清单改变内容版本及完整导入修订，不覆盖已有草稿。无数据库迁移或新依赖；旧Go二进制不认识带清单文档，旧前端不认识带图片响应，回退时这些草稿预览会失败，需恢复支持版本；原无图草稿不受影响。没有公开图片端点、一般材料下载或发布授权。

### 作者送审回执

已登录且当前有出题权限的作者，可以把自己草稿的一份已接纳参考报告提交送审。此入口默认关闭，须启用 `private_drafts.reference_requests` 和 `private_drafts.review_intake`。目前只有 [HTTP 接口](../api/problem-draft-management.openapi.yaml)，没有页面按钮。维护分配另见[审核人分配与证据摘要](#审核人分配与证据摘要)。

- `POST /api/v1/problem-draft-management/review-submissions` 接受 `request_key`、`draft_id`、`import_revision`、`reference_request_id` 四个字段。完整修订从本人列表或导入回执取得；报告 ID 是[本人报告](reference-requests.md#作者申请入口)的 `request_id`。请求须同源，带普通会话和 `X-OJ-Expected-Subject`；请求头只约束实际会话主体。
- 服务端在持有当前作者权限和本人草稿锁的事务中核对修订、已接纳报告和冻结计划，自行保存来源锁、正式 Plan、Profile、报告证据的摘要和结论。有对拍时也固定对应计划和结论。失败报告和没有对拍的历史报告可以留档。草稿仍为 `private-draft / not-run`。
- 成功返回 ID、原四字段、内容版本和服务器时间。同一作者用同键提交相同材料返回原回执；同键提交另一组有效材料返回 `REVIEW_KEY_CONFLICT`，不同键各自形成回执。遇到断连或 `REVIEW_WRITE_UNKNOWN`，保留原主体和四字段，通过 `GET /api/v1/problem-draft-management/review-submissions/{request_key}` 查看。404 只表示本次未观察到提交；客户端不自动重试 POST 或换键。撤权后不能读取旧回执。

schema 15 增加送审账本，唯一的 owner/key 索引用于回执和并发去重；草稿与报告外键索引避免维护父记录时全表扫描。每条写入维护四个索引条目，增加对应磁盘和 WAL 开销。关闭开关保留记录。维护窗口排空旧进程后独立升级；Down15 仅允许空账本，存在记录须向前修复。

### 审核人分配与证据摘要

独立的 [审核接口](../api/problem-review.openapi.yaml) 默认关闭，在 `private_drafts` 中设置 `"review_access": true` 才开启。无需继续开放作者送审入口。维护者分别管理审核资格和每一项分配；普通作者角色、运维归属字符串和 Worker 身份都不赋权。作者不能获分配本人草稿；维护者仍须核实不同账号背后是实际非作者。

- 维护操作每次追加一条带操作者、理由和服务端时间的记录；必须给出当前修订。分配也须确认目标审核资格修订。资格变化使旧分配失效，重授后须重新分配。改派及撤分配不删除旧记录；结果不明先查看已保存的修订，不盲目追加。
- `GET /api/v1/problem-review/{review_id}/evidence` 带当前普通会话及 `X-OJ-Expected-Subject`。数据库先锁审核人当前恢复与账号，再读最新资格、锁定该送审项并读最新分配。旧会话不可用；改密后以新会话登录仍按当前资格与分配检查。已取得锁的在途读取遵循普通会话边界。
- 摘要接口仅返回本项固定的完整修订、报告标识、摘要与记录结论；没有独立对拍时省略其字段，正式 AC 不表示可以批准。题面和标程通过下述同范围材料入口读取。

schema 16 新增两张只追加的历史表。主体／项和修订的复合主键支持反向查最新记录，每次追加更新对应主键；分配额外维护非空资格外键的索引，避免维护父记录时扫描全部分配。维护窗口排空旧进程后独立升级；Down16 只在两份历史均为空时允许，保留已有送审，存在历史须前滚。

### 审核对拍详情

`GET /api/v1/problem-review/{review_id}/comparison` 沿用 `review_access`、普通会话、预期主体和当前非作者分配检查。在同一短事务内核对送审项的完整导入修订、正式与独立计划、已接纳报告摘要及两项结论，随后返回证据摘要及对拍详情。无已记录结论时 `comparison: null`，即使曾准备对拍计划也不代表通过。对拍通过时返回匹配点数且无反例；有差异时返回第一个失败点的生成输入、预期输出和候选输出。由这些字段可定位当时的修订和差异，正式测试输入与答案、原始报告及 Worker 信息均不返回。

入口只接受送审 ID，拒绝查询参数、请求体和转义别名；结果不一致或超限时不返回部分内容。响应至多 4096 字节。读取不执行程序，不写决定，也不改变草稿或发布状态。

### 作者确认标程

启用 `review_intake` 后，作者用普通会话、预期主体头和本人原送审键请求 `GET /api/v1/problem-draft-management/review-submissions/{request_key}/solution`。只返回冻结计划里的标程原文、固定依赖、完整导入修订及正式计划摘要。源码和依赖合计至多 64 KiB，不返回正式输入、答案或存储地址。读材料不持数据库锁；读完后重新检查同一凭据、作者资格和完整绑定。

核对后，作者以同源 `POST .../{request_key}/solution-confirmation` 提交 `import_revision` 和 `formal_plan_sha256` 两个摘要。服务端再次验证源文件、依赖和当前资格，在写事务内复核绑定。绑定变化返回 409；相同草稿、完整修订及计划并发确认只保存一次，同绑定下的其他送审键可回读同一确认时间。计划或修订变化不能复用旧记录。确认结果未知时，用原键 GET 同一路径回读；404 只表示本次没有观察到。读取或验证成功不会自动确认。

这是作者对标程的确认。独立非作者终审仍必需，已退回的材料仍阻止批准。确认不会改草稿状态、验证结论或发布状态。schema 18 只追加确认账本，保存首次确认对应的送审项及当时的作者资格、会话代次和时间。主键支撑同绑定去重和查找，另有送审外键索引；每次新确认增加两个索引条目及相应 WAL。停机后只允许空账本回退；已有确认应前滚。

### 审核人确认题面语义

显式启用 `private_drafts.statement_confirmations`（需要 `review_access`）后，当前指定非作者可用同源 `POST /api/v1/problem-review/{review_id}/statement-confirmation` 记录对冻结题意、约束、输入输出定义及样例解释的四项核对。使用证据响应的分配修订、完整导入修订和证据摘要，并传原请求键及 `checklist_version: "oj.statement-semantics.v1"` 以明确声明四项均已人工核对。系统校验引用字节，不替人作出判断。

新增记录前在共享材料槽核验题面、样例及选中图片，随后重读真实会话；写事务复核相同主体、会话代次、角色、分配和冻结清单。事务以身份、送审项、同材料锁为顺序，已退回材料不能新增确认，确认后仍可退回；同材料其他送审项的退回也会使该确认不再用于准入。单次分配只追加一条确认，换人或撤权重授后不继承。已写成的历史保留；分配不变时可用 `GET /api/v1/problem-review/{review_id}/statement-confirmation/{request_key}` 回读原键，不重新读材料或写入。确认未知时用原键回读，404 不表示没有在途写入。

准入只在当前分配有有效确认且材料未退回时返回 `statement_semantics_confirmed: true`。还须具有同冻结绑定的可信渲染证据才解除 `statement_quality_missing`。题面确认不改变草稿、正式结果、批准或发布状态。schema 19 增加两条唯一索引：审核项＋分配修订用于查当前确认，审核人＋原键用于回执；每次新确认维护两个 B-tree 条目并增加相应 WAL。维护窗口停新版后仅允许空账本回退，非空须前滚。

### 批准准入状态

`GET /api/v1/problem-review/{review_id}/eligibility` 沿用审核开关、普通会话和预期主体头。仅当前被分配的非作者可读；同一短事务核验完整导入修订、冻结计划及已接纳报告，查询同材料是否已退回及同绑定的作者确认。响应包含固定证据摘要、策略版本 `oj.review-approval.v7`、`eligible: false` 及阻断码。读取不写决定，不能作为批准凭证。

| 阻断码 | 需要处理的事项 |
| --- | --- |
| `materials_returned` | 同一材料已被退回；此前的重复送审项也不能继续批准 |
| `policy_unavailable` | 来源、来源锁或参考 Profile 不在当前支持范围；旧证据仍可回读 |
| `formal_failed` | 正式验证结论不是 AC |
| `comparison_missing`、`comparison_mismatch` | 卷积缺已接纳的本站对拍通过证据，或记录了差异 |
| `independent_correctness_missing` | 非卷积题未命中可信独立正确性证据 |
| `statement_quality_missing`、`test_coverage_missing` | 当前题面尚缺有效人工确认或可信渲染证据，或尚缺足以通过准入的正式数据覆盖证据 |
| `input_validator_missing` | 当前绑定没有受信输入校验证据 |
| `checker_admission_missing`、`target_runtime_missing` | 尚无该修订的生产检查器或目标节点资源准入证据 |
| `solution_confirmation_missing` | 尚无作者对该完整修订及正式计划的标程确认 |

未核对的题面、完整数据覆盖、生产检查器和目标环境仍阻止批准。即使正式点、卷积对拍、输入校验及作者确认都通过，也不会开放批准。不接收客户端指定的规则版本或证据；报告绑定不一致则返回 503，不返回部分结果。

仅当现行 Profile、来源锁、题目来源、内容版本、完整导入修订和正式计划摘要同时命中已审查的绑定，才返回 `input_validator_evidence`。其字段为报告摘要 `report_sha256`、该题已校验输入数 `inputs` 和对照数 `controls`（一个接受、三个拒绝）。现已采信原 Library Checker v6 的对应记录及 A+B 17 点修订的独立复验。完整修订绑定来源材料和原记录；正式计划绑定送审用例的顺序与字节摘要。未命中绑定继续阻断并省略证据对象。此证据不证明目标节点、生产检查器或数据覆盖。

原 v6 九道非卷积题和 A+B 17 点修订以同样的精确绑定返回 `independent_correctness_evidence`，包含报告摘要、固定套件 `lc-small-oracles.v1` 和本题用例数。已冻结的标程输出在这些用例上与独立计算的答案逐 token 一致。A+B 新修订的 17 个冻结输入、4 个对照和 25 个独立小用例来自同一份新报告，只解除各自的输入校验与独立正确性缺项。正式失败或退回时仍可读取匹配的历史复验，但相应阻断保留。卷积仍须有本站对拍通过的已接纳报告。

十题 v6 以同样的精确绑定返回 `wrong_solution_evidence`：报告摘要、候选来源（上游或本地）、候选标识、原 checker 拒绝的点号，以及拒绝前已记录为接受的点号。点号从 1 开始，对应冻结正式计划的完整顺序（编号包含样例，证据只取非样例点）；接受点按探测顺序排列。空数组表示报告没有此类记录。每题仅证明一种固定错误可被发现，`test_coverage_missing` 及其他阻断保持。响应不包含输入、答案或候选源码。

十题 v6 以同样的精确绑定返回 `statement_rendering_evidence`，包含历史浏览器报告摘要及本题样例、渲染公式和输入输出格式块的数量。[桌面与窄屏验证](delivery-log.md#2026-11-11六题输入输出格式逐行排版)使用的是旧导入修订，但十题的内容版本、Markdown 和有序样例字节与 v6 一致。接口只展示已核实的历史观测。`statement_quality_missing` 只有在同时具有[当前非作者题面确认](#审核人确认题面语义)和可信渲染证据时才解除。

只有[A+B 的 17 点边界修订](problem-sources.md#ab-值域边界修订)精确命中已审查的来源、锁、内容版本、导入修订、正式计划和现行参考档位，且当前正式报告为 AC、材料未退回时，才返回 `test_coverage_evidence` 并解除 `test_coverage_missing`。摘要包含报告摘要、总点和样例数、非样例与仅样例命中的条件数，以及最大和的原样例点号。四种固定错解分别报告被 15 个非样例拒绝的点数，不公开隐藏点号、输入、答案或源码。该题输入规模固定，低效解对照不适用。其他题目、旧 A+B、正式失败或已退回时仍返回覆盖阻断；输入校验、独立正确性、题面、生产检查器、目标环境及人工确认仍分别判定。

### 审核材料读取

`GET /api/v1/problem-review/{review_id}/materials` 沿用审核开关、普通会话和预期主体头。以本项已接纳报告、正式计划和完整导入修订确定材料，不读取当前候选目录。返回固定验证摘要、原题面、指定样例与静态图片，以及正式标程文本。素性测试同时返回正式计划明确固定的四个依赖，名称只是编译相对名。接口不接受文件路径或对象哈希，不返回正式输入与答案、非选定源码、原始报告、存储路径或下载能力，也不执行程序。

首次授权在短事务内核对主体、当前资格、分配、报告摘要及完整导入绑定。随后在事务外使用和作者预览、导入共享的每实例材料槽，逐份核验并关闭快照。题面、样例及图片沿用预览的 1 MiB 限额；入口和依赖合计至多 64 KiB，JSON 响应至多 7 MiB。读完重查同一凭据、身份代次、资格及分配修订，再核对原证据和选定引用。失去授权、存储损坏或超限均不返回部分内容。无 schema 变更、审核决定或发布状态更新。

### 审核退回与重送

维护者在 `private_drafts` 同时开启 `review_access` 和默认关闭的 `review_returns` 后，当前被分配的非作者可以用 [退回入口](../api/problem-review.openapi.yaml) 提交理由。新决定要求请求带上本项证据中显示的分配修订、完整导入修订和证据摘要；任一变化都须重新读取。退回使用普通会话、预期主体头和同源 POST，只写决定与审计，不改草稿或发布状态。

- 审核人先保留原请求键和参数；若提交结果未知，通过 `GET /api/v1/problem-review/{review_id}/return/{request_key}` 查询。本人仍须有当前资格和分配。原键同参数返回同一回执，不重写理由；改派后失去访问者不能查该回执。
- 作者在 `GET /api/v1/problem-draft-management/review-submissions/{原送审键}/return` 查看理由。此前已产生的同材料重复项共享该退回；旧送审原键仍可恢复原回执。新键提交同材料返回 `REVIEW_MATERIALS_RETURNED`。重送须有不同完整导入修订，或更新经接纳的验证证据。
- 服务端按作者、完整导入修订及已验证的正式计划、Profile、报告证据与独立计划摘要确定材料身份；请求键、送审 ID 和参考申请 ID 不算材料变化。退回和新送审持有相同的事务锁，再用新语句检查完整摘要。锁键碰撞只会额外等待；唯一性以完整摘要为准。

schema 17 增加只追加的退回账本。送审主键、完整材料唯一索引和审核人／原键唯一索引支持回执、去重及外键检查，每次退回增加相应索引条目和 WAL。升级前排空旧进程；Down17 仅在账本为空时允许，已有决定须前滚。关闭写入口不解除已保存的重送限制。批准及独立发布尚未开放。

### 入库、版本与迁移

创建顺序为：新鲜会话读取→短出题授权预检→逐项持久化材料→再次读取同一实际凭据→同一READ COMMITTED事务核对恢复、账号及当前角色并写入草稿。全流程锁序为恢复→账号→角色→草稿，identity公开窄事务适配器，problem不读取identity私有SQL。材料复制/哈希和Redis读取均不在PG锁内。最终Read之前退出、过期或存储故障拒绝写入；Read之后的单token退出遵循普通会话“已接受在途工作不追溯撤销”合同，不宣称Redis与PG跨库原子退出。

`identity/postgres.ProblemAccess.LockAuthorSnapshot`在调用方的同一READ COMMITTED事务返回`ProblemAuthorSnapshot{Actor,RoleRevision}`；所有检查成功后才返回完整快照，失败返回零值。恢复、账号和角色共享锁保持到调用方提交或回滚，原`LockAuthor`委托该检查并保留error-only接口。快照不是凭据、提交证明或事务结束后的权限；后续领取或接纳报告必须重新锁定当前授权并比较完整快照。只比较enabled会把撤权后重授误认成原授权，role revision对此有区分；credential-only rehash不改变会话授权，改密产生的新AuthGeneration及恢复代次变化则会改变Actor。schema9的[参考申请](reference-requests.md#身份与可信配置)保存该原快照，当前作者读取或取消旧申请不改写它；没有调度、报告接纳或新增HTTP字段，草稿继续`not-run`，发布仍阻断。

`LoadBundle`接受维护者控制的稳定私有目录及**受信配置的来源锁原始字节**，不把bundle内的锁自行当作信任根。Go读取器核对完整文件集合、0700/0600及euid所有权、精确JSON字段/必填值/规范字节、版本、来源锁、选题映射、全部原始文件及样例关系；拒绝未完成标记、路径别名、链接、FIFO、公开权限、未列文件、未知schema和生产预算。预算为1MiB元数据、每对象128MiB、bundle2GiB、最多8192文件；每份Prepared只允许一个在途导入，Close拒绝活动导入。小元数据被冻结，大材料在授权后由Put核验原始字节；不是针对恶意同UID并发改写者的上传隔离。

每题保存题面、选中题目的输入/答案、原始源码/许可、草稿文档及四份来源证据和bundle索引。逻辑名绑定hash/size，不存宿主绝对路径，不按对象哈希授予下载权。bundle索引中其他题目的条目只作来源证据，本草稿仅拥有自己列出的材料引用。内容版本继续绑定原始草稿语义；请求摘要另绑定文档及有序全部引用（含证据/index），因而来源报告字节变化也会改变请求内容。

PG事务同一次提交owner、稳定ID、不可变文档/版本、全部引用及请求回执。同owner同键同内容返回原记录，不同内容为`ErrConflict`。`ErrCommitUnknown`后保持原键/原bundle核验；不得用新键制造重复记录。Put失败/未知不提交引用，已成功的无引用对象保留，当前没有全局磁盘额度或孤儿GC。业务读取限本人元数据/题面样例、授权配置候选摘要、本人导入回执和分页列表；不提供改写、删除、转让、共享或一般材料下载，状态结构性固定`private-draft/not-run`。

角色/审计及草稿/材料关联表始于schema7，不为历史账号授予权限；schema8新增`problem_drafts_owner_created_id_idx(owner_subject_id,created_at DESC,id COLLATE "C" DESC)`，使首屏和深页owner+seek查询不扫描/排序全部历史。每次新草稿多维护一个B-tree条目及相应WAL/磁盘，实际占用依数据分布而变；12000行合成查询计划及索引大小见交付记录，不将该样本当生产容量估算。原owner/key唯一索引支持幂等定位，主键支持本人ID读取，材料复合主键支持按草稿读取；不为列表附带大document或冗余覆盖字段。schema9 另增[参考申请账本与默认空policy](reference-requests.md)，schema 10 增加预期计划表，schema 11 增加参考领取预留，schema 12 增加参考报告和申请终态，schema 13 增加参考调度，schema 14 增加卷积对拍预期计划，schema 15 增加作者送审账本，schema 16 增加审核资格和分配，schema 17 增加退回账本，schema 18 增加作者确认账本，schema 19 增加审核人题面确认账本；均不修改既有草稿、角色或材料引用。

升级在维护窗口排空并停止旧版 API/Worker 及维护写入，独立 `migrate up` 至当前 schema 19 后启动匹配二进制。API 不自动迁移，新二进制拒绝其他 schema。迁移不会停止旧进程，不混跑；本次没有迁移已有持久化 localdev 或服务器。回退先停新版，[Down19](#审核人确认题面语义) 只允许空题面确认账本；[Down18](#作者确认标程) 只允许空标程确认账本；[Down17](#审核退回与重送) 只允许空退回账本；[Down16](#审核人分配与证据摘要) 只允许空访问历史；Down15 见[作者送审回执](#作者送审回执)；历史 Down14/Down13/Down12/Down11/Down10/Down9 的锁序与条件见[参考申请迁移](reference-requests.md#迁移与回退)。

历史schema8的普通CREATE INDEX在事务中建索引，需要协调写入，不使用CONCURRENTLY。Down8仅删列表索引并保留全部草稿/权限，重Up8保留资料；回到8后须匹配要求8的二进制，继续Down8至7后才可启动要求7的旧版。继续Down7至6仍仅允许角色、审计、草稿均空，存在数据则向前修复；Down6也保留原积压保护，不清表绕过。

真实PG专项覆盖默认拒绝、owner隔离、角色CAS/审计失败回滚、同键并发、引用失败回滚、三类撤销锁及连接失去后禁止另库提交。提交未知用真实PG的COMMIT/ReadyForQuery回包被测试传输消耗后断连验证，再按原键恢复；不是直接伪造成功。2048行填充的权威sqlc查询计划自然使用owner/key、草稿与材料主键索引。真实十题服务联调已提交785逻辑引用、622对象/1461321676字节，并在Store关闭后及新PG池读回；该语料专项的会话观察为stub，真实Redis身份边界由普通协议专项单独覆盖。具体运行证据归交付日志；不据此宣称完整P4、邮箱验证后的训练权限、审核、发布或部署完成。
