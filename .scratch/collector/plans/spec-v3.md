# Spec 管理化方案落地计划（v4）

**状态**：待用户评审（2026-11-22；v4 按第二轮独立复审修订）
**基础**：`main@461121c`（PR #6 / 06b 已合并；PR #9 / 02b plan 已合并）
**工作分支**：`docs/spec-v3`
**范围**：文档治理、spec v3、契约拆分、02b/06b 文档纳入、plans/tickets 前向引用规范化、PRD 清理、CI 约束
**非目标**：不修改产品行为、接口签名、配置字段、错误码、事件语义；不修改 06b 已合并实现；不恢复 02b 实现；不替用户拍板 02b 的 A/B。

---

## 0. 用户修正与中心原则

### 0.1 用户修正

1. **02b、06b 在修改范围内**：plan/ticket/文档必须按新 spec 规范调整。
2. **plans/tickets 必须调整**：只要语义不变、不与代码实现冲突，就必须改为新锚点。

### 0.2 中心原则

1. **不做兼容映射契约**：
   - 不引入 `refs-migration-log` 这类常驻解析层；
   - 不保留旧标题空壳；
   - 前向引用直接改为新锚点；
   - 历史事实/用户决策保留原文并标记，不“改写历史”；
   - 无法语义不变地改时，停止并走 plan 修订。
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
| 06b spec 改动 | `.scratch/collector/spec.md:621-710` 的 `#### 7.1 06b 冻结的 MQTT broker + 云端桩契约` |
| 06b worktree | `.claude/worktrees/06b` 仍存在；分支 `feat/06b-mqtt-broker-stub` 仍存在 |
| 06b plan | `.scratch/collector/plans/06b.md`，含 `spec §7.1` 引用 |
| 02b | plan 已并入 main；ticket `on-hold`；实现暂停；plan v2 待用户评审 |
| 02b 未决 | plan §4.5/§4.12/§4.13 的 A/B 尚未拍板；§10.8 依赖这些选择 |
| 02b plan | `.scratch/collector/plans/02b.md`，含 `spec §3` 追加指令 |
| 旧 spec | `.scratch/collector/spec.md`，状态 `v2-draft`，含实现细节、评审编号、ticket 编号 |
| 产品定型 | `docs/产品定型.md` 含旧接口、报文副本、D 决策表 |
| main 保护 | 必须经 PR；required checks `ci`、`gitleaks` |

---

## 2. 最终方案

1. **spec 版本化且受控**：
   - 当前规范包：`docs/spec/spec.md` + `docs/contracts/**`；
   - 包版本：`docs/spec/VERSION`（纯 `3.0.0`）；
   - v2-draft 用 tag `spec-v2-draft` 冻结；**删除工作区旧 spec**；
   - `docs/spec/CHANGELOG.md` 记录 REQ 与合同锚点变更；
   - semver：编辑 patch、增补 minor、破坏 major。
2. **spec 只放规范**：
   - 每条规范有稳定 `REQ-*` ID；
   - 不写理由、评审建议、决策编号、被否决方案、ticket 编号、实现历史、完整代码/配置/JSON。
3. **按事实类型分权**：
   - 接口签名 → `internal/interfaces`
   - 行为 → `docs/spec/spec.md` 的 `REQ-*`
   - 外部 Topic/code/event → `docs/contracts/external.md`
   - 配置键/默认值 → `internal/config/types.go` + `defaults.go` + `rules.go`；config.md 只导航
   - Store 语义 → `docs/contracts/store.md`（自包含；签名以 `interfaces.Store` 为准）
   - 模拟器契约 → `docs/contracts/simulator.md`
   - 决策/理由 → `docs/adr/**`
   - 术语 → `GLOSSARY.md`
   - 产品意图 → `docs/产品定型.md`
   - 架构/包布局 → `docs/architecture.md`
   - 单票验收 → ticket
   - 历史 → plans / tag
4. **兼容不污染规范，也不做映射层**：
   - 不保留旧标题空壳；
   - 前向引用直接改新锚点；
   - 历史事实/用户决策保留原文并标记 `(historical)`；
   - 无法语义不变地改时停止，另发 plan 修订。
5. **新工作最小上下文**：
   - 新 ticket 只允许 `REQ-*`、合同锚点、`ADR-*`；
   - 禁止裸 `spec §N`、决策编号、archive 路径、旧 plan 路径；
   - `docs/README.md` 定义当前/历史阅读路径；
   - `docs/agents/domain.md` 默认不读 archive/旧 plan。
6. **跨分支/worktree 同步**：
   - `VERSION` + `spec-check.sh` + PR 模板 + CI；
   - 分支落后 main、feature branch 改规范、规范包变更无版本，均失败。
7. **06b / 02b**：
   - 06b §7.1 → `docs/contracts/simulator.md`；06b plan/ticket 的前向引用同步更新；
   - 02b plan 的 `spec §3` 目标改为 `docs/contracts/store.md`；A/B 仍不拍板；解冻流程见 §6.1。

---

## 3. 不可破坏的兼容性约束

| 编号 | 约束 |
|---|---|
| C1 | 06b 已合并实现不改；ticket 状态/验收/提交哈希不改；只更新前向引用与 spec 归属 |
| C2 | 02b 保持 `on-hold`；不实现、不勾选、不标 done；不把未拍板 A/B 写成契约 |
| C3 | 01a/01b/01c/02a/06b 的验收、评论、提交哈希不改 |
| C4 | plans/tickets 的前向引用必须改为新锚点；历史事实/用户决策保留并标记；不建映射契约 |
| C5 | 不修改 `cmd/**`、`internal/**`、`go.mod`、`go.sum`、`internal/config/testdata/**`；允许修改 `Makefile`、`.github/**`、`scripts/spec-*` |
| C6 | 不修改 `docs/需求文档_*.md`、`docs/缺口台账.md` 内容 |
| C7 | ADR 原文不改写；ADR-0003/0004/0005 用 Amendment 声明“规范正文以 spec/contract 为准” |
| C8 | spec v3 不保留旧标题空壳 |
| C9 | 所有变更经 PR 进入 main；禁止直接 push main |
| C10 | 06b worktree/branch 在启用门禁前退役（执行前置检查，不是 PR 提交） |
| C11 | spec 不与 AGENTS/ADR/contracts/testing 重复同一规范内容 |
| C12 | 任何文档修改若会改变语义，停止并走 plan 修订 |
| C13 | tag 后删除 `.scratch/collector/spec.md`；仓库只保留一个当前 spec |
| C14 | 计划评审通过只授权“写文档”；spec v3 内容需独立用户批准后才改 `accepted` |

### 3.1 基线与语义 diff

```bash
git rev-parse HEAD > /tmp/spec-v3-base.txt
sha256sum cmd/** internal/** go.mod go.sum \
          docs/需求文档_*.md docs/缺口台账.md \
          > /tmp/spec-v3-code.sha
```

plans/tickets 允许引用变化，但必须逐文件证明：
- Status、验收勾选、日期、评论、代码块、被引用原文不变；
- 只改前向引用锚点/路径；
- 历史事实/用户决策保留并标记 `(historical)`；
- 无法证明的，停止并升级 plan 修订。

---

## 4. 目标文档结构

| 文档 | 唯一职责 |
|---|---|
| `docs/README.md` | 文档地图、按事实分权、当前/历史阅读路径 |
| `docs/spec/spec.md` | 行为契约（`REQ-*`） |
| `docs/spec/VERSION` | 规范包 semver（纯 `3.0.0`） |
| `docs/spec/CHANGELOG.md` | REQ + 合同锚点变更 |
| `docs/contracts/external.md` | Topic、JSON、code、事件、seq |
| `docs/contracts/config.md` | 配置导航、四层关系、校验分类 |
| `docs/contracts/store.md` | Store 规范语义（自包含；签名链接 interfaces） |
| `docs/contracts/simulator.md` | 模拟器契约（06b §7.1 + 非目标） |
| `docs/testing.md` | L1–L4、假时钟、bbolt、模拟器、数值判据 |
| `docs/traceability.md` | REQ/合同锚点 → test/ticket |
| `docs/architecture.md` | 组件、依赖方向、包布局 |
| `docs/adr/README.md` | ADR 索引 + D 决策索引（非映射层） |
| `docs/adr/0006-doc-authority-and-spec-change-policy.md` | 文档权威、版本、变更控制 |
| `.scratch/collector/issues/**` | 单票验收（前向引用更新，历史事实保留） |
| `.scratch/collector/plans/**` | 计划历史（前向引用更新，历史事实保留） |

**删除** `.scratch/collector/spec.md`（tag `spec-v2-draft` 后）。

### 4.1 按事实类型分权

| 事实类型 | 权威 | 说明 |
|---|---|---|
| 接口签名/冻结注释 | `internal/interfaces`（尤其 `store.go`） | spec/contract 只链接，不复制签名 |
| 行为语义 | `docs/spec/spec.md` `REQ-*` | 唯一规范来源 |
| 外部报文 | `docs/contracts/external.md` | 含 code/event 表 |
| 配置键/默认值/校验 ID | `internal/config/types.go` + `defaults.go` + `rules.go` | testdata 只是示例 |
| Store 规范语义 | `docs/contracts/store.md` | 自包含；02b pending 明确标注 |
| 模拟器契约 | `docs/contracts/simulator.md` | 含非目标 |
| 决策/理由 | `docs/adr/**` | 不重抄规范正文 |
| 术语 | `GLOSSARY.md` | 唯一 |
| 产品意图 | `docs/产品定型.md` | 规范语句链接 spec |
| 架构/包布局 | `docs/architecture.md` + `check-deps` | 不写进 spec |
| 单票验收 | ticket | 不改语义 |
| 历史 | plans / tag | 前向引用更新，历史事实标记 |

### 4.2 Spec 去重

| 内容 | 唯一归属 | spec 只做 |
|---|---|---|
| 流程/交付/版本控制规则 | `AGENTS.md` | 不写流程 |
| 决策理由/被否决方案 | ADR | 不写理由 |
| Topic/JSON/code 表/事件枚举表 | `external.md` | 链接；只写事件触发/去重语义 |
| 四层 YAML/键/默认值 | `types.go/defaults.go/rules.go` + `config.md` 导航 | 链接/摘要 |
| Store 细节 | `store.md` | 链接 |
| 模拟器 API/flags | `simulator.md` | 链接；`REQ-CLI-01` 只写通用子命令 |
| 测试分层/命令/数值判据 | `testing.md` | 链接/验收摘要 |
| 术语定义 | `GLOSSARY.md` | 使用术语，不重定义 |

去重 lint 只做可执行检查：
- spec 禁止完整 JSON/YAML/Go 代码块；
- spec 禁止 `D/P/Q/S/W/N` 决策编号；
- spec 不得出现与 external/store/simulator 相同的表头/标题；
- 语义重复由复审清单兜底，不声称 CI 能自动判断。

---

## 5. Spec v3 结构

```text
docs/spec/spec.md

# Spec v3.0.0：采集引擎行为契约

- Status: proposed（用户批准 spec 内容后改 accepted）
- Version: 3.0.0
- Supersedes: v2-draft（tag spec-v2-draft）
- Normative: §1–§9
- Change log: docs/spec/CHANGELOG.md

1. 目的与范围                         REQ-SCOPE-01
2. 系统不变量
   REQ-TIME-01        时间只来自 Clock
   REQ-SEAM-01        三种扩展点只经 interfaces
   REQ-SEAM-02        Store 只经 interfaces.Store；bbolt 不在 core
   REQ-QUALITY-01     最终质量只由内核计算
   REQ-QUALITY-02     last_success_at 只由新鲜观测推进
   REQ-QUALITY-03     质量真值表（OK/Partial/Failed/Unsupported/解码失败）
   REQ-QUALITY-04     离线判定与恢复（online + 全量快照）
   REQ-QUALITY-05     边界用严格大于（详细 ±1ns 测试在 testing.md）
3. 采集调度
   REQ-SCHED-01       相位锁定、不补跑
   REQ-SCHED-02       单设备 deadline、missed_tick
   REQ-SCHED-03       链路内串行、互不拖死
   REQ-SCHED-04       设备采集周期/链路 tick/节点上报周期关系
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
   REQ-EVENT-01       事件触发与去重语义（枚举表在 external.md）
   REQ-EVENT-02       episode 与 dedup_key
   REQ-EVENT-03       事件与属性两条通道
   REQ-UPLINK-01      ChirpStack 上行去重（deduplicationId / devEui+fCnt，内存窗口 1024）
7. 外部契约摘要
   链接 docs/contracts/external.md
8. 运维与 CLI
   REQ-OPS-01         SIGHUP 差别重载
   REQ-OPS-02         健康端点与指标
   REQ-OPS-03         日志字段与脱敏
   REQ-CLI-01         通用子命令契约（simulate flags 在 simulator.md）
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

## 6. v2 迁移矩阵（逐条，未覆盖不得合并）

| 旧位置 | 内容 | 新归属 |
|---|---|---|
| `spec.md:10-19` Problem | 两类现场、共同内核 | PRD §1/§2；spec §1 |
| `spec.md:21-32` Solution | 南向/内核/北向/物模型 | PRD §2；`docs/architecture.md` |
| `spec.md:34-45` In Scope | v0.1–v1.0 范围 | PRD §3；`REQ-SCOPE-01` |
| `spec.md:47-94` User Stories | 行为/测试/交付故事 | 行为 → REQ/ticket；测试 → testing；交付 → PRD |
| `spec.md:98-310` §1 配置 | 四层 YAML、默认值、示例 | `config.md` 导航 + `types/defaults/rules`；热重载 → `REQ-OPS-01` |
| `spec.md:309` 周期归属 | 设备周期/链路 tick/上报周期 | `REQ-SCHED-04` |
| `spec.md:311-322` §2 校验 | 规则分类、V70–V73 | `config.md` 分类 + `rules.go` |
| `spec.md:324-455` §3 接口 | Go 签名、Clock/Store 缝隙 | `internal/interfaces`；语义 → `REQ-TIME-01`、`REQ-SEAM-01/02` |
| `spec.md:456-546` §4 报文 | Topic/JSON/code/事件 | `external.md`；事件语义 → `REQ-EVENT-*` |
| `spec.md:547-564` §5.1 质量 | 真值表 | `REQ-QUALITY-01..05` |
| `spec.md:565-581` §5 可靠性 | 幂等/重试/队列/缓存/上行去重 | `REQ-INVOKE-*`、`REQ-QUEUE-*`、`REQ-CACHE-*`、`REQ-UPLINK-01`；理由 → ADR |
| `spec.md:582-592` §5.2 健康 | 端点/指标 | `REQ-OPS-02` |
| `spec.md:593-602` §6 子命令 | run/config-check/simulate/... | `REQ-CLI-01` |
| `spec.md:604-619` §7 模拟器 | 能力要求、端点 | `simulator.md`；能力 → `REQ-SIM-01` |
| `spec.md:621-710` §7.1 06b 契约 | broker/manifest/API | `simulator.md`（保留非目标） |
| `spec.md:711-754` Testing | L1–L4/假时钟/稳定性 | `docs/testing.md` |
| `spec.md:755-765` Out of Scope | 非目标 | spec §9；PRD §3 |
| `spec.md:766-771` Further Notes | 路线/交付/验收判据 | PRD §14；testing/tickets |
| `产品定型.md:213-353` §6.1–6.3 | 旧接口代码 | 删除，链接 `internal/interfaces` |
| `产品定型.md:355-369` §6.3.1 | 调度算法 | ADR-0003（Amendment 声明规范正文在 spec） |
| `产品定型.md:371-380` §6.4 | 组件职责 | `docs/architecture.md` |
| `产品定型.md:384-402` §7 | 包布局 | `docs/architecture.md` + `check-deps` |
| `产品定型.md:432-481` §9 | D 表/被否决 | `docs/adr/README.md` 决策索引（非映射表） |
| `产品定型.md:485-595` §10 | 报文 | `external.md` |
| `产品定型.md:612-622` §12 | 验收 | `docs/testing.md` |

**覆盖门禁**：逐条勾选，不以粗区间替代；任何未映射条目必须写“删除理由”。

### 6.1 02b 解冻流程（新治理下）

1. 用户对 02b plan §4.5/§4.12/§4.13 的 A/B 拍板；
2. 从 main 开 `spec-delta/02b-store`：
   - 更新 `docs/contracts/store.md`；
   - 更新 `VERSION` / `CHANGELOG`；
   - 不改代码；
3. 用户评审 spec-delta；
4. 合并 spec-delta；
5. 02b ticket 解冻：
   - merge main；
   - 引用已更新的 store contract；
   - 实现提交不再修改 spec/contract；
6. 02b plan 的实现提交项改为“引用 spec-delta PR”；ticket comments 同步；
7. A/B 未拍板前，02b 保持 `on-hold`，`store.md` 标 `Pending 02b plan approval`。

### 6.2 06b 的契约抽取

`docs/contracts/simulator.md` 包含：

- CLI flags；
- manifest schema；
- captured/invoke/control；
- `broker_disconnect`；
- 安全与独立客户端测试；
- 当前版本非目标：不提供 `/_requests`、ChirpStack fixture、`--config`、生产 MQTT Channel；
- 模拟器独立性：不加载生产配置、不依赖硬件假设、不 import 被测实现。

06b plan/ticket 的 `spec §7.1` 前向引用改为 `simulator.md`；ticket 状态/验收/提交哈希不变。

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

**不复制全量键**：

- 四层文件与引用关系；
- 校验分类；
- 示例指向 testdata（仅示例）；
- 权威：`types.go`（键）、`defaults.go`（默认值）、`rules.go`（校验 ID）；
- 热重载矩阵链接 `REQ-OPS-01`；
- 标注“全量键/默认值以代码为准；本文件只做导航”。

### 7.3 `store.md`

- 规范语义自包含；
- 签名/冻结注释链接 `internal/interfaces/store.go`；
- 包含 BeginAccepted/Finish/Lookup/LastSeq/LoadDevices/SaveDevices/LoadSnapshot/SaveSnapshot/DeleteExpired/Close；
- 02b 未批准补充标 `pending`；不写 A/B 选择；
- 02a/02b plans 是 provenance/history，不是新工作必读权威。

### 7.4 `testing.md` / `traceability.md`

- L1–L4、假时钟、bbolt 崩溃、模拟器、数值判据；
- 合同锚点格式：`EXT-CODE-01`、`CFG-V73`、`STORE-CLOSE-01`、`SIM-MANIFEST-01`；
- 测试标注：`// Covers: REQ-INVOKE-01, EXT-CODE-01`；
- traceability 状态：`covered` / `planned(ticket)` / `not-applicable`；
- CI 只失败于“无归属且无计划”。

---

## 8. 版本控制与漂移检测

### 8.1 规范包版本

- `docs/spec/VERSION` 为规范包版本（纯 `3.0.0`）；
- `CHANGELOG.md` 记录 REQ 与合同锚点变更；
- semver：编辑 patch、增补 minor、破坏 major；
- v2-draft → v3 为 major；
- spec 头部 `Status: proposed`；用户批准 spec 内容后改 `accepted`。

### 8.2 变更流程

- 规范变更 PR **只改** `docs/spec/**`、`docs/contracts/**`、`VERSION`、`CHANGELOG`；
- 代码/测试在后续 PR，引用 REQ/合同锚点；
- 不允许同一 PR 混合规范与代码（bootstrap PR 用 `spec-bootstrap` 豁免）；
- feature branch 不写规范。

### 8.3 CI 检查

`scripts/spec-check.sh` 子命令：

| 子命令 | 检查 |
|---|---|
| `lint` | spec 无决策/评审编号、理由、ticket 编号、完整代码/配置/JSON、旧标题空壳；无与 external/store/simulator 相同表头/标题 |
| `version` | spec/contracts 改了但 VERSION/CHANGELOG 没变 |
| `contracts` | config 与 types/defaults/rules；external code/event 在实现后启用；store 与 interfaces |
| `traceability` | REQ/锚点无归属且无计划 |
| `refs` | 只检查“前向引用”：plans/tickets 中未标记 `(historical)` 的裸 `spec §N`；忽略 fenced code / policy-example |
| `links` | 相对链接目标 |

运行规则：

- 只检查 PR diff 的新增/修改行；
- 历史事实/用户决策允许 `(historical)` 标记；refs 不要求历史编号归零；
- `spec-v3.md` 中的策略示例允许 `policy-example` 标记/allowlist；
- bootstrap PR 用 `spec-bootstrap` label，但合并前必须用同一脚本对目标状态跑一次并贴结果；
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
- `scripts/spec-impact.sh`（本地工具）
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

- 本地工具，扫描本地分支、worktree、tickets/plans；
- CI 不依赖它；CI 只做 merge-base/版本检查；
- 从 CHANGELOG 提取变更 REQ + 合同锚点。

### 9.5 CI 门禁

- 分支落后 main：失败；
- feature branch 改规范：失败；
- `spec-delta/*` 无版本/CHANGELOG：失败；
- `spec-delta/*` 同时改代码：失败；
- 代码 PR 声明 normative 但无 spec delta：失败；
- plans/tickets 未标记历史的前向 `spec §N`：失败；
- 新 ticket/plan 引用不存在的 REQ/锚点：失败。

### 9.6 AGENTS 规则

1. main 是规范唯一写入点；
2. feature branch 不写 spec/contracts；
3. 规范变更开 `spec-delta/<topic>`；
4. 三个检查点运行 `make spec-status`；
5. PR 写 `Spec-Version` / `Spec-Impact` / `REQ`；
6. 分支携带规范修改不得合并；
7. `.scratch/` 只放 issues/plans；当前规范在 `docs/spec/`，合同在 `docs/contracts/`；
8. AGENTS 不重复 spec 行为规则，只保留流程/约束并链接。

---

## 10. Plans / Tickets 规范化

### 10.1 引用分类

| 类型 | 处理 |
|---|---|
| **前向引用**（当前规范/合同指针） | 直接改新锚点 |
| **历史事实 / 用户决策**（某次提交改了什么、当时选了什么） | 保留原文，行内标 `(historical)` 或文件头说明；refs 检查排除 |
| **不确定** | 停止，走 plan 修订 |

### 10.2 范围

- `.scratch/collector/issues/**`
- `.scratch/collector/plans/**`
- `.scratch/collector/diffs/**`
- `.scratch/collector/ticket-dependency.html`
- `docs/代码走读/**` 中的前向引用
- 02b、06b 全部在范围内。

### 10.3 02b 特殊处理

- 前向引用 `spec §3` → `docs/contracts/store.md`；
- 历史决策/未决 A/B 保留，标 `(historical)` / `pending`；
- 不决定 A/B；
- 解冻流程见 §6.1；
- ticket 保持 `on-hold`。

### 10.4 06b 特殊处理

- 前向引用 `spec §7.1` → `docs/contracts/simulator.md`；
- ticket 状态 `done`、验收、提交哈希不变；
- 历史事实保留并标 `(historical)`。

### 10.5 语义 diff 证明

1. 只替换前向引用/路径/锚点；
2. 对每个文件生成掩码 diff：抹掉被替换 token 后，剩余文本逐字节一致；
3. 任何未命中替换规则的行不得变化；
4. Status、验收、日期、评论、代码块做哈希校验；
5. 无法满足的，停止并升级为 plan 修订，不做映射契约。

### 10.6 提交拆分

按组拆分，避免巨型 diff：

- `docs(refs): 01x plans/tickets`
- `docs(refs): 02x plans/tickets（含 02b）`
- `docs(refs): 06b plans/tickets`
- `docs(refs): 其他 plans/diffs/走读`
- 大 diff 写说明文档（AGENTS 版本控制要求）。

---

## 11. 产品定型与架构

### 11.1 保留

- §1 定位、§2 抽象依据、§3 目标/非目标、§4 角色/部署、§5 场景、§8 功能清单、§11 非功能、§13 风险、§14 路线。

### 11.2 移除/链接

| 位置 | 处理 |
|---|---|
| §5.3/§5.5 规范语句 | 改为链接 spec `REQ-QUALITY-04` / `REQ-OPS-01` |
| §6.1–6.3 接口 | 删除，链接 `internal/interfaces` |
| §6.3.1 调度 | 删除，链接 ADR-0003（Amendment 声明规范正文在 spec） |
| §6.4 组件 | 移到 `docs/architecture.md` |
| §6 依赖图 | 保留高层图或移到 architecture |
| §7 包布局 | 移到 `docs/architecture.md` |
| §9 D 表 | `docs/adr/README.md` 决策索引（非映射表） |
| §10 报文 | 删除，链接 external |
| §12 验收 | 删除，链接 testing |

### 11.3 事实修正

- `:81` links 注释；
- `:90` cache/ 目录；
- `:394` `internal/core/store` → `internal/store/bbolt`；
- `:561-571` code 表 → 链接；**已有 5042，补 5043–5044**；
- `:582-594` 事件表 → 补 `valve_fault_clear`；
- `:608` Go 精确版本；
- `:614` 24h 真实/假时钟；
- `:3` 状态与规范范围。

### 11.4 D 决策索引

`docs/adr/README.md` 只列：

- D 编号 + 一行决策；
- ADR 落点；
- `current` / `historical` / `superseded`。

不维护“旧 ID → 新 REQ”的逐项解析表；需要当前行为时直接读 spec/contract。

---

## 12. AGENTS 与 agent docs 精确改动

### 12.1 `AGENTS.md`

- Issue tracker 段改为：
  > Issues and plans live under `.scratch/<feature-slug>/`. The current behavior spec is `docs/spec/spec.md`; contracts are under `docs/contracts/`.
- Domain docs 段改为：读取 `docs/README.md`、`docs/spec/spec.md`、`docs/contracts/`、`GLOSSARY.md`、`docs/adr/`。
- 时间硬约束从“完整函数清单”改为一行规则 + `REQ-TIME-01` / ADR-0003 链接；不重复规范正文。
- 新增分支同步规则（§9.6）。

### 12.2 `docs/agents/issue-tracker.md`

- 删除 “The spec is `.scratch/<feature-slug>/spec.md`”；
- 改为：
  > The current behavior spec is `docs/spec/spec.md`. `.scratch/<feature-slug>/` holds issues and plans only.

### 12.3 `docs/agents/domain.md`

Before exploring 增加：

- `docs/README.md`
- `docs/spec/spec.md`
- `docs/contracts/`（按领域读取）
- `GLOSSARY.md`
- `docs/adr/`

默认不读 archive/旧 plans。

---

## 13. 分支与提交计划

### 13.1 分支

- 工作分支：`docs/spec-v3`（从 `main@461121c`）
- 最终 PR；required checks `ci`、`gitleaks`。

### 13.2 提交顺序

1. `plan: spec-v3 计划 v4（按第二轮复审修订）`（本文件）
2. `docs(adr): 新增 ADR-0006、ADR-0003/0004/0005 Amendment、docs/README.md`
3. `docs(agents): 定义 spec/contracts 位置与阅读路径，去重`
   - `AGENTS.md`
   - `docs/agents/issue-tracker.md`
   - `docs/agents/domain.md`
4. `docs: 新增 ADR 索引与 D 决策索引`
5. `docs(spec): tag spec-v2-draft；新增 spec v3 + VERSION/CHANGELOG；删除旧 spec`
6. `docs: 新增 external/config/store/simulator/testing/traceability/architecture`
7. `docs: 清理产品定型`
8. `docs(refs): 01x/02x/06b/其他 plans/tickets 前向引用规范化`
9. `ci: 新增 spec-check/spec-status/spec-impact/spec-archive、Makefile、PR 模板、workflow`

**执行前置/合并后检查（非提交）**：

- 退役 `.claude/worktrees/06b` 与 `feat/06b-mqtt-broker-stub`；
- 必须在启用 CI 门禁前完成；
- `git worktree remove` / `git branch -d` 不产生文件 diff，不作为 PR 提交。

### 13.3 tag 与归档

```bash
git tag -a spec-v2-draft <v2-commit> -m "spec v2-draft frozen before v3"
git push origin spec-v2-draft
```

`scripts/spec-archive.sh`：

```bash
git fetch --tags
git show spec-v2-draft:.scratch/collector/spec.md
```

不保留工作区全文副本；浅克隆审计先 fetch tag。

### 13.4 spec 内容批准门

- 计划评审通过只授权“写文档”；
- spec v3 + contracts 以 `Status: proposed` 提交；
- 用户单独评审 spec/contracts 内容；
- 用户明确批准后，单独提交 `Status: accepted`；
- 然后才合并/进入后续实现。

---

## 14. 验收标准

### 14.1 结构

- [x] `docs/spec/spec.md` v3.0.0，`Status: proposed`（accepted 待用户单独批准）。
- [x] `VERSION`、`CHANGELOG.md` 存在且一致（`VERSION` 纯 `3.0.0`）。
- [x] spec 无决策/评审编号、理由、ticket 编号、完整代码/配置/JSON、旧标题空壳。
- [x] v2 迁移矩阵逐条有归属；无未映射条目（覆盖表见 `.scratch/collector/diffs/spec-v3.md`）。
- [x] contracts external/config/store/simulator 存在。
- [x] testing/traceability/architecture/README/ADR README 存在。
- [x] 旧 `.scratch/collector/spec.md` 已删除；`git ls-files` 不再包含它。
- [x] PRD 无 Go/JSON/YAML 代码块、无重复规范、无 D 表正文。

### 14.2 兼容与规范化

- [x] 06b 实现/ticket 状态/验收/评论不变；§7.1 已移到 simulator contract；前向引用已改。
- [x] 02b 仍 `on-hold`；A/B 未被决定；plan 前向引用已改；解冻流程写入 ticket/plan。
- [x] 01a/01b/01c/02a 的验收、评论、提交哈希不变；前向引用已更新。
- [x] plans/tickets 无**未标记历史**的裸 `spec §N`；历史事实用行内 `historical` 或显式 `spec-refs: historical-file` 文件头标记。
- [x] 不新增 `refs-migration-log` 或其他兼容映射契约。
- [x] 语义 diff 通过：状态/验收/日期/提交哈希不变；02b 评论按 §6.1 同步，其余评论与代码块不变（证据见 `.scratch/collector/diffs/spec-v3.md` §2）。
- [x] `cmd/**`、`internal/**`、`go.mod`、`go.sum`、testdata 无变更。
- [x] ADR-0003/0004/0005 原文不变，Amendment 声明规范正文归属。

### 14.3 去重与控制

- [x] spec 与 AGENTS/ADR/contracts/testing 无重复规范段落。
- [x] AGENTS 只保留流程/约束，行为细节链接 spec/ADR。
- [x] `spec-check.sh` 六项检查在 bootstrap PR 通过；目标状态用同一脚本验证。
- [x] `make spec-status` 可检测落后/改规范/REQ 变更；`spec-delta` 例外（临时分支自测：普通分支改规范 exit 2，spec-delta 缺版本 exit 1，spec-delta 只改代码 exit 1）。
- [x] `spec-impact` 明确为本地工具；CI 只做 merge-base/版本检查。
- [x] PR 模板含 `Spec-Version` / `Spec-Impact` / `REQ`；至少人工检查，或 CI 解析。
- [x] tag `spec-v2-draft` 已推送，`spec-archive.sh` 可读取。

### 14.4 构建与批准

- [x] `make ci` 全绿（含 `spec-check` 与 `spec-status`）。
- [x] 第二轮复审 Blocker 与实施评审 B1–B10 已逐条处理（见 `.scratch/collector/diffs/spec-v3.md`）。
- [ ] spec 内容经用户单独批准。
- [x] 用户明确“计划评审通过”后才实现。

---

## 15. 风险与回滚

| 风险 | 缓解 |
|---|---|
| 旧 spec 未删导致双权威 | tag 后删除提交 + `git ls-files` 验收 |
| plans/tickets 规范化改写历史 | 前向/历史分类 + `(historical)` + 语义 diff |
| 迁移遗漏/区间错误 | 逐条迁移 + 覆盖门禁 + 复审 |
| ADR 与 spec 重复 | ADR-0003/0004/0005 Amendment 声明规范归属 |
| 02b 解冻仍违反规范/代码分离 | 独立 `spec-delta/02b-store` 先合并 |
| CI bootstrap 自相矛盾 | 前向引用语法 + 历史标记 + 目标状态验证 |
| config 漂移 | 权威指向 types/defaults/rules；config.md 只导航 |
| 06b worktree 旧 spec | 启用门禁前退役 |
| tag 命名误导 | 使用 `spec-v2-draft`，不标已发布 |

回滚：所有变更在 `docs/spec-v3` 分支，PR 未合并前整体可回退；合并后按提交 revert。

---

## 16. 执行前置条件

1. 本计划通过独立模型复审；
2. 用户明确“计划评审通过”；
3. 06b 已合并（已完成）；
4. 02b 保持 `on-hold`；
5. 06b worktree/branch 在启用门禁前退役（已完成：本地 worktree、本地分支与 `origin` 分支均已删除）；
6. 所有实现提交经 PR，不直接 push main。
7. 合并后自测：从 main 开临时分支修改规范包，`make spec-status` 应 exit 2；`spec-delta/*` 夹带代码应 exit 1。bootstrap PR 内用 `SPEC_BASE=<含 VERSION 的提交>` 模拟该路径。
