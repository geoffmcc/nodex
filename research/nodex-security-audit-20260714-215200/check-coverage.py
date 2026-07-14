#!/usr/bin/env python3
from __future__ import annotations
import csv
import sys
from pathlib import Path

ledger = Path(__file__).with_name("coverage-ledger.tsv")
required = ["path","sha256","size_bytes","file_kind","review_method","security_domains","review_status","finding_ids","notes","reviewed_at"]
terminal = {"reviewed", "binary_metadata_only", "generated_recorded", "blocked"}
with ledger.open(newline="", encoding="utf-8") as f:
    rows = list(csv.DictReader(f, delimiter="	"))
errors = []
if not rows:
    errors.append("coverage ledger has no rows")
for row in rows:
    path = row.get("path", "<unknown>")
    missing = [field for field in required if field not in row]
    if missing:
        errors.append(f"{path}: missing columns {missing}")
        continue
    if not row["review_method"].strip():
        errors.append(f"{path}: empty review_method")
    if not row["security_domains"].strip():
        errors.append(f"{path}: empty security_domains")
    if row["review_status"] == "pending":
        errors.append(f"{path}: pending")
    elif row["review_status"] not in terminal:
        errors.append(f"{path}: invalid review_status {row['review_status']}")
    if row["review_status"] == "blocked" and not row["notes"].strip():
        errors.append(f"{path}: blocked without exact reason in notes")
if errors:
    print("coverage check failed:")
    for error in errors:
        print("-", error)
    sys.exit(1)
print(f"coverage check passed: {len(rows)} rows")
