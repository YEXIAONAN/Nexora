# Phase 06 M2 — Schema v10 奖金基础迁移与约束验收报告

- 阶段：**M2 · Draft / 待产品审批**（结构性数据库迁移已在 PostgreSQL 17 与全量 Go -race 检查通过，不代表金额业务上线许可）
- 日期：2026-11-20（v10 重排后）
- 基线：`develop` `cc88b063cd4943fb7e65243b0f4526ef1a142345`（Alpha `v0.1.0-alpha.1`，v8 = `00008_p0_accounts_invitations.sql`）；M1 [PR #10](https://github.com/YEXIAONAN/Nexora/pull/10) 尚未合并。
- 本分支：`feature/phase06-m2-schema`，已合入 Alpha P0 基线，Draft PR #11；不自动合并。
- 迁移编号：奖金迁移曾因 Alpha P0 的 `00008` 顺延；整合 Phase07 后，保留 `00009_task_progress_reports.sql`，尚未发布的奖金迁移改为 `00010_bonus_foundation.sql`。00001–00008 历史内容未修改，迁移链为唯一连续的 00001–00010。
- 产品方向：仅 CNY/分；D9 改为 **B**（项目归档后允许独立奖金域续办，贡献历史冻结）；D1/A1 为**不同真实资金批次**，相同正式贡献可跨批次复用，但每批次同一贡献确认不能重复绑定。其余金额/资格/核定业务政策仍 Proposed。
- 安全约束：未修改 v1–v8；未对开发/MVP/生产数据库运行迁移；未开放奖金金额 API、付款、核定或授权激活。

## 1. 数据模型交付

本次追加：`apps/api/internal/database/migrations/00010_bonus_foundation.sql`。

| 对象 | 作用与边界 |
| --- | --- |
| `bonus_rounds` | 每 Team/Project 的真实独立奖金批次；项目内规范化 `funding_key` 唯一，支持项目归档后新建；不反向更改项目 |
| `bonus_round_versions` | 批次内部不可变版本与受限双指针；币种锁定 CNY/分、预算最小单位整数、版本级冻结 |
| `bonus_round_sources` | 指向 Phase05 真实有效确认（含 task/case/version/confirmation/digest），每版本确认 ID 唯一，但**跨奖金批次允许相同确认 ID** |
| `bonus_eligibility_decisions` | 每版本、每持有人显式资格快照与成员 epoch；M2 只建结构，不启用正式审核工作流 |
| `bonus_allocation_lines` | `(team,project,round,version,source,user)` 逐来源×成员金额行；外键直指真实贡献份额及本版本资格 |
| `bonus_member_totals` | 成员金额仅由明细 SUM 派生；禁止独立手工写总金额 |
| `bonus_adjustments` | 逐来源的追加式调整记录：原金额、新金额、差额、来源未分配前后快照、操作人、理由、幂等请求 |
| `bonus_role_grants` | 内部成员 epoch/外部身份与限时授权的候选结构；**M2 一律禁止 active** |
| `bonus_confirmations` | 正式核定记录候选约束；**M2 禁止 INSERT**，独立核定业务尚未批准/实现 |
| `bonus_events` | 批次/版本事件审计事实，禁止 UPDATE/DELETE |

## 2. 当前 DB 已能验证的约束

1. 所有奖金对象保持 Team/Project/Round/Version 复合范围，禁止跨租户错误关联。
2. `funding_key` 采用小写 ASCII 标识并在项目内唯一；学校奖和学院奖可在同项目建立两个独立批次。
3. `UNIQUE(version_id,confirmation_id)` 防止同一正式贡献在同一方案版本重复绑定；没有跨奖金批次的 `confirmation_id` 全局唯一性。
4. 奖金来源 INSERT 要求 Phase05 `effective_confirmed_version_id`、`confirmed` 状态、当前无未决争议和完全相同的 `version_digest`；使用 Team 锁序列化；不要求项目仍是 active。
5. 分配行必须绑定相同的 `case_version_id`、正式 `contribution_shares` 中真实 `user_id/share_bps` 和相同方案版本内的显式资格决定；ineligible 行的建议与最终金额必须为 0。
6. 预算与各行金额非负、至多 `900000000000000` 个最小单位；仅允许 `currency_code='CNY'`、`minor_unit=2`。
7. M2 **默认封锁**修改/删除、非 Draft 版本创建、授权激活及确认记录插入；M3 必须以新迁移 + 已批准权限模型有意识地解除封锁，不能误称当前 Schema 已有可操作的完整金额业务。
8. v10 Down 只有在**无任何奖金批次**时可回滚到 v9（保留 Phase07 进展上报）；一旦出现真实批次证据，必须拒绝破坏性回退。

## 3. 明确未实现的约束与政策（后续门禁）

- **暂未启用**：完整逐来源/全计划金额守恒的提交与核定事务、最大余数计算、正式资格确认、独立核定利益冲突谓词、撤销/成员 epoch 实时重验、版本升级/争议/收件操作、资金批次真实性的业务审计。
- `funding_key` 唯一只能避免**相同标识**的重复批次，不能证明两笔名称不同的预算来自不同真实款项；需要产品确定资金来源凭据、核验和去重策略。
- 源贡献若在保存后被修订/推翻，M2 只有**写入时**校验，正式核定事务必须再次校验并处理历史冲突。
- 已归档项目奖金域的授权与离队人身份尚需独立业务决策；绝不依靠 ROOT 自动绕过。
- Bonus Dispute、Notice、Receipt 的最终 Schema 尚未创建：D8/A5 待批准后再追加，不得擅自直接复用贡献收件表。
- 仅创建 Schema 的含义是“**可在隔离测试库验证约束**”，不是“真实用户现在可以填写/核定奖金”。
- 当前迁移只创建新表/新视图/新函数/新触发器，不改 `contribution_archive_guard` 或任何 v1–v8 历史迁移。

## 4. 回归测试

- `apps/api/internal/database/phase06_m2_integration_test.go`：v8（Alpha P0）→v10→空奖金库回滚到 v9→再 v10、新表/视图、真实归档项目后创建学校/学院两个批次、同项目资金键防重、CNY/负数/上限、禁提前确认/授权、拒绝带业务数据回滚。
- `apps/api/internal/contributions/bonus_foundation_integration_test.go`：通过真实 Phase05 业务链生成被确认的贡献来源；两个奖金批次都可引用；同一版本重复来源/摘要篡改拒绝；显式资格、份额匹配与只读成员汇总。
- 前一经验证版本的 Push CI [44546637262](https://github.com/YEXIAONAN/Nexora/actions/runs/44546637262) 已 **4/4 Success**，含 Go -race Integration、v8 迁移和迁移状态；本轮额外加强的内部授权 epoch NULL 检查仍须在更新后的 HEAD 上再次完成 CI。

## 5. M2 独立评审门禁

- [ ] 最新 CI Backend / Frontend / Containers / Secret Scan 全部成功。
- [ ] 真实 PostgreSQL 17 验证全部新增测试。
- [ ] 评估应用角色对新表的 SELECT/INSERT/UPDATE/DELETE 权限，不得因迁移拥有者权限造成跨 Team 数据泄露。
- [ ] 逐项验证当前覆盖的 DB 约束确实在 DB 层生效，不以 UI 或报告措辞替代。
- [ ] M1 PR #10 决策统一、最新 CI 通过且经用户明确授权合并后，才可考虑 M2 合并至 develop。
- [ ] D1/D2/D3/D4/D6/D7/D8/A1/A3/A5 的未决资金与授权条款不能被“结构已存在”推定为 Approved。

**结论：当前仅是受限可回滚的 v10 奖金基础 Schema 草案；在 CI 与产品审批门禁之前不合并、不上线、不迁移已有数据库。**


## 6. 历史回归与最终安全加固记录（2026-11-19）

- **首次 CI 失败原因**：旧的 Phase 04/Phase 05 回滚集成测试把最新 Schema 错当为固定 v6/v7，新版本 v8 导致回滚目标发生偏移；不是 v8 安装 SQL 错误。
- **整改**：Phase 05 专项测试固定在 v7；Phase 04 证据回滚测试按当前版本计算步数并实际尝试跨越 v6→v5 边界；没有跳过旧测试或修改历史迁移。
- **Alpha / Phase07 基线整合**：保留已发布 v1–v8 与 Phase07 `00009_task_progress_reports.sql`，尚未发布的奖金迁移顺延为 `00010_bonus_foundation.sql`；兼容范围为 v9–v10，`BonusSchemaVersion=10`，v9 部署不挂载奖金路由，v8 不再可接收流量。历史回滚测试固定历史边界或按当前版本计算步数，避免 v10 让 Phase04/05 测试错误回滚。合并后的隔离 PostgreSQL 集成验证仍待重跑，以下 CI 结果仅为历史记录。
- **整改后 CI（修订基础版）**：Push [44546637262](https://github.com/YEXIAONAN/Nexora/actions/runs/44546637262) 四项全部成功，真实 PostgreSQL 与 Go -race 已通过；最终 HEAD 的 CI 需要另行核实。
- **NULL 约束安全加固**：内部 `bonus_role_grants.member_authorization_version` 必须显式 `IS NOT NULL` 且 > 0；增加有效用户、角色 proposed、但 epoch 缺失的回归用例。仅添加约束与测试，不激活任何真实业务核定权限。
- 上述完整工作保留在 Draft PR #11；M1 PR #10 仍独立 Draft，均未自动合并、未部署或迁移 MVP 数据。
