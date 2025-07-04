# Rewind

**Simple, automatic version control for solo developers**


Rewind automatically saves every version of your files when they change, giving you instant access to your entire editing history. Perfect for when you need to quickly restore a file after an overzealous AI agent, a bad refactor, or just want to see what your code looked like yesterday.

## Why Rewind?

- 🔄 **Automatic**: No commits, no staging - just code and save
- ⚡ **Instant**: Restore any version of any file in seconds  
- 🎯 **Solo-friendly**: Built for individual developers, not teams
- 💾 **Local**: Everything stays on your machine
- 🧠 **Smart**: Ignores common files you don't want versioned (.git, node_modules, etc.)

## Quick Start

```bash
# Initialize rewind in your project
rewind init

# Start watching for changes (in another terminal)
rewind watch

# Check daemon status
rewind status

# View file history
rewind rollback src/main.js

# Rollback a file to a previous version
rewind rollback src/main.js --version 5

# Show differences between versions
rewind diff src/main.js --version 3
```

## Real-World Examples

### "Help! AI Agent Destroyed My Code"

You're working with an AI coding assistant that goes a bit too far with refactoring:

```bash
# Before: You have a working 200-line component
# AI Agent: "Let me optimize this for you!"
# After: Your component is now 5 lines and completely broken

# No problem - check the version history
rewind rollback components/UserProfile.tsx
# Shows: Version 15 (2 minutes ago) - working version before AI refactor

rewind rollback components/UserProfile.tsx --version 15
# ✅ Your working code is back instantly
```

### "I Deleted Something Important"

```bash
# Accidentally deleted a crucial function during cleanup
# Check if the file is in your deleted files

rewind restore
# Lists all deleted files with timestamps

# Or restore a specific deleted file
rewind restore utils/helpers.js
# ✅ Your deleted file is restored
```

### "What Did I Change?"

```bash
# See what changed between current version and version 3
rewind diff src/app.js --version 3

# View version history in different formats
rewind rollback src/app.js --json
rewind rollback src/app.js --csv
```

### "Experimental Feature Gone Wrong"

```bash
# You spent 2 hours implementing a feature that isn't working
# Check the file's version history
rewind rollback src/main.js

# Rollback to version from 2 hours ago
rewind rollback src/main.js --version 8 --confirm
# Your experiment is gone, but your working code is back
```

## Installation

### Download Binary

1. Download the latest release for your platform from [GitHub Releases](https://github.com/davenicholson-xyz/rewind/releases)
2. Extract and place the binary in your PATH
3. Make it executable: `chmod +x rewind`

### Build from Source

```bash
git clone https://github.com/davenicholson-xyz/rewind.git
cd rewind
go build
./rewind --version
```

## Commands

### Project Setup
- `rewind init [path]` - Initialize rewind in current or specified directory
- `rewind remove [--force]` - Remove rewind from current directory

### Daemon Control  
- `rewind watch` - Start the file watching daemon
- `rewind status` - Show daemon status and active watches

### File History
- `rewind rollback <file>` - Show version history for file
- `rewind rollback <file> --json` - Show history as JSON
- `rewind rollback <file> --csv` - Show history as CSV
- `rewind diff <file> [--version <n>]` - Show changes between versions

### Restore Operations
- `rewind rollback <file> --version <n>` - Rollback file to specific version
- `rewind rollback <file> --version <n> --confirm` - Rollback with confirmation
- `rewind restore` - List all deleted files for restoration
- `rewind restore <file>` - Restore specific deleted file
- `rewind restore --confirm` - Restore with confirmation prompts

## How It Works

1. **Initialize**: `rewind init` creates a `.rewind` directory with a SQLite database
2. **Watch**: `rewind watch` starts a daemon that monitors file changes using filesystem events
3. **Save**: Every time you save a file, rewind stores a copy with metadata and calculates a content hash
4. **Restore**: Access any previous version instantly through the CLI

## What Gets Versioned?

**Included by default:**
- Source code files (.js, .py, .go, .rs, etc.)
- Configuration files (.json, .yaml, .toml, etc.)  
- Documentation (.md, .txt, etc.)
- Any file you edit and save

**Excluded by default:**
- Git repositories (`.git/`)
- Dependencies (`node_modules/`, `vendor/`, etc.)
- Build artifacts (`dist/`, `build/`, etc.)
- Temporary files (`.tmp`, `.log`, etc.)
- IDE files (`.vscode/`, `.idea/`, etc.)

Customize what gets ignored by editing `.rewind/ignore` or creating `.rwignore` files in your project.

## Configuration

Rewind stores its configuration in `~/.config/rewind/`:
- `watchlist.json` - Projects being watched by the daemon

Project-specific settings in `.rewind/`:
- `ignore` - Files/patterns to ignore (like .gitignore)
- `versions/` - Stored file versions organized by path and timestamp
- `versions.db` - SQLite database with version metadata

## Performance

- **Storage**: Only stores files when content actually changes (deduplication by SHA256 hash)
- **Speed**: File operations are near-instant, even with thousands of versions
- **Memory**: Minimal overhead - the daemon uses ~10MB RAM typically
- **Disk**: Efficient storage with content-based deduplication

## vs Git

| Feature | Rewind | Git |
|---------|--------|-----|
| Setup | One command | Multiple steps |
| Commits | Automatic on save | Manual |
| Staging | None | Required |
| Solo dev | Perfect | Overkill |
| Teams | No | Yes |
| Branches | No | Yes |
| Remote sync | No | Yes |
| Speed | Instant | Fast |
| File restoration | Built-in | Manual checkout |

**Use Rewind when:** You want effortless local versioning for solo development

**Use Git when:** You need team collaboration, branching, or remote repositories

**Use Both:** Many developers use rewind for automatic local history and Git for project milestones and collaboration

## FAQ

**Q: Does rewind replace Git?**  
A: No, they solve different problems. Use rewind for automatic local versioning, Git for team collaboration and remote storage.

**Q: How much disk space does it use?**  
A: Only stores changed files once (deduplicated by content hash). A typical project might use 100-500MB for months of history.

**Q: Can I use it with Git?**  
A: Yes! Rewind ignores `.git/` by default. Many developers use both together.

**Q: What happens if I delete .rewind/?**  
A: You lose all version history, but your current files are unaffected. Just run `rewind init` to start fresh.

**Q: Does it work with large files?**  
A: Yes, but it's optimized for text files. Large binaries will consume more storage.

**Q: How do I stop the daemon?**  
A: Press Ctrl+C in the terminal where `rewind watch` is running, or kill the process.

## Contributing

This project is in active development. Issues, feature requests, and PRs welcome!

## License

MIT License - see [LICENSE](LICENSE) for details.

---

*Never lose code again. Never fear the AI agent. Code with confidence.*
