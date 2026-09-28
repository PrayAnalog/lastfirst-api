#!/bin/bash
# Usage: pr-lens.sh <command> [<args>...]
#
# Runs the PR Lens CLI at the version this repository pins, so a command
# taken from the vendored pr-lens skill does not pick up @latest.
exec npx @coldtea/pr-lens-cli@0.7.0 "$@"
