#!/bin/bash

results=()
overall_exit=0

run_suite() {
    local name=$1
    shift
    echo "=== Running $name ==="
    "$@"
    local rc=$?
    if [ $rc -eq 0 ]; then
        results+=("[PASS] $name")
    else
        results+=("[FAIL] $name")
        overall_exit=1
    fi
    echo ""
}

run_suite "backend-unit"        make test-backend-unit
run_suite "backend-integration" make test-backend-integration
run_suite "frontend-unit"       make test-frontend-unit
run_suite "frontend-component"  make test-frontend-component
run_suite "frontend-smoke"      make test-frontend-smoke

echo "=== Test Summary ==="
for r in "${results[@]}"; do
    echo "$r"
done
echo "==="

failed=$(printf '%s\n' "${results[@]}" | grep -c '^\[FAIL\]' || true)
if [ "$failed" -gt 0 ]; then
    echo "$failed failed"
fi

exit $overall_exit
