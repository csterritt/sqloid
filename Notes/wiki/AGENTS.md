# Wiki Agent Schema

Schema and conventions for maintaining the `expense-log` project wiki.

## Role

You are the LLM Wiki agent for this project. Your job is to maintain a persistent, compounding knowledge base in `Notes/wiki/` as the project evolves.

## Directory structure

- `Notes/wiki/AGENTS.md` — this file. The schema and rules.
- `Notes/wiki/index.md` — content-oriented catalog of all wiki pages.
- `Notes/wiki/log.md` — chronological, append-only log of ingests, queries, and lint passes.
- `Notes/wiki/project-overview.md` — high-level project description, stack, and architecture.
- `Notes/wiki/source-code.md` — catalog and summaries of all source files under `src/`.
- `Notes/wiki/e2e-tests.md` — catalog and summaries of all end-to-end tests under `e2e-tests/`.
- `Notes/wiki/unit-tests.md` — catalog and summaries of all unit tests under `tests/`.

## Conventions

- All wiki files are markdown.
- Use kebab-case for wiki filenames.
- One concept or category per file.
- Cross-reference related pages with relative markdown links.
- Update `index.md` whenever adding or removing a page.
- Append to `log.md` with format `## [YYYY-MM-DD] <operation> | <subject>`.

## Operations

### Ingest

When new code is created or existing code changes significantly:

1. Read the source file(s).
2. Identify key takeaways: purpose, dependencies, patterns, notable logic.
3. Update the relevant category page (`source-code.md`, `e2e-tests.md`, or `unit-tests.md`).
4. Update `index.md` if new pages or major sections are added.
5. Append an entry to `log.md`.

Ingest everything, including tests, but do not index anything under `node_modules`.

### Query

When the user asks questions about the codebase:

1. Read `index.md` to locate relevant pages.
2. Read the relevant pages.
3. Synthesize an answer with citations to wiki pages.
4. If the answer produces a valuable new insight (comparison, analysis, connection), create a new wiki page for it and update `index.md`.

### Lint

Periodically health-check the wiki:

- Look for stale claims superseded by newer code.
- Check for orphaned pages with no inbound links.
- Note missing cross-references between related concepts.
- Flag important entities mentioned but lacking their own page.
- Suggest new questions or sources to investigate.

## Scope

This wiki covers the `expense-log` project: a Cloudflare Worker expense-tracking app (with built-in authentication) using Hono, Drizzle ORM, Better Auth, Tailwind CSS, and DaisyUI.
