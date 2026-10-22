#!/usr/bin/env python3
"""Independently audit a successful smoke Result Bundle without Docker access."""
import argparse
import calendar
from collections import defaultdict
import hashlib
import json
import math
from pathlib import Path
import re
import time


def timestamp_ns(value):
    match = re.fullmatch(r"(.{19})(?:\.(\d{1,9}))?Z", value)
    if not match:
        raise ValueError(f"expected a UTC RFC3339 timestamp: {value}")
    seconds = calendar.timegm(time.strptime(match[1], "%Y-%m-%dT%H:%M:%S"))
    return seconds * 1_000_000_000 + int((match[2] or "").ljust(9, "0"))


def check(condition, message):
    if not condition:
        raise ValueError(message)


def audit(root):
    def read(name):
        return json.loads((root / name).read_text())

    checksummed = set()
    for line in (root / "checksums.sha256").read_text().splitlines():
        digest, name = line.split("  ", 1)
        check(Path(name).name == name, "unsafe checksum filename")
        check(name not in checksummed, "duplicate checksum entry")
        check(hashlib.sha256((root / name).read_bytes()).hexdigest() == digest, f"checksum mismatch: {name}")
        checksummed.add(name)
    check(checksummed == {p.name for p in root.iterdir() if p.is_file() and p.name != "checksums.sha256"}, "bundle files missing from checksum list")
    spec, summary, status = read("campaign.normalized.json"), read("summary.json"), read("status.json")
    check(status["status"] == "SUCCESS", "run did not succeed")
    rows = [json.loads(line) for line in (root / "operations.ndjson").read_text().splitlines()]
    check(rows, "no operations")
    check({r["sampleId"] for r in rows} == set(range(1, len(rows) + 1)), "sample IDs missing/duplicated, or canary included")
    check(len({r["containerId"] for r in rows}) == len(rows), "container ID reuse")
    grouped = defaultdict(list)
    stage_keys = []
    for row in rows:
        check(row["runId"] == status["runId"], "wrong run identity")
        check(row["status"] == "success" and row["probePid"] == 1, "failed cycle or non-PID1 probe")
        for metric in ["createNs", "startApiNs", "startToReadyNs", "readyObservedNs", "killApiNs", "destroyAckNs", "removeAckNs", "cycleAckNs"]:
            check(row[metric] > 0, f"missing/nonpositive {metric}")
        check(row["startApiNs"] <= row["readyObservedNs"] and row["startToReadyNs"] <= row["readyObservedNs"], "readiness timing order")
        check(row["killApiNs"] <= row["destroyAckNs"], "teardown timing order")
        check(sum(row[k] for k in ["createNs", "readyObservedNs", "destroyAckNs", "removeAckNs"]) <= row["cycleAckNs"], "exclusive stage subset exceeds E2E")
        check(timestamp_ns(row["finishedAt"]) >= timestamp_ns(row["startedAt"]), "negative wall duration")
        check(0 <= row["worker"] < row["concurrency"], "invalid worker ID")
        key = (row["phase"], row["concurrency"])
        if not stage_keys or stage_keys[-1] != key:
            stage_keys.append(key)
        grouped[key].append(row)
    expected = [("warmup", 1), ("latency", 1)]
    if spec["capacity"]["enabled"]:
        expected += [("capacity", n) for n in spec["capacity"]["workers"]]
    check(stage_keys == expected, "unexpected campaign phase order")
    check(len(grouped[("warmup", 1)]) >= spec["warmup"]["minCycles"], "too few warm-up cycles")
    check(len(grouped[("latency", 1)]) == spec["measurement"]["samples"], "wrong serial sample count")
    summaries = {(g["phase"], g["concurrency"]): g for g in summary["groups"]}
    check(len(summaries) == len(summary["groups"]) and set(summaries) == set(grouped), "summary groups differ")
    results = []
    for key in expected:
        samples, saved = grouped[key], summaries[key]
        check(saved["attempts"] == saved["successes"] == len(samples) and saved["errors"] == saved["errorRate"] == 0, "summary counts differ")
        elapsed = (max(timestamp_ns(r["finishedAt"]) for r in samples) - min(timestamp_ns(r["startedAt"]) for r in samples)) / 1e9
        check(math.isclose(saved["observedSeconds"], elapsed, rel_tol=1e-12), "duration differs")
        check(math.isclose(saved["completedCyclesPerSecond"], len(samples) / elapsed, rel_tol=1e-12), "throughput differs")
        for field in ["cycleAckNs", "startToReadyNs"]:
            values = sorted(r[field] for r in samples)
            for label, fraction in [("p50", .50), ("p95", .95), ("p99", .99)]:
                check(saved[field][label] == values[math.ceil(len(values) * fraction) - 1], f"{key} {field} {label} differs")
        for worker in range(key[1]):
            operations = sorted((r for r in samples if r["worker"] == worker), key=lambda r: r["sampleId"])
            check(operations, "configured worker never ran")
            for prev, next_row in zip(operations, operations[1:]):
                check(timestamp_ns(prev["finishedAt"]) <= timestamp_ns(next_row["startedAt"]), "worker overlapped its own cycles")
        results.append({"phase": key[0], "workers": key[1], "cycles": len(samples), "seconds": elapsed,
                        "cyclesPerSecond": len(samples) / elapsed, "e2eMeanMs": sum(r["cycleAckNs"] for r in samples) / len(samples) / 1e6,
                        **{label + "Ms": saved["cycleAckNs"][label] / 1e6 for label in ["p50", "p95", "p99"]}})
    placement = None
    if spec.get("placement") is not None:
        cpu = spec["placement"]["singleCpu"]
        before, after = read("placement.before.json"), read("placement.after.json")
        check(before["before"]["passed"] and after["passed"] and not before.get("canaryError"), "placement failed")
        check(before["cpu"] == after["cpu"] == cpu and before["gomaxprocs"] == 1, "wrong placement profile")
        check({p["role"] for p in before["canary"]} == {"probe", "shim"}, "canary evidence missing")
        check(spec["container"]["cpusetCpus"] == str(cpu) and spec["container"]["cgroupParent"] == "dlbench.slice", "wrong container placement")
        all_processes = before["before"]["processes"] + before["canary"] + after["processes"]
        for p in all_processes:
            check(p["threads"] and all(t["allowedCpus"] in (str(cpu), f"{cpu}-{cpu}") for t in p["threads"]), "thread mask mismatch")
            if p["role"] != "harness":
                check(p["effectiveCpus"] in (str(cpu), f"{cpu}-{cpu}"), "cgroup mask mismatch")
        check([(p["pid"], p["cgroup"]) for p in before["before"]["processes"]] == [(p["pid"], p["cgroup"]) for p in after["processes"]], "daemon identity changed")
        placement = {"cpu": cpu, "beforeAndAfterPassed": True,
                     "processes": [{"role": p["role"], "pid": p["pid"], "threads": len(p["threads"]), "effectiveCpus": p["effectiveCpus"], "effectiveMems": p["effectiveMems"]} for p in before["before"]["processes"] + before["canary"]]}
    return {"validated": True, "runId": status["runId"], "checksummedFiles": len(checksummed), "totalCycles": len(rows), "groups": results, "placement": placement}


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("bundle", type=Path)
    print(json.dumps(audit(parser.parse_args().bundle), indent=2))
