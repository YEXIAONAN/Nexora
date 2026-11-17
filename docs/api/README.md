# API 契约

OpenAPI 3.1 定义位于 [openapi.yaml](openapi.yaml)。

## 当前接口

### 系统（Phase 0）

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/healthz` | 进程存活，不访问数据库 |
| GET | `/readyz` | 数据库连接与 Schema 版本；未就绪返回 503 |
| GET | `/api/v1/meta` | 构建信息、环境、安装实例 ID 与 ROOT 引导状态 |

### 认证与平台管理（Phase 02，已开发并验证，PR #2 待合并）

| 方法 | 路径 | 认证 | 权限 | 说明 |
| --- | --- | --- | --- | --- |
| POST | `/api/v1/auth/login` | 无 | 公开（限流） | 登录，Set-Cookie 下发会话 |
| GET | `/api/v1/auth/me` | 会话 | 登录用户 | 当前用户信息 |
| DELETE | `/api/v1/auth/session` | 会话 | 登录用户 | 退出并撤销当前会话 |
| POST | `/api/v1/admin/users` | 会话 | ROOT / 平台管理员 | 创建普通账号 |
| DELETE | `/api/v1/admin/users/{userID}/sessions` | 会话 | ROOT / 平台管理员 | 撤销用户全部会话 |
| POST | `/api/v1/admin/platform-roles` | 会话 | 仅 ROOT | 授予平台管理员 |
| DELETE | `/api/v1/admin/platform-roles/{userID}` | 会话 | 仅 ROOT | 撤销平台管理员 |

### 团队与邀请（Phase 02，已开发并验证，PR #2 待合并）

| 方法 | 路径 | 认证 | 权限 | 说明 |
| --- | --- | --- | --- | --- |
| GET | `/api/v1/teams` | 会话 | 登录用户 | 我的团队（含本人角色） |
| POST | `/api/v1/teams` | 会话 | 登录用户 | 创建团队，创建者成为 owner |
| GET | `/api/v1/teams/{teamID}` | 会话 | active 成员 | 团队详情；非成员 404 |
| GET | `/api/v1/teams/{teamID}/permissions` | 会话 | active 成员 | 本人能力视图 |
| GET | `/api/v1/teams/{teamID}/members` | 会话 | active 成员 | active 成员列表 |
| PATCH | `/api/v1/teams/{teamID}/members/{userID}` | 会话 | owner / admin（受限） | 变更角色；最后 owner 保护 409 |
| DELETE | `/api/v1/teams/{teamID}/members/{userID}` | 会话 | owner / admin（受限）或本人退出 | 移除成员；立即生效 |
| POST | `/api/v1/teams/{teamID}/invitations` | 会话 | owner / admin | 邀请已有账号（admin/member，默认 72h） |
| GET | `/api/v1/teams/{teamID}/invitations` | 会话 | owner / admin | 团队邀请历史 |
| DELETE | `/api/v1/teams/{teamID}/invitations/{invitationID}` | 会话 | owner / admin | 取消 pending 邀请 |
| GET | `/api/v1/invitations` | 会话 | 登录用户 | 我收到的有效邀请 |
| POST | `/api/v1/invitations/{invitationID}/accept` | 会话 | 仅被邀请人 | 接受邀请并加入团队 |
| POST | `/api/v1/invitations/{invitationID}/decline` | 会话 | 仅被邀请人 | 拒绝邀请 |
| GET | `/api/v1/admin/teams` | 会话 | ROOT / 平台管理员 | 全部团队（仅可见性，不授予团队权限） |

不存在公众注册接口；团队角色（owner/admin/member）与平台角色相互独立。

## 响应约定

成功：

```json
{ "data": { "name": "Nexora", "version": "0.1.0" } }
```

失败：

```json
{
  "error": {
    "code": "not_found",
    "message": "the requested resource does not exist",
    "request_id": "6f1c..."
  }
}
```

- `fields` 仅在字段级校验失败时出现。
- 错误响应不包含堆栈、SQL 或连接串。
- 所有响应包含 `X-Request-Id`。
- `/api/v1` 下的变更请求（POST/PUT/PATCH/DELETE）必须携带 `X-Nexora-CSRF: 1` 头，缺失返回 403。
- 认证使用 HttpOnly Cookie `nexora_session`；生产环境强制 `Secure`，由 `NEXORA_ENV` 推导，无关闭开关。

## 生成前端类型

```bash
cd apps/web
pnpm api:types
```

生成结果写入 `apps/web/src/api/schema.d.ts`，已提交到仓库，确保没有网络时也能类型检查。

修改接口的顺序：

1. 更新 `openapi.yaml`。
2. 运行 `pnpm api:types`。
3. 更新前端调用与测试。
4. 更新本文件与相关 ADR。

## 版本与兼容

当前使用 URL 前缀 `/api/v1`。破坏性变更必须提供新版本前缀；新增可选字段视为兼容变更。
