# 01: ticket 状态与依赖图自动同步

**What to build:** 合并后的 ticket 自动从 `in-review` 变为 `done`，并重新生成 `.scratch/collector/ticket-dependency.html`；不再用 `@claude` 做机械记账。

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent

- [ ] `tools/ticketgraph` 解析 issues、计算依赖、生成确定性 HTML
- [ ] `tickets-check` / `tickets-guard` 进入 `make ci`，`feat/*` 分支不能写 `done`
- [ ] 合并后 workflow 用 GitHub App 建 bot PR 并自动合并
- [ ] PR 评论 `/accept <ticket>` 可在合并后手动重试
- [ ] 02b 回填为 `done` 并重新生成依赖图
- [ ] `docs/agents/issue-tracker.md` 与 `AGENTS.md` 记录状态机和触发方式
