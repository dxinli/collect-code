# 01: ticket 状态与依赖图自动同步

**What to build:** 合并后的 ticket 自动从 `in-review` 变为 `done`；依赖图由 `make tickets` 在本地生成，不入库；不再用 `@claude` 做机械记账。

**Blocked by:** None (can start immediately)

**Status:** in-review

- [x] `tools/ticketgraph` 解析 issues、计算依赖、生成本地 HTML
- [x] `tickets-guard` 进入 `make ci`，`feat/*` 分支不能写 `done`；`tickets-check`/pre-commit 已移除
- [x] 合并后 workflow 用 GitHub App 建 bot PR 并自动合并
- [x] PR 评论 `/accept <ticket>` 可在合并后手动重试
- [x] 02b 回填为 `done`
- [x] `docs/agents/issue-tracker.md` 与 `AGENTS.md` 记录状态机和触发方式；HTML 改为本地生成物

## Comments

- 2026-11-23 用户确认：C1（bot PR + auto-merge）、GitHub App、开启 auto-merge、加 `done` 门禁；计划已确认，跳过独立评审，直接实现。
- 2026-11-23 v2：用户选择“只去掉 HTML 冲突”；`ticket-dependency.html` 移出版本库并加入 `.gitignore`，`tickets-check` 与 `.githooks/pre-commit` 删除；状态模型保持不变。
- 实现：`tools/ticketgraph`（render/check/set-status/complete/ready/resolve/guard）、`make tickets*`、`ci.yml` 门禁、`tickets-sync.yml`、文档。
