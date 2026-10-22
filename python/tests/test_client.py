from __future__ import annotations

import unittest
from unittest.mock import patch

from dlbench_client import DlbenchClient


class DlbenchClientTest(unittest.TestCase):
    @patch("dlbench_client.client.subprocess.run")
    def test_single_cpu_zero_is_forwarded(self, run) -> None:
        DlbenchClient("dlbench").run_campaign("campaign.yaml", single_cpu=0)
        self.assertEqual(run.call_args.args[0], ["dlbench", "run", "--spec", "campaign.yaml", "--single-cpu", "0"])

    @patch("dlbench_client.client.subprocess.run")
    def test_invalid_cpu_never_runs(self, run) -> None:
        for value in [-1, True, "16"]:
            with self.assertRaises(ValueError):
                DlbenchClient().run_campaign("campaign.yaml", single_cpu=value)
        run.assert_not_called()

    def test_command_keeps_go_binary_as_single_timing_engine(self) -> None:
        client = DlbenchClient("/opt/dlbench/bin/dlbench")
        self.assertEqual(
            client.command(["run", "--spec", "campaign.yaml"]),
            ["/opt/dlbench/bin/dlbench", "run", "--spec", "campaign.yaml"],
        )


if __name__ == "__main__":
    unittest.main()
