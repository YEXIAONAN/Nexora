# Nexora

Nexora 是面向 ZERO_R 等小型研发团队的内部工程管理平台，以项目、任务、成果提交、审核验收和历史留痕为核心，底层支持多个独立 Team。正式需求基线为 `docs/product/product-requirements.md`：**V1.0 已于 2026-11-17 正式批准（方案 B：多 Team 数据与权限架构先行，首版优先交付 ZERO_R 核心业务闭环）**；V2.0 原始需求完整保留为长期产品规划与历史决策记录。

复用现有 Vue + Go + PostgreSQL 工程，不替换架构。当前工程工作代号：**Phase 03（项目与三级任务）**；Phase 02 已合入 develop `c3fb029`。阶段代号仅用于工程排期，不构成产品范围决策；Phase 03 不代表 V1.0 开发完成。

> **当前阶段：Phase 03 项目与三级任务（已开发并验证，[PR #3](https://github.com/YEXIAONAN/Nexora/pull/3) 待最终评审，未合并）。**
> 项目、成员与交接、里程碑、归档只读、三级任务、开始/取消、依赖、精确权重进度及历史快照，见 [设计](docs/development/phase-03-design.md) 与 [验收报告](docs/development/phase-03-report.md)。
> 成果版本、待验收、正式完成、驳回与重新提交属于 Phase 04，尚未开发。实际进度最高 99%，有效进度最高 79%；不能手动完成。
> 运行中的旧开发栈未自动升级。新迁移只在独立一次性 PostgreSQL 验证，部署升级需显式操作。

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
make bootstrap           # ROOT 安全初始化（交互式设置密码，无默认密码）
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
