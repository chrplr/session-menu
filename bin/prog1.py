#! /usr/bin/env python3
import os
import time

subject = os.environ.get("SUBJECT")
run = os.environ.get("RUN")

print(f"PROG1 running with SUBJECT='{subject}' and RUN='{run}'")

total = 10
for i in range(1, total + 1):
    time.sleep(1)
    bar = "#" * i + " " * (total - i)
    print(f"\rProgress: [{bar}] {i}/{total}", end="", flush=True)

print("\nDone!")

input("Press Enter to quit...")
