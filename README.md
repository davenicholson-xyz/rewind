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

# Start watching for changes
rewind watch

# Check status (in another terminal)
rewind status

# View file history
rewind log src/main.js

# Restore a file to a previous version
rewind restore src/main.js --version 5
```

## Real-World Examples

### "Help! AI Agent Destroyed My Code"

You're working with an AI coding assistant that goes a bit too far with refactoring:

```bash
# Before: You have a working 200-line component
# AI Agent: "Let me optimize this for you!"
# After: Your component is now 5 lines and completely broken

# No problem - restore the previous version
rewind log components/UserProfile.tsx
# Shows: Version 15 (2 minutes ago) - "Working version before AI refactor"

rewind restore components/UserProfile.tsx --version 15
# ✅ Your working code is back instantly
```

### "I Deleted Something Important"

```bash
# Accidentally deleted a crucial function during cleanup
# Don't remember exactly when you deleted it

rewind log utils/helpers.js --limit 20
# Browse through recent versions to find when it disappeared

rewind show utils/helpers.js --version 8
# Preview the version to confirm it has your function

rewind restore utils/helpers.js --version 8
# Restored! Your function is back
```

### "What Did I Change Yesterday?"

```bash
# See what changed in your main file yesterday
rewind diff src/app.js --since "24 hours ago"

# Compare current version with version from this morning  
rewind diff src/app.js --version 12

# Restore just the good parts from an older version
rewind show src/app.js --version 10 > temp.js
# Copy the parts you want, paste back into current file
```

### "Experimental Feature Gone Wrong"

```bash
# You spent 2 hours implementing a feature that isn't working
# Instead of debugging, just go back to before you started

rewind log src/
# Find the version from 2 hours ago

rewind restore-all --version 25
# Restores ALL files to version 25 state
# Your experiment is gone, but your working code is back
```

## Installation

### Download Binary

1. Download the latest release for your platform from [GitHub Releases](https://github.com/davenicholson-xyz/rewind/releases)
2. Extract and place the binary in your PATH
3. Make it executable: `chmod +x rewind`

### Install Script (Coming Soon)

```bash
curl -sSL https://get-rewind.dev | bash
```

### Build from Source

```bash
git clone https://github.com/davenicholson-xyz/rewind.git
cd rewind
go build
./rewind --version
```

## Commands

### Project Setup
- `rewind init` - Initialize rewind in current directory
- `rewind init /path/to/project` - Initialize rewind in specific directory

### Daemon Control  
- `rewind watch` - Start the file watching daemon
- `rewind status` - Show daemon status and active watches
- `rewind add /path/to/project` - Add a project to watch list
- `rewind remove /path/to/project` - Remove project from watch list

### File History
- `rewind log [file]` - Show version history for file or directory
- `rewind show <file> --version <n>` - Preview a specific version
- `rewind diff <file> [--version <n>]` - Show changes between versions

### Restore Files
- `rewind restore <file> --version <n>` - Restore file to specific version
- `rewind restore <file> --time "2 hours ago"` - Restore to point in time
- `rewind restore-all --version <n>` - Restore all files to specific version

## How It Works

1. **Initialize**: `rewind init` creates a `.rewind` directory with a SQLite database
2. **Watch**: `rewind watch` starts a daemon that monitors file changes
3. **Save**: Every time you save a file, rewind stores a copy with metadata
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

Customize what gets ignored by editing `.rewind/ignore` or creating `.rwignore` files.

## Performance

- **Storage**: Only changed files are stored (deduplication by content hash)
- **Speed**: File operations are near-instant, even with thousands of versions
- **Memory**: Minimal overhead - the daemon uses ~10MB RAM typically
- **Disk**: Efficient storage with automatic cleanup of old versions (configurable)

## vs Git

| Feature | Rewind | Git |
|---------|--------|-----|
| Setup | One command | Multiple steps |
| Commits | Automatic | Manual |
| Staging | None | Required |
| Solo dev | Perfect | Overkill |
| Teams | No | Yes |
| Branches | No | Yes |
| Remote sync | No | Yes |
| Speed | Instant | Fast |

**Use Rewind when:** You want effortless local versioning for solo development

**Use Git when:** You need team collaboration, branching, or remote repositories

## Configuration

Rewind stores its configuration in `~/.config/rewind/`:

- `watchlist.json` - Projects being watched
- `config.yaml` - Global settings (coming soon)

Project-specific settings in `.rewind/`:
- `ignore` - Files/patterns to ignore  
- `versions/` - Stored file versions
- `rewind.db` - Version metadata database

## FAQ

**Q: Does rewind replace Git?**  
A: No, they solve different problems. Use rewind for automatic local versioning, Git for team collaboration and remote storage.

**Q: How much disk space does it use?**  
A: Only stores changed files once (deduplicated). A typical project might use 100-500MB for months of history.

**Q: Can I use it with Git?**  
A: Yes! Rewind ignores `.git/` by default. Many developers use both together.

**Q: What happens if I delete .rewind/?**  
A: You lose all version history, but your current files are unaffected. Just run `rewind init` to start fresh.

**Q: Does it work with large files?**  
A: Yes, but it's optimized for text files. Large binaries will consume more storage.

## Contributing

This project is in active development. Issues, feature requests, and PRs welcome!

## License

MIT License - see [LICENSE](LICENSE) for details.

---

*Never lose code again. Never fear the AI agent. Code with confidence.*