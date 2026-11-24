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

Do not add execution labels such as `in-progress` or `done`; status covers them.

## Spec anchor and sub-issues

There is one durable spec anchor issue titled `Spec: 采集引擎行为契约`, carrying
`spec-parent`. It is the parent of every ticket and points at the authoritative
spec (`docs/spec/spec.md`), never copying it. It carries `Backlog` (a status
cannot be cleared once set) and is never closed or modified by ticket work;
`spec-parent` keeps it out of the claim queue.

Create a ticket as its sub-issue:

```bash
gh issue create --parent <anchor> --title "05a: ..." --body-file <file>
gh issue edit <anchor> --add-sub-issue <n>
```

List sub-issues:

```bash
gh api --paginate repos/<owner>/<repo>/issues/<anchor>/sub_issues --jq '.[].number'
```

## Claiming

The startable set is:

```bash
gh issue list --state open --search "status:todo -label:human -is:blocked"
```

Before writing, re-read the issue: `status` must still be `Todo` and
`issue_dependencies_summary.blocked_by` must be `0`. Then claim with the status
write above and comment on the issue. If the re-read shows `In progress`, skip
to the next ticket. An issue with `human` or an open blocker is never claimed.

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
`in-progress` or `done` label.

## Pull requests

Ticket PRs must contain the closing keyword so GitHub closes the issue when
the PR merges:

```md
Closes #31
```

That is the whole post-merge workflow. There is no ticket-sync bot and no
`Merged:` field; the issue timeline and its `closed` state are the record. When
the PR is marked ready, set the issue status to `In review`.

The required `guard` check enforces this on `feat/*` and `hotfix/*` PRs. It
also rejects code-file changes while a PR carries the `plan-review` label.

Plan-only PRs do not need a `Closes` line.

## When a skill says "publish to the issue tracker"

Create a GitHub issue as a sub-issue of the spec anchor.

## When a skill says "fetch the relevant ticket"

Read it with `gh issue view`; the implementation plan is in the ticket PR
that contains `Closes #<issue>`.

## Pull requests as a triage surface

**PRs as a request surface: no.** `/triage` only looks at issues in this repo.

## Complete-ticket walkthroughs

A complete ticket group (every ticket sharing a numeric prefix: `01a/01b/01c`,
`02a/02b`, `06a/06b`) is closed once all its issues are closed. At that point a
committed `docs/代码走读/<NN>-*.md` is expected: the link path with a diagram,
component relationships with a dependency diagram, and timing with a sequence
diagram where applicable. This is a review artifact, not a CI gate; check the
tracked files with `git ls-files 'docs/代码走读/*.md'`.
