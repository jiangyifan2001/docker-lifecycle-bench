#!/usr/bin/env python3
"""Explicit administrator-only preparation/restore; never called by dlbench run.

Changes runtime AllowedCPUs only. Does not restart services, grant access, or
delete containers. State is saved before the first change for recovery.
"""
import argparse
import errno
import json
import os
from pathlib import Path
import subprocess
import sys

UNITS = ("docker.service", "containerd.service", "dlbench.slice")


def output(*args):
    return subprocess.check_output(args, text=True).strip()


def property_value(unit, name):
    return output("systemctl", "show", unit, "--property=" + name, "--value")


def no_containers():
    if output("docker", "--host", "unix:///var/run/docker.sock", "ps", "-q"):
        raise RuntimeError("refusing host changes while any Docker container is running")


def restore_cpu_file(path, wanted, effective):
    try:
        path.write_text(wanted + "\n")
    except OSError as exc:
        # Older cpuset implementations reject clearing a populated group.
        # Restore the original effective CPU budget without moving tasks or
        # restarting services. Keep this residual explicit mask visible.
        if exc.errno != errno.ENOSPC or wanted or not effective:
            raise
        path.write_text(effective + "\n")
        print(f"Compatibility fallback: {path} explicitly retains {effective}; "
              "empty/inherited cpuset requires a later service restart on this kernel.")


def restore(state_path, state):
    no_containers()
    errors = []
    for unit in reversed(UNITS):
        try:
            subprocess.run(["systemctl", "set-property", "--runtime", unit,
                            "AllowedCPUs=" + state["units"][unit]["allowed"]], check=True)
            actual = property_value(unit, "AllowedCPUs")
            if actual != state["units"][unit]["allowed"]:
                raise RuntimeError(f"{unit} restore mismatch: {actual}")
            # A cleared systemd property need not clear the kernel's cpuset.
            group = property_value(unit, "ControlGroup")
            if not actual and group:
                if not group.startswith("/") or ".." in Path(group).parts or group == "/":
                    raise RuntimeError(f"unsafe unit cgroup: {group}")
                cpus_file = Path("/sys/fs/cgroup" + group, "cpuset.cpus")
                if cpus_file.exists():
                    expected_effective = state["units"][unit].get("effective")
                    if not expected_effective:
                        expected_effective = (cpus_file.parent.parent / "cpuset.cpus.effective").read_text().strip()
                    restore_cpu_file(cpus_file, state["units"][unit].get("kernelCpus", ""), expected_effective)
                    if (cpus_file.parent / "cpuset.cpus.effective").read_text().strip() != expected_effective:
                        raise RuntimeError(f"{unit} kernel CPU budget was not restored")
            if unit == "dlbench.slice" and state["units"][unit]["active"] != "active":
                group = property_value(unit, "ControlGroup")
                if group:
                    events = Path("/sys/fs/cgroup" + group, "cgroup.events").read_text()
                    if "populated 1" in events:
                        raise RuntimeError("refusing to stop a populated dlbench.slice")
                subprocess.run(["systemctl", "stop", unit], check=True)
            effective = property_value(unit, "EffectiveCPUs")
            expected = state["units"][unit].get("effective")
            if expected is not None and effective != expected:
                raise RuntimeError(f"{unit} effective CPUs after restore {effective!r}, expected {expected!r}")
        except Exception as exc:
            errors.append(str(exc))
    if errors:
        raise RuntimeError("; ".join(errors))
    print("Restored original CPU restrictions. Recovery state retained at", state_path)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=["prepare", "restore"])
    parser.add_argument("--cpu", type=int)
    parser.add_argument("--state", type=Path, required=True, help="new absolute JSON path under the operator's work directory")
    args = parser.parse_args()
    if os.geteuid() != 0:
        parser.error("run this separate preparation tool explicitly as administrator; it never invokes sudo")
    if not args.state.is_absolute() or not args.state.parent.is_dir():
        parser.error("--state requires an absolute path in an existing directory")
    if args.action == "restore":
        state = json.loads(args.state.read_text())
        if set(state["units"]) != set(UNITS):
            raise RuntimeError("unexpected recovery state units")
        restore(args.state, state)
        return
    if args.cpu is None or args.cpu < 0:
        parser.error("prepare requires --cpu with a non-negative logical CPU ID")
    online = output("cat", "/sys/devices/system/cpu/online")
    online_ids = set()
    for part in online.split(","):
        limits = [int(x) for x in part.split("-")]
        online_ids.update(range(limits[0], limits[-1] + 1))
    if args.cpu not in online_ids:
        raise RuntimeError(f"CPU {args.cpu} is offline or absent")
    no_containers()
    for unit in UNITS[:2]:
        if property_value(unit, "ActiveState") != "active":
            raise RuntimeError(f"{unit} must already be active; this helper does not start daemons")
    # A slice may exist but be inactive. Capture its effective property before
    # activation, without modifying or replacing its unit files.
    state = {"cpu": args.cpu, "units": {unit: {
        "allowed": property_value(unit, "AllowedCPUs"),
        "active": property_value(unit, "ActiveState"),
        "effective": property_value(unit, "EffectiveCPUs"),
    } for unit in UNITS}}
    for unit in UNITS:
        group = property_value(unit, "ControlGroup")
        cpus_file = Path("/sys/fs/cgroup" + group, "cpuset.cpus") if group else None
        state["units"][unit]["kernelCpus"] = cpus_file.read_text().strip() if cpus_file and cpus_file.exists() else ""
    with args.state.open("x") as stream:
        json.dump(state, stream, indent=2)
        stream.write("\n")
        stream.flush()
        os.fsync(stream.fileno())
    try:
        subprocess.run(["systemctl", "start", "dlbench.slice"], check=True)
        for unit in UNITS:
            subprocess.run(["systemctl", "set-property", "--runtime", unit, f"AllowedCPUs={args.cpu}"], check=True)
            actual = property_value(unit, "EffectiveCPUs")
            if actual != str(args.cpu):
                raise RuntimeError(f"{unit} effective CPUs {actual!r}, expected {args.cpu}")
    except BaseException:
        restore(args.state, state)
        raise
    print(f"Prepared logical CPU {args.cpu}; no daemon restart. Restore with --state {args.state}")


if __name__ == "__main__":
    try:
        main()
    except Exception as exc:
        print(f"ERROR: {exc}", file=sys.stderr)
        sys.exit(1)
