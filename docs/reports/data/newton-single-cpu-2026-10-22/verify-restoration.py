#!/usr/bin/env python3
"""Read-only post-run audit of the exact services and container inventory."""
import collections
import json
from pathlib import Path
import subprocess
import sys

root = Path(sys.argv[1])
state = json.loads((root / "host-state.json").read_text())
before = {}
for section in (root / "services.before.txt").read_text().strip().split("\n\n"):
    fields = dict(line.split("=", 1) for line in section.splitlines())
    before[fields["Id"]] = fields
services = []
for unit, original in state["units"].items():
    output = subprocess.check_output(["systemctl", "show", unit, "--property=Id,MainPID,ActiveState,ControlGroup,AllowedCPUs,EffectiveCPUs"], text=True)
    actual = dict(line.split("=", 1) for line in output.splitlines())
    assert actual["AllowedCPUs"] == original["allowed"]
    assert actual["EffectiveCPUs"] == original["effective"]
    assert actual["ActiveState"] == original["active"]
    assert actual.get("MainPID") == before[unit].get("MainPID")
    masks = collections.Counter()
    kernel = ""
    if actual.get("MainPID"):
        for thread in (Path("/proc") / actual["MainPID"] / "task").iterdir():
            try:
                lines = (thread / "status").read_text().splitlines()
            except FileNotFoundError:
                continue
            mask = next(line.split(":", 1)[1].strip() for line in lines if line.startswith("Cpus_allowed_list:"))
            assert mask == original["effective"]
            masks[mask] += 1
        assert masks
        cpu_file = Path("/sys/fs/cgroup" + actual["ControlGroup"], "cpuset.cpus")
        kernel = cpu_file.read_text().strip() if cpu_file.exists() else ""
    services.append({"unit": unit, "properties": actual, "threadMasks": masks, "kernelCpuMask": kernel,
                     "originalKernelCpuMask": original["kernelCpus"]})
old = (root / "containers.before.txt").read_text()
new = (root / "containers.after.txt").read_text()
assert old == new
assert not subprocess.check_output(["docker", "--host", "unix:///var/run/docker.sock", "ps", "-q"], text=True).strip()
print(json.dumps({"passed": True, "originalContainersPreserved": len(old.splitlines()),
                  "runningContainers": 0, "daemonPIDsUnchanged": True, "services": services}, indent=2))
