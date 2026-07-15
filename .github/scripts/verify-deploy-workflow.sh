#!/usr/bin/env bash
set -euo pipefail

workflow="${1:-.github/workflows/deploy.yml}"

grep -Fq "github.ref == 'refs/heads/main'" "$workflow"
grep -Fq "github.event_name == 'push'" "$workflow"
grep -Fq "github.event_name == 'workflow_dispatch' && inputs.deploy == true" "$workflow"

# A manual run must remain non-deploying unless the operator opts in.
awk '
  /^      deploy:$/ { in_deploy = 1; next }
  in_deploy && /^        default: false$/ { safe_default = 1 }
  END { exit !safe_default }
' "$workflow"
