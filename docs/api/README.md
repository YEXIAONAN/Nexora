# API 契约

OpenAPI 3.1 定义位于 [openapi.yaml](openapi.yaml)。

## 当前接口（Phase 0）

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/healthz` | 进程存活，不访问数据库 |
| GET | `/readyz` | 数据库连接与 Schema 版本；未就绪返回 503 |
| GET | `/api/v1/meta` | 构建信息、环境与安装实例 ID |

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
