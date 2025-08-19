# unfolder

Convert repository contents to text format for AI analysis.

## Overview

unfolder is a command-line tool written in Go that takes a directory containing source code and creates a single text file with all the contents in a structured format. This is particularly useful for feeding codebases to AI systems for analysis, code review, or documentation generation.

## Installation

### Download Binary

Download the latest release for your platform from the releases page.

### Build from Source

Requires Go 1.16 or later. Developed and tested with Go 1.25 on macOS 26.

```bash
git clone https://github.com/rinodrops/unfolder.git
cd unfolder
make build
```

The binary will be created in the `dist/` directory.

## Usage

```bash
unfolder [directory] [output]
```

### Arguments

- `directory` - Target directory to process (default: current directory)
- `output` - Output file or directory (default: current directory)

### Examples

```bash
# Process current directory, output to ./dirname.txt
unfolder

# Process specific repository
unfolder /path/to/repo

# Output to specific directory
unfolder /path/to/repo /tmp/

# Custom output filename
unfolder /path/to/repo report.txt
```

## Output Format

The generated file contains:

1. Header explaining the format
2. File sections starting with `--------`
3. File path on the next line
4. File contents
5. End marker `----END----`

Example:
```
This text describes a repository with code...
--------
main.go
package main
...
--------
README.md
# My Project
...
----END----
```

## Features

- Respects `.gitignore` patterns automatically
- Supports custom `.unfolderignore` files for additional exclusions
- Skips binary files (detected by null bytes)
- Ignores symbolic links and directories
- Cross-platform support (Windows, macOS, Linux)
- Supports complex gitignore patterns including wildcards and directory matching
- Auto-exclude by default the VCS directories such as`.git/`, `.svn/`, `.hg/`, `.bzr/`, `CVS/`, and `.darcs/`

## File Filtering

unfolder uses two types of ignore files:

- **`.gitignore`** - Excludes files for security reasons or unnecessary project artifacts (like `.env` files, binaries, build outputs)
- **`.unfolderignore`** - Excludes files that are unnecessary for code review (like documentation assets, images, test fixtures)

Additionally, unfolder automatically excludes:

- Binary files (detected by null bytes)
- Symbolic links
- The output file itself

### Supported Ignore Patterns

- `*.log` - Wildcard matching
- `temp/` - Directory exclusion
- `**/node_modules` - Recursive directory matching
- `build/**` - Everything under build directory
- `[Tt]est*` - Character class matching

## Building

### Build for All Platforms

```bash
make build
```

### Create Distribution Packages

```bash
make package
```

### Generate Checksums

```bash
make checksum
```

### Clean Build Artifacts

```bash
make clean
```

## Use Cases

- Preparing codebases for AI analysis
- Code review documentation
- Creating snapshots of project state
- Feeding repositories to large language models
- Generating consolidated documentation

## License

MIT