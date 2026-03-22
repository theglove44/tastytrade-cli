#!/usr/bin/env bash

set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  branch_closeout.sh --message "commit subject" path/to/file [more files...]

Stages the listed files, creates one commit, pushes the current branch, and
refreshes the local upstream tracking ref.
EOF
}

message=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --message|-m)
      if [[ $# -lt 2 ]]; then
        echo "branch_closeout.sh: --message requires a value" >&2
        usage >&2
        exit 1
      fi
      message=$2
      shift 2
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    --)
      shift
      break
      ;;
    *)
      break
      ;;
  esac
done

if [[ -z "${message}" ]]; then
  echo "branch_closeout.sh: --message is required" >&2
  usage >&2
  exit 1
fi

if [[ $# -lt 1 ]]; then
  echo "branch_closeout.sh: at least one file path is required" >&2
  usage >&2
  exit 1
fi

branch="$(git branch --show-current)"
if [[ -z "${branch}" ]]; then
  echo "branch_closeout.sh: unable to determine current branch" >&2
  exit 1
fi

echo "Branch: ${branch}"
git status --short --branch

git add -- "$@"
git commit -m "${message}"
commit_sha="$(git rev-parse --short HEAD)"
echo "Committed: ${commit_sha}"
git push origin "${branch}"
echo "Pushed: origin/${branch}"

if ! git fetch origin "${branch}"; then
  echo "branch_closeout.sh: warning: could not refresh origin/${branch}" >&2
fi

if ! git branch --set-upstream-to="origin/${branch}" "${branch}"; then
  echo "branch_closeout.sh: warning: push succeeded but upstream tracking could not be updated" >&2
  echo "branch_closeout.sh: run: git branch --set-upstream-to=origin/${branch} ${branch}" >&2
fi

git status --short --branch
