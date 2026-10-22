from __future__ import annotations

import os
import subprocess
from pathlib import Path
from typing import Mapping, Sequence


class DlbenchClient:
    """Invoke the canonical Go CLI without duplicating benchmark logic."""

    def __init__(self, binary: str | os.PathLike[str] = "dlbench") -> None:
        self.binary = os.fspath(binary)

    def command(self, args: Sequence[str]) -> list[str]:
        return [self.binary, *args]

    def run_campaign(
        self,
        spec: str | os.PathLike[str],
        *,
        env: Mapping[str, str] | None = None,
        single_cpu: int | None = None,
    ) -> subprocess.CompletedProcess[str]:
        command = self.command(["run", "--spec", os.fspath(Path(spec))])
        if single_cpu is not None:
            if isinstance(single_cpu, bool) or not isinstance(single_cpu, int) or single_cpu < 0:
                raise ValueError("single_cpu must be a non-negative logical CPU ID")
            command.extend(["--single-cpu", str(single_cpu)])
        return subprocess.run(command, check=False, text=True, env=env)

    def validate(self, spec: str | os.PathLike[str]) -> subprocess.CompletedProcess[str]:
        command = self.command(["validate", "--spec", os.fspath(Path(spec))])
        return subprocess.run(command, check=False, text=True)
