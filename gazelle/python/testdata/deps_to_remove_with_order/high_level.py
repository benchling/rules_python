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

"""High-level functionality - depends on both core and utils."""

import core
import utils

def high_level_operation():
    """High-level operation using both core and utils."""
    core_result = core.core_function()
    utils_result = utils.utility_function()
    return f"High level: {core_result} + {utils_result}"

def process_data():
    """Process data using all available functionality."""
    value = core.get_core_value()
    formatted = utils.format_output(value)
    return f"Processed: {formatted}"