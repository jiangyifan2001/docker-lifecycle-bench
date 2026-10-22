import errno
import importlib.util
from pathlib import Path
import unittest
from unittest.mock import Mock

spec = importlib.util.spec_from_file_location("host_preparation", Path(__file__).resolve().parents[2] / "scripts/single-cpu-host.py")
host_preparation = importlib.util.module_from_spec(spec)
spec.loader.exec_module(host_preparation)


class RecoveryTest(unittest.TestCase):
    def test_old_kernel_restores_original_budget(self):
        path = Mock()
        path.write_text.side_effect = [OSError(errno.ENOSPC, "populated cpuset"), None]
        host_preparation.restore_cpu_file(path, "", "0-191")
        self.assertEqual([c.args[0] for c in path.write_text.call_args_list], ["\n", "0-191\n"])

    def test_other_errors_are_not_silenced(self):
        path = Mock()
        path.write_text.side_effect = PermissionError(errno.EACCES, "not administrator")
        with self.assertRaises(PermissionError):
            host_preparation.restore_cpu_file(path, "", "0-191")
        self.assertEqual(path.write_text.call_count, 1)

    def test_explicit_original_mask_is_preserved(self):
        path = Mock()
        host_preparation.restore_cpu_file(path, "4-7", "4-7")
        path.write_text.assert_called_once_with("4-7\n")
