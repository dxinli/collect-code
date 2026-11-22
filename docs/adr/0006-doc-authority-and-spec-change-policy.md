# ADR-0006：文档按事实类型分权，规范包版本化并以独立 PR 变更

- 状态：Accepted
- 日期：2026-11-22
- 相关：`docs/README.md`、`docs/spec/spec.md`、`docs/contracts/**`、`AGENTS.md`

## 背景

上一版把产品意图、接口签名、配置模型、报文格式、测试策略、决策理由与评审过程全部塞进一个 `spec.md`。结果是：

1. 同一个事实在 spec、PRD、ADR 与代码里各有一份，修改时必然漂移；
2. 规范正文混入决策编号和评审记录，新 ticket 必须读历史才能判断当前行为；
3. 规范没有版本锚点，分支与 worktree 无法判断自己引用的是哪一版；
4. 旧章节被保留成“兼容空壳”，引用者继续依赖旧编号，无法真正收敛。

## 决策

### 1. 按事实类型确定唯一权威

| 事实类型 | 权威位置 |
|---|---|
| 接口签名与冻结注释 | `internal/interfaces/**` |
| 行为语义 | `docs/spec/spec.md` 的 `REQ-*` |
| 外部 Topic / JSON / code / 事件 | `docs/contracts/external.md` |
| 配置键 / 默认值 / 校验规则 | `internal/config/types.go`、`defaults.go`、`rules.go` |
| Store 规范语义 | `docs/contracts/store.md` |
| 模拟器契约 | `docs/contracts/simulator.md` |
| 决策与理由 | `docs/adr/**` |
| 术语 | `GLOSSARY.md` |
| 产品意图与路线 | `docs/产品定型.md` |
| 架构与包布局 | `docs/architecture.md` |
| 单票验收 | ticket |
| 历史过程 | plans 与 git tag |

其他文档只链接权威位置，不复制规范正文。

### 2. 规范包与版本

- 规范包 = `docs/spec/spec.md` + `docs/contracts/**`。
- 包版本存于 `docs/spec/VERSION`，使用 `MAJOR.MINOR.PATCH`。
- 编辑性修订升 patch；新增 `REQ-*` 或合同锚点升 minor；删除、改义或改变兼容性升 major。
- `docs/spec/CHANGELOG.md` 记录每次变更涉及的 `REQ-*` 与合同锚点。
- 旧规范以 git tag 冻结，不在工作区保留副本或旧标题空壳。

### 3. 引用方式

- 规范引用使用稳定的 `REQ-*`；合同引用使用合同锚点（如 `EXT-CODE-01`、`STORE-CLOSE-01`）。
- 不使用 `spec §N` 作为跨文档指针；章节号可以调整，锚点不可以。
- 历史文档中的旧章节引用保留原文并标 `(historical)`，不建立“旧编号 → 新锚点”的常驻映射层。
- 不确定如何在不改变语义的前提下更新时，停止修改并走计划修订。

### 4. 变更控制

1. 规范变更使用独立分支 `spec-delta/<topic>`，PR 只改 `docs/spec/**`、`docs/contracts/**`、`docs/spec/VERSION`、`docs/spec/CHANGELOG.md`。
2. 代码与测试在后续 PR 中引用新的 `REQ-*` / 合同锚点，不在规范 PR 中混入实现。
3. 首次建立规范包的 PR 可用 `spec-bootstrap` 标记豁免“只改文档”检查，但合并前必须对目标状态运行同一套检查。
4. 普通 feature branch 不修改规范包；发现规范错误时另开 `spec-delta`。
5. `main` 是规范唯一写入点；所有变更经 PR。

### 5. 跨分支与 worktree

- 分支在计划评审前、开始实现前、提交 PR 前运行 `make spec-status`。
- `spec-status` 用 merge-base 判断分支是否落后、比较规范包版本、检查本分支是否修改规范包。
- PR 模板记录 `Spec-Version`、`Spec-Impact`、`REQ`。
- 本地 `spec-impact` 可扫描分支、worktree 与 tickets 的引用变化；CI 只做 merge-base 与版本门禁。

### 6. ADR 只记录决策

- ADR 保留背景、理由、被否决方案与后果，不重抄规范正文。
- 已有 ADR 的规范正文迁移到 spec/contracts 时，用追加 Amendment 声明归属，不改写原文。
- 需要推翻旧决策时新增 ADR 并在旧 ADR 标注 `Superseded by ADR-NNNN`。

## 后果

- 新 ticket 的必读上下文收敛为：相关 `REQ-*`、合同锚点、ADR、ticket 本身。
- 规范与实现的漂移有明确检测点：版本号、CHANGELOG、CI 与 PR 字段。
- 历史仍可追溯，但历史不再是当前权威，也不需要被新工作读取。
- 代价是规范变更多一次独立 PR；这是为了让代码变更与规范变更可以分别评审、分别回滚。
