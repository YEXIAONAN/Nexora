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

集成测试必须使用事先独立创建、可丢弃的 `nexora_test` 或 `nexora_test_<suffix>` 数据库，以及具备创建 schema 权限的测试用户。凭据通过本地安全环境注入，不在文档中填写真实值。不要先对普通业务库执行迁移：

```bash
cd apps/api
NEXORA_ENV=test NEXORA_TEST_DB_DISPOSABLE=1 \
TEST_DATABASE_URL="postgres://test_runner@127.0.0.1:5432/nexora_test?sslmode=disable" \
  go test -tags=integration ./...
```

缺少 `TEST_DATABASE_URL` 时跳过，绝不回退到应用数据库变量。配置了 URL 但不满足测试模式、可丢弃确认、loopback 主机及专用库名时明确失败；远程主机、生产库名、连接重定向参数及相关 PG 环境覆盖被拒绝。URL 只允许可选的单个 `sslmode` 参数。

测试使用真实 PostgreSQL；每个 fixture 创建随机 `nexora_it_` schema，将连接的 `search_path` 限定到该 schema，在成功或失败后仅清理自己生成的 schema。迁移和 Repository 测试分别建立自己的基线，不依赖另一个测试的执行顺序。没有安全测试库时只能报告集成验证未完成。

## 密钥扫描与契约门禁

`make secret-scan` 在本地和 CI 均先执行扫描器回归测试，再扫描 Git 跟踪文件和未忽略的新文件。需要 Python 3.9+，仅使用标准库。输出仅包含路径、行号和规则，不打印命中内容；被跟踪的真实环境文件直接拒绝且不读取。

`infra/scripts/secret-scan-allowlist.json` 只豁免 `.env.example`、开发 Compose 和 CI 中已知虚拟 PostgreSQL 示例值，逐项绑定文件路径、规则及完整命中值的 SHA-256，并注明理由。没有整目录或整文件豁免；示例值改变、复制到其他路径或出现其他高风险凭据仍失败。新增例外必须单独审查，不能自动刷新白名单。

`make api-check` 在内存中重新生成 OpenAPI 类型并与提交文件比较，不写入源码；不一致时退出非零。本地 `make verify` 和 CI 都包含相同检查。

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
3. Secret scan：运行扫描器正反例回归测试和源码扫描，失败时返回非零。
4. Containers：`docker compose config`、构建 API 与 Web 镜像。

测试失败不得通过删除测试或降低类型检查绕过。
