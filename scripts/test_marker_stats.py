import unittest
from unittest.mock import patch
from urllib.error import HTTPError

from marker_stats import classify_address, fetch_records, summarize


class MarkerStatsTests(unittest.TestCase):
    def test_province_aliases_and_full_parent_names(self):
        cases = [
            ("전라북도 전주시 덕진구", ("전북특별자치도", "전북 전주시", "전북 전주시 덕진구")),
            ("경기 광주시 경안동", ("경기도", "경기 광주시", None)),
            ("광주 서구 상무대로", ("광주광역시", None, "광주 서구")),
            ("부산 서구 암남동", ("부산광역시", None, "부산 서구")),
            ("강원도 홍천군 홍천읍", ("강원특별자치도", "강원 홍천군", None)),
            ("세종특별자치시 한누리대로", ("세종특별자치시", None, None)),
            ("  서울\t강남구   역삼동  ", ("서울특별시", None, "서울 강남구")),
            ("경기도 수원시 권선구", ("경기도", "경기 수원시", "경기 수원시 권선구")),
            ("제주도 제주시 애월읍", ("제주특별자치도", "제주 제주시", None)),
            ("경기도", ("경기도", None, None)),
            (None, (None, None, None)),
            ("Address not found", (None, None, None)),
        ]
        for address, expected in cases:
            with self.subTest(address=address):
                self.assertEqual(classify_address(address), expected)

    def test_unknown_addresses_stay_in_total_and_user_fields_are_discarded(self):
        summary = summarize([
            {"markerId": 1, "address": "서울 강남구", "username": "never publish", "userId": 123},
            {"markerId": 2, "address": "경기 수원시 권선구"},
            {"markerId": 3, "address": None},
        ], "2026-10-17")
        self.assertEqual(summary["total_locations"], 3)
        self.assertEqual(summary["unclassified_addresses"], 1)
        self.assertEqual(sum(summary["provinces"].values()), 2)
        self.assertNotIn("username", str(summary))
        self.assertNotIn("never publish", str(summary))
        self.assertNotIn("userId", str(summary))

    def test_bad_payloads_do_not_generate_a_successful_snapshot(self):
        for records in ([], None, {"error": "upstream failure"}, [None], [{"markerId": True}],
                        [{"markerId": 1, "address": None}],
                        [{"markerId": 1, "address": "서울"}, {"markerId": 1, "address": "부산"}]):
            with self.subTest(records=records), self.assertRaises(ValueError):
                summarize(records, "2026-10-17")

    def test_same_named_districts_are_not_merged(self):
        summary = summarize([
            {"markerId": 1, "address": "부산광역시 중구"},
            {"markerId": 2, "address": "서울특별시 중구"},
        ], "2026-10-17")
        self.assertEqual(summary["districts"], {"부산 중구": 1, "서울 중구": 1})

    def test_transient_upstream_failure_retries_without_logging_response_body(self):
        error = HTTPError("https://example.test", 503, "Unavailable", {}, None)
        with patch("marker_stats.urlopen", side_effect=error) as request, patch("marker_stats.time.sleep") as sleep:
            with self.assertRaises(HTTPError):
                fetch_records()
        self.assertEqual(request.call_count, 3)
        self.assertEqual(sleep.call_count, 2)


if __name__ == "__main__":
    unittest.main()
