#! /usr/bin/env python3
import os

subject = os.environ.get("SUBJECT")

if not subject:
    print("Welcome!")
else:
    print(f"Welcome, participant {subject}!")

input("Press Enter to quit...")
