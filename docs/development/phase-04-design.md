# Phase 04 设计冻结

日期：2026-11-18（Asia/Shanghai）。来源：产品负责人 Phase 04 正式开发任务。设计先于业务实现；开发、验证和合并状态由阶段报告分别记录。

## 1. 开工与现有能力

重新 fetch 后 develop 为 c360327eef5ca73abb5c99379afc346815541a37（PR #3 合并），与用户提供定位相同；合并 CI 44118052169 的 Backend / Frontend / Secret scan / Containers 全部 success。相对 Phase 03 最终代码 47d94eb 无文件差异。工作区干净，创建 feature/phase04-submissions-reviews。main 不修改。

复用位置：

| 能力 | 位置 | Phase 04 使用方式 |
| --- | --- | --- |
| Cookie/CSRF、身份、平台角色 | auth、reqctx、httpx | 上传与业务动作同样认证/CSRF，不新增认证体系 |
| Team 锁及有效资格 | teams.Repository、projects.Service.Within | Team 锁先于任务/版本/审核，锁后实时重查成员 |
| 项目与参与者、归档 | projects | lead 唯一来源，归档业务严格只读 |
| 任务结构/执行权限/版本 | tasks | 新增受控成果工作流事务，不跨模块直接写 tasks |
| 叶子聚合与历史 | tasks/domain、history、repository_history | math/big，历史/快照/审计共事务 |
| 审计追加写 | audit | 复用不可变触发器及 actor/request_id |
| 集成与 E2E | testdb、e2e.sh、Playwright | 只用本次新建的一次性 PG17 及隔离文件目录 |

已检查 README、PRD、数据库/权限、ADR 0004/0005、Phase 03 设计/报告、路线图、OpenAPI、auth/projects/tasks/audit、迁移 1–5、前端任务工作区与现有测试。README 的 PR #3 未合并信息是待修正的历史快照。未发现与本次禁止自审/明确授予审核权限冲突的既有批准决策。

仓库没有成熟附件实现或受支持的对象存储配置。Docker 只读盘点不构成复用其他容器/卷的授权；不使用 SeaweedFS。新增私有文件根目录，独立于 Web 静态目录。

## 2. 新业务规则与权限

- 只有有效 Team + 项目成员中的当前任务负责人/协作者可提交或上传；项目 lead 未被分配该任务时不能代替提交者。普通执行进度权限保持 Phase 03 规则。
- 只对未阻塞的叶子任务，状态 in_progress 或 rejected 接受新成果；pending、awaiting_review、completed、cancelled、归档拒绝新提交。历史查询不改变状态。
- 内容至少有非空文字、合法证据链接或已成功暂存的附件之一；不判断业务成果的主观质量。说明上限 8000 字符，链接最多 20 个，每个 URL 最多 2048 字符。
- URL 只接受绝对 http/https、非空 host、无 userinfo；拒绝控制字符、危险协议。链接仅作为引用，不由服务器抓取，不声称自动验证 Git/PR 内容；前端外链使用 noopener/noreferrer。
- project lead 默认可审核；其他人需 lead 显式授予 project reviewer。Team owner/admin、ROOT、平台管理员不自动获得审核权。
- 一律禁止提交人审核自己的版本；没有其他合格审核人时必须先补充有效成员并授权。审核通过/驳回均要求 1–4000 字符意见。
- 授权/撤销要求原因并写审计。每次审核实时检查 Team、项目资格及 grant；project_members 增加资格世代，授权绑定该世代，退出后重新加入不会悄悄恢复旧审核授权。
- 明确授权的 reviewer 可读取所属项目的任务及成果，以完成审核；撤销后回落到普通负责人/协作者的原有读取范围。待我审核列表只返回本人可审、非本人提交、当前 awaiting_review 的版本。

## 3. 成果版本与状态机

一个 task_submissions 聚合根对应一个任务。每次新提交追加 task_submission_versions：稳定 UUID、同一任务单调递增的 version、author、summary、references、submitted_at、request_id、基于的 task version。唯一键防版本号和提交幂等键重复。

提交请求有 client_request_id UUID 与 expected_version。同一 actor、任务、幂等键、相同归一化内容重放只返回原版本，不再变状态；换内容复用键 409。不同请求在 Team 锁内串行，状态/版本冲突拒绝，不产生并行待审核版本。

任务明确保存 current_submission_version_id 与 accepted_review_id。最新提交不等于已验收。每个版本最多一条不可变正式决定；更正不能 UPDATE/DELETE，终态不重开。本阶段不提供改变正式结论的更正接口，未来必须以追加受控修订单独设计。

| 转换 | 唯一入口与证据 | 有效进度 |
| --- | --- | --- |
| pending→in_progress、pending/in_progress→cancelled | 原任务入口，取消必填原因 | 原规则 |
| in_progress/rejected→awaiting_review | 创建新有效版本并同事务绑定当前版本 | 80% |
| awaiting_review→completed | 对当前版本追加 approve 记录，非自审、明确资格 | 100% |
| awaiting_review→rejected | 对当前版本追加 reject 记录及意见 | 0% |
| rejected→in_progress | 原任务入口显式补正，必填原因，保留原版本/驳回记录 | min(actual,79%) |
| rejected→cancelled | 原合法取消权限与原因 | 剔除 |

通用状态入口仍不接受 awaiting_review/completed/rejected。awaiting_review 冻结任务分配/编辑/权重/结构，completed 终态只读；涉及待审核/完成后代的结构移动拒绝。已有 Phase 03 普通开始/取消、深度/依赖/权限保持回归。

rejected=0% 是本次明确的保守规则，会从待验收 80% 回到 0%；显式进入补正后才按原 actual 的上限 79% 参与计算，必须写历史与快照说明。

只有当前版本有效 approve 证据才解除未忽略依赖；待验收、驳回、取消都不解除。每次读取从数据库事实重新加载，避免旧缓存。取消/忽略语义不变。

## 4. 数据库完整性与事务

只新增 00006，绝不修改 00001–00005。新增：task_submissions、task_submission_versions、task_reviews、project_reviewers、submission_attachments；tasks 增加当前版本与通过记录引用；project_members 增加资格世代。

所有业务行含 team_id，UUID 及 (team,project,task,version) 复合外键，默认 RESTRICT。版本及正式审核禁止 UPDATE/DELETE；附件绑定后元数据和版本关系不可变。审核唯一(version_id)，版本唯一(task_id,version)、唯一(task_id,client_request_id)。

受控替换 Phase 03 tasks_guard_execution：合法来源/目标明确校验；不能仅改 status 完成。延迟约束触发器在提交时验证 current version 是该任务最新版本、正式决定指向当前版本、completed 关联 approve，rejected 关联 reject，版本/审核与任务状态同事务落地。审核插入还检查实时资格和禁止自审；仅 Service 检查不够。

submissions 模块拥有版本、审核、授权及附件元数据；tasks 模块拥有任务状态/进度与历史，通过受控工作流方法在同一 pgx.Tx 协作，不跨模块写对方表。数据库 view 暴露当前验收证据供 tasks 聚合，纯函数接受明确的当前/被审核版本及审核决定引用，不能仅相信 completed 字符串。

新提交/正式决定、任务状态/version、历史、进度快照及审计同事务提交。迟到旧版本审核、不存在版本、重复决定、旧 expected_version 均拒绝；失败全部回滚。历史快照不追溯改写。

## 5. 私有附件存储

采用单实例私有目录：NEXORA_ATTACHMENT_DIR；开发默认 apps/api/.data/attachments（从 API 工作目录解析），生产必须显式绝对路径。目录 0700、文件 0600，拒绝复用宽权限目录，部署不挂载为 Nginx 静态内容。Go os.Root 约束目录访问，服务端随机对象标识，O_EXCL 创建，fsync 后落地，禁止覆盖和路径逃逸。上传文件名只作为元数据，不能进入存储路径。

初期限制：单文件 10 MiB；版本最多 5 个附件、合计 25 MiB；Team 私有存储配额 512 MiB；未绑定暂存 24h 过期。允许 UTF-8 txt/md/csv、PDF、PNG、JPEG；扩展名与服务端内容检测必须匹配，拒绝 HTML/SVG、可执行/未知二进制、路径分隔符、控制字符及空文件。未接入恶意软件扫描，不将文件作为可信页面渲染。

上传为 multipart，要求 X-Nexora-Upload-Id UUID，重复相同文件请求返回原暂存记录，冲突内容拒绝；对象存储标识仍由服务端随机生成。文件写入后创建 staged 元数据；提交成果时在同一事务将本人的同任务暂存对象绑定具体版本。原文件不搬移，版本提交失败仍可重新使用暂存附件。

异常窗口与恢复：写文件/元数据失败先核实是否已提交，再安全删除未引用对象；无法确定数据库提交结果时保留对象由恢复流程处理，不能误删已提交文件。启动前及每小时维护扫描本私有根下 UUID Team 目录；过期 staged 标记 expired（审计），删除文件后记录 removed_at。删除失败仍计占用配额并可重试；bound 对象从不由清理流程删除。未引用文件经过 24h 宽限后清理，未引用临时 part 文件同样清理。提供本人暂存附件显式丢弃入口；已绑定对象不允许删除。

下载 URL 必须同时指定 Team/Project/Task/Version/Attachment，重新检查任务/审核读取权限与绑定关系；不存在或跨上下文 404。Content-Disposition 使用安全编码的 attachment，Content-Type 固定 application/octet-stream，nosniff、私有缓存、CSP sandbox。下载不直接渲染上传内容。绑定对象意外丢失时返回可解释的 503，保留数据库历史，绝不假造下载成功。

部署需同时备份数据库和私有文件根；单实例文件服务不跨副本共享。未来对象存储迁移须保留存储标识/哈希/版本引用和同样下载授权，不复用未经评估的历史卷。

## 6. 前端与 API

先更新 OpenAPI，再生成类型。新增成果版本列表/详情、提交和文件上传、审核权限管理、待我审核、通过/驳回意见、补正/重提与时间线。当前版本和具体被审核版本分开显示，已提交不标记完成。所有写入捕获已确认 Team/Project/Task/Version 及 expected_version；加载世代与幂等键避免迟到响应污染。审核权限不能由前端隐藏按钮代替后端校验。

任务工作区继续展示服务端项目进度；提交/审核成功后重新加载任务树/历史/快照。真实 Cookie/CSRF/API；不增加大型组件库或 Mock 后门。

## 7. 兼容风险、文档与验证

- 新数据库 view/证据列要求 v6，新服务启动前检查迁移版本，拒绝以旧 schema 半可用运行；不自动升级当前旧开发栈。
- Phase 03 纯函数的无证据 completed 仍为 0，新增有证据分支与正反例；旧进度快照原样保留。Phase 03 未来状态拒绝测试继续拒绝无证据通用转换。
- 新 reviewer 权限扩大了被授权成员的任务读取，需要跨 Team/Project、撤销/退出/重新加入、防自审回归。
- 真实 PG 验证空库、v5→v6、一次性回滚；包含旧任务和历史的数据升级不能覆盖过去快照。含 Phase 04 证据的破坏性回滚须明确拒绝，避免丢失历史。
- 附件与 DB 无分布式事务，以不可覆盖对象 + 暂存状态/幂等 + 失败补偿/恢复提供可解释一致性。真实文件系统和故障注入测试必须覆盖。
- README 说明 V1.0/V1.1/V2.0、Phase 01–03 已合并、Phase 04 特性状态，新增功能矩阵、真实命令、独立测试库、使用流程与部署边界。main 仅给发布检查清单，不修改旧发布代码或 README。

M1 设计/契约 → M2 Schema/不可变版本 → M3 成果/审核/状态/进度/依赖 → M4 私有附件安全 → M5 前端 → M6 README/文档 → M7 全门禁/E2E/PR/CI/评审。按里程碑普通提交，不自动合并。

### 待验收期间负责人离队（最终边界核查）

当前有效审核人仍可正式通过或驳回已提交的不可变版本，提交作者的历史成员身份不需要持续有效。否则 awaiting_review 冻结编辑且无法审核，会永久卡住任务。数据库只允许该场景改变 status / accepted_review_id / version / updated_at；延迟证据约束仍要求当前版本真实独立审核，其他赋值、执行及新提交仍要求有效负责人。驳回后负责人可正式重新分配补正。新增真实 PostgreSQL 红/绿回归覆盖 approve 和 reject；不授予离队者任何新权限。
