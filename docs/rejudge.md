# 维护者重判

维护命令可将**单条已完成的个人训练提交**按原源码、原语言、材料版本和清单重新排队。首次成功后，个人历史会显示新代次的排队、执行及最终结果；旧判定仍保留在尝试历史。当前支持固定 Desktop／Ubuntu M0 档位、已固定的隔离 A+B 及本机通用材料（含 Testlib SPJ）；Worker 须启用原语言。命令不启用公开提交、题目发布或比赛重判。

操作员须连接已升级的专用维护数据库凭据。数据库管理员在目标数据库为每位操作员配置独立的 PostgreSQL LOGIN，并直接授予 `oj_rejudge_operator`。命令使用数据库认证的 `session_user`，不接收自报操作员；间接成员或 `SET ROLE` 不代替直接授权。不要给运行 API/Worker 的账号授权，不向维护登录授予超级用户、创建角色或转授权权限。保留登录名以便回读，禁止改名或将旧名称重新分配给他人。

角色是受信运维边界。以下权限允许受信维护登录执行命令所需的 SQL；并不限制持有此登录凭据的人只能运行该命令。若需要委派给不可信调用者，应使用独立安全接口，不能发放这组数据库凭据。

```sql
-- 由数据库管理员执行；oj_named_maintainer 是已认证的独立 LOGIN 占位名。
CREATE ROLE oj_rejudge_operator NOLOGIN;
GRANT oj_rejudge_operator TO oj_named_maintainer;
GRANT SELECT ON goose_db_version, submission_materials, judge_tasks,
  submission_backlog_policy, training_queue_clock, submission_rejudges TO oj_rejudge_operator;
GRANT SELECT (id, subject_id, context_id, material_version, manifest_sha256)
  ON submissions TO oj_rejudge_operator;
GRANT SELECT (submission_id, judge_generation, recovery_generation, attempt_number, state, result_payload)
  ON judge_attempts TO oj_rejudge_operator;
GRANT UPDATE (singleton) ON submission_backlog_policy TO oj_rejudge_operator;
GRANT UPDATE (floor) ON training_queue_clock TO oj_rejudge_operator;
GRANT UPDATE (state, verdict, attempt_number, judge_generation, queue_version, virtual_finish, enqueue_order)
  ON judge_tasks TO oj_rejudge_operator;
GRANT INSERT ON submission_rejudges TO oj_rejudge_operator;
GRANT USAGE ON SEQUENCE training_enqueue_order TO oj_rejudge_operator;
```

维护端使用专用 `OJ_REJUDGE_DATABASE_URL`，与服务运行凭据分开设置。执行时把 `OJ_MATERIAL_ROOT` 指向该控制端实际读取的同一绝对目录。材料须在操作和 Worker 执行期间保持不可变；提交之前的核验不能证明后续节点可用。理由只写可审计的操作说明，勿包含凭据、选手源码或私有题目内容。

Ubuntu A+B 入队还须显式设置 `OJ_SANDBOX_EXPERIMENT=1`、`OJ_APLUSB_RUNTIME=ubuntu-arm64`、`OJ_APLUSB_MANIFEST_FILE`、`OJ_APLUSB_MANIFEST_SHA256`，与恢复库中的原版本、摘要、清单字节逐一核对，并重新核验恢复目录中的所有原对象。档位和可信摘要由维护端独立配置。Desktop A+B 原命令仍可用，状态回读不需要清单。

本机通用材料入队使用 `OJ_SANDBOX_EXPERIMENT=1`、`OJ_LOCAL_TEXT_MANIFEST_FILE` 和 `OJ_LOCAL_TEXT_MANIFEST_SHA256`，不混用 A+B 档位变量。若原清单为 Testlib SPJ，再用 `OJ_LOCAL_SPJ_CHECKER_FILE` 指向原检查器制品。命令核验原清单字节、版本、运行契约、制品摘要和材料对象，任何不匹配都拒绝入队。状态回读无需材料或检查器。

```sh
.local/bin/migrate rejudge enqueue "$DATABASE" "$SUBMISSION_ID" "$OLD_GENERATION" \
  "$MATERIAL_VERSION" "$MANIFEST_SHA256" "$REQUEST_KEY" "verify restored result" --confirm-rejudge
.local/bin/migrate rejudge status "$DATABASE" "$REQUEST_KEY"
```

数据库名称和原代次防止误操作。核对后按积压策略、版本时钟、任务的顺序加锁，以当前虚拟时间入队，与普通提交共享容量。入队和审计在同一事务：同登录、同键、同参数重试返回原回执，参数变化拒绝；不同请求也不能对同一旧代次重复生效。旧租约无法续期或更改新代次。回执未知时先用原键查；尚未查到仍可能有在途提交，如重试须原样使用同键。回执只证明已排队。

Schema 24 追加一张审计表，无历史回填。两个唯一索引分别支持按操作员和原键回读、限制每个提交的旧代次只能重判一次；额外成本为每次成功重判写入两条 B-tree 记录及 WAL。停止维护写入后，空审计表可回退。已有审计记录须前滚修复；无需删除历史。可撤销角色授权停止新增操作，保留已有记录。

隔离 PostgreSQL 竞态回归覆盖同键／不同键并发、积压拒绝、文件核验期间撤权、非训练范围、旧租约拒绝及非空审计表回退拒绝。原键回读的 `EXPLAIN (ANALYZE, BUFFERS)` 在关闭顺序扫描的夹具中使用主键索引，全程访问两个缓存块；不作为容量数据。`aplusb-restore-loop` 在两个临时 PostgreSQL 14.24 容器和断网参考槽实测：间接成员和缺失材料被拒，专用登录入队并按原键读回，真实 Worker 在还原材料上完成第二个 AC，旧证据不变。该专项在同一宿主，未验证现场独立故障域。

[Ubuntu 本地恢复与重判](delivery-evidence/aplusb-restored-rejudge.md#ubuntu-arm64-专用客体)还验证未设档位、同版本不同运输摘要均拒绝且无新增审计或尝试；合法请求在同一提交上完成第二个 AC。现场独立故障域仍待验证。
