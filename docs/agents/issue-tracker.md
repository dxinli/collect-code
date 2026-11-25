# Issue tracker: GitHub

Issues for this repo live as GitHub issues. Use the `gh` CLI for all
operations. The execution state is the native GitHub issue status; labels are
classification and routing only. A ticket's implementation plan lives in its
PR body; diff notes and archived plans stay under `.scratch/` (see
`.scratch/README.md`).

## Status

The only execution state is the native issue status:

| Status | Meaning |
|---|---|
| `Backlog` | Not ready: needs triage, needs info, or not yet specified |
| `Todo` | Fully specified; ready to be claimed |
| `In progress` | Claimed and being implemented |
| `In review` | The ticket PR is ready for review |
| `Done` | Closed as completed |
| `Not planned` | Closed as not planned |
| `Duplicate` | Closed as a duplicate |

Read one issue's status, labels and open-blocker count:

```bash
gh api repos/<owner>/<repo>/issues/<n> \
  --jq '{number,state,status:.status.name,labels:[.labels[].name],blocked_by:.issue_dependencies_summary.blocked_by}'
```

Write a status:

```bash
gh api -X PATCH repos/<owner>/<repo>/issues/<n> -f status="In progress"
```

Valid values are exactly the seven names above. Setting a status also updates
`state` / `state_reason` as needed: never write `Backlog`, `Todo`,
`In progress`, or `In review` to a closed issue. The terminal statuses are
`Done`, `Not planned`, and `Duplicate`.

There is no ongoing status automation. Humans and agents write status at the
moment of the transition:

- New issue: `Backlog` (the creator or triager sets it; the template also
  carries `needs-triage`).
- Triage to ready: `Todo` (add `human` for human-only work).
- Claim: `In progress`.
- `gh pr ready`: `In review`.
- Merge: `Done` (native automation; verify and set manually if it did not run).
- Close as not planned / duplicate: `Not planned` / `Duplicate`.
- Reopen: `Todo` if the issue is still fully specified and unclaimed, otherwise
  `Backlog`.

`gh issue view` (2.104) does not expose `status` as a `--json` field; use the
REST call above.

## Labels

Labels are classification and routing, never execution state:

| Label | Role |
|---|---|
| `bug` / `enhancement` | category |
| `needs-triage` | Backlog: maintainer still evaluating |
| `needs-info` | Backlog: waiting on the reporter |
| `human` | routing: the issue must be done by a human |
| `spec-parent` | the single spec anchor; not a work item |
| `plan-review` | PR-only: the plan is waiting for review |
| `code-review` | PR-only: implementation review is pending; the reviewer records `/code-approved <head-sha>` and removes it |

Do not add execution labels such as `in-progress` or `done`; status covers them.

## Spec and tickets (A)

The spec lives only in `docs/spec/spec.md`; changes go through a
`spec-delta/<topic>` PR. Do not create a spec issue. The single spec anchor #71
(`spec-parent`) is the parent of every **spec ticket** and of nothing else. Its
native sub-issue list is the ticket list: attaching a ticket extends it, and
ticket work never edits the anchor's body. The anchor points at the
authoritative spec, never copying it. It carries `Backlog` (a status cannot be
cleared once set) and is never closed or modified by ticket work; `spec-parent`
keeps it out of the claim queue.

A ticket is one of two kinds:

- **Spec ticket** — a slice cut from the spec, a split-out of an existing spec
  ticket, or a fix of an implementation ticket. Its title is
  `<NN>[<letter>]: <标题>`: a chapter with a single slice omits the letter
  (`07:`, `20:`); a split-out or fix takes the next unused letter in that
  numeric group (`01b` splits to `01c`; `06a`/`06b` add `06c`). It is a native
  sub-issue of #71.
- **Ordinary ticket** — docs, tests, scripts, a non-ticket problem, or code
  spanning several tickets. It is standalone: no parent, and its title needs no
  `NNx:` number (`docs:`, `test:`, `chore(...)`, `fix:`, … are fine).
  chore/hotfix titles are ordinary; a hotfix of a spec ticket's implementation
  is created as a spec ticket instead.

Create a spec ticket:

```bash
gh issue create --parent 71 --title "05a: ..." --template ticket.md
```

An ordinary ticket omits `--parent`:

```bash
gh issue create --title "docs: ..." --template ticket.md
```

`scripts/ticket-new.sh` wraps creation plus the status/label/dependency writes:

```bash
scripts/ticket-new.sh --title "05a: ..." --body-file /tmp/brief.md --spec-ticket
scripts/ticket-new.sh --title "docs: ..." --body-file /tmp/brief.md
scripts/ticket-new.sh --title "chore(...): ..." --body-file /tmp/brief.md
scripts/ticket-new.sh --title "..." --body-file /tmp/brief.md --human --blocked-by 42,43 --dry-run
```

It rejects a brief missing any required template field (`**Ticket:**` with an
id, `**What to build:**`, `**Current behavior:**`, `**Desired behavior:**`,
`**Key interfaces:**`, `**Acceptance criteria:**`, `**Out of scope:**`). It
creates a standalone ticket by default; `--spec-ticket` attaches #71 and
requires a title matching `<NN>[<letter>]: ...`. It writes the status (`Todo` by
default, `--status Backlog` on request) and adds `human` / native `blocked-by`
edges. `--human` requires status `Todo`. The raw commands above are the
equivalent.

Without the script, set its status (`Todo` when the brief is complete,
otherwise `Backlog`) and add `human` when it is human-only. Ordinary tickets
omit `--parent`; spec tickets add it.

New requests start at `Backlog` + `needs-triage` through the template
(`gh issue create --template ticket.md`); `ticket-new.sh` is for tickets whose
brief is already complete. If the behavior changes, open a `spec-delta/<topic>`
PR and keep the ticket in `Backlog` until the spec is accepted, then move it to
`Todo`.

List sub-issues:

```bash
gh api --paginate repos/<owner>/<repo>/issues/71/sub_issues --jq '.[].number'
```

External `to-spec`, `to-tickets`, and `implement-spec` are not part of the
current flow. When a repo-local adapter is written, map `ready-for-agent` to
status `Todo` and `ready-for-human` to `Todo` + `human`; do not create those
labels.

## Claiming

The startable set is:

```bash
gh issue list --state open --search "status:todo -label:human -is:blocked"
```

Before writing, re-read the issue: `status` must still be `Todo` and
`issue_dependencies_summary.blocked_by` must be `0`. Then claim with the status
write above and comment on the issue. If the re-read shows `In progress`, skip
to the next ticket. An issue with `human` or an open blocker is never claimed.

`scripts/claim.sh` runs the query, re-reads every candidate, and only then
writes `In progress` and leaves the claim comment:

```bash
scripts/claim.sh        # first startable issue
scripts/claim.sh 76     # a specific issue
```

Without an argument it skips candidates that are no longer startable; with a
number it exits non-zero instead of writing.

## Dependencies (blocked)

Use GitHub's native issue dependencies. An issue is startable when every
blocker is closed.

```bash
gh issue edit <child> --add-blocked-by <blocker>,<blocker>
gh api repos/<owner>/<repo>/issues/<n> --jq .issue_dependencies_summary.blocked_by
```

`issue_dependencies_summary.blocked_by` counts only **open** blockers; `0`
means startable. `gh issue view <n> --json blockedBy` lists closed blockers
too, so filter them by state if you use it. The search qualifier `is:blocked`
is the cheap live filter.

Blocking is never copied into status or a label. The status may be `Todo` while
the issue still has an open blocker; the claim query and the re-read are the
gate.

Fallback when native dependencies are unavailable: put
`Blocked by: #<n>, #<n>` at the top of the child body.

## Triage labels

The canonical `triage` skill roles map to status plus labels in
[`triage-labels.md`](triage-labels.md). Execution uses status; there is no
`in-progress` or `done` label. The state role is the native status: do not
require an `in-progress` / `ready-for-agent` / `done` label, and do not treat
the absence of a state label as an error.

## Plan

A ticket PR's body carries the plan. Before writing it, verify every fact the
plan depends on and record the verification; an unverified assertion must not
enter the plan. The Plan section includes a **已核实事实** table whose rows
read `事实 | 核实方式 | 结论`:

| 事实 | 核实方式 | 结论 |
|---|---|---|
| `check-spec` 的 contracts/covers 读取 `internal/**` | 读 `scripts/spec-check.sh` 的 `spec_contracts` 与 `spec_covers` | 只改 Go 也必须跑 `check-spec` |
| 非 ASCII 路径会被 git C-quote | 在临时仓库执行 `git diff --name-only` | 脚本必须使用 `-c core.quotePath=false` |
| `actions/checkout` 默认 `fetch-depth: 1`，没有完整历史 | 读 action README 的 `fetch-depth` 默认值，并核对 `ci.yml` | 需要 merge-base/diff 的 job 必须 `fetch-depth: 0` |
| ADR-0007 已 Accepted，plan-review 期间只允许 `docs/`、`.scratch/`、`AGENTS.md`、`CLAUDE.md`、`GLOSSARY.md` | 读 `docs/adr/0007-main-protection-minimum-gate.md` 并对照 `guard` step | 白名单之外的根 `*.md`（如 `README.md`）不可用 |

The four required categories are defined in [AGENTS.md](../../AGENTS.md#plan);
this section defines the table format and shows one filled example. Each row
names the command, ADR, or document used to verify the fact. A fact that
cannot be verified is not written as an assertion: resolve it before plan
review, or stop and raise the open question on the issue. A real plan lists
every fact it depends on; the example above stays at four rows to show the
shape. The table is reviewed with the plan before the `plan-review` label is
removed.

The rule exists because four approved plans were invalidated by unverified
facts: #83 (`check-spec` reads Go), #87 (git C-quotes non-ASCII paths), #89
(third-party predicate semantics), #93 (an Accepted ADR's allowlist).

### Approval record

A code-file change needs a plan approval record: a PR conversation comment or a
submitted PR review by `github.repository_owner` whose body contains the single
line

```text
/plan-approved <sha>
```

`<sha>` is a plan-stage commit on the PR branch (typically the PR head at
approval time). `guard` accepts the record only when:

- the comment author is the repository owner;
- the SHA resolves and is an ancestor of the PR head;
- the SHA is not part of the base history (it must be introduced by this PR);
- `git diff <base>...<sha>` contains only the plan allowlist.

The comment timestamp is not compared with commit timestamps: a record
pointing at a plan-stage SHA is accepted even if it was written later. The gate
proves that a plan-stage commit was approved, not that approval happened before
code was pushed; human review remains the final gate.

Force-push or rebase that drops the SHA invalidates the record; comment again.
New code commits after a valid record do not invalidate it. A new plan-only
commit after the record also does not mechanically invalidate it; re-comment
when a plan change is material. Editing the plan in the PR body is not
machine-detectable for the same reason.

The `plan-review` label remains the waiting marker; removing it does not
replace the record. Label and comment changes do not trigger a workflow run, so
re-run the workflow (or push the next commit) after recording approval. Exempt
branches: `spec-delta/*`, `dependabot/*`, `probe/*`.

Inspect the current records:

```bash
{
  gh api --paginate "repos/<owner>/<repo>/issues/<n>/comments" \
    --jq '.[] | select(.user.login == "<owner>") | .body'
  gh api --paginate "repos/<owner>/<repo>/pulls/<n>/reviews" \
    --jq '.[] | select(.user.login == "<owner>") | .body'
} | grep '^/plan-approved '
```

## Pull requests

Every non-exempt PR must contain exactly one `Closes #N` line so GitHub closes
the issue when the PR merges:

```md
Closes #31
```

Other closing keywords (`Fixes`, `Resolves`, lowercase variants) and a second
closing reference are rejected. Closing lines inside fenced code blocks are
ignored.

The linked issue must be open, must not be #71 or a PR, and its body must carry
a real `**Ticket:** <id>`.

The PR body must also satisfy:

- `Spec-Version` equals `docs/spec/VERSION`;
- `Spec-Impact` is `none`, `patch`, `minor`, or `major`, and is not `none` when
  the PR changes `docs/spec/` or `docs/contracts/`;
- `REQ` is `none` or a list of `REQ-*` / contract anchors (ranges allowed).
  The value must be on the same line as `**REQ:**`; a separator-only value is
  rejected.

Exempt: `dependabot/*` and `probe/*` skip all checks; `spec-delta/*` skips the
`Closes`/issue checks but still requires `Spec-Version`, `Spec-Impact`, and
`REQ`. Plan-only ticket PRs carry `Closes #N` from creation, so there is no
separate plan-only exception.

That is the whole post-merge workflow. There is no ticket-sync bot and no
`Merged:` field; the issue timeline and its `closed` state are the record.

After implementation:

1. Run the `code-review` skill (self-review) and fix findings.
2. Set the issue status to `In review`, run `gh pr ready`, and add the
   `code-review` label.
3. The reviewer records code approval (see below) and removes `code-review`.
4. Re-run the workflow; once `guard` passes, run `gh pr merge --auto --squash`.

### Code review approval record

A non-draft, non-exempt PR needs a code review record: a PR conversation
comment or a `COMMENTED`/`APPROVED` review by `github.repository_owner` with the
single line

```text
/code-approved <head-sha>
```

`<head-sha>` must resolve to the head of the workflow run, and that head must
equal the live PR head. Any new commit, amend, rebase, or force-push
invalidates the record; the reviewer records the new head. `guard` also fails
while the `code-review` label is present. Draft PRs fail rather than skip: a
green draft check would leave a false-green window before the
`ready_for_review` run registers. Comments, reviews, and labels do not trigger a
run: after recording approval and removing the label, re-run the workflow.
Re-adding the label after a green run does not automatically invalidate it;
re-run if you do. Exempt: `dependabot/*`, `probe/*`.

Inspect the head and records:

```bash
gh pr view <n> --json headRefOid --jq .headRefOid
{
  gh api --paginate "repos/<owner>/<repo>/issues/<n>/comments" \
    --jq '.[] | select(.user.login == "<owner>") | .body'
  gh api --paginate "repos/<owner>/<repo>/pulls/<n>/reviews" \
    --jq '.[] | select(.user.login == "<owner>") | select(.state == "COMMENTED" or .state == "APPROVED") | .body'
} | grep '^/code-approved '
```

If the PR is closed without merging, set the issue status back to `Todo` (still
ready) or `Backlog`, and comment why.

The required `guard` check enforces the metadata and plan approval on every
non-exempt PR, and the code review record on every non-draft, non-exempt PR.
When a PR carries the `plan-review` label, it rejects code-file changes on the
next workflow run. Code-file changes also require a valid `/plan-approved <sha>`
record (see [Plan](#plan)). Label, comment, and review changes do not trigger a
run, so re-run the workflow after adding a label or recording approval.

## When a skill says "publish to the issue tracker"

Create a GitHub issue using the ticket template. Classify it by
[Spec and tickets (A)](#spec-and-tickets-a): a spec ticket is a native
sub-issue of #71; an ordinary ticket is standalone.

## When a skill says "fetch the relevant ticket"

Read it with `gh issue view`; the implementation plan is in the ticket PR
that contains `Closes #<issue>`.

## Pull requests as a triage surface

**PRs as a request surface: no.** `/triage` only looks at issues in this repo.
