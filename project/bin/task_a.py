#!/usr/bin/env python3
"""Dummy task A: prints a message taken from config.py, then its arguments.

It is in bin/ and executable, so that session-menu finds it and gives it the
common arguments right after its name.
"""

import os
import sys

# config.py is in the project folder, the parent of bin/.
sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
import config  # noqa: E402

print(f"{config.GREETING} task A ({config.LAB}, screen at {config.SCREEN_DISTANCE_CM} cm)")
print("arguments:", sys.argv[1:])
print("SUBJECT in the environment:", os.environ.get("SUBJECT"))

input("Press Enter to return to the menu...")
