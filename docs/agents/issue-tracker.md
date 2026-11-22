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

`Status:` is a two-phase state:

- `ready-for-agent`: dependencies are done; work can start.
- `in-review`: implementation is complete and a PR is open; **dependents stay blocked**.
- `done`: written only after the PR is merged, by the `tickets-sync` bot.
- `on-hold` / `needs-info`: human decisions; the bot never flips these.

`feat/*` PRs must declare the delivered ticket in the PR body:

```md
- **Ticket:** 02b
- **Ticket:** 01a,01b,01c
```

A `ticket:<id>` label also works; the branch name `feat/<id>-` is the fallback.

Commands:

```bash
make tickets                         # regenerate .scratch/collector/ticket-dependency.html
make tickets TICKET_DIR=.scratch/<feature>
make tickets-check                   # fail if the committed HTML is stale
make tickets-guard                   # reject done on feat/* PRs (CI)
make tickets-accept TICKET=02b       # mark done + regenerate (local)
make hooks                           # enable .githooks/pre-commit
```

Merge is the completion signal. `.github/workflows/tickets-sync.yml` marks the
ticket(s) `done`, regenerates the HTML, opens a `chore/tickets-sync-*` PR, and
auto-merges it after checks. To retry after a failure, comment `/accept 02b` on
the merged PR, or run the `Ticket Sync` workflow with `ticket=02b`:

```bash
gh workflow run tickets-sync.yml -f ticket=02b
```

Only the bot writes `done`; `tickets-guard` fails a `feat/*` PR whose ticket is
already `done`.
