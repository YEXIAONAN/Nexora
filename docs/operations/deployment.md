# 部署说明

> **v0.1.0-alpha.2：待最终验证，尚未发布。** 本文件的 Compose 是开发配置，不是生产交付。本次版本准备不代表远程 Release、测试或 CI 已成功；历史阶段报告也不能替代当前版本证据。Docker 不可用时须明确未验证完整启动。生产部署、TLS/云环境与真人团队试用尚未验收；真人试用待真实用户参与，不阻碍内部 Alpha。发布说明见 [CHANGELOG](../../CHANGELOG.md)。

## 开发环境一键启动与平台边界

安装与账号上手见 [README](../../README.md)。`./start` 是 Bash 脚本，适用于 macOS/Linux；Windows 推荐 WSL。原生 Windows 可运行 Go 后端，但没有提供原生 PowerShell 一键启动脚本。PR #13 尚未在本地合并，真实 Windows CI 尚未运行，不作为 Alpha.2 已验证能力。

- 默认使用本地 Compose；未显式指定环境文件时，缺少 `.env` 会从 `.env.example` 创建。
- `NEXORA_ENV_FILE=/绝对路径/dev.env ./start` 使用指定文件；相对路径以仓库根目录为基准。显式指定的文件不存在时直接报错，不创建或加载默认 `.env`。
- `NEXORA_EXTERNAL_DATABASE=1` 跳过全部 Compose 操作，**仍迁移 `NEXORA_DATABASE_URL` 指定的库**。运行前确认主机、库名和备份，不把它当作只读连接检查。
- `NEXORA_API_PORT` 与 `NEXORA_HTTP_ADDR` 同时设置时端口必须一致；脚本同步 Vite API 目标，前端使用严格端口。
- 仅复用 `/readyz` 数据库/schema 检查通过的 API。`root_initialized` 为 `true` 才表示已初始化；`false` 时可交互式初始化；读取失败或值无效显示未知，不推定成功，也不设置默认密码。
- 可复用已有服务；`Ctrl+C` 仅停止本次启动的子进程，保留已有服务和数据库。

## 本地/单机 Docker Compose

```bash
cp .env.example .env
# 修改数据库密码与端口；不要使用示例密码
docker compose --profile full up -d --build
```

服务：

| 服务 | 说明 | 默认端口 |
| --- | --- | --- |
| postgres | PostgreSQL 17，数据卷 `postgres_data` | 127.0.0.1:5432 |
| migrate | 一次性迁移任务，成功后 API 启动 | — |
| api | Go API，非 root 用户 | 127.0.0.1:8080 |
| web | Nginx + 构建后的 SPA，代理 `/api`、`/healthz`、`/readyz` | 127.0.0.1:3000 |

启动顺序由 `depends_on` 条件保证：

```text
postgres healthy → migrate completed → api healthy → web
```

验证：

```bash
curl -fsS http://localhost:8080/healthz
curl -fsS http://localhost:8080/readyz
curl -fsS http://localhost:8080/api/v1/meta
curl -fsS http://localhost:3000/ >/dev/null
```

## ROOT 初始化与团队开通

全新安装完成迁移后，必须显式初始化 ROOT 账号；不存在默认密码，默认用户名为 `admin`：

```bash
# 本机开发
make bootstrap

# 容器环境（交互式终端，无回显读取并确认密码）
docker compose --profile full exec api nexora-bootstrap -username admin
```

- 重复引导会被安全拒绝（数据库部分唯一索引保证全库仅一个 ROOT）。
- 引导事件写入追加式审计 `root.bootstrap`。
- 自动化可向命令标准输入提供单行密码；仍须显式提供，命令不生成任何默认凭据。
- `GET /api/v1/meta` 的 `root_initialized` 字段供前端展示初始化状态。
- ROOT 恢复流程尚未实现；不得通过更改用户名或普通接口修改 `system_root`。

ROOT 负责平台用户管理和创建团队时指定 Owner，不因创建团队自动加入，也不绕过团队、项目或奖金授权。先创建独立 Owner 用户（开放注册或由「用户管理」创建），再由 Owner 邀请成员、Owner/Admin 创建项目并任命 lead，lead 管理项目成员与任务。奖金还需独立的金额负责人和核定人及显式批次授权；完整上手步骤见 README 第 4–9 节。

## 生产注意事项

1. **认证已合并，生产环境仍须单独验收**：必须部署 HTTPS、强凭据、反向代理和运行权限边界。开发 Compose 固定 `NEXORA_ENV=development`，不应直接当作生产配置。
2. **TLS**：在反向代理或负载均衡器终止 TLS；HSTS 仅在 `NEXORA_ENV=production` 时由 API 返回。
3. **密钥**：数据库密码、会话密钥通过部署平台注入，不写入镜像或仓库。
4. **迁移与角色**：生产禁止 `NEXORA_MIGRATE_ON_START=true`（配置校验会拒绝启动）；使用独立 migration owner 运行 `nexora-migrate -command up`，完成下文 v11 授权配置后再部署 API。API 只能使用普通 app 凭据，不复用迁移 owner。
5. **多副本**：迁移只由一个 Job 执行；API 进程无状态，但当前私有附件使用本地目录，不可据此宣称整个系统可直接水平扩容。
6. **数据库与恢复**：托管 PostgreSQL 建议开启自动备份与时间点恢复；验证数据库与附件的同恢复点备份，并在单独审核目标库执行恢复演练。
7. **日志**：容器 stdout JSON 日志接入采集系统；按 `request_id` 关联请求。
8. **健康检查**：`/healthz` 用于重启判断；`/readyz` 用于流量接入。不要把 `/healthz` 当作依赖检查。

本机 Go 默认监听 `127.0.0.1:8080`；Compose 的 API 在容器内显式监听 `:8080`，宿主机发布端口均限制到 loopback。生产应通过独立、经审查的配置显式注入 `NEXORA_ENV=production`、数据库连接、受控监听地址和真实凭据，配置数据库 TLS 与入口 HTTPS，禁止复用示例密码或自动迁移。当前没有提供或执行生产发布。

## 构建参数

API 镜像支持：

```bash
# COMMIT 由构建环境提供，必须是经核实的实际构建提交。
docker build -f infra/docker/api.Dockerfile \
  --build-arg VERSION=0.1.0-alpha.2 \
  --build-arg COMMIT="${COMMIT:?请提供实际构建提交}" \
  --build-arg BUILD_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ) \
  -t nexora-api:0.1.0-alpha.2 .
```

构建信息通过 `GET /api/v1/meta` 暴露，不包含敏感配置。API 默认版本与 Web 包版本均为 `0.1.0-alpha.2`；链接时注入的版本可覆盖 API 默认值，应核对实际构建参数。以上命令仅为构建示例，不表示镜像、标签或远程 Release 已发布。

若构建环境无法访问 `proxy.golang.org`，可覆盖模块代理（`go.sum` 仍负责校验内容）：

```bash
docker build -f infra/docker/api.Dockerfile \
  --build-arg VERSION=0.1.0-alpha.2 \
  --build-arg COMMIT="${COMMIT:?请提供实际构建提交}" \
  --build-arg BUILD_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ) \
  --build-arg GOPROXY=https://goproxy.cn,direct \
  -t nexora-api:0.1.0-alpha.2 .
```

Web 镜像构建参数 `VITE_API_BASE_URL` 默认为空，表示同源 `/api`。

## Alpha.2 升级与 schema 兼容

最终迁移链为 **v1–v11**；已发布 **v1–v8 保持原始字节不变**。v9 为 `00009_task_progress_reports.sql`，v10 为 `00010_bonus_foundation.sql`（原未发布的奖金 v9 重编号），v11 为 `00011_bonus_workflow.sql`。不要修改历史迁移或手改 `goose_db_version` 掩盖 schema 不一致；若曾运行未发布旧编号迁移，应先核对真实结构与记录，单独审核迁移方案，不自动清库。

API 启动与 `/readyz` 仅接受 **v9–v11**；完整奖金能力要求 v11。各版本边界如下：

| 数据库版本 | 当前 API 启动 / `/readyz` | 奖金能力 |
| --- | --- | --- |
| v8 及更早版本 | 拒绝，必须先升级 | 不可用 |
| v9 | 接受 | 不挂载奖金路由，404 |
| v10 | 接受 | 不挂载奖金路由，404；有基础表不等于开放只读 API |
| v11 | 接受（仍需通过数据库与角色检查） | 全部奖金读写、独立核定、发布和模拟发放 |
| v12+ | 拒绝 | 不支持，不应强行降级 |

升级顺序：

1. 确认目标主机、数据库、schema 和真实迁移状态，审核维护窗口并暂停写入。备份并验证数据库（`make backup && make verify-backup` 仅适用于已核实的脚本目标），同时备份私有附件；数据库备份验证不能代替附件备份。
2. 使用独立 migration owner 显式迁移至 v11；v9/v10 的启动兼容仅用于过渡，不能当作 Alpha.2 奖金验收环境。
3. 由 migration owner 按 [应用数据库角色](application-database-role.md)执行通用授权，再在相同、已审核的 `search_path` 会话执行 v11 配置：

   ```sql
   -- app_role 是 psql 变量，须预先设为经审核的普通应用角色名。
   SELECT bonus_configure_application_role(:'app_role'::name);
   ```

   该步骤配置奖金表读取及工作流入口函数调用，并收回直接奖金写入和辅助函数权限；迁移不会猜测应用角色。**configure 之后禁止批量 `GRANT EXECUTE ON ALL FUNCTIONS`**。如重跑通用授权，必须再由 migration owner 执行 configure，并检查最终有效权限（包含继承及 PUBLIC）。v9/v10 尚无该函数，不执行此步骤。
4. 使用普通 app 凭据部署/重启 API，检查 `/readyz`、`/api/v1/meta` 与日志，确认 schema v11、实际版本及奖金路由。生产角色检查不修改 ACL，失败时应审核授权，而非让 API 改用 owner。
5. 在隔离测试环境补齐当前版本升级、成员/权限、独立核定、发布、本人结果和模拟发放的验证证据，再完成发布记录。不得在真实数据库制造测试奖金或伪造已通过的结果。

数据库 app 账号属于可信后端；这些授权和检查不等同于 RLS，也不保证数据库凭据完全泄露后仍能隔离身份。不可将 app 凭据交给终端用户。

## 回退与恢复边界

**v11 的 Down 明确拒绝执行**，因为回退会把已有奖金事实重新置于旧语义；`-allow-destructive` 也不能将其变成安全可逆迁移。不提供或承诺“一键回滚”，不要通过删表、清库、关闭约束或改版本记录强行降级。

需回退时，保留原库和附件，使用升级前已验证的同恢复点备份，恢复到**单独审核的目标库及私有附件目录**；核对 schema、业务/贡献/奖金/审计记录与附件完整性，再验证匹配该 schema 的应用版本并审批切换。恢复会丢失备份时间点之后的变更，须单独核对处置；不得覆盖或清理未知数据、正在服务的数据库或来源不明的数据卷。

## 附件与历史恢复资料

生产须配置私有绝对路径 `NEXORA_ATTACHMENT_DIR` 并赋予 API 用户写权限。Docker 镜像使用 `/app/.data/attachments`，Compose 使用独立 `attachment_data` 卷。数据库和文件须同恢复点备份；单机目录方案不支持直接横向扩容，不提供病毒扫描。禁止将附件目录挂到 Nginx 静态站点路径。

历史 Phase 04/05 的 schema v6/v7 说明、Phase 06 M2/M3 的 v10 只读说明不代表当前部署要求。联合恢复的历史命令与边界见 [备份与恢复](backup-restore.md#phase-05-m6-数据库与附件联合恢复演练)，须在当前 v11 上重新验证；历史演练通过不等于 Alpha.2 已验收，更不等于生产部署通过。
