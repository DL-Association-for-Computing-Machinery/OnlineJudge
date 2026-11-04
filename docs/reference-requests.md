# 持久化参考验证申请

本页描述内部Go应用用例与PostgreSQL申请账本，不是已开放的浏览器功能。`internal/problemreference`将本人私有草稿的参考验证意图持久保存，提供申请、原键只读回执与取消；`internal/problemvalidation`继续负责实际执行验证。当前没有申请消费者、Worker领取/续租/报告接口、HTTP路由或管理按钮，不会因申请成功改变草稿`not-run`、生产配置或发布阻断。

## 身份与可信配置

`NewCatalog`只接收维护装配层已通过`problem.LoadBundle`及显式受信来源锁验证的Prepared，保存脱离底层文件的ImportIdentity及由`problemvalidation.ResolveSource`产生的完整Profile。最多十来源且共用同一来源锁；允许空目录，以便退役全部来源后仍读取/取消既有申请。配置目录不持有Prepared或大材料文件，创建后修改输入map、关闭Prepared或移动材料目录不改变已保存的配置。

每份配置固定`source_id`、题目内容version、覆盖全部引用的import_revision、source_lock_sha256、`oj.reference-request.v1`及最多16KiB紧凑Profile原始JSON和SHA-256。Profile从封闭Resolver生成，调用者不能为某个申请指定预算。配置副本深复制字节，读取历史回执验证原字节摘要，不用当前Resolver重写旧Profile。`Specification.Valid`仅验证历史身份结构和字节一致性，不是Profile语义、材料或执行准入检查。允许受信合成锁验证持久化协议，不代表它可以通过实际Runner的固定来源锁门禁；现有Profile仅在固定Desktop维护环境验过，不能套用于未知服务器。

`Service.Enqueue(ctx, credential, requestKey, draftID)`只接受普通会话凭据、owner范围请求键及草稿ID；Receipt/Cancel只接凭据和原键。每次通过实际SessionReader取得当前ProblemActor，再由存储在同一READ COMMITTED事务重新核验恢复、账号和当前作者角色。凭据不入库；客户端不能提交主体、权限修订、来源锁、Plan/Profile或执行报告。单token退出在最后会话观察之后仍遵循已接受在途边界，不承诺Redis与PG原子撤回。

入库保存原`ProblemAuthorSnapshot`中的AuthGeneration、RecoveryID、RoleRevision及owner。以后受权准备、领取或接纳报告必须重新检查并比较完整快照；当前enabled不能使撤权后重授的旧任务复活。当前作者可读回/取消本人旧申请，回执和取消不把旧快照改成新权限。

## 事务、幂等与取消

锁序为恢复→账号→角色→本人草稿→policy→申请。problem公开窄`ReferenceAccess`适配器在调用方同一事务检查权限并锁定草稿身份；申请模块不引用problem或identity私有SQL。调用使用五秒context预算，另有独立一秒尽力回滚；不保证远端事务已结束。材料I/O及程序执行不在事务内。

申请先以owner/key无行锁查询原回执：相同draft_id返回原记录及原配置，不同draft_id为冲突。新申请才锁定本人草稿，再锁policy；取得policy后必须重新查询原键，最后才匹配当前目录与检查额度。第二次查询处理“并发赢家刚占满最后额度”的情况，不能将同键恢复误报为额度不足。原键恢复不依赖当前目录、额度或policy仍可用，也不会创建新记录。其他owner的键和不存在的键同为not-found。

当前只有`queued`和`cancelled`两种状态。取消先按owner/key只读定位，再按同样锁序锁草稿、policy和申请行，重读状态后原子更新queued→cancelled；重复取消返回同一状态和首次cancelled_at。它保留原ID、配置、授权快照及历史，不停止Worker，也不会重新排队。

新申请或取消在COMMIT回执失去且结果不确定时返回零结果及`ErrCommitUnknown`；保留原键，通过独立只读Receipt恢复。上下文在存储已成功后取消也按写回执未知处理。not-found仅是当次观察，不能证明另一个请求不在途；不使用新键绕过未知结果。只读回执失败不触发重写或清理。

## 有界排队与未完成边界

schema9默认创建空policy，缺失时拒绝新申请。只有显式受信维护调用`problemreference/postgres.Configure(ctx,pool,Limits)`才能设置额度，启动不会自动写入。`1≤Owner≤Global≤1024`，具体值是维护选择，不是硬件容量结论。下调额度不删除已接受记录；旧原键仍可恢复，新的不同键在超额时拒绝。

新增和取消都串行取得singleton policy行锁。计数沿用积压准入的MATERIALIZED窗口，最多读取Global条queued：达到窗口立即拒绝，未满时全局与本人计数均精确。部分B-tree `problem_reference_requests_queued_owner(owner_subject_id,id) WHERE state='queued'`排除终态历史，支持有界计数；每次入队和取消产生索引/WAL写入及存储成本，实际代价依分布而定。owner/key唯一索引支持回执，主键支持固定ID，配套真实查询计划归交付记录。

额度只限制queued数量，不限制历史增长、请求频率或正在执行的沙箱数量。永久撤权、账号禁用会留下仍占额度的queued；本期没有隐式清理或绕过权限的维护取消。后续受审计失效处理、跨选手判题与参考任务共享的单Worker/单槽容量仍须完成，不能把两套独立队列限额当作共享槽。

申请阶段不读大材料，不填Plan摘要、attempt、判定或Stop证明。后续获得受权容量后，在PG事务外完整冻结实际材料，重验全部申请身份与授权后记录Plan；控制端还需独立拥有expected Plan才能接纳可信Worker的完整报告。当前Prepared依赖导入bundle的os.Root，不是历史materialstore的通用读取器，消费阶段必须显式接通持久化材料。十题254点的既有本机维护验收不能替代此链路、质量检查、非作者终审或发布。

## 迁移与回退

二进制精确要求schema9。维护窗口排空并停止旧API/Worker及维护写入，独立`migrate up`后启动匹配版本；已运行旧进程不会因版本变更自动停止，不滚动混跑。此交付只在隔离临时数据库验证，没有改动持久化localdev或目标服务器。

迁移9新增policy和requests，不修改既有草稿、角色、提交或材料引用。Down9按凭据表→申请表→草稿表→policy锁表，遵循实际首次表访问及删除外键触发器时所需父表锁，区别于业务行锁序；仅在**申请历史及policy均为空**时允许移除。已配置、queued或cancelled都拒绝，须前向修复，不能通过清表绕过。空库Down9/重Up9保留schema8及更早的既有资料；继续回退8/7/6分别按[草稿](problem-drafts.md#入库版本与迁移)和[积压](submission-backlog.md)合同执行。

普通Docker backend负责目录/服务单测；四包sqlc及真实临时PostgreSQL验证所有权、并发最后额度、强制policy等待后的幂等、取消竞争、撤权锁、死事务、实际丢COMMIT回包、历史摘要损坏、查询计划及非空回退拒绝。服务与真实PG的组合测试明确使用合成会话观察，不冒充真实Redis、浏览器或Worker消费验收。命令归[README](../README.md#构建与测试docker-desktop)，结果归[交付记录](delivery-log.md)。
