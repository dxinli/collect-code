# Issue tracker: Local Markdown

Issues and plans for this repo live as markdown files in `.scratch/`.

## Conventions

- One feature per directory: `.scratch/<feature-slug>/`
- The current behavior spec is `docs/spec/spec.md`; contracts are under `docs/contracts/`. `.scratch/<feature-slug>/` holds issues and plans only.
- Implementation issues are one file per ticket at `.scratch/<feature-slug>/issues/<NN>-<slug>.md`, numbered from `01`, never a single combined tickets file
- Triage state is recorded as a `Status:` line near the top of each issue file
- Comments and conversation history append to the bottom of the file under a `## Comments` heading

## When a skill says "publish to the issue tracker"

Create a new file under `.scratch/<feature-slug>/` (creating the directory if needed).

## When a skill says "fetch the relevant ticket"

Read the file at the referenced path. The user will normally pass the path or the issue number directly.

## Ticket state and automation

`Status:` uses the five mattpocock triage labels:

- `needs-triage`: not yet triaged.
- `needs-info`: missing information.
- `ready-for-agent`: fully specified, AFK-ready.
- `ready-for-human`: needs a human decision or action.
- `wontfix`: not going to be done.

Lifecycle is recorded with two optional fields, not Status:

- `**PR:** #N`: written when the PR opens; derived state `in-review`.
- `**Merged:** #N`: written by the post-merge bot; derived state `done`.

Dependencies are computed: `ready now` = `ready-for-agent` + all dependencies
merged; `blocked` = `ready-for-agent` + an unmerged dependency. `in-review` and
all `needs-*` / `ready-for-human` / `wontfix` states keep dependents blocked.

`feat/*` PRs must declare the delivered ticket in the PR body:

```md
- **Ticket:** 02b
- **Ticket:** 01a,01b,01c
```

A `ticket:<id>` label also works; the branch name `feat/<id>-` is the fallback.

Commands:

```bash
make tickets                         # generate .scratch/collector/ticket-dependency.html locally
make tickets TICKET_DIR=.scratch/<feature>
make tickets-guard                   # reject Merged on feat/* PRs (CI)
make tickets-pr TICKET=05a PR=19     # record **PR:** #19 when the PR opens
make tickets-accept TICKET=02b       # record Merged locally
```

When a ticket PR opens, run `make tickets-pr` (or add `**PR:** #N` manually)
and commit the issue file. The post-merge bot writes `**Merged:** #N`; do not
write either field from a feature branch except `PR:`.

`ticket-dependency.html` is a **local, gitignored** view. Do not commit it;
the post-merge bot only writes `**Merged:**`, not the HTML.

Merge is the completion signal. The workflow runs on `push` to `main`, finds
the merged PR associated with the commit, and writes `**Merged:** #N`. The
generated dependency page is local-only. To retry after a failure, comment
`/accept 02b` on the merged PR, or run the `Ticket Sync` workflow with
`ticket=02b`:

```bash
gh workflow run tickets-sync.yml -f ticket=02b
```

Only the bot writes `Merged:`; `tickets-guard` fails a `feat/*` PR whose ticket
is already merged.

## Complete-ticket walkthroughs

A complete ticket is every ticket sharing a numeric prefix (`01a/01b/01c`,
`02a/02b`, `06a/06b`); a single ticket such as `07` is its own group. Once
every member is merged (`**Merged:**` set), a committed
`docs/代码走读/<NN>-*.md` must exist. The
document covers the link path (with a diagram), component or object
relationships (with a dependency diagram), and timing (with a sequence diagram
where applicable).

The check runs after merge, not on the PR: the `walkthroughs` job in
`Ticket Sync` runs on every push to `main` and fails while a complete group has
no committed walkthrough. It neither changes `done` nor blocks dependents.
Untracked drafts do not count. Check locally with:

```bash
make tickets-walkthrough
```
