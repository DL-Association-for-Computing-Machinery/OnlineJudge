"""Regression checks for formal wrong candidates and evidence binding."""

import copy
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parents[2] / "scripts"))
from library_checker_oracles import oracle_result, small_cases
from library_checker_quality import control_result, input_controls
from library_checker_wrong_cases import LOCAL_WRONG_CASES, local_wrong_result, wrong_output
from problem_source_export import validate_report
import validate_library_checker as runner


class LocalWrongTests(unittest.TestCase):
    def setUp(self):
        source, path = "library-checker:staticrmq", "data_structure/staticrmq"
        self.item = {"source_id": source, "path": path,
                     "cases": [{"id": f"case_{i}", "sample": i == 0} for i in range(9)]}
        self.inventory = {f"source/{path}/{name}": {"sha256": char * 64} for name, char in
                          (("sol/correct.cpp", "a"), ("verifier.cpp", "b"), ("checker.cpp", "c"))}
        for n in (7, 8, 3):
            self.inventory[f"problems/staticrmq/in/case_{n}.in"] = {"sha256": "d" * 64}
            self.inventory[f"problems/staticrmq/out/case_{n}.out"] = {"sha256": "e" * 64}

    def test_fixed_mutations_produce_complete_but_wrong_results(self):
        fixtures = {
            "unionfind": (b"3 3\n0 0 1\n0 1 2\n1 0 2\n", b"0\n", b"1\n"),
            "associative_array": (b"3\n0 0 1\n0 0 2\n1 0\n", b"1\n", b"2\n"),
            "point_add_range_sum": (b"2 2\n1 2\n0 0 4\n1 0 2\n", b"3\n", b"7\n"),
            "staticrmq": (b"2 1\n1 5\n0 2\n", b"5\n", b"1\n"),
            "zalgorithm": (b"abab\n", b"0 0 2 0\n", b"4 0 2 0\n"),
            "suffixarray": (b"ababa\n", b"0 2 4 1 3\n", b"4 2 0 3 1\n"),
            "convolution_mod": (b"2 2\n1 2\n3 4\n", b"3 6 0\n", b"3 10 8\n"),
        }
        self.assertEqual({"library-checker:" + name for name in fixtures}, LOCAL_WRONG_CASES.keys())
        for name, (data, wrong, correct) in fixtures.items():
            with self.subTest(name=name):
                actual = wrong_output("library-checker:" + name, data)
                self.assertEqual(actual, wrong)
                self.assertEqual(len(actual.split()), len(correct.split()))
                self.assertNotEqual(actual.split(), correct.split())

    def test_only_normal_output_and_checker_wa_produce_evidence(self):
        with tempfile.TemporaryDirectory() as temp:
            base = Path(temp)
            (base / "in").mkdir()
            (base / "out").mkdir()
            for n in (7, 8, 3):
                (base / "in" / f"case_{n}.in").write_bytes(b"2 2\n1 5\n0 2\n0 1\n")
                (base / "out" / f"case_{n}.out").write_bytes(b"1\n1\n")
            for data, codes, calls in ((b"5\n1\n", (0, 1), 4), (b"5\n1\n", (0, 0, 0), 6),
                                       (b"5\n1\n", (3,), 2), (b"5\n", (1,), 1),
                                       (b"5\v1\n", (1,), 1), (None, (1,), 1)):
                with self.subTest(data=data, codes=codes), patch.object(runner, "checked") as checked:
                    verdicts = iter(codes)
                    def run(command, **options):
                        if "stdout" in options:
                            if data is None:
                                raise subprocess.CalledProcessError(2, command)
                            options["stdout"].write(data)
                            return subprocess.CompletedProcess(command, 0)
                        return subprocess.CompletedProcess(command, next(verdicts))
                    checked.side_effect = run
                    if data == b"5\n1\n" and codes == (0, 1):
                        report = runner.run_local_wrong_case(base, self.item, self.inventory, base / "actual")
                        self.assertEqual((report["case_position"], report["checker_exit"]), (9, 1))
                        self.assertEqual([(r["case_position"], r["checker_exit"]) for r in report["preceding_ac"]], [(8, 0)])
                    else:
                        with self.assertRaises((ValueError, subprocess.CalledProcessError)):
                            runner.run_local_wrong_case(base, self.item, self.inventory, base / "actual")
                    self.assertEqual(checked.call_count, calls)

    def test_v6_requires_exact_candidate_and_formal_input_identity(self):
        source = self.item["source_id"]
        row = {"source_id": source, "cases": 9, "input_validator": "passed-all", "reference_checker": "passed-all",
               "invalid_output_checker_exit": 1, "input_controls": [control_result(*p) for p in input_controls(source)],
               "wrong_solution": None, "slow_solution": None,
               "independent": oracle_result(self.item, self.inventory, small_cases(source))}
        lock = {"commit": "pinned"}
        previous = {"schema": "oj.source-validation.v5", "commit": "pinned", "problems": [row]}
        prior = {"case_id": "case_7", "case_position": 8, "input_sha256": "d" * 64, "answer_sha256": "e" * 64,
                 "candidate_exit": 0, "checker_exit": 0}
        witness = {"suite": "lc-formal-wrong.v1", "candidate": "right-endpoint-only", "case_id": "case_8",
                   "case_position": 9, "input_sha256": "d" * 64, "answer_sha256": "e" * 64,
                   "checker_sha256": "c" * 64, "candidate_exit": 0, "checker_exit": 1, "preceding_ac": [prior]}
        current = {**previous, "schema": "oj.source-validation.v6", "problems": [{**row, "local_wrong_solution": witness}]}
        validate_report(previous, lock, [self.item], inventory=self.inventory)
        validate_report(current, lock, [self.item], inventory=self.inventory, required_schema=current["schema"])
        self.assertEqual(local_wrong_result(self.item, self.inventory, 9), witness)
        for key, value in (("case_id", "case_0"), ("candidate", "another"), ("preceding_ac", []),
                           ("candidate_exit", 1), ("checker_exit", 3), ("input_sha256", "f" * 64)):
            with self.subTest(key=key), self.assertRaisesRegex(ValueError, "incomplete validation"):
                invalid = copy.deepcopy(current)
                invalid["problems"][0]["local_wrong_solution"][key] = value
                validate_report(invalid, lock, [self.item], inventory=self.inventory)
        with self.assertRaisesRegex(ValueError, "invalid.*position"):
            local_wrong_result(self.item, self.inventory, 1)
        with self.assertRaisesRegex(ValueError, "missing local"):
            validate_report({**current, "problems": [row]}, lock, [self.item], inventory=self.inventory)
        self.item["cases"][7]["sample"] = True
        with self.assertRaisesRegex(ValueError, "formal point"):
            validate_report(current, lock, [self.item], inventory=self.inventory)
