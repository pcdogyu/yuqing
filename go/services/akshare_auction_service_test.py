#!/usr/bin/env python3

import threading
import time
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


class HoldingBackfillConcurrencyTest(unittest.TestCase):
    def test_market_holdings_detail_requests_are_concurrent(self):
        active = 0
        max_active = 0
        call_count = 0
        lock = threading.Lock()

        class FakeAK:
            def stock_gdfx_free_holding_detail_em(self, date):
                return FakeFrame([{"股票代码": "002230", "报告期": date, "股东名称": "全国社保基金一一八组合"}])

            def stock_gdfx_holding_detail_em(self, date, indicator, symbol):
                nonlocal active, max_active, call_count
                with lock:
                    call_count += 1
                    active += 1
                    max_active = max(max_active, active)
                time.sleep(0.03)
                with lock:
                    active -= 1
                if indicator == "基金" and symbol == "新进":
                    return FakeFrame([{"股票代码": "300059", "报告期": date, "股东名称": "易方达基金"}])
                return FakeFrame([])

        items, warnings = service.fetch_market_holdings_for_period(FakeAK(), "20260331", workers=4)

        self.assertEqual(warnings, [])
        self.assertEqual(call_count, len(service.HOLDING_DETAIL_TYPES) * len(service.HOLDING_DETAIL_CHANGES))
        self.assertGreaterEqual(max_active, 2)
        self.assertEqual(sorted(item["stock_code"] for item in items), ["002230", "300059"])


if __name__ == "__main__":
    unittest.main()
