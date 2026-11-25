# AGENTS.md

本文件是仓库级 agent 指令，适用于所有编码代理。指令的唯一来源是本文件；`CLAUDE.md` 只是 Claude Code 的导入入口，不要在其中另写规则。

## Agent 约定

### Issue tracker

Issues live in GitHub Issues; diff notes and archived plans live under `.scratch/<feature-slug>/`; a ticket's implementation plan lives in its PR body. The current behavior spec is `docs/spec/spec.md`; contracts are under `docs/contracts/`. See `docs/agents/issue-tracker.md`.

The execution state is the native GitHub issue status (`Backlog` / `Todo` / `In progress` / `In review` / `Done` / `Not planned` / `Duplicate`), not a label. Labels are classification and routing only: `bug`, `enhancement`, `needs-triage`, `needs-info`, `human`, `spec-parent`, `plan-review`, `code-review`; their mapping is in `docs/agents/triage-labels.md`. Blocking is expressed only by native issue dependencies and is read live; it is never copied into status.

**Spec and tickets (A).** The spec lives only in `docs/spec/spec.md`; changes go through a `spec-delta/<topic>` PR, never a spec issue. The single spec anchor #71 (`spec-parent`) is the parent of every **spec ticket** and of nothing else; its native sub-issue list is the ticket list. Ordinary tickets and chore/hotfix tickets are standalone and do not join the spec tree. The anchor carries `Backlog` (a status cannot be cleared once set) and is never closed or modified by ticket work, including its body; `spec-parent` keeps it out of the claim queue. The anchor points at the authoritative spec; it does not duplicate it.

**Creating a ticket.** Classify it as a **spec ticket** (a spec slice, a split-out of a spec ticket, or a fix of an implementation ticket) or an **ordinary ticket** (docs, tests, scripts, a non-ticket problem, or code spanning several tickets). A spec ticket's title is `<NN>[<letter>]: <标题>` and it attaches #71; an ordinary ticket is standalone with no `NNx:` prefix. The naming details, parent, and the `ticket-new.sh` flags are authoritative in `docs/agents/issue-tracker.md`. New requests start at `Backlog` + `needs-triage` through the template; if the behavior changes, open a `spec-delta/<topic>` PR and keep the ticket in `Backlog` until the spec is accepted, then move it to `Todo`.

**External skills.** `to-spec`, `to-tickets`, and `implement-spec` are not part of the current flow. When a repo-local adapter is written, map `ready-for-agent` to status `Todo` and `ready-for-human` to `Todo` + `human`; do not create those labels.

### Domain docs

Read `docs/README.md`, `docs/spec/spec.md`, the relevant files under `docs/contracts/`, `GLOSSARY.md`, and the ADRs that touch the area. See `docs/agents/domain.md`.

## 项目

智能采集应用：部署在边缘（LoRaWAN 网关 / 融合终端）上的采集引擎。一套二进制，按配置装配南向驱动，北向统一 MQTT JSON。
需求原文见 `docs/需求文档_1.md`、`docs/需求文档_2.md`。真实硬件与协议资料的缺口台账见 `docs/缺口台账.md`。

## 硬约束

- Go，交叉编译 `linux/arm64`，同时保留 `linux/amd64` 用于本机联调；单二进制 + YAML 配置 + systemd；`CGO_ENABLED=0` 静态编译。**不引入外部/网络数据库，不引入 Web 后台**；允许嵌入式 KV（`go.etcd.io/bbolt`）承载幂等与状态（决策 4=B，见 `docs/缺口台账.md` E6）。sqlite 的 cgo 绑定与静态编译冲突，故不用。
- 南向驱动、北向通道、物模型适配器都必须经接口注入，不允许在引擎内核里直接 import 具体实现。
- **时间只能从注入的 `Clock` 进入内核**；内核与驱动不得直接调用运行时计时函数，业务超时不用 `context.WithTimeout`。规则见 `REQ-TIME-01`，背景见 ADR-0003，静态检查为 `scripts/check-time.sh`。
- 持久化只经 `Store` 接口进入内核；bbolt 是唯一生产实现，测试用内存实现。签名以 `internal/interfaces/store.go` 为准，语义见 `docs/contracts/store.md`。
- ChirpStack 是既有系统：本服务只经其 API 做上下行，绝不改动或抢占其端口/会话。
- 当前无实物：所有硬件相关数值一律标记为 `ASSUMED`，集中登记在 `docs/缺口台账.md`，不得散落在代码里当常量口口相传。

## 规范与文档（硬约束）

1. `main` 是规范的唯一写入点；所有规范变更经 PR。
2. 当前行为规范是 `docs/spec/spec.md`，合同在 `docs/contracts/`；`.scratch/` 只放 diff 说明与归档计划，新 ticket 的实施计划在 PR body。
3. 普通 feature branch **不修改** `docs/spec/**`（含 `docs/spec/VERSION`、`docs/spec/CHANGELOG.md`）与 `docs/contracts/**`。
4. 规范变更另开 `spec-delta/<topic>` 分支；该 PR 只改规范包与版本/变更记录，代码与测试随后单独提交并引用 `REQ-*` / 合同锚点。
5. 计划评审前、开始实现前、提交 PR 前，运行 `make spec-status`；分支落后或规范包改动未带版本时必须先处理。
6. 所有非豁免 PR 的正文写且只写一行 `Closes #<issue>`，并填写 `Spec-Version`、`Spec-Impact`、`REQ` 三项。`guard` 校验 `Closes` 目标（open 票、非 #71/PR、正文含 `**Ticket:**`）与三项；`dependabot/*`、`probe/*` 全免，`spec-delta/*` 仅免 `Closes`/issue 校验。ticket ID 以 issue 标题前缀和正文 `**Ticket:**` 为准。
7. 引用规范使用 `REQ-*`，引用合同使用合同锚点；不要用 `spec §N` 作为跨文档指针。历史文档中的旧引用保留并标 `(historical)`。
8. 本文件只记录流程与硬约束，不复制规范正文；行为与格式以 spec/contracts 为准。

## Ticket 交付流程（硬约束）

认领 `status:todo` 且无未关闭 blocker 的 issue 时，遵循以下流程。

### 元指令（每张工单必做）

完成一张工单的收尾后，在该 issue 留评论：

> 📋 流程反馈：本次流程中哪一步最卡 / 哪条规则最没用。
> 若无摩擦，写"无"。连续多次"无"时流程将固化升级。

这是流程的采样机制，你的反馈直接决定 AGENTS.md 的演进方向。

### 认领

- 首选 `scripts/claim.sh [<n>]`：它做下面的查询、重读、`In progress` 写入与评论；等价命令如下。
- 查询可开工集合：`gh issue list --state open --search "status:todo -label:human -is:blocked"`。
- 选中后重读：status 仍为 `Todo`，且 `issue_dependencies_summary.blocked_by == 0`。
- 第一笔写：`gh api -X PATCH repos/{owner}/{repo}/issues/<n> -f status="In progress"`，并在 issue 评论认领。
  重读发现已是 `In progress` = 被抢，跳过换下一张。
- 带 `human` label 或存在未关闭 blocker 的 issue 不得认领。
- 立即开分支 + **draft PR**，body 首行含 `Closes #N`。

### Plan

- PR body 分两节：**Plan**（目标/改动清单/Non-goals/风险点）+ **实现记录**（留空）。
- 计划前先核实事实，再写断言。Plan 必含 **已核实事实** 表，每行 `事实 | 核实方式 | 结论`。至少覆盖：相关 Accepted ADR 的约束；每个检查实际读取什么（例如 `check-spec` 的 contracts/covers 会读 Go）；第三方 action/工具在固定版本下的关键语义；脚本处理路径的编码与非 ASCII 测试矩阵。无法核实的事实不能作为断言写入 Plan：先核实，或把待决问题提到 issue 并停下。格式、示例与要求理由见 [docs/agents/issue-tracker.md](docs/agents/issue-tracker.md#plan)。
- 计划前读 [docs/agents/ci.md](docs/agents/ci.md)：CI 全量运行，标签变更不触发；需要重新评估时 Re-run all jobs。
- 打 `plan-review` 标签后停下等评审。标签未移除、或没有有效的 `/plan-approved <sha>` 记录前，禁止推送任何代码 commit。
- **通过信号 = reviewer 在 PR 会话评论或 review 正文中留 `/plan-approved <sha>`，并移除 `plan-review` 标签**。`<sha>` 是计划阶段的 commit；`guard` 校验它是 PR head 的祖先、由本 PR 引入，且该点只含计划文件。agent 不得自己写这条记录。恢复工作时先重读 PR 的 label、body、comments；若 reviewer 只留了评论、没有移除 label，先处理意见，不得开工。
- 记录与标签规则豁免 `spec-delta/*`、`dependabot/*`、`probe/*`；`probe/*` 是临时 CI 验证分支，用完关闭，不合并。
- draft PR 的 `guard` 会因 code review 门禁为红，这是预期；计划通过看 `/plan-approved` 与标签移除，不看 `guard` 颜色。
- 收到评审意见：更新 body + 评论说明改动原因。
  意见涉及 ticket 验收标准本身 → 不改 plan，去 issue 评论提案，人工裁决。

### Implement

- plan 定稿后逐条实现，每 commit 对应清单一条并在 body 勾选。
- 实现中发现 plan 错误：停下，PR 评论说明，打回 `plan-review`。

### 收尾

- 全部完成后：跑测试 → 自审 `/code-review`（修复发现的问题）→ 把 issue status 设为 `In review` → `gh pr ready` → 给 PR 加 `code-review` 标签。
- 人工 review 通过后，reviewer 在 PR 会话评论或 review 正文留 `/code-approved <head-sha>` 并移除 `code-review` 标签；记录必须对应当前 head。agent 不得自写这条记录。
- 记录后 Re-run all jobs，确认 `guard` 通过；**没有有效记录、或 `code-review` 标签仍在，不得开 auto-merge**。批准后的任何新 commit 都会使记录失效，需要重新评审。
- `guard` 通过后：使用 `gh pr merge --auto --squash` 预约合并，**不要判断 CI 状态、
  不要手动合并**——合并时机由 GitHub 决定。
- 合并后确认 issue status 为 `Done`；若原生自动化未设，手动补。
- PR 关闭但未合并：把 issue status 设回 `Todo`（仍 ready）或 `Backlog`，评论原因。
- 如果 auto-merge 长时间未合并（CI 红/冲突），留评论说明卡点后停止，
  等待人工介入。不要尝试其他合并方式。

### 纪律

- 一 ticket = 一 PR。范围外需求开新 issue，不塞入本工单。
- 不使用 `implement-spec`；本仓库的 ticket 按上面的单 PR 流程实现。
- 永不修改 issue body——只用评论提案。
- 优先级：ticket 验收标准 > plan > 实现偏好。

## 版本控制（硬约束）

1. 变更文件列表保持整洁，对每阶段的修改进行提交，好追溯问题。
2. 如果整个 `implement` 阶段（多个 commit）的 diff 太大，比如跨多个模块、代码变更行数较多，可以写一个 diff 说明文档。
3. 每个 ticket 开始时切分支（基于 main 分支创建）并开 draft PR，计划写在 PR body 评审；实现完成后 `gh pr ready`，按 Ticket 交付流程合并。
4. 每个 feat 分支不能混入不同的 ticket；如果 ticket 未完成或者有问题但 PR 已合并，开一个 hotfix 分支。
5. 单仓无法完成的工作（如并行任务）使用 worktree 方式。
