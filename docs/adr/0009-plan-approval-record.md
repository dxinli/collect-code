# ADR-0009：计划通过使用可校验的评论记录

- 状态：Accepted
- 日期：2026-11-25
- 相关：ADR-0007、ADR-0008、`AGENTS.md`、`docs/agents/issue-tracker.md`、#98

## 背景

`plan-review` 流程原先以“移除标签”作为计划通过信号。标签移除后不留记录，无法证明 PR 经过计划评审；`guard` 只在标签存在时检查文件范围，不带标签直接推代码会完全放行。与此同时，标签变更不触发 workflow（ADR-0008），门禁强度依赖人是否记得 Re-run。

GitHub 的 PR review approval 需要 reviewer 与作者不同；本仓库的 agent 与维护者共用同一账号，且分支保护 required reviews 为 0，因此不能把 review approval 作为通过信号。

## 决策

1. 代码文件变更的 PR 必须带一条计划通过记录：由 `github.repository_owner` 在 PR 上评论，正文单独一行 `/plan-approved <sha>`。
2. `<sha>` 必须是 PR head 的祖先，且 `git diff <base>...<sha>` 只包含 plan allowlist（`docs/`、`.scratch/`、`AGENTS.md`、`CLAUDE.md`、`GLOSSARY.md`）。记录点在代码之后、或被 force-push/rebase 移出历史时，记录无效。
3. `plan-review` 标签保留为等待标记；ADR-0007 的文件范围检查不变。标签移除不替代记录。
4. 豁免 `spec-delta/*`、`dependabot/*`、`probe/*` 分支；probe 分支是测试用豁免，不是安全边界。
5. 记录是审计证据，不是身份隔离。同一账号仍可写记录；owner 过滤只排除外部评论者。
6. 标签或评论变更不触发 workflow；记录后需 Re-run all jobs，或由下一次代码 push 触发。PR 正文的计划被编辑无法机器识别；material change 由 reviewer 重新评论。

## 后果

- 计划通过从“一次性标签动作”变成“时间线上可查询的 SHA 记录”，可审计、可重复校验。
- 代码改动在无记录时被 `guard` 拒绝；记录点含代码时同样拒绝，避免事后批准。
- 多一个操作步骤：reviewer 评论记录并 Re-run；忘记 Re-run 不会 false-green，只会看到旧的失败状态。
- 同账号限制仍未解决；需要身份隔离时应另开 issue 引入独立 reviewer 账号或 GitHub App。
