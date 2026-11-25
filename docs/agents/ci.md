# CI 检查与路径归属

本文件是 CI 检查归属的唯一权威：每个检查读什么、属于哪条路径组、缓存层是什么。路径分类的代码在 `../../scripts/ci-scope.sh`；修改分类时同时更新本文件与自测用例。

## 路径组

| 组 | 匹配 | 触发 |
|---|---|---|
| Both | `Makefile`、`.github/workflows/ci.yml` | `check-go` + `check-spec` |
| Go | `cmd/`、`internal/`、`configs/`、`*.go`、`go.mod`、`go.sum`、`go.work*`、`.golangci.*`、`scripts/check-*`、`scripts/verify-static.sh` | `check-go` |
| Spec/文档 | `docs/`、`GLOSSARY.md`、`scripts/spec-*.sh`、`.scratch/` | `check-spec` |
| None | 其余路径 | 仅 `ci-scope-guard`；`ci` 成功并输出 notice |

事件规则：

- `workflow_dispatch`、`BASE` 为空、`BASE` 全零、`git diff` 失败 → 两组都跑（fail-safe）。
- PR 用 `base...head`；push 用 `base..head`。
- 未命中任何组时 `ci` 仍成功，但日志有 `::notice`、job summary 有说明。
- 新增顶层目录/根文件必须显式分类（Go / Spec / Mixed / Ignore），否则 `ci-scope-guard` 失败。维护步骤见文末。

## 检查与输入

| 检查 | 读什么 | 路径组 | 缓存 / 说明 |
|---|---|---|---|
| `ci-scope-guard` | tracked 顶层条目 | 始终 | 无；新增目录未分类时失败 |
| `ci-scope` | `git diff --name-only` + 路径表 | 始终 | 无；输出 `go` / `spec` |
| `check-go` | Go 代码、配置、双架构静态构建 | Go | setup-go 模块/构建缓存 |
| `check-spec-code` | `internal/config|interfaces|sim`、`docs/contracts/`、`internal/**/*_test.go` 的 `Covers: REQ-*` | Go（在 `check-go` 内）；文档 PR 也会随 `check-spec` 跑 | 无 |
| `check-spec` | `docs/spec/`、`docs/contracts/`、`docs/traceability.md`、docs 链接、`.scratch/` | Spec/文档 | 无 |
| `lint` | Go 代码 | Go | golangci-lint-action 缓存 |
| `gitleaks` | 完整 git 历史 | 始终 | gitleaks action 缓存 |
| `guard` | PR 元数据与 diff | 始终 | 无 |

关键点：`spec-check` 不只是读 spec 文档；`contracts` 校验 `internal/config`、`internal/interfaces`、`internal/sim` 与合同一致，`covers` 校验 Go 测试的 `Covers: REQ-*`。因此它们在 Go 路径由 `check-go` 内的 `check-spec-code` 执行；文档路径仍跑完整 `check-spec`。

## 缓存层

| 层 | 内容 | 缓存机制 | 失效条件 |
|---|---|---|---|
| 工具链 | Go 1.27.4 | setup-go | Go 版本 |
| 依赖 | `GOMODCACHE` | setup-go | `go.sum` |
| 构建/测试 | `GOCACHE`（content-addressed） | setup-go | Go 版本 + `go.sum`；源码变化只让对应包失效 |
| Linter | 二进制 + 分析缓存 | golangci-lint-action | `v2.15.0`；分析缓存按源码增量 |

docs-only 变更不触碰任何 Go 层；Go 变更只失效构建/测试与 linter 分析，不重下模块或 linter。

## 修改路径分类

1. 改 `scripts/ci-scope.sh` 的 `classify_path` 与 `classify_top_level`。
2. 在 `--self-test` 增加/调整用例。
3. 更新本文件的路径组与检查表。
4. 本地运行 `make ci-scope-test` 与 `make ci-scope-guard`；CI 会运行同一脚本。

新增顶层条目的处理：

- 属于 Go 代码/构建：加入 Go/Mixed 分类，并确认 `classify_path` 覆盖其文件。
- 属于文档/规范：加入 Spec 分类。
- 既非 Go 也非文档（如工具目录、许可证）：加入 ignore 列表并写明理由。
- 任何情况下都不要把条目直接交给 `None` 而不记录。
