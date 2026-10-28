#!/usr/bin/env python3
"""Dummy task B: prints a message taken from config.toml, then its arguments.

It is in bin/ and executable, so that session-menu finds it and gives it the
common arguments right after its name.
"""

import os
import sys
import tomllib  # Python 3.11 or later

# config.toml is in the project folder, the parent of bin/.
CONFIG = os.path.join(os.path.dirname(os.path.dirname(os.path.abspath(__file__))), "config.toml")
with open(CONFIG, "rb") as f:
    config = tomllib.load(f)

print(f"{config['greeting']} task B ({config['lab']}, screen at {config['screen_distance_cm']} cm)")
print("arguments:", sys.argv[1:])
print("SUBJECT in the environment:", os.environ.get("SUBJECT"))

if "--fail" in sys.argv:
    # The menu shows the exit code and waits for Enter.
    print("failing on purpose (--fail)")
    sys.exit(2)

input("Press Enter to return to the menu...")
