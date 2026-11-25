# ADR-0011：code review 通过使用 head 绑定的记录

- 状态：Accepted
- 日期：2026-11-25
- 相关：ADR-0008（trigger Amendment）、ADR-0009、`AGENTS.md`、`docs/agents/issue-tracker.md`、#101

## 背景

`code-review` 标签只表示“实现评审进行中”，reviewer 移除它之后流程才允许 auto-merge。CI 不检查标签，带标签也能开 auto-merge；“人工评审通过”只靠纪律。本仓库 agent 与维护者共用账号，GitHub 不允许作者 approve 自己的 PR，原生 required review 不可用。required check 又绑定 commit：评论或标签变更不产生新 run，draft→ready 也不在默认 `pull_request` types 中。

## 决策

1. 非 draft、非豁免 PR 必须带一条 code review 通过记录：由 `github.repository_owner` 在 PR 会话评论或 review 正文中留单独一行 `/code-approved <head-sha>`。
2. `<head-sha>` 必须解析为**当前 PR head**；批准后的任何新 commit、amend、rebase 或 force-push 都使记录失效，需要重新评审。这一点与 plan approval 的祖先语义不同，是 code review 的故意选择。
3. `guard` 在 `code-review` 标签仍存在时失败；标签是等待标记，不是通过信号。
4. draft PR 跳过 code review 检查；`on.pull_request.types` 加入 `ready_for_review`，保证 draft→ready 时重新评估。
5. 记录复用 ADR-0009 的来源与 owner 过滤；它是审计证据，不是身份隔离，review 用 COMMENTED 即可。
6. 评论、review、标签变更不触发 workflow；reviewer 记录并移除标签后，由人或 agent Re-run all jobs。
7. `dependabot/*`、`probe/*` 豁免；`spec-delta/*` 与其他票一样需要记录。
8. required check 名称保持 `ci`、`guard`、`gitleaks`；不改分支保护。

## 后果

- 没有当前 head 的记录时 `guard` 失败，auto-merge 无法完成；误开 auto-merge 也不会越过红灯。
- 批准后修一个 typo 也要重新评审；这是“批准的是当前代码”的代价。
- 每次批准后需要一次手动 Re-run；不会 false-green，但流程多一步。
- draft→ready 会多跑一次全量 `ci`/`gitleaks`；每张票一次，可接受。
- 同账号限制仍在；需要身份隔离时应另开 issue 引入独立 reviewer 账号或 GitHub App。
