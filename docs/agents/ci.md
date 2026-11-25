# CI 门禁与检查归属

本文件是 CI 门禁的权威：一个 workflow、三个 job、每个检查读什么、缓存层是什么。CI 不做路径过滤；每次 PR/push 全量运行。

## 门禁结构

| job | 运行 | 内容 |
|---|---|---|
| `ci` | PR、push main、手动 dispatch | checkout、setup-go、golangci-lint、`make check-go`、`make check-spec` |
| `guard` | PR only | plan-review 文件范围、`/plan-approved` 记录、`Closes #<issue>` |
| `gitleaks` | 始终（PR、push main、手动 dispatch） | 完整历史密钥扫描 |

required checks：`ci`、`guard`、`gitleaks`。

没有路径过滤文件、没有 changed-path action、没有按路径跳过的检查。本 PR 实测 `ci` 约 28s（热缓存）；docs-only 也跑全量。路径过滤只能省 docs-only 约 30s，却需要 CI/plan-review/spec-delta 三套集合、额外 action 与标签触发，收益不抵维护成本。

## 标签只读

- `plan-review`、`code-review` 是流程标签，不是 workflow 触发器。
- 增删标签不会起新 run。`guard` 在运行时用 `gh pr view` 读取当前标签，所以需要重新评估时用 Re-run all jobs。
- 把 `plan-review` 加到一个已经包含代码的 PR 时，标签变更不会触发 run；必须 Re-run all jobs 才会用当前标签检查该 diff。这是移除标签触发的代价，ADR-0008 记录该取舍。
- `Closes` 检查在 PR 事件读取当前 body；改 body 后同样需要 Re-run。
- `/plan-approved` 是 PR 评论，不是标签；评论同样不会触发 run。记录后需 Re-run all jobs，或由下一次代码 push 触发。
- `on.pull_request` 只有默认的 opened/synchronize/reopened；没有 labeled/unlabeled。

## 计划通过记录

代码文件变更需要 reviewer 在 PR 评论 `/plan-approved <sha>`。`guard` 用 `gh api --paginate` 读取 PR 评论（需要 `issues: read`），并校验：

- 评论作者是 `github.repository_owner`；
- SHA 能在 checkout 中解析，且是 PR head 的祖先；
- SHA 不由 base 历史引入（必须是本 PR 的提交）；
- `git diff <base>...<sha>` 只含 plan allowlist（`docs/`、`.scratch/`、`AGENTS.md`、`CLAUDE.md`、`GLOSSARY.md`）。

记录点含代码、属于 base 历史、或被 force-push/rebase 移出历史时无效。评论时间不与提交时间比较：记录指向旧计划 SHA 时，事后评论仍会被接受；门禁只证明计划阶段提交被批准。新的计划-only commit 也不会自动使旧记录失效，material change 需 reviewer 重新评论。豁免 `spec-delta/*`、`dependabot/*`、`probe/*`。格式、查询命令与人工步骤见 [issue-tracker 的 Plan 节](issue-tracker.md#plan)。

## 路径策略（流程规则）

CI 本身全量；下面两条是流程规则，不是 CI 门控：

| 策略 | 用途 | 定义位置 | 集合 |
|---|---|---|---|
| plan-review 范围 | 计划评审期间允许的文件 | `ci.yml` 的 `guard` step | `docs/`、`.scratch/`、`AGENTS.md`、`CLAUDE.md`、`GLOSSARY.md` |
| spec-delta 范围 | 规范变更 PR 允许的文件 | `scripts/spec-status.sh` | `docs/spec/`、`docs/contracts/` |
| 检查内部读取 | 各检查读取的固定路径 | `scripts/spec-check.sh` 等 | spec 包、链接、covers |

- plan-review 只允许 `docs/`、`.scratch/`、`AGENTS.md`、`CLAUDE.md`、`GLOSSARY.md`；其余全部拒绝（包括 `scripts/spec-*.sh`、`configs/`、`internal/`、`.github/`、其他根 md）：工具脚本、配置与代码属于实现阶段。
- spec-delta 的任何其他路径（含额外文档、`.scratch`）都失败；bootstrap（base 无 spec 包）同样只允许 spec 包路径。需要说明时写 `docs/spec/CHANGELOG.md`。
- 两条规则各自只有一个定义位置；不要为它们新增 CI 路径过滤。

## 检查与输入

| 检查 | 读什么 | 缓存 |
|---|---|---|
| `check-go` | Go 代码、配置、双架构静态构建 | setup-go 模块/构建缓存 |
| `lint` | Go 代码 | golangci-lint action 缓存 |
| `check-spec` | spec、contracts、traceability、docs 与根 `*.md` 链接、`.scratch`、`Covers: REQ-*` | 无 |
| `gitleaks` | 完整 git 历史 | gitleaks action 缓存 |
| `guard` | PR 标签、diff、body、comments | 无 |

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
4. 修改流程规则：plan-review 与 plan approval 改 `ci.yml` 的 `guard` step；spec-delta 改 `spec-status.sh`；同步本表。
5. `ci.yml` 的改动会改变门禁，必须人工 review。
6. 标签/PR body/评论变更不触发 run；需要时 Re-run all jobs。

## 移植

模式是「一个 workflow + 全量语言/规范检查 + 流程检查 job」：换语言只换 `make` 目标与 setup 步骤；required check 名称保持不变。不要为单语言仓库加入 changed-path 分类器。
