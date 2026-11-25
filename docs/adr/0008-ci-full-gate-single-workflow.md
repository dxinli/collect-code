# ADR-0008：CI 采用全量门禁，单 workflow 三 job

- 状态：Accepted
- 日期：2026-11-25
- 相关：ADR-0007、AGENTS.md、`docs/agents/ci.md`、`.github/workflows/ci.yml`

## 背景

#87 引入自研路径分类器，#89 换成标准 changed-path action，#91 又尝试收口 plan-guard 与 spec-status。每一步都暴露下一层路径策略：CI 门控、plan-review 范围与 spec-delta 范围各有一套集合；标签事件又让 plan-guard 重复起 run。

全量门禁的热缓存实测约 28s；路径过滤只能省 docs-only 约 30s，却需要三套路径集合、额外 action 与标签触发。用户决定重做为全量门禁，接受 docs-only 也跑全量。

## 决策

1. `.github/workflows/ci.yml` 是唯一的 PR workflow；jobs 为 `ci`、`guard`、`gitleaks`；required checks 仍为 `ci`、`guard`、`gitleaks`。
2. 不做路径过滤；每次 PR/push 全量运行 `make check-go`、lint、`make check-spec`。docs-only 也跑全量。
3. `.github/workflows/plan-guard.yml` 删除；plan-review 文件范围与 `Closes` 检查迁入 `ci.yml` 的 `guard` job。ADR-0007 的 allowlist（`docs/`、`.scratch/`、`AGENTS.md`、`CLAUDE.md`、`GLOSSARY.md`）与 Closes 规则不变。
4. 所有 workflow 去掉 `labeled`/`unlabeled` 触发；标签只读。标签变更不触发 run；`guard` 在下一次运行时用 `gh pr view` 读取当前标签。
5. spec-delta 只允许 `docs/spec/**` 与 `docs/contracts/**`；bootstrap（base 无 spec 包）同样只允许 spec 包路径。

## 后果

- docs-only 从按需的约 6s 回到全量约 28s（热缓存）；接受。
- 把 `plan-review` 加到一个已经包含代码的 PR 时，`guard` 不会自动重跑；必须 Re-run all jobs 才能用当前标签检查该 diff。ADR-0007 的「不再依赖人肉 review」在本场景下减弱为：push 时自动，标签添加时需要一次手动重跑。
- required checks、allowlist、Closes 规则不变；`claude.yml` 的 review 触发不变。
- 不再需要路径分类器、dorny 或 `path-filters.yml`；新增检查直接加到对应 job。
