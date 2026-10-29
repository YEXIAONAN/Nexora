# 贡献指南

## 开发流程

1. 从 `develop` 创建分支：`feature/<scope>-<summary>` 或 `fix/<scope>-<summary>`。
2. 先实现最小可验证的业务闭环，再扩展。
3. 本地执行 `make verify`；数据库相关改动还需执行集成测试。
4. 使用 Conventional Commits 提交。
5. 提交 Pull Request，说明变更、验证命令、数据库迁移与风险。

## 提交信息

```text
<type>(<scope>): <summary>

<body>
```

允许的 type：`feat`、`fix`、`refactor`、`test`、`docs`、`chore`、`build`、`ci`、`perf`。
scope 使用模块名，例如 `auth`、`teams`、`tasks`、`reviews`、`contributions`、`bonus`。

## 提交前检查

```bash
make lint
make test
make build
make secret-scan
git diff --check
git status --short
```

涉及数据库时：

```bash
make migrate-up
make migrate-status
cd apps/api && TEST_DATABASE_URL="..." go test -tags=integration ./...
```

## 代码要求

- 权限必须在后端强制执行，前端隐藏按钮不构成权限控制。
- 所有团队数据查询必须显式限定 `team_id`。
- 业务状态变化必须写入审计记录，并使用事务保证一致性。
- 金额使用整数最小货币单位或 PostgreSQL `numeric`，禁止 `float`。
- 时间以 UTC 存储，展示时转换。
- 新增或修改接口时先更新 `docs/api/openapi.yaml`，再生成前端类型。
- 不引入 Mock 数据冒充真实实现；未完成功能必须明确标注。

## 数据库迁移

- 迁移文件位于 `apps/api/internal/database/migrations`，命名 `NNNNN_description.sql`。
- 每个迁移必须包含 `-- +goose Up` 与 `-- +goose Down`。
- 已合并到 `main` 的迁移不得修改；新增变更使用新 migration。
- 生产环境禁止自动迁移，必须显式运行 `nexora-migrate -command up`。
- 破坏性变更（删列、改类型、数据重写）必须提供回填与回滚说明。

## 安全

禁止提交 `.env`、令牌、私钥、数据库密码或真实个人数据。
仅 `.env.example` 可以包含无害的开发默认值。
发现凭据泄漏时，先在 GitHub 撤销/轮换，再处理 Git 历史，不要只删除文件。
