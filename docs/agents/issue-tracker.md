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
| `code-review` | PR-only: the implementation review is pending; remove before auto-merge |

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

## Pull requests

Ticket PRs must contain the closing keyword so GitHub closes the issue when
the PR merges:

```md
Closes #31
```

That is the whole post-merge workflow. There is no ticket-sync bot and no
`Merged:` field; the issue timeline and its `closed` state are the record.

After implementation:

1. Run the `code-review` skill (self-review) and fix findings.
2. Set the issue status to `In review`, run `gh pr ready`, and add the
   `code-review` label.
3. A human reviews the diff and removes `code-review`.
4. Only then run `gh pr merge --auto --squash`.

If the PR is closed without merging, set the issue status back to `Todo` (still
ready) or `Backlog`, and comment why.

The required `guard` check enforces `Closes #N` on `feat/*` and `hotfix/*` PRs.
When a PR carries the `plan-review` label, it rejects code-file changes on the
next workflow run. Label changes do not trigger a run, so re-run the workflow
after adding the label if you need the current diff checked.

Plan-only PRs do not need a `Closes` line.

## When a skill says "publish to the issue tracker"

Create a GitHub issue using the ticket template. Classify it by
[Spec and tickets (A)](#spec-and-tickets-a): a spec ticket is a native
sub-issue of #71; an ordinary ticket is standalone.

## When a skill says "fetch the relevant ticket"

Read it with `gh issue view`; the implementation plan is in the ticket PR
that contains `Closes #<issue>`.

## Pull requests as a triage surface

**PRs as a request surface: no.** `/triage` only looks at issues in this repo.
