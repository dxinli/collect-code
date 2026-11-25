# CI 门禁与检查归属

本文件是 CI 门禁的权威：一个 workflow、三个 job、每个检查读什么、缓存层是什么。CI 不做路径过滤；每次 PR/push 全量运行。

## 门禁结构

| job | 运行 | 内容 |
|---|---|---|
| `ci` | PR、push main、手动 dispatch | checkout、setup-go、golangci-lint、`make check-go`、`make check-spec` |
| `guard` | PR only | plan-review 文件范围、`/plan-approved` 记录、ticket 元数据（`Closes`/`Spec-*`/`REQ`）、`/code-approved` 记录 |
| `gitleaks` | 始终（PR、push main、手动 dispatch） | 完整历史密钥扫描 |

required checks：`ci`、`guard`、`gitleaks`。

没有路径过滤文件、没有 changed-path action、没有按路径跳过的检查。本 PR 实测 `ci` 约 28s（热缓存）；docs-only 也跑全量。路径过滤只能省 docs-only 约 30s，却需要 CI/plan-review/spec-delta 三套集合、额外 action 与标签触发，收益不抵维护成本。

## 标签只读

- `plan-review`、`code-review` 是流程标签，不是 workflow 触发器。
- 增删标签不会起新 run。`guard` 在运行时用 `gh pr view` 读取当前标签，所以需要重新评估时用 Re-run all jobs。
- 把 `plan-review` 加到一个已经包含代码的 PR 时，标签变更不会触发 run；必须 Re-run all jobs 才会用当前标签检查该 diff。这是移除标签触发的代价，ADR-0008 记录该取舍。
- 元数据检查在运行时读取当前 PR body、目标 issue 与 `docs/spec/VERSION`；改 body/issue 后同样需要 Re-run。
- `/plan-approved` 是 PR 评论，不是标签；评论同样不会触发 run。记录后需 Re-run all jobs，或由下一次代码 push 触发。
- `/code-approved` 同样不触发 run；`code-review` 标签存在时 `guard` 失败，记录并移除标签后需 Re-run。
- `on.pull_request` 的类型是 opened/synchronize/reopened/ready_for_review；没有 labeled/unlabeled，也没有 `pull_request_review` 触发。`ready_for_review` 由 ADR-0011 加入。

## 计划通过记录

代码文件变更需要 reviewer 在 PR 会话评论或 review 正文中留 `/plan-approved <sha>`。`guard` 用 `gh api --paginate` 读取 issue comments 与 review bodies（comments 需要 `issues: read`），并校验：

- 评论作者是 `github.repository_owner`；review 只采纳 `COMMENTED`/`APPROVED`，`DISMISSED`/`CHANGES_REQUESTED` 不算；
- SHA 能在 checkout 中解析，且是 PR head 的祖先；
- SHA 不由 base 历史引入（必须是本 PR 的提交）；
- `git diff <base>...<sha>` 只含 plan allowlist（live 值见 `scripts/path-policy.sh` 的 `PLAN_ALLOW_RE`）。

记录点含代码、属于 base 历史、或被 force-push/rebase 移出历史时无效。评论时间不与提交时间比较：记录指向旧计划 SHA 时，事后评论仍会被接受；门禁只证明计划阶段提交被批准。新的计划-only commit 也不会自动使旧记录失效，material change 需 reviewer 重新评论。豁免 `spec-delta/*`、`dependabot/*`、`probe/*`。格式、查询命令与人工步骤见 [issue-tracker 的 Plan 节](issue-tracker.md#plan)。

## Ticket 元数据

所有非豁免 PR 必须满足：

- 正文有且只有一行 `Closes #<n>`；其他关闭关键字（`Fixes`、`Resolves`、小写变体）与第二条关闭引用被拒绝；代码围栏内的示例不计入。目标必须是 open 的 issue、不是 PR、不是 #71，且正文含真实 `**Ticket:** <id>`；
- `Spec-Version` 等于 `docs/spec/VERSION`；
- `Spec-Impact` ∈ {none, patch, minor, major}；改了 `docs/spec/` 或 `docs/contracts/` 时不得为 `none`；
- `REQ` 为 `none`，或 `REQ-*` / 合同锚点列表（允许范围写法与常见分隔符）；值必须在 `**REQ:**` 同一行，只有分隔符会被拒绝。

`dependabot/*`、`probe/*` 全免；`spec-delta/*` 仅免 `Closes`/issue 校验，仍要求 `Spec-Version`、`Spec-Impact`、`REQ`。计划阶段的票 PR 从创建起就带 `Closes #N`，没有单独的 plan-only 例外。规则来源见 [ADR-0010](../adr/0010-guard-ticket-metadata.md)。

## Code review 记录

非 draft、非豁免 PR 需要 reviewer 在会话评论或 review 正文中留 `/code-approved <head-sha>`：

- 评论作者是 `github.repository_owner`；review 只采纳 `COMMENTED`/`APPROVED`，`DISMISSED`/`CHANGES_REQUESTED` 不算；
- SHA 必须解析为本次 run 的 head，且与当前 PR head 相同；新 commit、amend、rebase、force-push 都会使记录失效；run 落后于当前 head 时失败；
- `code-review` 标签仍在时 `guard` 失败；标签是等待标记，不是通过信号；
- draft PR **失败**，不跳过：draft 上的绿色 check 会在 `ready_for_review` 新 run 注册前形成 false-green 窗口。`ready_for_review` 会触发一次新 run，保证 draft→ready 被重新评估。

评论、review、标签变更都不触发 run；记录并移除标签后需 Re-run all jobs。批准后再加回 `code-review` 不会自动使绿色 check 变红；需要时手动 Re-run。`dependabot/*`、`probe/*` 豁免；`spec-delta/*` 仍需要记录。规则来源见 [ADR-0011](../adr/0011-code-review-approval-record.md)。

## 路径策略（流程规则）

CI 本身全量；下面是流程规则，不是 CI 门控。live 值只有一个来源：`scripts/path-policy.sh`。

| 策略 | 用途 | live 值 |
|---|---|---|
| plan-review 范围 | 计划评审期间允许的文件 | `PLAN_ALLOW_RE` |
| spec-delta 范围 | 规范变更 PR 允许的文件 | `SPEC_DELTA_ALLOW_RE` |
| 版本触发 | 需要 VERSION/CHANGELOG 的文件 | `SPEC_VERSION_TRIGGER_RE` |
| 本地-only 目录 | 不参与 spec links 检查 | `LOCAL_ONLY_DIRS` |

- `ci.yml`、`spec-status.sh`、`spec-impact.sh`、`spec-check.sh` 都 source `scripts/path-policy.sh`；不要在别处复制集合或正则。
- `make spec-check` 先跑 `path-policy.sh --self-test`：检查正则行为、消费者引用、无第二份可执行副本、local-only 目录与 `.gitignore` 一致，并动态验证 links 忽略本地目录。
- ADR 正文与索引保留当时决策；live 值以 `scripts/path-policy.sh` 为准（索引已标注）。
- 不要为路径规则新增 CI 路径过滤。

## 检查与输入

| 检查 | 读什么 | 缓存 |
|---|---|---|
| `check-go` | Go 代码、配置、双架构静态构建 | setup-go 模块/构建缓存 |
| `lint` | Go 代码 | golangci-lint action 缓存 |
| `check-spec` | spec、contracts、traceability、docs 与根 `*.md` 链接、`.scratch`、`Covers: REQ-*` | 无 |
| `gitleaks` | 完整 git 历史 | gitleaks action 缓存 |
| `guard` | PR 标签、diff、body、comments、目标 issue、`docs/spec/VERSION`、`isDraft`、head SHA | 无 |

`check-spec` 的 contracts/covers 读 Go 代码，但只用 shell 工具，不依赖 Go 工具链；它在同一个 `ci` job 里全量运行。

## 缓存层

| 层 | 内容 | 机制 | 失效 |
|---|---|---|---|
| 工具链 | Go（版本见 `.github/workflows/ci.yml`） | setup-go | Go 版本 |
| 依赖 | `GOMODCACHE` | setup-go | `go.sum` |
| 构建/测试 | `GOCACHE`（content-addressed） | setup-go | Go 版本 + `go.sum`；源码变化只让对应包失效 |
| Linter | 二进制 + 分析缓存 | golangci-lint-action（版本见 `ci.yml`） | linter 版本；分析缓存按源码增量 |

每次 PR 都跑 Go 层；缓存命中约 36s，冷启动约 150s。

## 本地与维护

1. 本地 `make ci` = `check-go + lint + check-spec`；CI 另加 `guard` 与 `gitleaks`。
2. 新增 Go 检查：加到 `make check-go` 或 `ci` job；不要新增路径过滤。
3. 新增 spec 检查：加到 `make check-spec`。
4. 路径集合只改 `scripts/path-policy.sh`；新增 local-only 目录还要同步 `.gitignore`（self-test 检查一致）；plan-review/record/元数据逻辑改 `ci.yml`；spec-delta 语义改 `spec-status.sh`；同步本表。
5. `ci.yml` 与 `scripts/path-policy.sh` 的改动会改变门禁，必须人工 review。
6. 标签/PR body/评论/review/issue 变更不触发 run；需要时 Re-run all jobs。

## 移植

模式是「一个 workflow + 全量语言/规范检查 + 流程检查 job」：换语言只换 `make` 目标与 setup 步骤；required check 名称保持不变。不要为单语言仓库加入 changed-path 分类器。
