# Spec 管理化方案落地计划（v3）

**状态**：待用户评审（2026-11-22；按用户修正 v3）
**基础**：`main@461121c`（PR #6 / 06b 已合并；PR #9 / 02b plan 已合并）
**工作分支**：`docs/spec-v3`
**范围**：文档治理、spec v3、契约拆分、**02b/06b 文档纳入调整**、历史 plans/tickets 直接规范化、PRD 清理、CI 约束
**非目标**：不修改产品行为、接口签名、配置字段、错误码、事件语义；不修改 06b 已合并实现；不恢复 02b 实现；不替用户拍板 02b 的 A/B。

---

## 0. 用户修正与中心原则

### 0.1 用户修正

1. **02b、06b 也在修改范围内**：它们的 plan/ticket/文档必须按新 spec 规范调整；此前计划把它们排除，已修正。
2. **plans/tickets 必须调整**：只要语义不变、不与代码实现冲突，就必须更新到新的 spec 锚点/合同/REQ；不能保留旧引用。

### 0.2 中心原则

1. **不做兼容映射契约**：
   - 不引入 `refs-migration-log.md` 这类常驻解析层；
   - 不保留旧标题空壳；
   - 文档在语义不变的前提下直接改为新锚点；
   - 若改引用会改变语义，停止并走 plan 修订，而不是靠映射表兼容。
2. **spec 内容不能重复**：
   - spec 之间不重复；
   - spec 与 `AGENTS.md`、ADR、contracts、testing、config 之间不重复；
   - 每类事实只有一个权威，其他文档只链接。

---

## 1. 当前事实

| 项 | 状态 |
|---|---|
| `main` | `461121c Merge pull request #6 from dxinli/feat/06b-mqtt-broker-stub` |
| 06b | 已合并；`internal/sim/**` 在 main；ticket `done` |
| 06b spec 改动 | `.scratch/collector/spec.md:621` 起有 `#### 7.1 06b 冻结的 MQTT broker + 云端桩契约` |
| 06b worktree | `.claude/worktrees/06b` 仍存在；分支 `feat/06b-mqtt-broker-stub` 仍存在 |
| 06b plan | `.scratch/collector/plans/06b.md`，含 `spec §7.1` 引用 |
| 02b | plan 已并入 main；ticket `on-hold`；实现暂停；plan v2 待用户评审 |
| 02b 未决 | plan §4.5/§4.12/§4.13 的 A/B 尚未拍板；§10.8 依赖这些选择 |
| 02b plan | `.scratch/collector/plans/02b.md`，含 `spec §3` 追加指令 |
| spec v2 | `.scratch/collector/spec.md`，含实现细节、评审编号、ticket 编号 |
| 产品定型 | `docs/产品定型.md` 含旧接口、报文副本、D 决策表 |
| main 保护 | 必须经 PR；required checks `ci`、`gitleaks` |

---

## 2. 最终方案

1. **spec 版本化且受控**：
   - 当前规范包：`docs/spec/spec.md` + `docs/contracts/**`；
   - 包版本：`docs/spec/VERSION`（`v3.0.0`）；
   - v2 用 tag `spec-v2.0.0` 冻结；工作区不保留全文副本；
   - `docs/spec/CHANGELOG.md` 记录 REQ 与合同锚点变更；
   - semver：编辑 patch、增补 minor、破坏 major。
2. **spec 只放规范**：
   - 每条规范有稳定 `REQ-*` ID；
   - 不写理由、评审建议、决策编号、被否决方案、ticket 编号、实现历史、完整代码/配置/JSON。
3. **按事实类型分权**（不是全局链）：
   - 接口签名 → `internal/interfaces`
   - 行为 → `docs/spec/spec.md` 的 `REQ-*`
   - 外部 Topic/code/event → `docs/contracts/external.md`
   - 配置导航/校验分类 → `docs/contracts/config.md` + 代码/testdata
   - Store 契约 → `docs/contracts/store.md`（`interfaces.Store` + 02a；02b pending）
   - 模拟器契约 → `docs/contracts/simulator.md`
   - 决策与理由 → `docs/adr/**`
   - 术语 → `GLOSSARY.md`
   - 产品意图 → `docs/产品定型.md`
   - 架构/包布局 → `docs/architecture.md`
   - 单票验收 → tickets
   - 历史 → plans / tag
4. **兼容不污染规范，也不做映射层**：
   - 不保留旧标题空壳；
   - 02b/06b 及所有 plans/tickets 在语义不变时直接更新到新锚点；
   - 若引用更新会改变语义，停止并另发 plan 修订；
   - 不创建常驻 `refs-migration-log`。
5. **新工作最小上下文**：
   - 新 ticket 只允许 `REQ-*`、合同锚点、`ADR-*`；
   - 禁止 `spec §N`、决策编号、archive 路径、旧 plan 路径；
   - `docs/README.md` 定义当前/历史阅读路径；
   - `docs/agents/domain.md` 默认不读 archive/旧 plan。
6. **跨分支/worktree 同步**：
   - `VERSION` + `spec-check.sh` + PR 模板 + CI；
   - 分支落后 main、feature branch 改规范、规范包变更无版本，均失败。
7. **06b / 02b 处理**：
   - 06b §7.1 → `docs/contracts/simulator.md`；06b plan/ticket 的 spec 引用同步更新；
   - 02b plan 的 `spec §3` 目标改为 `docs/contracts/store.md`；A/B 仍不拍板；ticket 保持 `on-hold`。

---

## 3. 不可破坏的兼容性约束

| 编号 | 约束 |
|---|---|
| C1 | 06b 已合并实现不改；ticket 状态/验收/提交哈希不改；只更新文档引用与 spec 归属 |
| C2 | 02b 保持 `on-hold`；不实现、不勾选、不标 done；不把未拍板 A/B 写成契约 |
| C3 | 01a/01b/01c/02a/06b 的验收、评论、提交哈希不改 |
| C4 | plans/tickets 在语义不变时**必须**更新到新锚点；不保留旧 `spec §N` 引用；不建映射契约 |
| C5 | 不修改 `cmd/**`、`internal/**`、`go.mod`、`go.sum`、`internal/config/testdata/**`；允许修改 `Makefile`、`.github/**`、`scripts/spec-*` |
| C6 | 不修改 `docs/需求文档_*.md`、`docs/缺口台账.md` 内容 |
| C7 | ADR 原文不改写；ADR-0004 用 Amendment |
| C8 | spec v3 不保留旧标题空壳 |
| C9 | 所有变更经 PR 进入 main；禁止直接 push main |
| C10 | 06b worktree/branch 在启用门禁前退役 |
| C11 | spec 不与 AGENTS/ADR/contracts/testing 重复同一规范内容 |
| C12 | 任何文档修改若会改变语义，停止并走 plan 修订，不允许“兼容映射”绕过 |

### 3.1 基线

```bash
git rev-parse HEAD > /tmp/spec-v3-base.txt
sha256sum .scratch/collector/issues/*.md \
          .scratch/collector/plans/*.md \
          .scratch/collector/diffs/*.md \
          .scratch/collector/ticket-dependency.html \
          docs/需求文档_*.md \
          docs/缺口台账.md \
          docs/代码走读/*.md \
          > /tmp/spec-v3-historical.sha
```

plans/tickets 在规范化阶段允许变化，但必须用“语义 diff”证明只改了引用/格式：
- 状态、验收勾选、日期、评论、代码块、被引用原文不变；
- 仅引用锚点、路径、REQ/contract 名称变化；
- 逐文件记录改动理由，不建立常驻映射契约。

---

## 4. 目标文档结构

| 文档 | 唯一职责 |
|---|---|
| `docs/README.md` | 文档地图、按事实分权、当前/历史阅读路径 |
| `docs/spec/spec.md` | 行为契约（`REQ-*`） |
| `docs/spec/VERSION` | 规范包 semver |
| `docs/spec/CHANGELOG.md` | REQ + 合同锚点变更 |
| `docs/contracts/external.md` | Topic、JSON、code、事件、seq |
| `docs/contracts/config.md` | 配置导航、四层关系、校验分类（不复制全量键） |
| `docs/contracts/store.md` | Store 契约（`interfaces.Store` + 02a；02b pending） |
| `docs/contracts/simulator.md` | 模拟器契约（06b §7.1 + 非目标） |
| `docs/testing.md` | L1–L4、假时钟、bbolt、模拟器、数值判据 |
| `docs/traceability.md` | REQ/合同锚点 → test/ticket |
| `docs/architecture.md` | 组件、依赖方向、包布局 |
| `docs/adr/README.md` | ADR 索引 + D 决策索引 |
| `docs/adr/0006-doc-authority-and-spec-change-policy.md` | 文档权威、版本、变更控制 |
| `.scratch/collector/issues/**` | 单票验收（更新引用，不改语义） |
| `.scratch/collector/plans/**` | 计划历史（更新引用，不改语义） |

**不新增** `docs/refs-migration-log.md`。历史引用在当前文档中直接改到新锚点；无法语义不变地改的，停在此处走 plan 修订。

### 4.1 按事实类型分权

| 事实类型 | 权威 | 冲突处理 |
|---|---|---|
| 接口签名 | `internal/interfaces` | 代码/契约测试为准；spec 只写行为 |
| 行为语义 | `docs/spec/spec.md` `REQ-*` | spec delta |
| 外部报文 | `docs/contracts/external.md` | 合同 PR + spec delta（如影响行为） |
| 配置键/默认值 | `internal/config` + `testdata/config/base` | config.md 只导航；代码/testdata 为准 |
| Store 语义 | `internal/interfaces/store.go` + 已批准 02a | 02b 未批准部分标 pending |
| 决策/理由 | `docs/adr/**` | 新 ADR 或 Amendment |
| 术语 | `GLOSSARY.md` | 更新 glossary |
| 产品意图 | `docs/产品定型.md` | 产品评审 |
| 架构/包布局 | `docs/architecture.md` + `check-deps` | 架构 PR |
| 单票验收 | ticket | 计划评审 |

### 4.2 Spec 去重

禁止 spec 重复以下内容：

| 内容 | 唯一归属 | spec 只做 |
|---|---|---|
| 流程/交付/版本控制规则 | `AGENTS.md` | 不写流程 |
| 决策理由/被否决方案 | ADR | 不写理由 |
| Topic/JSON/code/事件表 | `docs/contracts/external.md` | 链接 |
| 四层 YAML/键/默认值 | `docs/contracts/config.md` + 代码/testdata | 链接/摘要 |
| Store 实现细节 | `docs/contracts/store.md` | 链接 |
| 模拟器 API | `docs/contracts/simulator.md` | 链接 |
| 测试分层/命令/数值判据 | `docs/testing.md` | 链接/验收摘要 |
| 术语定义 | `GLOSSARY.md` | 使用术语，不重定义 |

同样，`AGENTS.md` 不重复 spec 的行为规则；硬约束只保留一句规则 + 链接。`ADR` 不重抄完整 spec；只记录决策与理由。

---

## 5. Spec v3 结构

```text
docs/spec/spec.md

# Spec v3.0.0：采集引擎行为契约

- Status: proposed（用户批准后改 accepted）
- Version: 3.0.0
- Supersedes: v2.0.0（tag spec-v2.0.0）
- Normative: §1–§9
- Change log: docs/spec/CHANGELOG.md

1. 目的与范围                         REQ-SCOPE-01
2. 系统不变量
   REQ-TIME-01        时间只来自 Clock
   REQ-QUALITY-01     最终质量只由内核计算
   REQ-QUALITY-02     last_success_at 只由新鲜观测推进
   REQ-QUALITY-03     质量真值表（OK/Partial/Failed/Unsupported/解码失败）
   REQ-QUALITY-04     离线判定与恢复（online + 全量快照）
   REQ-QUALITY-05     边界用严格大于；window±1ns 测试
3. 采集调度
   REQ-SCHED-01       相位锁定、不补跑
   REQ-SCHED-02       单设备 deadline、missed_tick
   REQ-SCHED-03       链路内串行、互不拖死
4. 服务调用语义
   REQ-INVOKE-01      (device, seq) 至多一次受理
   REQ-INVOKE-02      两阶段 accepted→final；终态未知 4004
   REQ-INVOKE-03      分级确认；仅 C 级回 code=0
   REQ-INVOKE-04      C 级判定算法（opening/state_word）
   REQ-INVOKE-05      失败码矩阵与重试边界
5. 下行队列与缓存
   REQ-QUEUE-01       同 device+service 只留最新
   REQ-QUEUE-02       每设备上限、cmd_dropped、精确撤销
   REQ-QUEUE-03       cmd_superseded_after_send
   REQ-CACHE-01       事件有界缓存、恢复先事件后快照
   REQ-CACHE-02       event_dropped 可观测
6. 事件语义
   REQ-EVENT-01       事件枚举与触发
   REQ-EVENT-02       episode 与 dedup_key
   REQ-EVENT-03       事件与属性两条通道
7. 外部契约摘要
   链接 docs/contracts/external.md
8. 运维与 CLI
   REQ-OPS-01         SIGHUP 差别重载
   REQ-OPS-02         健康端点与指标
   REQ-OPS-03         日志字段与脱敏
   REQ-CLI-01         子命令契约
   REQ-SIM-01         模拟器能力要求
9. Out of Scope
```

### 5.1 Spec 禁止内容

- 决策/评审编号：`D1–D28`、`P1=A`、`S1/S3/W4`、`N2/N7`；
- “评审结论”“已定稿”“被否决方案”“理由”；
- ticket 编号、提交哈希、实现历史；
- 完整 Go 接口、完整 YAML、完整 JSON、测试命令、测试类名；
- 旧章节重定向空壳；
- 版本历史章节（只留元数据 + CHANGELOG 链接）；
- AGENTS 的流程/交付/版本控制规则。

---

## 6. v2 迁移矩阵（未覆盖不得合并）

| v2 位置 | 内容 | 新归属 |
|---|---|---|
| `spec.md:10-19` Problem | 两类现场、共同内核 | PRD §1/§2；spec §1 目的 |
| `spec.md:21-32` Solution | 南向/内核/北向/物模型 | PRD §2；`docs/architecture.md` |
| `spec.md:34-45` In Scope | v0.1–v1.0 范围 | PRD §3；spec `REQ-SCOPE-01` |
| `spec.md:47-94` User Stories | 行为/测试/交付故事 | 行为 → REQ/ticket；测试/构建 → testing/tickets；交付 → PRD |
| `spec.md:98-310` §1 配置 | 四层 YAML、默认值、示例 | `contracts/config.md` 导航 + testdata；热重载 → `REQ-OPS-01` |
| `spec.md:311-322` §2 校验 | 规则分类、V70–V73 | `contracts/config.md` 分类 + `rules.go` |
| `spec.md:324-455` §3 接口 | Go 签名、Clock/Store 缝隙 | `internal/interfaces`；语义 → `REQ-TIME-01`、`REQ-SEAM-01/02` |
| `spec.md:456-546` §4 报文 | Topic/JSON/code/事件 | `contracts/external.md`；事件语义 → `REQ-EVENT-*` |
| `spec.md:547-564` §5.1 质量 | 真值表 | `REQ-QUALITY-01..05` |
| `spec.md:565-581` §5 可靠性 | 幂等/重试/队列/缓存 | `REQ-INVOKE-*`、`REQ-QUEUE-*`、`REQ-CACHE-*`；理由 → ADR-0004/0005 |
| `spec.md:582-592` §5.2 健康 | 端点/指标 | `REQ-OPS-02` |
| `spec.md:593-602` §6 子命令 | run/config-check/simulate/... | `REQ-CLI-01` |
| `spec.md:604-619` §7 模拟器 | 能力要求、端点 | `contracts/simulator.md`；能力 → `REQ-SIM-01` |
| `spec.md:621-710` §7.1 06b 契约 | broker/manifest/API | `contracts/simulator.md`（保留非目标） |
| `spec.md:712+` Testing | L1–L4/假时钟/稳定性 | `docs/testing.md` |
| `spec.md:665-681` Out of Scope/Further | 非目标、路线 | spec §9；PRD §14 |
| `产品定型.md:213-353` §6.1–6.3 | 旧接口代码 | 删除，链接 `internal/interfaces` |
| `产品定型.md:355-369` §6.3.1 | 调度算法 | ADR-0003 |
| `产品定型.md:371-380` §6.4 | 组件职责 | `docs/architecture.md` |
| `产品定型.md:384-402` §7 | 包布局 | `docs/architecture.md` + `check-deps` |
| `产品定型.md:432-481` §9 | D 表/被否决 | `docs/adr/README.md` 决策索引 |
| `产品定型.md:485-595` §10 | 报文 | `contracts/external.md` |
| `产品定型.md:612-622` §12 | 验收 | `docs/testing.md` |

**覆盖门禁**：实施 PR 必须附“v2 行区间 → 新归属”的勾选表；任何未映射条目必须写明“删除理由”，否则不得合并。

### 6.1 02b 的边界

- 02b plan §10.8 依赖未拍板的 A/B（§4.5/§4.12/§4.13）；
- 本次**不决定** A/B，也不把 §10.8 写成当前契约；
- 但 02b plan 中的 `spec §3` 目标改为 `docs/contracts/store.md`，这是文档位置变更，不改变待选语义；
- `store.md` 只包含 `internal/interfaces/store.go` 冻结语义 + 02a 已批准内容；
- 02b 补充标 `Pending 02b plan approval`；
- 02b ticket 保持 `on-hold`。

### 6.2 06b 的契约抽取

`docs/contracts/simulator.md` 包含：

- CLI flags；
- manifest schema；
- captured/invoke/control；
- `broker_disconnect`；
- 安全与独立客户端测试；
- **当前版本非目标**：不提供 `/_sim/requests`、ChirpStack fixture、`--config`、生产 MQTT Channel；06a/12a/12b 后续追加，不改本节契约；
- 模拟器独立性：不加载生产配置、不依赖硬件假设、不 import 被测实现。

06b plan/ticket 中的 `spec §7.1` 引用同步改为 `docs/contracts/simulator.md`；ticket 状态/验收/提交哈希不变。

---

## 7. 契约文档

### 7.1 `external.md`

- 5 Topic；
- 属性 JSON；
- 服务调用/回执；
- `seq` 契约；
- code 0/4001–4004/4041/5040–5044；
- stage；
- 事件枚举（含 `valve_fault_clear`）；
- dedup/episode；
- `target_opening`；
- LWT。

### 7.2 `config.md`

**不复制全量键**。内容：

- 四层文件与引用关系；
- 校验分类；
- 示例指向 `internal/config/testdata/config/base/`；
- 规则 ID 指向 `internal/config/rules.go`；
- 热重载矩阵链接 `REQ-OPS-01`；
- 标注“全量键/默认值以代码/testdata 为准；本文件只做导航”。

### 7.3 `store.md`

- 从 `internal/interfaces/store.go` 冻结注释 + 02a 已批准语义派生；
- 包含 BeginAccepted/Finish/Lookup/LastSeq/LoadDevices/SaveDevices/LoadSnapshot/SaveSnapshot/DeleteExpired/Close；
- 02b 未批准补充标 `pending`；不写 A/B 选择。

### 7.4 `testing.md` / `traceability.md`

- L1–L4、假时钟、bbolt 崩溃、模拟器、数值判据；
- 合同锚点格式：`EXT-CODE-01`、`CFG-V73`、`STORE-CLOSE-01`、`SIM-MANIFEST-01`；
- 测试标注：`// Covers: REQ-INVOKE-01, EXT-CODE-01`；
- traceability 状态：`covered` / `planned(ticket)` / `not-applicable`；
- CI 只失败于“无归属且无计划”。

---

## 8. 版本控制与漂移检测

### 8.1 规范包版本

- `docs/spec/VERSION` 为 **规范包版本**（spec + contracts）；
- `CHANGELOG.md` 记录 REQ 与合同锚点变更；
- semver：编辑 patch、增补 minor、破坏 major；
- v2 → v3 为 major；
- spec 头部 `Status: proposed`，用户批准合并后改 `accepted`。

### 8.2 变更流程

- 规范变更 PR **只改** `docs/spec/**`、`docs/contracts/**`、`VERSION`、`CHANGELOG`；
- 代码/测试在后续 PR，引用 REQ/合同锚点；
- 不允许同一 PR 混合规范与代码（bootstrap PR 用 `spec-bootstrap` 豁免）；
- feature branch 不写规范。

### 8.3 CI 检查（合并为一个脚本）

`scripts/spec-check.sh` 子命令：

| 子命令 | 检查 |
|---|---|
| `lint` | spec 无决策/评审编号、理由、ticket 编号、完整代码/配置/JSON、旧标题空壳；无与 AGENTS/ADR/contracts 重复的规范段落 |
| `version` | spec/contracts 改了但 VERSION/CHANGELOG 没变 |
| `contracts` | config 与 testdata/代码；external code/event 在实现后启用；store 与 interfaces |
| `traceability` | REQ/锚点无归属且无计划 |
| `refs` | plans/tickets/新文档不得出现裸 `spec §N`、决策编号、archive 路径 |
| `links` | 相对链接目标；历史文件在允许范围内 |

运行规则：

- 只检查 PR diff 的新增/修改行；
- `refs` 对 plans/tickets 逐步收紧：本 PR 直接规范化 02b/06b 及全部 plans/tickets，目标为零裸 `spec §N`；
- bootstrap PR 用 `spec-bootstrap` label 豁免；
- `fetch-depth: 0` 或显式 fetch base；
- 先并入现有 `ci` job，非 required；稳定后更新 branch protection。

### 8.4 不一致处理

- 停止合并；
- 按事实类型判定权威；
- 开 spec delta；
- 更新版本；
- 代码/测试后续 PR 引用 REQ；
- 不允许静默漂移。

---

## 9. 跨分支 / worktree 同步

### 9.1 工件

- `docs/spec/VERSION`
- `scripts/spec-status.sh`
- `scripts/spec-impact.sh`
- `scripts/spec-archive.sh`
- PR 模板
- `AGENTS.md` 新章节

### 9.2 检查点

```bash
git fetch origin
git merge origin/main
make spec-status
```

1. 计划评审前；2. 开始实现前；3. 提交 PR 前。

### 9.3 `spec-status.sh`

- 用 merge-base 判断是否落后；
- 比较规范包版本；
- 检查分支是否修改 `docs/spec/**` 或 `docs/contracts/**`；
- `spec-delta/*` 允许修改规范；其他分支修改则 exit 2；
- 扫描分支引用的 REQ/合同锚点是否被改。

### 9.4 `spec-impact.sh`

- 从 CHANGELOG 提取变更 REQ + 合同锚点；
- 扫描本地分支、worktree、tickets/plans；
- 输出受影响项。

### 9.5 CI 门禁

- 分支落后 main：失败；
- feature branch 改规范：失败；
- `spec-delta/*` 无版本/CHANGELOG：失败；
- `spec-delta/*` 同时改代码：失败；
- 代码 PR 声明 normative 但无 spec delta：失败；
- plans/tickets 出现裸 `spec §N`：失败；
- 新 ticket/plan 引用不存在的 REQ/锚点：失败。

### 9.6 AGENTS 规则

1. main 是规范唯一写入点；
2. feature branch 不写 spec/contracts；
3. 规范变更开 `spec-delta/<topic>`；
4. 三个检查点运行 `make spec-status`；
5. PR 写 `Spec-Version` / `Spec-Impact` / `REQ`；
6. 分支携带规范修改不得合并；
7. `.scratch/` 只放 tickets/plans；当前规范在 `docs/spec/`，合同在 `docs/contracts/`；
8. AGENTS 不重复 spec 行为规则，只保留流程/约束并链接。

---

## 10. Plans / Tickets 直接规范化

### 10.1 范围

- `.scratch/collector/issues/**`
- `.scratch/collector/plans/**`
- `.scratch/collector/diffs/**`
- `.scratch/collector/ticket-dependency.html`
- `docs/代码走读/**` 中的 spec 引用
- 02b、06b 全部在范围内。

### 10.2 规则

| 可改 | 不可改 |
|---|---|
| `spec §2` → `REQ-*` / contract 锚点 | Status、验收勾选、日期、评论、决策 |
| 旧决策编号 → `ADR-*` | 代码/配置块、被引用原文 |
| 旧 spec 路径 → 新路径 | 历史事实错误（保留原样） |

### 10.3 02b 特殊处理

- plan 中 `spec §3` 目标改为 `docs/contracts/store.md`；
- 不改变 A/B 选项、不决定 A/B、不把未决语义写成契约；
- ticket 保持 `on-hold`；
- 若某处引用更新必须依赖 A/B 决策，停止并保留原文，另发 02b plan 修订。

### 10.4 06b 特殊处理

- plan/ticket 中 `spec §7.1` 改为 `docs/contracts/simulator.md`；
- ticket 状态 `done`、验收、提交哈希不变；
- 不做实现修改。

### 10.5 语义 diff 证明

1. 只替换等价引用/路径/锚点；
2. 对每个文件生成掩码 diff：抹掉被替换 token 后，剩余文本逐字节一致；
3. 任何未命中替换规则的行不得变化；
4. 状态、验收、日期、评论、代码块做哈希校验；
5. 若某文件无法满足，停止并升级为 plan 修订，不做映射契约。

---

## 11. 产品定型与架构

### 11.1 保留

- §1 定位、§2 抽象依据、§3 目标/非目标、§4 角色/部署、§5 场景、§8 功能清单、§11 非功能、§13 风险、§14 路线。

### 11.2 移除/链接

| 位置 | 处理 |
|---|---|
| §5.3/§5.5 规范语句 | 改为链接 spec `REQ-QUALITY-04` / `REQ-OPS-01` |
| §6.1–6.3 接口 | 删除，链接 `internal/interfaces` |
| §6.3.1 调度 | 删除，链接 ADR-0003 |
| §6.4 组件 | 移到 `docs/architecture.md` |
| §6 依赖图 | 保留高层图或移到 architecture |
| §7 包布局 | 移到 `docs/architecture.md` |
| §9 D 表 | `docs/adr/README.md` 决策索引 |
| §10 报文 | 删除，链接 external |
| §12 验收 | 删除，链接 testing |

### 11.3 事实修正

- `:81` links 注释；
- `:90` cache/ 目录；
- `:394` `internal/core/store` → `internal/store/bbolt`；
- `:561-571` code 表 → 链接/补 5042–5044；
- `:582-594` 事件表 → 补 `valve_fault_clear`；
- `:608` Go 精确版本；
- `:614` 24h 真实/假时钟；
- `:3` 状态与规范范围。

### 11.4 D 决策索引

`docs/adr/README.md` 逐项列出 D1–D28：

- 有 ADR 的 → ADR；
- 无 ADR 的 → 一行决策 + 当前 REQ/contract，或标“历史，不作当前权威”；
- PRD §9 删除前必须每项有落点。

---

## 12. 分支与提交计划

### 12.1 分支

- 工作分支：`docs/spec-v3`（从 `main@461121c`）
- 最终 PR；required checks `ci`、`gitleaks`。

### 12.2 提交顺序

1. `plan: spec-v3 计划 v3（按用户修正）`（本文件）
2. `docs(agents): 定义 spec/contracts 位置与阅读路径，并去除与 spec 的重复`
   - `AGENTS.md`
   - `docs/agents/issue-tracker.md`
   - `docs/agents/domain.md`
3. `docs(adr): 新增 ADR-0006 与 ADR-0004 Amendment`
4. `docs: 新增文档地图、ADR 索引、D 决策索引`
5. `docs: tag spec-v2.0.0 + 新增 spec v3 + VERSION/CHANGELOG`
6. `docs: 新增 external/config/simulator/testing/traceability/architecture 契约`
7. `docs: 清理产品定型`
8. `docs: 直接规范化所有 plans/tickets 的 spec 引用（含 02b/06b）`
9. `ci: 新增 spec-check/spec-status/spec-impact/spec-archive、Makefile、PR 模板、workflow`
10. `chore: 退役 06b worktree/branch`

### 12.3 tag 与归档

```bash
git tag -a spec-v2.0.0 <v2-parent-commit> -m "spec v2.0.0 frozen before v3"
git push origin spec-v2.0.0
```

`scripts/spec-archive.sh`：

```bash
git fetch --tags
git show spec-v2.0.0:.scratch/collector/spec.md
```

不保留工作区全文副本；浅克隆审计必须先 fetch tag。

---

## 13. 验收标准

### 13.1 结构

- [ ] `docs/spec/spec.md` v3.0.0，`Status: proposed`；用户批准后改 accepted。
- [ ] `VERSION`、`CHANGELOG.md` 存在且一致。
- [ ] spec 无决策/评审编号、理由、ticket 编号、完整代码/配置/JSON、旧标题空壳。
- [ ] v2 迁移矩阵逐条有归属；无未映射条目。
- [ ] contracts external/config/store/simulator 存在。
- [ ] testing/traceability/architecture/README/ADR README 存在。
- [ ] PRD 无 Go/JSON/YAML 代码块、无重复规范、无 D 表正文。

### 13.2 兼容与规范化

- [ ] 06b 实现/ticket 状态/验收/评论不变；§7.1 已移到 simulator contract；plan/ticket 引用已改。
- [ ] 02b 仍 `on-hold`；A/B 未被决定；plan 的 `spec §3` 已改为 store contract；ticket 原文语义不变。
- [ ] 01a/01b/01c/02a 的验收、评论、提交哈希不变；引用已更新。
- [ ] 所有 plans/tickets 无裸 `spec §N`。
- [ ] 不新增 `refs-migration-log` 或其他兼容映射契约。
- [ ] 语义 diff 通过：状态/验收/日期/评论/代码块不变。
- [ ] `cmd/**`、`internal/**`、`go.mod`、`go.sum`、testdata 无变更。
- [ ] ADR-0004 原文不变，Amendment 完整。

### 13.3 去重与控制

- [ ] spec 与 AGENTS/ADR/contracts/testing 无重复规范段落。
- [ ] AGENTS 只保留流程/约束，行为细节链接 spec/ADR。
- [ ] `spec-check.sh` 六项检查可在 bootstrap PR 通过（豁免）。
- [ ] `make spec-status` 可检测落后/改规范/REQ 变更；`spec-delta` 例外。
- [ ] `spec-impact` 覆盖 REQ + 合同锚点。
- [ ] PR 模板含 `Spec-Version` / `Spec-Impact` / `REQ`。
- [ ] tag `spec-v2.0.0` 已推送，`spec-archive.sh` 可读取。

### 13.4 构建

- [ ] `make ci` 全绿。
- [ ] 独立模型复审通过。
- [ ] 用户明确“计划评审通过”后才实现。

---

## 14. 风险与回滚

| 风险 | 缓解 |
|---|---|
| v2 迁移遗漏 | 迁移矩阵 + 覆盖门禁 + 独立复审 |
| 02b 未拍板决策被冻结 | 只改目标路径，不决定 A/B；store.md 标 pending |
| plans/tickets 改引用改变语义 | 语义 diff + 逐文件理由；无法保持则停止 |
| spec 与 AGENTS 重复 | 去重矩阵 + spec-check lint + 独立复审 |
| CI bootstrap 失败 | PR diff 范围 + allowlist + `spec-bootstrap` 豁免 |
| 合同与代码漂移 | 声明源方向；config 导航、external 后续生成/校验 |
| 06b worktree 旧 spec | 启用门禁前退役 |
| tag 不可见 | 显式创建/推送 + `spec-archive.sh` |
| PRD 清理丢决策 | D 决策索引逐项落点 |

回滚：所有变更在 `docs/spec-v3` 分支，PR 未合并前整体可回退；合并后按提交 revert。

---

## 15. 执行前置条件

1. 本计划通过独立模型复审；
2. 用户明确“计划评审通过”；
3. 06b 已合并（已完成）；
4. 02b 保持 `on-hold`；
5. 所有实现提交经 PR，不直接 push main。
