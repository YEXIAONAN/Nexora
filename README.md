# Nexora

![Nexora](https://socialify.git.ci/YEXIAONAN/Nexora/image?description=1&font=Rokkitt&language=1&name=1&owner=1&pattern=Circuit+Board&theme=Light)

**Nexora — 团队协作与项目交付管理平台**

Nexora 是一个面向团队协作和项目交付的管理平台，围绕团队、项目、任务与贡献建立完整的协作流程。

从任务分配、进度上报、成果验收，到贡献核算与奖金管理，Nexora 希望让团队工作过程可追踪、交付结果可验证、成员贡献可核算。

> 不只是管理任务，更是让每一份贡献都有据可循。

## 核心功能

### 团队与项目管理

- 多团队协作与数据隔离
- 团队成员邀请及角色管理
- 项目创建、负责人任命与成员管理
- 团队与项目级权限控制

### 任务与交付管理

- 最多三级任务结构
- 任务负责人、协作者及优先级管理
- 工作进展上报与历史记录
- 成果提交、版本管理与独立验收
- 项目里程碑与进度历史

### 贡献与奖金管理

- 基于正式交付结果建立贡献案例
- 独立贡献核定流程
- 人工预算与奖金方案编制
- 独立核定、正式发布与版本修订
- 模拟发放及完整操作审计

### 权限与安全

- 平台管理员、团队角色与项目权限分离
- 基于团队和项目成员关系的业务授权
- Argon2id 密码哈希
- HttpOnly Cookie 会话管理
- CSRF 防护、会话撤销与审计记录

## 技术栈

| 模块 | 技术 |
| --- | --- |
| 后端 | Go、Chi、pgx |
| 数据库 | PostgreSQL 17 |
| 数据库迁移 | Goose |
| 前端 | Vue 3、TypeScript |
| 构建工具 | Vite、pnpm |
| 状态管理 | Pinia |
| 路由 | Vue Router |
| UI | Tailwind CSS |
| API 契约 | OpenAPI 3.1 |
| 测试 | Go Testing、Vitest、Playwright |

## 项目结构

```text
Nexora/
├── apps/
│   ├── api/             # Go 后端 API
│   └── web/             # Vue 前端应用
├── infra/               # 容器、脚本及运维工具
├── docs/
│   ├── product/         # 产品需求与业务文档
│   ├── api/             # API 契约
│   ├── development/     # 开发者文档
│   ├── operations/      # 部署与运维文档
│   └── user-guide/      # 用户使用手册
├── Makefile
├── start
└── README.md
```

## 快速开始

### 环境要求

| 依赖 | 版本 |
| --- | --- |
| Docker + Docker Compose | Compose v2 |
| Go | 1.26+ |
| Node.js | 22+ |
| pnpm | 10.x |

支持 macOS / Linux。Windows 推荐通过 WSL 运行启动脚本。

### 安装与启动

克隆项目：

```bash
git clone https://github.com/YEXIAONAN/Nexora.git
cd Nexora
```

启用 pnpm：

```bash
corepack enable
corepack prepare pnpm@10.34.5 --activate
```

配置环境：

```bash
cp .env.example .env
```

启动服务：

```bash
./start
```

启动脚本会自动完成数据库启动、数据库迁移、后端 API 启动、ROOT 状态检查以及前端启动。

首次启动时，根据终端提示完成 ROOT 管理员初始化。

### 访问服务

| 服务 | 地址 |
| --- | --- |
| Web 前端 | http://127.0.0.1:3000 |
| API | http://127.0.0.1:8080 |
| 存活检查 | http://127.0.0.1:8080/healthz |
| 就绪检查 | http://127.0.0.1:8080/readyz |

默认开发环境开放用户注册。

按 `Ctrl+C` 停止本次启动的 API 和前端。PostgreSQL 容器及数据会保留，再次执行 `./start` 即可继续使用。

更多环境配置、外部数据库和手动启动方式，请阅读运维文档。

## 权限设计

Nexora 将平台管理、团队管理与项目业务权限分离。

- **ROOT：** 平台级管理员，负责平台用户管理。
- **Owner / Admin：** 团队级管理角色，负责团队成员与团队资源。
- **Project Lead：** 项目负责人，负责项目成员、任务与交付管理。
- **Member：** 普通成员，根据授权参与项目协作。

平台管理员身份不会自动获得团队、项目或奖金业务权限。所有关键业务操作均由后端执行权限校验。

## 文档

- [产品需求文档](docs/product/product-requirements.md)
- [API 契约](docs/api/openapi.yaml)
- [开发文档](docs/development/)
- [部署说明](docs/operations/deployment.md)
- [数据库角色配置](docs/operations/application-database-role.md)
- [版本更新记录](CHANGELOG.md)

## 开发与验证

常用开发命令：

```bash
make test       # 单元测试
make lint       # 代码规范检查
make build      # 前后端构建
make verify     # 完整本地验证
make e2e        # Playwright E2E 测试
```

项目使用 Go 单元测试、前端 Vitest 和 Playwright 进行质量验证。

具体测试结果、CI 状态与版本变更记录以 [CHANGELOG.md](CHANGELOG.md) 及对应 CI 工作流为准。

## 项目状态

**当前版本：Alpha.4**

项目持续迭代中。当前版本已覆盖团队管理、项目协作、任务交付、贡献核算及奖金管理等核心业务流程。

生产部署前，请先阅读运维文档，完成数据库角色配置、备份恢复验证及必要的安全配置。

---

**Nexora**

让协作有序，让交付有据，让贡献有价值。
