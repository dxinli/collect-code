# ADR-0013：记录化门禁——标签检查退役，放行记录自动触发放行 CI

- 状态：Accepted
- 日期：2026-11-26
- 相关：ADR-0008、ADR-0011、ADR-0012、`AGENTS.md`、`docs/agents/ci.md`、`.github/workflows/ci.yml`、`.github/workflows/record-dispatch.yml`、#110、#112

## 背景

ADR-0012 确立评论驱动推进后，流程反馈（#110）指出两处摩擦：其一，批准记录（`/code-approved`）到达后不触发任何 run，guard 又在 `code-review` 标签存在时失败，必须 agent 清标签后再手动 Re-run all jobs 才放行合并——每轮批准多一次动作与一轮等待；其二，「每 commit 勾选 body」在小票上是机械开销。技术前提已核实：GITHUB_TOKEN 触发的事件不创建新 run（防递归），但仓库已配置 PAT（`CLAUDE_GITHUB_TOKEN`）；`issue_comment` 触发的 run 执行在默认分支、check 不落 PR head sha，不能直接当门禁；`gh workflow run --ref <branch>` 的 run 落分支 head sha，可满足 required checks。

## 决策

1. **guard 标签检查退役，记录检查完整接管**。删除「`plan-review` 在 → 只许计划文件」与「`code-review` 在 → 红」两分支：无有效 `/plan-approved` 记录时代码文件被拒、无当前 head 的 `/code-approved` 记录时非 draft 失败，保护不减。记录检查本体（行正则、owner、plan 祖先/计划点范围、code = head）零改动。标签退为纯记账（agent 增删、人看进度），`guard` 不读标签。
2. **放行记录自动触发放行 CI**。新增辅助 workflow `record-dispatch.yml`（非门禁，如 `claude.yml` 同类）：`issue_comment` 粗过滤（PR 评论且含 `/code-approved`）+ step 内裸行严判（与 guard 同正则）后，用 PAT 执行 `gh workflow run "CI" --ref <PR head branch> -f pr=<n>`。`ci.yml` 的 guard 扩展 `workflow_dispatch` 路径（`pr` 输入；base/branch 经 `gh pr view` 解析，head 取 run checkout 即分支当时 head，保留「run head = live head」的过期检查；与事件路径汇合同一校验段）。
3. **收尾次序反转**：`gh pr ready` 后立即 `gh pr merge --auto --squash` 预约；记录到达 → 转发 → run 落 head → guard 绿 → GitHub 自动合并。「没有有效记录不得开 auto-merge」反转为「预约可提前，放行由 guard 记录门禁保证；无记录时 guard 红、合并不会发生」。
4. **计划记录不触发 run**：`/plan-approved` 驱动 agent 开工，代码 push 会起 run；触发过滤只认 `/code-approved`（放行记录触发 CI）。
5. agent 收到有效记录后只做记账（清标签、回处理记录）；转发链未生效（PAT 缺失 / `record-dispatch.yml` 不在默认分支）时退 `gh run rerun`，机制与 ADR-0012 的退路一致。
6. 按阶段勾选实现记录（实现 / 自审修复 / 交付各批一次，每条括注 commit sha），替代逐 commit 勾选。

## 备选方案

- **workflow_dispatch 手动入口（不加转发件）**：一键新 run，但自动化程度不变，仍需人或 agent 在场；`gh run rerun` 已覆盖。
- **恢复 `pull_request_review` / labeled 触发**：ADR-0008/0011 因 run 膨胀否决；未改变前提，继续否决。
- **issue_comment 直接跑门禁**：该 run 的 check 落默认分支 sha，满足不了 required checks；只可做转发（本决策采用）。
- **guard 合成 check run（Checks API）**：可在任意 sha 上报 check，但门禁逻辑要在两类事件里跑两遍、状态机复杂；转发 dispatch 更直白。
- **GITHUB_TOKEN 起 dispatch**：防递归限制下不触发新 run；必须 PAT。

## 后果

- 记录评论成为唯一放行动作：reviewer 一行评论 → CI 自动 → guard 绿 → 自动合并，零人工步骤、agent 可不在场。
- guard 语义变化（标签检查退役）属门禁变更，`ci.yml` 改动须人工评审（CI 维护规则）。
- 转发链依赖 PAT（`CLAUDE_GITHUB_TOKEN`）与「转发件在默认分支」两个条件；失效时退 `gh run rerun`，流程不破。
- 非 owner 评论含 `/code-approved` 会起一次转发 job（严判拦下、不 dispatch）；代价数秒，guard 兜底真实性。
- 模板/处理记录的引用块行命中粗过滤但被严判拦下——引用块纪律（ADR-0012 决策 3）继续有效。
- 合并前无法实测 `issue_comment` 转发链（workflow 文件取自默认分支）；dispatch 路径（`gh workflow run`）可在 PR 分支实测。
