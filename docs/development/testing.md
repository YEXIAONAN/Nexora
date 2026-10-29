# 测试说明

## 分层

| 层 | 范围 | 命令 |
| --- | --- | --- |
| Go 单元测试 | 配置校验、错误映射、中间件、健康检查 | `make test-api` |
| Go 集成测试 | 真实 PostgreSQL、迁移幂等、Repository | `go test -tags=integration ./...` |
| 前端单元/组件测试 | Pinia store、API 失败处理、状态徽章 | `make test-web` |
| 类型检查 | `vue-tsc --noEmit` | `pnpm typecheck` |
| 静态检查 | gofmt、go vet、ESLint、Prettier | `make lint` |
| 构建 | Go build、Vite production build | `make build` |
| 容器 | Dockerfile 构建、Compose 配置 | CI `containers` job |

## 本地执行

```bash
make test
make lint
```

集成测试：

```bash
make dev-db
make migrate-up
cd apps/api
TEST_DATABASE_URL="postgres://nexora:nexora_dev_password@localhost:5432/nexora?sslmode=disable" \
  go test -tags=integration ./...
```

集成测试使用真实表与迁移，不创建 mock 数据库。

## 覆盖重点

优先覆盖高风险业务，而不是追求数字：

- 权限与跨团队 IDOR（Phase 1 起）
- 状态机非法转换（Phase 2）
- 进度聚合与权重（Phase 2）
- 验收版本与重提（Phase 3）
- 贡献修订留痕（Phase 4）
- 金额舍入、合计上限、并发确认（Phase 5）
- 导出权限过滤（Phase 7）

## CI

`.github/workflows/ci.yml`：

1. Backend：格式、vet、迁移、`go test -race -tags=integration`。
2. Frontend：lint、format check、typecheck、test、build。
3. Secret scan：检查已跟踪文件中的常见凭据模式。
4. Containers：`docker compose config`、构建 API 与 Web 镜像。

测试失败不得通过删除测试或降低类型检查绕过。
