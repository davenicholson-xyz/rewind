<p align="center">
<img src="/images/logo1.png" alt="rewind logo" width="140px"/>
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

# Rollback a file to a previous version
rewind rollback src/main.js --version 5

# Show differences between versions
rewind diff src/main.js --version 3
```

## Commands

### Project Setup
- `rewind init [path]` - Initialize rewind in current or specified directory
- `rewind remove [--force]` - Remove rewind from current directory

### Daemon Control  
- `rewind service start` - Start the file watching service
- `rewind servie stop` - Start the file watching service
- 
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

Customize what gets ignored by editing `.rewind/ignore` or creating `.rwignore` files in your project.

## Contributing

This project is in active development. Issues, feature requests, and PRs welcome!

## License

MIT License - see [LICENSE](LICENSE) for details.

---

*Never lose code again. Never fear the AI agent. Code with confidence.*
