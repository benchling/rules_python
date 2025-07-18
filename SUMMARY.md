# Implementation Summary: `deps_to_remove` Feature for Gazelle Python Extension

## Overview

This document summarizes the implementation of the `deps_to_remove` feature for `py_library` targets in the Gazelle Python extension. The feature allows automatic generation of a `deps_to_remove` attribute that contains dependencies violating ordering constraints defined in a `deps-order.txt` file.

## Feature Requirements

- **All dependencies** must be included in the `deps` attribute (normal behavior)
- **Violating dependencies** must also be included in the `deps_to_remove` attribute
- **Dependency ordering** is defined by a `deps-order.txt` file at the repository root
- **Files listed earlier** in `deps-order.txt` can be depended upon by files listed later, but not vice versa

## Implementation Details

### 1. Attribute Support (`kinds.go`)

**File**: `/workspaces/rules_python/gazelle/python/kinds.go`

Added `deps_to_remove` attribute support to `pyLibraryKind`:
```go
MergeableAttrs: map[string]bool{
    "srcs": true,
    "deps_to_remove": true,
},
ResolveAttrs: map[string]bool{
    "deps": true,
    "pyi_deps": true,
    "deps_to_remove": true,
},
```

### 2. Target Builder Enhancement (`target.go`)

**File**: `/workspaces/rules_python/gazelle/python/target.go`

- Added `depsToRemove` field to `targetBuilder` struct
- Added helper methods: `addDepToRemove()`, `addDepsToRemove()`
- Updated `build()` method to store source files for ordering constraints

### 3. Dependency Order Resolution (`resolve.go`)

**File**: `/workspaces/rules_python/gazelle/python/resolve.go`

#### Core Components:

**DepsOrderResolver Structure:**
```go
type DepsOrderResolver struct {
    fileToIndex    map[string]int     // File to ordering index mapping
    loaded         bool               // Loading state
    importToSrcs   map[string][]string // Import to source files mapping
}
```

**Key Methods:**
- `LoadDepsOrder()` - Parses `deps-order.txt` file
- `GetMedianIndex()` - Calculates median ordering index for source files
- `ShouldAddToDepsToRemove()` - Determines if dependency violates ordering
- `RegisterImportSources()` - Maps import specs to source files

#### Dependency Processing Logic:

1. **During Import Registration**: Map import specs to their source files
2. **During Dependency Resolution**: Register dependency labels to source files
3. **During Rule Finalization**: Apply ordering constraints to create `deps_to_remove`

### 4. Language Integration (`language.go`)

**File**: `/workspaces/rules_python/gazelle/python/language.go`

Updated `NewLanguage()` to initialize the resolver with `DepsOrderResolver`:
```go
return &Python{
    Resolver: Resolver{
        depsOrderResolver: NewDepsOrderResolver(),
    },
}
```

## Algorithm Details

### Ordering Constraint Logic

1. **File Indexing**: Each file in `deps-order.txt` gets an index (0, 1, 2, ...)
2. **Median Calculation**: For targets with multiple sources, calculate median index
3. **Violation Detection**: If `currentTargetIndex < dependencyTargetIndex`, it's a violation
4. **Attribute Population**: Violating dependencies are added to both `deps` and `deps_to_remove`

### Path Matching Strategy

The implementation handles path matching flexibly:
- Tries exact path matches first (`pkg/file.py`)
- Falls back to filename matches (`file.py`)
- Supports both repo-relative and package-relative paths

## Test Coverage

### Test Case 1: Valid Dependencies (`deps_to_remove_with_order`)

**Files**: `core.py` → `utils.py` → `high_level.py`

**Scenario**: All dependencies follow correct ordering
- `utils.py` depends on `core.py` ✅ (valid: index 1 → index 0)
- `high_level.py` depends on both ✅ (valid: index 2 → index 0,1)

**Expected Result**: No `deps_to_remove` attributes (all dependencies are valid)

### Test Case 2: Ordering Violations (`deps_to_remove_ordering_violation`)

**Files**: `foundation.py` → `middleware.py` → `application.py`

**Scenario**: Contains dependency ordering violations
- `foundation.py` depends on `middleware.py` ❌ (violation: index 0 → index 1)
- `middleware.py` depends on `application.py` ❌ (violation: index 1 → index 2)

**Expected Result**:
- `foundation` target: `deps_to_remove = [":middleware"]`
- `middleware` target: `deps_to_remove = [":application"]`

## File Structure

```
gazelle/python/
├── kinds.go                 # Attribute definitions
├── target.go               # Target building logic
├── resolve.go              # Dependency resolution & ordering
├── language.go             # Language initialization
└── testdata/
    ├── deps_to_remove_with_order/           # Valid dependencies test
    │   ├── deps-order.txt
    │   ├── core.py, utils.py, high_level.py
    │   ├── BUILD.in, BUILD.out
    │   └── test.yaml
    └── deps_to_remove_ordering_violation/   # Violation test
        ├── deps-order.txt
        ├── foundation.py, middleware.py, application.py
        ├── BUILD.in, BUILD.out
        └── test.yaml
```

## Usage

### 1. Create `deps-order.txt` at repository root:
```
# Comments are supported
core/base.py
utils/helpers.py
features/advanced.py
```

### 2. Run Gazelle:
```bash
bazel run //:gazelle
```

### 3. Generated BUILD files will include `deps_to_remove`:
```python
py_library(
    name = "advanced",
    srcs = ["advanced.py"],
    deps = [
        "//core:base",      # Valid dependency
        "//utils:helpers",  # Valid dependency
    ],
    # deps_to_remove is empty - no violations
)

py_library(
    name = "helpers",
    srcs = ["helpers.py"],
    deps = ["//features:advanced"],        # Invalid dependency
    deps_to_remove = ["//features:advanced"],  # Marked for removal
)
```

## Benefits

1. **Automated Detection**: Automatically identifies dependency ordering violations
2. **Build Compatibility**: All dependencies remain in `deps` for build correctness
3. **Tooling Integration**: `deps_to_remove` can be used by linters, analyzers, etc.
4. **Flexible Configuration**: Simple text file configuration
5. **Backward Compatible**: No `deps-order.txt` means no constraints applied


---

This implementation provides a robust foundation for dependency ordering enforcement in Python projects using Bazel and Gazelle.
