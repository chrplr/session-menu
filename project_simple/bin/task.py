#!/usr/bin/env python3
"""Dummy task: prints its options, then waits for Enter.

It is in bin/ and executable, so that session-menu finds it and gives it the
common arguments (-s SUBJECT) right after its name, then the options of the
session line.
"""

import argparse
import os
import sys

parser = argparse.ArgumentParser()
parser.add_argument("-s", dest="subject", required=True, help="subject ID")
parser.add_argument("--run", type=int, default=1, help="run number")
parser.add_argument("--practice", action="store_true", help="a short practice run")
parser.add_argument("--fail", action="store_true", help="exit with code 2")
args = parser.parse_args()

print("arguments:", sys.argv[1:])
print(f"subject {args.subject!r}, run {args.run}" + (", practice" if args.practice else ""))
print("SUBJECT in the environment:", os.environ.get("SUBJECT"))

if args.fail:
    # The menu shows the exit code and waits for Enter.
    print("failing on purpose (--fail)")
    sys.exit(2)

input("Press Enter to return to the menu...")
