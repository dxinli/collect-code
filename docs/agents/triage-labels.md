# Triage Labels

The `triage` skill speaks in terms of five canonical state roles and two
category roles. This file maps those roles to the label strings used in this
repo's issue tracker.

| Role in mattpocock/skills | Label in our tracker | Meaning |
|---|---|---|
| `needs-triage` | `needs-triage` | Maintainer needs to evaluate this issue |
| `needs-info` | `needs-info` | Waiting on reporter for more information |
| `ready-for-agent` | `ready-for-agent` | Fully specified, ready for an AFK agent |
| `ready-for-human` | `ready-for-human` | Requires human implementation |
| `wontfix` | `wontfix` | Will not be actioned |
| `bug` | `bug` | Something is broken |
| `enhancement` | `enhancement` | New feature or improvement |

At triage time every issue carries exactly one of the five triage state labels
above and exactly one category label. Triage transitions:

```text
unlabeled -> needs-triage -> needs-info / ready-for-agent / ready-for-human / wontfix
needs-info -> needs-triage (after the reporter replies)
```

Execution and terminal labels are separate from the triage five:

- `in-progress`: an agent has claimed a `ready-for-agent` issue and is implementing it.
- `done`: added by the `issue-done` workflow when a completed issue closes.
- `plan-review`: PR-only; marks a ticket PR whose plan is waiting for review.
  It is not an issue state.

`wontfix` closes the issue. A rejected enhancement also gets a record under
`.out-of-scope/<concept>.md`; a rejected bug or an already-implemented request
does not.

`ready-for-agent` and `ready-for-human` require an agent brief comment. The
issue body is the brief for tickets created in this repo; use the fields in
`.github/ISSUE_TEMPLATE/ticket.md`.
