<p align="center">
<img src="/images/logo1.png" alt="rewind logo" width="240px"/>
<h2>Rewind</h2>
</p>

**Simple, automatic version control for solo developers**

Rewind automatically saves every version of your files when they change, giving you instant access to your entire editing history. Perfect for when you need to quickly restore a file after an overzealous AI agent, a bad refactor, or just want to see what your code looked like yesterday.

[![Release](https://img.shields.io/github/release/davenicholson-xyz/rewind.svg)](https://github.com/davenicholson-xyz/rewind/releases/latest)
[![Platforms](https://img.shields.io/badge/platforms-linux%20|%20macos-blue)]()

## Why Rewind?

- 🔄 **Automatic**: No commits, no staging - just code and save
- ⚡ **Instant**: Restore any version of any file in seconds  
- 🎯 **Solo-friendly**: Built for individual developers, not teams
- 💾 **Local**: Everything stays on your machine
- 🧠 **Smart**: Ignores common files you don't want versioned (.git, node_modules, etc.)

## Install

Install on linux/mac with the following script
```sh
curl -sSL https://raw.githubusercontent.com/davenicholson-xyz/wallchemy/main/install.sh | bash
```

## Quick Start

```bash
# Initialize rewind in your project
rewind init
```

Edit your files like normal. When you want to rollback to a previous version...

```bash
# View file history
rewind rollback src/main.js

# Tag important versions
rewind tag src/main.js "stable-release"

# Rollback a file to a previous version
rewind rollback src/main.js --version 5

# Rollback to a tagged version
rewind rollback src/main.js --tag "stable-release"

# Rollback to a previous version by time
rewind rollback src/main.js --time-ago 2h

# Rollback ALL tracked files to a previous time
rewind rollback --time-ago 2h

# Show differences between versions
rewind diff src/main.js --version 3
```

## Commands

### Project Setup
- `rewind init [path]` - Initialize rewind in current or specified directory
- `rewind remove [--force]` - Remove rewind from current directory

### Daemon Control  
- `rewind service start` - Start the file watching service
- `rewind service stop` - Stop the file watching service

### File History
- `rewind rollback <file>` - Show version history for file
- `rewind rollback <file> --json` - Show history as JSON
- `rewind rollback <file> --csv` - Show history as CSV
- `rewind diff <file> [--version <n>]` - Show changes between versions

### Tagging Versions
- `rewind tag <file> <tag_name>` - Tag the latest version of a file
- `rewind tag <file> <tag_name> --version <n>` - Tag a specific version

### Restore Operations
- `rewind rollback <file> --version <n>` - Rollback file to specific version
- `rewind rollback <file> --tag <tag_name>` - Rollback file to tagged version
- `rewind rollback <file> --time-ago <duration>` - Rollback to last version before specified time (e.g., 2h, 30m, 1d)
- `rewind rollback --time-ago <duration>` - Rollback ALL tracked files to specified time ago (filesystem-wide)
- `rewind rollback <file> --version <n> --confirm` - Rollback with confirmation
- `rewind restore` - List all deleted files for restoration
- `rewind restore <file>` - Restore specific deleted file
- `rewind restore --confirm` - Restore with confirmation prompts

### Storage Management
- `rewind purge --keep-last <n>` - Keep only the last n versions per file
- `rewind purge --older-than <duration>` - Remove versions older than specified time (e.g., 7d, 2w, 1h)
- `rewind purge --max-size <size>` - Remove oldest versions to keep total size under limit (e.g., 1GB, 500MB)
- `rewind purge --dry-run` - Preview what would be removed without deleting
- `rewind purge --force` - Skip confirmation prompt

**Note:** Tagged versions are always preserved during purge operations, and at least one version per file is always kept.

Customize what gets ignored by editing `.rewind/ignore` or creating `.rwignore` files in your project.

## Contributing

This project is in active development. Issues, feature requests, and PRs welcome!

## License

MIT License - see [LICENSE](LICENSE) for details.
