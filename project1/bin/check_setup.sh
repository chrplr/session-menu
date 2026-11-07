#!/usr/bin/env bash
# Dummy check of the setup: prints its options, then waits for Enter.
#
# It is in bin/ and executable, so that session-menu finds it and gives it the
# common arguments (-s SUBJECT) right after its name.

subject=""
while [ $# -gt 0 ]; do
    case "$1" in
        -s) subject="$2"; shift 2 ;;
        *) echo "unknown option: $1" >&2; exit 2 ;;
    esac
done

echo "Checking the setup for subject \"$subject\""
echo "SUBJECT in the environment: $SUBJECT"

read -r -p "Press Enter to return to the menu..."
