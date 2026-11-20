# AGENTS.md

本文件是仓库级 agent 指令，适用于所有编码代理。指令的唯一来源是本文件；`CLAUDE.md` 只是 Claude Code 的导入入口，不要在其中另写规则。

## Agent 约定

### Issue tracker

Issues, specs and tickets live as markdown under `.scratch/<feature-slug>/` in this repo. See `docs/agents/issue-tracker.md`.

### Domain docs

Single-context: `GLOSSARY.md` at the root plus `docs/adr/`. See `docs/agents/domain.md`.

## 项目

智能采集应用：部署在边缘（LoRaWAN 网关 / 融合终端）上的采集引擎。一套二进制，按配置装配南向驱动，北向统一 MQTT JSON。
需求原文见 `docs/需求文档_1.md`、`docs/需求文档_2.md`。真实硬件与协议资料的缺口台账见 `docs/缺口台账.md`。

## 硬约束

- Go，交叉编译 `linux/arm64`，同时保留 `linux/amd64` 用于本机联调；单二进制 + YAML 配置 + systemd；`CGO_ENABLED=0` 静态编译。**不引入外部/网络数据库，不引入 Web 后台**；允许嵌入式 KV（`go.etcd.io/bbolt`）承载幂等与状态（决策 4=B，见 `docs/缺口台账.md` E6）。sqlite 的 cgo 绑定与静态编译冲突，故不用。
- 南向驱动、北向通道、物模型适配器都必须经接口注入，不允许在引擎内核里直接 import 具体实现。
- **时间只能从注入的 `Clock` 进入内核**。`internal/core/*` 与 `internal/drivers/*` 禁止直接调用 `time.Now/NewTicker/NewTimer/After/AfterFunc/Sleep`；业务超时不用 `context.WithTimeout`（真实定时器无法被假时钟快进）。见 ADR-0003 决策 5。
- 持久化只经 `Store` 接口进入内核；bbolt 是唯一生产实现，测试用内存实现（ADR-0004 §4a）。
- ChirpStack 是既有系统：本服务只经其 API 做上下行，绝不改动或抢占其端口/会话。
- 当前无实物：所有硬件相关数值一律标记为 `ASSUMED`，集中登记在 `docs/缺口台账.md`，不得散落在代码里当常量口口相传。

## Ticket 交付流程（硬约束）

每个 ticket 开始实现之前，必须按顺序完成：

1. **Plan**：用 `/plan`（其他代理使用等效的计划模式）生成实施计划；使用干净上下文（新会话或子代理），并切换模型（平台支持时）。计划文档提交，提交格式：`plan: 01a 实施计划（待评审）`
2. **独立评审**：用另一个模型、干净上下文评审计划；评审人必须自己读 spec/ticket/ADR/代码，不能只看计划自述。计划文档提交，提交格式：`plan: 01a 计划 v2（按独立评审修订）`
3. **交用户评审**：把交叉评审后的结果交给用户；无法把握的点给 A/B 两案和推荐，由用户拍板。用户每提出一次问题，修改后按照格式提交。
4. **用户明确说“计划评审通过”**：只有得到这句明确确认，才可以开始写代码。
5. **计划提交**：计划文档每个阶段均需要单独提交（`plan: ...`），不要混在一个提交。
6. 未通过计划评审前：不写实现、不提交实现、不标 `done`、不开始下一个 ticket。
7. **实现**：`/implement`（其他代理按计划实现）过程中，按照功能点分多次提交。

实现过程中若发现计划需要改变规则/范围/输出契约，停止实现，回到第 1–4 步重新评审；不得用“实现中发现”代替用户评审。

流程违规的处理：回退全部未提交实现，恢复到上一个已交付提交，按本流程从 Plan 重新执行。流程规则文档（如本文件）本身应最早处理并单独提交。

## 版本控制（硬约束）

1. 变更文件列表保持整洁，对每阶段的修改进行提交，好追溯问题。
2. 如果整个 `implement` 阶段（多个 commit）的 diff 太大，比如跨多个模块、代码变更行数较多，可以写一个 diff 说明文档。
3. 每个 ticket 开始时切分支（基于 main 分支创建）；ticket 完成实现后创建 PR，等用户审核。
4. 每个 feat 分支不能混入不同的 ticket；如果 ticket 未完成或者有问题但 PR 已合并，开一个 hotfix 分支。
5. 单仓无法完成的工作（如并行任务）使用 worktree 方式。

## 代码走读

针对每个完整 ticket（如 01a、01b、01c 合起来才算一个 ticket，而非单个子 ticket）生成一份代码走读设计：

1. 链路（包含图）
2. 组件或者说对象关系（包含依赖图）
3. 时序（包含时序图），如果有的话
