# 备份与恢复

Git 仓库与数据库备份是两条独立链路。代码推送成功不代表数据可恢复。

## 备份

```bash
make backup
```

脚本行为（`infra/scripts/backup.sh`）：

1. 确认 `postgres` 容器正在运行。
2. 使用 `pg_dump --format=custom --no-owner --no-privileges` 输出到 `backups/`。
3. 用 `pg_restore --list` 验证归档可读；失败则删除文件并返回非零。
4. 生成 SHA-256 校验文件。
5. 按 `NEXORA_BACKUP_RETENTION_DAYS`（默认 14 天）清理过期文件。

输出：

```text
backups/nexora_nexora_20261029T120000Z.dump
backups/nexora_nexora_20261029T120000Z.dump.sha256
```

## 恢复验证

```bash
make verify-backup
# 或指定文件
infra/scripts/verify-backup.sh backups/nexora_nexora_20261029T120000Z.dump
```

验证流程：

1. 创建临时数据库 `nexora_restore_check_<timestamp>`。
2. 恢复归档。
3. 查询 `system_settings.installation_id` 与 `goose_db_version` 最高版本。
4. 断言基线表存在且安装 ID 非空。
5. 删除临时数据库。

只有上述步骤全部成功，备份才算通过验证。

## 恢复到指定数据库

```bash
infra/scripts/restore.sh backups/<file>.dump --target-db nexora_restore_manual
```

默认拒绝覆盖主数据库。确需覆盖时必须：

```bash
NEXORA_ALLOW_PRIMARY_RESTORE=1 \
  infra/scripts/restore.sh backups/<file>.dump --target-db nexora --drop-existing --yes
```

执行前先完成一次已验证备份，并停止 API 写入。

## 自动备份

crontab 示例（每天 03:15 UTC）：

```cron
15 3 * * * cd /srv/nexora && NEXORA_BACKUP_RETENTION_DAYS=30 ./infra/scripts/backup.sh >> /var/log/nexora-backup.log 2>&1
```

建议：

- 备份目录挂载到独立磁盘或同步到对象存储（启用服务端加密与版本控制）。
- 每周至少执行一次 `verify-backup.sh`，并记录结果。
- 监控备份任务退出码与最新文件时间；只有文件生成不算成功。
- 定期演练恢复到临时数据库，核对关键表行数与业务抽样。

## 保留与恢复目标

当前为单机 Compose 开发环境，未定义正式 RPO/RTO。生产部署前需明确：

- 备份频率与保留周期（建议每日全量 + 托管数据库 PITR）。
- 允许的数据丢失窗口（RPO）。
- 恢复时间目标（RTO）。
- 异地副本与加密密钥管理。

这些指标必须在真实环境演练后写入本节，而不是预先声称。


## Phase 04 附件与数据库一致恢复

现有 backup.sh / verify-backup.sh 只备份/恢复 PostgreSQL，不能视为附件备份。运行在 schema v6 后，先暂停所有 API 写入及回收进程，记录维护窗口和 schema 版本，在同一窗口执行数据库备份并复制 NEXORA_ATTACHMENT_DIR（Docker 为 attachment_data 卷）到私有备份介质，保留文件相对键、权限和校验清单。两者验证成功后再恢复写入；数据库或文件任一失败则该恢复点无效。

恢复时保持 API 停止，恢复匹配数据库和私有目录，检查 bound 元数据的每个 storage_key、byte_size 与 SHA-256；通过下载授权测试和缺失文件检查后再启动 API。缺失/损坏文件应告警并从匹配备份恢复，不能删除审核、版本或改为假链接。启动后回收只处理暂存、过期和超出 24 小时宽限的无元数据文件，不删除 bound 历史。

v6 down 仅适用于无成果根且无附件记录的空业务升级验证；存在证据时会拒绝，不可禁用触发器绕过。旧应用回退必须恢复事先验证的匹配备份，不能直接删除 Phase 04 证据表。
