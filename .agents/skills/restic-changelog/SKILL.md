---
name: restic-changelog
description: >-
  Polish or write restic user-facing changelog snippets under changelog/unreleased
  (calens-style). Use for changelog/unreleased, calens, or unreleased changelog files.
---

# Restic changelog (unreleased)

Entries live in `changelog/unreleased/` (one file per change) and are assembled by [calens](https://github.com/restic/calens). Match tone and length to `changelog/0.18.0_2025-03-27/`.

## Workflow

- Fix only what needs fixing (category, scope, links, grammar). Do not rephrase entries unnecessarily that already fit the project.
- Keep every existing GitHub issue/PR URL; do not invent links. High issue/PR numbers are normal. Do not rename the file unless asked.
- Before finishing: all original restic issue/PR links are still present and correct; add a **pull/** link when applicable (for example the merge that fixed the bug), even in files named `issue-*`.
- Validate with `calens` from the repository root; if `calens` is not on `PATH`, use `~/go/bin/calens`.

## Style

- **Title**: `Bugfix:` / `Enhancement:` / `Change:` + specific summary. A subcommand in the title may stay short as `` `backup` ``; `` `restic backup` `` is fine too.
- **Body**: Short prose (often one block); past vs present where it helps. No hype, no “you”/“your”. Prefer **the `cmd` command** for subcommands; **`` `restic cmd` ``** alone is fine when clearer. **Do not** write **`` the `restic cmd` command ``** (redundant: the backticked span already names the `restic` CLI). That rule does **not** forbid **`` the `backup -v` command ``** or **`` the `key passwd` command ``**: there the backticks wrap a subcommand (and optional flags), not `restic …`. Do not use a bare `` `cmd` `` alone when naming the CLI entrypoint.
- **Precision**: For script-visible behavior (exit codes, partial work, skipped files), pick accurate wording—for example *incomplete* backup vs *failed* backup—so readers infer the right automation semantics.
- **Env vars**: Say “the environment variable `NAME`”, not only `` `NAME` ``.
- **Links**: Blank line, then one `https://github.com/restic/restic/issues/…` or `…/pull/…` per line (do not break URLs). Include at least one **pull/** URL when the change merged via GitHub (usual case).
- **Layout**: Wrap prose around **80 columns** when practical. Use a fenced code block only when a short example helps.

## Avoid

Churn on already-good text; mini design docs; vague marketing; headline grammar mistakes; the redundant phrase **`` the `restic cmd` command ``** (see **Body** above—do not over-apply to **`` the `backup -v` command ``**-style phrasing).

## Skeleton

```markdown
Enhancement: Summary with `backup` or similar in the title if helpful

The `backup` command … (or `restic backup` …). Problem vs fix in plain language.

https://github.com/restic/restic/issues/NNNN
https://github.com/restic/restic/pull/NNNN
```

See **Workflow** for `calens` (and link checks) after editing.
