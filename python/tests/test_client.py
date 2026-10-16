from __future__ import annotations

import unittest

from dlbench_client import DlbenchClient


class DlbenchClientTest(unittest.TestCase):
    def test_command_keeps_go_binary_as_single_timing_engine(self) -> None:
        client = DlbenchClient("/opt/dlbench/bin/dlbench")
        self.assertEqual(
            client.command(["run", "--spec", "campaign.yaml"]),
            ["/opt/dlbench/bin/dlbench", "run", "--spec", "campaign.yaml"],
        )


if __name__ == "__main__":
    unittest.main()
