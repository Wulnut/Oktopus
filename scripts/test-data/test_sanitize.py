import json
import re
import unittest
from pathlib import Path

from sanitize import Sanitizer


class SanitizerSerialTest(unittest.TestCase):
    def test_rewrites_alphanumeric_serial_fields_and_embedded_json_independent_of_key_order(self):
        raw_serial = "GPONAB12CD34"
        document = {
            "info_raw": json.dumps({"result_params": {"SerialNumber": raw_serial}}),
            "device_sn": f"proto::{raw_serial}",
            "manufacturer": "GPON",
        }

        sanitized = Sanitizer().rewrite(document)

        self.assertEqual("proto::SN-DEV-0001", sanitized["device_sn"])
        self.assertEqual(
            "SN-DEV-0001",
            json.loads(sanitized["info_raw"])["result_params"]["SerialNumber"],
        )
        self.assertEqual("GPON", sanitized["manufacturer"])
        self.assertNotIn(raw_serial, json.dumps(sanitized))

    def test_committed_fixture_serial_fields_are_synthetic(self):
        fixture_dir = Path(__file__).resolve().parents[2] / "fixtures"
        allowed = re.compile(r"^(?:[^:]+::)?SN-DEV-\d+$")

        def inspect(value, location):
            if isinstance(value, dict):
                for key, child in value.items():
                    normalized = re.sub(r"[^a-z0-9]", "", key.lower())
                    if normalized in {"sn", "devicesn", "serialnumber"} and isinstance(child, str):
                        self.assertRegex(child, allowed, f"unsanitized serial at {location}.{key}")
                    inspect(child, f"{location}.{key}")
            elif isinstance(value, list):
                for index, child in enumerate(value):
                    inspect(child, f"{location}[{index}]")
            elif isinstance(value, str) and value[:1] in {"{", "["}:
                try:
                    nested = json.loads(value)
                except json.JSONDecodeError:
                    return
                inspect(nested, f"{location}<json>")

        for fixture in fixture_dir.glob("*.json"):
            inspect(json.loads(fixture.read_text()), fixture.name)


class TestDataScriptAuthHeaderTest(unittest.TestCase):
    def test_controller_scripts_send_raw_jwt_authorization_header(self):
        scripts_dir = Path(__file__).resolve().parent
        for script_name in ("capture.sh", "seed-test-tenant.sh"):
            source = (scripts_dir / script_name).read_text()
            self.assertNotIn(
                "Authorization: Bearer ${TOKEN}",
                source,
                f"{script_name} is incompatible with controller AuthMiddleware",
            )
            self.assertIn("Authorization: ${TOKEN}", source)


if __name__ == "__main__":
    unittest.main()
