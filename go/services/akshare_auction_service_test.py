#!/usr/bin/env python3

import unittest

import akshare_auction_service as service


class FakeFrame:
    def __init__(self, rows):
        self._rows = rows
        self.empty = len(rows) == 0

    def iterrows(self):
        for index, row in enumerate(self._rows):
            yield index, row


class SelectAuctionRowTest(unittest.TestCase):
    def test_prefers_0926_history_row_for_0925_snapshot(self):
        frame = FakeFrame(
            [
                {"时间": "2026-07-14 09:24:00", "成交额": 100},
                {"时间": "2026-07-14 09:25:00", "成交额": 200},
                {"时间": "2026-07-14 09:26:00", "成交额": 300},
            ]
        )

        row = service.select_auction_row(frame)

        self.assertEqual(row["成交额"], 300)

    def test_falls_back_to_0925_row_without_0926_history(self):
        frame = FakeFrame(
            [
                {"时间": "09:24:00", "成交额": 100},
                {"时间": "09:25:00", "成交额": 200},
            ]
        )

        row = service.select_auction_row(frame)

        self.assertEqual(row["成交额"], 200)


if __name__ == "__main__":
    unittest.main()
