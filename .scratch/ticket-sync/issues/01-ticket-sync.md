# 01: ticket 状态与依赖图自动同步

**What to build:** 合并后自动写 `**Merged:** #N`；`in-review` / `done` 由 `**PR:**` / `**Merged:**` 派生；依赖图由 `make tickets` 在本地生成，不入库；不再用 `@claude` 做机械记账。

**Blocked by:** None (can start immediately)

**Status:** ready-for-human

- [x] `tools/ticketgraph` 解析 issues、计算依赖、生成本地 HTML
- [x] `tickets-guard` 进入 `make ci`，`feat/*` 分支不能写 `Merged:`；`tickets-check`/pre-commit 已移除
- [x] 合并后 workflow 用 GitHub App 建 bot PR 并自动合并
- [x] PR 评论 `/accept <ticket>` 可在合并后手动重试
- [x] 02b 等 9 个已完成 ticket 回填为 `ready-for-agent` + `**Merged:** historical`
- [x] `docs/agents/issue-tracker.md` 与 `AGENTS.md` 记录 5 个 triage 标签、PR/Merged 字段和触发方式；HTML 改为本地生成物

## Comments

- 2026-11-23 用户确认：C1（bot PR + auto-merge）、GitHub App、开启 auto-merge、加 `done` 门禁；计划已确认，跳过独立评审，直接实现。
- 2026-11-23 v2：用户选择“只去掉 HTML 冲突”；`ticket-dependency.html` 移出版本库并加入 `.gitignore`，`tickets-check` 与 `.githooks/pre-commit` 删除；状态模型保持不变。
- 2026-11-23 v3：状态收敛为 mattpocock 的 5 个 triage 标签；生命周期改用 `**PR:**` / `**Merged:**` 字段，`in-review` / `done` 改为派生；9 个已完成 ticket 回填 `Merged: historical`。
- 实现：`tools/ticketgraph`（render/set-status/set-pr/complete/ready/resolve/guard）、`make tickets*`、`ci.yml` 门禁、`tickets-sync.yml`、文档。
