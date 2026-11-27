"""Require real execution of database acceptance tests; skipped tests fail CI."""
import json
import sys

records = [json.loads(line) for line in open(sys.argv[1]) if line.strip()]
results = {r["Test"]: r["Action"] for r in records if r.get("Test") and r["Action"] in ("pass", "fail", "skip")}
for test, result in results.items():
    print(f"{result}: {test}")
if not results or any(result != "pass" for result in results.values()):
    sys.exit("Missing, skipped, or failed database tests")
for required in sys.argv[2:]:
    if results.get(required) != "pass":
        sys.exit(f"Required database test did not pass: {required}")
