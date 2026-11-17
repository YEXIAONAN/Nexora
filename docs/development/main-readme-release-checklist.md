# main README 发布同步检查清单

本次 Phase 04 只修改功能分支并向 develop 提 PR，不直接修改 main。此清单供以后获准发布时使用；不能把特性分支测试通过写成已发布。

- [ ] 核对远端 main / develop HEAD、Phase 04 PR 合并事实及发布提交的全部 CI。
- [ ] 通过发布 PR 将准确的 Nexora 标题、V1.0 方案 B、V1.1 / V2.0 边界同步到 main。
- [ ] 依据最终合并 SHA 更新 Phase 01–04 状态和功能矩阵，移除过期 PR 待合并说明。
- [ ] 删除 main 中普通 nexora 开发库的集成测试示例，采用专用 test-db.sh 和现有 testdb 护栏；不能放宽库名、loopback 或 disposable 检查。
- [ ] 在空环境验证安装、迁移、ROOT 引导、API / Web 启动和健康检查命令。
- [ ] 验证 make verify、独立库 race / integration、make e2e、OpenAPI、扫描与镜像构建。
- [ ] 核对 schema v6、生产私有绝对目录、Docker 附件卷、数据库与文件同恢复点备份及回退拒绝规则。
- [ ] 明确运行中的旧开发栈可能仍用旧 schema；发布 PR 不自动授权升级或重启它。
- [ ] 核对权限矩阵、自审禁止、V2 独立审核、状态与进度说明、单实例存储边界。
- [ ] 所有截图均来自真实验收环境，未交付的贡献核算、奖金与导出仍标记为后续范围。
- [ ] 获得发布批准后才合并发布 PR；保留审核、CI、备份及迁移验收记录。
