# 文档地图

本仓库的文档按**事实类型**分权：每类事实只有一个权威位置，其余文档只链接。规范包版本与变更规则见 [ADR-0006](adr/0006-doc-authority-and-spec-change-policy.md)。

## 当前工作的阅读路径

新 ticket、评审与实现只需按以下顺序读取相关部分：

1. [docs/产品定型.md](产品定型.md)：产品意图、范围、路线。
2. [docs/spec/spec.md](spec/spec.md)：行为契约，按 `REQ-*` 检索。
3. [docs/contracts/](contracts/)：按领域读取外部报文、配置、Store、模拟器契约。
4. [docs/adr/](adr/)：只读与当前变更相关的决策与理由。
5. [GLOSSARY.md](../GLOSSARY.md)：术语。
6. 当前 ticket：GitHub Issues；实施计划：ticket PR body；历史计划与 diff 说明：`.scratch/README.md`（归档索引）。
7. [docs/agents/ci.md](agents/ci.md)：CI 检查读什么、路径组与缓存。

**默认不读**：历史 tag、已实现 ticket 的计划正文。它们用于追溯，不是当前行为权威。

## 权威位置

| 事实类型 | 权威 | 说明 |
|---|---|---|
| 接口签名、冻结注释 | `internal/interfaces/**` | 规范与合同只链接，不复制签名 |
| 行为语义 | [docs/spec/spec.md](spec/spec.md) | 稳定 `REQ-*` |
| 外部 Topic / JSON / code / 事件 | [docs/contracts/external.md](contracts/external.md) | 云端对接的唯一来源 |
| 配置键 / 默认值 / 校验规则 | `internal/config/types.go`、`defaults.go`、`rules.go` | `configs/` 与 testdata 只是示例 |
| Store 语义 | [docs/contracts/store.md](contracts/store.md) | 签名以 `interfaces.Store` 为准 |
| 模拟器契约 | [docs/contracts/simulator.md](contracts/simulator.md) | CLI、manifest、HTTP API |
| 决策与理由 | [docs/adr/](adr/) | 不重抄规范正文 |
| 术语 | [GLOSSARY.md](../GLOSSARY.md) | 唯一术语表 |
| 产品意图 | [docs/产品定型.md](产品定型.md) | 行为细节链接规范 |
| 架构与包布局 | [docs/architecture.md](architecture.md) | 依赖方向与包职责 |
| 测试分层与判据 | [docs/testing.md](testing.md) | L1–L4、假时钟、崩溃恢复 |
| CI 检查归属 | [docs/agents/ci.md](agents/ci.md) | 检查 → 输入 → 路径组 → 缓存 |
| 追踪状态 | [docs/traceability.md](traceability.md) | `REQ/锚点 → test/ticket` |
| Tracker 根 | GitHub issue `Spec: 采集引擎行为契约`（`spec-parent`） | 所有 spec ticket 的 sub-issue 父级（sub-issue 列表即票列表）；普通票与 chore/hotfix 独立；执行状态用原生 issue status |
| 单票验收 | GitHub Issues | 合并即关闭；执行状态用原生 issue status；历史状态用评论追溯 |
| 流程与交付规则 | [AGENTS.md](../AGENTS.md) | 不在规范中重复 |

## 规范包与版本

- 当前版本：`docs/spec/VERSION`。
- 变更记录：`docs/spec/CHANGELOG.md`。
- 旧规范：tag `spec-v2-draft`；用 `make spec-archive` 读取，不在工作区保留副本。
- 变更流程：独立分支 `spec-delta/<topic>`，PR 只改规范包与版本/变更记录；代码与测试在后续 PR 引用新锚点。详见 ADR-0006。

## 目录

```text
docs/
├── README.md                # 本文件
├── 产品定型.md               # 产品意图与路线
├── 需求文档_1.md / _2.md     # 原始需求，不改写
├── 缺口台账.md               # ASSUMED 与事实缺口
├── architecture.md          # 组件、依赖方向、包布局
├── testing.md               # 测试分层与数值判据
├── traceability.md          # REQ / 合同锚点追踪
├── spec/
│   ├── spec.md              # 行为契约
│   ├── VERSION              # 规范包 semver
│   └── CHANGELOG.md         # 变更记录
├── contracts/
│   ├── external.md          # 云端报文与 code/event
│   ├── config.md            # 配置导航
│   ├── store.md             # 持久化语义
│   └── simulator.md         # 模拟器契约
├── adr/                     # 决策记录与索引
└── agents/                  # agent 的仓库约定
```
