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

"""Middleware layer - ILLEGALLY depends on application (ordering violation)."""

import application  # This violates ordering constraints!

def middleware_function():
    """Middleware function that illegally uses application layer."""
    return f"middleware: {application.app_function()}"

def process_data():
    """Process data using application layer (violation)."""
    return application.get_app_data()