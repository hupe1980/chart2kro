#!/usr/bin/env bash
# Runs the action's checks: `convert --check` against OUTPUT_DIR, and `diff` of the
# chart against OUTPUT_DIR/rgd.yaml on the base branch. Writes outputs and a job
# summary; fails per FAIL_ON_BREAKING.
#
# Env: CHART, OUTPUT_DIR, ARGS, CHECK, DIFF, FAIL_ON_BREAKING, BASE_REF,
#      CHART2KRO (binary, default: chart2kro), GITHUB_OUTPUT, GITHUB_STEP_SUMMARY
set -uo pipefail

bin="${CHART2KRO:-chart2kro}"
out="${GITHUB_OUTPUT:-/dev/null}"
summary="${GITHUB_STEP_SUMMARY:-/dev/stdout}"
# Split ARGS like a shell would (quotes respected, nothing evaluated).
args=()
if [[ -n "${ARGS:-}" ]]; then
  while IFS= read -r -d '' a; do args+=("$a"); done < <(printf '%s' "$ARGS" | xargs printf '%s\0')
fi

stale=false
breaking=false
status=0

{
  echo "## chart2kro"
  echo
} >>"$summary"

if [[ "${CHECK:-true}" == "true" ]]; then
  log="$(mktemp)"
  "$bin" convert "$CHART" ${args[@]+"${args[@]}"} -o "$OUTPUT_DIR" --check -q 2>"$log"
  rc=$?

  case $rc in
    0) echo "✅ \`$OUTPUT_DIR\` is up to date." >>"$summary" ;;
    9)
      stale=true
      status=9
      {
        echo "❌ \`$OUTPUT_DIR\` is stale: $(sed -n 's/^Error: //p' "$log" | head -1)"
        echo
        echo "Regenerate it with:"
        echo
        echo '```bash'
        echo "chart2kro convert $CHART ${ARGS:-} -o $OUTPUT_DIR"
        echo '```'
      } >>"$summary"
      ;;
    *)
      cat "$log" >&2
      echo "❌ \`convert\` failed (exit $rc): $(sed -n 's/^Error: //p' "$log" | head -1)" >>"$summary"
      echo "stale=false" >>"$out"
      echo "breaking=false" >>"$out"
      exit "$rc"
      ;;
  esac
  echo >>"$summary"
fi

if [[ "${DIFF:-true}" == "true" ]]; then
  base_file="$(mktemp)"

  # Fetch only the base commit in shallow CI checkouts; never make a full clone shallow.
  depth=""
  if [[ "$(git rev-parse --is-shallow-repository 2>/dev/null)" == "true" ]]; then
    depth="--depth=1"
  fi

  if [[ -z "${BASE_REF:-}" ]]; then
    echo "ℹ️ No base branch (not a pull request): API diff skipped." >>"$summary"
  elif ! git fetch --quiet $depth origin "$BASE_REF" 2>/dev/null ||
    ! git show "FETCH_HEAD:${OUTPUT_DIR#./}/rgd.yaml" >"$base_file" 2>/dev/null; then
    echo "ℹ️ \`$OUTPUT_DIR/rgd.yaml\` does not exist on \`$BASE_REF\`: API diff skipped." >>"$summary"
  else
    json="$(mktemp)"
    "$bin" diff "$CHART" ${args[@]+"${args[@]}"} --against "$base_file" --format json -q >"$json"
    rc=$?

    case $rc in
      0 | 8)
        n="$(jq '.changes | length' "$json")"
        if [[ $rc -eq 8 ]]; then
          breaking=true
          echo "❌ Breaking or destructive API changes against \`$BASE_REF\`:" >>"$summary"
        elif [[ "$n" -gt 0 ]]; then
          echo "✅ Compatible API changes against \`$BASE_REF\`:" >>"$summary"
        else
          echo "✅ No API changes against \`$BASE_REF\`." >>"$summary"
        fi

        if [[ "$n" -gt 0 ]]; then
          {
            echo
            echo "| Severity | Path | Change |"
            echo "|---|---|---|"
            jq -r '.changes[] | "| \(.severity) | `\(.path)` | \(.message | gsub("\\|"; "\\\\|")) |"' "$json"
          } >>"$summary"
        fi

        if [[ $rc -eq 8 && "${FAIL_ON_BREAKING:-true}" == "true" && $status -eq 0 ]]; then
          status=8
        fi
        ;;
      *)
        echo "❌ \`diff\` failed (exit $rc)." >>"$summary"
        [[ $status -eq 0 ]] && status=$rc
        ;;
    esac
  fi
fi

echo "stale=$stale" >>"$out"
echo "breaking=$breaking" >>"$out"

exit "$status"
