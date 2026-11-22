# 01: ticket 状态与依赖图自动同步

**What to build:** 合并后的 ticket 自动从 `in-review` 变为 `done`，并重新生成 `.scratch/collector/ticket-dependency.html`；不再用 `@claude` 做机械记账。

**Blocked by:** None (can start immediately)

**Status:** in-review

- [x] `tools/ticketgraph` 解析 issues、计算依赖、生成确定性 HTML
- [x] `tickets-check` / `tickets-guard` 进入 `make ci`，`feat/*` 分支不能写 `done`
- [x] 合并后 workflow 用 GitHub App 建 bot PR 并自动合并
- [x] PR 评论 `/accept <ticket>` 可在合并后手动重试
- [x] 02b 回填为 `done` 并重新生成依赖图
- [x] `docs/agents/issue-tracker.md` 与 `AGENTS.md` 记录状态机和触发方式

## Comments

- 2026-11-23 用户确认：C1（bot PR + auto-merge）、GitHub App、开启 auto-merge、加 `done` 门禁；计划已确认，跳过独立评审，直接实现。
- 实现：`tools/ticketgraph`（render/check/set-status/complete/ready/resolve/guard）、`make tickets*`、`.githooks/pre-commit`、`ci.yml` 门禁、`tickets-sync.yml`、文档。
- 02b 已按合并事实回填 `done`，依赖图重新生成；本地 `make check`、`make test-race`、`golangci-lint`、`make spec-check`、`make spec-status` 全绿。
