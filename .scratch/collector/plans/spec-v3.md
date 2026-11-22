# Spec 管理化方案落地计划（v1）

**状态**：待独立评审（2026-11-22）
**基础**：`main@461121c`（PR #6 / 06b 已合并；PR #9 / 02b plan 已合并）
**范围**：文档治理、spec v3、契约拆分、历史引用规范化、PRD 清理、CI 约束
**非目标**：不修改产品行为、接口签名、配置字段、错误码、事件语义；不修改 06b 已合并实现；不恢复 02b 实现。

---

## 0. 当前事实

| 项 | 状态 |
|---|---|
| `main` | `461121c Merge pull request #6 from dxinli/feat/06b-mqtt-broker-stub` |
| 06b | 已合并；`internal/sim/**` 在 main；ticket `done` |
| 06b spec 改动 | `.scratch/collector/spec.md:621` 起有 `#### 7.1 06b 冻结的 MQTT broker + 云端桩契约` |
| 06b worktree | `.claude/worktrees/06b` 仍存在，分支 `feat/06b-mqtt-broker-stub` |
| 02b | plan 已并入 main；ticket `on-hold`；实现暂停 |
| 02b Store 语义 | `.scratch/collector/plans/02b.md` §10.8 |
| spec v2 | `.scratch/collector/spec.md`，681+90 行，仍包含实现细节、评审编号、ticket 编号 |
| 产品定型 | `docs/产品定型.md` 仍含旧接口代码、报文副本、D 决策表 |
| main 保护 | 必须经 PR；required checks：`ci`、`gitleaks` |

### 0.1 本次要解决的最终问题

1. **spec 不是行为契约**：混入 Go 签名、完整 YAML、校验规则目录、测试实现、评审编号、决策理由、ticket 编号。
2. **同一事实多份权威**：spec / PRD / ADR / 代码 / testdata 重复，且已漂移（旧接口、`internal/core/store`、Go 版本、code/event 表）。
3. **spec 反复漂移**：实现推进导致回写 spec；版本无生命周期，无变更判据。
4. **新工作上下文过重**：新 ticket 需要读旧 spec、旧决策、迁移映射，才能判断该做什么。
5. **非 main 分支/worktree 无法感知 spec 变化**：02b、06b 已发生；没有版本、门禁、影响广播。
6. **评审建议与决策写进 spec**：`P1=A`、`S1/S3`、`D22`、`已定稿` 等不可独立理解的内容留在规范正文。
7. **兼容层污染规范结构**：保留旧标题空壳会让 spec 变成重定向目录。

### 0.2 最终方案（不再回退到中间方案）

1. **spec 版本化且受控**：
   - 当前规范：`docs/spec/spec.md`，版本 `v3.0.0`；
   - v2 用 git tag `spec-v2.0` 冻结，不在工作区保留副本；
   - `docs/spec/VERSION` + `docs/spec/CHANGELOG.md`；
   - semver：编辑 patch、增补 minor、破坏 major；
   - 任何规范变更必须独立 spec-delta PR + 版本/CHANGELOG；feature branch 不得改 spec。
2. **spec 只放规范**：
   - 每条规范有稳定 `REQ-*` ID；
   - 不写理由、评审建议、决策编号、被否决方案、ticket 编号、实现历史、完整代码/配置/JSON；
   - 决策在 ADR；评审在 plan/PR；实现选择在 ticket/plan。
3. **一个事实一个家**：
   - 外部报文：`docs/contracts/external.md`
   - 配置：`docs/contracts/config.md`
   - Store 实现契约：`docs/contracts/store.md`
   - 模拟器契约：`docs/contracts/simulator.md`
   - 测试策略：`docs/testing.md`
   - 决策：`docs/adr/**` + `docs/adr/README.md`
   - 产品意图：`docs/产品定型.md`
   - 行为规范：`docs/spec/spec.md`
   - 单票验收：`.scratch/collector/issues/**`
   - 计划历史：`.scratch/collector/plans/**`
4. **兼容不污染规范**：
   - 不保留旧标题空壳；
   - 旧引用通过 `docs/refs-migration-log.md` + tag `spec-v2.0` 解析；
   - 历史 tickets/plans 做“仅引用 token”规范化，不改语义。
5. **新工作最小上下文**：
   - 新 ticket 只允许引用 `REQ-*`、contract 锚点、`ADR-*`；
   - 禁止 `spec §N`、决策编号、archive 路径、旧 plan 路径；
   - `docs/README.md` 定义“当前工作路径”与“历史审计路径”；
   - `docs/agents/domain.md` 明确默认不读 archive/migration/旧 plan。
6. **跨分支/worktree 同步**：
   - `docs/spec/VERSION` 机器可读；
   - `scripts/spec-status.sh` 检查落后、引用变更、分支是否改 spec；
   - `scripts/spec-impact.sh` 列出受变更 REQ 影响的分支/worktree/ticket；
   - PR 模板写 `Spec-Version` / `Spec-Impact` / `REQ`；
   - CI 对落后 main、feature branch 改 spec、无版本变更的 spec 修改失败。
7. **06b / 02b 归属**：
   - 06b 已合并；其 §7.1 从 spec 移到 `docs/contracts/simulator.md`；
   - 02b 保持 `on-hold`；其 plan §10.8 移到 `docs/contracts/store.md`；
   - 02b 解冻后再 merge main、规范化引用、按 contract 实现。

---

## 1. 不可破坏的兼容性约束

| 编号 | 约束 |
|---|---|
| C1 | 不修改 06b 已合并实现与 ticket 历史；只移动其 spec 契约文本 |
| C2 | 02b 保持 `on-hold`；本次不实现、不勾选验收、不标 done |
| C3 | 已完成的 01a/01b/01c/02a/06b 的验收、评论、提交哈希不改 |
| C4 | 历史 tickets/plans 只做引用 token 规范化；不改状态、验收、日期、评论、代码块、被引用原文 |
| C5 | 不修改 `cmd/**`、`internal/**`、`go.mod`、`go.sum`、`internal/config/testdata/**` |
| C6 | 不修改 `docs/需求文档_*.md`、`docs/缺口台账.md` 的内容 |
| C7 | ADR 原文不改写；ADR-0004 的路径修订用 Amendment |
| C8 | spec v3 不保留旧标题空壳；旧位置只存在于 tag `spec-v2.0` |
| C9 | 所有变更经 PR 进入 main；禁止直接 push main |
| C10 | 06b worktree 不在本次修改范围；需要清理时单独处理 |

### 1.1 基线校验

```bash
git rev-parse HEAD > /tmp/spec-v3-base.txt
sha256sum .scratch/collector/issues/*.md \
          .scratch/collector/plans/*.md \
          docs/需求文档_*.md \
          docs/缺口台账.md \
          > /tmp/spec-v3-historical.sha
```

每个实现 PR 前运行 `sha256sum -c /tmp/spec-v3-historical.sha`；引用规范化阶段单独允许 token 变化，并用掩码 diff 证明无非引用变化。

---

## 2. 目标文档结构与权威顺序

### 2.1 文档地图

| 文档 | 唯一职责 |
|---|---|
| `docs/README.md` | 文档地图、权威顺序、当前/历史阅读路径、变更政策 |
| `docs/spec/spec.md` | 行为契约（`REQ-*`） |
| `docs/spec/VERSION` | 当前 semver |
| `docs/spec/CHANGELOG.md` | 版本变更、受影响 REQ |
| `docs/contracts/external.md` | Topic、JSON、code、事件、seq 契约 |
| `docs/contracts/config.md` | 四层 YAML、默认值、校验分类 |
| `docs/contracts/store.md` | Store 实现契约（来自 02b plan §10.8） |
| `docs/contracts/simulator.md` | 模拟器契约（来自 06b spec §7.1） |
| `docs/testing.md` | L1–L4、假时钟、bbolt 崩溃、模拟器、数值判据 |
| `docs/traceability.md` | `REQ -> test/ticket` 映射 |
| `docs/adr/README.md` | ADR 索引 + D 编号映射 |
| `docs/adr/0006-doc-authority-and-spec-change-policy.md` | 文档权威、版本化、变更控制 |
| `docs/refs-migration-log.md` | 历史引用等价表（审计用） |
| `.scratch/collector/issues/**` | 单票验收 |
| `.scratch/collector/plans/**` | 计划历史 |

### 2.2 权威顺序

1. 代码与契约测试（接口签名）
2. `GLOSSARY.md`（术语）
3. `docs/adr/**`（决策与理由）
4. `docs/spec/spec.md`（行为规范）
5. `docs/contracts/**`（外部/配置/存储/模拟器契约）
6. tickets（单票验收）
7. `docs/产品定型.md`（产品意图）
8. plans / archive（历史，不作为当前权威）

冲突处理：以更高权威为准；发现不一致时开 spec delta，不允许静默修改低权威副本。

---

## 3. Spec v3 结构

```text
docs/spec/spec.md

# Spec v3.0.0：采集引擎行为契约

- Status: accepted
- Supersedes: v2.0.0（tag spec-v2.0）
- Normative: §1–§8
- Change log: docs/spec/CHANGELOG.md

1. 目的与范围
2. 系统不变量
   REQ-TIME-01        时间只来自 Clock
   REQ-QUALITY-01     最终质量只由内核计算
   REQ-QUALITY-02     last_success_at 只由新鲜观测推进
3. 采集调度
   REQ-SCHED-01       相位锁定、不补跑
   REQ-SCHED-02       单设备 deadline
4. 服务调用语义
   REQ-INVOKE-01      (device, seq) 至多一次受理
   REQ-INVOKE-02      分级确认；仅 C 级回 code=0
   REQ-INVOKE-03      失败码语义
5. 下行队列与缓存
   REQ-QUEUE-01       同 device+service 只留最新
   REQ-QUEUE-02       队列上限与 cmd_dropped
   REQ-CACHE-01       事件有界缓存与补发顺序
6. 外部契约摘要
   链接 docs/contracts/external.md
7. 运维行为
   REQ-OPS-01         SIGHUP 差别重载
   REQ-OPS-02         健康端点与指标
   REQ-OPS-03         日志字段与脱敏
8. 验收摘要
   链接 docs/testing.md
9. Out of Scope
10. 版本历史
```

### 3.1 Spec 禁止内容

- `D1–D28`、`P1=A`、`S1/S3/W4`、`N2/N7` 等编号；
- “评审结论”“已定稿”“被否决方案”“理由”；
- ticket 编号、提交哈希、实现历史；
- 完整 Go 接口、完整 YAML、完整 JSON、测试命令、测试类名；
- 旧章节重定向空壳。

### 3.2 条款格式

```markdown
### REQ-INVOKE-01 至多一次受理

引擎 MUST 对同一 `(device, seq)` 至多受理一次。
首次受理后 MUST 先持久化 `accepted`，再调用驱动 `Write`。
```

不写理由；理由在 ADR-0004。

---

## 4. 契约与测试文档

### 4.1 `docs/contracts/external.md`

从 spec §4 与产品定型 §10 合并，必须包含：

- 5 个 Topic；
- 属性上报 JSON；
- 服务调用/回执 JSON；
- `seq` 契约；
- code 表 0、4001–4004、4041、5040–5044；
- `data.stage`；
- 事件枚举（含 `valve_fault_clear`）；
- 事件去重与 `dedup_key`；
- `target_opening` 来源；
- 节点级 LWT。

### 4.2 `docs/contracts/config.md`

从 spec §1/§2 合并：

- 四层文件与引用关系；
- 全部键、类型、默认值、必填/互斥；
- 校验分类；
- 示例；
- 指向 `internal/config/testdata/config/base/`；
- 热重载矩阵链接。

### 4.3 `docs/contracts/store.md`

从 `.scratch/collector/plans/02b.md` §10.8 提取：

- JSON 往返与精度边界；
- `LoadDevices` 顺序不保证；`LoadSnapshot` 保留输入顺序；
- `Close` 幂等 / `ErrClosed`；
- `SaveSnapshot(nil)`；
- `Property.Quality` opaque；
- `Record.Phase` 只接受 accepted/final；
- `LastSeq` 水位；
- corrupt record fail-fast；
- 空 device 前置条件；
- 崩溃恢复测试口径。

不写 02b 的评审编号、ticket 编号、方案比较。

### 4.4 `docs/contracts/simulator.md`

从 `.scratch/collector/spec.md` §7.1（06b 已合并）提取：

- CLI flags；
- manifest schema；
- `/_sim/captured`、`/_sim/invoke`、`/_sim/control`；
- `broker_disconnect` 语义；
- 安全与独立客户端测试要求；
- 后续 06a/12a/12b 的扩展规则。

去掉“06b 冻结”“06b 只装配”等 ticket/历史语言。

### 4.5 `docs/testing.md`

从 spec Testing Decisions 与产品定型 §12 合并：

- L1–L4；
- 假 Clock 与 CI 静态检查；
- bbolt 崩溃恢复；
- 模拟器验收；
- L1 24h 快进 / L3 30–60min / L4 真实 24h；
- 数值判据。

---

## 5. 版本控制与漂移检测

### 5.1 版本规则

| 变更 | 版本 |
|---|---|
| 编辑性修正 | patch |
| 新增/澄清规范 | minor |
| 改变既有行为/外部契约 | major |

每次变更必须更新 `docs/spec/CHANGELOG.md`，写清 REQ、类型、影响。

### 5.2 变更流程

1. 开 `spec-delta/<topic>` 分支，从 main 创建；
2. 只改 spec/contracts + 版本 + CHANGELOG；
3. 独立评审 + 用户批准；
4. 合并；
5. feature branch 再 merge main；
6. 代码/测试与 spec 同变更集或前置 PR。

### 5.3 CI 检查

| 检查 | 失败条件 |
|---|---|
| `spec-lint` | spec 出现决策/评审编号、理由、ticket 编号、完整 Go/YAML/JSON、旧标题空壳 |
| `version-bump` | spec 改了但 `VERSION`/CHANGELOG 没变 |
| `contract-consistency` | code/event/config 与 contracts 不一致 |
| `traceability` | `REQ-*` 无测试/ticket 归属 |
| `refs-lint` | 新 ticket/plan 出现 `spec §N`、决策编号、archive 路径 |
| `link-check` | 相对链接目标不存在 |

### 5.4 不一致处理

- 停止合并；
- 判定权威（行为 spec / 签名 interfaces / 报文 contracts / 理由 ADR）；
- 开 spec delta；
- 更新版本；
- 同一变更集更新代码/测试/contract；
- 不允许静默漂移。

---

## 6. 跨分支 / worktree 同步

### 6.1 工件

- `docs/spec/VERSION`
- `scripts/spec-status.sh`
- `scripts/spec-impact.sh`
- `scripts/spec-sync-all.sh`
- PR 模板字段
- `AGENTS.md` 新章节

### 6.2 检查点

每个分支/worktree 在以下时点运行：

```bash
git fetch origin
git merge origin/main
make spec-status
```

1. 计划评审前；
2. 开始实现前；
3. 提交 PR 前。

### 6.3 `spec-status.sh`

输出：

- 当前分支 spec 版本；
- main spec 版本；
- 落后 commit 数；
- 本分支引用的 `REQ-*` 是否被改；
- 本分支是否修改 spec/contracts。

退出码：

- 0：同步；
- 1：落后；
- 2：分支修改 spec（策略违规）。

### 6.4 `spec-impact.sh`

从 CHANGELOG 提取变更 REQ，扫描分支/worktree/tickets/plans，输出受影响项。

### 6.5 CI 门禁

- 分支 spec 版本 < main：失败；
- feature branch 改 spec/contracts：失败，除非 `spec-change` PR；
- spec-change PR 无版本/CHANGELOG：失败；
- spec-change PR 同时改代码：失败；
- 代码 PR 声明 normative 但无关联 spec 变更：失败；
- ticket/plan 引用不存在的 REQ：失败。

### 6.6 AGENTS 新规则

1. main 是 spec/contracts 唯一写入点；
2. feature branch 不写 spec/contracts；
3. 需要 spec 变更时开 `spec-delta/<ticket>`；
4. 三个检查点运行 `make spec-status`；
5. PR 写 `Spec-Version` / `Spec-Impact` / `REQ`；
6. 分支携带 spec 修改时不得合并。

---

## 7. 历史引用规范化

### 7.1 范围

- `.scratch/collector/issues/**`
- `.scratch/collector/plans/**`
- `docs/产品定型.md`（仅引用与过期副本）
- 不碰需求原文、缺口台账、代码块、被引用原文。

### 7.2 规则

| 可改 | 不可改 |
|---|---|
| `spec §2` → `REQ-*` 或 contract 锚点 | Status、验收勾选、日期、评论、决策 |
| 旧决策编号 → `ADR-*` | 代码/配置块、被引用原文 |
| 旧 spec 路径 → 新路径 | 历史事实错误（保留原样） |

### 7.3 证明语义不变

1. 只替换等价表中的精确 token；
2. 掩码 diff：抹掉被替换 token 后，剩余文本逐字节一致；
3. 所有替换记录进 `docs/refs-migration-log.md`；
4. 无命中 token 的行不得变化；
5. 歧义引用保留 `TODO-REF`，人工确认；未清零不合并。

### 7.4 02b / 06b 特殊处理

- 06b 已合并：§7.1 移到 `docs/contracts/simulator.md`；06b plan 中 `spec §7.1` 引用规范化；不改 06b 实现/验收/评论。
- 02b：plan `spec §3` 引用规范化到 `docs/contracts/store.md`；ticket 保持 `on-hold`；加一行解冻说明；不实现。

---

## 8. 产品定型清理

保留：

- §1 定位、§2 抽象依据、§3 目标/非目标、§4 角色/部署、§5 场景、§8 功能清单、§11 非功能、§13 风险、§14 路线。

移除/链接：

| 位置 | 处理 |
|---|---|
| §6.1–6.3 Go 接口 | 删除，链接 `internal/interfaces` |
| §6.3.1 调度算法 | 删除，链接 ADR-0003 |
| §6.4 组件表 | 移入 spec v3 组件职责 |
| §7 包布局 | 删除 |
| §9 D 表 | 改为 `docs/adr/README.md` 索引 |
| §10 报文 | 删除，链接 external contract |
| §12 验收细节 | 删除，链接 testing |

事实修正：

- `产品定型.md:81` links 注释；
- `:90` cache/ 目录；
- `:394` `internal/core/store` → `internal/store/bbolt`；
- `:561-571` code 表 → 链接/补 5042–5044；
- `:582-594` 事件表 → 补 `valve_fault_clear`；
- `:608` Go 1.27；
- `:614` 24h 真实/假时钟；
- `:3` 状态与规范范围。

---

## 9. 分支与提交计划

### 9.1 分支

- 工作分支：`docs/spec-v3`（从 `main@461121c` 创建）
- 最终通过 PR 合并；main 受保护，required checks `ci`、`gitleaks`。

### 9.2 提交

1. `plan: spec-v3 管理化方案落地计划`（本文件）
2. `docs(adr): 新增 ADR-0006 文档权威与 spec 变更政策`
3. `docs: 新增文档地图、ADR 索引与阅读路径`
4. `docs: 归档 spec v2（tag spec-v2.0）并新增 spec v3`
5. `docs: 新增外部、配置、Store、模拟器契约与测试策略`
6. `docs: 规范化历史 tickets/plans 的 spec 引用`
7. `docs: 清理产品定型中的接口、报文与决策副本`
8. `ci: 增加 spec 版本、契约一致性与引用检查`
9. `docs: 增加 PR 模板与 spec 同步说明`

### 9.3 06b worktree 收尾

- 06b 已合并；`.claude/worktrees/06b` 与 `feat/06b-mqtt-broker-stub` 可在本分支完成后清理；
- 不在本 PR 中删除 worktree；单独提交/操作。

---

## 10. 验收标准

### 10.1 结构

- [ ] `docs/spec/spec.md` v3.0.0 存在，含 `REQ-*`，无决策/评审编号、无完整代码/配置/JSON。
- [ ] `docs/spec/VERSION` 与 spec 头一致。
- [ ] `docs/spec/CHANGELOG.md` 存在。
- [ ] `docs/contracts/external.md`、`config.md`、`store.md`、`simulator.md` 存在。
- [ ] `docs/testing.md`、`docs/traceability.md` 存在。
- [ ] `docs/README.md`、`docs/adr/README.md`、`docs/refs-migration-log.md` 存在。
- [ ] `docs/产品定型.md` ≤ 300 行，无 Go/JSON/YAML 代码块。
- [ ] `docs/spec/spec.md` 无旧标题空壳。

### 10.2 兼容

- [ ] 06b 实现、ticket、评论不变；§7.1 已移到 simulator contract。
- [ ] 02b 仍 `on-hold`；plan 引用已规范化；未实现。
- [ ] 01a/01b/01c/02a 的验收、评论、提交哈希不变。
- [ ] 历史引用规范化掩码 diff 通过。
- [ ] `cmd/**`、`internal/**`、`go.mod`、`go.sum`、testdata 无变更。
- [ ] ADR-0004 原文不变，仅有 Amendment。

### 10.3 控制

- [ ] `make spec-status` 可检测落后/改 spec/REQ 变更。
- [ ] `make spec-impact` 可列出受影响分支/worktree。
- [ ] CI 检查 spec-lint、version-bump、contract-consistency、traceability、refs-lint、link-check。
- [ ] PR 模板含 `Spec-Version` / `Spec-Impact` / `REQ`。
- [ ] AGENTS 规则已更新。

### 10.4 构建

- [ ] `make ci` 全绿。
- [ ] 独立模型评审通过。
- [ ] 用户明确“计划评审通过”后才进入实现。

---

## 11. 风险与回滚

| 风险 | 缓解 |
|---|---|
| 06b §7.1 提取遗漏 | 逐条对照 06b spec diff；契约测试/文档 review |
| 02b plan 语义被误改 | 掩码 diff + 只替换引用 token |
| 历史 ticket/plan 状态被改 | 校验 Status、复选框、日期、评论哈希 |
| spec v3 仍混入决策 | spec-lint 禁用 token；独立评审 |
| CI 门禁误伤在途分支 | 先合并 06b；02b 冻结；门禁在 v3 PR 后启用 |
| main 保护阻塞 | 全部经 PR；required checks 通过后合并 |
| 版本号漂移 | CI version-bump；spec 头与 VERSION 一致 |

回滚：所有变更在 `docs/spec-v3` 分支，PR 未合并前可整体回退；合并后可按提交 revert。

---

## 12. 执行前置条件

1. 本计划通过独立模型评审；
2. 用户明确“计划评审通过”；
3. 06b 已合并（已完成）；
4. 02b 保持 `on-hold`；
5. 所有实现提交经 PR，不直接 push main。
