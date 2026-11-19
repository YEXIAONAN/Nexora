# Phase 05 M5 可靠性与质量加固验收报告

日期：2026-11-19（Asia/Shanghai）。分支：`feature/phase05-m5-hardening`。范围：贡献告知业务幂等、前端重复请求与取消传播、状态/成员展示整理、外部证据链接协议加固、过期状态文档更新。Schema 保持 v7；未修改 `00001–00007` 历史迁移、后端权限模型、OpenAPI 契约或奖金逻辑。

## 1. 基线与门禁

- develop：`9c8b0e558933b9b6fcdc22ad49acf7f587bb32e3`，为 M4 PR [#7](https://github.com/YEXIAONAN/Nexora/pull/7) 的 Merge Commit。
- M4 合并后 develop CI [44468968227](https://github.com/YEXIAONAN/Nexora/actions/runs/44468968227) 四项成功，确认后才开始 M5。
- `feature/phase05-m5-hardening` 远端分支存在且起点等于 develop；本地跟踪分支从其建立。
- 开工工作区干净，无 stash、无未提交修改；未修改 `main`/`develop`，未操作开发数据库。
- 本次只新增提交，不修改历史报告中的既有测试结果。

## 2. 重复告知与幂等修复

### 2.1 修复前事实

- `contribution_notifications` 只有主键与普通索引，没有 `(confirmation_id, recipient_id, channel)` 业务唯一约束。
- `PrepareNotice` 复用通用 `caseCommand`，幂等仅依赖 `client_request_id` 的审计重放。
- 网络超时、响应丢失、页面刷新或重复点击会生成新的 `client_request_id`，从而向同一核定版本、同一接收人插入第二条告知。
- 告知插入会触发 `contribution_touch` 增加案例 revision；旧逻辑先校验 `expected_version` 再插入，因此并发重试还可能得到 409。

### 2.2 业务去重键

一个告知的业务身份定义为：

```text
team_id + project_id + task_id + case_id + case_version_id
+ confirmation_id + recipient_id + channel(authenticated_ack)
```

`confirmation_id` 与具体核定版本绑定：同一核定的重试命中同一业务键；新修订版本产生新的 confirmation，因此必须单独告知。不会把新版本错误合并到旧告知。

### 2.3 事务与并发

新增 `noticeCommand`：

1. 校验会话、案例可见性、`CanEditCases`、项目 active；
2. 校验 `client_request_id` 重放；
3. 通过 `contribution_lock` 按 Team→Case 顺序加锁；
4. 锁内重新读取案例权威 revision；
5. 先按业务键查找最早告知：存在则返回该不可变告知 ID，不再插入，也不再因首次插入导致的 revision 变化而 409；
6. 仅当不存在时校验 `expected_version`、当前有效确认、确认 ID 和受益人，再插入；
7. 同一事务写入 command 完成审计并返回权威 revision。

因此：

- 相同 `client_request_id`：标准审计重放，返回原结果；
- 不同 `client_request_id`、相同业务意图：返回最早告知，不新增记录；
- 并发相同意图：Team 锁串行化，最终只有一条告知，两个请求返回同一 ID；
- 新修订版本：不同 confirmation，生成独立告知；
- 收件 POST 仍绑定实际 notice、版本、确认和摘要，原收件与不可变审计证据不受影响。

### 2.4 既有重复记录

未删除或改写任何历史行。若数据库在 M5 前已存在重复告知，新请求解析到 `prepared_at,id` 最早的一条，不再产生第三条；历史重复仍保留可审计。未增加数据库唯一索引，原因是直接建唯一索引会要求删除或改写既有重复历史，违反不可变审计边界。当前生产代码只有 `PrepareNotice` 一个告知写入路径，Team→Case 锁保证应用层串行化；如未来出现新的直接写入路径，应另批追加迁移并先批准历史兼容方案。

## 3. 前端请求与质量加固

| 文件 | 变更 |
| --- | --- |
| `views/MyContributionResultsView.vue` | 同一案例多个告知只读取一次 detail/evidence/versions；每项目只读取一次成员；per-loader Map 不跨 Team/Project/身份存活 |
| `views/ProjectContributionsView.vue` | `getTaskTree`、`listVersions` 接收并传递 `AbortSignal`；移除本地重复状态标签 |
| `api/tasks.ts`、`api/submissions.ts` | 可选 `AbortSignal`，保持既有调用兼容 |
| `components/contributions/contributionPresentation.ts` | 统一案例状态词表；成员名称解析；删除生产未使用的 `canSubmitShares`/`totalBasisPoints` |
| `ContributionVersionPanel`、`ContributionHistoryPanel`、`ContributionCaseView`、`ContributionAppraisalView` | 优先显示真实成员名称，未知身份显示 `成员 xxxxxxxx…`，不再无故暴露完整 UUID |
| `ContributionEvidencePanel.vue` | 外部证据仅激活 HTTPS；HTTP 仅允许 loopback 开发地址；拒绝带凭据 URL 和非 HTTP(S) 协议 |
| `e2e/phase05.spec.ts` | 真实栈模拟响应丢失后用新请求 ID 和旧 revision 重试，验证返回同一告知且仍只有一条 |

没有新增批量 API、缓存层、自动评分、奖金或跨 Team 共享缓存。缓存只存在于单次 loader 调用内，上下文切换时随 generation/abort 一并丢弃。

## 4. 测试与证据

先写失败测试（提交 `5e50412`），在修复前实际失败：

```text
--- FAIL: TestServiceContributionNoticeIntentDeduplication
    the same confirmed result and recipient created a duplicate notice
--- FAIL: TestServiceConcurrentNoticeIntentIsSingle
    conflict: the resource changed; reload before retrying
```

修复后（`b9194b0`）同一测试通过。最终门禁：

| 检查 | 结果 |
| --- | --- |
| Vitest | 25 文件 / **209 项通过**（旧 206 基线；删除 1 项仅测试辅助函数断言，新增 4 项行为测试，净增 3） |
| Playwright 全量真实栈 | **31/31 通过**（约 1.3m；原 25 + M4 6，新增丢响应重试断言位于第 3 条场景） |
| Go 全量 `-race -count=1 -tags=integration ./...` | 全部包 ok；contributions 163.945s、database 128.927s、httpapi 97.601s，PostgreSQL 17.11 |
| `make verify` | 通过（lint、gofmt/vet、类型、格式、构建、契约、secret scan 8 项自测） |
| 覆盖场景 | 同请求重放；不同请求 ID 同业务意图；旧 revision 的低响应重试；并发准备；新修订重新告知；收件绑定；缺少新版本收件时归档 409；跨 Team/Project 隔离；取消与迟到响应；M4 全流程 UI |

专用测试容器只使用 `nexora-devtest-pg`（loopback 5433、`nexora_test`/`nexora_e2e`），测试后删除；未连接或迁移开发数据库。

## 5. 提交

| 交付 | SHA |
| --- | --- |
| 失败回归测试 | `5e50412` |
| 告知幂等修复 | `b9194b0` |
| 贡献标签与成员展示整理 | `cf690af` |
| 前端请求去重与取消传播 | `cd7fa92` |
| 状态文档与验收报告 | 本提交 |

## 6. 仍存在的限制

- 既有重复告知按不可变历史保留，不在本次自动清理；界面可能仍显示 M5 前的历史重复记录，需数据治理单独批准处理方式。
- 未增加数据库唯一索引；并发安全依赖所有告知写入都经过贡献 Service 与 Team→Case 锁。新增旁路写入必须先补迁移与约束。
- 本人结果仍按项目与案例逐个读取，未引入批量契约；大量项目/历史下的分页与批量性能留待后续契约设计。
- 离队成员结果披露与可信送达通道仍未获产品批准；缺少可信收件时归档继续受阻。
- 不包含 M6、奖金、金额、排名或完整通知系统。

## 7. 评审 PR

指向 `develop` 的评审 PR 与本报告最终 CI 结果见交付回复及 PR Checks；不自动合并，不进入 M6。
