# Nexora V1.0

Nexora 当前优先服务 ZERO_R 内部小型研发团队，V1.0 目标是任务分配、执行、成果提交、负责人验收与必要操作记录。完整多团队、贡献核算、奖金自动分配和甘特图不属于当前版本范围。

复用现有 Vue + Go + PostgreSQL 工程，不替换架构。仓库中的 V2.0、Phase 0 及旧路线图记录是历史背景；当前 Phase 01 为工程基线修复，Phase 02 才进入身份认证与权限开发。

> **当前阶段：V1.0 Phase 01 工程基线修复。**
> 当前版本只有健康检查、实例元数据与工程基础设施，没有认证、团队、项目或任务接口。
> 认证与授权完成前，请勿将本服务暴露到公网。构建通过不代表数据库及完整启动已验证。

## 技术栈

| 层 | 选型 |
| --- | --- |
| 前端 | Vue 3、TypeScript、Vite、Pinia、Vue Router、Tailwind CSS |
| 后端 | Go、chi、pgx、goose、PostgreSQL 17 |
| 契约 | OpenAPI 3.1，前端类型由契约生成 |
| 工程 | pnpm、Docker Compose、GitHub Actions、ESLint、Prettier、Vitest |

## 目录结构

```text
apps/
  api/            Go 模块化单体（cmd + internal）
  web/            Vue 3 单页应用
docs/
  api/            OpenAPI 契约与接口说明
  architecture/   系统架构、数据库、权限、ADR
  development/    本地开发、测试、路线图、阶段报告
  operations/     部署与备份恢复
  product/        产品需求与业务规则
infra/
  docker/         API / Web 镜像与 nginx 配置
  scripts/        备份、恢复、验证与密钥扫描
.github/          CI 与依赖更新
```

## 快速开始（本地开发）

前置条件：Go 1.26.1+、Node.js 24（CI 基线；package.json 允许 Node 22）、pnpm 10.34.5、Python 3.9+（扫描门禁）、Docker（含 Compose v2）。

```bash
make env                 # 从 .env.example 生成 .env（仅开发默认值）
make dev-db              # 启动 PostgreSQL
make migrate-up          # 应用数据库迁移
make dev-api             # 终端 A：http://127.0.0.1:8080
make dev-web             # 终端 B：http://127.0.0.1:5173
```

验证：

```bash
curl -s http://127.0.0.1:8080/healthz
curl -s http://127.0.0.1:8080/readyz
curl -s http://127.0.0.1:8080/api/v1/meta
```

Vite 开发服务器将 `/api`、`/healthz`、`/readyz` 代理到 `127.0.0.1:8080`。

## 容器化运行

```bash
docker compose --profile full up -d --build
# Web: http://localhost:3000
# API: http://localhost:8080
```

仅启动数据库时使用 `docker compose up -d postgres`（默认 profile 只包含 PostgreSQL）。

## 质量门禁

```bash
make lint          # gofmt、go vet、ESLint、Prettier、vue-tsc
make test          # Go 单元测试 + Vitest
make build         # Go 构建 + 前端生产构建
make secret-scan   # 已跟踪文件的密钥扫描
make verify        # 以上全部
```

数据库集成测试只允许显式配置的独立可丢弃 PostgreSQL，不能使用普通开发或生产库。每个测试使用自动清理的独立 schema，详见测试说明：

```bash
cd apps/api
NEXORA_ENV=test NEXORA_TEST_DB_DISPOSABLE=1 \
TEST_DATABASE_URL="postgres://test_runner@127.0.0.1:5432/nexora_test?sslmode=disable" \
  go test -tags=integration ./...
```

## 数据库与备份

```bash
make migrate-status    # 迁移状态
make backup            # 生成并校验备份
make verify-backup     # 恢复到临时库并验证
```

详见 [备份与恢复](docs/operations/backup-restore.md)。

## 文档

- [产品需求与业务规则](docs/product/product-requirements.md)
- [系统架构](docs/architecture/system-architecture.md)
- [数据库设计](docs/architecture/database.md)
- [权限矩阵](docs/architecture/permissions.md)
- [API 契约](docs/api/openapi.yaml) / [接口说明](docs/api/README.md)
- [本地开发](docs/development/local-development.md)
- [测试说明](docs/development/testing.md)
- [开发路线图](docs/development/roadmap.md)
- [部署说明](docs/operations/deployment.md)
- [备份与恢复](docs/operations/backup-restore.md)
- [架构决策记录](docs/architecture/adr/)

## 分支与提交

- `main`：可发布分支
- `develop`：集成分支
- `feature/*`、`fix/*`：短生命周期分支

提交遵循 Conventional Commits，例如 `feat(tasks): implement task lifecycle`。
详细约定见 [CONTRIBUTING.md](CONTRIBUTING.md)。

## 许可

许可尚未确定，因此本仓库暂不包含 LICENSE 文件。在许可明确前，代码不授予外部使用许可。
