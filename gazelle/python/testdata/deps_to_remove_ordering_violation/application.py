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

"""Application layer - depends on middleware and foundation (valid)."""

import foundation
import middleware

def app_function():
    """Application function using both foundation and middleware."""
    return f"app: {foundation.foundation_function()} + {middleware.middleware_function()}"

def get_app_data():
    """Get application data."""
    return {
        "app": True,
        "foundation": foundation.get_foundation_data(),
        "middleware": middleware.process_data()
    }