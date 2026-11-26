# ADR-0011：code review 通过使用 head 绑定的记录

- 状态：Accepted
- 日期：2026-11-25
- 相关：ADR-0008（trigger Amendment）、ADR-0009、`AGENTS.md`、`docs/agents/issue-tracker.md`、#101

## 背景

`code-review` 标签只表示“实现评审进行中”，reviewer 移除它之后流程才允许 auto-merge。CI 不检查标签，带标签也能开 auto-merge；“人工评审通过”只靠纪律。本仓库 agent 与维护者共用账号，GitHub 不允许作者 approve 自己的 PR，原生 required review 不可用。required check 又绑定 commit：评论或标签变更不产生新 run，draft→ready 也不在默认 `pull_request` types 中。

## 决策

1. 非 draft、非豁免 PR 必须带一条 code review 通过记录：由 `github.repository_owner` 在 PR 会话评论或 **COMMENTED/APPROVED** review 正文中留单独一行 `/code-approved <head-sha>`；`DISMISSED`/`CHANGES_REQUESTED` 不算。
2. `<head-sha>` 必须解析为本次 workflow run 的 head，且该 head 必须等于当前 PR head；批准后的任何新 commit、amend、rebase 或 force-push 都使记录失效，需要重新评审。这一点与 plan approval 的祖先语义不同，是 code review 的故意选择。
3. `guard` 在 `code-review` 标签仍存在时失败；标签是等待标记，不是通过信号。
4. draft PR **失败**，不跳过：draft 上的绿色 check 会在 `ready_for_review` 新 run 注册前形成 false-green 窗口。`on.pull_request.types` 加入 `ready_for_review`，保证 draft→ready 时重新评估。
5. 记录复用 ADR-0009 的来源与 owner 过滤；它是审计证据，不是身份隔离，review 用 COMMENTED 即可。
6. 评论、review、标签变更不触发 workflow；reviewer 记录并移除标签后，由人或 agent Re-run all jobs。批准后再加回 `code-review` 不会自动使绿色 check 变红；需要时手动 Re-run。
7. `dependabot/*`、`probe/*` 豁免；`spec-delta/*` 与其他票一样需要记录。
8. required check 名称保持 `ci`、`guard`、`gitleaks`；不改分支保护。

## 备选方案

- **GitHub 原生 required review approval**：要求 reviewer 与作者不同；本仓库单账号，作者无法 approve 自己的 PR，不可行。
- **GitHub merge queue / `merge_group`**：能在合并时重新评估，但需要启用 merge queue 并迁移流程；本仓库 PR 量与分支模型不需要，且超出本票范围。
- **`pull_request_review` / `labeled` / `unlabeled` 触发自动重跑**：能消除手动 Re-run，但会重现 #89 的 run 膨胀；本票选择文档化手动 Re-run。
- **第二个 reviewer 账号或 GitHub App**：能提供身份隔离，但需要额外账号与凭据；本票只做审计记录，把身份隔离留作后续 issue。

## 后果

- 没有当前 head 的记录时 `guard` 失败，auto-merge 无法完成；误开 auto-merge 也不会越过红灯。
- draft PR 的 `guard` 为红；这是预期，避免 draft 绿色 check 在 ready 转换时形成 false-green。
- 批准后修一个 typo 也要重新评审；这是“批准的是当前代码”的代价。
- 每次批准后需要一次手动 Re-run；不会 false-green，但流程多一步。
- 批准后加回 `code-review` 标签不会自动使绿色 check 变红；需要时手动 Re-run。
- draft→ready 会多跑一次全量 `ci`/`gitleaks`；每张票一次，可接受。
- 同账号限制仍在；需要身份隔离时应另开 issue 引入独立 reviewer 账号或 GitHub App。

## Amendment（2026-11-26，ADR-0012）

标签移除动作改由 agent 代管（见 ADR-0012 决策 2）：reviewer 只留 `/code-approved <head-sha>` 记录，agent 据记录移除 `code-review` 并 Re-run。本文背景与决策 6 中“reviewer 移除标签”的表述按历史语境保留 (historical)；head 绑定语义与 `guard` 校验不变。
