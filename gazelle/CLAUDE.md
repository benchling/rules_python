# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Common Commands

### Building the Project

```bash
# Build everything in the workspace
bazel build //...

# Build a specific target
bazel build //python:gazelle_binary
```

### Running Tests

```bash
# Run all tests
bazel test //...

# Run a specific test
bazel test //python:python_test_simple_binary

# Run tests with verbose output
bazel test //... --test_output=all
```

### Running Gazelle

```bash
# Run gazelle to update BUILD files
bazel run //:gazelle

# Run gazelle with specific flags
bazel run //:gazelle -- --mode=diff  # Show diff instead of updating files
bazel run //:gazelle -- --mode=fix   # Update files (default mode)
```

## Code Architecture

The Gazelle Python plugin is a Bazel Gazelle extension that generates BUILD files for Python code. The codebase is primarily written in Go.

### Core Components

1. **Language Extension** (`python/language.go`): The main entry point for the Gazelle extension, implementing the `language.Language` interface. It consists of two main components:
   - `Configurer`: Handles configuration and directives
   - `Resolver`: Resolves dependencies

2. **Configuration** (`python/configure.go`, `pythonconfig/pythonconfig.go`): Manages configuration settings for the plugin, including:
   - Directives handling (e.g., `python_root`, `python_generation_mode`, etc.)
   - Package and project settings

3. **Build File Generation** (`python/generate.go`): Contains the core logic for generating Bazel rules:
   - Scans Python files to determine types (libraries, binaries, tests)
   - Creates appropriate rules based on file patterns and directives
   - Handles different generation modes (file, package, project)

4. **Python File Parser** (`python/file_parser.go`, `python/parser.go`): Parses Python files to:
   - Extract import statements
   - Identify module dependencies
   - Process annotations in comments
   - Handle type checking imports

5. **Dependency Resolution** (`python/resolve.go`): Resolves import statements to Bazel targets:
   - Maps import names to first-party and third-party dependencies
   - Uses manifest file to map third-party imports to repository labels
   - Supports different label conventions and normalizations

6. **Manifest System** (`manifest/manifest.go`): Manages the mapping between Python imports and Bazel targets:
   - Reads and writes the `gazelle_python.yaml` manifest file
   - Maps Python module names to their distribution packages

### Key Concepts

1. **Generation Modes**:
   - `file`: Creates one target per file
   - `package`: Creates one target per package (default)
   - `project`: Creates one target for all related files across subdirectories

2. **Python Root**: Sets the root directory of a Python project for import resolution

3. **Entry Points**:
   - `__init__.py`: Marks a Python package
   - `__main__.py`: Entry point for Python binaries
   - `__test__.py`: Entry point for test targets

4. **Directives**: Configuration options in BUILD files (e.g., `# gazelle:python_root`)

5. **Annotations**: Inline configurations in Python files (e.g., `# gazelle:ignore`)

### Dependencies

1. **Go Libraries**:
   - `github.com/bazelbuild/bazel-gazelle`: Core Gazelle framework
   - `github.com/emirpasic/gods`: Data structures (sets, maps)
   - `github.com/bmatcuk/doublestar`: File pattern matching

2. **Python Import Management**:
   - Uses a manifest file (usually `gazelle_python.yaml`) to map Python imports to Bazel targets
   - Supports automatic generation via `modules_mapping` rule