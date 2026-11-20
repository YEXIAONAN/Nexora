# Phase 06 M3 — 奖金只读 API 与工程验收报告

- 阶段：**M3 Draft（依赖 M1/M2，未合并、未部署、未开放写入）**
- 日期：2026-11-20（Alpha 基线整合后）
- 分支：`feature/phase06-m3-my-results-readonly`
- 基线：M2 Schema v9（`feature/phase06-m2-schema`，[PR #11](https://github.com/YEXIAONAN/Nexora/pull/11)，Draft）；M1 设计 [PR #10](https://github.com/YEXIAONAN/Nexora/pull/10)，Draft；`develop` `cc88b06`（Alpha `v0.1.0-alpha.1`，v8 P0）
- 目标：修复 M3 半成品，交付可测试、可扩展的奖金只读基础能力，**不开放正式核定、预算、资格、调整或付款写入**

## 1. 本轮修复的半成品问题

| 问题 | 修复 |
| --- | --- |
| 本人结果只返回金额，未区分“已核定”和“已支付” | 响应新增 `status="confirmed"` 与 `payment_status="not_recorded"`，明确系统不执行/不记录付款 |
| 仅校验 Team 成员，Team 成员但无任何 Project 成员关系时返回 200 空集合 | 访问快照同时要求活跃 Team 成员和至少一个活跃 Project 成员；每条结果仍要求该 Project 的活跃成员关系 |
| v7/v8 路由开关硬编码 `>= 8` | 新增 `database.SupportsBonusSchema` / `BonusSchemaVersion`，启动与测试共用同一判定（现为 v9） |
| OpenAPI 缺少方案版本、批次、派生状态字段 | 补充批次列表/详情契约，并重新生成 `schema.d.ts` |
| 生成类型与 YAML 不一致导致 Frontend CI 失败 | `pnpm api:types` 重新生成，`pnpm api:check` 通过 |
| 受限应用角色对新表的 SELECT 未在部署文档中明确 | 更新 `docs/operations/application-database-role.md`，仅授予 SELECT，明确无 INSERT/UPDATE/DELETE |
| 金额聚合为 PostgreSQL `numeric` | 显式 `::bigint`，避免 JSON 数字精度/类型歧义 |

## 2. 已交付 API

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/v1/teams/{teamID}/bonus-results/mine` | 仅当前会话本人的生效、正式核定结果；起草/待核定不返回 |
| GET | `/api/v1/teams/{teamID}/projects/{projectID}/bonus-rounds` | 当前 Team + Project 的奖金批次元数据；不含预算/成员金额 |
| GET | `/api/v1/teams/{teamID}/projects/{projectID}/bonus-rounds/{roundID}` | 单个批次、方案版本历史与正式有效版本；不含金额 |

响应边界：

- `bonus-results/mine` 只按服务端会话身份取 `user_id`，不接受客户端指定受益人；同一人的不同批次分别返回。
- 批次接口返回 `funding_key`、`funding_label`、`funding_source`、派生 `status`、`latest_version`、`effective_confirmed_version`、`versions`。
- `amounts_visible` 在本阶段恒为 `false`，用于区分“金额被隐藏”和“正式金额为 0”。
- 无数据返回显式 `items: []`，不伪造测试奖金。

## 3. 权限与隔离矩阵

| 场景 | 本人结果 | 批次列表/详情 |
| --- | --- | --- |
| 未登录 | 401 | 401 |
| 无效/过期/已撤销会话 | 401 | 401 |
| 非 Team 成员 | 404 | 404 |
| Team 成员但非 Project 成员 | 404 | 404 |
| 活跃 Team + Project 成员 | 200，仅本人结果 | 200，仅元数据，无金额 |
| ROOT（无对应成员关系） | 404 | 404 |
| ROOT（有正常成员关系） | 与普通成员一致，无额外绕过 | 与普通成员一致，无额外绕过 |
| 跨 Team / 跨 Project ID | 404 | 404 |
| 归档项目 | 依成员关系正常返回 | 依成员关系正常返回 |

所有访问判定与数据查询在**同一条 SQL 快照**中完成；会话撤销、成员移除在下一次请求立即生效。ROOT、Team Owner、Project Lead 身份本身不产生奖金权限。

## 4. Schema v8 / v9 兼容

- 启动与 `/readyz` 接受 v8–v9；v7、v10 及未迁移数据库拒绝。
- v8（Alpha P0）：P0 注册/登录、用户管理、邀请、Team/Project、任务与贡献功能正常；奖金路由**不挂载**，请求得到安全 404，不会查询缺失表产生 500。
- v9：挂载全部奖金只读路由。
- v8→v9 升级测试验证 P0 用户（email/is_active）、待接受邀请、Team、Project 数据保持不变；v9 迁移、空库回滚与历史 Phase05 回归均在隔离 `testdb` schema 中验证；未修改 00001–00008 迁移。

## 5. 验证结果

| 检查 | 结果 |
| --- | --- |
| `gofmt -l` / `go vet` / `go vet -tags=integration` | PASS |
| `go test ./...` | PASS |
| `go test -race -count=1 -tags=integration ./...`（独立 disposable PostgreSQL 17） | PASS |
| `make verify`（lint、unit、build、api-check、secret scan、script safety） | PASS |
| `pnpm api:check` | PASS |
| Vitest | 209/209 PASS |
| Frontend lint / format / typecheck / build | PASS |
| Secret scan | PASS |

新增 PostgreSQL 集成测试覆盖：归档项目可读、同项目两个独立批次、草稿不形成正式结果、跨 Team/Project、Team-only 成员、ROOT、过期/撤销会话、撤销 Project 成员、受限应用角色只读与写入拒绝、v8 路由缺失。

## 6. 明确未实现 / 待批准

1. **正向正式结果未做数据级验证**：M2 的 `bonus_m2_confirmation_disabled` 仍禁止插入 `bonus_confirmations`，因此测试环境本人结果为空是预期行为；未通过关闭触发器或伪造确认来制造测试金额。
2. **写入能力全部未开放**：创建批次、预算、资格、调整、提交、正式核定均需要 M1 权限模型和后续经批准的迁移；本轮未解除 M2 门禁。
3. **奖金金额可见性授权未实现**：M2 的 `bonus_role_grants` 仍只能 `proposed`，当前没有任何角色可查看预算或成员金额；批次接口对所有项目成员统一隐藏金额。
4. **付款状态不跟踪**：`payment_status="not_recorded"` 只表示本阶段不记录付款，不代表未付款或已付款事实。
5. **同一批次多版本**：查询按 `bonus_rounds` 一行返回，不会把版本计为多笔奖金；但 M2 只允许单一草稿版本，无法构造多版本正向夹具。
6. **生产授权重跑**：应用 v9 迁移后必须按更新后的部署文档重新执行 SELECT 授权，否则只读接口会因缺少新表权限返回 500。
7. D2–D4、D6–D8、D10、A2–A5 等金额、资格、独立核定、离队送达与外部审计政策仍为 **Proposed**，未因本阶段代码推定批准。

## 7. 边界确认

- 未修改 `main`、`develop`、历史迁移 00001–00008、MVP 运行环境或真实业务数据库。
- 未自动合并任何 PR，未部署，未执行付款，未创建正式奖金记录。
- 未新增框架或依赖；保持 Handler → Service → Repository → PostgreSQL 分层。
