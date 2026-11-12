"""Keep the Z-algorithm historical scale cases and alternating witness distinct."""

import hashlib
from pathlib import Path
import sys
import unittest


sys.path.insert(0, str(Path(__file__).resolve().parents[2] / "scripts"))
from library_checker_coverage import observe
from library_checker_supplement import boundary_cases


class ZAlgorithmSupplementTests(unittest.TestCase):
    def test_alternating_revision_appends_only_the_validated_witness(self):
        old = list(boundary_cases("zalgorithm"))
        new = list(boundary_cases("zalgorithm-alternating"))
        self.assertEqual(len(old), 2)
        self.assertEqual(new[:-1], old)
        self.assertEqual(len(new), 3)
        inp, ans = new[-1]
        self.assertIn("alternating", observe("zalgorithm", inp))
        self.assertEqual(hashlib.sha256(inp).hexdigest(),
                         "969c1430b31d90507c8883d7584a27f56fc6688310ff559dd1adec7dc40fb482")
        self.assertEqual(hashlib.sha256(ans).hexdigest(),
                         "00f7f624dc46f6d9434a7d7cc283ccb5044ab302425e4f41e61d209ba7202736")
