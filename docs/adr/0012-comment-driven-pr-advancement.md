# ADR-0012：评论驱动的 PR 推进——记录唯一信号，agent 代管标签

- 状态：Accepted
- 日期：2026-11-26
- 相关：ADR-0009、ADR-0011、`AGENTS.md`、`docs/agents/issue-tracker.md`、`docs/agents/ci.md`、`scripts/pr-comments.sh`、#110

## 背景

ADR-0009 与 ADR-0011 已确立“记录是审计证据，标签只是等待标记，移除不替代记录”，但通过信号仍要求 reviewer 做「记录 + 移除标签」两步：`AGENTS.md` 的开工与 auto-merge 条件都绑定标签移除，reviewer 漏删标签即卡流程。评论与标签变更又都不触发 workflow，评论到达后没有任何常驻的“读评论 → 执行”驱动，推进依赖人工戳会话。另外，agent 与维护者共用账号（见 ADR-0011 背景）：agent 代发的记录行会被 `guard` 当作有效记录，模板必须防止代发裸行。Claude Code 内置的 `/ship`、`/steward` watch 具备处理 review 评论、增删标签、重跑 check 的能力面，且明确与 auto-merge 共存，但被约束 never to merge。

## 决策

1. **PR 评论是唯一控制通道，记录是唯一通过信号**。reviewer 的全部动作 = 写评论：一整行 `/plan-approved <sha>` 或 `/code-approved <head-sha>`（格式与 `guard` 校验沿用 ADR-0009/0011，不变）；其余一切评论按反馈处理。reviewer 永不移除标签、永不合并。
2. **标签增删由 agent 代管**。标签语义不变（在 = 等待/锁定），`guard` 的两处标签检查不改。agent 收到有效记录 → 移除对应标签 → 开工/收尾；收到反馈 → 改 plan/改码并在评论中说明。
3. **记录行只认裸行**（整行严格匹配）。agent 发布的模板与处理记录中的记录行必须用引用块或行内代码呈现，防止 owner 身份的代发裸行被 `guard` 误判为通过。
4. **agent 每次处理完回一条处理记录评论**（做了什么、为什么），作为已读游标，也充实 PR 的工作记录。
5. **watch 载体复用 `/ship`、`/steward`**（仅由用户一行启动）；协议不依赖具体载体，未启动 watch 时退化为人工戳会话，语义不变。
6. **收尾预约合并**：`gh pr merge --auto --squash` 是预约，不是合并，时机由 GitHub 决定；禁止手动合并与判断 CI 状态。若载体因 never-merge 约束拒绝执行预约命令，退化为用户一键开启 auto-merge，agent 只负责让 check 通过。
7. `scripts/pr-comments.sh` 提供与 `guard` 同口径的三类判定（approved/invalid/feedback）；`guard` 仍是最终门禁。判定口径变更时两处同步。

## 备选方案

- **追加 `plan-approved`/`code-approved` 标记标签作为信号**：要么 reviewer 回到两动作，要么丢掉 sha 绑定；新标签同样面临“移除”语义，不如记录。
- **GitHub 原生 Approve 单击**：动作最少，但丢显式 sha 的记录文本，偏离 ADR-0009/0011 的审计格式；单账号下原生 approve 本就不可用（见 ADR-0011）。
- **标签冻结（永不移除，标签栏当履历）**：PR timeline 本就记录标签增删，履历动机不成立；且“标签在 = 待评审”失效，待办可见性需另找载体。
- **扩展 `claude.yml`，由评论直接触发 CI bot 干活**：全自动，但要大改其章程、权限与 turn 额度，且 GITHUB_TOKEN 推送不回触发 CI；留作后续 issue。
- **reviewer 亲自删标签（本决策前的现行做法）**：两步动作漏一步即卡流程，正是本决策要消除的摩擦。

## 后果

- reviewer 单动作（一条评论）；标签生命周期完全落在 agent 侧。agent 失职（未删标签、未 Re-run）时 `guard` 红灯兜底，等待人工。
- owner 身份的模板贴出裸行即等效通过；引用块纪律是硬要求，`pr-comments.sh --self-test` 固化“引用行/行内代码不算记录”。
- 评论、标签变更仍不触发 run；agent 删标签后仍需 Re-run all jobs（或由下一次 push 触发）。
- never-merge 与预约合并的张力有明确退路（用户一键 auto-merge）；实测载体拒绝预约时按退路执行。
- `guard` 结构零改动；`AGENTS.md` 与 `docs/agents/*` 措辞与本 ADR 对齐。ADR-0009/0011 中“reviewer 移除标签”的表述按历史语境保留 (historical)，决策正文不改写。

## Amendment（2026-11-26，ADR-0013）

决策 2 中「`guard` 的两处标签检查不改」被 ADR-0013 更新：标签检查退役，标签降为纯记账，记录检查完整接管。删标签随之降为记账动作；「Re-run all jobs」演进为记录评论自动触发放行（转发失效时仍可 `gh run rerun`）。记录信号、引用块纪律与模板流程不变。
