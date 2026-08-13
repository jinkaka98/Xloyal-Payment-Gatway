import importlib.util
import unittest
from datetime import datetime
from pathlib import Path


SPEC = importlib.util.spec_from_file_location("neko_helper", Path(__file__).with_name("neko-helper.py"))
HELPER = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(HELPER)


class NekoHelperTest(unittest.TestCase):
    def test_portal_timezone_is_wib(self):
        observed = datetime(2026, 8, 13, 23, 30, tzinfo=HELPER.timezone.utc).astimezone(HELPER.WIB)
        self.assertEqual(observed.strftime("%d/%m/%Y %H:%M"), "14/08/2026 06:30")


if __name__ == "__main__":
    unittest.main()
