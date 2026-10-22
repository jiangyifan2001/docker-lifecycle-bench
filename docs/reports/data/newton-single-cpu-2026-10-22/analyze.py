#!/usr/bin/env python3
"""Generate report tables from a completed, independently verified experiment."""
import collections
import datetime as dt
import json
import math
from pathlib import Path
import re
import statistics
import sys

root = Path(sys.argv[1])
bundles = list((root / "results").iterdir())
assert len(bundles) == 1
bundle = bundles[0]
rows = [json.loads(line) for line in (bundle / "operations.ndjson").read_text().splitlines()]
assert all(row["status"] == "success" for row in rows)
groups = collections.defaultdict(list)
for row in rows:
    groups[(row["phase"], row["concurrency"])].append(row)

def stamp(value):
    value = re.sub(r"\.(\d+)", lambda match: "." + match[1][:6].ljust(6, "0"), value)
    return dt.datetime.fromisoformat(value.replace("Z", "+00:00"))

metrics = ("createNs", "startApiNs", "startToReadyNs", "readyObservedNs",
           "killApiNs", "destroyAckNs", "removeAckNs", "cycleAckNs")
cpu_samples = []
columns = ("usr", "nice", "sys", "iowait", "irq", "soft", "steal", "guest", "gnice", "idle")
for line in (root / "mpstat.log").read_text().splitlines():
    parts = line.split()
    if len(parts) != 12 or parts[1] not in ("16", "272") or len(parts[0]) != 8:
        continue
    timestamp = stamp("2026-10-22T" + parts[0] + "+08:00")
    cpu_samples.append((timestamp, int(parts[1]), dict(zip(columns, map(float, parts[2:])))))

tables = []
for (phase, workers), samples in groups.items():
    start = min(stamp(row["startedAt"]) for row in samples)
    finish = max(stamp(row["finishedAt"]) for row in samples)
    entry = {"phase": phase, "workers": workers, "cycles": len(samples),
             "startUTC": start.isoformat(), "finishUTC": finish.isoformat(), "metricsMs": {}, "cpuUtilization": {}}
    for metric in metrics:
        values = sorted(row[metric] / 1e6 for row in samples)
        entry["metricsMs"][metric] = {"mean": statistics.mean(values),
            **{label: values[math.ceil(len(values) * quantile) - 1]
               for label, quantile in (("p50", .5), ("p95", .95), ("p99", .99))}}
    for cpu in (16, 272):
        selected = [sample for timestamp, number, sample in cpu_samples
                    if number == cpu and start <= timestamp - dt.timedelta(seconds=1) and timestamp <= finish]
        assert selected
        average = {name: statistics.mean(sample[name] for sample in selected) for name in columns}
        entry["cpuUtilization"][str(cpu)] = {"oneSecondSamples": len(selected), **average,
            "activeExcludingIowaitAndSteal": sum(average[name] for name in ("usr", "nice", "sys", "irq", "soft"))}
    tables.append(entry)

source = json.loads((root / "probe-source-liuhui.json").read_text())[0]
destination = json.loads((root / "probe-image.json").read_text())[0]
same_content = all(source[key] == destination[key] for key in ("Created", "Architecture", "Os", "RootFS", "Config"))
assert same_content
print(json.dumps({"runId": rows[0]["runId"], "totalCycles": len(rows), "errors": 0,
                  "warmupExcludedCycles": sum(len(v) for k, v in groups.items() if k[0] != "warmup"),
                  "imageContentMatchesLiuhui": same_content,
                  "sourceImageId": source["Id"], "newtonImageId": destination["Id"],
                  "probeRootFS": destination["RootFS"], "groups": tables}, indent=2))
