# Test case for deps_to_remove with deps-order.txt

This test case verifies that the `deps_to_remove` attribute is correctly populated
based on the dependency ordering constraints defined in `deps-order.txt`.

## Test scenario:

1. **deps-order.txt** defines the following order:
   - `core.py` (index 0) - foundational code
   - `utils.py` (index 1) - utility functions  
   - `high_level.py` (index 2) - high-level functionality

2. **Dependencies**:
   - `high_level.py` imports `core` and `utils` (valid - higher index depending on lower)
   - `utils.py` imports `core` (valid - higher index depending on lower)
   - `core.py` imports nothing (valid - no dependencies)

3. **Expected behavior**:
   - All dependencies should appear in `deps` 
   - No dependencies should appear in `deps_to_remove` (all are valid)

## Files:
- `core.py` - basic functionality
- `utils.py` - depends on core
- `high_level.py` - depends on both core and utils
- `deps-order.txt` - defines allowed dependency order