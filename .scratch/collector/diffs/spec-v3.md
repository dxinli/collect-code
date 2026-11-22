# spec-v3 实施 diff 说明

<!-- spec-refs: historical-file -->

**范围**：`main@461121c` .. `docs/spec-v3`（spec 管理化方案落地）。
**性质**：文档、规范包与 CI 门禁；不改产品行为、接口签名、配置字段、错误码或事件语义。
**旧规范**：冻结于 tag `spec-v2-draft`；本文的旧行号均指该 tag 中的 `.scratch/collector/spec.md`。

## 1. v2 迁移覆盖

| 旧位置 | 内容 | 新归属 | 状态 |
|---|---|---|---|
| `spec.md:10-19` | Problem | PRD §1/§2；spec §1 | covered |
| `spec.md:21-32` | Solution | PRD §2；`docs/architecture.md` | covered |
| `spec.md:34-45` | In Scope | PRD §3；`REQ-SCOPE-01` | covered |
| `spec.md:47-94` | User Stories | PRD §5/§8/§12；REQ/tickets/testing | covered |
| `spec.md:98-310` | §1 配置与示例 | `contracts/config.md` 导航；`types/defaults/rules` | covered |
| `spec.md:309` | 周期归属 | `REQ-SCHED-04` | covered |
| `spec.md:311-322` | §2 校验 | `contracts/config.md` 分类；`rules.go` | covered |
| `spec.md:324-455` | §3 接口 | `internal/interfaces`；`REQ-TIME-01`、`REQ-SEAM-01/02` | covered |
| `spec.md:456-546` | §4 报文 | `contracts/external.md`；`REQ-EVENT-*` | covered |
| `spec.md:547-564` | §5.1 质量 | `REQ-QUALITY-01..05` | covered |
| `spec.md:565-581` | §5 可靠性 | `REQ-INVOKE-*`、`REQ-QUEUE-*`、`REQ-CACHE-*` | covered |
| `spec.md:580` | **上行去重**（原缺口） | `REQ-UPLINK-01`（评审补入） | covered |
| `spec.md:582-592` | §5.2 健康 | `REQ-OPS-02` | covered |
| `spec.md:588` | **固定指标名**（原缺口） | `REQ-OPS-02` 指标表（评审补入） | covered |
| `spec.md:573` | **LoRa 按窗口计时**（原缺口） | `REQ-INVOKE-05` 段落（评审补入） | covered |
| `spec.md:593-602` | §6 子命令 | `REQ-CLI-01` | covered |
| `spec.md:604-619` | §7 模拟器 | `contracts/simulator.md`；`REQ-SIM-01` | covered |
| `spec.md:621-710` | §7.1 06b 契约 | `contracts/simulator.md` | covered |
| `spec.md:711-754` | Testing | `docs/testing.md` | covered |
| `spec.md:755-765` | Out of Scope | spec §9；PRD §3 | covered |
| `spec.md:766-771` | Further Notes | PRD §14；`docs/testing.md`；tickets | covered |
| `产品定型.md` §6.1–6.3 | 旧接口代码 | 删除，链接 `internal/interfaces` | covered |
| `产品定型.md` §6.3.1 | 调度算法 | ADR-0003 Amendment；`REQ-SCHED-*` | covered |
| `产品定型.md` §6.4 | 组件职责 | `docs/architecture.md` | covered |
| `产品定型.md` §7 | 包布局 | `docs/architecture.md` | covered |
| `产品定型.md` §9 | D 表与被否决方案 | `docs/adr/README.md` + ADRs | covered |
| `产品定型.md` §10 | 报文副本 | `contracts/external.md` | covered |
| `产品定型.md` §12 | 验收 | `docs/testing.md`、tickets | covered |

评审提出的 B1/B2/B3 三项已在上表标注“评审补入”；没有未映射条目。

## 2. 语义 diff 证据

变更只涉及文档、scratch 记录、Makefile、CI 与 `scripts/spec-*`。受保护路径无变化：

```text
$ git diff --exit-code main...HEAD -- \
    cmd internal go.mod go.sum configs \
    docs/需求文档_1.md docs/需求文档_2.md docs/缺口台账.md
(no output)
```

plans/tickets 的修改类型：

| 文件 | 修改类型 |
|---|---|
| `plans/01b.md`、`plans/01c.md`、`plans/02a.md` | 仅文件头历史说明与显式 `spec-refs: historical-file` 标记 |
| `plans/06b.md` | 文件头历史说明；`spec §7.1` 前向引用改为 `contracts/simulator.md` |
| `diffs/06b.md` | 仅文件头历史说明与显式标记 |
| `issues/01b`、`issues/01c`、`issues/06b` | 仅文件头历史说明与显式标记 |
| `issues/02b-store-bbolt.md` | 按计划 §6.1 同步解冻流程；`Status`/验收不变 |
| `plans/02b.md` | `spec §3` 前向引用改为 `contracts/store.md`；A/B 仍未决定 |

状态、验收与提交哈希未改：

```text
$ for f in $(git diff --name-only main...HEAD -- '.scratch/collector/issues/*.md'); do
    git diff main...HEAD -- "$f" | grep -E '^[-+](\*\*Status:|\- \[)' && echo "UNEXPECTED"
  done
(no output)

$ for f in $(git diff --name-only main...HEAD -- '.scratch/collector/issues/*.md'); do
    git diff main...HEAD -- "$f" | grep -E '^[-+].*[0-9a-f]{7}' && echo "UNEXPECTED"
  done
(no output)
```

未新增 `refs-migration-log` 或任何“旧编号 → 新锚点”的常驻映射层。历史引用只通过文件头显式标记或行内 `historical` 豁免；前向引用已改为 `REQ-*` / 合同锚点。

**掩码 diff 结果**：比较时移除历史标记行与行内标记，并把 `spec §N`、`.scratch/collector/spec.md`、`docs/contracts/{store,simulator}.md` 归一为 `<REF>`。结果：

- `plans/01b.md`、`plans/01c.md`、`plans/02a.md`、`diffs/06b.md`、`issues/01b`、`issues/01c`、`issues/06b`：无差异（仅历史标记）。
- `plans/02b.md`、`issues/02b`：差异集中在 `spec §3` → `store` 合同、解冻流程与 `docs/testing.md` 指向；A/B 选择未变。
- `plans/06b.md`：差异集中在引用目标改为 `simulator` 合同 / `REQ-OPS-02`；实现事实与计划决定未变。

没有发现状态、验收、提交哈希或计划决策的语义变化。

## 3. 规范包与 CI 证据

- `docs/spec/VERSION` = `3.0.0`；`docs/spec/spec.md` 为 `Status: proposed`。
- tag `spec-v2-draft` 已推送；`make spec-archive` 可读取旧规范。
- `spec-check.sh` 六项：lint、version、contracts、traceability、refs、links。
- `spec-status.sh` 已接入 `make ci`：
  - 分支落后 main 失败；
  - 非 `spec-delta/*` 修改规范包失败；
  - `spec-delta/*` 缺 VERSION/CHANGELOG 或夹带代码失败；
  - bootstrap（base 尚无 `docs/spec/VERSION`）允许创建规范包，但不得夹带代码。
- `spec-impact.sh` 仅本地使用；CI 不调用。
- PR 模板包含 `Spec-Version`、`Spec-Impact`、`REQ`。

本地结果：

```text
$ make ci
...
golangci-lint: 0 issues
spec-check: lint ok
spec-check: version ok (3.0.0)
spec-check: contracts ok
spec-check: traceability ok (60 ids)
spec-check: refs ok
spec-check: links ok
spec-status: ok
```

GitHub PR #10 的 `ci` 与 `gitleaks` 均通过。

## 4. 已知未完成项

- `docs/spec/spec.md` 仍为 `Status: proposed`；需用户单独批准 spec/contracts 内容后，再提交 `Status: accepted`。
- 02b 仍 `on-hold`；A/B 未拍板；解冻必须先合并 `spec-delta/02b-store`。
- `docs/traceability.md` 中大量条目仍是 `planned(ticket)`；对应实现落地后才改为 `covered`。
- 06b 的实现与 ticket 验收未改；只迁移了契约归属与前向引用。
