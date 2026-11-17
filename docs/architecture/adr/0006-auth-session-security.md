# ADR 0006：认证与会话安全

- 状态：已接受（Accepted）
- 日期：2026-11-17
- 批准依据：产品负责人 2026-11-17 选择方案 B，Phase 02 开发身份认证、会话与 ROOT 安全引导。

## 背景

Phase 02 引入用户身份与会话。需要在不引入外部身份服务的前提下，提供可审计、可撤销、抗枚举与抗 CSRF 的认证机制，并保证 ROOT 引导无默认凭据。

## 决策

### 密码哈希

- 算法：Argon2id（`golang.org/x/crypto/argon2`），PHC 字符串格式存储（`$argon2id$v=19$m=65536,t=3,p=4$<salt>$<hash>`）。
- 参数：`time=3`、`memory=64 MiB`、`threads=4`、`keyLen=32`、`saltLen=16`（RFC 9106 第二推荐选项）。参数以本机基准测试实测耗时为准记录于 Phase 02 验收报告，不盲目设置极高成本。
- 盐：每用户独立，`crypto/rand` 生成。
- 验证：按存储 PHC 中的参数重算，`subtle.ConstantTimeCompare` 比较。
- 用户不存在时仍对预生成哑哈希执行一次验证，抵御用户名枚举时序侧信道；登录失败统一返回 401 `invalid_credentials` 语义（`unauthorized` 码），不区分用户不存在与密码错误。
- 密码策略：长度 10–128 字节（Argon2 输入上限防 DoS）、非空白、不得与用户名相同（忽略大小写）。策略在引导、账号创建处统一校验。

### 会话

- 令牌：32 字节 `crypto/rand`，base64 RawURL 编码，经 `nexora_session` Cookie 下发；数据库仅存 SHA-256 摘要（64 位 hex，唯一约束）。
- 会话记录：`sessions` 表含 `user_id`、`token_hash`、`expires_at`、`revoked_at`、`user_agent`（截断 256）。绝对过期（默认 12h，`NEXORA_SESSION_TTL` 可配），不做滑动续期。
- 撤销：退出登录撤销当前会话；ROOT/平台管理员可撤销指定用户全部会话；撤销与过期在服务端每次请求判定。
- 防会话固定：登录成功总是签发全新会话令牌。
- 团队权限不缓存在会话中：成员关系每次请求实时查询，移除成员即时失效。

### Cookie 策略（环境派生，无降级开关）

| 环境 | HttpOnly | Secure | SameSite |
| --- | --- | --- | --- |
| production | 是 | **是（强制）** | Lax |
| development / test | 是 | 否（本地 HTTP） | Lax |

- Secure 由 `NEXORA_ENV` 派生，不提供环境变量关闭；生产无法带入不安全开发配置。
- Cookie `Path=/`，host-only（不设 Domain），`Max-Age` 与会话 TTL 一致；退出/失效时下发删除 Cookie。
- 长期认证令牌不进 localStorage；前端仅持久化“当前 Team 偏好”这类非凭据数据。

### CSRF 防护

- 组合策略：SameSite=Lax Cookie + 变更类请求（POST/PUT/PATCH/DELETE）必须携带自定义头 `X-Nexora-CSRF: 1`；API 不启用 CORS（无 `Access-Control-Allow-Origin`），跨站脚本无法在预检失败后携带自定义头。
- 所有业务变更均为非 GET；前端 API 客户端对变更请求统一附加该头。
- 缺少该头的变更请求返回 403 `forbidden`（CSRF 校验失败），并有集成测试覆盖。

### 登录限流

- 进程内滑动窗口（模块化单体单实例，ADR 0001），键 = 客户端 IP + 用户名。
- 默认窗口 15 分钟、最多 10 次失败（`NEXORA_LOGIN_WINDOW`、`NEXORA_LOGIN_MAX_FAILURES` 可配，启动校验）。
- 超限返回 429 `rate_limited`；登录成功清除该键计数。窗口内超限后即使密码正确也拒绝。

### ROOT 引导

- 独立命令 `nexora-bootstrap`（`cmd/bootstrap`），显式运行，不随服务启动执行。
- 交互式：TTY 下无回显读取密码并二次确认；非 TTY（自动化）从 stdin 读取单行密码，仍须操作者显式提供，无默认值。
- 用户名默认 `waiting`，可改；校验用户名格式与密码策略。
- 唯一性：`users_single_system_root` 部分唯一索引；并发引导只有一个成功，其余收到“已初始化”安全拒绝；重复引导拒绝。
- 引导成功写入 `root.bootstrap` 审计事件（同事务）；任何输出与日志不包含密码。
- 普通 HTTP 接口不能创建或修改 ROOT；恢复流程不在本阶段实现，未来必须专用受控程序，不得经用户名变更实现。

### 账号创建与邀请

- 不开放公众注册。ROOT/平台管理员经 `POST /api/v1/admin/users` 创建普通账号（`system_root` 恒为 false，接口不接受该字段）。
- 团队邀请仅面向已存在账号，具备授权（owner/admin）、有效期（默认 72h，1–168h）、接受人身份约束（仅被邀请者本人可接受/拒绝）；详见 ADR 0004 实施细则。

## 依赖增补

- `golang.org/x/crypto`（argon2）
- `golang.org/x/term`（引导命令无回显读密码）

两者加入 ADR 0002 依赖集，版本锁定于 `go.mod`/`go.sum`。

## 后果

- 配置新增 `NEXORA_SESSION_TTL`、`NEXORA_LOGIN_MAX_FAILURES`、`NEXORA_LOGIN_WINDOW`，启动期校验。
- `/api/v1/meta` 增加 `root_initialized`（boolean/null；查询失败为 null，不误导前端）。
- 限流为进程内状态：多副本部署时各副本独立计数（当前单实例架构下可接受；水平扩展时需重新评估，记录为已知限制）。
- 密码修改/重置自助流程不在 Phase 02 范围，记录为已知限制与后续阶段候选。