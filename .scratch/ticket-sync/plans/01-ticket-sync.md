# ticket-sync 实施计划

- **状态：** 用户 2026-11-23 明确“不需要评审 plan，刚才已经过了，直接实现”；计划按已确认决策直接进入实现。
- **分支：** `chore/ticket-sync`（基于 main）。
- **目标：** ticket 状态只在合并后由确定性脚本写成 `done`，并同步生成依赖图；`@claude` 不再承担记账。
- **非目标：** 不改 `docs/spec/**`、`docs/contracts/**`、`VERSION`、`CHANGELOG`；不改采集引擎行为；不引入新的运行时依赖。

## 1. 已确认决策

| 项 | 决策 |
|---|---|
| 写 main 的方式 | C1：bot 建 PR，checks 过后自动合并；不改 `main` 保护规则 |
| bot 身份 | 专用 GitHub App（`contents: write`、`pull-requests: write`、`issues: write`） |
| auto-merge | 开启 `allow_auto_merge`；required approvals 已是 0 |
| `done` 门禁 | `feat/*` 分支写 `done` 直接 CI 失败；只有 `chore/tickets-sync-*` 豁免 |

## 2. 状态机

```
ready-for-agent ──(实现完成，开 PR)──> in-review ──(合并)──> done
                                          │                  ▲
                                          └─ 依赖方仍 blocked ─┘
                                            只有 tickets-sync bot 写 done
```

- 依赖判断只认 `done`；`in-review` 不解锁任何下游。
- `on-hold` / `needs-info` 不由 bot 自动翻；合并时只评论提醒。
- 未知状态按“未完成”处理（依赖满足时显示 ready，否则 blocked），但 `set-status` 只接受白名单。

## 3. 触发方式

### 3.1 自动：PR 合并

- 事件：`push: branches: [main]`；合并会产生一次 main 的 push。
- 用 `gh api repos/<owner>/<repo>/commits/<sha>/pulls` 找到与该 commit 关联且已合并的 PR。
- 解析 ticket：PR body 的 `Ticket:` / `Tickets:` → label `ticket:<id>` → 分支 `feat/<id>-`。
- 执行：`ticketgraph complete`（`in-review` → `done`）→ `ticketgraph render`。
- 结果：bot 建 `chore/tickets-sync-<ids>-<run>` 分支和 PR（只改 `.scratch/`），label `tickets-sync`，然后 `gh pr merge --auto --squash`。
- bot PR 通过 `ci` + `gitleaks` 后自动合并；main 上的状态和 HTML 随之更新。
- 原 PR 收到评论：同步 PR 链接 + 新解锁的 ticket。
- 不用 `pull_request_target`：公开仓库从 2026-12-02 起默认限制该事件，`push` 同时避免这个策略告警。

### 3.2 手动：PR 评论 `/accept 02b`

- 事件：`issue_comment: types: [created]`。
- 条件：
  - 评论在 PR 上（不是普通 issue）；
  - 评论者 `author_association` 为 `OWNER` / `MEMBER` / `COLLABORATOR`；
  - PR 已经合并（未合并时评论拒绝：`done 只能在合并后写；请先合并 PR`）；
  - 评论形如 `/accept 02b` 或 `/accept 01a,01b,01c`；
  - 省略参数时按 3.1 的规则从 PR 解析。
- 用途：合并后自动 job 失败、或需要补跑时的手动入口。执行体与 3.1 完全相同。

### 3.3 手动：workflow_dispatch

- Actions → `tickets-sync` → Run workflow。
- 输入：`ticket`（必填，如 `02b`；多个用逗号）、`pr`（可选，用于评论和 commit message）、`dry_run`（可选）。
- `dry_run=true` 只打印将要做的修改，不建分支、不建 PR。

### 3.4 本地

```bash
make tickets-accept TICKET=02b   # 改工作区 + 重新生成 HTML，需自行提交/PR
make tickets-check               # 校验 HTML 与 issues 一致
make tickets                     # 只重新生成
make hooks                       # 启用 .githooks/pre-commit
```

### 3.5 不触发 / 拒绝

| 情况 | 行为 |
|---|---|
| PR 未合并 | `/accept` 评论拒绝；自动 trigger 不触发 |
| 无 ticket 映射且分支是 `feat/*` | `tickets-guard` 失败，要求 PR 写 `Ticket:` |
| 无 ticket 映射且分支不是 `feat/*` | 跳过，评论说明 |
| ticket 已是 `done` | no-op |
| ticket 是 `on-hold` / `needs-info` | 跳过并在 PR 评论，交给人判断 |
| bot PR（`chore/tickets-sync-*`） | `tickets-guard` 跳过 |
| 普通 `push` / 非 PR 事件 | `tickets-guard` 跳过 |

## 4. 组件与文件

| 文件 | 作用 |
|---|---|
| `tools/ticketgraph/*` | Go 工具：parse / graph / render / resolve / guard / complete |
| `.scratch/collector/ticket-dependency.html` | 生成物，`<meta name="generator" content="tools/ticketgraph">` |
| `Makefile` | `tickets`、`tickets-check`、`tickets-guard`、`tickets-accept`、`tickets-ci`、`hooks` |
| `.github/workflows/ci.yml` | 加 `tickets-check` / `tickets-guard`；`.scratch/`-only PR 走快速路径 |
| `.github/workflows/tickets-sync.yml` | 合并后 / `/accept` / dispatch 的确定性同步 |
| `.github/pull_request_template.md` | 增加 `**Ticket:**` |
| `.githooks/pre-commit` | issue 变更但 HTML 未重生成时拒绝提交 |
| `docs/agents/issue-tracker.md` | 状态机、命令、触发方式 |
| `AGENTS.md` | 固化“PR=in-review、合并后 bot 写 done、feat 分支禁止 done” |
| `.scratch/ticket-sync/issues/01-ticket-sync.md` | 本工作自身的 ticket |

## 5. `ticketgraph` 设计

```bash
ticketgraph render    --dir .scratch/collector [--check]
ticketgraph set-status --dir .scratch/collector --ticket 02b --status in-review
ticketgraph complete  --dir .scratch/collector --ticket 02b[,01a] [--pr 12] [--json]
ticketgraph ready     --dir .scratch/collector [--json]
ticketgraph resolve   --event "$GITHUB_EVENT_PATH" | --branch/--body-file/--labels
ticketgraph guard     --dir .scratch/collector --event "$GITHUB_EVENT_PATH"
```

- 解析 `**Status:**`、`**Blocked by:**`、`**Note:**`；`Blocked by: None...` 视为无依赖；`19b + 外部 P0 资料` 只取 `19b`，外部条件记录在备注。
- 计算：done / in-review / ready / blocked / on-hold / needs-info、依赖深度、波次、关键路径、直接解锁、传递影响、新解锁集合。
- HTML 确定性：不写 `time.Now()`，按 ticket ID 自然排序；`--check` 逐字节比较。
- 渲染复用现有版式，新增 `in-review`、`on-hold` 状态；`ready now` 只含依赖已完成的 `ready-for-agent`。
- 首次渲染修正现有漂移（02a 已 done、02b 回填 done）。

## 6. CI 门禁

- `tickets-check`：生成结果与提交的 HTML 不一致 → 失败。
- `tickets-guard`：
  - 只在 PR 事件运行；
  - `chore/tickets-sync-*` 跳过；
  - `feat/*` 或显式 ticket 映射：任一 ticket 为 `done` → 失败；
  - `feat/*` 无映射 → 失败；
  - `on-hold`/`needs-info` 只警告。
- `.scratch/`-only PR（bot PR）跳过 Go build/test/lint/race，只跑 `tickets-check`、`tickets-guard`、`spec-status`；job 名仍是 `ci`，不新增 required context。
- `fmt-check` 扩展为检查 `cmd internal tools`。

## 7. 合并后 workflow 细节

- 用 `push: branches: [main]`；checkout `ref: main`。
- 通过 `commits/<sha>/pulls` 找到已合并的 PR；bot PR 按 `chore/tickets-sync-*` 分支或 `tickets-sync` label 跳过。
- 用 `actions/create-github-app-token@v3`（Node 24）和 `client-id`；`permission-*` 输入；
  `TICKETS_APP_CLIENT_ID`（repository variable）是 v3 的 client ID，`TICKETS_APP_ID` 仍用于 commit email。
- `concurrency: group: tickets-sync`，`cancel-in-progress: false`。
- bot PR 创建后开启 auto-merge，并轮询到 MERGED（超时 20 分钟）再释放锁，避免两次合并的 HTML 互相覆盖。
- strict 检查导致分支落后时 rebase 后重新生成、force-push bot 分支并重试。
- 失败：在原 PR 评论 run 链接；`workflow_dispatch` 可重试；ticket 停在 `in-review`，依赖方保持 blocked（fail-closed）。

## 8. 一次性设置（用户操作）

1. 创建 GitHub App `collect-code-tickets-bot`，权限 `Contents: Read and write`、`Pull requests: Read and write`、`Issues: Read and write`、`Metadata: Read`；只安装到 `dxinli/collect-code`；不启用 webhook。
2. 生成 private key；在 Actions secrets 添加 `TICKETS_APP_PRIVATE_KEY`，并保留 `TICKETS_APP_ID`（bot commit email 用）。
3. 在 Actions **Variables** 添加 `TICKETS_APP_CLIENT_ID`（GitHub App 设置页的 Client ID）；`create-github-app-token@v3` 用 `client-id`，不再用已弃用的 `app-id`。
4. Settings → General → Pull requests → 开启 **Allow auto-merge**。
5. 创建 label `tickets-sync`（workflow 缺失时会用 `gh label create --force` 补建）。

## 9. 回填与验收

- 02b 已合并（PR #12），但 main 上仍是 `ready-for-agent`；实现完成后用 `complete` 回填为 `done`，并生成新的 HTML。
- 验收：
  - `make tickets-check` 通过；
  - `go test ./...`、`go vet ./...`、`gofmt`、`go fix` 通过；
  - `tickets-guard` 对 `feat/*` + `done` 的夹具失败，对 bot 分支跳过；
  - HTML 统计与 issue 文件一致（02a/02b/06b 等 done，03a/04a/05a/11/12a/19a ready）；
  - `tickets-sync.yml` 在合并后能生成 bot PR 并自动合并（合并后首次用 `workflow_dispatch` dry-run 验证）。

## 10. 实施顺序

1. plan + ticket 提交；
2. parse / graph / render + 测试；
3. 回填 02b + 生成 HTML；
4. Makefile + CI 门禁 + pre-commit + PR 模板；
5. `tickets-sync.yml`；
6. 文档（issue-tracker、AGENTS）；
7. 本地检查全部通过，提交 PR。
