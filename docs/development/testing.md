# 测试说明

## 分层

| 层 | 范围 | 命令 |
| --- | --- | --- |
| Go 单元测试 | 配置校验、错误映射、中间件、健康检查、权限矩阵推导 | `make test-api` |
| Go 集成测试 | 真实 PostgreSQL、迁移幂等、Repository、认证与跨团队授权 | `go test -tags=integration ./...` |
| 前端单元/组件测试 | Pinia store、API 客户端 CSRF/401、登录与团队视图 | `make test-web` |
| 浏览器 E2E | 真实栈登录、团队创建、邀请、成员管理、会话失效闭环 | `make e2e`（本地） |
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

## 浏览器 E2E（本地门禁）

`make e2e`（`infra/scripts/e2e.sh`）在完全一次性的栈上运行 Playwright，使用系统 Chrome（`channel: 'chrome'`，无需下载浏览器）：

- 仅连接一次性容器 `nexora-devtest-pg`（loopback `127.0.0.1:5433`），重建专用 `nexora_e2e` 数据库并在退出时删除；psql 通过 `docker exec` 在容器内执行，不依赖宿主机客户端。
- 不触碰运行中的开发栈（5432/8080/3000）、其他项目容器或任何卷。
- ROOT 密码每次运行随机生成，仅经环境变量注入测试进程，不落盘、不打印。
- API 监听 `127.0.0.1:8081`，Vite 监听 `127.0.0.1:5173`（可用 `E2E_WEB_PORT` 覆盖）；脚本退出时 trap 清理进程与数据库。

用例（`apps/web/e2e/phase02.spec.ts`）覆盖真实闭环：未认证重定向、ROOT 登录与创建团队、经真实管理 API 创建账号、邀请→接受→成员角色、普通成员无管理控件、角色晋升、移除后原会话立即失去团队访问（404）、最后 owner 退出保护（409）、多团队顶栏切换、会话失效回到登录页并提示。E2E 为本地门禁，不进入 CI 四个 job；CI 仍以单元/集成测试为准。

## 密钥扫描与契约门禁

`make secret-scan` 在本地和 CI 均先执行扫描器回归测试，再扫描 Git 跟踪文件和未忽略的新文件。需要 Python 3.9+，仅使用标准库。输出仅包含路径、行号和规则，不打印命中内容；被跟踪的真实环境文件直接拒绝且不读取。

PostgreSQL URL 与 `POSTGRES_PASSWORD` 赋值两条规则一致地把纯 shell 变量展开（如 `${PGPASSWORD}`，可带引号）视为运行时注入而非源码凭据，不予命中；E2E 脚本即用此方式从 gitignored 文件注入口令。任何字面口令、混合字面与变量（如 `pre${VAR}`、`${VAR}post`）或查询参数中的字面 `password`/`sslpassword` 仍判失败。回归测试同时覆盖“纯展开通过”与“字面/混合口令失败”正反例，确保该豁免不削弱对真实凭据的检测。

`infra/scripts/secret-scan-allowlist.json` 只豁免 `.env.example`、开发 Compose 和 CI 中已知虚拟 PostgreSQL 示例值，逐项绑定文件路径、规则及完整命中值的 SHA-256，并注明理由。没有整目录或整文件豁免；示例值改变、复制到其他路径或出现其他高风险凭据仍失败。新增例外必须单独审查，不能自动刷新白名单。

`make api-check` 在内存中重新生成 OpenAPI 类型并与提交文件比较，不写入源码；不一致时退出非零。本地 `make verify` 和 CI 都包含相同检查。

## 覆盖重点

优先覆盖高风险业务，而不是追求数字：

- 权限与跨团队 IDOR（Phase 02 已覆盖：真实 PostgreSQL 集成测试 + 浏览器 E2E）
- 状态机非法转换（Phase 03）
- 进度聚合与权重（Phase 03）
- 验收版本与重提（Phase 04）
- 贡献修订留痕（V1.1 / Phase 05）
- 金额舍入、合计上限、并发确认（V2.0 / Phase 06）
- 导出权限过滤（V2.0 / Phase 07）

## CI

`.github/workflows/ci.yml`：

1. Backend：格式、vet、迁移、`go test -race -tags=integration`。
2. Frontend：lint、format check、typecheck、test、build。
3. Secret scan：运行扫描器正反例回归测试和源码扫描，失败时返回非零。
4. Containers：`docker compose config`、构建 API 与 Web 镜像。

测试失败不得通过删除测试或降低类型检查绕过。

## Phase 03 验证

项目/任务专项：internal/projects/service_integration_test.go、internal/tasks 的领域与集成测试、internal/httpapi/projects_tasks_integration_test.go、TestPhase03FreshUpgradeAndRollback。沿用 testdb 护栏，真实 PG17 随机 schema，覆盖并发状态、结构/依赖环及成员退出/执行竞争。

make e2e 包含 Phase 02 的 11 项和 Phase 03 的 9 项串行真实 API/浏览器用例。覆盖三级任务、人员分配、参与者可见性、79% 上限、阻塞与忽略、权重历史、里程碑、归档、Team 切换，检查手机宽度不横向溢出。截图在 .cache/phase03-workspace-{desktop,dark,mobile}.png。Vitest 的 5 项 ProjectDetailView 竞态用例使用可控 Promise，不用 sleep。

本次一次性 PG 为 nexora-devtest-pg（loopback 5433、无其他项目挂载），随机口令只存忽略的 .cache/devtest-pg-password（0600），显式注入 TEST_DATABASE_URL，不加载开发 .env。用后只停止并删除本次创建的容器及其自有数据，不执行 prune。运行中的开发服务 /readyz 只读观察仍为 schema v1，未升级。

PR #3 首轮评审补充 TaskEditing.spec.ts 4 项组件回归：执行中的汇总任务可创建/移入子任务；失效协作者可显式清除（含无其他可选成员）。修复前 4 项失败，修复后全量 Vitest 66 项通过；真实 E2E 现为 20 项。
