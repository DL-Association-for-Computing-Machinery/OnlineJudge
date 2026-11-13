# 登录会话与 Redis 设计

2026-11-01：正式身份核心、[普通 Redis 存储协议](#79-普通-redis-会话协议)、[已有会话 HTTPS](#710-已有普通会话的-https-适配)及[两步登录恢复](#711-跨请求登录尝试与-cookie-发放)已有分项实现；[独立会话子实验](../../tests/sessionexperiment/README.md)保留旧 RDB、逻辑回档、轮换及保持登录等历史证据。默认 API 尚未接入账号入口，封装候选不等于服务端 pending 注册表、自动恢复、资源权限或生产准入；下文产品规则保持不变。

日期：2026-11-01。版本：0.22。状态：Session＋Redis、分场景期限、比赛账号、Argon2id、日常邮箱验证注册/找回及可替换 SMTP 接口已确认；SCS＋go-redis、go-mail 及 PostgreSQL 持久化发信任务＋独立受限发送执行器优先验证，Gin sessions 备选。River 开源核心优先验证，专用任务表备选；短期 AES-GCM 载荷、River 仅存业务 ID、独立密钥文件、不考虑付费方案及第 12 节令牌/重发/重试规则已确认；PostgreSQL 会话只作对比，不替换 Redis 基线。身份核心、普通 Redis 协议、两步 HTTPS、显式 TLS API、改密与浏览器账号面板已有分项实现；六会话/改密 OpenAPI 已补齐，默认 Compose 未启用会话，生产参数、邮件/资源权限和完整会话仍待交付，未部署。关联 [总体架构](system-design.md)、[ADR-001](001-go-gin.md)、[需求](../requirements.md)。

保持登录的后续子实验已覆盖默认关闭、主动选择 24 小时/7 天、Cookie 持久化、跨副本共享三名额，以及缩短期限下的续期/过期/迟到保存。SCS `RememberMe` 只控制持久化，期限由请求独立配置和服务端载荷选择；保存、结果核验、放弃共用策略上界，原绝对期限不变。普通 Cookie 改为非持久化，带 Cookie 再登录拒绝，活动参数不能改变登录时选择。仅测试夹具和同进程 TLS API，参赛期限、正式权限提升轮换、浏览器关闭/重开及完整 Gin/CSRF 仍待验证；细节见实验 README。

凭据轮换的后续子实验在三种 profile 下验证满额一换一、原绝对期限及策略保持、旧墓碑与迟到保存拒绝。SCS 默认轮换会先删除并重置期限，测试适配器改为请求内暂存删除意图、恢复原 deadline、在既有双代次守卫下以单脚本转移名额；不接入正式应用。并发轮换/新登录、失效/过期、池/OOM 及畸形索引有分项证据；写后丢回执仍 503/无 Cookie，不提供正式轮换结果协议。真实权限提升/敏感操作重认证、跨进程/主机、浏览器/Gin/CSRF 和执行中错误回滚仍未验收；细节见会话实验 README。

## 1. 决策与边界

来源防护的后续子实验将标准库 CrossOriginProtection 包在所有测试 TLS 路由之前，额外对显式 Origin 做当前 HTTPS/Host 精确匹配并拒空/多值，拒 same-site/cross-site，不配置 trusted origin/绕过。所有写探针先执行 POST 门禁，避免安全方法绕过来源检查后仍写会话。测试证明跨来源无 Cookie/状态变化、合法来源生命周期及先于后端 I/O 的拒绝；两来源头缺失仅兼容非浏览器驱动，不证明身份。没有 CORS 授权，真实浏览器、规范公共 origin/反代和正式 Gin 适配仍待验证；不能据此宣布完整 CSRF 或 AC23 通过。

管理 scope 的后续子实验已验证独立 Cookie/路径/Redis 命名空间，固定 30 分钟/8 小时期限，不能继承个人保持登录；读取与保存均校验 scope 和非持久化策略。缩短期限、跨副本撤销、共享账号/恢复代次、混合并发及入口 OOM 已有分项证据。仍是预认证测试身份，不验证真实管理资格或敏感操作重认证；独立名额计数也不代表已确定管理额度或跨 scope 总额度。实际行为和边界见[会话实验](../../tests/sessionexperiment/README.md)。

审计后新增 PostgreSQL 会话对比验证，Redis 仍为既有设计基线，仅在独立实验中验证，未接入正式登录。[普通会话对照](../../tests/sessionexperiment/README.md#postgresql-普通会话名额与撤销对照2026-10-31)已有双 TLS API 三名额、撤销/过期晚保存拒绝、整事务 INSERT 故障回滚和部分索引查询计划证据；后续[账号同事务子项](../../tests/sessionexperiment/README.md#postgresql-账号撤权与同事务回滚2026-10-31)覆盖当前启用状态/认证代次、撤权清载荷与名额、双方向锁等待及 UPDATE 故障整事务回滚。后续全局恢复门禁、同库旧备份及显式绝对过期凭据清理的有限流程和实测统一见[会话实验](../../tests/sessionexperiment/README.md)，不等于完整恢复或生产准入。

显式清理只删绝对到期凭据，主体/账号/门禁继续保留；不验证时钟回拨、自动调度、VACUUM/膨胀或长期容量。同库恢复不修复最新账号封禁/密码/权限事实。相同普通会话守卫的有限 Store 成本采样已完成 42 样本，读取与写入呈不同取舍；方法、逐轮范围及持久化/观测边界见 [M0 成本记录](../m0-experiment.md#普通会话-store-有限成本采样2026-10-31)。继续保留 Redis 基线并推进正式 P3；生产负载/容量、完整恢复/清理准入仍待验收，不据此维护第二份权威存储。比赛专用会话和换机管理随比赛阶段接入；日常邮箱规则不因骨架交付改变，邀请制试用仍待单独确认。引入时机见 [开发与交付计划](../phase-1-development-plan.md)。

用户在 Session/JWT 比较后指定 Redis，确定服务端 Session＋Redis 共享会话＋HttpOnly Cookie，随后确认分场景期限、普通三会话、正式线下比赛单参赛会话及独立队伍账号方案。本轮明确“优先验证 SCS＋go-redis，保留 Gin sessions 为备选”；这决定验证顺序，不等于正式准入、依赖安装或兼容性已通过。期限初值保持不变，精确依赖/Redis 版本与部署规格仍未定。

- Redis 保存活动会话，所有 API 副本访问同一权威会话存储；不用粘性会话或单进程内存保证正确性。
- 账号、密码验证信息、封禁状态、权限与持久化审计归关系数据库；按现有 PostgreSQL 优先验证路线设计。Session 中的身份不替代当前业务权限检查。
- 提交、成绩、判题任务及租约仍归持久化数据库，文件存储与 Worker 缓存路线不变；除第 7.13 节独立登录限流外，不把 Redis 用作判题队列，也不自动批准榜单缓存、其他限流或 Pub/Sub 实现。
- 首期浏览器登录采用不透明随机凭证，不引入 JWT 登录链路；JWT 保留未来跨服务/第三方访问场景评估。Worker 使用独立身份，不复用浏览器会话。

取舍：Redis 提供跨副本共享会话存储，活动会话读写与业务表分开；代价是额外常驻服务、内存/连接预算与故障恢复工作。PostgreSQL 会话方案保留为已比较备选，不作为故障时自动切换的第二份会话事实。本项目尚无 Redis/数据库会话性能对照或容量证明。

## 2. 会话与授权流程

1. 登录通过校内可用的账号验证后，由成熟库和密码学安全随机源创建高熵凭证；Redis 保存凭证摘要对应的主体、创建时间、空闲/绝对期限及版本信息。凭证不能携带可被客户端修改后直接信任的角色，不记录原始凭证到日志。
2. 只有会话创建成功后才发送登录成功与 Cookie。浏览器 Cookie 使用 HttpOnly、Secure、明确 SameSite 与尽量窄的域范围，前端与 API 优先同源部署；全链路 HTTPS，校内断外网时证书信任仍须成立。凭证不写入 localStorage/sessionStorage，浏览器代码草稿与认证存储分离。
3. 每次需登录的请求校验 Redis 会话、服务端时间与期限，再检查当前账号状态和所需资源权限。按数据库路线核对持久化账号认证代次，封禁、密码变更或全部下线可通过递增代次使旧会话失效；不得只信任 Session 中复制的旧权限。
4. 空闲续期不得越过绝对期限；降低续期写频率但不放宽期限。续期只原子更新仍存在且版本匹配的有效会话，不允许请求结束时无条件写回已撤销/过期会话。登录或权限提升后轮换凭证，旧凭证失效。
5. 退出登录须撤销服务端会话并清除浏览器 Cookie；撤销失败明确告知，不把仅删除 Cookie 宣称为服务端已退出。改密码/封禁的持久化状态变更为权威，Redis 清理失败不能恢复旧账号权限；跨数据库与 Redis 不声称原子事务。
6. Cookie 自动随请求携带，因此写操作配置 CSRF 防护和来源校验，SameSite 只作为附加保护；身份验证与资源授权分别在服务端执行。期限、多会话及敏感操作再次验证遵循下节，具体中间件和协议待实现验证。

上述逐请求的账号状态、权限和代次核验仍会访问数据库；不能因为引入 Redis 就声称消除了数据库授权开销。若后续缓存授权，必须单独设计失效与生效时限，不能以性能优化绕过封禁和撤权。

测试专用保存尝试原语已验证：服务端冻结凭据/载荷/原期限，核验完整会话与名额后才能获得正向时点证据；不存在仍是未知，重试不生成新尝试。放弃原子保留无载荷墓碑至原绝对期限，即使原保存尚未到达也能阻止迟到写入；失败不能宣称取消成功。核验/放弃沿用全局行 → 账号行的锁序与双代次校验；核验不会保证后续并发撤销前持续有效。这些原语未提供正式 API、持久化候选、浏览器权限绑定或崩溃恢复，不能接受浏览器传入的候选数据，也不声称 PostgreSQL/Redis 分布式事务。

### 2.1 分场景有效期

| 场景 | 空闲期限 | 最长有效期与行为 |
| --- | --- | --- |
| 普通训练登录 | 2 小时 | 登录起 12 小时；到期重新验证身份 |
| 自用设备主动勾选“保持登录” | 24 小时 | 登录起 7 天；默认不勾选，公用机不建议启用 |
| 管理后台 | 30 分钟 | 登录起 8 小时；改权限、重置账号等敏感操作再次验证身份 |
| 受监考的正式线下比赛 | 按赛程配置，赛前检查覆盖长时间思考/本地编辑 | 独立参赛会话覆盖赛前登录至赛后关闭窗口；每场配置有限截止时间，不无限续期 |

空闲和最长期限均由服务端判定，取更早的失效条件；凭证轮换不能重置最长期限。管理权限不能继承个人“保持登录”的长有效期；普通身份、管理授权及参赛资格分别检查，具体 Cookie/路由隔离待库验证。敏感操作的再次验证不等于重新延长所有登录状态。

比赛结束时服务端立即停止接受计分提交；赛后结果查看窗口首期默认 30 分钟、可由赛务配置，窗口内仍执行封榜和代码可见性规则，不能提前开放题解或隐藏成绩。窗口结束撤销参赛会话、停用专用队伍账号，个人账号的日常训练身份不随比赛结束被删除。延期或延长比赛由获授权工作人员调整并审计，同时核对账号启停、会话期限及 Redis TTL；不能只改页面倒计时，也不能复活已经撤销的会话。实际赛事起止、空闲期限须在赛前检查中确认。

本地编辑不必然产生服务器请求，自动榜单轮询也不代表用户仍在操作。实现须区分有效活动与后台刷新、提供到期提醒；会话自然过期保留同一用户的本地草稿，重新登录恢复后由用户确认提交，不自动重放；主动退出提供默认勾选且可取消的本账号草稿清理选项，与服务端退出分别报告结果，并防其他标签的迟到保存复活已清理内容，见 [本地草稿规则](frontend-sandbox-options.md#32-已确认的-indexeddb-本地草稿规则)。活动采样/续期频率待设计，不能依赖客户端自行延长服务端期限；切换账号仍遵守草稿隔离。关闭浏览器不能替代显式退出和服务端过期。

### 2.2 多会话与换机

- 普通个人账号最多 3 个有效普通会话，提供会话列表、退出指定会话及退出其他会话；超额时先核验密码，再让用户选择撤销已有会话，不静默挤掉另一处登录。尚未完成选择的登录不能获得完整业务权限。
- 正式线下比赛按“比赛＋参赛主体”限制为 1 个有效参赛会话：个人赛按个人，团队赛按本场队伍。个人的其他普通会话不能绕过限制读取受限比赛内容或提交；管理后台期限也不能通过普通会话绕过。
- 第二次独立参赛登录被拒绝，原会话继续使用；同一有效会话的页面刷新和多个标签页不重复占用名额。该限制控制浏览器会话，不能证明只有一台物理电脑，更不能替代监考；IP 仅作辅助记录，不作为唯一身份或硬绑定。
- 换机、浏览器数据丢失或需重置时，由获授权工作人员核对身份、撤销旧参赛会话、释放名额，再允许新登录，记录操作者、原因和时间；重置密码同时使旧凭证失效。不得先发新会话再撤旧会话，或凭短暂心跳中断自动抢占。
- 普通三会话和比赛单会话的检查、占位、撤销及过期清理须跨 API 原子协调；并发登录不能超额、已撤销会话的迟到续期不能重新占位。Redis 故障按第 3 节处理，列表/索引清理不能依赖扫描整个键空间。
- 线上团队训练使用不同成员的个人账号，分别受普通会话限制；不对整队套用线下单参赛会话。工作人员使用各自个人账号，首期不另设未确认的后台设备数量上限。

### 2.3 比赛账号与历史归属

| 场景 | 已确认的账号规则 |
| --- | --- |
| 个人赛 | 使用个人账号；正式线下赛额外建立受限参赛会话 |
| 正式线下团队赛 | 每场生成独立队伍账号，绑定本场参赛队与报名确认的成员名单快照，仅限本场权限 |
| 线上团队训练 | 成员个人账号提交，保留实际提交者，按本场队伍合并计分 |

队伍账号使用各自独立随机密码，通过受控方式发放；校内工作人员能在断外网时完成账号发放、密码重置和换机。密码按安全验证信息保存，明文密码不写入日志。热身赛和正式赛分别授权，赛前登录不开放正式题目；不自动将正式账号授权到其他比赛。

专用账号赛后停用但不删除提交、成绩及成员名单快照。个人退队/换队不改写历史归属，赛后由绑定成员通过个人账号按授权查看；共享队伍账号的提交不推断具体作者或给所有成员计为独立解题。管理员、裁判和气球工作人员使用各自账号及比赛内角色，便于审计。

选择代价：相比统一短期登录或一律单设备，分场景规则更符合训练与机房比赛，但需维护授权范围、期限配置及并发名额。相比所有赛事均发临时账号或全部用个人账号，三类身份保留个人历史并方便线下发号；这属于已确认产品规则，尚无运行验证。

## 3. 故障与恢复

| 情况 | 设计行为 |
| --- | --- |
| API 副本退出或重启 | 其他副本用相同会话配置和共享 Redis 继续校验；不依赖原进程内存 |
| 会话不存在、已过期或被撤销 | 需要重新登录，不能仅凭旧 Cookie 重建登录状态 |
| Redis 超时或不可用 | 需登录的操作返回明确的暂不可用错误，不冒充账号无效；不退回本地会话或绕过鉴权，公开内容可在权限边界内继续服务 |
| 会话创建或续期失败 | 不返回虚假的创建/续期成功；受影响操作按失败策略处理，避免无限重试 |
| Redis 数据丢失 | 活动会话可能失效并需重新登录；数据库中已接收提交、成绩与任务不因此删除 |
| 旧快照恢复、重启加载旧数据或故障切换可能回退状态 | 恢复期间暂停接受旧会话；确认唯一可用会话后端，在关系数据库持久化新生成的全局恢复标识并重建活动会话空间，再开放重新登录。API 校验该标识；数据库备份恢复后同样先设置新标识再开放，不能仅从回档整数值加一 |

最后一行是首期保守恢复设计，不承诺故障期间无感登录或自动高可用。恢复操作须协调所有 API 副本和旧 Redis 端点，避免新旧端点同时写入；无法证明会话状态连续时不得直接恢复旧登录。已经通过鉴权的在途请求不会被删除会话自动撤销，敏感操作在业务提交前按其事务边界再次核验权限。

测试夹具已验证的实现顺序：开始恢复原子写入独立随机新标识并关闭入口；保存按全局行 → 账号行取得共享锁，核验账号代次/全局标识后执行 Redis 操作。完成按本轮标识及关闭状态条件开放，防止迟到完成误开新一轮；失败不自动开放。新名额空间绑定两者，旧键留至原期限。全局主键仍按命名空间定点读/锁/更新，真实锁超时及逻辑回档用例提供顺序证据；共享锁会增加恢复等待，字符串存储与容量、生产迁移及索引仍待设计。

数据库回档全过程必须由外部机制独占协调并隔离旧 API/Redis 端点；备份也会恢复启用状态，必须先停止并排空旧 API，在回档后写新标识/关闭入口，才能重建认证服务。CAS 仅约束完成操作，不能阻止另一操作者再次 restore 覆盖标识/门禁。随机标识避免确定性回档复用，不是外部单调 fence；它只使旧会话失效，不修复回退的密码、封禁和权限。早期实验使用单机专用 schema 的逻辑回档和同进程双 TLS API 重建。

普通会话专项另验证当前 API：用 PostgreSQL 逻辑备份恢复到另一个临时集群，并在原 Redis 命名空间保留旧记录及相同密钥，作为恢复代次的对照。若不换代次，恢复出的旧权威在存储层仍会接纳该会话；实际流程先换代次并闭门。闭门时 API 启动失败，旧代次无法打开门禁；以新代次重新开放后，两个独立 TLS API 均拒绝旧会话和未完成登录 Cookie，同一账号可重新登录。此测试不改变生产中隔离旧 Redis 并重建会话空间的要求，也不验收生产恢复编排、账号事实对账或 PITR。

Redis 持久化方式、是否需要副本/Sentinel、重启是否保留有效会话及恢复时限待资源与演练结果确定；普通 AOF/RDB 或异步复制不能证明撤销记录绝不回退。初期可评估单实例校内部署，但须明确其登录服务单点和故障窗口，不能声称 API 多副本已经覆盖该故障点。

## 4. 资源与部署要求

- 会话用途独立预算，首期不与大体积、可淘汰的业务缓存混放。若未来共用机器，仍需独立实例或经验证的容量隔离；不同键前缀或逻辑数据库不等于独立内存额度。
- 会话实例优先验证 `maxmemory`＋`noeviction`，所有会话设置受控 TTL；内存不足时相关写入可能失败，须告警和处理登录/续期错误。`maxmemory` 不等于进程 RSS 硬上限，系统、连接及持久化缓冲另留余量。
- 独立 Redis 7.2.4 夹具已验证：保存 Lua 从首字节声明 `#!lua`，让入口已超限的写入在业务逻辑前失败，避免传统脚本先清索引再分配而绕过 OOM 检查；满名额时也优先返回存储不可用。只读与撤销脚本保留原行为并实测。该声明不提供脚本执行期间硬上限或错误回滚，Cluster 默认行为不同，不能直接外推；正式中间件续期与容量仍需验收。
- 各 API 的连接池、命令超时、有限重试及重试幂等性统一预算；压测登录突发、每请求读取、续期、过期回收和重新登录潮。不得无限增加连接或把在线人数当作连接数。

当前会话子实验在同进程双 TLS API 中设置每主池 `MaxActiveConns=8`，禁用独立 pipeline 池，普通与管理 scope 共用本副本客户端。真实 `CLIENT LIST` 观察该有限路径合计 16 条，饱和失败和释放恢复已有证据；观察客户端、驱动、PubSub 等不在此预算验证内。该数字是夹具参数，不是生产预算；跨进程/主机、吞吐、内存、数据库连接及重连峰值仍待实测。复现和限制见[会话实验](../../tests/sessionexperiment/README.md)。
- Redis 仅受控内网可达，启用 ACL/认证与最小命令/键权限，跨主机传输按部署威胁边界启用 TLS；不向选手网络公开服务端口，不交给 Worker/沙箱凭据。配置与凭据不写入源码或日志。
- 依赖安装前核对精确版本、许可证、维护、安全公告及 Go 客户端/会话库兼容性；没有锁定版本或可用启动命令。通用期限初值已确认；每场赛程期限、内存额度与故障后可接受重登录窗口仍待确定。

## 5. 验收与依据

在真实 Redis、PostgreSQL 与至少两个 API 副本下验证：跨副本登录、退出、轮换、封禁/撤权、密码变更、空闲和绝对过期、续期与撤销竞态；覆盖 Redis 超时/内存不足、重启、旧数据恢复和会话代次隔离，确保旧凭证不复活。检查 Cookie 属性、CSRF、凭证不入日志、前端草稿不含登录凭证，以及断外网的新浏览器登录。对应 AC17/18/19/20 和新增 AC23；均尚未执行。

AC23 进一步覆盖：三种期限初值及边界、保持登录不默认开启、后台轮询不无限保活、本地编辑到期后草稿恢复、管理重认证；多个 API 同时发起第 4 个普通会话/第 2 个参赛会话、同会话重试、多标签页、换机与旧请求竞态；校内重置、比赛结束时拒绝提交、赛后窗口停用、延赛期限协调、普通会话不能绕过比赛限制，以及退队后历史名单与成绩不改变。

参考（2026-10-25 只读核对，组件能力不代表本项目已通过验证）：

- [OWASP Session 管理](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html)、[JWT 的撤销与取舍](https://cheatsheetseries.owasp.org/cheatsheets/JSON_Web_Token_Cheat_Sheet.html)。
- [Redis 内存淘汰](https://redis.io/docs/latest/develop/reference/eviction/)、[持久化](https://redis.io/docs/latest/operate/oss_and_stack/management/persistence/)、[复制限制](https://redis.io/docs/latest/operate/oss_and_stack/management/replication/)、[安全配置](https://redis.io/docs/latest/operate/oss_and_stack/management/security/)。

## 6. 会话库与 Redis 客户端验证路线

2026-10-25 用户确认优先验证 SCS＋go-redis，保留 Gin sessions 为备选；现有产品规则和 AC23 不因候选库默认行为改变。独立实验已运行 SCS，正式核心已锁定 go-redis 并实现第 7.9 节存储协议；SCS/Gin HTTP 适配及完整准入继续待验证：

| 路线 | 依据及适配工作 |
| --- | --- |
| SCS＋go-redis 存储适配 | 用户确认优先验证；SCS 提供空闲/绝对期限、凭证轮换和保持登录，提供 go-redis 后端。需验证 net/http 中间件与 Gin 的响应/错误生命周期、不同身份的策略隔离 |
| gin-contrib/sessions＋Redis 后端 | Gin 集成直接、有 Redis 后端，保留备选；需核对所选后端的实际客户端依赖，并补齐期限、轮换及 OJ 会话规则，不能仅设置 Cookie MaxAge |
| 直接使用 Redis 客户端自建全部会话能力 | 控制灵活，但自行承担完整安全生命周期和测试成本，首期不推荐 |

SCS 的官方 goredisstore 文档示例采用 go-redis/v9，默认键含 token，不能不经审查直接当作满足本文的凭证摘要存储要求。普通会话列表、原子名额限制、撤销与迟到保存竞态、恢复代次及比赛权限均需应用/存储适配层验证；不能用无条件保存覆盖撤销结果。若适配成本过高，应回到备选比较，不为了库默认行为降低已确认规则。精确版本、Go 兼容性、许可证与传递依赖、安全公告、资源和故障行为须在正式引入前核验。

依据：[SCS](https://github.com/alexedwards/scs)、[SCS go-redis 后端](https://github.com/alexedwards/scs/tree/master/goredisstore)、[Gin sessions](https://github.com/gin-contrib/sessions)、[go-redis](https://github.com/redis/go-redis)。性能优劣须由本项目实测，不采用库的性能宣传作为容量证明。

验证出口：锁定兼容版本及依赖证据；用真实 Redis、数据库和至少两个 Gin API 副本执行 AC23，分别报告中间件适配、Cookie/期限、凭证摘要存储、普通/管理/比赛隔离、并发名额、轮换/撤销/迟到保存及 Redis 故障恢复；同时检查连接池总额、超时与有限重试。应用负责业务授权和比赛身份，存储适配层负责共享会话协议；SCS 和 Redis 客户端都不代替这些职责。任一关键行为不能满足时记录证据并重新比较 Gin sessions，不混用两套权威会话或静默回退。

## 7. 密码保存与校验

用户明确确认：“采用 Argon2id＋每个密码独立随机盐＋可升级的参数记录，具体参数等硬件确定后验证”。这是算法与存储方向确认，不是具体 Go 依赖或成本参数准入。scrypt、bcrypt 和 PBKDF2 保留为比较背景，首期不并行引入或自动回退；选择 Argon2id 的代价是密码计算占用 CPU 与内存，需要联合控制登录峰值。

- 使用成熟实现；每次创建密码记录、改密/重置或重新哈希时，由密码学安全随机源生成独立盐，不使用账号名、固定盐或可预测值。随机失败则操作失败，不降级为固定值。
- 关系数据库按第 7.2 节保存算法标识、算法版本、成本参数、盐及派生哈希；当前结构化密码核心见第 7.1 节，网络解码/账号用例另行交付。盐无需保密；原始密码、完整密码验证记录和原始登录凭证不写日志，不保存明文或可逆加密密码。
- 校验读取记录中的受支持版本与参数，严格限制编码长度、参数范围和分配上限，畸形/不支持记录明确失败并产生脱敏诊断，不无限分配内存、不自动降级。当前核心在派生前核验结构与允许策略，使用成熟实现及常量时间摘要比较；密码字节上限显式给定，不能静默截断。未来编码入口仍须独立限制解析/解码前大小。
- 当前写入策略与允许验证的旧策略分开。旧记录验证成功且确需升级时，按当前策略和新盐重新计算；以旧记录/凭据版本为条件更新，避免覆盖并发改密或重置。同一密码的成本升级不等同于改密码；密码重置须按第 2 节更新认证代次并使旧会话失效。发放新会话前须复核账号状态及凭据版本，不能凭重置前的校验结果登录。
- 失败登录不升级记录，不因请求量高而降低算法强度。算法、格式和参数升级需兼容现有记录并回归验证；不能仅根据“数值变大”断定策略更强。具体回退与旧版本兼容须随正式实现记录。

密码哈希与高熵 Session 凭证摘要的用途不同，不将慢密码哈希用于每次请求的会话查询。

学校资源有限，需联合验证密码计算参数和登录并发上限：登录/注册/改密/重置需要密码计算，普通已登录请求使用会话；多 API 总 CPU/内存、赛前集中登录、失败登录及批量发号均计入预算，限制排队和计算并发，不能为吞吐改用普通 SHA-256。参数随硬件实测确定，支持后续成功登录时升级旧参数；同步 Go 核心、个人登录解析与第 7.13 节共享准入已有分项实现，生产资源数值及完整准入仍待验收，不将 `argon2.IDKey` 本身当作完整账号系统。

硬件确定后再记录内存成本、迭代次数、并行度、盐/输出长度与各副本计算额度；使用当时可靠建议作为基线，测量单次延迟及集中登录下的总 CPU/RSS、排队、拒绝和恢复时间。计算未真正结束前不能因 HTTP 超时提前释放其资源名额；同机多个 API 的额度之和必须受主机预算约束。只在需要密码校验的流程承担该成本，普通 Session 查询不重新运行 Argon2id。

对应新增 AC24：同一密码在不同新记录中生成不同盐并均能正确验证；错误密码、畸形编码、越界参数与不支持版本失败；旧策略验证与升级可回归，升级/重置/并发登录不覆盖新凭据或产生有效旧会话；检查日志无秘密、校内断外网可校验，以及多副本集中登录和批量发号资源有界。核心与账号用例分项验收，参数不填写未经实测的默认数值。

参考（2026-10-25）：[OWASP 密码存储](https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html)、[Go Argon2](https://pkg.go.dev/golang.org/x/crypto/argon2)、[Go bcrypt](https://pkg.go.dev/golang.org/x/crypto/bcrypt)。尚未执行密码哈希基准或负载测试。

### 7.1 同步密码记录核心

`internal/identity/password` 通过 `New(Policy, Limits)` 显式创建 Hasher：`Policy.Current` 是唯一写入 tuple，`Legacy` 是允许验证且需迁移的旧 tuple；构造时复制并拒绝重复、越界和当前策略混入旧策略。`Create([]byte)` 使用独立密码学随机盐；`Verify([]byte, Record)` 返回 `Match`/`NeedsUpgrade`，错误密码为全 false、无错误，畸形/不支持记录与非法输入返回稳定分类错误。只有旧策略密码匹配才提示迁移，不隐式重哈希或写库，也不以数值大小推断强度。

记录固定为格式 1、`argon2id`、算法版本 `0x13`，包含内存 KiB、迭代、并行度、盐长、摘要长及盐/摘要字节。当前采用结构字段，便于用有限 tuple 严格匹配；暂不引入尚无互操作要求的 PHC 编码/解析器。数据库结构列映射见第 7.2 节；未来传输编码/解析另验，不能直接接受网络 JSON 作为此 API 的输入，也不能将完整记录输出到 HTTP 或日志。依赖复用已锁定的 [x/crypto v0.58.0](https://pkg.go.dev/golang.org/x/crypto@v0.58.0/argon2)（BSD-3-Clause），仅从间接改为直接依赖，未升级版本或新增模块。

所有 `Limits` 必填，无生产默认值：内存、迭代、并行度、密码/盐/摘要字节上限。支持范围为非零迭代/并行度、内存至少 `8 × lanes` 且整除 `4 × lanes`、盐至少 8 字节、摘要至少 16 字节，拒绝库会调整的内存布局；这些是模块结构边界，**不是生产安全参数建议**。长度/乘法按宽类型校验并受平台 int 范围约束；记录须先通过版本、长度、上限及完整 tuple 匹配才运行派生。允许列表控制兼容范围，Limits 控制配置预算；每次运算有界不代表并发总资源有界。

密码按原始字节处理：两入口均拒绝空值和超过显式字节上限的输入，不 trim、Unicode 归一化或截断；产品强度规则归上层。调用期间不得并发修改输入密码或记录切片。Hasher 策略构造后不可变；零值失败关闭，随机失败不返回部分记录。不提供日志，不保证完整内存擦除；只有固定长度摘要比较使用常量时间，整个错误/成功流程不提供防枚举耗时保证。

底层接口同步且没有 context/超时 goroutine；第 7.3 节增加共享实例内的计算准入包装。上线网络入口前仍须配置并验证多 API 总预算，资源名额一直持有至实际计算结束。同密码重哈希的数据库条件更新原语见第 7.2 节，验证/升级应用用例见第 7.4 节；会话门禁与 Redis 协议见第 7.5—7.9 节，HTTP 发放与恢复编排继续待接入。生产参数、真实登录/邮件、权限/会话、容量与完整 AC24 继续待验收。

### 7.2 主体凭据持久化与条件更新

`internal/identity` 拥有主体状态，`internal/identity/postgres` 用 sqlc/pgx 持久化；密码记录校验统一复用 `Hasher.CheckRecord`。SQL 事务不运行 KDF，注册编排在开启事务前通过共享计算预算生成占位凭据。身份表与提交/Worker 的私有表分离，不回填历史主体或对提交/Worker 新增身份外键。主体为不可复用的 32 字符小写十六进制 ID（128 位），不是登录名；没有删除/复用或 UPSERT 覆盖凭据的接口。个人邮箱映射及其主体外键见第 7.7 节，注册和激活见第 12 节。

一行 `identity_credentials` 原子保存 `enabled`、正 bigint 的 `credential_version`/`auth_generation` 与完整密码记录。格式/算法/版本、规范内存布局和长度有数据库约束，整数映射在窄化前检查。盐/摘要为 `bytea`，各最多 64 字节；这是存储格式上限，不是生产 KDF 参数建议。构造 Store 时检查当前及全部旧策略均可容纳；创建/替换/重哈希只写 Current，读取可接受已批准 Legacy。不提供 PHC/JSON 解析器，不直接接收网络记录。

| 受信内部操作 | 条件与结果 |
| --- | --- |
| `Initialize` | 显式指定启用开关、写 Current；新主体从版本 1/1 开始，重复 ID 冲突且不覆盖 |
| `Get` / `State` | 前者返回含密码的内部快照，后者只选四个状态字段，供旧记录退役后准备撤权；均只是时点读取 |
| `RehashCAS` | 仍启用且预期两代次匹配，仅凭据版本 +1；认证代次不变 |
| `ReplacePasswordCAS` | 预期两代次匹配，两代次均 +1；不重新启用禁用账号 |
| `DisableCAS` | 仍启用且预期两代次匹配，仅认证代次 +1、关闭账号；不解析旧密码材料，不清 Redis |

递增字段到 bigint 上界后拒绝，不回绕。零行更新为冲突，不读后自动重试；数据库错误保留内部原因，但稳定错误文字不回显 PostgreSQL 的完整失败记录。每次操作有最多 5 秒的调用方 context，短期限优先；数据库/连接错误仍可能是写结果未知，不能视为确定未写入或自动改用新版本重试。已受控验证客户端收到取消错误后，后台 UPDATE 仍可提交完整新行；取消错误不是服务端已停止的屏障。[pgx v5.11.0](https://github.com/jackc/pgx/blob/v5.11.0/pgconn/pgconn.go) 的后台清理及 [PostgreSQL 14 取消协议](https://www.postgresql.org/docs/14/protocol-flow.html#PROTOCOL-FLOW-CANCELING-REQUESTS)支持此边界。池由调用方创建、限制并关闭，Store 不创建第二份连接池。

这些方法**不是公开账号管理或完整登录/自动升级服务**。`enabled` 只是账号开关，不代表邮箱已验证、训练访问或管理权限。`RehashCAS` 无法证明新旧记录属于同一密码：受信用例必须先成功验证旧快照，再以同一密码生成 Current，全部 KDF 在事务/行锁外进行，最后提交 CAS。重置令牌消费、操作人授权、审计、真实发会话前复核及 Redis 撤权联动仍待实现；不能凭旧 `Get`/`State` 结果发放权限。

主体表由迁移 3 引入；API 严格要求匹配 schema，邮箱模型始于5（第 7.7 节），当前版本及回退见[训练队列迁移](../submission-backlog.md#升级与回退)。旧二进制按各自精确版本检查拒绝新schema，不支持滚动混跑。实际启用须停止旧实例、独立迁移、启动匹配新版，本次没有部署。迁移 3 的 Down 仍按身份表→submissions 的固定顺序取得排他表锁；仅当 `identity_credentials` 与 `submissions` 均为空才允许移除主体表，不要求材料表或 Worker 表也为空。迁移 4/5 的回退另受第 7.6/7.7 节保护。跨多个迁移的 `DownTo` 不承诺整体原子，不能用回退现有用户/提交库代替前滚修复。数据库旧备份还会回退账号事实及版本，两代次 CAS 不替代维护隔离、恢复代次与最新安全状态核对。

仅新增主键索引：支持主体读取及版本条件 UPDATE 的点查，代价是每行索引存储及初始化/更新的维护，不为版本字段另建二级索引。两模块从同一权威迁移生成各自的查询包；关闭未使用模型生成，避免将身份私有模型散入提交模块；生成差异检查同时覆盖两包。[PostgreSQL 14 READ COMMITTED](https://www.postgresql.org/docs/14/transaction-iso.html#XACT-READ-COMMITTED) 的等待后条件重查由真实锁实验验证，实际计划与边界结果归交付日志，不据此宣称容量或完整账号验收。

### 7.3 密码计算准入

`NewBounded(hasher, AdmissionLimits)` 创建同步 `BoundedHasher`，`Create(ctx, password)` 与 `Verify(ctx, password, record)` 共用同一计算额度。`MaxInFlight` 必须为正；`MaxWaiting` 非负、零表示不排队；启用队列必须显式给定正 `MaxQueueWait`，负期限拒绝。没有生产默认值，零值/未初始化 Hasher/空 context 失败关闭。密码及完整记录先做廉价校验，再尝试准入，非法输入不占排队或计算额度。

额度立即可用则取得一个名额，否则尝试保留有限等待位；队列已满返回 `ErrBusy`，排队自身超时返回 `ErrQueueTimeout`。调用方取消/期限错误保留 `context.Canceled`/`context.DeadlineExceeded` 分类；当已观察到调用方取消时优先返回该错误。等待结束先释放等待位，排队计时器不限制已开始的计算。获准后再次检查调用方 context，避免已取消请求继续开始工作；取消仍可与后续检查竞争，不承诺成功返回瞬间 context 永不被取消。

实际密码工作同步运行，不用后台 goroutine 伪造中断。开始后即使 HTTP/context 已超时也必须等真实派生或随机源调用结束才释放名额；这不承诺固定返回时限。返回前再次检查原调用方 context，取消则返回零记录/零校验结果，不能保留正确密码的 `Match` 或旧策略升级提示。创建结果的盐/摘要会尽力清空，但仍无完整内存擦除保证。错误密码、底层随机失败、排队失败与取消均须正确释放自己拥有的容量。

该上限只属于**一个共享实例**；同一进程的登录、注册、改密、重置和重哈希须使用同一实例才可称进程额度，不能每请求/账号/策略新建一个。各 API 的静态额度之和及 RSS/CPU 仍须按目标主机实测，未自动提供跨进程总预算或 100 人登录容量。持久化/Redis 租约到期无法停止仍在执行的 Argon2，不能据此回收名额；本项也不提供来源/账号速率限制、公平性承诺、防枚举或正式登录授权。

复用已锁定的 [x/sync v0.24.0 semaphore](https://pkg.go.dev/golang.org/x/sync@v0.24.0/semaphore)（BSD-3-Clause），仅将间接依赖标为直接，版本与 go.sum 不变。[对应源码](https://github.com/golang/sync/blob/v0.24.0/semaphore/semaphore.go)的获取失败不转移名额、成功后调用方负责释放合同已核对；排队人数在调用该库前独立限定。验证、失败及环境证据见[交付日志](../delivery-log.md)；尚无生产参数或负载验收。

### 7.4 密码验证与条件升级用例

`identity.NewCredentialVerifier(store, boundedHasher)` 连接第 7.2/7.3 节的能力，`VerifyPassword(ctx, subjectID, secret)` 以已解析的不透明主体 ID 为内部输入，不将 ID 作为个人登录标识。个人仅邮箱登录、昵称可重复的决定保持；邮箱解析/映射见第 7.7 节，其认证编排见第 7.8 节，普通协议与两步 HTTPS/密钥/准入见第 7.9—7.13 节，默认 API 仍待接入。Store 与共享 BoundedHasher 必须使用兼容的批准策略，所有密码流程共用同一预算实例。

流程为读取 Snapshot → 核对主体 ID、启用状态与有效两代次 → 有界密码验证。Current 匹配只读；Legacy 匹配后，以**同一密码**生成 Current，再按旧两代次执行 `RehashCAS`。两次计算分别准入且均在数据库锁外；调用方在整个用例返回前不得修改密码字节。错误密码、禁用或不存在的主体返回 `ErrCredentials`，没有防枚举耗时保证；非法记录、准入满额/超时、随机失败和存储错误分别保留，不将升级失败降级为验证成功。

升级前拒绝凭据版本溢出，返回的主体/启用状态必须仍相同、凭据版本恰好 +1、认证代次不变。冲突立即失败，不重读或盲重试；两个正确密码验证竞争同一旧版本时可能只有一个成功。随后通过 `State` 重读并比较 ID、启用状态和**两代次**，任一改变即失败。成功结果仅含无密码 State 与本次成功路径的 Upgraded 提示，不输出完整密码记录。

结果仅描述末次读取时的状态，**不是可复用证明、会话发放或资源授权**。读取之后仍可发生改密/撤权，第 7.5 节提供主体行状态协调原语；正式会话仍须接入完整的恢复、名额、结果确认与使用协议，并在后续请求验证当前账号事实。应用用例不创建会话、不更新 Redis，也没有令牌消费、注册激活或权限管理。

错误和取消返回零结果，但升级可能已经提交：CAS 回执错误、升级后的末次读取失败或撤权、成功返回前取消，都不触发回滚或补偿写入。存储操作错误优先保留，不能用 `ctx.Err()` 丢掉未知写入分类；仅在阶段成功后检查取消并阻止后续工作。零结果中的 Upgraded=false 不说明数据库未升级。已覆盖的真实双池竞争、应用调用前屏障、同密码升级及模拟回执错误见[交付日志](../delivery-log.md)，不将这些用例提升为完整 AC24/AC23 或生产登录通过。

### 7.5 会话存储操作的主体状态协调

`postgres.Store.WithCredentials(ctx, id, verifiedRevision, operation)` 比较**两代次**；`WithAccount(ctx, id, authGeneration, operation)` 只比较认证代次，避免同密码重哈希误撤销会话。它们仅是纯主体状态协调原语，**不可单独作为正式会话入口**；正式会话存储必须使用第 7.6 节同时核验恢复代次的新接口。两者都检查启用状态、不解析密码记录；传入版本不是公开登录凭证，调用方不能直接信任浏览器提供的值。enabled 不代表邮箱激活或资源权限，原语不验证密码、期限、会话名额或恢复标识。

复用同一调用方连接池，在显式 READ COMMITTED 事务中按主体主键 `SELECT ... FOR SHARE`，取得锁后检查状态与 context，再同步执行受信的短时会话存储回调。共享锁允许两个操作并行，同时阻止改密、重哈希和禁用的非主键 UPDATE；KEY SHARE 不足以提供这一冲突关系，见 [PostgreSQL 14 行锁](https://www.postgresql.org/docs/14/explicit-locking.html#LOCKING-ROWS)。等待中的门禁取得锁后核对最新行，不能沿用等待前快照。新查询复用主键，无新索引/迁移；行锁仍占用连接、阻塞同主体更新并可能写锁元数据，不能当作零成本普通读取。

持锁范围仅用于已完成密码工作后的有界会话存储操作；不得在回调中运行 KDF、SMTP、任意外部业务、完整 HTTP handler、同主体 Store 重入或逃逸任务，不得提前发布 Cookie/HTTP 成功。回调至多执行一次，接收派生 context 与无密码 State；五秒是合作取消预算，**不是回调或锁持有时长的硬上界**。回调成功后检查取消并完成事务才返回成功；普通错误、取消或 panic 均安排独立一秒的尽力 rollback，panic 继续传播。[pgx v5.11.0](https://pkg.go.dev/github.com/jackc/pgx/v5@v5.11.0#Conn.BeginTx) 的 context 不会自动回滚整个事务，也不能打断回调。

回调开始前的非法输入、缺失/禁用、版本冲突或错误保证回调零次。开始后的回调错误、取消和 Commit 错误统一带 `ErrGuardedOperation`，保留底层 cause；数据库错误仍可识别 `ErrStore`，普通回调错误不伪装成数据库错误。Error 文本固定，不拼接回调私有内容；消费者须先识别“操作可能已发生”，不能因底层又匹配 Conflict/Canceled 就自动重试。没有补偿或结果确认，事务 rollback 不表示外部效果已撤回。

连接丢失时，数据库锁可能先于回调结束释放；已受控验证旧 backend 退出后另池改密提交，旧回调随后仍可产生外部效果标记，包装器最终报未知结果。该标记是受控回调模拟，不是 Redis 实现或网络故障恢复。新会话候选必须携带核验过的主体/认证代次及第 7.6 节要求的恢复标识；后续使用核对当前事实可拒绝旧会话，却不能撤回已执行的外部操作。正式接入还须会话自身的条件写入、名额、结果确认及使用协议，**不能仅凭此包装器宣称 PostgreSQL/Redis 分布式事务或完整生产会话一致性**。

全局恢复状态及同事务入口已由第 7.6 节提供，保持全局行 → 主体行锁序；不能嵌套调用本节方法假装共用事务。旧备份可能重复主体版本，纯主体原语不解决恢复隔离、权限或回档安全事实。真实双向等待、共享锁、取消/连接丢失和自然主键计划的证据归[交付日志](../delivery-log.md)。

### 7.6 正式全局恢复状态与双层协调

迁移 4 新增 `identity_session_recovery` 单例：generation 只能为空串或合法 32 位小写 hex，空串只允许闭门。迁移只插入空代次、disabled 占位，不自动建立可用会话状态；缺失行时普通读取、门禁及 Begin 均失败关闭，不运行时补建。单例主键保证最多一行，增加一份唯一索引存储及全局更新/锁的代价；表只有一行，查询计划可自然选择顺序扫描，不为 generation 另建索引或强制索引扫描。

`RecoveryState(ctx)` 只是维护元数据读取，闭门时也可返回状态，不授予会话。受信 `BeginRecovery(ctx)` 在数据库锁外用系统密码学随机源取得 16 字节、编码为新代次，再更新**既有**行并闭门；随机源失败不访问持久化。它不是外部单调 fence，独立随机性只避免回档计数器的确定性重复。Begin 错误仍可能已经提交，不能自动重试生成新代次；`FinishRecovery(ctx, expected)` 仅当当前代次匹配且仍闭门才开门，旧完成、重复完成及竞争失败均不补救式开门。

Begin/Finish 是维护原语，**不执行或证明**旧 API 隔离、Redis 清理、最新密码/封禁/权限核对。恢复前必须由外部流程独占协调、停止并排空旧实例；数据库恢复后先换新代次并闭门，完成校验/清理后才可显式 Finish。它们不自动清理账号数据，也不构成备份恢复编排。数据库锁丢失时旧回调仍可能执行，第 7.5 节的未知副作用限制保持不变。

新候选使用 `WithSessionCredentials(ctx, id, verifiedRevision, recoveryID, operation)`，已有会话使用 `WithSession(ctx, id, SessionVersion{AuthGeneration, RecoveryID}, operation)`。公共入口严格拒绝空/非法代次；私有实现用明确 scope 区分纯主体和 Session 路径，不会因空字符串退回旧门禁。新路径在**同一** READ COMMITTED 事务先锁全局行、检查 open/预期代次，再锁主体行检查两代次或仅认证代次；回调接收无密码 SessionState，包含已经核验的主体与恢复标识。合作 context、至多一次回调、ErrGuardedOperation/ErrStore、取消/panic 清理与非跨存储事务的合同均沿用第 7.5 节。

恢复代次必须在密码验证**开始前**从 enabled=true 且代次有效的 RecoveryState 捕获，闭门时不启动验证，再随候选贯穿整个验证/升级流程；恢复或冲突后不能读取新代次给旧验证结果重新贴标签。底层 Store 接口不能独自证明调用方遵守顺序，第 7.8 节为个人邮箱密码流程固定该执行路径。真实交错验证了：先捕获 A、密码用例未返回时完成 B、主体两代次不变，A 候选仍拒绝；重新捕获并重启整个验证才可使用 B。这不是完整登录、Redis 名额/条件写入/结果确认或恢复验收。

迁移 4 的 Down 按全局表→主体表→submissions 加排他锁，仅在**确实唯一的空代次闭门占位行**且 credentials/submissions 均空时允许回退；缺失行、已初始化代次（即使已闭门）或任一业务表非空均拒绝，须前滚修复。当前 schema 要求见第 7.7 节；旧 migration-3/4 回归各自固定在版本 3/4，不让新保护掩盖旧保护。完整实测、两种单表保留分支、双向等待及自然单行计划见[交付日志](../delivery-log.md)；未部署，真实 Redis 与生产恢复仍待交付。

### 7.7 个人邮箱映射与原子初始化

2026-10-31 用户确认整个邮箱忽略字母大小写匹配，原始地址保留显示；点号、加号等服务商别名不自动合并。`identity.ParseEmail` 返回私有字段的 Email 值对象，以完整地址的小写形式作 Key，Original 保留精确接受的输入。零值无效，不隐式 trim、不以昵称或 `mail.Address.String()` 代替地址。邮箱是个人信息，不能将完整值或数据库错误详情写入日志。

首版工程支持范围为 ASCII 裸 addr-spec：先用锁定 Go 1.26.11 的 [net/mail](https://raw.githubusercontent.com/golang/go/go1.26.11/src/net/mail/message.go) 解析，再要求无显示名/注释/列表/引号/地址字面量、无空白/控制字节，local 最多 64 字节、完整地址最多 254 字节；域名采用 1—63 字节的字母/数字/连字符标签，不允许空标签、首尾连字符或尾点。没有 DNS/MX 或实际投递验证。标准库解析邮件头且不做 Unicode 规范化，不能直接当产品匹配策略。Unicode 域名输入/IDNA 转换及 SMTPUTF8 暂不支持；符合 ASCII 标签规则的 `xn--` 输入可接受，但不验证 IDNA 合法性。这是实现限制，不是用户取消国际化邮箱需求；不能随意 Unicode 小写或仅折叠域名来替代整址匹配决定。

迁移 5 新增 `identity_personal_emails`：subject_id 主键/外键绑定 credentials；email_key 为 `text COLLATE "C" UNIQUE`，original_email 保留原地址，email_verified 默认 false。数据库限制 ASCII 可见字符、总长度及 Key 与原地址的小写一致，完整产品语法由 Email 值对象检查；读取时重新验证，坏元数据失败关闭。两份唯一索引用于主体和邮箱点查/冲突裁决，新增写入、唯一性检查及索引存储成本；自然 EXPLAIN/BUFFERS 在合成填充上检查两索引和凭据主键，不代表生产容量。昵称不参与唯一性，本次无昵称字段/profile。

`Store.InitializePersonal(ctx, id, email, enabled, currentRecord)` 为受信内部原语：先校验 Email、ID 与 Current 记录；密码须由调用方在事务外经共享计算准入生成。一个 READ COMMITTED 事务通过 sqlc WithTx 插入凭据和未验证映射，第二步失败回滚第一步；已有主体 ID（包括无邮箱的比赛/通用主体）或整址 Key 均返回冲突，不复用、绑定或覆盖旧主体。没有 UPSERT 覆盖、删除、改邮箱或设置 verified 的运行接口；原 Initialize 保持通用主体功能。五秒 context ceiling 和独立一秒尽力 rollback 沿用现有模式；提交/连接错误可能已生效，空结果不证明未写入，不自动重试生成新 ID/邮箱。

`PersonalByID` 与 `LookupPersonalByEmail` 只返回无密码 State、Email 和 EmailVerified 的时点元数据；通用主体没有邮箱时返回 NotFound。它们不验证密码、不发会话、不授权训练。Enabled 与 EmailVerified 独立，验证不能复活封禁；本次只验证事实分离，未接入“未验证不能提交”的业务门禁，不代表 AC25 已完成。公开注册前仍须解决未验证占位保留/清理、重复注册安全恢复/重发、防枚举、令牌原子消费、邮件任务与最终权限复核，不能直接暴露 InitializePersonal 为 signup。

邮箱模型始于schema5，API 要求[匹配 schema](../submission-backlog.md#升级与回退)（后续[积压准入](../submission-backlog.md)、[私有草稿](../problem-drafts.md#私有草稿入库与本人读取)及[参考申请](../reference-requests.md#迁移与回退)）；旧二进制按各自精确版本检查在启动时拒绝新schema，运行中旧进程不会自动停止，须排空/停旧实例、独立迁移后启动匹配新版，无滚动混跑或部署。迁移不为旧主体补邮箱或角色。Down5按凭据→邮箱顺序加排他表锁并拒绝任何非空映射（包括未验证/禁用），需前滚修复；只有邮箱表为空才可移除，保留既有通用凭据、全局恢复及提交数据。此锁序与初始化一致，避免先锁邮箱再删除外键触发器时反向请求凭据锁；不承诺任意并发读取/DDL无死锁，迁移仍须在维护期协调。多个Down不整体原子，Down5成功不豁免迁移4/3的保护。实际初始化分别在第一写/第二写后等待，与Down的提交/回滚交错、原始显示、唯一竞争和无孤儿证据见[交付日志](../delivery-log.md)。

### 7.8 个人凭据到会话门禁的认证路径

`NewPersonalCredentialGate(store, sharedBoundedHasher)` 只接受一个组合 PersonalCredentialStore：同一依赖拥有邮箱映射、凭据读写、全局恢复状态和最终 Session 门禁，内部构造既有 CredentialVerifier，避免误拼不同 Store 的相同 ID/版本。一个接口实例不是物理同库证明；受信实现必须让所有方法使用同一权威数据库/主体命名空间，不能按方法路由到不同租户、缓存或滞后副本。Store 与密码工作使用兼容批准策略，所有流程共享既有计算预算，不在构造器里另建预算或选择生产参数。

`WithPersonalCredentials(ctx, email, secret, operation)` 固定：校验 context/Email/回调 → **只读取一次有效 open 恢复代次** → 邮箱定位主体 → VerifyPassword 的真实验证/可选同密码升级及末次 State → 以原恢复代次和验证返回的完整 Revision 调用 WithSessionCredentials → 在同事务 global→account 门禁内至多执行一次受信存储回调。映射返回的旧 Revision 不作最终凭据证据；Legacy 升级后必须使用新 credential_version。无缓存候选、代次补读/重贴、自动重试、失败升级降级成功或补偿已提交升级。

closed/missing 恢复状态在邮箱查找与 KDF 前失败，格式错误恢复状态及坏映射失败关闭；邮箱不存在、密码不匹配或当前主体禁用均不运行回调。密码字节策略继续由共享 BoundedHasher 检查，无防枚举耗时保证。调用方提供整条路径的期限并在返回前保持 secret 不变；底层每次数据库操作的五秒合作预算不等于整个用例五秒硬上限。回调不得运行 KDF、重入、启动逃逸任务或发布 HTTP/Cookie，沿用第 7.5/7.6 节合作取消、锁丢失及未知外部效果限制。除不存在邮箱/主体等明确归一为 ErrCredentials 的情形外，保留原有错误分类，特别是 ErrStore 与 ErrGuardedOperation 不被 ctx.Err 覆盖；失败不证明升级或回调未生效。

EmailVerified 与 Enabled/密码认证仍分离；本入口供第 7.9 节会话存储受信接入，不决定未验证用户的完整会话产品策略，也不授予训练权限。本层不发布 Cookie；第 7.10/7.11 节的独立 HTTPS 生命周期和两步登录已有分项实现，默认公开 API 尚未挂载。邮件验证或角色/资源权限仍未实现，不能据此宣称 AC01/23/24/25 完整通过。

本次无 SQL、迁移、索引或模块依赖变化，仍精确要求 schema 5；回退需停用依赖此入口的调用方，移除用例即可，数据库无需回退。真实双池验证恢复发生在查找后/验证用例中的 KDF 完成后且末次 State 前/最终门禁前时旧请求均拒绝；应用屏障不冒充数据库锁等待。未知升级用例在真实提交后由 wrapper 注入回执错误，不是真实网络故障。升级/改密/禁用、回调持有全局锁及取消的证据归[交付日志](../delivery-log.md)。未部署，完整登录/恢复与生产容量仍待验收。

### 7.9 普通 Redis 会话协议

`internal/session/redisstore` 提供普通会话核心，复用同一个 Authority 的 PersonalCredentialGate、WithSessionCredentials 与 WithSession。单个权威 PostgreSQL owner、相同 Redis 命名空间/策略是受信实现约束。生产普通策略空闲 2 小时、绝对 12 小时，每个主体＋auth_generation＋随机 recovery ID 最多三个活动名额；credential_version 不分裂名额，同密码 Rehash 不撤销现有会话。允许显式缩短期限用于实验，拒绝放宽普通上界；保持登录、管理和比赛策略尚未接入。

Create 在数据库锁外生成 32 字节随机凭据，完成密码认证并进入双层门禁后读取 Redis TIME，冻结私有、不可修改的 Attempt，再执行首次写入。冻结前失败返回 nil；冻结后所有错误（包括未知 Redis 结果、PG Commit 失败）保留同一 Attempt。不能以新 Create 代替失败重试。只有另一次成功 Check 才得到可发布的 Credential；缺记录为 ErrUnknown，不能断言未写入。Retry 只重放原始元数据/原期限；Touch 改变初始回执后 Retry/Check 冲突而不覆盖，当前 Read 可以继续有效。裸 Attempt 只供受信进程内使用；跨请求的受认证密文格式由第 7.11 节 Login 提供，不能接收浏览器自行填写的 tuple。

Read 不续期；Touch 仅更新仍有效的记录及名额空闲到期，不超过原绝对期限。两者先定位后进入 PG 门禁，并在门禁内重新验证完整 Redis 记录，定位数据不授权。Abandon 可在尚未观察到保存时建立墓碑，Rehash 后仍可清理原尝试，匹配的已 Touch 记录也可撤销；Revoke 针对当前凭据。墓碑与令牌记录保留到原绝对期限，阻挡迟到原写；缺记录的 Revoke 返回 ErrNotFound，不能确认未知期限的撤销。PG/Redis 错误不意味着外部写入回滚，PG 锁丢失后的迟到效果仍需恢复隔离处理。

Redis 只存令牌 SHA-256 摘要。七个 Lua 文件组成六个脚本入口，核对字段数量/长度、版本、主体/两代次/恢复标识、时间关系、绝对 TTL、活动名额及绑定摘要；版本用规范十进制字符串精确比较到 int64 上界。不可变字段的长度前缀 SHA-256 用于发现字段漂移，不是 MAC，也不抵抗任意 Redis 写入或旧快照恢复。Check/Read/locate 声明只读；写脚本保持 Redis OOM 入场拒绝，脚本运行时报错仍可能留下部分写入，必须按未知效果处理。

客户端锁定 `go-redis/v9 v9.23.0`（BSD-2-Clause），沿用实验锁定版本；所需 cpuid/v2 从 2.3.0 提升至 2.4.0，其他传递版本归 go.mod/go.sum。配置显式超时/物理连接上限，禁止隐式命令重试及额外 pipeline 池，一次拨号，禁用维护通知和身份 SETINFO。相关固定版[选项源码](https://github.com/redis/go-redis/blob/v9.23.0/options.go)、[依赖](https://github.com/redis/go-redis/blob/v9.23.0/go.mod)、[许可证](https://github.com/redis/go-redis/blob/v9.23.0/LICENSE)与 [SETINFO 历史公告](https://github.com/redis/go-redis/security/advisories/GHSA-92cp-5422-2mw7)已核对；完整依赖漏洞检查继续由 CI 执行。调用方共享实例并负责关闭客户端、多 API 总预算及整个调用期限。

构造检查当前服务为 Redis 7.2.x standalone primary、noeviction，持续配置和恢复隔离由运维保证；启动检查不能证明后续自动切换/时钟/回档安全。CI隔离夹具固定Redis7.2.17镜像摘要，真实 PG 迁移到当前 schema，协议本身不新增权威会话表。测试含双客户端并发三名额、精确重试、实际TTL、坏字段/索引、超时/OOM、PG锁等待/连接终止与未知结果，以及私有草稿消费者在材料持久化后重新读取会话；注入丢回执、TIME错误和改写测试脚本后的部分写入须与真实网络故障区分。[参考申请](../reference-requests.md)的服务/PG组合测试使用合成会话观察，不借用这些会话专项结果声称申请已接通真实Redis/HTTP。命令见[README](../../README.md)，实测归[交付记录](../delivery-log.md)。协议回退须先停止依赖调用方并隔离/换代次处理旧会话空间，schema各自按所属合同回退，不盲目复用旧数据。完整HTTP/SCS生命周期、验证邮箱与训练权限、轮换/其他策略、恢复编排、生产容量及完整AC23/M0均待验收。

### 7.10 已有普通会话的 HTTPS 适配

`internal/session/httptransport.New(store, Config{Origin, Timeout})` 只适配同一 `redisstore.Store` 的已有普通会话。独立 Handler 提供 `GET /api/v1/session`（Read）、`POST /api/v1/session/activity`（Touch）及 `POST /api/v1/session/logout`（Revoke）。默认 Gin/API、Compose 和公开 OpenAPI **未挂载**这些路径；本组测试先经受信 Create/Check 取得凭据，本构造器不发放登录 Cookie。跨请求候选恢复与登录发放见第 7.11 节的单独构造器，错误后不可盲目新建 Create。不引入 SCS 自动 LoadAndSave、第二份会话状态或隐式保活，后续 SCS/Gin 完整生命周期验证路线保持待验。

构造器冻结无路径的可信 HTTPS origin 和大于零、至多五秒的合作调用期限；每个请求必须直接 TLS 且 Host 精确等于配置 authority。直连模式忽略 `Forwarded`/`X-Forwarded-*`，当前反代终止 TLS 的默认 Nginx 不兼容此约束；可选单跳 mTLS 入口见第 7.14 节，不得伪造 `r.TLS` 绕过。所有结果（含 401/403/404/405/503）为 `Cache-Control: no-store`，无 CORS 许可。安全方法只允许明确的 GET，HEAD/OPTIONS/写接口的 GET 不触库。

POST 在触库前调用标准库 [CrossOriginProtection.Check](https://pkg.go.dev/net/http@go1.26.11#CrossOriginProtection.Check)，并额外要求至少一个正向来源证据：唯一、非空、精确匹配配置的 `Origin`，或唯一 `Sec-Fetch-Site: same-origin`。任一出现的头都须合法，拒绝跨站、same-site、none、null、空值、多值及相互矛盾的组合；两头皆无也拒绝。非浏览器客户端须显式提供来源，头本身不证明身份。该防护仅覆盖本 Handler 两个 POST，不代表全站 CSRF 或真实浏览器验收通过。

仅从 Cookie 读取 `__Host-oj_session`；标准库逐行严格 ParseCookie，再跨所有行要求恰一个同名 Cookie，凭据须为规范 256-bit base64url。其他畸形 Cookie 同样失败关闭，不接受 query/body/Authorization 替代凭据。读取和活动响应仅含主体 ID 与创建/空闲/绝对期限，不暴露令牌、认证代次或恢复标识，不发送 Set-Cookie；身份时点观察不授予验证邮箱、训练、角色或资源权限。活动是显式 POST，后台轮询不能调用它无限保活；实际产品活动识别、采样和限流另验。

只有 Revoke 返回成功且调用 context 仍有效才返回 204，并以相同名称、`Path=/`、无 Domain、Secure、HttpOnly、SameSite=Lax、Max-Age=0/过去 Expires 清理 Cookie。缺记录不代表确认撤销；任何失败不清 Cookie。两种 ErrStore、恢复闭门、明确未知结果、取消/超时优先 503。排除上述错误后，明确 Redis 失效/缺记录即使带 `ErrGuardedOperation` 也返回 401；401 只表示会话未被接受，不是退出确认或外部效果回滚证明。其他受保护错误保持 503，未受保护的账号失效返回 401；其余未知错误为 503。不自动重试、补偿或泄漏内部错误。合作期限不是强制终止或撤回外部效果。

真实专项复用原私有 Redis/PostgreSQL 夹具和两个 TLS 测试服务器；合成 Cookie 由测试注入，没有浏览器 Cookie jar、跨浏览器关闭/重开、正式反代或公开账号流程验收。HTTP 单元测试覆盖来源/Cookie/方法前置拒绝及错误优先级；真实状态核对轮询不变、显式续期、跨入口退出、实际撤销后适配器丢回执与恢复拒绝。运行命令沿用 README 普通会话专项，证据归[交付记录](../delivery-log.md)。无依赖、SQL、迁移或 schema 变化；回退停用调用方并移除适配，不回滚数据库。未部署，完整 AC01/23/24/25/M0、生产容量与恢复编排继续待验收。

### 7.11 跨请求登录尝试与 Cookie 发放

`redisstore.NewLogin(store, LoginConfig)` 在同一权威 Store 上增加两步恢复。`Prepare` 完成既有邮箱密码认证、捕获原恢复代次和完整凭据版本、Redis TIME 冻结原 tuple，**不写会话或名额**；只有最终 PostgreSQL 门禁成功且 context 有效才封装返回。首次响应丢失可以重新准备，因为没有会话写入；已提交同密码升级仍可能保留。`Complete` 解封原候选后只执行原样 Retry，再独立 Check；仅两次成功且 context 有效后发放普通 Cookie。错误不新建 token、不重贴恢复代次、不补偿或重试一个新的 Create。相同候选并发确认可重复返回同一 token；HTTP 无法保证响应或 Cookie 恰好发送一次。

候选用锁定 Go 1.26.11 标准库 `crypto/aes` 与 [`cipher.NewGCMWithRandomNonce`](https://pkg.go.dev/crypto/cipher#NewGCMWithRandomNonce) 做 AES-256-GCM，随机 96-bit nonce 随密文保存。明文仅含私有 Attempt tuple、原凭据及冻结截止，不含密码；Redis 继续只保存 token 摘要。封装认证绑定用途/格式版本、外层 key ID、namespace、精确 HTTPS origin、idle/absolute 策略与 Window。只接受规范 base64url、受限 key ID、至多 1536 字节封装；解析浏览器语法不证明密文有效或身份授权。没有公开可填充的 Attempt snapshot，也不是 JWT 或第二份权威 Session。

密钥必须是跨副本/进程共享的 32-byte 材料，不在启动时自动生成或回退。仅当前 key 用于封装，最多四个 key 可解封，构造器复制 key map/材料并拒绝全零材料；文件加载见下节，不提供环境变量回退、分发或自动轮换。操作者负责每个 key 的跨副本累计封装次数不超过标准库规定的 `2^32`，以及安全配置。保留旧解密 key 才能恢复/放弃旧候选；如需保留原期限内协议允许的清理能力，应覆盖该候选期限保留旧 key。提前丢钥不等于撤销，会使候选失败关闭；已经写入但未收到 Cookie 的会话仍按原 idle/absolute 自然失效，不能承诺立即释放名额。

Window 必须显式给定，粒度毫秒，大于零且同时不超过五分钟和普通 Idle。`loginUntil = 原 Redis created + Window` 不随封装、重试、确认或副本重建延长；Retry/Check 的实际 Lua 执行在所有成功分支之前检查它。新可选协议参数只接受规范正整数或既有裸 Attempt 明确传入的 `0`，非法/缺失值不降级为不限期。截止仅由 AEAD 保护并作为受信参数传入，不增加 Redis 记录字段或改变原 binding，因此已准入会话的 Read/Touch 不需要密文。原绝对期限和初始 idle 保持不变；登录窗口到期仍可用保留密文请求 Abandon，但须遵守原全局/主体门禁及初始 idle/absolute 校验。不存在的记录在初始 idle 已过后拒绝建立墓碑；已有匹配记录的墓碑保留到原 absolute。过期密文不能续用来登录。

`httptransport.NewLogin(manager, Config)` 复用第 7.10 节的 direct TLS、固定 Origin/Host、正向同源证据及 no-store。同一 manager 转发已有会话的 Read/Touch/Revoke，避免误拼不同 owner。它增加以下 POST，默认 Gin/API、Compose 和公开 OpenAPI 仍**未挂载**：

| 路径 | 合同 |
| --- | --- |
| `/api/v1/session/login/begin` | 至多 4096-byte JSON，恰含字符串 email/password；拒绝重复/未知字段、尾随数据、无效 UTF-8 或未配对 Unicode surrogate。复用既有 Unicode 校验的单一实现 `internal/jsoninput`，密码按字节送共享计算预算，不修剪/规范化。成功仅 202 与 `__Host-oj_login`，不发会话凭据 |
| `/api/v1/session/login/complete` | 从唯一尝试 Cookie 恢复原候选；成功 204，发 `__Host-oj_session` 并清尝试 Cookie。普通 Cookie 为 Secure/HttpOnly/SameSite=Lax、Path=/、无 Domain、无持久 MaxAge/Expires；token 不进正文 |
| `/api/v1/session/login/abandon` | 只撤销密文指定候选；成功 204 且仅清尝试 Cookie，不清或覆盖另一个会话 Cookie。失败不能宣称已放弃 |

尝试 Cookie 为同样的安全 host scope，MaxAge/Expires 按原截止剩余时间给定，不按新 Window 重置；HTTP 发送前已到期则拒发，实际截止继续由 Redis 校验，浏览器秒级期限不构成额外授权。合法持有的密文是短期敏感恢复能力，不能写入日志、查询参数、local/sessionStorage 或业务草稿。所有失败都不发/清 Cookie：确定输入错误为 400，类型/体积为 415/413，凭据/旧候选失效为 401，名额/候选冲突或有效旧会话为 409，存储/恢复/取消/超时/未知结果优先 503。共享 KDF 名额、有限排队和底层调用期限保持原合同；密码超长不伪装服务故障，没有防枚举或集中登录容量准入结论。

Begin 已见尝试 Cookie 时拒绝再准备；Begin/Complete 已见有效普通会话时拒绝覆盖，确定失效的旧会话可重新认证，未知读取结果仍失败关闭。准备/确认失败不清旧 Cookie；直到确认成功才替换它。没有 Cookie 的并发 Begin 可产生不同、尚未写入的候选，不提供浏览器级唯一 pending、最新用户意图排序、遗失所有候选清理或服务端 pending 索引。尝试 Cookie 到期/丢失可能丢失恢复能力；未知已写会话最多按原 idle/absolute 占位。Abandon 后已通过 Check 的旧响应仍可能迟到并设置已撤销 Cookie，后续 Read 拒绝；没有撤回 HTTP 响应或跨存储原子发布保证。现有 Touch 改变回执后原 Complete 冲突，不能生成新候选掩盖结果。

跨 API/进程重建要求同一 PostgreSQL owner、Redis namespace/连续性、key ring、Origin、策略及 Window；密文不可任意搬到另一配置。平台回档/旧快照/账号事实恢复继续遵守原全局恢复隔离，不凭密文绕过。共享时间配置异常可能使 HTTP 端提前拒发；以 Redis 的冻结窗口和普通期限为最终准入边界。回退先停止新登录调用方，已有普通记录格式及 schema 5 不变，无 SQL/迁移/依赖变更；未完成候选停止恢复，不用数据回退“修复”。

验证命令沿用 README 普通会话专项；两独立 Store/连接池/TLS 入口、真实 Cookie jar、进程内状态丢弃后的 manager 重建、未知回执、执行点截止、墓碑/发放交错、改密/禁用/恢复和输入边界归[交付记录](../delivery-log.md)。Cookie jar 不是浏览器 UI/关闭重开或真实反代验收；默认入口、SCS/Gin 完整生命周期、邮件/资源权限、生产限流阈值/预算/容量和完整 AC01/23/24/25/M0 继续待验，未部署。

### 7.12 登录密钥文件加载

`LoadLoginSecrets` 接受显式绝对规范路径，`NewLoginFromSecrets` 只把不透明快照交给原构造器，仍要求显式 Origin/Window；不挂载默认 API。Linux/Darwin 使用已锁定的 `golang.org/x/sys/unix` v0.49.0（BSD-3-Clause），逐级 openat/O_NOFOLLOW 打开并 fstat；其他平台明确拒绝。所有目录须 root/当前 euid 所有且不可被组/其他用户写，唯一例外是 root 所有的 sticky 公共祖先，每个子组件仍独立检查。拒绝路径中的符号链接，包括 Darwin 的 `/tmp`、`/var` 别名；调用方须配置真实规范路径。

叶文件须 root/当前 euid 所有、单硬链接、普通文件且仅 0400/0600，大小 1—4096 bytes。非阻塞打开避免 FIFO 挂起；有界读取前后核对 fd 的设备/inode/权限/所有者/链接/大小/mtime/ctime，错误统一为不含路径或底层错误的 `ErrLoginSecrets`。这些检查依赖受信本地文件系统和正确 ACL，不证明 NFS/ACL 或持钥特权进程安全，也不能证明恶意可信写入者下的原子快照。以新文件原子替换并显式重建 manager，禁止运行中原地改写；已加载 manager 不自动重读。

JSON 必须恰有 `version`（整数词法 `1`）、`current`（key ID）、`sealing`（1—4 个 ID 到 key 的对象）、`limiting`（独立稳定 key）。每个 key 为 32 bytes 的规范无 padding base64url；拒绝零值、重复材料/用途复用、未知/重复/缺失/null 字段、坏 Unicode、尾随数据及超限内容。示意仅用占位符，不是可用配置：`{"version":1,"current":"active","sealing":{"active":"<32-byte raw base64url>"},"limiting":"<distinct 32-byte raw base64url>"}`。快照直接值/指针的已支持合法格式脱敏，JSON 无私有字段，完整输出边界见第 7.16 节；内存清理属尽力而为，Go 分配器/GC 不提供密钥完全擦除保证。

封装轮换保留旧 key 并更新 current；限流 HMAC 材料须独立且跨副本/封装轮换稳定，加载器本身不执行限流，共享准入见下节。缺文件/坏权限/坏配置失败关闭，无自动生成、chmod、环境回退或秘密日志。未读取/创建真实生产密钥；生产路径、分发、备份、轮换操作和阈值仍待实际部署准备。无 SQL/迁移/会话记录变化；停用加载调用方即可回退。测试与验证证据见[交付记录](../delivery-log.md)。

### 7.13 共享登录限流

`redisstore.NewLimitedLogin` 将原 Login、Store/连接池/namespace 与文件快照的独立 HMAC key 绑定；`httptransport.NewLimitedLogin` 要求该配置存在。旧 HTTP `NewLogin` 接收已配置 manager 时同样自动执行准入，不能经旧入口跳过；未配置旧构造器仅保留参考/测试兼容。核心 Prepare/Complete/Abandon 不推测网络来源，直接调用者必须先准入；后续默认 API 必须选择严格构造路径。本轮不挂载 Gin/Compose/公开 OpenAPI，不选择生产阈值。

五个 `LoginBucket` 必须显式配置 Count/Window：Begin 全局、来源、规范邮箱；Complete/Abandon 共用另一组全局、来源。窗口为整数毫秒 1ms—24h，额度 1—1,000,000，均是实现上界而非生产建议。HTTP 在 TLS/Origin/Host/方法、Cookie 格式、Begin 严格 JSON/邮箱之后，在已有会话 Read/密码计算/候选恢复之前准入。Begin/Complete 的旧 session token 廉价格式检查也先完成；Abandon 不读取或授权另一个 session。直连来源只接受实际 RemoteAddr 的 IP:port，拒绝 zone/无效地址/零端口，IPv4-mapped IPv6 归一为 IPv4；直连入口忽略代理头，显式代理模式见第 7.14 节。

沿用已锁定 go-redis 和无重试、显式连接/超时配置，无新依赖。评估 [go-redis/redis_rate v10](https://github.com/go-redis/redis_rate/blob/v10/rate.go) 的单 key GCRA 后，因三维联合拒绝须全无写入而采用短小受限 Lua 业务事务；执行 flags/错误边界核对 [Redis Lua 文档](https://redis.io/docs/latest/develop/programmability/lua-api/)，非自写网络/加密协议。标准库 HMAC-SHA256 以独立材料和用途/namespace/操作/维度绑定 Redis key；邮箱仅用既有 Email.Key，Redis 不保存原始邮箱、IP 或密码。Begin 与恢复预算分离，封装密钥轮换不重置预算；不按登录成功、错密码、账号存在性分桶或退款。

每 namespace 固定一个持久 config key，值含版本与覆盖全部五维参数/限流 key 的 HMAC 指纹，不把指纹放进 key 前缀来绕开旧全局额度。`InitializeLoginLimits` 是显式部署准备操作：仅对已隔离、确认初始状态的 namespace 建立不存在的标记，相同无 TTL 标记幂等，已有不匹配/损坏拒绝；不证明初始 namespace 无旧数据或恢复安全。构造器只读，实际准入每次检查标记值、类型与无 TTL；缺失/改变/损坏均 503，不自动创建、清理或重配。

每次只执行一次 EVAL（standalone/no-cluster，不启 allow-oom），Redis TIME 决定固定窗口。所有维度先核对类型、规范整数计数、窗口对齐、期限算术及真实 PEXPIRETIME，再决定准入；损坏优先于额度耗尽。到期相等时物理 key 仍可能存在，完整验证后 `now >= deadline` 逻辑视空；存活窗口必须匹配当前窗口。任维耗尽统一 429，所有计数/key/TTL 不变，不公开维度、余额或按邮箱计算的 Retry-After。成功按全局→来源→邮箱顺序记账，每个 SET 同时附 PXAT，后续脚本错误仍使已写计数有界到期；Lua 不保证错误回滚，不补偿、不退款、不自动重试。超时/断连/OOM/坏状态统一 503，均不发/清 Cookie，不执行后续 KDF 或会话操作；迟到计数仍可能落地。

全局额度限制成功准入造成的来源/邮箱 key 基数，拒绝不创建或续 TTL，过期自然回收；除单个 config 外无永久记录。不同窗口时，保留量仍须按较长 TTL 内可接受的全局窗口数核算，不把单窗额度当总内存上界。固定窗口边界允许约两倍短时突发；请求额度不替代 BoundedHasher 的单进程计算/排队上限、多 API 总 CPU/内存/连接预算或入口抗 DDoS。校园共享出口、分布式来源、针对目标邮箱耗尽导致的可用性和公平性仍需生产参数/压测；不声称完整防枚举或响应耗时保密。

配置/HMAC 改变会使不一致副本失败关闭，不能用在线 Initialize 重置额度。变更、Redis 丢数据/旧快照/时钟异常或回退须先隔离全部入口并执行明确恢复/重配流程；限流标记不能替代已有持久化账号恢复代次，不能证明旧快照连续。HMAC 更换/namespace 更换及计数清理不自动实施，可能影响既有候选和会话，部署时另验。无 SQL/迁移/会话记录改动；回退须停用新登录，不能以接回未限流入口维持公开登录。合成故障/真实存储、HTTP 证据归[交付记录](../delivery-log.md)。

### 7.14 单跳受信反向代理

`httptransport.NewProxiedLimitedLogin` 为独立可选入口，要求同一已配置限流的 Login、固定 HTTPS Origin/调用预算及 `ProxyConfig.CertificateSHA256`：1—16 个互不重复的非零客户端叶证书 DER 的 SHA-256。构造时复制指纹，轮换以显式重建 Handler 生效；可先重建为新旧并存，切换代理后再移除旧指纹，不自动加载、分发或续签证书。原 `New`/`NewLogin`/`NewLimitedLogin` 仍保持 direct TLS/socket 来源，不会因请求带转发头进入代理模式。

代理模式在全部会话与登录路由、方法/CSRF/业务调用之前检查：真实 backend TLS、固定 `Request.Host`、首个 PeerCertificate 指纹命中且与某条非空 VerifiedChains 的首证书一致。后端服务器必须配置正确的客户端 CA 及 `tls.RequireAndVerifyClientCert`；仅有 PeerCertificates、任意 verified chain、有效但未列入指纹的客户端或明文均不足。代理模式不回退直连；进入 Handler 后的信任拒绝统一 403/no-store、无 Cookie/准入/存储操作。无证书/不受信证书等在 TLS 握手阶段失败，不产生 HTTP 403。证书身份用于确认受信代理，不把 Worker 证书或同 CA 任意客户端自动授权为代理。

认证代理必须删除客户端传入的转发头，从实际客户端连接覆写：恰一个 `X-Forwarded-For` 裸 IP（无端口/zone/列表/空白，拒绝 unspecified/multicast）、恰一个 `X-Forwarded-Proto: https`、恰一个与固定 public host 完全相等的 `X-Forwarded-Host`；同时保留经校验的原公开 Host。拒绝任何 `Forwarded`/`X-Real-IP` 字段，不支持多跳追加链或头优先级猜测。IP 归一 IPv4-mapped 地址后仅以请求局部值传入共享限流，不修改 RemoteAddr/TLS 或存入共享 handler 状态。固定 Origin、Sec-Fetch-Site、Cookie scope、两步候选和恢复期限继续按原合同执行。

应用无法独立证明受信代理面向客户端真的使用 HTTPS，持有有效代理私钥者可伪造来源；公开端 HTTPS/Host 校验、后端隔离、CA/证书分发与访问边界须在真实部署另验。测试复用标准库 `httputil.ReverseProxy.Rewrite/SetXForwarded`，在 hop-by-hop 清理之后重建可信头，未复制入站 XFF；`SetURL` 后显式恢复已验证 Host，并删除 X-Real-IP。不提供通用代理协议解析器、CIDR 信任、动态 Origin、明文 backend 或生产 Nginx 配置。

真实 Redis/PostgreSQL 专项含两个独立 Store、公开 TLS→mTLS backend、两个实际 Linux loopback 客户端来源、同源跨副本限流/异源隔离、恶意转发及 Connection 头、共享 Go Cookie jar 登录/读取/活动/退出，以及同 CA 未授权证书和无证书拒绝。合成 TLS 状态的单元边界与真实握手明确区分；Cookie jar 仍不代表浏览器验收。命令沿用 README 专项，实际结果归[交付记录](../delivery-log.md)。无依赖/SQL/迁移或默认 API 接线；回退先停用代理登录入口，不以接回未限流路径替代。未部署，生产阈值、浏览器/完整账号权限与恢复和完整 M0 仍待验。

### 7.15 显式配置与运行时装配

`internal/session/runtime.Open(ctx, Config)` 接受受信的 Go typed 配置，拥有一个身份 PG pool、一个 Redis client、一个 Hasher/共享 BoundedHasher 和严格限流 Handler。外部只得到 `ServeHTTP`/`Close`，无裸 Store、client、pool 或绕过排空的 Handler。每个 API 进程应只构造一个 Runtime，后续账号工作也须共享计算准入；多个 Runtime/进程的预算不会自动合并。它不解析 JSON/环境配置文件或启动监听器；显式命令接线由第 7.17 节提供，默认 Compose/OpenAPI 未启用。

Config 显式给定 KDF 当前/旧完整 tuple、执行上限、准入人数/队列期限、五个登录额度、namespace、私有密钥文件、固定 public HTTPS Origin、HTTP/整个启动预算、direct TLS 或 pinned mTLS 模式。普通会话固定 2h/12h；零/未知模式、direct 携带代理 pins、缺预算和 owner 配置不合法均拒绝。HTTP Config/ProxyConfig 与 Redis setup 的纯校验复用原构造器规则；KDF/channel 的 1024 在途/65536 等待是实现上界而非生产推荐。StartupTimeout 为大于零、至多 1 分钟的合作预算，从本地准备前开始，准备后及发布前重新检查；同步文件/driver 准备不能由 context 强制打断，取消不证明底层已撤销。

DatabaseConfig 不接受任意 DSN/callback/备用端点：显式 Host/Port/Database/Username/Password/ApplicationName、连接数 1—1024、整毫秒 1ms—5s 调用期限、最大寿命/空闲时间/健康检查周期。用锁定 pgx v5.11.0 官方 parser 的无秘密常量模板建立合法配置，禁止它消费的 24 个非空 PG 环境设置（完整列表随源码维护），不修改进程环境；PGDATA/PG_MAJOR/PG_VERSION 等非连接镜像元数据不受此限制。模板显式禁用 TLS 文件/指定空 passfile，随后成组替换端点/身份/TLS 并清空 Fallbacks，防止残留明文降级；parser 仍可能 stat 默认路径，不加载其中隐含凭据/证书。受控 RuntimeParams 固定 public search_path、应用标签及显式 statement/lock/idle-in-transaction 期限；pool 最小/最小空闲连接为零，所有数量与时间上限冻结在配置。Redis 使用原显式预算/无重试 ClientConfig，不读取其他隐含设置。

PG/Redis 分别显式选择 PrivateTransport 或 VerifiedTLS。前者要求无 TLS config，仅适用于已验证的受信私网/本地连接，本组件不自动证明网络私密；后者要求 MinVersion 至少 TLS1.2、明确 ServerName、非 InsecureSkipVerify，克隆 TLS 和 RootCAs，无明文 fallback。调用方仍须管理并保持 TLS backing material 不变。Config/DatabaseConfig 的值和指针支持已测试的 16 种合法值格式（含数值动词）及 JSON 固定脱敏；非法 `%w` 等 Go 诊断、反射和主动字段访问不在此保证内，原始 Config 禁止进入日志或无关编码器。公开启动错误为固定配置/密钥/不可用分类或 context 取消/期限，不暴露路径、凭据或 driver cause。

Open 只读检查当前 schema、有效 open 恢复状态、原 Redis 7.2 standalone/primary/noeviction 条件及已存在且匹配的限流 marker；失败释放自有资源，不迁移、不初始化 marker、不 Begin/FinishRecovery、不轮换 key。精确schema切换须在维护窗口排空并停止旧API、Worker及维护写入，不混跑；[Down17](../problem-drafts.md#审核退回与重送)仅允许空退回账本；[Down16](../problem-drafts.md#审核人分配与证据摘要)仅允许空访问历史；[Down15](../problem-drafts.md#作者送审回执)仅允许空送审账本；Down14 仅允许无对拍计划时回退；Down13 仅允许空领取历史回退；Down12 仅允许空报告与终态回退；Down11 仅允许空预留表回退；Down10 仅允许空计划表回退；Down9 仅在参考申请历史与policy均为空时允许，详见[迁移与回退](../reference-requests.md#迁移与回退)。本次没有迁移现有持久化localdev或部署服务器。成功描述启动时的观察，不证明持续可用、备份连续、生产参数或外部恢复隔离；每个请求仍执行原权威/限流门禁。

可选`PrivateDrafts`使用同一PG池/Redis会话权威组装[私有草稿HTTP](../problem-drafts.md#私有草稿入库与本人读取)，显式预载受信候选与独立来源锁、打开Linux私有材料根；不暴露底层客户端。全部资源随同一Runtime排空后关闭，启动部分失败释放已加载的Prepared/Local。省略能力保持原路由集，不自动授予出题权或创建目录。

Close 在同一 mutex 下停止入口和登记活动请求，503/no-store/无 Cookie 拒绝后续请求，等待**实际 ServeHTTP 返回**后再恰一次关闭资源。运行中 KDF 超过请求期限仍被计入；每个 Close 调用仅用各自 context 等待同一完成信号，超时不提前关资源、不重新开放、不遗弃后台排空。先停止 listener，再等待服务器请求结束并 Close；单独 server.Close 不证明业务/KDF 已退出。零值 Runtime 失败关闭，Handler panic 仍注销活动计数，由服务器/上层既有恢复层处理 panic。

真实 PG/Redis 专项验证双 Runtime/两步 Cookie 生命周期、Legacy 升级、TLS→mTLS 代理、缺失/不匹配 marker、闭门恢复、错 schema/Redis 失败、真实表锁等待后的启动取消/期限、精确 backend 消失及 PG/Redis 连接释放；确定性阻塞 Handler/取消、并发关闭和 panic 以 unit race 验证。数据库 TLS 最终配置有断言，显式命令/文件解析见第 7.17 节；真实存储 TLS 握手、浏览器/生产代理、生产参数/多 API 总预算、完整权限/恢复和 M0 另验，未部署。命令沿用 README backend/会话专项，实际结果归[交付记录](../delivery-log.md)；回退停用装配调用方，无 SQL/迁移/依赖变化。

### 7.16 会话敏感对象的输出保护

`Credential`、`Attempt`、`Continuation`、`LoginSecrets`、`LoginConfig`、`Login` 以值接收器实现 fmt.Formatter，直接值及非 nil 指针在已测试的 16 组合法值格式下只输出既有固定脱敏标签；包括原 Stringer 不覆盖的数值动词。宽度、精度和引用样式不应用到敏感表示，typed nil 保持 `<nil>`。这修复了数值动词反射展开 token/key 数组的问题，不改变显式 `Value()`、凭证解析、密文候选恢复、key 轮换或会话存储。

`LoginConfig` 是受信 Go 构造输入，不是备份/导出或密钥文件格式。其值/指针标准 JSON 输出改为固定字符串 `"[redacted login sealing configuration]"`，标准 slice/map/导出结构字段编码同样不暴露 key map；typed nil 保持 `null`。其余五种 opaque 类型保留 `{}`/typed nil `null`。当前仓库没有业务消费 LoginConfig 的 JSON；第 7.12 节私有密钥文件与密封 payload 使用独立结构，格式均不变，无数据迁移或回滚步骤。回退代码会重新暴露旧输出风险，不能据此恢复秘密日志/配置 dump。

范围仅限声明的直接值/指针合法格式和上述 JSON 行为。非法 `%w` 等 Go 诊断、反射、绕过类型方法的重新包装和主动字段/`Value()` 访问不在保证内；任何秘密对象仍禁止进入日志、错误或无关编码器。本保护降低意外输出风险，不提供日志安全隔离或完全内存擦除。回归使用真实 AEAD/admission 的合成对象，逐字检查固定输出及 nil/JSON 容器，并核对原凭证/密文 roundtrip 与 key 未变化；故障信息只含类型/动词，不输出敏感表示。实际 backend/race 与真实协议专项结果见[交付记录](../delivery-log.md)，默认 API、浏览器/生产部署与完整 M0 仍待验。

### 7.17 显式会话 API 入口

`cmd/api` 在非空 `OJ_SESSION_CONFIG_FILE` 下调用 `apiapp.Run`，一个进程只拥有一个 Runtime 和一个 HTTPS listener。必须同时显式配置 `OJ_API_ADDR`（数字端口，可用 0 供隔离测试），与实验 token 模式互斥；这些检查先于配置、秘密和网络访问。未启用时保留 bootstrap/实验模式。启动参数和密码环境合同、镜像入口见 [README](../../README.md#显式启用普通会话-api)。基础能力开放六个会话路径及健康检查，可选改密能力见第 7.18 节；不开放注册、邮件或训练资源。

`apiconfig.Parse` 是无 I/O 的版本 1 JSON 解析器，不接受 DSN、内联密码/key、任意环境名或回调。文件至多 64 KiB、一个根对象；拒绝未知字段、重复解码字段名（包括转义别名）、非小写 snake_case 名、null、错误 Unicode、整数溢出、尾随内容、深度超过 12、单数组超过 16 项或总字段超过 256。时间必须是显式 Go duration 字符串，数字零不替代遗漏预算。下表列出完整配置层次；空数组仍须显式给定，`tls` 对象不能省略。

| 层次 | 字段及约束 |
| --- | --- |
| 根 | `version: 1`、`mode: "direct"/"proxy"`、`namespace`、`secrets_file`、`origin`、`login_window`、`startup_timeout`、`http_timeout`；原生 owner 校验及普通 2h/12h 不变 |
| `database` | `host`、`port`、`database`、`username`、`application_name`、`security`、`auth`、`connections`、`timeout`、`max_lifetime`、`max_idle`、`health_check`、`tls`；端点/池及 PG 环境规则见第 7.15 节 |
| `redis` | `address`、`username`、`security`、`auth`、`connections`、`timeout`、`tls`；沿用有限连接/无重试；`auth: "none"` 禁止非空 username |
| `password` | `current`、`legacy: []`、`limits`、`admission`；current 与每个 legacy 含 `memory_kib`、`iterations`、`parallelism`、`salt_bytes`、`digest_bytes` |
| `password.limits` | `max_memory_kib`、`max_iterations`、`max_parallelism`、`max_password_bytes`、`max_salt_bytes`、`max_digest_bytes`；沿用密码 owner 与固定数据库记录上限 |
| `password.admission` | `max_in_flight`、显式 `max_waiting`、`max_queue_wait`；无等待时后两者为 0 与 `"0s"`，其他组合按 owner 校验 |
| `login_limits` | `begin_global`、`begin_source`、`begin_email`、`recovery_global`、`recovery_source`；各含 `count`、`window`，须匹配已初始化的共享 marker |
| 根 `tls` | `certificate_file`、`key_file`；direct 不带 CA，proxy 必须另给 `ca_file`；listener 禁止 `server_name` |
| `proxy_fingerprints` | direct 为 `[]`；proxy 为 1—16 个唯一非零、64 字符小写十六进制叶证书 DER SHA-256 |
| 存储 `tls` | private 模式为 `{}`；tls 模式须有 `ca_file`、`server_name`，可另给成对 `certificate_file`/`key_file`；禁止明文 fallback/跳过校验 |
| `server` | `read_header_timeout`、`read_timeout`、`write_timeout`、`idle_timeout`、`shutdown_timeout`、`max_header_bytes`；header/read/write/shutdown 为正且至多 1m，idle 至多 10m，read ≥ header，write ≥ HTTP 期限，header 1 KiB—1 MiB |
| 可选 `private_drafts` | `material_root`、`bundle_directory`、外置`source_lock_file`、1—10唯一`source_ids`、正且至多1m的`timeout`、1—10的`max_in_flight`；write ≥ 草稿调用期限。省略关闭，导入/本人预览共享材料段预算；可选 `reference_requests: true` 单独启用本人申请，不读取材料或自动配置额度。路径/权限见[README](../../README.md#显式启用私有草稿-http) |

数据库/Redis 的 `security` 只能为 `"private"`/`"tls"`，`auth` 只能为 `"none"`/`"env"`。后者分别从固定的两个密码环境读取非空值；前者拒绝对应非空残留。解析结果不包含环境获取副作用，配置/解析错误只给固定类别。解析夹具使用合成参数，不能直接作为生产 KDF、限流或资源推荐。

配置、公开证书/CA 与原登录密钥复用 `checkedfile` 的逐级 fd 校验。配置/公开材料允许组/其他只读，禁止执行/特殊位/组或其他写；私钥及登录密钥仅 0400/0600。所有者、目录、单硬链接、无符号链接、读取前后快照及可信本地文件系统限制不变；路径均为绝对规范路径。公开证书/私钥各至多 64 KiB、CA 至多 1 MiB；读取后尽力清理字节。TLS 使用标准库验证，最低 1.2，无自定义弱 cipher；启动核对叶证书有效期、匹配私钥及相应用途。proxy 在同一监听器 `RequireAndVerifyClientCert`，应用再核对 pin；健康检查同样要求真实 mTLS。证书不热重载，轮换须重启并协调多副本。

启动先注册信号，再在同一累计启动期限内读取有界配置/材料、构造 Runtime 和绑定监听器，阶段间检查取消。同步文件读取不能强制中断；Runtime 只检查 schema/恢复/marker，不初始化或修复。bind 或启动取消失败用有限清理预算关闭已构造 Runtime，未准备完成不记录 ready 地址。会话路由对六个完整路径及可选改密/私有草稿路径的所有方法直达同一Handler，保留原Host/TLS/RemoteAddr，不使用Gin前缀重定向；草稿item为单段32位小写十六进制ID，题面样例预览仅可追加精确`/preview`，方法、CSRF和Cookie合同由各能力检查。panic在响应提交前固定503且清Cookie，提交后由net/http中止连接，不输出原异常。

SIGTERM/interrupt 后 listener 停止接收，网络 Shutdown 与 Runtime.Close 共用**同一个总退出预算**；Runtime 同时拒绝新工作，并等待实际 Handler/KDF 返回才关存储。网络超时仍启动 Runtime 排空；退出预算耗尽返回非零，不记 graceful，不以强关连接冒充业务排空。只有网络与 Runtime 均确认完成才记 `API stopped gracefully`。启动取消且清理成功可正常退出，其他启动/监听/退出失败非零。日志不输出配置、文件路径、密码、原始 TLS 握手诊断或 panic。

超时退出也不证明远端查询即时结束、事务已回滚或副作用不存在。PG14 默认不在查询执行中轮询客户端连接，[连接检测](https://www.postgresql.org/docs/14/runtime-config-connection.html#GUC-CLIENT-CONNECTION-CHECK-INTERVAL)与[statement_timeout](https://www.postgresql.org/docs/14/runtime-config-client.html#GUC-STATEMENT-TIMEOUT)约束不同；进程直接退出会终止 driver 的异步取消机会。专项保持 blocker 不释放，超时场景按已配置 DB 查询预算加固定观察余量核验精确 backend 消失；正常排空与启动取消仍按原严格连接释放断言。应用退出预算与服务端查询期限须分别配置和监控，不自动增加连接轮询参数或提供强制回滚承诺。

真实命令测试与原协议专项共用隔离 PG/Redis，构建并启动 API 子进程；结果见[交付记录](../delivery-log.md)。默认 Compose/Nginx 和公开 OpenAPI 未启用此模式，真实存储 TLS、生产参数/证书与代理、浏览器、账号权限/恢复及完整 M0 仍待验，未部署。无依赖、SQL、迁移变化；回退停服务后移除显式配置环境，不自动撤销会话或清理存储。

### 7.18 已登录用户修改密码

显式的 `POST /api/v1/account/password` 接受且仅接受非空字符串 `current_password`、`new_password`，最多 4096 请求体字节；拒绝未知/重复解码字段名、null、坏 Unicode 和尾随内容。仅从真实普通会话选择主体，不接受 ID、版本或恢复代次输入；复用原 direct/pinned proxy TLS、精确 Host、正向同源证据、严格 Cookie 和总 HTTP 期限。接口及错误/Cookie 合同见[独立账号 OpenAPI](../../api/account.openapi.yaml)。未配置改密能力时为 404，默认 Compose 未启用。

`identity.PasswordChanger` 使用首次真实 session Read 的 `SessionVersion`：固定 SubjectID/AuthGeneration/RecoveryID，初始恢复读不得换成新的 epoch，验密 snapshot 必须仍 enabled 且匹配原认证代次。共享 BoundedHasher 在数据库锁外校验当前密码并创建新 Current 记录；Legacy 不先 RehashCAS，失败改密不附带旧参数升级。最终 `ReplacePasswordForSession` 在同一事务按恢复 `FOR SHARE` → 带 enabled/两代次/溢出条件的 UPDATE → commit 执行；不先持有账号共享锁再升级写锁。只调用一次最终替换，不根据失败回执重新读取版本或补偿。成功同时推进 credential_version/auth_generation；旧会话和未完成登录候选被原权威门禁拒绝，无须依赖 Redis 删除或跨存储事务。新登录使用新代次的独立名额集合。

会话有效性的时点为请求首次成功 Read。之后 logout 或自然过期不会撤回已受理的改密；当前密码及最终账号/恢复 CAS 仍必需。账号与恢复代次始终以首次 session fence 为准；验密 snapshot 读取后、最终条件 UPDATE 前发生的凭据 revision 变化会冲突，不采用最新 revision 自动重试。仅改变 CredentialVersion 的重哈希若在 snapshot 读取前已完成，本次可读取新 CV 后合法继续；提交后的并发变化也不能撤回已完成的改密。任一修订为 int64 上限则拒绝；失败返回零结果，提交或回执错误保持可能已写入的分类，不能盲目重试。

Store 保留原共享 work；`redisstore.NewPasswordChange` 仅从同一 Login/store 的 authority/work 组装 Read、独立限流、规则与 native Changer。HTTP 构造器的可选 capability 须绑定同一个 `*Login`，原构造器不隐式新增能力。Runtime 追踪整个同步调用及 KDF，沿用第 7.15/7.17 节的实际排空和共享总退出预算。

新密码强度由独立不可变 `password.Acceptance` 处理，不改变既有密码的原始字节校验。用户于 2026-11-01 明确接受首期规则：至少 **15 个 Unicode code point**、允许空格、不强制字符组合，并拒绝常见弱密码；按原始字节处理，不 trim/归一化/截断。显式最低长度可提高至 64；字节上限沿同一个 Hasher，至少容纳最低字符数的四倍字节。规则参考 [NIST 单因素密码建议](https://pages.nist.gov/800-63-4/sp800-63b.html)，用户确认不等于 NIST 整体合规或生产字典已通过验收。字节完全相同的新旧密码在当前密码验证成功后拒绝；Unicode NFC 没有引入，既有记录和输入语义保持。

`password_change` 可选配置全部显式：`minimum_characters`、绝对规范 `blocklist_file`、三个 `{count,window}` 对象 `global`/`source`/`subject`。规则文件仅有词法整数 `version: 1` 和非空 `sha256` 数组；每项为完整原始 UTF-8 密码的 64 字符小写十六进制 SHA-256，拒绝零摘要、重复、未知/null/坏 Unicode/尾随内容。文件 ≤1 MiB，纯机制最多 16,384 摘要，文件字节预算可能先达到；不保存用户密码，摘要表是已知弱密码的精确整串比较，不是密码存储/KDF、子串匹配或在线泄漏查询。读取使用第 7.17 节 public 文件检查；不热重载、不自动下载字典。正式字典的来源、许可、覆盖充分性与更新/运维仍须生产验收，测试仅用合成词条。

独立 `PasswordChangeLimits` 在认证会话得到可信主体后、任何 KDF 前原子扣取全局/实际来源/主体三维固定窗口额度。复用原有有界 Lua，但采用独立 HMAC purpose、`oj-password-change-limit:` 前缀和持久 marker；不共用邮箱桶或修改旧 login marker。marker 同时绑定完整 Acceptance 指纹（版本域、最低长度、字节预算、排序后的固定长度摘要集合），顺序变化无影响；指纹仅进入 metadata，不进入计数 key，防止改规则获得新额度。拒绝不写任何维度或 TTL；部分/未知效果不退款、不重试。

显式初始化只适用于隔离下已知初始 namespace；Runtime 只读核验。规则/词典、额度或 key 不匹配/丢失时启动或请求失败关闭，不自动重建预算。变更规则须隔离旧副本并受控维护 marker、保留应有计数；本阶段不提供热更新或通用迁移平台，也不以换 namespace 绕过全局额度。

成功提交后仅返回 204、空体，删除 `__Host-oj_session` 和 `__Host-oj_login` 两个原 scope Cookie，不发放新会话。失败不改 Cookie；429 不披露维度/余量/Retry-After，400 区分新密码规则、当前/新输入长度及未改变，401 拒绝凭据，409 要求重新登录后操作。存储/恢复/取消/未知写入优先 503，结果可能已提交，应重新登录核验；Cookie 清除不能代替持久化改密，进程/网络退出也不保证远端即时回滚。

原 schema 5、密码格式、会话协议和旧配置均兼容，无依赖/迁移变化；SQL 仅新增专用条件 UPDATE。回退先停用并移除可选配置，重启/回退旧二进制，成功密码变更不撤回。真实双客户端竞态/锁序/取消/丢回执、主键 EXPLAIN，以及双 API 进程/代理的拒绝、撤销、Cookie 和重新登录证据见[交付记录](../delivery-log.md)。[浏览器账号](../account-browser.md)已有本机分项验证；其余浏览器、生产字典/预算/证书与存储 TLS、邮箱找回/注册、完整权限恢复及 AC01/23/24/25/M0 继续待验，未部署。

## 8. 注册、找回与账号发放

用户确认采用“日常邮箱验证注册＋密码登录＋邮箱自助找回，正式比赛采用校内发号和人工核验重置”。这确认产品流程，不代表邮件供应商、发信组件或运行效果已验证。

| 场景 | 已确认路线与设计边界 |
| --- | --- |
| 日常注册 | 验证所填邮箱后激活账号；未验证状态仅允许完成验证等必要操作，不开放训练提交或管理权限。是否开放注册可配置，具体字段、邮箱变更和重复注册规则待设计 |
| 日常登录 | 个人账号仅以邮箱和密码校验，昵称用于显示且允许重复，不作唯一登录身份；沿用 Session，不要求每次收邮件或依赖供应商在线 |
| 日常找回 | 发信至已验证邮箱，凭有效一次性令牌设置新密码，成功后撤销旧会话；仅提出找回请求不会改密、锁定账号或使既有会话失效 |
| 正式比赛 | 校内受控发号，授权工作人员依据参赛名单及现场身份核验后重置；不以外部邮箱/短信为前置条件，沿用第 4 节比赛身份和期限 |
| 断外网或邮件故障 | 注册激活与邮箱找回可暂停并明确提示暂不可用；不能静默跳过验证。既有账号密码登录、提交、判题、榜单和赛务须继续运行 |

2026-10-31 用户确认日常个人账号仅邮箱登录、昵称可重复，整址忽略大小写、保留原始显示且不合并点号/加号别名；比赛专用账号的校内发号方案不变。内部解析/映射支持范围见第 7.7 节，公开重复注册/重发、邮箱变更及其余注册字段合同仍待设计，不将昵称作为账号唯一键。第 7.11 节已有独立两步登录 Handler，默认公开 API 尚未挂载；正式注册、邮件激活及资源权限仍未实现。

相比纯用户名注册与人工找回，邮箱能减少日常恢复的人工负担，但新增域名、投递、滥发控制及外部可用性依赖；校园统一认证和短信未选定。正式比赛保留人工核验成本，以满足校内独立运行。

待实现约束：验证/重置使用密码学安全随机令牌，绑定账号、用途和期限；验证记录保存摘要，令牌不入日志。激活与重置令牌不能互用；一次性消费及账号状态更新须原子化，重置同时推进认证代次，防止并发重用或迟到登录恢复旧权限。邮件链接只打开确认页面，GET 不执行改密或消费令牌；链接基址由可信配置生成。失效账号不得被找回流程重新激活。令牌期限与重发规则按第 12 节已确认配置；未验证账号保留期仍待细化。

发送与校验按账号、来源和全局预算限流，考虑校园共享出口，不能只按 IP 限制；找回响应及响应耗时避免泄露账号存在性。邮件不包含原密码或新密码。人工重置须核验主体、限定比赛范围、操作人再次认证并审计原因，不仅凭用户名放行；重置后的凭据交付方式及核验材料待操作规程明确。参考：[OWASP 找回密码](https://cheatsheetseries.owasp.org/cheatsheets/Forgot_Password_Cheat_Sheet.html)。

邮件渲染使用部署方配置的规范 HTTPS 原点，拒绝路径、查询参数、片段和用户信息；链接目的固定为 `/account/activate` 或 `/account/reset-password`。凭据放在 `#token=` 片段中，打开链接时不进入 HTTP 请求目标。待接入的确认页须读取并移除片段，不持久化令牌，设置 `Referrer-Policy: no-referrer`，并仅在用户提交时以 POST 请求体发送令牌。邮件正文和令牌不得记录日志；真实邮件客户端、链接重写行为及确认页另验。

AC25 覆盖上述流程、跨 API 并发单次消费、邮件失败与真正断外网运行。内部签发、激活和找回消费已有真实 PostgreSQL 分项验证；公开流程与投递验收仍待完成。

内部 Redis 邮件准入分两组：申请按全局、可信来源及规范邮箱计数，须在账号查找前调用；确认按全局、可信来源及用途绑定的令牌摘要计数，须在密码计算前调用。注册和找回共享同组的全局与来源配额，申请与确认组相互独立。使用登录限流密钥的独立 HMAC 域，Redis 键不含明文邮箱、地址或令牌。各副本须共享固定窗口配置和显式预置的持久标记；标记缺失或不匹配即拒绝，不在启动或请求时重建。配额拒绝不写计数或延长期限。可选配置 `email_limits` 要求明确填写 `request_global`、`request_source`、`request_mailbox`、`confirm_global`、`confirm_source`、`confirm_token` 的 `count` 和 `window`。隔离恢复的开放命令在空 Redis 上准备并核验此标记；配置本身不启用公开路由。申请端仍须统一处理账号是否存在和邮箱维度拒绝。

启用 `email_confirmation` 时必须同时显式配置 `email_limits`、确认密码的 `minimum_characters` 和 `blocklist_file`。运行时读取密码规则并核验既有 Redis 标记，配置不完整或标记不一致时拒绝启动。确认服务先解析用途绑定的令牌，再扣共享额度，随后按现有密码政策和与登录共用的计算预算生成凭据。密码最少 15 个 Unicode 码点，拒绝名单中的完整密码，不修剪或归一化；数据库消费冲突统一返回链接不可用，写结果未知不自动重试。

## 9. 邮件接口与组件验证路线

用户确认采用可替换 SMTP 接口＋优先验证 `wneessen/go-mail`，供应商等域名和学校现有服务明确后再定。接口方向已确认，库仅确定验证顺序，不代表正式准入、版本锁定或安装；域名/DNS 管理权限及可用 SMTP 均暂不确定，仅无需新增付费且允许应用使用的服务可进入候选；免费试用不能作为长期零付费依据。

| 方案 | 适用条件与代价 |
| --- | --- |
| 学校或协会已有 SMTP | 若允许应用自动发信且配额、交接和投递满足要求，可复用现有服务；实际权限、限流和可用性待核验 |
| 托管事务邮件（历史比较） | 阿里云邮件推送、Resend 此前用于说明 SMTP 可替换性，不是当前采购候选；付费套餐排除，免费额度须另核验长期条款、超额不扣费和可用性 |
| 自建公网邮件服务器 | 可控性较高，但需承担发信信誉、退信、DNS、维护和监控；学校资源有限，建议首期不选 |

业务侧仅依赖小型发信接口，SMTP 连接、认证、TLS、供应商配置及 go-mail 类型封装在适配层；模板与账号规则不直接依赖供应商 SDK。更换供应商仍须验证域名、额度、错误语义和实际投递，不承诺只换配置即可无差异运行。若供应商专用功能成为必要条件，再评估其官方 API/SDK。go-mail 提供邮件组装、模板、TLS 和超时支持；Go 标准库 `net/smtp` 较底层且冻结新功能。库正式引入前核验精确版本、Go 兼容性、MIT 及传递许可证、安全公告，尤其已公开地址编码问题的修复范围，不将验证顺序视为已安全准入。

用户已确认优先验证 PostgreSQL 持久化发信任务＋独立受限发送执行器，避免外部 SMTP 延迟占满 API 请求；与判题任务分开调度和预算，暂不增加队列中间件，详见第 10 节。投递存在超时后结果未知和重复发送，不能承诺恰好一次；过期或被替代任务不得无限重试。摘要验证记录与异步发信所需的秘密载荷须分别设计，已选业务侧短期 AES-GCM 载荷、River 仅存 ID 和独立密钥文件；保留期/轮换细节待确定，不能直接持久化明文重置链接。

验收前核验发信域名及 DNS 权限、供应商要求的 SPF/DKIM/DMARC 等记录、强制 TLS 与证书验证、配额/预算、退信和实际收件表现；邮件服务受理不等于进入收件箱。开发测试收件器与真实邮箱联调分别记录结果。供应商仍待确认，River 开源核心优先验证、专用任务表备选，无采购、DNS 修改或实际发送。

内部 SMTP 适配器固定使用 [go-mail v0.8.2](https://github.com/wneessen/go-mail/releases/tag/v0.8.2)，支持隐式 TLS 或强制 STARTTLS，均验证服务端证书且不降级到明文。单次尝试受调用方期限和配置上限共同约束。收到最终 DATA 受理响应后，即使连接随后中断也返回已受理；未收到最终响应时返回结果未知。明确的 SMTP 4xx 与 5xx 分别归临时失败与永久拒绝。队列只接收结果类别，不接收 SMTP 原始错误、凭据或正文。

本地 TLS 收件器验证受理、临时及永久拒绝、断连、证书拒绝、认证拒绝和超时。真实 SMTP、供应商预算、队列重试及恢复仍待验收；服务器受理不等于进入收件箱。

参考（2026-10-25）：[阿里云 SMTP](https://www.alibabacloud.com/help/en/direct-mail/smtp-endpoints)、[发信域名配置](https://help.aliyun.com/zh/direct-mail/user-guide/how-to-configure-sending-domain-names)、[Resend SMTP](https://resend.com/docs/send-with-smtp)、[go-mail](https://github.com/wneessen/go-mail)、[安全公告](https://github.com/wneessen/go-mail/security)、[net/smtp](https://pkg.go.dev/net/smtp)。

## 10. 异步发信任务验证路线

用户确认优先验证 PostgreSQL 持久化发信任务＋独立受限的发送执行器。这确定验证方向，不代表已实现或已通过故障恢复测试；后续已确认 River 开源核心优先验证、专用任务表备选；Redis 会话用途与判题任务验证路线不变。以下保留备选比较。

| 方案 | 收益与代价 |
| --- | --- |
| 进程内 goroutine/内存队列 | 接入少，但进程重启会丢失未完成任务，多 API 无共享进度；不建议作为可靠发信的唯一任务存储 |
| PostgreSQL 持久化发信任务 | 可将账号验证记录与发信意图放入同一事务，减少跨存储一致性问题；需要任务领取、重试、清理和数据库资源预算 |
| Redis＋Asynq | 提供现成重试、调度等能力；增加队列持久化/恢复管理，数据库业务与入队仍需可靠衔接；与 Session 共用实例还需评估资源及故障影响 |

验证设计：发信任务与判题队列分表、分配额；逻辑隔离不要求独占服务器。账号验证记录与发信意图在同一数据库事务提交或回滚，不先发信后补记录。执行器领取任务后释放短事务，再进行 SMTP I/O，不能长时间占用行锁或数据库连接等待外部邮件。多执行器须原子领取并具备超时恢复机制；供应商可能已受理而任务未确认，重复发送仍须容忍。控制总发送速率、重试次数/退避、任务年龄与积压，过期令牌任务停止发送，网络恢复后不能瞬间放出全部积压。

用户确认优先验证 River 开源核心、保留小型专用任务表备选；正式准入与版本待验证，不复用可执行不可信程序的判题 Worker 来保管 SMTP 凭据。邮件载荷方案已确认，见第 11 节；保留期和轮换细节待设计。参考：[PostgreSQL 队列式读取与 SKIP LOCKED](https://www.postgresql.org/docs/current/sql-select.html)、[Asynq](https://github.com/hibiken/asynq)、[River 事务入队](https://riverqueue.com/docs/transactional-enqueueing)。

AC25 进一步验证：事务回滚不留孤立发信任务；提交成功后进程崩溃仍可恢复任务；两执行器竞争、领取后崩溃、租约/救援超时及迟到完成不得覆盖新状态；SMTP 受理后断连按结果未知处理；永久失败不无限重试，过期/被替代令牌不发送；断网积压有界、恢复速率受限，观察数据库连接、任务表增长、清理及判题延迟。任务幂等不等于 SMTP 恰好一次投递。当前均未运行，预算数值待硬件和供应商条件明确。

### 10.1 River 开源核心验证路线与专用表备选

用户确认优先验证 River 开源核心，保留小型专用任务表为备选。这只确定验证顺序，不表示依赖安装、正式准入或批准 River Pro。两者均可沿用 PostgreSQL，不需新增 Redis 队列或消息代理；不能未经实测声称 River 太重或专用表一定更快。

| 比较项 | River | 小型专用任务表 |
| --- | --- | --- |
| 可靠执行 | 提供事务入队、重试、调度及卡住任务恢复；仍须按本项目故障场景验证 | 字段可很少，但原子领取、租约/尝试代次、崩溃恢复、退避、清理和迟到回报保护须自行维护 |
| 业务适配 | 账号事务通过 pgx 驱动入队，任务处理器调用 SMTP 接口；邮件业务规则由 OJ 保留 | SQL 可由 sqlc 生成，业务字段和状态完全自定，适合范围稳定且很窄的任务 |
| 运维与资源 | 多一套库维护的表、迁移和后台维护任务，需核验连接、空闲开销、轮询/通知和清理负载 | 可按单场景精简，但调优、监控与故障诊断工具需自己补齐 |
| 迁移与升级 | 官方支持导出迁移 SQL，可评估纳入既有 Goose 独立迁移流程；升级需跟踪上游 schema | 统一由 Goose 管理自有表；协议演进及兼容责任全部由项目承担 |
| 许可与功能范围 | 仓库标注 MPL-2.0，River Pro 付费能力已排除；开源正式版本及传递依赖另审 | 少一个任务库依赖，但自研并发协议的审查和长期维护成本更高 |

倾向 River 的理由是本项目已经要求多 API、受限执行、崩溃恢复和重试，可靠性工作并不会因邮件类型只有两种而消失。先用开源核心验证有限范围，避免为少数邮件重新维护一套通用队列；若准入实测表明维护负担或资源成本不合适，再采用严格限于发信的专用表，不扩成通用调度平台。

配合既有路线的验证边界：

- **sqlc＋pgx**：已固定 River v0.51.2、pgx v5.11.0，沿用 Go 1.26.11 和 PostgreSQL 14。入队适配器与令牌摘要、密文共用签发事务；River 参数只存载荷 ID，使用独立邮件队列和最多六次尝试。sqlc 只管理自有业务 SQL。
- **迁移**：River 八步 SQL 一起放入单个事务会触发 PostgreSQL 枚举值限制（`55P04`）。使用 [v0.51.2 官方迁移器](https://github.com/riverqueue/river/blob/v0.51.2/rivermigrate/river_migrate.go) 的逐步提交，由显式维护命令执行，沿用 Goose 的独占维护锁。Goose 不管理 River 表，应用启动不迁移。只接受固定的连续迁移版本；中途中断可从已提交版本继续，入队在版本不全或未知时拒绝。
- **发送额度**：开源核心的每实例 `MaxWorkers` 只约束单实例并发。schema 22 提供全局发送许可：每个执行器从同一 PostgreSQL 单行取得许可，按数据库时钟和操作员配置的最短许可间隔放行。空闲不会积累额度，缺配置或提交结果未知时不放行；已取得的许可不得转给其他任务或重用。每次放行写一行并产生 WAL，没有额外索引或逐邮件历史。许可限制数据库准入速率；提交和调度延迟可能使 SMTP 启动靠近，发送器仍须限制并发并在开始 SMTP 前立即取用许可。数据库时钟前跳可能提前放行，后跳会暂停放行；恢复时不得把同步提交视为异步副本或旧备份仍有最新状态的保证。供应商配置和目标容量待验证。
- **秘密与业务有效性**：不采用 River Pro；用户已选任务仅存业务 ID、业务侧短期 AES-GCM 加密载荷和独立密钥文件；密钥、清理、访问权与日志/UI 脱敏须完整设计。两种方案均须发送前检查令牌有效性、控制任务保留和 SMTP 结果未知，不将队列库当成完整邮件系统。

运行准入时比较相同硬件与负载下的空闲/高峰 CPU、RSS、连接、数据库写入与判题延迟。真实 PostgreSQL 已验证同事务提交/回滚、迁移锁和从部分版本继续；两执行器竞争、进程终止、断网积压/恢复、过期丢弃及目标环境仍待验收。River v0.51.2 及 pgx 驱动为 MPL-2.0，未复制或改动上游代码。发行镜像前还须核对依赖许可证和分发要求。

查阅日期：2026-10-25。来源：[River 项目与许可](https://github.com/riverqueue/river)、[事务入队](https://riverqueue.com/docs/transactional-enqueueing)、[入门与 pgx 配置](https://riverqueue.com/docs)、[迁移及 SQL 导出](https://riverqueue.com/docs/migrations)、[重试](https://riverqueue.com/docs/job-retries)、[卡住任务恢复](https://riverqueue.com/docs/stuck-jobs)、[Pro 加密任务](https://riverqueue.com/docs/pro/encrypted-jobs)。

## 11. 邮件敏感载荷保存与密钥管理

用户确认采用业务侧短期加密载荷＋River 仅存业务 ID＋Go 标准库 AES-GCM，密钥通过独立文件注入，并明确不考虑付费方案。一次性令牌验证记录只保存摘要，但后台重试发信需要原始令牌，仅有摘要无法还原链接。用户密码始终只作不可逆哈希，本节只讨论短期发信令牌的可恢复密文。

`internal/identity/emailtoken` 提供此流程的保护组件：生成 256 位随机令牌，接受规范 base64url 形式，并按注册激活或密码找回分离 SHA-256 摘要。AES-256-GCM 以随机 nonce 封存发信令牌，认证载荷 ID、主体、恢复代次、原收件地址、用途、签发时间及两个截止时间。时间按 PostgreSQL 微秒精度固定；超过发送截止不得解密，已经发出的链接仍按令牌有效期和数据库状态判断。仅当前密钥可加密，轮换期间可显式保留旧密钥解密；加密使用总量须由集成层限制。保护组件本身不签发、消费或发送实际链接。

| 方案 | 取舍 |
| --- | --- |
| 业务侧短期加密载荷＋River 参数仅业务 ID | 用户已选；沿用开源核心、独立清理敏感数据，代价是管理密钥、轮换及权限 |
| River Pro 加密任务（已排除） | 因不考虑付费方案，不再作为候选；仅保留历史比较 |
| 只存内存或明文任务参数 | 内存无法满足重启恢复；明文不符合既有敏感信息边界，不建议采用 |

保护组件使用 Go 标准库 `NewGCMWithRandomNonce`；同一密钥所有副本累计加密次数须受接口上限约束并提前轮换，不能按副本各算一份额度。密钥由密码学安全随机源生成，不用口令或示例值；随机失败明确失败。解密认证失败不输出明文、不降级。通过 AAD 绑定载荷 ID、用途、版本和目标等稳定上下文，防止记录替换；数据库记录密钥 ID，不保存密钥。River 参数只保存业务 ID，验证记录仍只保存令牌摘要；同事务保存验证记录、受保护载荷与任务。

密钥通过独立文件在部署时注入，只读挂载并限制所有者/访问权限，不进入仓库、镜像、任务表或日志，不与数据库备份混放。有权限的 API 负责加密、发送执行器负责解密，判题 Worker 不持有密钥；对称加密不能以职责划分宣称 API 无法解密。此方案不能抵御持钥主机完全失陷。不引入付费 KMS/Vault 服务。

`emailtoken.LoadProtector` 在 Linux/Darwin 上读取独立密钥文件：绝对规范路径，受信任的本地目录，不经过符号链接；文件须为单硬链接的普通文件、由 root 或进程有效 UID 所有，权限为 `0400` 或 `0600`，大小不超过 4096 字节。文件是单个 JSON 对象：`format` 固定为 `oj.email-payload-keys.v1`，`current` 指定用于加密的 key ID，`keys` 以 key ID 为成员名、规范无填充 base64url 的 32 字节密钥为值。key ID 仅用 1–16 个小写 ASCII 字母、数字、下划线或连字符；最多四把，必须非零且互不相同。部署须生成独立随机密钥，不能复用登录封装或限流密钥。

读取返回固定的密钥快照；重新加载文件需要调用方显式构造新的保护器。缺失或无效只返回通用错误，后续运行时接线须暂停依赖密钥的发信流程并告警，既有登录和判题应继续可用。应用路径配置、跨副本轮换分发、真实邮件投递和独立备份恢复仍待验收。

载荷在发送终态、令牌消费/撤销或到期后尽快清理；SMTP 结果未知时仅在有效期内有界重试，不延长令牌期限。旧备份恢复不得复活已消费/撤销令牌，需设计恢复代次或统一失效；在线删除不会自动删除备份旧密文，须联合设计备份保留与密钥退役。轮换时限期保留解密仍需的旧密钥，密钥丢失则待发任务明确失败，不回退明文；重新申请生成新令牌和任务。

AC25 细化覆盖密文往返、篡改/错误密钥/错误上下文拒绝、多副本密钥版本、随机源异常边界、文件缺失/权限/格式错误、轮换重试、日志/UI 脱敏、清理与备份恢复。参考：[Go 认证加密](https://pkg.go.dev/crypto/cipher#NewGCMWithRandomNonce)。当前组件覆盖密文往返、绑定、到期、私有文件准入，以及从文件加载轮换密钥的重叠及退役；跨进程分发和其余运行行为须在相应集成层验证。

## 12. 已确认的令牌有效期、重发与发信重试

2026-10-25 用户明确接受推荐方案：旧链接暂时保留，其余参数采用默认值。下表为已确认的首期默认配置，保留配置入口；属于产品规则确认，不是 OWASP 指定数值或运行验证结果。

| 项目 | 首期默认值与语义 |
| --- | --- |
| 注册验证链接 | 从令牌生成起 24 小时有效 |
| 密码找回链接 | 从令牌生成起 30 分钟有效 |
| 用户主动重发 | 同账号、同用途至少间隔 60 秒；生成新令牌与新发送任务，不延长旧令牌期限 |
| 有效链接数量 | 同账号、同用途最多 3 个；达到上限暂缓新申请，不挤掉或撤销旧链接，待到期或成功消费后释放额度 |
| 临时失败自动重试 | 最多重试 5 次，含首次共最多 6 次；退避并加入随机抖动 |
| 每个任务发送窗口 | 从该任务/新令牌生成起最多 10 分钟，且不超过令牌剩余有效期；窗口结束停止自动发送，不延长令牌期限 |
| 永久失败 | 停止自动重试并记录脱敏原因；SMTP 已受理后不因用户未打开邮件自动重发 |

### 12.1 旧链接保留与并发消费

用户重发后，旧链接在其原有效期内继续有效；任意一个链接成功完成同账号、同用途操作时，原子更新账号状态并使该用途其他链接全部失效。打开链接不算消费；只有服务端确认完成邮箱激活或密码重置才算成功。密码重置同时撤销旧会话，沿用账号认证代次规则。

例如先生成找回链接 A，再重发生成 B：A 仍可在原期限内完成重置，成功后 A、B 均不可再次使用。不同用途的令牌不能互用。并发消费不同链接只能有一次成功；跨 API 的新令牌创建、3 个名额检查与操作完成须按同一账号/用途协调，不能留下绕过批量失效的旧代次令牌。已失效任务即使迟到发送，其链接也不能生效，发送前仍须检查当前有效性。

取舍：旧链接保留可避免邮件延迟与乱序导致用户反复收到失效链接；代价是短期内有多个有效凭证，故限制为 3 个，并在成功消费时整体失效。60 秒间隔和 3 链接上限不替代账号、来源与全局防滥发额度；这些额外额度数值及限流实现仍待供应商和资源条件确定。公开找回入口不能通过上限错误或响应耗时泄露账号是否存在。

### 12.2 自动重试、载荷清理与验收

自动重试复用原任务与原令牌，用户主动重发才生成新令牌；两类计数分别管理，实际 SMTP 尝试仍受全局发送预算约束。临时失败、永久失败和结果未知分别记录；SMTP 受理后断连可能造成重复投递，结果未知的有界重试同样受次数和窗口限制，不承诺恰好一次。

内部发信读取器每次尝试以业务 ID 查询一次 PostgreSQL：按现有主键关联密文、令牌、账号、邮箱和恢复门禁，在同一快照复核账号启用及认证代次、邮箱验证状态、恢复代次、签发时间及两个截止时间。随后验证密文绑定及账本摘要，再将收件地址和令牌交给可信邮件渲染器。任务不存在、已撤销或超时会被拒绝；查询失败或密文校验失败不会返回发信内容。无需迁移或新增索引。

读完即释放数据库连接。`internal/maildelivery` 为已领取的任务复核载荷、生成链接并执行至多一次 SMTP 调用。它使用数据库快照计算剩余发送窗口，以本地读开始时的单调时钟为起点，扣掉读取和渲染时间，避免直接比较数据库与执行器的主机时钟。调用方只收到脱敏结果类别。数据库快照之后的并发变化可能使已经准备的链接失效；发送与账号事务没有原子性，链接使用时仍需验证当前账号和恢复代次。

准备好邮件后，单次发送在调用 SMTP 前要求共享许可，并用同一个发送窗口约束许可读取和 SMTP。许可拒绝、结果未知或在取许可期间取消都返回延后，不调用 SMTP；执行器应延后任务且不消耗 SMTP 尝试次数。若等待许可期间发送窗口到期，返回载荷失效。已失效载荷不会开始请求许可。生产装配须传入 PostgreSQL 许可接口 `mailqueue.SendRate.TryTake`。

发送终态清理该任务的加密载荷不撤销已发送链接；验证记录的摘要、到期时间和消费状态保留至所需生命周期结束。`mailqueue.CompleteTx` 按 River 任务主键锁定当前行，复核队列、类型、运行状态、领取次数、领取时间、执行器 ID 和载荷 ID，再调用 River 的事务完成接口和 `RetireEmailPayload`。完成和删除一起提交或回滚；已撤销的领取不能删除新领取所需的密文。使用现有任务和密文主键，不加索引；每次完成更新一行队列状态并最多删除一行密文。调用方使用与默认 River schema 配置一致的连接和事务，任何错误回滚，不在事务内发送 SMTP。

有效链接数按令牌状态计算，不按待发任务数计算；用户主动重发创建新载荷。River v0.51.2 的自动结果回报不核对领取身份，账号邮件队列因此采用 `mailqueue.Dispatcher`：复用固定 River 驱动原子领取，同一队列不能同时启动 River Client 的默认 worker。发送前复核领取，发送期限从领取前开始计算；写回用相同的领取身份锁行校验。发送回调必须尊重取消。外部 SMTP 已开始后仍可能丢确认、重复投递，数据库不能撤回外部受理。

未获许可的延期不扣发送次数；临时失败和结果未知退避重试，至多六次。两分钟未写回的领取可由另一执行器单行救援；因无法确定是否已调用 SMTP，旧领取占用一次。终态和密文删除同事务，任务仅记录脱敏结果类别。单次领取用 River 现有索引，写回用任务主键，救援按运行状态与队列查找；未新增索引或迁移。独立 `mail-worker` 串行处理，一次至多救援并领取各一个任务；接入真实共享许可与 SMTP 适配器。实际进程被强制终止后的本地救援已验证，测试只调整旧领取时间戳以缩短等待。定时清理、供应商及现场规模仍待验。

### 12.3 签发持久化与迁移

内部 `RegisterPersonal` 不接受申请人提供的密码。它生成随机主体和不返回的随机占位凭据，在持有当前恢复代次共享锁的事务中一并写入未验证邮箱、第一枚激活令牌摘要、加密载荷和 River 任务。重复规范地址不改变已有账号，也不消耗加密额度；入队失败则全部回滚，已预留的加密额度不退回。公开入口须先限流并统一响应，按另行定义的流程处理重复申请和重发。无新表或索引。

后台 `ProcessEmailRequest` 使用入站时绑定的恢复代次，不把旧请求重新标记为新代次。首次注册调用原子注册；未验证的启用账号可以申请重发；只有已验证的启用账号才可申请找回。确定不符合条件时不发邮件，不改变密码或会话。后台存储错误可能发生在提交之后，不能自动重提；本用例不生成公开响应，也不等待 SMTP。

schema 21 新增签发摘要账本和短期密文表。内部 `IssueEmailToken` 只支持已存在且启用的个人账号：未验证邮箱只可签发激活链接，已验证邮箱只可签发找回链接。调用方须提供观察到的账号认证代次和恢复代次。事务依次锁恢复代次、账号和邮箱，取数据库当前时间，再检查同账号、同用途的 60 秒重发间隔及同用途、当前代次至多 3 个未过期链接。签发不撤销旧链接。密文另外绑定账号认证代次；只有密文和摘要进入各自的表。入队适配器仅接收载荷 ID 与同一事务，失败回滚三者；提交结果未知不得自动重试。

时间范围索引各一条，分别服务近期签发检查与最多读取 3 条有效链接。每次签发维护令牌表的主键、摘要唯一索引和两条范围索引，并写密文表主键及相应 WAL。签发在加密前要求独立适配器为当前密钥预留一次用量；失败后不退还。适配器不能借用此数据库连接池或获取账号锁。历史按摘要查找与清理须在消费和执行器阶段接入；本次无公开签发、真实 River、邮件发送、密钥文件或生产用量适配器。生产接入前须验证跨副本上限、独立预算持久化及恢复时轮换或独立预算保留。

维护窗口停旧 API、Worker 及维护写入后执行独立迁移，启动匹配 schema 21 的二进制。Down21 先停新版，只允许两个新表都为空；有签发历史应向前修复。该迁移不改已有提交和队列。

schema 22 建立空的 `account_mail_send_rate` 表。维护角色须在启用发送器之前运行 `migrate mail-send-rate <spacing-ms>`（100 至 60,000 毫秒，按供应商允许的发送速率选取）。命令核验应用 schema，按单行主键配置，和许可使用同一行锁；重复配置不写新版本，缩短间隔保留现有截止，延长按差值推迟现有截止。运行角色只需表读取和行锁所需的 `UPDATE (next_permit_at)`，不应有插入、删除或改间隔权限。所有执行器使用同一数据库及 schema，调度端的限速拒绝不能消耗 SMTP 尝试次数。回退先停并排空发送器；Down22 仅删除许可状态，保留任务、令牌和训练数据；若随后 Down21 被非空签发历史阻止，数据库停在 21。再次迁移到 22 后须重建配置，恢复时先隔离旧发送器。

### 12.4 邮箱激活与找回消费

内部 `ActivateEmail` 用激活用途的摘要查找锁键，再按恢复代次、账号、邮箱顺序锁定。锁后用数据库时间复核原令牌、签发与截止时间、当前账号代次及尚未验证状态。邮箱所有者须在确认时提供新密码；上层先执行现行密码规则和有界密码计算。数据库在同一事务替换凭据、推进凭据和认证代次、标记邮箱已验证，并清除当前代次未到期的激活密文。旧会话和同代次所有激活令牌随即失效；摘要历史保留，过期及旧代次密文仍需后台清理。提交结果未知不得自动重发。

内部 `ResetPasswordByEmail` 复用锁顺序及锁后复核，只接受密码找回用途，且当前账号必须启用、邮箱必须已验证。申请找回不会修改密码或会话；真正完成找回才在同一事务更换密码、推进两个账号代次并清除同用途待发密文。已经发出并清理密文的链接仍可按摘要消费；先成功的链接使同代次其他找回链接及旧会话失效。

两种消费使用现有摘要唯一索引、账号主键与签发范围索引，无新迁移或索引，也不签发会话、不授予训练权限。以后允许改邮箱时，必须在同一账号锁下推进认证代次或另加地址版本绑定。

可选 HTTP 适配层定义 [邮箱确认 API](../../api/account.openapi.yaml)：激活和找回各有一个同源 HTTPS POST，链接打开页面的 GET 不消费令牌。令牌只从至多 4096 字节的 JSON 请求体读取，拒绝查询参数及重复或未知字段。来源地址取直连客户端或受信 mTLS 代理；确认成功返回 204，不设置 Cookie。提交结果不明时返回 503，客户端应先尝试登录，不自动重发。未配置确认服务时接口保持关闭；申请、重发和实际投递仍需端到端联调。

AC25 增补：完整流程还须用可控时钟验证 24 小时/30 分钟、60 秒、10 分钟边界和最多 6 次尝试；覆盖 A/B 邮件乱序、先旧后新或先新后旧消费、跨 API 并发消费与重发、并发第 4 个链接、达到上限不挤掉旧链接、到期释放名额、清理已投递载荷后链接仍有效，以及已失效任务停止发送、迟到邮件不能恢复旧权限。分项验证见[交付记录](../delivery-log.md)。

依据：[OWASP 找回密码](https://cheatsheetseries.owasp.org/cheatsheets/Forgot_Password_Cheat_Sheet.html)支持随机、限时、一次性令牌及限流原则；具体数值和旧链接并存策略由本项目确认。后续前端配套及请求/缓存/刷新约定已确认，见 [前端设计](frontend-sandbox-options.md#31-已确认的路由请求缓存与刷新约定)；IndexedDB＋idb 及草稿保存/恢复/清理规则已确认，见前端设计第 3.2 节；题面渲染组件及约定后续已确认，见前端设计第 3.3 节；首期题型与判定原则后续已确认，见 [判题设计](judging-design.md)；题包模型与 2025-09 子集导入验证路线已确认，见 [题包设计](problem-package-design.md)；SPJ 协议与复用路线后续已确认，见判题设计第 4.1 节；浮点比较规则已确认，见判题设计第 3 节，数值实现待验证；测试顺序/提交内串行/首次失败停止已确认，见判题设计第 5.1 节，编排实现待验证；分类、资源、反馈及自测已确认，实施按总体架构和专题文档验证；供应商、密码参数、密钥轮换及其他已列缺口继续保留。
