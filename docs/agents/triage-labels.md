# Triage Labels

The `triage` skill speaks in terms of five canonical state roles and two
category roles. Execution state lives in the native GitHub issue status; this
file maps the roles to status plus the remaining labels. The state role is the
status itself: do not require a state label, and do not flag its absence as a
conflict.

| Role in mattpocock/skills | Tracker representation | Meaning |
|---|---|---|
| `needs-triage` | status `Backlog` + label `needs-triage` | Maintainer needs to evaluate |
| `needs-info` | status `Backlog` + label `needs-info` | Waiting on reporter |
| `ready-for-agent` | status `Todo`, no `human` label | Fully specified, ready for an AFK agent |
| `ready-for-human` | status `Todo` + label `human` | Requires human implementation |
| `wontfix` | close with reason `not planned` (status `Not planned`) | Will not be actioned |
| `bug` | label `bug` | Something is broken |
| `enhancement` | label `enhancement` | New feature or improvement |

Triage transitions:

```text
unlabeled -> status Backlog + label needs-triage
needs-triage -> needs-info (status Backlog + label needs-info)
needs-triage -> ready-for-agent (status Todo, no human)
needs-triage -> ready-for-human (status Todo + label human)
needs-triage -> wontfix (close as not planned)
needs-info -> needs-triage (after the reporter replies)
```

Apply the role by writing status and labels together. Remove `needs-triage` and
`needs-info` when moving to `Todo`, and remove `needs-info` when returning to
`needs-triage`. Never add an `in-progress` or `done` label; execution uses
`In progress` / `Done` status.

- `human` is routing only. An issue with it is `Todo` but must not be claimed
  by an agent.
- `spec-parent` marks the single spec anchor. It is a container with `Backlog`
  status, not a triage state, and is never claimed. Only spec tickets are its
  sub-issues; ordinary and chore/hotfix tickets are standalone.
- `plan-review` is PR-only; it marks a ticket PR whose plan is waiting for
  review. `code-review` is PR-only; it marks an implementation review pending.
  The reviewer only comments (`/plan-approved <sha>` / `/code-approved
  <head-sha>`); the agent adds and removes both as bookkeeping. `guard`
  enforces the record only and does not read these labels (ADR-0013).
- `blocked` is not a label or status. It is read live from native issue
  dependencies; an issue may be `Todo` with an open blocker, and the claim query
  filters it out with `-is:blocked`.

`wontfix` closes the issue as `not planned`. A rejected enhancement also gets a
record under `.out-of-scope/<concept>.md`; a rejected bug or an
already-implemented request does not.

`ready-for-agent` and `ready-for-human` require an agent brief comment. The
issue body is the brief for tickets created in this repo; use the fields in
`.github/ISSUE_TEMPLATE/ticket.md`.
