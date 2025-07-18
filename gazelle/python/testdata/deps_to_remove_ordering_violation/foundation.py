# Copyright 2023 The Bazel Authors. All rights reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""Foundation layer - ILLEGALLY depends on middleware (ordering violation)."""

import middleware  # This violates ordering constraints!


def foundation_function():
    """Foundation function that uses middleware."""
    return f"foundation: {middleware.middleware_function()}"


def get_foundation_data():
    """Get foundation data."""
    return {"foundation": True, "data": middleware.process_data()}
