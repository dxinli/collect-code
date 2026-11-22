# Spec 管理化方案落地计划（v2）

**状态**：待用户评审（已按独立模型评审修订，2026-11-22）
**基础**：`main@461121c`（PR #6 / 06b 已合并；PR #9 / 02b plan 已合并）
**工作分支**：`docs/spec-v3`
**范围**：文档治理、spec v3、契约拆分、历史引用迁移、PRD 清理、CI 约束
**非目标**：不修改产品行为、接口签名、配置字段、错误码、事件语义；不修改 06b 已合并实现；不恢复 02b 实现。

---

## 0. 对独立评审的处理

独立评审结论为 `reject`。本版逐条处理：

| 评审项 | 处理 |
|---|---|
| B1 02b 未拍板 A/B 被冻结 | **接受**：本次不移动 02b plan §10.8；`store.md` 只从 `internal/interfaces/store.go` + 02a 已批准语义派生，02b 补充标 `pending` |
| B2 缺少 v2 迁移矩阵 | **接受**：新增 §6 完整迁移矩阵；未映射条目不得合并 |
| B3 spec 新位置与 AGENTS 冲突 | **接受**：AGENTS.md + `docs/agents/issue-tracker.md` + `domain.md` 作为最早独立提交 |
| B4 历史引用批量替换改变语义 | **接受**：不再批量改写已完成 tickets/plans；只新增迁移映射，新文档用新锚点 |
| B5 合同无版本/无影响扫描 | **接受**：引入“规范包版本”，spec + contracts 同版本；impact 扫描合同锚点 |
| B6 CI 在 bootstrap PR 失败 | **接受**：检查只作用于 PR diff；历史文件 baseline/allowlist；bootstrap 豁免；fetch-depth 0 |
| B7 流程缺少 plan v2 / AGENTS 提交 | **接受**：本文件为 plan v2；AGENTS 规则最早单独提交 |
| B8 全局权威链错误 | **接受**：改为“按事实类型分权”的矩阵 |
| B9 config.md 手工镜像制造漂移 | **接受**：config.md 改为导航/摘要，不复制全量键；生成方案后续单独做 |
| R1 ADR-0004 Amendment 不完整 | **接受**：Amendment 覆盖路径、接口位置、方法集链接 |
| R2 D1–D28 无 ADR 的决策无家 | **接受**：建立决策索引，逐项指定 ADR/REQ/contract 或标“历史，不作当前权威” |
| R3 版本历史与 CHANGELOG 重复 | **接受**：spec 不设“版本历史”章节，只留元数据 + CHANGELOG 链接 |
| R4 PRD 仍有规范重复/架构无家 | **接受**：新增 `docs/architecture.md`；PRD 规范语句改为链接 |
| R5 REQ/合同锚点/traceability 未定义 | **接受**：定义锚点格式、测试标注、三种状态 |
| R6 脚本/CI 细节不足 | **接受**：定义 spec-status 分支类型、merge-base、fetch-depth、Makefile/ci.yml |
| R7 06b worktree/branch 仍持旧 spec | **接受**：启用门禁前退役 worktree/branch |
| R8 v2 归档仅靠 tag 不可审计 | **接受**：显式 tag 创建/推送 + `spec-archive.sh` 按需 fetch；不保留工作区全文副本 |
| R9 引用替换不等于语义不变 | **接受**：历史事实/实施指令不改；02b 路径变更走新的 plan 修订 |
| R10 06b 契约可能丢失非目标边界 | **接受**：simulator.md 保留当前版本非目标 |
| R11 diffs/HTML/代码走读未纳入基线 | **接受**：纳入基线或明确 allowlist |
| R12 §5.2 与 §6.5 矛盾 | **接受**：规范变更 PR 只改 docs/version/CHANGELOG；代码/测试在后续 PR |
| S1–S8 | **接受**：合并检查脚本、声明源方向、统一 semver、内容判据、proposed 状态、Go 版本、模拟器独立性、Store 从 interfaces 派生 |

---

## 1. 当前事实

| 项 | 状态 |
|---|---|
| `main` | `461121c Merge pull request #6 from dxinli/feat/06b-mqtt-broker-stub` |
| 06b | 已合并；`internal/sim/**` 在 main；ticket `done` |
| 06b spec 改动 | `.scratch/collector/spec.md:621` 起有 `#### 7.1 06b 冻结的 MQTT broker + 云端桩契约` |
| 06b worktree | `.claude/worktrees/06b` 仍存在；分支 `feat/06b-mqtt-broker-stub` 仍存在 |
| 02b | plan 已并入 main；ticket `on-hold`；实现暂停；plan v2 待用户评审 |
| 02b 未决 | plan §4.5/§4.12/§4.13 的 A/B 尚未拍板；§10.8 依赖这些选择 |
| spec v2 | `.scratch/collector/spec.md`，含实现细节、评审编号、ticket 编号 |
| 产品定型 | `docs/产品定型.md` 含旧接口、报文副本、D 决策表 |
| main 保护 | 必须经 PR；required checks `ci`、`gitleaks` |

---

## 2. 最终方案

1. **spec 版本化且受控**：
   - 当前规范包：`docs/spec/spec.md` + `docs/contracts/**`；
   - 包版本：`docs/spec/VERSION`（`v3.0.0`）；
   - v2 用 tag `spec-v2.0.0` 冻结，工作区不保留全文副本；
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
   - Store 契约 → `docs/contracts/store.md`（从 `interfaces.Store` + 02a 派生）
   - 模拟器契约 → `docs/contracts/simulator.md`
   - 决策与理由 → `docs/adr/**`
   - 术语 → `GLOSSARY.md`
   - 产品意图 → `docs/产品定型.md`
   - 架构/包布局 → `docs/architecture.md`
   - 单票验收 → tickets
   - 历史 → plans / tag / migration log
4. **兼容不污染规范**：
   - 不保留旧标题空壳；
   - 历史 tickets/plans **不批量改写**；新增 `docs/refs-migration-log.md` 作为等价表；
   - 新文档只引用新锚点。
5. **新工作最小上下文**：
   - 新 ticket 只允许 `REQ-*`、合同锚点、`ADR-*`；
   - 禁止 `spec §N`、决策编号、archive 路径、旧 plan 路径；
   - `docs/README.md` 定义当前/历史阅读路径；
   - `docs/agents/domain.md` 默认不读 archive/migration/旧 plan。
6. **跨分支/worktree 同步**：
   - `VERSION` + `spec-check.sh` + PR 模板 + CI；
   - 分支落后 main、feature branch 改规范、规范包变更无版本，均失败。
7. **06b / 02b**：
   - 06b §7.1 → `docs/contracts/simulator.md`，保留非目标边界；
   - 02b 保持 `on-hold`；本次不移动 §10.8；解冻后另发 plan 修订。

---

## 3. 不可破坏的兼容性约束

| 编号 | 约束 |
|---|---|
| C1 | 不修改 06b 已合并实现、ticket、评论、验收 |
| C2 | 02b 保持 `on-hold`；本次不实现、不勾选、不标 done；不把未拍板 A/B 写成契约 |
| C3 | 01a/01b/01c/02a/06b 的验收、评论、提交哈希不改 |
| C4 | **不批量改写**已完成 tickets/plans；只新增迁移映射 |
| C5 | 不修改 `cmd/**`、`internal/**`、`go.mod`、`go.sum`、`internal/config/testdata/**`；允许修改 `Makefile`、`.github/**`、`scripts/spec-*` |
| C6 | 不修改 `docs/需求文档_*.md`、`docs/缺口台账.md` 内容 |
| C7 | ADR 原文不改写；ADR-0004 用 Amendment |
| C8 | spec v3 不保留旧标题空壳 |
| C9 | 所有变更经 PR 进入 main；禁止直接 push main |
| C10 | 06b worktree/branch 在启用门禁前退役；不在 spec v3 PR 中静默改它 |

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

历史文件在 PR 中必须哈希不变；新文档不在此基线内。

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
| `docs/refs-migration-log.md` | 历史引用等价表（审计用） |
| `.scratch/collector/issues/**` | 单票验收 |
| `.scratch/collector/plans/**` | 计划历史 |

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
- 版本历史章节（只留元数据 + CHANGELOG 链接）。

### 5.2 条款格式

```markdown
### REQ-INVOKE-01 至多一次受理

引擎 MUST 对同一 `(device, seq)` 至多受理一次。
首次受理后 MUST 先持久化 `accepted`，再调用驱动 `Write`。
```

理由在 ADR-0004；spec 不写理由。

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
- 本次**不移动** §10.8 到 `store.md`；
- `store.md` 只包含 `internal/interfaces/store.go` 冻结语义 + 02a 已批准内容；
- 02b 补充标 `Pending 02b plan approval`，不作为当前权威；
- 02b 解冻后另发 plan 修订，再追加 Store 语义。

### 6.2 06b 的契约抽取

`docs/contracts/simulator.md` 包含：

- CLI flags；
- manifest schema；
- captured/invoke/control；
- `broker_disconnect`；
- 安全与独立客户端测试；
- **当前版本非目标**：不提供 `/_sim/requests`、ChirpStack fixture、`--config`、生产 MQTT Channel；06a/12a/12b 后续追加，不改本节契约；
- 模拟器独立性：不加载生产配置、不依赖硬件假设、不 import 被测实现。

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

后续可单独做生成物：`make gen-config-doc` + `git diff --exit-code`；本次不引入代码生成。

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
- v2 → v3 为 major（结构/身份变化）；
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
| `lint` | spec 无决策/评审编号、理由、ticket 编号、完整代码/配置/JSON、旧标题空壳 |
| `version` | spec/contracts 改了但 VERSION/CHANGELOG 没变 |
| `contracts` | config 与 testdata/代码；external code/event 在实现后启用；store 与 interfaces |
| `traceability` | REQ/锚点无归属且无计划 |
| `refs` | 仅新文档：禁止 `spec §N`、决策编号、archive 路径 |
| `links` | 新文档相对链接；历史文件 allowlist |

运行规则：

- 只检查 PR diff 的新增/修改行；
- 历史 tickets/plans/diffs/HTML/代码走读在 allowlist；
- bootstrap PR 用 `spec-bootstrap` label 豁免；
- `fetch-depth: 0` 或显式 fetch base；
- 先并入现有 `ci` job，非 required；稳定后再更新 branch protection。

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

- 用 merge-base 判断是否落后，不用“落后 commit 数”；
- 比较规范包版本；
- 检查分支是否修改 `docs/spec/**` 或 `docs/contracts/**`；
- `spec-delta/*` 允许修改规范；其他分支修改则 exit 2；
- 扫描分支引用的 REQ/合同锚点是否被改。

### 9.4 `spec-impact.sh`

- 从 CHANGELOG 提取变更 REQ + 合同锚点；
- 扫描本地分支、worktree、tickets/plans（仅新文档/活跃工作）；
- 输出受影响项。

### 9.5 CI 门禁

- 分支落后 main：失败；
- feature branch 改规范：失败；
- `spec-delta/*` 无版本/CHANGELOG：失败；
- `spec-delta/*` 同时改代码：失败；
- 代码 PR 声明 normative 但无 spec delta：失败；
- 新 ticket/plan 引用不存在的 REQ/锚点：失败。

### 9.6 AGENTS 规则

1. main 是规范唯一写入点；
2. feature branch 不写 spec/contracts；
3. 规范变更开 `spec-delta/<topic>`；
4. 三个检查点运行 `make spec-status`；
5. PR 写 `Spec-Version` / `Spec-Impact` / `REQ`；
6. 分支携带规范修改不得合并；
7. `.scratch/` 只放 tickets/plans；当前规范在 `docs/spec/`，合同在 `docs/contracts/`。

---

## 10. 历史文档策略

**不批量改写已完成 tickets/plans。**

原因：评审确认大量 `spec §N` 不是 token-only，而是实施指令（02b plan §10.8）或历史事实（06b plan §7.1）。替换路径会改变语义。

做法：

1. 保留历史原文；
2. 新增 `docs/refs-migration-log.md`：
   - 旧位置 → 新 REQ/合同锚点；
   - 标注“历史引用，不作当前权威”；
3. 新 ticket/plan 只引用新锚点；
4. 02b 的路径变更：解冻时另发 plan 修订，不在本 PR 做；
5. 06b plan 的 `spec §7.1`：迁移日志记录，原文不改；
6. CI `refs-lint` 只检查**新增/修改行**，历史文件 allowlist；
7. `diffs/06b.md`、`ticket-dependency.html`、`docs/代码走读` 纳入基线/allowlist，不纳入新检查。

---

## 11. 产品定型与架构

### 11.1 产品定型保留

- §1 定位、§2 抽象依据、§3 目标/非目标、§4 角色/部署、§5 场景、§8 功能清单、§11 非功能、§13 风险、§14 路线。

### 11.2 移除/链接

| 位置 | 处理 |
|---|---|
| §5.3/§5.5 规范语句 | 改为链接 spec `REQ-QUALITY-04` / `REQ-OPS-01`，不重复正文 |
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
- `:608` Go 精确版本（`go.mod` 的 `go 1.27` + toolchain `go1.27.4`）；
- `:614` 24h 真实/假时钟；
- `:3` 状态与规范范围。

### 11.4 D 决策索引

`docs/adr/README.md`：

| D | 决策简述 | 当前归属 | 状态 |
|---|---|---|---|
| D1–D3… | 从 PRD §9 提取 | ADR-0001/0004… | current |
| 无 ADR 的 D | 一行决策 + 现有行为 | spec `REQ-*` / contract | historical or current |
| 已废弃 | 保留一行 | ADR-0006 | superseded |

要求：PRD §9 删除前，D1–D28 每项必须有落点；否则不得删除。

---

## 12. 分支与提交计划

### 12.1 分支

- 工作分支：`docs/spec-v3`（从 `main@461121c`）
- 最终 PR；required checks `ci`、`gitleaks`。

### 12.2 提交顺序

1. `plan: spec-v3 计划 v2（按独立评审修订）`（本文件）
2. `docs(agents): 定义 spec/contracts 位置与阅读路径`
   - `AGENTS.md`
   - `docs/agents/issue-tracker.md`
   - `docs/agents/domain.md`
3. `docs(adr): 新增 ADR-0006 与 ADR-0004 Amendment`
4. `docs: 新增文档地图、ADR 索引、D 决策索引`
5. `docs: tag spec-v2.0.0 + 新增 spec v3 + VERSION/CHANGELOG`
6. `docs: 新增 external/config/simulator/testing/traceability 契约`
7. `docs: 新增 architecture + 清理产品定型`
8. `docs: 新增 refs-migration-log（不改历史原文）`
9. `ci: 新增 spec-check/spec-status/spec-impact/spec-archive、Makefile、PR 模板、workflow`
10. `chore: 退役 06b worktree/branch`（单独提交/操作）

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

不保留工作区全文副本；浅克隆审计必须先 fetch tag。README 写明。

---

## 13. 验收标准

### 13.1 结构

- [ ] `docs/spec/spec.md` v3.0.0，`Status: proposed`；用户批准后改 accepted。
- [ ] `VERSION`、`CHANGELOG.md` 存在且一致。
- [ ] spec 无决策/评审编号、理由、ticket 编号、完整代码/配置/JSON、旧标题空壳。
- [ ] v2 迁移矩阵逐条有归属；无未映射条目。
- [ ] contracts external/config/store/simulator 存在。
- [ ] testing/traceability/architecture/README/ADR README/refs-migration-log 存在。
- [ ] PRD 无 Go/JSON/YAML 代码块、无重复规范、无 D 表正文。

### 13.2 兼容

- [ ] 06b 实现/ticket/评论/验收不变；§7.1 已移到 simulator contract。
- [ ] 02b 仍 `on-hold`；未把 A/B 写成契约；plan 原文未改。
- [ ] 01a/01b/01c/02a 历史哈希不变。
- [ ] `cmd/**`、`internal/**`、`go.mod`、`go.sum`、testdata 无变更。
- [ ] ADR-0004 原文不变，Amendment 完整。
- [ ] 历史 diffs/HTML/代码走读在 allowlist。

### 13.3 控制

- [ ] `spec-check.sh` 六项检查可在 bootstrap PR 通过（豁免）。
- [ ] `make spec-status` 可检测落后/改规范/REQ 变更；`spec-delta` 例外。
- [ ] `spec-impact` 覆盖 REQ + 合同锚点。
- [ ] PR 模板含 `Spec-Version` / `Spec-Impact` / `REQ`。
- [ ] AGENTS 规则已更新且最早单独提交。
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
| 02b 未拍板决策被冻结 | 本次不移动 §10.8；store.md 只从 interfaces/02a 派生 |
| 历史引用改写语义 | 不批量改写；迁移日志 + 新文档锚点 |
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
