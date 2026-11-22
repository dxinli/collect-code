# Domain Docs

How the engineering skills should consume this repo's domain documentation when exploring the codebase.

## Before exploring, read these

- **`docs/README.md`**: the document map and authority by fact type
- **`docs/spec/spec.md`**: the current behavior contract; start from the `REQ-*` that names the behavior
- **`docs/contracts/`**: read only the contract for the area you're working in
- **`GLOSSARY.md`** at the repo root
- **`docs/adr/`**: read ADRs that touch the area you're about to work in

Do not read the archived v2 spec, old plans, or `docs/代码走读/` by default. They are history, not current authority.

If any of these files don't exist, **proceed silently**. Don't flag their absence.

## File structure

Single-context repo:

```
/
├── GLOSSARY.md
├── docs/
│   ├── README.md
│   ├── spec/
│   ├── contracts/
│   └── adr/
└── src/          (or cmd/ + internal/ in this Go project)
```

## Use the glossary's vocabulary

When your output names a domain concept (issue title, refactor proposal, hypothesis, test name), use the term as defined in `GLOSSARY.md`. Don't drift to synonyms the glossary explicitly avoids.

## Flag ADR conflicts

If your output contradicts an existing ADR, surface it explicitly rather than silently overriding.
