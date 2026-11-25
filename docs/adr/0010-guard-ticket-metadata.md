# ADR-0010：guard 覆盖所有非豁免 PR 的票元数据

- 状态：Accepted
- 日期：2026-11-25
- 相关：ADR-0007（决策 3 扩展）、ADR-0009、`AGENTS.md`、#99

## 背景

ADR-0007 决策 3 只要求 `feat/*`、`hotfix/*` 的 PR 带 `Closes #<issue>`；普通票走 `chore/*`、`docs/*`、`fix/*` 时完全跳过。PR 模板的 `Spec-Version`、`Spec-Impact`、`REQ` 只写在模板里，没有校验；`Closes` 也可以指向 anchor #71、已关闭 issue 或另一个 PR。

## 决策

1. 所有非豁免 PR 必须有且只有一行 `Closes #<n>`；其他 GitHub 关闭关键字（`Fixes`、`Resolves` 等）与第二条关闭引用同样拒绝。代码围栏内的示例不计入。
2. 目标必须是 open 的 issue、不是 PR、不是 #71，且正文含真实的 `**Ticket:** <id>`。
3. `Spec-Version` 必须等于 `docs/spec/VERSION`。
4. `Spec-Impact` ∈ {none, patch, minor, major}；改了 `docs/spec/` 或 `docs/contracts/` 时不得为 `none`。
5. `REQ` 非空；值为 `none`，或 `REQ-*` / 合同锚点列表（允许范围写法，容忍反引号与常见分隔符）。
6. `dependabot/*`、`probe/*` 全免；`spec-delta/*` 免 `Closes`/issue 校验，但仍校验 `Spec-Version`、`Spec-Impact`、`REQ` 与规范包/impact 一致性。
7. 移除 plan-only 的 `Closes` 例外：计划阶段的票 PR 从创建起就带 `Closes #N`。
8. required check 名称保持 `ci`、`guard`、`gitleaks`。

## 后果

- `chore/*`、`docs/*`、`fix/*` 的票 PR 也受 `Closes` 约束；没有对应 ticket issue 的 PR 失败。
- `spec-delta/*` 仍校验 `Spec-*` 与 `REQ`；`dependabot/*`、`probe/*` 全免。
- ADR-0007 决策 3 的覆盖范围被扩展，决策 2（plan-review 文件范围）不变。
- `REQ` 只校验格式，不校验锚点是否真实存在；PR 正文不在 `spec-check`/`spec-status` 的扫描范围内。
