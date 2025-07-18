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

"""Utility functions - depends on core."""

import core

def utility_function():
    """Utility function that uses core functionality."""
    base_value = core.get_core_value()
    return f"utility result: {base_value * 2}"

def format_output(value):
    """Format output using core function."""
    core_result = core.core_function()
    return f"{core_result} -> {value}"