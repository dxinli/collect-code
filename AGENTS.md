# AGENTS.md

本文件是仓库级 agent 指令，适用于所有编码代理。指令的唯一来源是本文件；`CLAUDE.md` 只是 Claude Code 的导入入口，不要在其中另写规则。

## Agent 约定

### Issue tracker

Issues live in GitHub Issues; diff notes and archived plans live under `.scratch/<feature-slug>/`; a ticket's implementation plan lives in its PR body. Triage labels are mapped in `docs/agents/triage-labels.md`. The current behavior spec is `docs/spec/spec.md`; contracts are under `docs/contracts/`. See `docs/agents/issue-tracker.md`.

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
6. PR 描述填写 `Spec-Version`、`Spec-Impact`、`REQ` 三项；ticket PR 在正文写 `Closes #<issue>`，由 GitHub 在合并时自动关闭。ticket ID 以 issue 标题前缀和正文 `**Ticket:**` 为准。
7. 引用规范使用 `REQ-*`，引用合同使用合同锚点；不要用 `spec §N` 作为跨文档指针。历史文档中的旧引用保留并标 `(historical)`。
8. 本文件只记录流程与硬约束，不复制规范正文；行为与格式以 spec/contracts 为准。

## Ticket 交付流程（硬约束）

认领 `ready-for-agent` 的 issue 时，遵循以下流程。

### 元指令（每张工单必做）

完成一张工单的收尾后，在该 issue 留评论：

> 📋 流程反馈：本次流程中哪一步最卡 / 哪条规则最没用。
> 若无摩擦，写"无"。连续多次"无"时流程将固化升级。

这是流程的采样机制，你的反馈直接决定 AGENTS.md 的演进方向。

### 认领

- 原子换标签：`ready-for-agent` → `in-progress`，并在 issue 评论认领。
  换标签失败 = 被抢，跳过换下一张。
- 立即开分支 + **draft PR**，body 首行含 `Closes #N`。

### Plan

- PR body 分两节：**Plan**（目标/改动清单/Non-goals/风险点）+ **实现记录**（留空）。
- 打 `plan-review` 标签后停下等评审。标签未移除前，禁止推送任何代码 commit。
- 收到评审意见：更新 body + 评论说明改动原因。
  意见涉及 ticket 验收标准本身 → 不改 plan，去 issue 评论提案，人工裁决。

### Implement

- plan 定稿后逐条实现，每 commit 对应清单一条并在 body 勾选。
- 实现中发现 plan 错误：停下，PR 评论说明，打回 `plan-review`。

### 收尾

- 全部完成后：跑测试 → 自查 → `gh pr ready`。
- 统一使用 `gh pr merge --auto --squash` 预约合并，**不要判断 CI 状态、
  不要手动合并**——合并时机由 GitHub 决定。
- 如果 auto-merge 长时间未合并（CI 红/冲突），留评论说明卡点后停止，
  等待人工介入。不要尝试其他合并方式。

### 纪律

- 一 ticket = 一 PR。范围外需求开新 issue，不塞入本工单。
- 永不修改 issue body——只用评论提案。
- 优先级：ticket 验收标准 > plan > 实现偏好。

## 版本控制（硬约束）

1. 变更文件列表保持整洁，对每阶段的修改进行提交，好追溯问题。
2. 如果整个 `implement` 阶段（多个 commit）的 diff 太大，比如跨多个模块、代码变更行数较多，可以写一个 diff 说明文档。
3. 每个 ticket 开始时切分支（基于 main 分支创建）并开 draft PR，计划写在 PR body 评审；实现完成后 `gh pr ready`，按 Ticket 交付流程合并。
4. 每个 feat 分支不能混入不同的 ticket；如果 ticket 未完成或者有问题但 PR 已合并，开一个 hotfix 分支。
5. 单仓无法完成的工作（如并行任务）使用 worktree 方式。
