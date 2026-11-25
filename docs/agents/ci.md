# CI 检查与路径过滤

本文件是 CI 检查归属的权威：哪些检查常开、哪些按路径门控、缓存层是什么。路径规则的唯一来源是 `.github/path-filters.yml`；workflow 只消费过滤结果。修改规则时同步更新本文件。

## 模式

- **常开**：`check-spec`（纯 git/grep/sed，不依赖 Go 工具链）与 `gitleaks`。
- **门控**：`check-go`、lint、`setup-go`——仅在 Go 输入变更时运行。
- **未知路径 fail-safe**：`non_docs` 为真表示「变更不全是文档」，因此任何未分类路径都会跑 Go，而不是静默跳过。
- required check 名称始终是 `ci`；job 级 `if` 跳过在 GitHub 上报告 success，不会阻塞合并。

## 路径过滤

`.github/path-filters.yml` 只有两组：

| 过滤 | 语义 | 触发 |
|---|---|---|
| `go` | Go 源码与构建输入 | Go 检查 |
| `non_docs` | `**` 排除文档集合：`docs/`、任意 `*.md`、`.scratch/`、`scripts/spec-*.sh` | Go 检查（fail-safe） |

Go 步骤条件：`go == true || non_docs == true || event == workflow_dispatch`。

推论：

- 纯 docs：两条都假 → 只跑 `check-spec`。
- 纯 Go、Go+docs、未知路径：Go 与 spec 都跑。
- 工具脚本、workflow 变更会跑 Go；这是有意的安全默认，不为它们维护第三类规则。
- 新增文档类路径：只改 `non_docs` 的负向列表。
- 新增 Go 源码或构建输入：加入 `go`；`non_docs` 已覆盖。

过滤器名用下划线（`non_docs`），因为 `steps.changes.outputs.non-docs` 在 GitHub 表达式里会被解析为减法。

## 检查与输入

| 检查 | 读什么 | 运行条件 | 缓存 |
|---|---|---|---|
| `check-spec` | spec、contracts、traceability、docs 与根 `*.md` 的链接、`.scratch`、Go 测试的 `Covers: REQ-*` | 总是 | 无 |
| `check-go` | Go 代码、配置、双架构静态构建 | `go || non_docs` | setup-go 模块/构建缓存 |
| `lint` | Go 代码 | `go || non_docs` | golangci-lint action 缓存 |
| `gitleaks` | 完整 git 历史 | 总是 | gitleaks action 缓存 |
| `guard` | PR 元数据与 diff | 总是 | 无 |

`check-spec` 的 contracts/covers 读 Go 代码，但只用 grep/sed，不依赖 Go 工具链；因此 Go-only PR 也跑完整 spec 检查，不再需要 `check-spec-code` 桥接。

## 依赖

- `dorny/paths-filter` v4.0.3，固定完整 SHA，同行版本注释；Dependabot 的 `github-actions` 配置负责更新。
- PR 事件走 GitHub REST，需要 `pull-requests: read`；push 与其他事件走本地 git（checkout 已存在）。
- action 取不到 diff 会让 job 失败（fail-closed）；不要加 `continue-on-error` 吞掉。
- 使用 SHA 而不是 tag：`tj-actions/changed-files` 的 CVE-2025-30066 证明 tag 可被移动。

## 缓存层

| 层 | 内容 | 机制 | 失效 |
|---|---|---|---|
| 工具链 | Go（版本见 `.github/workflows/ci.yml`） | setup-go | Go 版本 |
| 依赖 | `GOMODCACHE` | setup-go | `go.sum` |
| 构建/测试 | `GOCACHE`（content-addressed） | setup-go | Go 版本 + `go.sum`；源码变化只让对应包失效 |
| Linter | 二进制 + 分析缓存 | golangci-lint-action（版本见 `ci.yml`） | linter 版本；分析缓存按源码增量 |

docs-only 不触碰任何 Go 层；Go 变更只失效构建/测试与 linter 分析。

## 修改与维护

1. 路径规则：只改 `.github/path-filters.yml`。文档路径加到 `non_docs` 负向列表；Go 输入加到 `go`。
2. 新检查：读 Go 的复用 Go 条件；廉价且通用的直接常开；不新增分类脚本。
3. action 更新：Dependabot 提交 SHA 与同行版本注释；核对 release notes 后合并。
4. 本地：`make ci` 是完整 gate；`make check` 是纯 Go 快速检查；`make check-spec` 单独跑规范检查。
5. `.github/workflows/**` 与 `.github/path-filters.yml` 的改动会改变门禁本身，必须人工 review。

## 可复制模板

```yaml
- uses: actions/checkout@v7
  with:
    fetch-depth: 0
- id: changes
  uses: dorny/paths-filter@<sha> # v4.0.3
  with:
    filters: .github/path-filters.yml
- if: steps.changes.outputs.go == 'true' || steps.changes.outputs.non_docs == 'true' || github.event_name == 'workflow_dispatch'
  run: make check-go
- run: make check-spec
```

换语言：把 `go` 过滤改成该语言的源码与构建输入，`non_docs` 的负向列表保持「文档集合」。required check 仍放在这个始终运行的 job 上。
