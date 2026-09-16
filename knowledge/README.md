# Knowledge Repository

Versioned engineering knowledge (spec §13, §21). Markdown + Git is the source of truth. Obsidian is only a viewer: open this folder as a vault.

## Scopes

Knowledge flows **down**, never sideways.

```text
global/              → every task
stacks/<stack>/      → tasks whose repository is on that stack
clients/<slug>/      → every project of that client, and nothing else
projects/<slug>/     → that project only (all its repositories)
```

Repository-specific knowledge lives in the repository itself (README, CLAUDE.md, docs/).

| Folder | Holds |
|--------|-------|
| [global/](global/) | Engineering and security principles |
| [stacks/go/](stacks/go/) | Go architecture, testing, libraries |
| [stacks/dotnet/](stacks/dotnet/) | .NET architecture, EF Core, testing |
| [stacks/node/](stacks/node/) | Node/frontend guidance |
| [clients/](clients/) | One folder per client: conventions, compliance, branding |
| [templates/](templates/) | Document templates (M3.1) |
| [contracts/](contracts/) | Document contracts. Start with [frontmatter.md](contracts/frontmatter.md) |
| [projects/](projects/) | One folder per project. [example/](projects/example/) shows the layout |

## Rules

- Every document has frontmatter. See [contracts/frontmatter.md](contracts/frontmatter.md).
- One concern per document. No giant files.
- Approved versions are immutable. Change them through a new version.
