#!/usr/bin/env bash
# Dummy questionnaire: takes the subject ID as its only argument, not as
# "-s SUBJECT". So the session lines run it with '!', without the common
# arguments:
#
#   !questionnaire.sh {subject}

if [ $# -ne 1 ]; then
    echo "usage: questionnaire.sh SUBJECT" >&2
    exit 2
fi

echo "Questionnaire of subject \"$1\""
read -r -p "How do you feel (1-5)? " answer
echo "answer: $answer"
