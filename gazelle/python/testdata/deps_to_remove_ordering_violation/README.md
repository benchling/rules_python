# Test case for deps_to_remove with ordering violations

This test case verifies that the `deps_to_remove` attribute is correctly populated
when there are dependency ordering violations defined in `deps-order.txt`.

## Test scenario:

1. **deps-order.txt** defines the following order:
   - `foundation.py` (index 0) - foundational code
   - `middleware.py` (index 1) - middle layer
   - `application.py` (index 2) - application layer

2. **Dependencies**:
   - `application.py` imports `middleware` and `foundation` (valid - higher depending on lower)
   - `middleware.py` imports `application` (INVALID - lower depending on higher)
   - `foundation.py` imports `middleware` (INVALID - lower depending on higher)

3. **Expected behavior**:
   - All dependencies should appear in `deps`
   - Violating dependencies should also appear in `deps_to_remove`:
     - `middleware`'s dependency on `application` should be in `deps_to_remove`
     - `foundation`'s dependency on `middleware` should be in `deps_to_remove`

## Files:
- `foundation.py` - foundational functionality (illegally depends on middleware)
- `middleware.py` - middle layer (illegally depends on application)
- `application.py` - application layer
- `deps-order.txt` - defines allowed dependency order
