"""The fixed small corpus is reproducible and mismatches fail closed."""

import copy
import hashlib
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parents[2] / "scripts"))
from library_checker_oracles import MAX_OUTPUT_BYTES, oracle_result, small_cases
from library_checker_quality import control_result, input_controls
from problem_source_export import validate_report
import validate_library_checker as runner


def probes(source):
    return {data: answer for _, data, answer in small_cases("library-checker:" + source)}


class OracleTests(unittest.TestCase):
    def test_counts_and_hand_calculated_anchors(self):
        counts = {"aplusb": 25, "unionfind": 10, "associative_array": 4, "static_range_sum": 8,
                  "point_add_range_sum": 8, "staticrmq": 8, "zalgorithm": 62, "suffixarray": 62,
                  "primality_test": 5, "convolution_mod": 6}
        for source, count in counts.items():
            with self.subTest(source=source):
                values = probes(source)
                self.assertEqual(len(values), count)
                self.assertTrue(all(data.endswith(b"\n") and answer.endswith(b"\n") and
                                    0 < len(data) <= 4096 and 0 < len(answer) <= MAX_OUTPUT_BYTES
                                    for data, answer in values.items()))
        # Changing the ordered corpus would invalidate previously issued v5
        # reports. A new suite must keep this historical evidence verifiable.
        fingerprint = hashlib.sha256()
        for source in sorted(counts):
            for case, data, answer in small_cases("library-checker:" + source):
                fingerprint.update(b"\0".join((source.encode(), case.encode(), data, answer)) + b"\0")
        self.assertEqual(fingerprint.hexdigest(), "4a4427c8952e68ff0ad7ba13af66c07f57930ee588f1cef0e9431eecf43828ae")
        self.assertEqual(probes("aplusb")[b"1000000000 1000000000\n"], b"2000000000\n")
        self.assertEqual(list(probes("unionfind").values())[-1], b"0\n1\n0\n")
        self.assertEqual(list(probes("associative_array").values())[2], b"0\n1000000000000000000\n0\n1\n0\n1\n")
        self.assertEqual(list(probes("static_range_sum").values())[2].split()[:4],
                         [b"1000000000", b"2000000000", b"3000000000", b"3000000000"])
        self.assertEqual(list(probes("point_add_range_sum").values())[0], b"0\n2000000000\n3000000000\n")
        self.assertEqual(probes("staticrmq")[b"1 1\n1000000000\n0 1\n"], b"1000000000\n")
        self.assertEqual(probes("zalgorithm")[b"ababa\n"], b"5 0 3 0 1\n")
        self.assertEqual(probes("suffixarray")[b"ababa\n"], b"4 2 0 3 1\n")
        batch = list(probes("primality_test").values())[2].split()
        self.assertEqual([batch[n - 1] for n in (1, 2, 47, 49, 121, 251, 256)],
                         [b"No", b"Yes", b"Yes", b"No", b"No", b"Yes", b"No"])
        self.assertEqual(probes("convolution_mod")[b"3 2\n1 0 1\n2 4\n"], b"2 4 2 4\n")
        self.assertEqual(probes("convolution_mod")[b"1 1\n998244352\n998244352\n"], b"1\n")

    def test_disagreement_and_nonstandard_space_do_not_reach_checker(self):
        source = {"source_id": "library-checker:zalgorithm"}
        cases = (("small-01", b"aa\n", b"2 1\n"),)
        for output in (b"2 0\n", b"2\v1\n", b"2 1 0\n"):
            with self.subTest(output=output), tempfile.TemporaryDirectory() as temp, \
                    patch.object(runner, "small_cases", return_value=cases), \
                    patch.object(runner, "checked") as checked:
                def run(_, **options):
                    if "stdout" in options:
                        options["stdout"].write(output)
                checked.side_effect = run
                with self.assertRaisesRegex(ValueError, "small-01.*disagree"):
                    runner.run_small_oracles(Path(temp), source, {}, Path(temp))
                self.assertEqual(checked.call_count, 2)

    def test_validator_and_checker_failures_do_not_emit_witness(self):
        for position, expected_calls in ((0, 1), (2, 3)):
            with self.subTest(position=position), tempfile.TemporaryDirectory() as temp, \
                    patch.object(runner, "small_cases", return_value=(("small-01", b"aa\n", b"2 1\n"),)), \
                    patch.object(runner, "checked") as checked:
                def run(command, **options):
                    if checked.call_count == position + 1:
                        raise subprocess.CalledProcessError(1, command)
                    if "stdout" in options:
                        options["stdout"].write(b" 2\t1\r\n")
                checked.side_effect = run
                with self.assertRaisesRegex(ValueError, "small-01 failed"):
                    runner.run_small_oracles(Path(temp), {"source_id": "library-checker:zalgorithm"}, {}, Path(temp))
                self.assertEqual(checked.call_count, expected_calls)

    def test_v5_requires_complete_ordered_source_bound_evidence(self):
        source = "library-checker:staticrmq"
        path = "data_structure/staticrmq"
        item = {"source_id": source, "path": path, "cases": [{"id": "sample_00", "sample": True}]}
        inventory = {f"source/{path}/{file}": {"sha256": value * 64} for file, value in
                     (("sol/correct.cpp", "a"), ("verifier.cpp", "b"), ("checker.cpp", "c"))}
        row = {"source_id": source, "cases": 1, "input_validator": "passed-all", "reference_checker": "passed-all",
               "invalid_output_checker_exit": 1, "input_controls": [control_result(*c) for c in input_controls(source)],
               "wrong_solution": None, "slow_solution": None}
        lock = {"commit": "pinned"}
        previous = {"schema": "oj.source-validation.v4", "commit": "pinned", "problems": [row]}
        current = {**previous, "schema": "oj.source-validation.v5",
                   "problems": [{**row, "independent": oracle_result(item, inventory, small_cases(source))}]}
        validate_report(previous, lock, [item], inventory=inventory)
        validate_report(current, lock, [item], inventory=inventory, required_schema=current["schema"])
        with self.assertRaisesRegex(ValueError, "identity"):
            validate_report(previous, lock, [item], inventory=inventory, required_schema=current["schema"])
        for corrupt in (lambda r: r["cases"].pop(), lambda r: r["cases"].reverse(),
                        lambda r: r.update(validator_sha256="d" * 64),
                        lambda r: r["cases"][0].update(oracle_sha256="e" * 64),
                        lambda r: r["cases"][0].update(checker_exit=1)):
            damaged = copy.deepcopy(current)
            corrupt(damaged["problems"][0]["independent"])
            with self.assertRaisesRegex(ValueError, "incomplete validation"):
                validate_report(damaged, lock, [item], inventory=inventory)
