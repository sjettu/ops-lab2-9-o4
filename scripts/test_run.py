import importlib.util
from pathlib import Path
import tempfile
import unittest

spec = importlib.util.spec_from_file_location("runner", Path(__file__).with_name("run.py"))
runner = importlib.util.module_from_spec(spec)
spec.loader.exec_module(runner)


class Preconditions(unittest.TestCase):
    def test_cpu_ranges(self):
        self.assertEqual(runner.cpu_set("0,2-4"), {0, 2, 3, 4})
        for invalid in ["", "4-2", "-1", "0;id"]:
            with self.assertRaises(ValueError):
                runner.cpu_set(invalid)

    def test_only_loopback(self):
        self.assertEqual(runner.loopback("http://127.0.0.1:19090/"), "http://127.0.0.1:19090")
        for invalid in ["https://example.com", "http://user:pass@localhost", "http://localhost/path"]:
            with self.assertRaises(ValueError):
                runner.loopback(invalid)

    def test_request_rejects_changing_or_invalid_input(self):
        with tempfile.TemporaryDirectory() as temp:
            path = Path(temp) / "request.json"
            for body in [b'{"text":"{{seq}}"}', b'{"text":" "}', b'{"other":"hello"}', b"[]", b"x" * 1048577]:
                path.write_bytes(body)
                with self.assertRaises(ValueError):
                    runner.validate_request(path)
            body = b'{"text":"hello world"}'
            path.write_bytes(body)
            self.assertEqual(runner.validate_request(path), body)


if __name__ == "__main__":
    unittest.main()
