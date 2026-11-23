# 01 迁移：本地 Markdown ticket → GitHub Issues

**状态：** 用户直接指示“创建计划，然后实施”；本计划不经过独立评审。

## 目标

把 ticket 的唯一状态源从 `.scratch/collector/issues/*.md` 迁到 GitHub Issues，并删除为此自建的整套状态机。

迁移后：

- 5 个 state label：`needs-triage`、`needs-info`、`ready-for-agent`、`ready-for-human`、`wontfix`
- 2 个 category label：`bug`、`enhancement`（GitHub 默认已有）
- 生命周期 = issue `open` / `closed`
- PR 合并时用 `Closes #N` 原生关闭 issue
- 依赖 = GitHub 原生 issue dependencies（`blockedBy`）
- 计划、diff 说明、走读草稿仍放 `.scratch/`
- 唯一的自制件：`make tickets` 列出“没有未关闭 blocker 的 ready-for-agent issue”

## 非目标

- 不迁移 `.scratch/collector/plans`、`diffs`
- 不引入 `.out-of-scope/`（当前没有需要记录的拒绝项）
- 不把外部 PR 纳入 triage 队列（`PRs as a request surface: no`）
- 不删除 GitHub App / secrets；迁移完成后由用户决定是否清理

## 状态与角色

每个 triage 过的 issue 恰好一个 category + 一个 state：

```text
category: bug | enhancement
state:    needs-triage | needs-info | ready-for-agent | ready-for-human | wontfix
```

状态迁移与原生 `triage` skill 一致：

```text
unlabeled → needs-triage → needs-info / ready-for-agent / ready-for-human / wontfix
needs-info → needs-triage（报告者回复后）
```

`ready-for-agent` 的 issue 正文即 agent brief：包含 What to build、验收标准、Out of scope。新建 issue 使用原生 brief 字段；迁移进来的 issue 保留原有正文。

## 导入映射

每个 `.scratch/collector/issues/<file>.md`：

| 源 | 目标 |
|---|---|
| `# 01a: 标题` | issue title `01a: 标题` |
| `**Status:** ready-for-agent` | state label |
| `**Status:** needs-info` | state label |
| 全部 ticket | category label `enhancement` |
| `**What to build:**`、验收清单、`## Comments` | issue body |
| `**Blocked by:** 01a,02a` | 原生 `blockedBy` 边 |
| `**PR:** #N`、`**Merged:** #N` | body 的 `## History` |
| `.scratch/collector/plans/<id>.md` 存在 | body 的 `**Plan:**` 链接 |
| `Merged:` 非空 | 创建后 close（reason: completed） |

Body 首行写 `**Ticket:** <id>`，保证脚本稳定解析。

## 实施步骤

1. 打 tag `pre-issues-migration`（回滚点）。
2. 创建 4 个 state label（`wontfix` 已存在；`bug`/`enhancement` 已存在）。
3. 用 `/tmp` 下的一次性脚本导入 33 个 issue：
   - dry-run 输出清单，不写 API
   - 创建 issue，记录 ticket ID → issue number
   - 抽查一条依赖边（`01b blocked by 01a`），确认原生依赖可用后批量添加 62 条边
   - 关闭 9 个已合并 issue
4. 验证：
   - 33 个 issue，9 closed
   - label 分布：32 `ready-for-agent`、1 `needs-info`
   - 依赖边数量正确
   - `make tickets` 输出的 ready 集合等于迁移前基线：`03b, 12a, 11, 06a, 17a, 19a`；blocked 17 个
5. 清理仓库：
   - 删除 `tools/ticketgraph/`
   - 删除 `.github/workflows/tickets-sync.yml`
   - 删除 `.scratch/collector/issues/`、`.scratch/ticket-sync/`
   - 新增 `scripts/tickets-ready.sh`、`.github/ISSUE_TEMPLATE/ticket.md`、`docs/agents/triage-labels.md`
   - 改写 `docs/agents/issue-tracker.md`、`AGENTS.md`、`docs/README.md`
   - 更新 `Makefile`、`.github/workflows/ci.yml`、`.github/pull_request_template.md`、`.gitignore`
6. `make ci`、`make tickets` 全绿。
7. 提交 PR，由用户合并。

## 文件影响

删除：

- `tools/ticketgraph/`（11 个文件，2215 行）
- `.github/workflows/tickets-sync.yml`（315 行）
- `.scratch/collector/issues/`（33 个文件）
- `.scratch/ticket-sync/`（旧工具自身的 ticket/plan）
- `Makefile` 的 6 个 ticket target、3 个变量
- `ci.yml` 的 docs-only 特殊路径

新增：

- `scripts/tickets-ready.sh`
- `.github/ISSUE_TEMPLATE/ticket.md`
- `docs/agents/triage-labels.md`

修改：

- `AGENTS.md` 第 6、8–10 条与 Issue tracker 段
- `docs/agents/issue-tracker.md`（改为 GitHub 操作手册）
- `docs/README.md` 的 ticket 位置
- `.github/pull_request_template.md`（`Ticket:` → `Closes`）
- `.gitignore`（去掉生成 HTML 条目）

## 验证

- `make ci` 通过
- `make tickets` 与迁移前基线一致
- `grep -rn 'ticketgraph\|tickets-sync\|tickets-guard'` 只剩 git 历史/plan 文档
- GitHub 上抽查：
  - 一个未关闭 issue 的 blocked-by 关系
  - 一个已关闭 issue 的 label
  - 一个依赖全部关闭的 issue 出现在 ready 列表

## 回滚

- 迁移前打 tag `pre-issues-migration`
- 本地 issue 文件在验证通过前不删除
- GitHub Issues 是追加式操作；导入有误可关闭错误 issue 后重跑（新编号）
- 代码清理在独立 commit，可 revert

## 风险

- **原生依赖不可用**：已用 GraphQL introspection 确认 `blockedBy` / `addBlockedBy` 存在，`gh 2.104` 支持 `--add-blocked-by`；失败则回退为 body `Blocked by: #N`
- **issue 编号与 ticket ID 不同**：title 前缀 + body `**Ticket:**` 双保险
- **离线不可用**：`make tickets` 需要 `gh` 登录；计划和代码仍在本地
- **通知噪音**：一次性创建 33 个 issue，仅仓库 owner 收到
- **`.scratch` 只读历史**：删除的 33 个文件仍可从 git 历史恢复

## 实施结果

- 导入 33 个 issue（#25–#57），其中 9 个已合并的 issue 立即 close。
- 建立 62 条原生 `blockedBy` 边；抽查 `03b` 的 open blocker 为 0。
- `make tickets` 与迁移前基线一致：6 ready（03b、06a、11、12a、17a、19a）、17 blocked、1 needs-info。
- `make ci` 通过；`spec-check` 不再扫描已删除的 `.scratch/collector/issues/`。
- `tickets-sync` label 已删除。
- `TICKETS_APP_*` secrets 保留未删，由用户决定是否清理。
