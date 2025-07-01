#!/usr/bin/env bash

set -euo pipefail

has_regression=false

awk '
  BEGIN { section="" }

  $0 ~ /sec\/op[[:space:]]+vs base/      { section = "time" }
  $0 ~ /B\/op[[:space:]]+vs base/        { section = "memory" }

  /^[[:alnum:]_\/-]+-?[0-9]+/ {
    for (i = 1; i <= NF; i++) {
      if ($i ~ /^+[0-9.]+%$/) {
        delta = $i + 0.0
        if ((section == "time" || section == "memory") && delta > 10.0) {
          printf "❌ Benchmark test `%s` failed, `%s` is %s% worse\n", $1, section, delta
          exit_code = 1
        }
        break
      }
    }
  }

  END {
    if (exit_code == 1) {
      exit 1
    } else {
      print "✅ Benchmarks within acceptable limits"
    }
  }
' "$1"
