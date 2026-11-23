# 01 main 分支保护与 agent 推送策略（实施计划）

> **已归档（deferred）**：本计划未实施；当前决策见 ADR-0007。保留原文作为历史。

- **状态**：待评审；**过渡窗口内由用户本人临时关闭 `enforce_admins`，其余平台、CI 与代码变更未执行**
- **基线**：`origin/main` `b07d617`（PR #58 合入后）
- **工作分支**：`plan/main-protection`
- **目的**：记录“agent 不得直推 main、本人可直推”的目标方案，以及为什么现在先不做
- **范围**：仅本文档；不改 branch protection/rulesets，不改 `.github/workflows/**`，不改 `AGENTS.md`
- **用户决策（2026-11-23）**：
  1. 暂不落地规则集/CI/设置层的硬约束；
  2. 本地 Claude Code 的直推约束先不做；
  3. 先出计划文档，等文档调整稳定后再执行；
  4. **过渡期例外（2026-11-23 追加）**：临时放宽 main 保护以便本人直接调文档，采用最小改动 `enforce_admins=false`，由用户本人执行；文档稳定后立即恢复。

---

## 0. 问题陈述

现状（`gh api repos/dxinli/collect-code/branches/main/protection`）：

| 项 | 当前值 |
|---|---|
| 必须经 PR | 是（`required_approving_review_count = 0`） |
| Required checks | `ci`、`gitleaks`，`strict = true` |
| `enforce_admins` | `true` |
| 强推 / 删除 | 禁止 |
| 对话解决 | 要求（`required_conversation_resolution = true`） |
| Rulesets | 无（`[]`） |

问题：文档类改动（包括 `AGENTS.md`、README、`.scratch/**`）也必须切分支、开 PR、等 `ci`/`gitleaks`；用户希望“本人可以直接提交，agent 必须走 PR”。

核心事实：**GitHub 只认凭据，不认“谁在敲命令”**。本人与本地 agent 共用同一个 SSH key 时，GitHub 看到的都是 `dxinli`，平台无法区分。因此该目标只能分两层实现：

- **平台层**：约束非本人凭据的 actor（远程 `@claude`、GitHub App、`GITHUB_TOKEN`、bot 账号）；
- **本地层**：同凭据的 agent 只能靠 hook、权限规则或独立 OS 用户/容器隔离。

---

## 1. 目标与非目标

### 目标（待文档稳定后）

1. 本人可以 `git push origin main`（非强推）直接提交；
2. 任何非本人凭据的 agent 直推 main 被拒绝，必须开 PR；
3. 本人也不能强推或删除 `main`（保留最后一道不可逆保护）；
4. 本地 Claude Code 的直推有可用的拦截手段；
5. 不改变现有 `ci`/`gitleaks` 门禁语义。

### 非目标（本轮明确不做）

1. **不修改 CI**：不加路径过滤、不做 docs-only 快车道；`make ci` 与 `ci.yml` 保持原样；
2. **不修改 branch protection/rulesets**；
3. **不新增本地 hook 或 `.claude/settings.json` deny 规则**；
4. 不迁移到组织仓库；
5. 不改变 ticket/plan 流程本身。

---

## 2. 关键事实与证据

| 事实 | 证据 |
|---|---|
| 仓库是个人仓库，owner 是 User | `gh api repos/:owner/:repo` → `owner.type = "User"` |
| 当前只有旧式 branch protection，无 rulesets | `gh api repos/:owner/:repo/rulesets` → `[]` |
| 旧 protection 无 bypass 名单，只有 `enforce_admins` 开关 | protection JSON；`enforce_admins.enabled = true` |
| Rulesets 支持 `bypass_actors`，actor_type 含 `Integration`、`OrganizationAdmin`、`RepositoryRole`、`Team`、`DeployKey`、`User` | GitHub REST schema |
| `bypass_mode` 取值：`always`、`pull_request`、`exempt`；`pull_request` 仅适用于分支规则集 | GitHub REST schema |
| `User` 类型的 `actor_id` 为数字用户 ID；本人 `dxinli` = `44126836` | `gh api users/dxinli` |
| 远程 `@claude` 当前优先使用 `CLAUDE_GITHUB_TOKEN`（PAT） | `.github/workflows/claude.yml` |
| PAT 的身份是 `dxinli`；若配置了 bypass，PAT 会命中 | GitHub 身份模型 |
| 本地 Claude Code 会设置 `CLAUDECODE=1`、`AI_AGENT=...` | 本次会话 `env` 实测 |
| 本地 agent 的 git 远程与本人相同 | `remote.origin.url = git@github.com:dxinli/collect-code.git` |

> 结论：规则集能挡住 App / `GITHUB_TOKEN` / bot，但挡不住“用本人 SSH key 的本地 agent”。后者必须靠本地手段或凭据隔离。

---

## 3. 方案选项

| 方案 | 做法 | 优点 | 缺点 | 结论 |
|---|---|---|---|---|
| **A. Rulesets + User bypass** | 规则集 A 管 PR/checks，bypass 仅 `dxinli`；规则集 B 管强推/删除 | 精确；本人可直推；agent 走 PR；B 不被 bypass | 需要删除旧 protection；需改 `claude.yml` token；本地同凭据仍可绕过 | **目标方案（暂缓）** |
| **B. 旧 protection + `enforce_admins=false`** | 关闭 `enforce_admins` | 改动最小，本人可直推 | 管理员同时获得强推/删分支能力；用本人 PAT 的 agent 也绕过；无审计区分 | 过渡期可短期使用，不推荐长期 |
| **C. 保持现状** | 什么都没改 | 最安全；无迁移风险 | 文档改动仍要 PR | **本轮选择** |
| **D. 暂时移除 required checks** | 保留 PR，去掉 `ci`/`gitleaks` | 文档 PR 更快 | 放弃 CI 门禁；与用户“先不做硬约束”一致但代价大 | 不推荐 |

> 用户已明确：本轮选 **C**，等文档稳定后再评估 **A**。

---

## 4. 目标设计（方案 A，暂不执行）

### 4.1 规则集 A：`main-pr`

- 目标：`~DEFAULT_BRANCH`
- 规则：
  - `pull_request`
    - `required_approving_review_count`：**待定**（0 或 1，见 §9 Q1）
    - `dismiss_stale_reviews_on_push`：`true`
    - `require_code_owner_review`：`false`
    - `require_last_push_approval`：`false`
    - `required_review_thread_resolution`：`true`
  - `required_status_checks`
    - `strict_required_status_checks_policy`：`true`
    - required checks：`ci`、`gitleaks`
- Bypass：
  - `{ "actor_type": "User", "actor_id": 44126836, "bypass_mode": "always" }`
  - 若个人仓库 API 不接受 `User`，回退 `{ "actor_type": "RepositoryRole", "actor_id": <admin role id> }`，执行时用 API 校验。

### 4.2 规则集 B：`main-immutable`

- 目标：`~DEFAULT_BRANCH`
- 规则：`non_fast_forward`、`deletion`
- Bypass：无

> 两个规则集独立求值。A 的 bypass 只豁免 A，不豁免 B。因此本人可直推，但不能强推/删 `main`。

### 4.3 凭据分离（决定“谁是 agent”）

- **远程 `@claude`**：`claude.yml` 不能再用本人 PAT。改为 GitHub App installation token（`Integration` actor，不在 bypass 名单），或退回 `GITHUB_TOKEN`。推荐 App token，因为其 push 仍会触发 CI。
  - 仓库已有 `TICKETS_APP_ID` / `TICKETS_APP_PRIVATE_KEY` / `TICKETS_APP_CLIENT_ID`，可评估复用；或新建专用 App。
- **本地 Claude Code**：仍使用本人 SSH key，平台层无效。可选：
  - 软约束：`.githooks/pre-push` 在 `CLAUDECODE=1` 且目标 `refs/heads/main` 时拒绝；`make hooks` 安装；`.claude/settings.json` deny `Bash(git push --no-verify:*)`。
  - 硬隔离：agent 使用独立 bot 账号/App token，并以独立 OS 用户或容器运行，admin key 仅存在于本人会话。
- 用户本轮选择：**本地先不做**。

### 4.4 AGENTS.md 规则（待执行时补）

- agent 不得直接 push `main`，所有文件变更走 PR；
- 本地 hook 在 `CLAUDECODE` 会话下拦截直推；
- 规则集 B 对所有人禁止强推/删除 `main`。

### 4.5 明确不做的 CI 改动

- 不加入 `paths`/`paths-ignore`；
- 不做 docs-only 快车道；
- 不调整 `make ci` 的检查集合。

原因：用户明确“暂不做 CI 层面的硬约束”，且当前文档 PR 的 CI 实测只需 13–45 秒，不是瓶颈。

---

## 5. 过渡期策略（需用户单独批准）

默认：**保持现状（方案 C）**。如果调整文档期间确实需要自由直推，按风险从低到高：

1. **保持现状**：继续 PR。风险最低，无平台改动。
2. **`enforce_admins=false`**：本人可直推；但也能强推/删 `main`，且使用本人 PAT 的 agent 同样绕过。建议仅在短时间窗内使用，完成文档调整后立即恢复。
3. **提前落地规则集 A/B**：即目标方案；需要同时改 token，否则远程 agent 仍可用 PAT 绕过。
4. **移除 required checks**：不推荐；PR 仍存在但没有 CI 门禁。

> 无论选哪种过渡方案，都必须先保存当前 protection JSON（见附录 A），并优先“先建后删”。

### 5.1 已选过渡方案（2026-11-23）

用户选择：**由本人执行最小改动，关闭 `enforce_admins`**。当前 protection JSON 已保存（附录 A）。

```bash
# 放宽：允许 admin 直推 main
gh api -X DELETE repos/dxinli/collect-code/branches/main/protection/enforce_admins

# 验证：应输出 false
gh api repos/dxinli/collect-code/branches/main/protection --jq .enforce_admins.enabled

# 文档稳定后恢复：重新开启 admin 约束
gh api -X POST repos/dxinli/collect-code/branches/main/protection/enforce_admins

# 恢复后验证：应输出 true
gh api repos/dxinli/collect-code/branches/main/protection --jq .enforce_admins.enabled
```

执行者与原因：

- 该命令是安全削弱操作，agent 侧的自动权限分类器已拒绝执行；
- 由用户本人在会话输入框用 `!` 前缀执行，或在自己的终端执行；
- agent 只负责验证与记录，不代为关闭。

窗口期代价：

- admin（本人）可直推、强推、删除 `main`；
- 使用本人 PAT 的 agent（含当前 `claude.yml` 的 `CLAUDE_GITHUB_TOKEN`）同样绕过；
- 其他 actor 仍受 PR + `ci`/`gitleaks` 约束。

退出条件：文档调整完成且用户说“恢复保护”后，立即执行 POST 恢复并验证 `enforce_admins=true`。窗口期内不创建规则集、不改 CI、不改 `claude.yml`。

---

## 6. 实施步骤（文档稳定后执行）

### Phase 0：快照与预检

1. 保存当前 protection JSON（附录 A）；
2. 确认 `gh api repos/:owner/:repo/rulesets` 仍为空或列出既有规则集；
3. 确认 `CLAUDE_GITHUB_TOKEN` 的用途与权限（无法读取值，需人工确认是否为本人 PAT）；
4. 确认 GitHub App 是否已安装到本仓库、是否具备 `contents: write`、`pull-requests: write`、`issues: write`。

### Phase 1：创建规则集 A/B

1. `POST /repos/:owner/:repo/rulesets` 创建 A，校验 bypass 是否接受 `User`；
2. 创建 B；
3. 用 `GET /repos/:owner/:repo/rulesets/{id}` 复核规则与 bypass；
4. **不删除旧 protection**，先验证规则集已生效。

### Phase 2：删除旧 protection

1. `DELETE /repos/:owner/:repo/branches/main/protection`；
2. 立即验证本人直推、Agent 直推、强推/删除四条路径。

### Phase 3：改 `claude.yml` 凭据

1. 用 `actions/create-github-app-token` 生成 App token，或显式改用 `GITHUB_TOKEN`；
2. 移除 `CLAUDE_GITHUB_TOKEN` 的 PAT 回退；
3. 验证 App token push 能触发 CI。

### Phase 4：本地约束（若届时选择）

1. 新增 `.githooks/pre-push` 与 `make hooks`；
2. 可选 `.claude/settings.json` deny 规则；
3. `AGENTS.md` 补充 agent 推送规则。

### Phase 5：验证与文档

1. 按 §7 矩阵逐项验证；
2. 更新 `AGENTS.md` / `docs/agents/issue-tracker.md`（如需）；
3. 记录最终配置与回滚步骤。

---

## 7. 验收矩阵

| 场景 | 期望 |
|---|---|
| 本人终端 `git push origin main`（fast-forward） | 成功 |
| 本人 `git push --force origin main` | 被规则集 B 拒绝 |
| 本人删除 `main` | 被规则集 B 拒绝 |
| 远程 agent（App/`GITHUB_TOKEN`）直推 `main` | 被规则集 A 拒绝 |
| 远程 agent 推 feature 分支并开 PR | 成功 |
| agent PR 的 `ci`/`gitleaks` | 正常触发并通过 |
| agent 合并 PR（Q1=1 approve 时） | 被拒，等待本人 approve |
| 本地 agent（软约束）`git push origin main` | pre-push hook 拒绝 |
| 本地 agent `git push --no-verify origin main` | 被 deny 规则或 hook 说明拦截（软约束可被有意绕过） |

---

## 8. 风险与回滚

| 风险 | 缓解 |
|---|---|
| 删除旧 protection 后规则集未生效，出现空窗 | 先建规则集并验证，再删旧 protection |
| `User` bypass 在个人仓库不被接受 | 回退 `RepositoryRole: admin`；执行前用 API 试建并校验 |
| `CLAUDE_GITHUB_TOKEN` 是本人 PAT，导致远程 agent 绕过 | Phase 3 必须一起改；未改 token 前不要宣布“agent 已受约束” |
| 本人获得直推后误强推/删分支 | 规则集 B 无 bypass，平台层兜底 |
| 本地 agent 与本人同凭据 | 软约束仅防误操作；要硬保证需凭据隔离 |
| 规则集配置错误导致本人也被挡 | 保留附录 A，可恢复旧 protection；规则集可删除重建 |

回滚：

1. 删除新建规则集；
2. 用附录 A 的 JSON 通过 `PUT /repos/:owner/:repo/branches/main/protection` 恢复旧 protection；
3. 还原 `claude.yml` 与本地 hook 改动。

---

## 9. 待决问题

| 编号 | 问题 | 选项 | 当前状态 |
|---|---|---|---|
| Q1 | agent 的 PR 是否需要本人 1 个 approve 才能合并？ | 0（CI 绿即可）/ 1（人工 gate） | 未决 |
| Q2 | 本地 Claude Code 的直推约束？ | 软约束 / 凭据隔离 / 不做 | 用户已选“先不做” |
| Q3 | 文档调整期间是否临时放宽当前 protection？ | 保持现状 / `enforce_admins=false` / 提前落地规则集 / 移除 checks | 已决：本人临时关闭 `enforce_admins`（§5.1） |
| Q4 | 远程 agent 复用 `TICKETS_APP` 还是新建专用 App？ | 复用 / 新建 | 未决 |
| Q5 | 规则集建立后是否保留 `strict_required_status_checks_policy`？ | 保留 / 关闭 | 未决，默认保留 |

---

## 附录 A：当前 protection 快照（`2026-11-23`）

```json
{
  "required_status_checks": {
    "strict": true,
    "contexts": ["ci", "gitleaks"],
    "checks": [
      {"context": "ci", "app_id": 15368},
      {"context": "gitleaks", "app_id": 15368}
    ]
  },
  "required_pull_request_reviews": {
    "dismiss_stale_reviews": true,
    "require_code_owner_reviews": false,
    "require_last_push_approval": false,
    "required_approving_review_count": 0
  },
  "required_signatures": {"enabled": false},
  "enforce_admins": {"enabled": true},
  "required_linear_history": {"enabled": false},
  "allow_force_pushes": {"enabled": false},
  "allow_deletions": {"enabled": false},
  "block_creations": {"enabled": false},
  "required_conversation_resolution": {"enabled": true},
  "lock_branch": {"enabled": false},
  "allow_fork_syncing": {"enabled": false}
}
```

恢复命令（仅在需要回滚时执行）：

```bash
gh api -X PUT repos/dxinli/collect-code/branches/main/protection \
  --input /tmp/main-protection-restore.json
```

## 附录 B：规则集草案（仅记录，不执行）

```json
{
  "name": "main-pr",
  "target": "branch",
  "enforcement": "active",
  "conditions": {
    "ref_name": {"include": ["~DEFAULT_BRANCH"], "exclude": []}
  },
  "bypass_actors": [
    {"actor_id": 44126836, "actor_type": "User", "bypass_mode": "always"}
  ],
  "rules": [
    {
      "type": "pull_request",
      "parameters": {
        "required_approving_review_count": 0,
        "dismiss_stale_reviews_on_push": true,
        "require_code_owner_review": false,
        "require_last_push_approval": false,
        "required_review_thread_resolution": true
      }
    },
    {
      "type": "required_status_checks",
      "parameters": {
        "strict_required_status_checks_policy": true,
        "required_status_checks": [
          {"context": "ci"},
          {"context": "gitleaks"}
        ]
      }
    }
  ]
}
```

```json
{
  "name": "main-immutable",
  "target": "branch",
  "enforcement": "active",
  "conditions": {
    "ref_name": {"include": ["~DEFAULT_BRANCH"], "exclude": []}
  },
  "bypass_actors": [],
  "rules": [
    {"type": "non_fast_forward"},
    {"type": "deletion"}
  ]
}
```

---

## 10. 执行前置条件

在用户明确说“按计划执行”之前：

1. 不创建/修改/删除任何规则集；
2. 不修改 `ci.yml`、`claude.yml`、`AGENTS.md`；
3. 不新增 `.githooks/**` 或 `.claude/settings.json`；
4. 本文档只作为评审材料。

**例外（§5.1）**：用户本人已于过渡窗口内关闭 `enforce_admins`；该操作由用户执行，agent 不代为执行，且文档稳定后必须恢复。
