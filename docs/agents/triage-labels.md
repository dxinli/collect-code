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

Every triaged issue carries exactly one state label and exactly one category
label. State transitions:

```text
unlabeled -> needs-triage -> needs-info / ready-for-agent / ready-for-human / wontfix
needs-info -> needs-triage (after the reporter replies)
```

`wontfix` closes the issue. A rejected enhancement also gets a record under
`.out-of-scope/<concept>.md`; a rejected bug or an already-implemented request
does not.

`ready-for-agent` and `ready-for-human` require an agent brief comment. The
issue body is the brief for tickets created in this repo; use the fields in
`.github/ISSUE_TEMPLATE/ticket.md`.
