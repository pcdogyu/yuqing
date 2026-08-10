#!/usr/bin/env python3
"""Small AKShare HTTP adapter for A-share opening auction amounts and research reports.

The Go scheduler calls:
  GET /api/a-stock/auction?date=YYYY-MM-DD
  GET /api/a-stock/evening-snapshot?date=YYYY-MM-DD
  GET /api/a-stock/tdx-quote?codes=000001,600000
  GET /api/a-stock/sector-fund-flow?sector_type=行业资金流&indicator=今日&source=eastmoney
  GET /api/a-stock/stock-fund-flow?indicator=今日&source=eastmoney
  GET /api/a-stock/sector-constituents?sector_type=行业资金流&sector_name=半导体
  GET /api/a-stock/margin-trading?date=YYYY-MM-DD&market=all
  GET /api/a-stock/dividend-events?date=YYYY-MM-DD&codes=000001,600000&window_days=3
  GET /api/a-stock/holdings?period=YYYYMMDD&code=002230
  GET /api/stock-research?code=002230&start=YYYY-MM-DD&end=YYYY-MM-DD

This service uses AKShare's Eastmoney pre-market minute API and returns the
JSON contract consumed by scheduler-service. It intentionally stays on the
standard library so only AKShare and its normal data dependencies are needed.
"""

from __future__ import annotations

import argparse
import concurrent.futures
import datetime as dt
import hashlib
import html
import json
import math
import os
import re
import sys
import threading
import time
import traceback
import urllib.parse
import urllib.request
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from typing import Any


DEFAULT_HOST = "127.0.0.1"
DEFAULT_PORT = 8087
DEFAULT_WORKERS = 12
DEFAULT_HOLDING_WORKERS = 6
DEFAULT_CACHE_DIR = Path("data") / "akshare-cache" / "a-stock-auction"
DEFAULT_TRADING_DAY_CACHE_TTL_SEC = 6 * 60 * 60
DEFAULT_TDX_QUOTE_CACHE_TTL_SEC = 60
DEFAULT_RESEARCH_SYMBOLS = ["002230", "300059", "000001", "600519", "300750", "000858", "601318"]
DEFAULT_TDX_HQ_SERVERS = [
    ("119.147.212.81", 7709),
    ("119.147.212.81", 7721),
    ("202.108.253.130", 7709),
    ("180.153.18.170", 7709),
    ("47.103.48.45", 7709),
]
HOLDING_DETAIL_TYPES = ["基金", "QFII", "社保", "券商", "信托", "保险"]
HOLDING_DETAIL_CHANGES = ["新进", "增加", "不变", "减少"]
TIANTIAN_FUND_REGULAR_REPORT_URL = "https://fund.eastmoney.com/gonggao/dingqibaogao.html"
EASTMONEY_CLIST_URLS = [
    "https://push2delay.eastmoney.com/api/qt/clist/get",
    "http://push2delay.eastmoney.com/api/qt/clist/get",
    "https://push2.eastmoney.com/api/qt/clist/get",
    "http://push2.eastmoney.com/api/qt/clist/get",
    "https://82.push2.eastmoney.com/api/qt/clist/get",
    "http://82.push2.eastmoney.com/api/qt/clist/get",
]
EASTMONEY_A_STOCK_FS = "m:0+t:6,m:0+t:80,m:1+t:2,m:1+t:23"
EASTMONEY_FIELDS = "f12,f14,f2,f5,f6"
EASTMONEY_EVENING_FIELDS = "f12,f14,f2,f3,f6,f8,f10,f22"
EASTMONEY_SORT_FIELD = "f12"
AUCTION_SNAPSHOT_UNIT_NORMALIZE_MIN_ITEMS = 1000
AUCTION_SNAPSHOT_UNIT_NORMALIZE_AMOUNT_THRESHOLD = 1_000_000_000_000
AUCTION_SNAPSHOT_UNIT_NORMALIZE_VOLUME_THRESHOLD = 100_000_000
AUCTION_SNAPSHOT_UNIT_NORMALIZE_SOURCES = {"eastmoney_clist", "akshare_spot_em"}
EASTMONEY_SECTOR_TYPE_IDS = {
    "行业资金流": "2",
    "概念资金流": "3",
    "地域资金流": "1",
}
EASTMONEY_SECTOR_FUND_FLOW_SPECS = {
    "今日": {
        "sort_field": "f62",
        "stat": "1",
        "fields": "f12,f14,f2,f3,f62,f184,f66,f69,f72,f75,f78,f81,f84,f87,f204,f205,f124",
        "change_pct": "f3",
        "main_net_inflow": "f62",
        "main_net_inflow_pct": "f184",
        "super_large_net_inflow": "f66",
        "super_large_net_inflow_pct": "f69",
        "large_net_inflow": "f72",
        "large_net_inflow_pct": "f75",
        "medium_net_inflow": "f78",
        "medium_net_inflow_pct": "f81",
        "small_net_inflow": "f84",
        "small_net_inflow_pct": "f87",
        "top_stock": "f204",
    },
    "5日": {
        "sort_field": "f164",
        "stat": "5",
        "fields": "f12,f14,f2,f109,f164,f165,f166,f167,f168,f169,f170,f171,f172,f173,f257,f258,f124",
        "change_pct": "f109",
        "main_net_inflow": "f164",
        "main_net_inflow_pct": "f165",
        "super_large_net_inflow": "f166",
        "super_large_net_inflow_pct": "f167",
        "large_net_inflow": "f168",
        "large_net_inflow_pct": "f169",
        "medium_net_inflow": "f170",
        "medium_net_inflow_pct": "f171",
        "small_net_inflow": "f172",
        "small_net_inflow_pct": "f173",
        "top_stock": "f257",
    },
    "10日": {
        "sort_field": "f174",
        "stat": "10",
        "fields": "f12,f14,f2,f160,f174,f175,f176,f177,f178,f179,f180,f181,f182,f183,f260,f261,f124",
        "change_pct": "f160",
        "main_net_inflow": "f174",
        "main_net_inflow_pct": "f175",
        "super_large_net_inflow": "f176",
        "super_large_net_inflow_pct": "f177",
        "large_net_inflow": "f178",
        "large_net_inflow_pct": "f179",
        "medium_net_inflow": "f180",
        "medium_net_inflow_pct": "f181",
        "small_net_inflow": "f182",
        "small_net_inflow_pct": "f183",
        "top_stock": "f260",
    },
}
FUND_FLOW_SOURCES = ("eastmoney", "ths", "sina")
STOCK_FUND_FLOW_SOURCES = ("eastmoney", "ths", "sina")
FUND_FLOW_NUMERIC_FIELDS = (
    "change_pct",
    "main_net_inflow",
    "main_net_inflow_pct",
    "super_large_net_inflow",
    "super_large_net_inflow_pct",
    "large_net_inflow",
    "large_net_inflow_pct",
    "medium_net_inflow",
    "medium_net_inflow_pct",
    "small_net_inflow",
    "small_net_inflow_pct",
)
STOCK_FUND_FLOW_NUMERIC_FIELDS = (
    "price",
    "change_pct",
    "turnover_pct",
    "amount",
    "in_amount",
    "out_amount",
    "main_net_inflow",
    "main_net_inflow_pct",
    "super_large_net_inflow",
    "super_large_net_inflow_pct",
    "large_net_inflow",
    "large_net_inflow_pct",
    "medium_net_inflow",
    "medium_net_inflow_pct",
    "small_net_inflow",
    "small_net_inflow_pct",
)
MARGIN_MARKETS = ("sse", "szse")
THS_FUND_FLOW_SYMBOLS = {
    "今日": "即时",
    "5日": "5日排行",
    "10日": "10日排行",
}
SINA_MONEYFLOW_BASE = "https://vip.stock.finance.sina.com.cn/quotes_service/api/json_v2.php/"
SH_SZ_A_STOCK_PREFIXES = (
    "000",
    "001",
    "002",
    "003",
    "300",
    "301",
    "600",
    "601",
    "603",
    "605",
    "688",
    "689",
)
EASTMONEY_HEADERS = {
    "Accept": "application/json,text/plain,*/*",
    "Accept-Language": "zh-CN,zh;q=0.9,en;q=0.8",
    "Connection": "close",
    "Referer": "https://quote.eastmoney.com/center/gridlist.html",
    "User-Agent": (
        "Mozilla/5.0 (Windows NT 10.0; Win64; x64) "
        "AppleWebKit/537.36 (KHTML, like Gecko) Chrome/136.0.0.0 Safari/537.36"
    ),
}
ASTOCK_2026_MARKET_HOLIDAYS = {
    # 2026 State Council holiday schedule. A-share recommendation jobs must
    # stay closed on these dates even if an upstream calendar source drifts.
    "2026-01-01",
    "2026-01-02",
    "2026-01-03",
    "2026-02-15",
    "2026-02-16",
    "2026-02-17",
    "2026-02-18",
    "2026-02-19",
    "2026-02-20",
    "2026-02-21",
    "2026-02-22",
    "2026-02-23",
    "2026-04-04",
    "2026-04-05",
    "2026-04-06",
    "2026-05-01",
    "2026-05-02",
    "2026-05-03",
    "2026-05-04",
    "2026-05-05",
    "2026-06-19",
    "2026-06-20",
    "2026-06-21",
    "2026-09-25",
    "2026-09-26",
    "2026-09-27",
    "2026-10-01",
    "2026-10-02",
    "2026-10-03",
    "2026-10-04",
    "2026-10-05",
    "2026-10-06",
    "2026-10-07",
}

_akshare_module: Any | None = None
_akshare_error: str | None = None
_akshare_lock = threading.Lock()
_pytdx_hq_api: Any | None = None
_pytdx_error: str | None = None
_pytdx_lock = threading.Lock()
_trading_day_cache_lock = threading.Lock()
_trading_day_cache: dict[str, Any] = {"fetched_at": 0.0, "dates": []}
_tdx_quote_cache_lock = threading.Lock()
_tdx_quote_cache: dict[str, dict[str, Any]] = {}


def trading_day_cache_ttl_sec() -> int:
    try:
        return int(os.getenv("AKSHARE_TRADING_DAY_CACHE_TTL_SEC", str(DEFAULT_TRADING_DAY_CACHE_TTL_SEC)))
    except ValueError:
        return DEFAULT_TRADING_DAY_CACHE_TTL_SEC


def tdx_quote_cache_ttl_sec() -> int:
    try:
        return int(os.getenv("TDX_QUOTE_CACHE_TTL_SEC", str(DEFAULT_TDX_QUOTE_CACHE_TTL_SEC)))
    except ValueError:
        return DEFAULT_TDX_QUOTE_CACHE_TTL_SEC


def load_akshare() -> Any:
    global _akshare_module, _akshare_error
    with _akshare_lock:
        if _akshare_module is not None:
            return _akshare_module
        if _akshare_error:
            raise RuntimeError(_akshare_error)
        try:
            import akshare as ak  # type: ignore
        except Exception as exc:  # pragma: no cover - depends on host env
            _akshare_error = (
                "akshare is not installed or cannot be imported; run "
                "python -m pip install -r requirements-akshare.txt"
            )
            raise RuntimeError(_akshare_error) from exc
        _akshare_module = ak
        return ak


def load_pytdx_hq_api() -> Any:
    global _pytdx_hq_api, _pytdx_error
    with _pytdx_lock:
        if _pytdx_hq_api is not None:
            return _pytdx_hq_api
        if _pytdx_error:
            raise RuntimeError(_pytdx_error)
        try:
            from pytdx.hq import TdxHq_API  # type: ignore
        except Exception as exc:  # pragma: no cover - depends on host env
            _pytdx_error = (
                "pytdx is not installed or cannot be imported; run "
                "python -m pip install -r requirements-akshare.txt"
            )
            raise RuntimeError(_pytdx_error) from exc
        _pytdx_hq_api = TdxHq_API
        return _pytdx_hq_api


def tdx_hq_servers() -> list[tuple[str, int]]:
    raw = os.getenv("TDX_HQ_SERVERS", "").strip()
    if not raw:
        return DEFAULT_TDX_HQ_SERVERS
    servers: list[tuple[str, int]] = []
    for item in raw.split(","):
        item = item.strip()
        if not item:
            continue
        host, _, raw_port = item.partition(":")
        try:
            port = int(raw_port or "7709")
        except ValueError:
            port = 7709
        if host:
            servers.append((host, port))
    return servers or DEFAULT_TDX_HQ_SERVERS


def local_today() -> str:
    return dt.datetime.now(dt.timezone(dt.timedelta(hours=8))).strftime("%Y-%m-%d")


def utc_now_iso() -> str:
    return dt.datetime.now(dt.timezone.utc).isoformat().replace("+00:00", "Z")


def normalize_date(value: str | None) -> str:
    if not value:
        return local_today()
    value = value.strip()
    for fmt in ("%Y-%m-%d", "%Y%m%d"):
        try:
            return dt.datetime.strptime(value, fmt).strftime("%Y-%m-%d")
        except ValueError:
            pass
    return local_today()


def normalize_optional_date(value: str | None) -> str:
    if not value:
        return ""
    if isinstance(value, dt.datetime) or isinstance(value, dt.date) or hasattr(value, "strftime"):
        try:
            return value.strftime("%Y-%m-%d")
        except Exception:
            pass
    value = str(value).strip()
    if value.lower() in {"nat", "nan", "none"}:
        return ""
    for fmt in ("%Y-%m-%d", "%Y%m%d", "%Y/%m/%d", "%Y.%m.%d"):
        try:
            return dt.datetime.strptime(value, fmt).strftime("%Y-%m-%d")
        except ValueError:
            pass
    if "T" in value:
        return normalize_optional_date(value.split("T", 1)[0])
    if " " in value:
        return normalize_optional_date(value.split(" ", 1)[0])
    return value


def date_add(value: str, days: int) -> str:
    base = dt.datetime.strptime(normalize_date(value), "%Y-%m-%d").date()
    return (base + dt.timedelta(days=days)).strftime("%Y-%m-%d")


def optional_date_in_range(value: str, start: str, end: str) -> bool:
    normalized = normalize_optional_date(value)
    if not normalized:
        return False
    return start <= normalized <= end


def parse_trade_date(value: Any) -> str:
    text = text_value(value)
    if not text:
        return ""
    if " " in text:
        text = text.split(" ", 1)[0]
    for fmt in ("%Y-%m-%d", "%Y%m%d"):
        try:
            return dt.datetime.strptime(text, fmt).strftime("%Y-%m-%d")
        except ValueError:
            pass
    return ""


def latest_trading_day(ak: Any, today: str) -> str:
    try:
        dates = trading_day_calendar(ak)
    except Exception:
        return today
    latest = ""
    for parsed in dates:
        if parsed <= today and parsed > latest:
            latest = parsed
    return latest or today


def reset_trading_day_calendar_cache() -> None:
    with _trading_day_cache_lock:
        _trading_day_cache["fetched_at"] = 0.0
        _trading_day_cache["dates"] = []


def fetch_trading_day_calendar(ak: Any) -> list[str]:
    frame = ak.tool_trade_date_hist_sina()
    dates: list[str] = []
    try:
        iterator = frame.iterrows()
    except Exception:
        return dates
    for _, row in iterator:
        value = first_existing(row, ["trade_date", "交易日", "date", "日期"])
        parsed = parse_trade_date(value)
        if parsed:
            dates.append(parsed)
    return normalize_a_stock_trading_dates(dates)


def trading_day_calendar(ak: Any) -> list[str]:
    now = time.time()
    ttl = trading_day_cache_ttl_sec()
    if ttl > 0:
        with _trading_day_cache_lock:
            cached_dates = list(_trading_day_cache.get("dates") or [])
            fetched_at = float(_trading_day_cache.get("fetched_at") or 0)
        if cached_dates and now - fetched_at < ttl:
            return cached_dates

    dates = fetch_trading_day_calendar(ak)
    if dates and ttl > 0:
        with _trading_day_cache_lock:
            _trading_day_cache["dates"] = list(dates)
            _trading_day_cache["fetched_at"] = now
    return dates


def normalize_a_stock_trading_dates(dates: list[str]) -> list[str]:
    normalized: list[str] = []
    for value in dates:
        parsed = parse_trade_date(value)
        if not parsed:
            continue
        try:
            day = dt.datetime.strptime(parsed, "%Y-%m-%d").date()
        except ValueError:
            continue
        if day.weekday() >= 5:
            continue
        if parsed in ASTOCK_2026_MARKET_HOLIDAYS:
            continue
        normalized.append(parsed)
    return sorted(set(normalized))


def trading_day_status(ak: Any, requested_date: str) -> dict[str, Any]:
    date = normalize_date(requested_date)
    dates = trading_day_calendar(ak)
    if not dates:
        return {
            "_http_status": 503,
            "date": date,
            "is_trading_day": False,
            "source": "akshare.tool_trade_date_hist_sina",
            "reason": "calendar_unavailable",
            "message": "A-share trading calendar is unavailable.",
        }
    previous = ""
    next_date = ""
    latest = ""
    is_trading_day = date in set(dates)
    for item in dates:
        if item <= date:
            latest = item
        if item < date:
            previous = item
        if item > date and not next_date:
            next_date = item
    reason = "trading_day" if is_trading_day else "market_closed"
    message = "A-share market is open." if is_trading_day else "A-share market is closed; stock recommendations are disabled."
    return {
        "date": date,
        "is_trading_day": is_trading_day,
        "latest_trading_day": latest,
        "previous_trading_day": previous,
        "next_trading_day": next_date,
        "source": "akshare.tool_trade_date_hist_sina",
        "reason": reason,
        "message": message,
    }


def finite_float(value: Any) -> float:
    try:
        number = float(value)
    except (TypeError, ValueError):
        return 0.0
    if math.isnan(number) or math.isinf(number):
        return 0.0
    return number


def text_value(value: Any) -> str:
    if value is None:
        return ""
    try:
        if isinstance(value, float) and (math.isnan(value) or math.isinf(value)):
            return ""
    except Exception:
        pass
    text = str(value).strip()
    if text.lower() in {"nan", "nat", "none"}:
        return ""
    return text


def first_existing(row: Any, names: list[str]) -> Any:
    for name in names:
        try:
            value = row[name]
        except Exception:
            continue
        if text_value(value) != "":
            return value
    return None


def row_to_dict(row: Any) -> dict[str, Any]:
    if isinstance(row, dict):
        return row
    try:
        return row.to_dict()
    except Exception:
        return {}


def json_safe_value(value: Any) -> Any:
    if value is None:
        return None
    try:
        if isinstance(value, float) and (math.isnan(value) or math.isinf(value)):
            return None
    except Exception:
        pass
    try:
        is_na = bool(value != value)
        if is_na:
            return None
    except Exception:
        pass
    if isinstance(value, (str, int, bool)):
        return value
    if isinstance(value, float):
        return value
    if isinstance(value, (dt.datetime, dt.date)):
        return value.isoformat()
    if hasattr(value, "isoformat"):
        try:
            return value.isoformat()
        except Exception:
            pass
    return str(value)


def json_safe_row(row: Any) -> dict[str, Any]:
    return {str(key): json_safe_value(value) for key, value in row_to_dict(row).items()}


def row_time_text(row: Any) -> str:
    value = first_existing(row, ["时间", "time", "datetime", "日期"])
    return text_value(value)


def row_time_key(row: Any) -> str:
    time_text = row_time_text(row)
    match = re.search(r"\b(\d{1,2}):(\d{2})(?::(\d{2}))?", time_text)
    if not match:
        return time_text
    hour = int(match.group(1))
    minute = int(match.group(2))
    second = int(match.group(3) or 0)
    return f"{hour:02d}:{minute:02d}:{second:02d}"


def select_auction_row(frame: Any) -> Any | None:
    if frame is None or getattr(frame, "empty", False):
        return None
    rows = []
    try:
        iterator = frame.iterrows()
    except Exception:
        return None
    for _, row in iterator:
        rows.append((row_time_key(row), row))
    fill_rows = [item for item in rows if item[0].startswith("09:26")]
    if fill_rows:
        return fill_rows[-1][1]
    auction_rows = [item for item in rows if item[0].startswith("09:25")]
    if auction_rows:
        return auction_rows[-1][1]
    before_auction = [item for item in rows if item[0] <= "09:25:59"]
    if before_auction:
        return before_auction[-1][1]
    if rows:
        return rows[-1][1]
    return None


def call_pre_market_minute(ak: Any, code: str) -> Any:
    # AKShare has used both start_time/end_time and start_date/end_date in docs
    # across versions. Try the current spelling first and fall back.
    try:
        return ak.stock_zh_a_hist_pre_min_em(
            symbol=code, start_time="09:24:00", end_time="09:26:00"
        )
    except TypeError:
        return ak.stock_zh_a_hist_pre_min_em(
            symbol=code, start_date="09:24:00", end_date="09:26:00"
        )


def is_sh_sz_code(code: Any) -> bool:
    normalized = text_value(code).zfill(6)
    return len(normalized) == 6 and normalized.isdigit() and normalized.startswith(SH_SZ_A_STOCK_PREFIXES)


def normalize_sh_sz_code(code: Any) -> str:
    normalized = text_value(code).lower()
    if normalized.startswith(("sh", "sz")):
        normalized = normalized[2:]
    normalized = normalized.zfill(6)
    if is_sh_sz_code(normalized):
        return normalized
    return ""


def tdx_market_for_code(code: Any) -> int | None:
    normalized = normalize_sh_sz_code(code)
    if not normalized:
        return None
    if normalized.startswith(("600", "601", "603", "605", "688", "689")):
        return 1
    return 0


def tdx_quote_item_from_row(row: dict[str, Any]) -> dict[str, Any] | None:
    code = normalize_sh_sz_code(row.get("code"))
    if not code:
        return None
    price = finite_float(row.get("price"))
    if price <= 0:
        return None
    last_close = finite_float(row.get("last_close"))
    pct = finite_float(row.get("pct"))
    if pct == 0 and last_close > 0:
        pct = (price / last_close - 1) * 100
    item = {
        "code": code,
        "name": text_value(row.get("name")),
        "price": round(price, 4),
        "open": round(finite_float(row.get("open")), 4),
        "pct": round(pct, 4),
        "source": "tdx",
        "cached": bool(row.get("cached")),
        "fetched_at": row.get("fetched_at") or utc_now_iso(),
    }
    return item


def parse_tdx_quote_codes(value: str | None) -> list[str]:
    if not value:
        return []
    result: list[str] = []
    seen: set[str] = set()
    for item in value.replace(";", ",").split(","):
        code = normalize_sh_sz_code(item)
        if not code or code in seen:
            continue
        seen.add(code)
        result.append(code)
    return result


def has_resolved_stock_name(code: Any, name: Any) -> bool:
    normalized_code = text_value(code).zfill(6)
    normalized_name = text_value(name)
    if not normalized_name:
        return False
    if is_placeholder_stock_name(normalized_name):
        return False
    if normalized_name == normalized_code:
        return False
    return not normalized_name.isdigit()


def is_placeholder_stock_name(name: Any) -> bool:
    normalized_name = text_value(name)
    if not normalized_name:
        return False
    if normalized_name in {"金十数据整理", "金十数据", "金十快讯", "金十资讯", "金十全站", "金十期货"}:
        return True
    return "数据整理" in normalized_name


def stock_code_name_items_from_frame(frame: Any, source: str, limit: int = 0) -> list[dict[str, str]]:
    items: list[dict[str, str]] = []
    seen_codes: set[str] = set()
    for _, row in frame.iterrows():
        code = text_value(first_existing(row, ["代码", "code", "股票代码"]))
        name = text_value(first_existing(row, ["名称", "name", "股票名称"]))
        if not code or not is_sh_sz_code(code) or not has_resolved_stock_name(code, name):
            continue
        code = code.zfill(6)
        if code in seen_codes:
            continue
        seen_codes.add(code)
        items.append({"code": code, "name": name, "source": source, "updated_at": utc_now_iso()})
        if limit > 0 and len(items) >= limit:
            break
    return items


def load_stock_code_names(ak: Any, limit: int = 0) -> tuple[list[dict[str, str]], str]:
    errors: list[str] = []
    for loader_name, source in (("stock_info_a_code_name", "akshare_code_name"), ("stock_zh_a_spot_em", "eastmoney_spot")):
        loader = getattr(ak, loader_name, None)
        if loader is None:
            errors.append(f"{loader_name}: unavailable")
            continue
        try:
            items = stock_code_name_items_from_frame(loader(), source, limit)
            if items:
                return items, ""
            errors.append(f"{loader_name}: no usable code names")
        except Exception as exc:
            errors.append(f"{loader_name}: {exc}")
    return [], "; ".join(errors)


def load_symbols(ak: Any, explicit_codes: list[str], limit: int, code_names: list[dict[str, str]] | None = None) -> list[dict[str, str]]:
    names_by_code = {text_value(item.get("code")).zfill(6): text_value(item.get("name")) for item in code_names or []}
    if explicit_codes:
        symbols = []
        for code in explicit_codes:
            normalized = text_value(code).zfill(6)
            if not is_sh_sz_code(normalized):
                continue
            symbols.append({"code": normalized, "name": names_by_code.get(normalized, "")})
            if limit > 0 and len(symbols) >= limit:
                break
        return symbols
    if code_names is None:
        code_names, _ = load_stock_code_names(ak, limit)
    symbols = [{"code": item["code"], "name": item["name"]} for item in code_names if item.get("code")]
    if limit > 0:
        symbols = symbols[:limit]
    return symbols


def normalize_symbol_limit(limit: int) -> int:
    return max(0, limit)


def should_retry_full_market_snapshot_with_eastmoney(limit: int, items: list[dict[str, Any]]) -> bool:
    return limit <= 0 and 0 < len(items) < 1000


def fetch_market_snapshot(ak: Any, trade_date: str, limit: int) -> list[dict[str, Any]]:
    frame = ak.stock_zh_a_spot_em()
    items: list[dict[str, Any]] = []
    fetched_at = utc_now_iso()
    for _, row in frame.iterrows():
        code = text_value(first_existing(row, ["代码", "code", "股票代码"]))
        name = text_value(first_existing(row, ["名称", "name", "股票名称"]))
        if not code or not is_sh_sz_code(code) or not has_resolved_stock_name(code, name):
            continue
        price = finite_float(first_existing(row, ["最新价", "今开", "开盘", "price"]))
        volume = finite_float(first_existing(row, ["成交量", "volume"]))
        amount = finite_float(first_existing(row, ["成交额", "amount"]))
        items.append(
            {
                "trade_date": trade_date,
                "code": code.zfill(6),
                "name": name,
                "auction_price": price,
                "auction_volume": volume,
                "auction_amount": amount,
                "source": "akshare_spot_em",
                "status": "ok" if amount > 0 or volume > 0 else "no_auction_amount",
                "fetched_at": fetched_at,
            }
        )
        if limit > 0 and len(items) >= limit:
            break
    return items


def fetch_evening_snapshot(ak: Any, trade_date: str, limit: int) -> list[dict[str, Any]]:
    frame = ak.stock_zh_a_spot_em()
    items: list[dict[str, Any]] = []
    fetched_at = utc_now_iso()
    for _, row in frame.iterrows():
        code = text_value(first_existing(row, ["代码", "code", "股票代码"]))
        name = text_value(first_existing(row, ["名称", "name", "股票名称"]))
        if not code or not is_sh_sz_code(code) or not has_resolved_stock_name(code, name):
            continue
        price = finite_float(first_existing(row, ["最新价", "当前价", "price"]))
        change_pct = parse_percent_value(first_existing(row, ["涨跌幅", "change_pct"]))
        volume_ratio = finite_float(first_existing(row, ["量比", "volume_ratio"]))
        turnover_pct = parse_percent_value(first_existing(row, ["换手率", "turnover_pct"]))
        amount = finite_float(first_existing(row, ["成交额", "amount"]))
        speed = parse_percent_value(first_existing(row, ["涨速", "speed"]))
        items.append(
            {
                "trade_date": trade_date,
                "code": code.zfill(6),
                "name": name,
                "price": price,
                "change_pct": change_pct,
                "volume_ratio": volume_ratio,
                "turnover_pct": turnover_pct,
                "amount": amount,
                "speed": speed,
                "source": "akshare_spot_em",
                "status": "ok" if price > 0 and amount > 0 else "no_evening_market_data",
                "fetched_at": fetched_at,
            }
        )
        if limit > 0 and len(items) >= limit:
            break
    return items


def eastmoney_rows_to_items(rows: list[dict[str, Any]], trade_date: str, limit: int) -> list[dict[str, Any]]:
    fetched_at = utc_now_iso()
    items: list[dict[str, Any]] = []
    seen_codes: set[str] = set()
    for row in rows:
        code = text_value(row.get("f12"))
        name = text_value(row.get("f14"))
        if not code or not is_sh_sz_code(code) or not has_resolved_stock_name(code, name):
            continue
        code = code.zfill(6)
        if code in seen_codes:
            continue
        seen_codes.add(code)
        price = finite_float(row.get("f2"))
        volume = finite_float(row.get("f5"))
        amount = finite_float(row.get("f6"))
        items.append(
            {
                "trade_date": trade_date,
                "code": code,
                "name": name,
                "auction_price": price,
                "auction_volume": volume,
                "auction_amount": amount,
                "source": "eastmoney_clist",
                "status": "ok" if amount > 0 or volume > 0 else "no_auction_amount",
                "fetched_at": fetched_at,
            }
        )
        if limit > 0 and len(items) >= limit:
            break
    return items


def eastmoney_rows_to_evening_items(rows: list[dict[str, Any]], trade_date: str, limit: int) -> list[dict[str, Any]]:
    fetched_at = utc_now_iso()
    items: list[dict[str, Any]] = []
    seen_codes: set[str] = set()
    for row in rows:
        code = text_value(row.get("f12"))
        name = text_value(row.get("f14"))
        if not code or not is_sh_sz_code(code) or not has_resolved_stock_name(code, name):
            continue
        code = code.zfill(6)
        if code in seen_codes:
            continue
        seen_codes.add(code)
        price = finite_float(row.get("f2"))
        change_pct = finite_float(row.get("f3"))
        amount = finite_float(row.get("f6"))
        turnover_pct = finite_float(row.get("f8"))
        volume_ratio = finite_float(row.get("f10"))
        speed = finite_float(row.get("f22"))
        items.append(
            {
                "trade_date": trade_date,
                "code": code,
                "name": name,
                "price": price,
                "change_pct": change_pct,
                "volume_ratio": volume_ratio,
                "turnover_pct": turnover_pct,
                "amount": amount,
                "speed": speed,
                "source": "eastmoney_clist",
                "status": "ok" if price > 0 and amount > 0 else "no_evening_market_data",
                "fetched_at": fetched_at,
            }
        )
        if limit > 0 and len(items) >= limit:
            break
    return items


def evening_snapshot_metric_warning(items: list[dict[str, Any]]) -> str:
    if not items:
        return ""
    missing_metrics: list[str] = []
    if not any(finite_float(item.get("volume_ratio")) > 0 for item in items):
        missing_metrics.append("volume_ratio")
    if not any(finite_float(item.get("speed")) != 0 for item in items):
        missing_metrics.append("speed")
    if not missing_metrics:
        return ""
    return (
        "evening snapshot missing usable "
        + "/".join(missing_metrics)
        + " metrics; rows are returned for diagnostics only"
    )


def should_normalize_auction_snapshot_units(items: list[dict[str, Any]]) -> bool:
    if len(items) < AUCTION_SNAPSHOT_UNIT_NORMALIZE_MIN_ITEMS:
        return False
    snapshot_source_count = 0
    total_volume = 0.0
    total_amount = 0.0
    for item in items:
        if text_value(item.get("source")) in AUCTION_SNAPSHOT_UNIT_NORMALIZE_SOURCES:
            snapshot_source_count += 1
        total_volume += finite_float(item.get("auction_volume"))
        total_amount += finite_float(item.get("auction_amount"))
    if snapshot_source_count * 2 < len(items):
        return False
    return (
        total_amount >= AUCTION_SNAPSHOT_UNIT_NORMALIZE_AMOUNT_THRESHOLD
        or total_volume >= AUCTION_SNAPSHOT_UNIT_NORMALIZE_VOLUME_THRESHOLD
    )


def normalize_auction_snapshot_units(items: list[dict[str, Any]]) -> list[dict[str, Any]]:
    if not should_normalize_auction_snapshot_units(items):
        return items
    normalized: list[dict[str, Any]] = []
    for item in items:
        copied = dict(item)
        volume = finite_float(copied.get("auction_volume"))
        amount = finite_float(copied.get("auction_amount"))
        if volume > 0:
            copied["auction_volume"] = volume / 100
        if amount > 0:
            copied["auction_amount"] = amount / 100
        normalized.append(copied)
    return normalized


def eastmoney_page_size(limit: int) -> int:
    if limit > 0:
        return max(1, min(limit, 100))
    return 100


def eastmoney_target_row_count(total: int, limit: int) -> int:
    if total <= 0:
        return max(0, limit)
    if limit > 0:
        return min(total, limit)
    return total


def fetch_eastmoney_snapshot(trade_date: str, limit: int) -> list[dict[str, Any]]:
    page_size = eastmoney_page_size(limit)
    errors: list[str] = []
    for base_url in EASTMONEY_CLIST_URLS:
        rows: list[dict[str, Any]] = []
        total = 0
        try:
            page = 1
            while True:
                params = {
                    "pn": str(page),
                    "pz": str(page_size),
                    "po": "1",
                    "np": "1",
                    "fltt": "2",
                    "invt": "2",
                    "fid": EASTMONEY_SORT_FIELD,
                    "fs": EASTMONEY_A_STOCK_FS,
                    "fields": EASTMONEY_FIELDS,
                    "_": str(int(time.time() * 1000)),
                }
                query = urllib.parse.urlencode(params)
                url = base_url + "?" + query
                payload: dict[str, Any] | None = None
                page_errors: list[str] = []
                for attempt in range(3):
                    try:
                        request = urllib.request.Request(url, headers=EASTMONEY_HEADERS)
                        with urllib.request.urlopen(request, timeout=20) as response:
                            payload = json.loads(response.read().decode("utf-8"))
                        break
                    except Exception as exc:  # pragma: no cover - external service variability
                        page_errors.append(f"{base_url} page {page} attempt {attempt + 1}: {exc}")
                        time.sleep(0.3 * (attempt + 1))
                if payload is None:
                    raise RuntimeError("; ".join(page_errors) or f"{base_url}: request failed")
                data = payload.get("data") if isinstance(payload, dict) else None
                page_rows = data.get("diff") if isinstance(data, dict) else None
                if total <= 0:
                    total = int(data.get("total") or 0) if isinstance(data, dict) else 0
                if not isinstance(page_rows, list) or not page_rows:
                    if rows:
                        break
                    raise RuntimeError(f"{base_url}: empty diff on page {page}")
                rows.extend(page_rows)
                target_rows = eastmoney_target_row_count(total, limit)
                if limit > 0 and len(rows) >= limit:
                    break
                if target_rows > 0 and len(rows) >= target_rows:
                    break
                if len(page_rows) < page_size:
                    break
                page += 1
            if rows:
                return eastmoney_rows_to_items(rows, trade_date, limit)
            errors.append(f"{base_url}: empty diff")
        except Exception as exc:  # pragma: no cover - external service variability
            errors.append(str(exc))
            continue
    raise RuntimeError("; ".join(errors) or "Eastmoney clist returned no data")


def fetch_eastmoney_evening_snapshot(trade_date: str, limit: int) -> list[dict[str, Any]]:
    page_size = eastmoney_page_size(limit)
    errors: list[str] = []
    for base_url in EASTMONEY_CLIST_URLS:
        rows: list[dict[str, Any]] = []
        total = 0
        try:
            page = 1
            while True:
                params = {
                    "pn": str(page),
                    "pz": str(page_size),
                    "po": "1",
                    "np": "1",
                    "fltt": "2",
                    "invt": "2",
                    "fid": EASTMONEY_SORT_FIELD,
                    "fs": EASTMONEY_A_STOCK_FS,
                    "fields": EASTMONEY_EVENING_FIELDS,
                    "_": str(int(time.time() * 1000)),
                }
                query = urllib.parse.urlencode(params)
                url = base_url + "?" + query
                payload: dict[str, Any] | None = None
                page_errors: list[str] = []
                for attempt in range(3):
                    try:
                        request = urllib.request.Request(url, headers=EASTMONEY_HEADERS)
                        with urllib.request.urlopen(request, timeout=20) as response:
                            payload = json.loads(response.read().decode("utf-8"))
                        break
                    except Exception as exc:  # pragma: no cover - external service variability
                        page_errors.append(f"{base_url} evening page {page} attempt {attempt + 1}: {exc}")
                        time.sleep(0.3 * (attempt + 1))
                if payload is None:
                    raise RuntimeError("; ".join(page_errors) or f"{base_url}: evening request failed")
                data = payload.get("data") if isinstance(payload, dict) else None
                page_rows = data.get("diff") if isinstance(data, dict) else None
                if total <= 0:
                    total = int(data.get("total") or 0) if isinstance(data, dict) else 0
                if not isinstance(page_rows, list) or not page_rows:
                    if rows:
                        break
                    raise RuntimeError(f"{base_url}: empty evening diff on page {page}")
                rows.extend(page_rows)
                target_rows = eastmoney_target_row_count(total, limit)
                if limit > 0 and len(rows) >= limit:
                    break
                if target_rows > 0 and len(rows) >= target_rows:
                    break
                if len(page_rows) < page_size:
                    break
                page += 1
            if rows:
                return eastmoney_rows_to_evening_items(rows, trade_date, limit)
            errors.append(f"{base_url}: empty evening diff")
        except Exception as exc:  # pragma: no cover - external service variability
            errors.append(str(exc))
            continue
    raise RuntimeError("; ".join(errors) or "Eastmoney evening clist returned no data")


def fetch_one_auction(ak: Any, symbol: dict[str, str], trade_date: str) -> dict[str, Any]:
    code = symbol["code"]
    name = symbol.get("name", "")
    fetched_at = utc_now_iso()
    try:
        frame = call_pre_market_minute(ak, code)
        row = select_auction_row(frame)
        if row is None:
            return {
                "trade_date": trade_date,
                "code": code,
                "name": name,
                "auction_price": 0,
                "auction_volume": 0,
                "auction_amount": 0,
                "source": "akshare_pre_min",
                "status": "no_auction_data",
                "fetched_at": fetched_at,
            }
        price = finite_float(first_existing(row, ["最新价", "收盘", "开盘", "price"]))
        volume = finite_float(first_existing(row, ["成交量", "volume"]))
        amount = finite_float(first_existing(row, ["成交额", "amount"]))
        return {
            "trade_date": trade_date,
            "code": code,
            "name": name,
            "auction_price": price,
            "auction_volume": volume,
            "auction_amount": amount,
            "source": "akshare_pre_min",
            "status": "ok" if amount > 0 or volume > 0 else "no_auction_amount",
            "fetched_at": fetched_at,
        }
    except Exception as exc:  # pragma: no cover - external service variability
        return {
            "trade_date": trade_date,
            "code": code,
            "name": name,
            "auction_price": 0,
            "auction_volume": 0,
            "auction_amount": 0,
            "source": "akshare_pre_min",
            "status": "failed",
            "fetched_at": fetched_at,
            "error": str(exc),
        }


def cache_path(cache_dir: Path, trade_date: str) -> Path:
    return cache_dir / f"{trade_date}.json"


def read_cache(cache_dir: Path, trade_date: str) -> dict[str, Any] | None:
    path = cache_path(cache_dir, trade_date)
    if not path.exists():
        return None
    try:
        return json.loads(path.read_text(encoding="utf-8"))
    except Exception:
        return None


def write_cache(cache_dir: Path, trade_date: str, payload: dict[str, Any]) -> None:
    cache_dir.mkdir(parents=True, exist_ok=True)
    cache_path(cache_dir, trade_date).write_text(
        json.dumps(payload, ensure_ascii=False, separators=(",", ":")),
        encoding="utf-8",
    )


def item_has_usable_amount(item: dict[str, Any]) -> bool:
    if not is_sh_sz_code(item.get("code")) or not has_resolved_stock_name(item.get("code"), item.get("name")):
        return False
    status = text_value(item.get("status")).lower()
    if status and status != "ok":
        return False
    return finite_float(item.get("auction_amount")) > 0 or finite_float(item.get("auction_volume")) > 0


def payload_has_usable_items(payload: dict[str, Any]) -> bool:
    items = payload.get("items")
    if not isinstance(items, list):
        return False
    return any(isinstance(item, dict) and item_has_usable_amount(item) for item in items)


def source_key(parts: list[str]) -> str:
    return hashlib.sha1("|".join(parts).encode("utf-8")).hexdigest()


def report_date_text(row: Any) -> str:
    value = first_existing(row, ["日期", "date", "报告日期", "publish_date", "research_date"])
    return normalize_optional_date(text_value(json_safe_value(value)))


def report_summary(row: Any) -> str:
    parts: list[str] = []
    industry = text_value(first_existing(row, ["行业", "industry"]))
    if industry:
        parts.append(f"行业：{industry}")
    month_count = text_value(first_existing(row, ["近一月个股研报数"]))
    if month_count:
        parts.append(f"近一月研报数：{month_count}")
    for year in ("2026", "2027", "2028"):
        eps = text_value(first_existing(row, [f"{year}-盈利预测-收益"]))
        pe = text_value(first_existing(row, [f"{year}-盈利预测-市盈率"]))
        if eps or pe:
            parts.append(f"{year} EPS {eps or '--'} / PE {pe or '--'}")
    return "；".join(parts)


def research_symbols_from_env() -> list[str]:
    raw = os.getenv("AKSHARE_RESEARCH_DEFAULT_SYMBOLS", "")
    values = [item.strip().zfill(6) for item in raw.split(",") if item.strip()]
    return values or DEFAULT_RESEARCH_SYMBOLS


def resolve_research_symbols(ak: Any, query: dict[str, list[str]]) -> tuple[list[str], str]:
    code_value = first_query_value(query, "code") or first_query_value(query, "symbol") or ""
    explicit_codes = [item.strip().zfill(6) for item in code_value.split(",") if item.strip()]
    if explicit_codes:
        return explicit_codes, ""

    company = first_query_value(query, "company") or first_query_value(query, "name") or ""
    company = company.strip()
    if not company:
        return research_symbols_from_env(), "no code/company supplied; used default research symbols"

    frames = []
    for loader in (getattr(ak, "stock_info_a_code_name", None), getattr(ak, "stock_zh_a_spot_em", None)):
        if loader is None:
            continue
        try:
            frames.append(loader())
        except Exception:
            continue
    for frame in frames:
        try:
            iterator = frame.iterrows()
        except Exception:
            continue
        for _, row in iterator:
            code = text_value(first_existing(row, ["代码", "code", "股票代码"]))
            name = text_value(first_existing(row, ["名称", "name", "股票名称"]))
            if code and (company in name or name in company):
                return [code.zfill(6)], ""
    return [], f"company not found in A-share symbol list: {company}"


def research_report_item(row: Any, fallback_code: str) -> dict[str, Any] | None:
    code = text_value(first_existing(row, ["股票代码", "code", "股票代码"])) or fallback_code
    code = code.zfill(6) if code else ""
    name = text_value(first_existing(row, ["股票简称", "名称", "name", "股票名称"]))
    title = text_value(first_existing(row, ["报告名称", "title", "标题"]))
    institution = text_value(first_existing(row, ["机构", "institution"]))
    rating = text_value(first_existing(row, ["东财评级", "评级", "rating"]))
    research_date = report_date_text(row)
    pdf_url = text_value(first_existing(row, ["报告PDF链接", "pdf_url", "PDF链接"]))
    if not title:
        return None
    raw_payload = json.dumps(json_safe_row(row), ensure_ascii=False, separators=(",", ":"))
    key = source_key(["akshare_stock_research", code, title, institution, research_date, pdf_url])
    return {
        "code": code,
        "name": name,
        "kind": "report",
        "title": title,
        "institution": institution,
        "analyst": "",
        "rating": rating,
        "target_price": "",
        "research_date": research_date,
        "publish_time": research_date,
        "source_url": pdf_url,
        "source_type": "akshare_stock_research",
        "source_key": key,
        "summary": report_summary(row),
        "raw_payload": raw_payload,
        "pdf_url": pdf_url,
        "pdf_status": "pending" if pdf_url else "no_pdf",
    }


def date_in_range(value: str, start: str, end: str) -> bool:
    if value:
        if start and value < start:
            return False
        if end and value > end:
            return False
    return True


def fetch_research_reports_for_symbol(ak: Any, symbol: str, start: str, end: str) -> list[dict[str, Any]]:
    frame = ak.stock_research_report_em(symbol=symbol)
    items: list[dict[str, Any]] = []
    try:
        iterator = frame.iterrows()
    except Exception:
        return items
    for _, row in iterator:
        item = research_report_item(row, symbol)
        if item is None:
            continue
        if not date_in_range(text_value(item.get("research_date")), start, end):
            continue
        items.append(item)
    return items


def normalize_sector_fund_flow_sector_type(value: str | None) -> str:
    text = text_value(value)
    if text in {"概念", "概念资金", "概念资金流"}:
        return "概念资金流"
    return "行业资金流"


def normalize_sector_fund_flow_indicator(value: str | None) -> str:
    text = text_value(value)
    if text in {"5", "5日"}:
        return "5日"
    if text in {"10", "10日"}:
        return "10日"
    return "今日"


def normalize_fund_flow_source(value: str | None) -> str:
    text = text_value(value).lower()
    aliases = {
        "em": "eastmoney",
        "east_money": "eastmoney",
        "东方财富": "eastmoney",
        "akshare_sector_fund_flow": "eastmoney",
        "同花顺": "ths",
        "10jqka": "ths",
        "新浪": "sina",
        "sina_finance": "sina",
        "sinafinance": "sina",
        "average": "all",
        "aggregate": "all",
    }
    if not text:
        return "all"
    return aliases.get(text, text)


def fund_flow_field_counts_json(fields: list[str] | tuple[str, ...] | set[str]) -> str:
    counts = {field: 1 for field in sorted(set(fields)) if text_value(field)}
    return json.dumps(counts, ensure_ascii=False, separators=(",", ":")) if counts else "{}"


def parse_fund_flow_field_counts(raw: Any) -> dict[str, int]:
    text = text_value(raw)
    if not text:
        return {}
    try:
        parsed = json.loads(text)
    except Exception:
        return {}
    if not isinstance(parsed, dict):
        return {}
    counts: dict[str, int] = {}
    for key, value in parsed.items():
        try:
            number = int(float(value))
        except Exception:
            number = 0
        if number > 0:
            counts[str(key)] = number
    return counts


def parse_amount_to_yuan(value: Any, default_unit: str = "") -> float:
    text = text_value(value).replace(",", "")
    if not text or text in {"-", "--"}:
        return 0.0
    multiplier = 1.0
    if "亿元" in text or "亿" in text:
        multiplier = 100000000.0
    elif "万元" in text or "万" in text:
        multiplier = 10000.0
    elif default_unit == "亿":
        multiplier = 100000000.0
    elif default_unit == "万":
        multiplier = 10000.0
    for token in ("亿元", "万元", "元", "亿", "万", "%"):
        text = text.replace(token, "")
    return finite_float(text) * multiplier


def parse_unit_number(value: Any, default_unit: str = "") -> float | None:
    text = text_value(value).replace(",", "")
    if not text or text in {"-", "--"}:
        return None
    multiplier = 1.0
    if "亿元" in text or "亿股" in text or "亿份" in text or "亿" in text:
        multiplier = 100000000.0
    elif "万元" in text or "万股" in text or "万份" in text or "万" in text:
        multiplier = 10000.0
    elif default_unit == "亿":
        multiplier = 100000000.0
    elif default_unit == "万":
        multiplier = 10000.0
    for token in ("亿元", "万元", "元", "亿股", "万股", "亿份", "万份", "股/份", "股", "份", "亿", "万", "%"):
        text = text.replace(token, "")
    return finite_float(text) * multiplier


def akshare_date_arg(date_text: str) -> str:
    normalized = normalize_date(date_text)
    return normalized.replace("-", "")


def parse_percent_value(value: Any, decimal_ratio: bool = False) -> float:
    text = text_value(value).replace(",", "")
    if not text or text in {"-", "--"}:
        return 0.0
    has_percent = "%" in text
    text = text.replace("%", "")
    number = finite_float(text)
    if decimal_ratio and not has_percent:
        number *= 100.0
    return number


def set_numeric_field(item: dict[str, Any], row: Any, fields: list[str], key: str, names: list[str], transform: Any = finite_float) -> None:
    value = first_existing(row, names)
    if text_value(value) == "":
        item[key] = 0.0
        return
    item[key] = transform(value)
    fields.append(key)


def sector_fund_flow_item(row: Any, trade_date: str, sector_type: str, indicator: str) -> dict[str, Any] | None:
    name = text_value(first_existing(row, ["名称", "板块名称", "版块名称", "name"]))
    if not name:
        return None
    rank = int(finite_float(first_existing(row, ["序号", "排名", "rank"])))
    raw_payload = json.dumps(json_safe_row(row), ensure_ascii=False, separators=(",", ":"))
    fields: list[str] = []
    item = {
        "trade_date": trade_date,
        "sector_type": sector_type,
        "indicator": indicator,
        "rank": rank,
        "name": name,
        "top_stock": text_value(first_existing(row, [f"{indicator}主力净流入最大股", "今日主力净流入最大股", "主力净流入最大股"])),
        "source_type": "eastmoney",
        "raw_payload": raw_payload,
        "fetched_at": utc_now_iso(),
    }
    set_numeric_field(item, row, fields, "change_pct", [f"{indicator}涨跌幅", "今日涨跌幅", "涨跌幅"])
    set_numeric_field(item, row, fields, "main_net_inflow", [f"{indicator}主力净流入-净额", "今日主力净流入-净额", "主力净流入-净额"])
    set_numeric_field(item, row, fields, "main_net_inflow_pct", [f"{indicator}主力净流入-净占比", "今日主力净流入-净占比", "主力净流入-净占比"])
    set_numeric_field(item, row, fields, "super_large_net_inflow", [f"{indicator}超大单净流入-净额", "今日超大单净流入-净额", "超大单净流入-净额"])
    set_numeric_field(item, row, fields, "super_large_net_inflow_pct", [f"{indicator}超大单净流入-净占比", "今日超大单净流入-净占比", "超大单净流入-净占比"])
    set_numeric_field(item, row, fields, "large_net_inflow", [f"{indicator}大单净流入-净额", "今日大单净流入-净额", "大单净流入-净额"])
    set_numeric_field(item, row, fields, "large_net_inflow_pct", [f"{indicator}大单净流入-净占比", "今日大单净流入-净占比", "大单净流入-净占比"])
    set_numeric_field(item, row, fields, "medium_net_inflow", [f"{indicator}中单净流入-净额", "今日中单净流入-净额", "中单净流入-净额"])
    set_numeric_field(item, row, fields, "medium_net_inflow_pct", [f"{indicator}中单净流入-净占比", "今日中单净流入-净占比", "中单净流入-净占比"])
    set_numeric_field(item, row, fields, "small_net_inflow", [f"{indicator}小单净流入-净额", "今日小单净流入-净额", "小单净流入-净额"])
    set_numeric_field(item, row, fields, "small_net_inflow_pct", [f"{indicator}小单净流入-净占比", "今日小单净流入-净占比", "小单净流入-净占比"])
    item["field_counts_json"] = fund_flow_field_counts_json(fields)
    return item


def eastmoney_sector_fund_flow_rows_to_items(
    rows: list[dict[str, Any]], trade_date: str, sector_type: str, indicator: str, limit: int
) -> list[dict[str, Any]]:
    spec = EASTMONEY_SECTOR_FUND_FLOW_SPECS[indicator]
    ranked_rows = [row for row in rows if isinstance(row, dict) and text_value(row.get("f14"))]
    ranked_rows.sort(key=lambda row: finite_float(row.get(spec["main_net_inflow"])), reverse=True)
    items: list[dict[str, Any]] = []
    for rank, row in enumerate(ranked_rows, start=1):
        mapped = {
            "序号": rank,
            "名称": text_value(row.get("f14")),
            f"{indicator}涨跌幅": row.get(spec["change_pct"]),
            f"{indicator}主力净流入-净额": row.get(spec["main_net_inflow"]),
            f"{indicator}主力净流入-净占比": row.get(spec["main_net_inflow_pct"]),
            f"{indicator}超大单净流入-净额": row.get(spec["super_large_net_inflow"]),
            f"{indicator}超大单净流入-净占比": row.get(spec["super_large_net_inflow_pct"]),
            f"{indicator}大单净流入-净额": row.get(spec["large_net_inflow"]),
            f"{indicator}大单净流入-净占比": row.get(spec["large_net_inflow_pct"]),
            f"{indicator}中单净流入-净额": row.get(spec["medium_net_inflow"]),
            f"{indicator}中单净流入-净占比": row.get(spec["medium_net_inflow_pct"]),
            f"{indicator}小单净流入-净额": row.get(spec["small_net_inflow"]),
            f"{indicator}小单净流入-净占比": row.get(spec["small_net_inflow_pct"]),
            f"{indicator}主力净流入最大股": text_value(row.get(spec["top_stock"])),
        }
        item = sector_fund_flow_item(mapped, trade_date, sector_type, indicator)
        if item is None:
            continue
        item["raw_payload"] = json.dumps(json_safe_row(row), ensure_ascii=False, separators=(",", ":"))
        items.append(item)
        if limit > 0 and len(items) >= limit:
            break
    return items


def fetch_eastmoney_sector_fund_flow_rank(
    trade_date: str, sector_type: str, indicator: str, limit: int
) -> list[dict[str, Any]]:
    spec = EASTMONEY_SECTOR_FUND_FLOW_SPECS[indicator]
    sector_type_id = EASTMONEY_SECTOR_TYPE_IDS[sector_type]
    errors: list[str] = []
    for base_url in EASTMONEY_CLIST_URLS:
        rows: list[dict[str, Any]] = []
        total = 0
        try:
            page = 1
            while True:
                params = {
                    "pn": str(page),
                    "pz": "100",
                    "po": "1",
                    "np": "1",
                    "ut": "b2884a393a59ad64002292a3e90d46a5",
                    "fltt": "2",
                    "invt": "2",
                    "fid0": spec["sort_field"],
                    "fs": f"m:90 t:{sector_type_id}",
                    "stat": spec["stat"],
                    "fields": spec["fields"],
                    "rt": "52975239",
                    "_": str(int(time.time() * 1000)),
                }
                url = base_url + "?" + urllib.parse.urlencode(params)
                payload: dict[str, Any] | None = None
                page_errors: list[str] = []
                for attempt in range(3):
                    try:
                        request = urllib.request.Request(url, headers=EASTMONEY_HEADERS)
                        with urllib.request.urlopen(request, timeout=20) as response:
                            payload = json.loads(response.read().decode("utf-8"))
                        break
                    except Exception as exc:  # pragma: no cover - external service variability
                        page_errors.append(f"{base_url} page {page} attempt {attempt + 1}: {exc}")
                        time.sleep(0.3 * (attempt + 1))
                if payload is None:
                    raise RuntimeError("; ".join(page_errors) or f"{base_url}: request failed")
                data = payload.get("data") if isinstance(payload, dict) else None
                page_rows = data.get("diff") if isinstance(data, dict) else None
                if total <= 0:
                    total = int(data.get("total") or 0) if isinstance(data, dict) else 0
                if not isinstance(page_rows, list) or not page_rows:
                    if rows:
                        break
                    raise RuntimeError(f"{base_url}: empty diff on page {page}")
                rows.extend(page_rows)
                if total > 0 and len(rows) >= total:
                    break
                if len(page_rows) < 100:
                    break
                page += 1
            if rows:
                return eastmoney_sector_fund_flow_rows_to_items(rows, trade_date, sector_type, indicator, limit)
            errors.append(f"{base_url}: empty diff")
        except Exception as exc:  # pragma: no cover - external service variability
            errors.append(str(exc))
            continue
    raise RuntimeError("; ".join(errors) or "Eastmoney sector fund flow returned no data")


def fetch_sector_fund_flow_rank(
    ak: Any | None, trade_date: str, sector_type: str, indicator: str, limit: int, akshare_error: str = ""
) -> list[dict[str, Any]]:
    errors: list[str] = []
    if akshare_error:
        errors.append(f"AKShare load failed: {akshare_error}")
    if ak is not None:
        try:
            frame = ak.stock_sector_fund_flow_rank(indicator=indicator, sector_type=sector_type)
        except Exception as exc:
            errors.append(f"AKShare stock_sector_fund_flow_rank failed: {exc}")
        else:
            items = sector_fund_flow_frame_to_items(frame, trade_date, sector_type, indicator, limit)
            if items:
                return items
            errors.append("AKShare stock_sector_fund_flow_rank returned no rows")
    try:
        return fetch_eastmoney_sector_fund_flow_rank(trade_date, sector_type, indicator, limit)
    except Exception as exc:
        errors.append(f"Eastmoney sector fund flow fallback failed: {exc}")
        raise RuntimeError("; ".join(errors)) from exc


def sector_fund_flow_frame_to_items(
    frame: Any, trade_date: str, sector_type: str, indicator: str, limit: int
) -> list[dict[str, Any]]:
    items: list[dict[str, Any]] = []
    try:
        iterator = frame.iterrows()
    except Exception:
        return items
    for _, row in iterator:
        item = sector_fund_flow_item(row, trade_date, sector_type, indicator)
        if item is None:
            continue
        items.append(item)
        if limit > 0 and len(items) >= limit:
            break
    return items


def ths_sector_fund_flow_frame_to_items(
    frame: Any, trade_date: str, sector_type: str, indicator: str, limit: int
) -> list[dict[str, Any]]:
    rows: list[Any] = []
    try:
        iterator = frame.iterrows()
    except Exception:
        return []
    for _, row in iterator:
        rows.append(row)
    rows.sort(key=lambda row: parse_amount_to_yuan(first_existing(row, ["净额", "净额(亿)"]), "亿"), reverse=True)
    items: list[dict[str, Any]] = []
    for rank, row in enumerate(rows, start=1):
        name = text_value(first_existing(row, ["行业", "概念", "板块", "名称"]))
        if not name:
            continue
        fields: list[str] = []
        item: dict[str, Any] = {
            "trade_date": trade_date,
            "sector_type": sector_type,
            "indicator": indicator,
            "rank": rank,
            "name": name,
            "top_stock": text_value(first_existing(row, ["领涨股"])),
            "source_type": "ths",
            "raw_payload": json.dumps(json_safe_row(row), ensure_ascii=False, separators=(",", ":")),
            "fetched_at": utc_now_iso(),
        }
        set_numeric_field(item, row, fields, "change_pct", ["行业-涨跌幅", "涨跌幅"], parse_percent_value)
        set_numeric_field(item, row, fields, "main_net_inflow", ["净额", "净额(亿)"], lambda value: parse_amount_to_yuan(value, "亿"))
        item["field_counts_json"] = fund_flow_field_counts_json(fields)
        items.append(item)
        if limit > 0 and len(items) >= limit:
            break
    return items


def fetch_ths_sector_fund_flow_direct(sector_type: str, indicator: str) -> Any:
    try:
        import pandas as pd  # type: ignore
    except Exception as exc:
        raise RuntimeError("pandas is required for THS direct fallback") from exc
    from io import StringIO

    board = {"行业资金流": "hyzjl", "概念资金流": "gnzjl"}[sector_type]
    referer = f"https://data.10jqka.com.cn/funds/{board}/"
    headers = {
        "Accept": "text/html, */*; q=0.01",
        "Accept-Language": "zh-CN,zh;q=0.9,en;q=0.8",
        "Connection": "close",
        "Referer": referer,
        "User-Agent": EASTMONEY_HEADERS["User-Agent"],
        "X-Requested-With": "XMLHttpRequest",
    }
    if indicator == "今日":
        first_url = f"https://data.10jqka.com.cn/funds/{board}/field/tradezdf/order/desc/ajax/1/free/1/"
        page_url = f"https://data.10jqka.com.cn/funds/{board}/field/tradezdf/order/desc/page/{{}}/ajax/1/free/1/"
    else:
        board_days = indicator.replace("日", "")
        first_url = f"https://data.10jqka.com.cn/funds/{board}/board/{board_days}/field/tradezdf/order/desc/ajax/1/free/1/"
        page_url = f"https://data.10jqka.com.cn/funds/{board}/board/{board_days}/field/tradezdf/order/desc/page/{{}}/ajax/1/free/1/"
    frames: list[Any] = []
    for page in range(1, 20):
        url = first_url if page == 1 else page_url.format(page)
        request = urllib.request.Request(url, headers=headers)
        with urllib.request.urlopen(request, timeout=20) as response:
            text = response.read().decode("gbk", errors="replace")
        try:
            table = pd.read_html(StringIO(text))[0]
        except Exception:
            if page == 1:
                continue
            break
        if getattr(table, "empty", False):
            break
        frames.append(table)
        if len(table) < 50:
            break
    if not frames:
        raise RuntimeError("THS direct sector fallback returned no tables")
    return pd.concat(frames, ignore_index=True)


def fetch_ths_sector_fund_flow_rank(
    ak: Any, trade_date: str, sector_type: str, indicator: str, limit: int
) -> list[dict[str, Any]]:
    symbol = THS_FUND_FLOW_SYMBOLS[indicator]
    try:
        if sector_type == "概念资金流":
            frame = ak.stock_fund_flow_concept(symbol=symbol)
        else:
            frame = ak.stock_fund_flow_industry(symbol=symbol)
    except Exception:
        frame = fetch_ths_sector_fund_flow_direct(sector_type, indicator)
    return ths_sector_fund_flow_frame_to_items(frame, trade_date, sector_type, indicator, limit)


def sina_moneyflow_json(endpoint: str, params: dict[str, Any]) -> list[dict[str, Any]]:
    url = SINA_MONEYFLOW_BASE + endpoint + "?" + urllib.parse.urlencode(params)
    headers = dict(EASTMONEY_HEADERS)
    headers["Referer"] = "https://vip.stock.finance.sina.com.cn/moneyflow/"
    request = urllib.request.Request(url, headers=headers)
    with urllib.request.urlopen(request, timeout=20) as response:
        text = response.read().decode("utf-8", errors="replace").strip()
    if not text:
        return []
    if "=" in text and not text.lstrip().startswith("["):
        text = text.split("=", 1)[1].strip().rstrip(";")
    parsed = json.loads(text)
    return parsed if isinstance(parsed, list) else []


def fetch_sina_moneyflow_pages(endpoint: str, params: dict[str, Any], limit: int) -> list[dict[str, Any]]:
    rows: list[dict[str, Any]] = []
    page = 1
    page_size = 100 if limit <= 0 else min(max(limit, 1), 100)
    while page <= 20:
        page_params = dict(params)
        page_params["page"] = page
        page_params["num"] = page_size
        page_rows = sina_moneyflow_json(endpoint, page_params)
        if not page_rows:
            break
        rows.extend(page_rows)
        if limit > 0 and len(rows) >= limit:
            return rows[:limit]
        if len(page_rows) < page_size:
            break
        page += 1
    return rows


def sina_sector_fund_flow_rows_to_items(
    rows: list[dict[str, Any]], trade_date: str, sector_type: str, indicator: str, limit: int
) -> list[dict[str, Any]]:
    suffix = "" if indicator == "今日" else "_" + indicator.replace("日", "")
    ranked_rows = [row for row in rows if text_value(row.get("name"))]
    ranked_rows.sort(key=lambda row: finite_float(row.get("netamount" + suffix)), reverse=True)
    items: list[dict[str, Any]] = []
    for rank, row in enumerate(ranked_rows, start=1):
        fields: list[str] = []
        item: dict[str, Any] = {
            "trade_date": trade_date,
            "sector_type": sector_type,
            "indicator": indicator,
            "rank": rank,
            "name": text_value(row.get("name")),
            "top_stock": text_value(row.get("ts_name")) if indicator == "今日" else "",
            "source_type": "sina",
            "raw_payload": json.dumps(json_safe_row(row), ensure_ascii=False, separators=(",", ":")),
            "fetched_at": utc_now_iso(),
        }
        set_numeric_field(item, row, fields, "change_pct", ["avg_changeratio" + suffix], lambda value: parse_percent_value(value, True))
        set_numeric_field(item, row, fields, "main_net_inflow", ["netamount" + suffix], parse_amount_to_yuan)
        set_numeric_field(item, row, fields, "main_net_inflow_pct", ["ratioamount" + suffix], lambda value: parse_percent_value(value, True))
        item["field_counts_json"] = fund_flow_field_counts_json(fields)
        items.append(item)
        if limit > 0 and len(items) >= limit:
            break
    return items


def fetch_sina_sector_fund_flow_rank(trade_date: str, sector_type: str, indicator: str, limit: int) -> list[dict[str, Any]]:
    fenlei = "1" if sector_type == "概念资金流" else "0"
    if indicator == "今日":
        endpoint = "MoneyFlow.ssl_bkzj_bk"
        params = {"fenlei": fenlei, "sort": "netamount", "asc": "0"}
    else:
        suffix = indicator.replace("日", "")
        endpoint = "MoneyFlow.ssl_bkzjlxt"
        params = {"fenlei": fenlei, "sort": "netamount_" + suffix, "asc": "0"}
    rows = fetch_sina_moneyflow_pages(endpoint, params, limit)
    return sina_sector_fund_flow_rows_to_items(rows, trade_date, sector_type, indicator, limit)


def fetch_sector_fund_flow_source(
    ak: Any | None,
    trade_date: str,
    sector_type: str,
    indicator: str,
    limit: int,
    source: str,
    akshare_error: str = "",
) -> list[dict[str, Any]]:
    if source == "eastmoney":
        return fetch_sector_fund_flow_rank(ak, trade_date, sector_type, indicator, limit, akshare_error)
    if source == "ths":
        if ak is None:
            raise RuntimeError(f"AKShare load failed: {akshare_error or 'akshare unavailable'}")
        return fetch_ths_sector_fund_flow_rank(ak, trade_date, sector_type, indicator, limit)
    if source == "sina":
        return fetch_sina_sector_fund_flow_rank(trade_date, sector_type, indicator, limit)
    raise ValueError(f"unsupported sector fund flow source: {source}")


def eastmoney_stock_fund_flow_frame_to_items(frame: Any, trade_date: str, indicator: str, limit: int) -> list[dict[str, Any]]:
    items: list[dict[str, Any]] = []
    try:
        iterator = frame.iterrows()
    except Exception:
        return items
    for _, row in iterator:
        code = compact_stock_code(first_existing(row, ["代码", "股票代码", "code"]))
        name = text_value(first_existing(row, ["名称", "股票简称", "name"]))
        if not code or not is_sh_sz_code(code) or not has_resolved_stock_name(code, name):
            continue
        fields: list[str] = []
        item: dict[str, Any] = {
            "trade_date": trade_date,
            "indicator": indicator,
            "code": code,
            "rank": int(finite_float(first_existing(row, ["序号", "排名", "rank"]))),
            "name": name,
            "source_type": "eastmoney",
            "raw_payload": json.dumps(json_safe_row(row), ensure_ascii=False, separators=(",", ":")),
            "fetched_at": utc_now_iso(),
        }
        set_numeric_field(item, row, fields, "price", ["最新价", "price"])
        set_numeric_field(item, row, fields, "change_pct", [f"{indicator}涨跌幅", "今日涨跌幅", "涨跌幅"])
        set_numeric_field(item, row, fields, "main_net_inflow", [f"{indicator}主力净流入-净额", "今日主力净流入-净额", "主力净流入-净额"])
        set_numeric_field(item, row, fields, "main_net_inflow_pct", [f"{indicator}主力净流入-净占比", "今日主力净流入-净占比", "主力净流入-净占比"])
        set_numeric_field(item, row, fields, "super_large_net_inflow", [f"{indicator}超大单净流入-净额", "今日超大单净流入-净额", "超大单净流入-净额"])
        set_numeric_field(item, row, fields, "super_large_net_inflow_pct", [f"{indicator}超大单净流入-净占比", "今日超大单净流入-净占比", "超大单净流入-净占比"])
        set_numeric_field(item, row, fields, "large_net_inflow", [f"{indicator}大单净流入-净额", "今日大单净流入-净额", "大单净流入-净额"])
        set_numeric_field(item, row, fields, "large_net_inflow_pct", [f"{indicator}大单净流入-净占比", "今日大单净流入-净占比", "大单净流入-净占比"])
        set_numeric_field(item, row, fields, "medium_net_inflow", [f"{indicator}中单净流入-净额", "今日中单净流入-净额", "中单净流入-净额"])
        set_numeric_field(item, row, fields, "medium_net_inflow_pct", [f"{indicator}中单净流入-净占比", "今日中单净流入-净占比", "中单净流入-净占比"])
        set_numeric_field(item, row, fields, "small_net_inflow", [f"{indicator}小单净流入-净额", "今日小单净流入-净额", "小单净流入-净额"])
        set_numeric_field(item, row, fields, "small_net_inflow_pct", [f"{indicator}小单净流入-净占比", "今日小单净流入-净占比", "小单净流入-净占比"])
        item["field_counts_json"] = fund_flow_field_counts_json(fields)
        items.append(item)
        if limit > 0 and len(items) >= limit:
            break
    return items


def fetch_eastmoney_stock_fund_flow_rank(ak: Any, trade_date: str, indicator: str, limit: int) -> list[dict[str, Any]]:
    frame = ak.stock_individual_fund_flow_rank(indicator=indicator)
    return eastmoney_stock_fund_flow_frame_to_items(frame, trade_date, indicator, limit)


def ths_stock_fund_flow_frame_to_items(frame: Any, trade_date: str, indicator: str, limit: int) -> list[dict[str, Any]]:
    rows: list[Any] = []
    try:
        iterator = frame.iterrows()
    except Exception:
        return []
    for _, row in iterator:
        rows.append(row)
    rows.sort(key=lambda row: parse_amount_to_yuan(first_existing(row, ["净额", "净额(亿)"]), "亿"), reverse=True)
    items: list[dict[str, Any]] = []
    for rank, row in enumerate(rows, start=1):
        code = compact_stock_code(first_existing(row, ["股票代码", "代码", "code"]))
        name = text_value(first_existing(row, ["股票简称", "名称", "name"]))
        if not code or not is_sh_sz_code(code) or not has_resolved_stock_name(code, name):
            continue
        fields: list[str] = []
        item: dict[str, Any] = {
            "trade_date": trade_date,
            "indicator": indicator,
            "code": code,
            "rank": rank,
            "name": name,
            "source_type": "ths",
            "raw_payload": json.dumps(json_safe_row(row), ensure_ascii=False, separators=(",", ":")),
            "fetched_at": utc_now_iso(),
        }
        set_numeric_field(item, row, fields, "price", ["最新价", "当前价"])
        set_numeric_field(item, row, fields, "change_pct", ["涨跌幅", "阶段涨跌幅"], parse_percent_value)
        set_numeric_field(item, row, fields, "turnover_pct", ["换手率", "连续换手率"], parse_percent_value)
        set_numeric_field(item, row, fields, "in_amount", ["流入资金", "流入资金(亿)"], lambda value: parse_amount_to_yuan(value, "亿"))
        set_numeric_field(item, row, fields, "out_amount", ["流出资金", "流出资金(亿)"], lambda value: parse_amount_to_yuan(value, "亿"))
        set_numeric_field(item, row, fields, "main_net_inflow", ["净额", "净额(亿)", "资金流入净额"], lambda value: parse_amount_to_yuan(value, "亿"))
        set_numeric_field(item, row, fields, "amount", ["成交额", "成交额(亿)"], lambda value: parse_amount_to_yuan(value, "亿"))
        item["field_counts_json"] = fund_flow_field_counts_json(fields)
        items.append(item)
        if limit > 0 and len(items) >= limit:
            break
    return items


def fetch_ths_stock_fund_flow_rank(ak: Any, trade_date: str, indicator: str, limit: int) -> list[dict[str, Any]]:
    symbol = THS_FUND_FLOW_SYMBOLS[indicator]
    frame = ak.stock_fund_flow_individual(symbol=symbol)
    return ths_stock_fund_flow_frame_to_items(frame, trade_date, indicator, limit)


def sina_stock_fund_flow_rows_to_items(rows: list[dict[str, Any]], trade_date: str, indicator: str, limit: int) -> list[dict[str, Any]]:
    ranked_rows = [row for row in rows if compact_stock_code(row.get("symbol"))]
    ranked_rows.sort(key=lambda row: finite_float(row.get("netamount")), reverse=True)
    items: list[dict[str, Any]] = []
    for rank, row in enumerate(ranked_rows, start=1):
        code = compact_stock_code(row.get("symbol"))
        name = text_value(row.get("name"))
        if not code or not is_sh_sz_code(code) or not has_resolved_stock_name(code, name):
            continue
        fields: list[str] = []
        item: dict[str, Any] = {
            "trade_date": trade_date,
            "indicator": indicator,
            "code": code,
            "rank": rank,
            "name": name,
            "source_type": "sina",
            "raw_payload": json.dumps(json_safe_row(row), ensure_ascii=False, separators=(",", ":")),
            "fetched_at": utc_now_iso(),
        }
        set_numeric_field(item, row, fields, "price", ["trade"])
        set_numeric_field(item, row, fields, "change_pct", ["changeratio"], lambda value: parse_percent_value(value, True))
        set_numeric_field(item, row, fields, "turnover_pct", ["turnover"])
        set_numeric_field(item, row, fields, "amount", ["amount"], parse_amount_to_yuan)
        set_numeric_field(item, row, fields, "in_amount", ["inamount"], parse_amount_to_yuan)
        set_numeric_field(item, row, fields, "out_amount", ["outamount"], parse_amount_to_yuan)
        set_numeric_field(item, row, fields, "main_net_inflow", ["netamount"], parse_amount_to_yuan)
        set_numeric_field(item, row, fields, "main_net_inflow_pct", ["ratioamount"], lambda value: parse_percent_value(value, True))
        item["field_counts_json"] = fund_flow_field_counts_json(fields)
        items.append(item)
        if limit > 0 and len(items) >= limit:
            break
    return items


def fetch_sina_stock_fund_flow_rank(trade_date: str, indicator: str, limit: int) -> list[dict[str, Any]]:
    if indicator != "今日":
        return []
    rows = fetch_sina_moneyflow_pages(
        "MoneyFlow.ssl_bkzj_ssggzj",
        {"bankuai": "", "sort": "netamount", "asc": "0"},
        limit,
    )
    return sina_stock_fund_flow_rows_to_items(rows, trade_date, indicator, limit)


def fetch_stock_fund_flow_source(
    ak: Any | None,
    trade_date: str,
    indicator: str,
    limit: int,
    source: str,
    akshare_error: str = "",
) -> list[dict[str, Any]]:
    if source == "eastmoney":
        if ak is None:
            raise RuntimeError(f"AKShare load failed: {akshare_error or 'akshare unavailable'}")
        return fetch_eastmoney_stock_fund_flow_rank(ak, trade_date, indicator, limit)
    if source == "ths":
        if ak is None:
            raise RuntimeError(f"AKShare load failed: {akshare_error or 'akshare unavailable'}")
        return fetch_ths_stock_fund_flow_rank(ak, trade_date, indicator, limit)
    if source == "sina":
        return fetch_sina_stock_fund_flow_rank(trade_date, indicator, limit)
    raise ValueError(f"unsupported stock fund flow source: {source}")


def sector_constituent_frame_to_items(frame: Any, source: str, limit: int) -> list[dict[str, Any]]:
    items: list[dict[str, Any]] = []
    try:
        iterator = frame.iterrows()
    except Exception:
        return items
    for _, row in iterator:
        code = compact_stock_code(first_existing(row, ["代码", "股票代码", "code", "f12"]))
        name = text_value(first_existing(row, ["名称", "股票名称", "股票简称", "name", "f14"]))
        if not code or not is_sh_sz_code(code) or not has_resolved_stock_name(code, name):
            continue
        items.append(
            {
                "code": code,
                "name": name,
                "source": source,
                "updated_at": utc_now_iso(),
            }
        )
        if limit > 0 and len(items) >= limit:
            break
    return items


def fetch_sector_constituent_items(
    ak: Any, sector_type: str, sector_name: str, indicator: str, limit: int
) -> tuple[list[dict[str, Any]], list[str]]:
    errors: list[str] = []
    if sector_type == "概念资金流":
        fetchers = [
            ("eastmoney_concept_constituents", lambda: ak.stock_board_concept_cons_em(symbol=sector_name)),
        ]
    else:
        fetchers = [
            ("eastmoney_industry_constituents", lambda: ak.stock_board_industry_cons_em(symbol=sector_name)),
            ("eastmoney_industry_fund_flow_summary", lambda: ak.stock_sector_fund_flow_summary(symbol=sector_name, indicator=indicator)),
        ]
    for source, fetcher in fetchers:
        for attempt in range(3):
            try:
                frame = fetcher()
                items = sector_constituent_frame_to_items(frame, source, limit)
            except Exception as exc:  # pragma: no cover - external service variability
                errors.append(f"{source} attempt {attempt + 1}: {exc}")
                time.sleep(0.3 * (attempt + 1))
                continue
            if items:
                return items, errors
            errors.append(f"{source} returned no rows")
            break
    return [], errors


def average_fund_flow_items(items: list[dict[str, Any]], key_fields: list[str], numeric_fields: tuple[str, ...]) -> list[dict[str, Any]]:
    grouped: dict[str, dict[str, Any]] = {}
    for item in items:
        key = "\x1f".join(text_value(item.get(field)) for field in key_fields)
        if not key.strip("\x1f"):
            continue
        avg = grouped.setdefault(
            key,
            {
                "base": {field: item.get(field) for field in key_fields},
                "fields": {field: {"sum": 0.0, "count": 0} for field in numeric_fields},
                "counts": {},
                "sources": set(),
                "name": text_value(item.get("name")),
                "top_stock": "",
                "top_stock_net": None,
                "fetched_at": text_value(item.get("fetched_at")),
            },
        )
        source = text_value(item.get("source_type"))
        if source:
            avg["sources"].add(source)
        if not avg.get("name") and text_value(item.get("name")):
            avg["name"] = text_value(item.get("name"))
        if text_value(item.get("fetched_at")) > text_value(avg.get("fetched_at")):
            avg["fetched_at"] = text_value(item.get("fetched_at"))
        counts = parse_fund_flow_field_counts(item.get("field_counts_json"))
        if not counts:
            counts = {field: 1 for field in numeric_fields if field in item}
        for field in numeric_fields:
            if counts.get(field, 0) <= 0:
                continue
            avg["fields"][field]["sum"] += finite_float(item.get(field))
            avg["fields"][field]["count"] += 1
            avg["counts"][field] = int(avg["counts"].get(field, 0)) + 1
        top_stock = text_value(item.get("top_stock"))
        if top_stock:
            net = finite_float(item.get("main_net_inflow"))
            if avg["top_stock_net"] is None or net > float(avg["top_stock_net"]):
                avg["top_stock"] = top_stock
                avg["top_stock_net"] = net

    averaged: list[dict[str, Any]] = []
    for avg in grouped.values():
        out = dict(avg["base"])
        if avg.get("name"):
            out["name"] = avg["name"]
        for field in numeric_fields:
            field_avg = avg["fields"][field]
            out[field] = field_avg["sum"] / field_avg["count"] if field_avg["count"] else 0.0
        if avg.get("top_stock"):
            out["top_stock"] = avg["top_stock"]
        out["source_count"] = int(avg["counts"].get("main_net_inflow", 0))
        out["source_types"] = ",".join(sorted(avg["sources"]))
        out["source_type"] = "average"
        out["field_counts_json"] = fund_flow_field_counts_json([])
        if avg["counts"]:
            out["field_counts_json"] = json.dumps(avg["counts"], ensure_ascii=False, separators=(",", ":"))
        out["raw_payload"] = '{"aggregation":"average"}'
        out["fetched_at"] = text_value(avg.get("fetched_at")) or utc_now_iso()
        averaged.append(out)
    averaged.sort(key=lambda item: (-finite_float(item.get("main_net_inflow")), text_value(item.get("name")) or text_value(item.get("code"))))
    for rank, item in enumerate(averaged, start=1):
        item["rank"] = rank
    return averaged


def normalize_holding_period(value: str | None) -> str:
    raw = text_value(value)
    text = raw.upper().replace("-", "").replace("/", "").replace(".", "")
    if len(text) == 6 and text[4] == "Q" and text[:4].isdigit():
        quarter_end = {"1": "0331", "2": "0630", "3": "0930", "4": "1231"}.get(text[5])
        return text[:4] + quarter_end if quarter_end else ""
    if len(text) == 8 and text.isdigit():
        return text
    from_text = holding_report_period_from_text(raw)
    if from_text:
        return from_text
    parsed = normalize_optional_date(value)
    compact = parsed.replace("-", "") if parsed else ""
    return compact if len(compact) == 8 and compact.isdigit() else ""


def holding_report_period_from_text(value: str | None) -> str:
    text = text_value(value)
    if not text:
        return ""
    compact = text.upper().replace("-", "").replace("/", "").replace(".", "")
    match = re.search(r"(20\d{2})Q([1-4])", compact)
    if match:
        return match.group(1) + {"1": "0331", "2": "0630", "3": "0930", "4": "1231"}[match.group(2)]
    quarter_words = {
        "1": "0331",
        "一": "0331",
        "第一": "0331",
        "2": "0630",
        "二": "0630",
        "第二": "0630",
        "3": "0930",
        "三": "0930",
        "第三": "0930",
        "4": "1231",
        "四": "1231",
        "第四": "1231",
    }
    match = re.search(r"(20\d{2})\s*年?\s*(第?\s*[一二三四1234])\s*(季度|季报|季)", text)
    if match:
        key = match.group(2).replace("第", "").strip()
        quarter_end = quarter_words.get(key)
        return match.group(1) + quarter_end if quarter_end else ""
    return ""


def holding_period_to_sina_quarter(period: str) -> str:
    period = normalize_holding_period(period)
    if len(period) != 8:
        return ""
    quarter_map = {"0331": "1", "0630": "2", "0930": "3", "1231": "4"}
    quarter = quarter_map.get(period[4:])
    return period[:4] + quarter if quarter else ""


def normalize_holder_type(value: Any) -> str:
    text = text_value(value).lower()
    if not text:
        return "other"
    if "基金" in text or "fund" in text:
        return "fund"
    if "社保" in text or "social" in text:
        return "social_security"
    if "qfii" in text or "rqfii" in text:
        return "qfii"
    if "券商" in text or "证券" in text or "broker" in text:
        return "broker"
    if "保险" in text or "insurance" in text:
        return "insurance"
    if "信托" in text or "trust" in text:
        return "trust"
    if "银行" in text or "理财" in text:
        return "bank_wealth"
    if "个人" in text or "自然人" in text:
        return "natural_person"
    if "机构" in text or "公司" in text:
        return "institution"
    return "other"


def normalize_fund_company(value: Any, fallback: Any = "") -> str:
    text = text_value(value)
    if text:
        return text
    name = text_value(fallback)
    if "基金" not in name:
        return ""
    for suffix in ["基金管理有限公司", "管理有限公司", "股份有限公司", "有限责任公司"]:
        idx = name.find(suffix)
        if idx > 0:
            return name[: idx + len(suffix)].strip()
    idx = name.find("基金")
    if idx > 0:
        return name[: idx + len("基金")].strip()
    return ""


def holding_disclosure_scope(source_type: str) -> str:
    if source_type == "stock_gdfx_free_holding_detail_em":
        return "free_float_top10"
    if source_type == "stock_gdfx_holding_detail_em":
        return "top10"
    if source_type in {"stock_fund_stock_holder", "tushare_fund_portfolio"}:
        return "fund_quarterly"
    return "quarter_disclosure"


def compact_stock_code(value: Any) -> str:
    text = text_value(value)
    digits = "".join(ch for ch in text if ch.isdigit())
    if len(digits) >= 6:
        return digits[-6:]
    return text.zfill(6) if text else ""


def normalize_margin_market(value: str | None) -> str:
    text = text_value(value).lower()
    aliases = {
        "sh": "sse",
        "shanghai": "sse",
        "沪": "sse",
        "沪市": "sse",
        "上海": "sse",
        "sz": "szse",
        "shenzhen": "szse",
        "深": "szse",
        "深市": "szse",
        "深圳": "szse",
        "all": "all",
        "": "all",
    }
    return aliases.get(text, text if text in {"sse", "szse"} else "all")


def margin_market_label(market: str) -> str:
    return "沪市" if market == "sse" else "深市"


def set_margin_number(
    item: dict[str, Any], row: Any, target: str, names: list[str], default_unit: str = ""
) -> None:
    value = first_existing(row, names)
    item[target] = parse_unit_number(value, default_unit) if value is not None else None


def margin_summary_item(row: Any, market: str, fallback_date: str) -> dict[str, Any]:
    trade_date = normalize_optional_date(first_existing(row, ["信用交易日期", "交易日期", "date", "trade_date"])) or fallback_date
    money_unit = "亿" if market == "szse" else ""
    volume_unit = "亿" if market == "szse" else ""
    item: dict[str, Any] = {
        "trade_date": trade_date,
        "market": market,
        "market_label": margin_market_label(market),
        "source_type": f"akshare_stock_margin_{market}",
        "raw_payload": json.dumps(json_safe_row(row), ensure_ascii=False, separators=(",", ":")),
        "fetched_at": utc_now_iso(),
    }
    set_margin_number(item, row, "margin_buy_amount", ["融资买入额"], money_unit)
    set_margin_number(item, row, "margin_balance", ["融资余额"], money_unit)
    set_margin_number(item, row, "short_sell_volume", ["融券卖出量"], volume_unit)
    set_margin_number(item, row, "short_balance_volume", ["融券余量"], volume_unit)
    set_margin_number(item, row, "short_balance_amount", ["融券余额", "融券余量金额"], money_unit)
    set_margin_number(item, row, "margin_trading_balance", ["融资融券余额"], money_unit)
    return item


def margin_summary_frame_to_items(frame: Any, market: str, trade_date: str) -> list[dict[str, Any]]:
    items: list[dict[str, Any]] = []
    try:
        iterator = frame.iterrows()
    except Exception:
        return items
    for _, row in iterator:
        item = margin_summary_item(row, market, trade_date)
        if not item["trade_date"]:
            item["trade_date"] = trade_date
        items.append(item)
    return items


def margin_detail_item(row: Any, market: str, fallback_date: str, rank: int) -> dict[str, Any] | None:
    code = compact_stock_code(first_existing(row, ["标的证券代码", "证券代码", "code"]))
    name = text_value(first_existing(row, ["标的证券简称", "证券简称", "name"]))
    if not code:
        return None
    trade_date = normalize_optional_date(first_existing(row, ["信用交易日期", "交易日期", "date", "trade_date"])) or fallback_date
    item: dict[str, Any] = {
        "trade_date": trade_date,
        "market": market,
        "market_label": margin_market_label(market),
        "rank": rank,
        "code": code,
        "name": name,
        "source_type": f"akshare_stock_margin_detail_{market}",
        "raw_payload": json.dumps(json_safe_row(row), ensure_ascii=False, separators=(",", ":")),
        "fetched_at": utc_now_iso(),
    }
    set_margin_number(item, row, "margin_buy_amount", ["融资买入额"])
    set_margin_number(item, row, "margin_balance", ["融资余额"])
    set_margin_number(item, row, "margin_repay_amount", ["融资偿还额"])
    set_margin_number(item, row, "short_sell_volume", ["融券卖出量"])
    set_margin_number(item, row, "short_balance_volume", ["融券余量"])
    set_margin_number(item, row, "short_repay_volume", ["融券偿还量"])
    set_margin_number(item, row, "short_balance_amount", ["融券余额"])
    set_margin_number(item, row, "margin_trading_balance", ["融资融券余额"])
    return item


def margin_detail_frame_to_items(frame: Any, market: str, trade_date: str, limit: int = 0) -> list[dict[str, Any]]:
    items: list[dict[str, Any]] = []
    try:
        iterator = frame.iterrows()
    except Exception:
        return items
    for _, row in iterator:
        item = margin_detail_item(row, market, trade_date, len(items) + 1)
        if item is None:
            continue
        items.append(item)
        if limit > 0 and len(items) >= limit:
            break
    return items


def fetch_margin_trading_market(
    ak: Any, trade_date: str, market: str, limit: int = 0
) -> tuple[list[dict[str, Any]], list[dict[str, Any]]]:
    date_arg = akshare_date_arg(trade_date)
    if market == "sse":
        summary_frame = ak.stock_margin_sse(start_date=date_arg, end_date=date_arg)
        detail_frame = ak.stock_margin_detail_sse(date=date_arg)
    elif market == "szse":
        summary_frame = ak.stock_margin_szse(date=date_arg)
        detail_frame = ak.stock_margin_detail_szse(date=date_arg)
    else:
        raise ValueError(f"unsupported margin market: {market}")
    return (
        margin_summary_frame_to_items(summary_frame, market, trade_date),
        margin_detail_frame_to_items(detail_frame, market, trade_date, limit),
    )


def holding_row_item(row: Any, source_type: str, fallback_period: str, fallback_code: str = "", fallback_name: str = "") -> dict[str, Any] | None:
    code = compact_stock_code(first_existing(row, ["股票代码", "stock_code", "code"])) or compact_stock_code(fallback_code)
    name = text_value(first_existing(row, ["股票简称", "股票名称", "name", "stock_name"])) or fallback_name
    period = normalize_holding_period(first_existing(row, ["报告期", "截止日期", "END_DATE", "date"])) or normalize_holding_period(fallback_period)
    holder_name = text_value(first_existing(row, ["股东名称", "基金名称", "持股机构简称", "持股机构全称", "holder_name"]))
    holder_code = text_value(first_existing(row, ["基金代码", "持股机构代码", "holder_code"]))
    holder_type_raw = first_existing(row, ["股东类型", "持股机构类型", "holder_type"])
    holder_type = normalize_holder_type(holder_type_raw or holder_name)
    fund_code = text_value(first_existing(row, ["基金代码", "fund_code"])) if holder_type == "fund" else ""
    if holder_type == "fund" and not fund_code:
        fund_code = holder_code
    fund_company = normalize_fund_company(first_existing(row, ["基金公司", "基金管理人", "管理人", "fund_company"]), holder_name)
    if not code or not period or not holder_name:
        return None
    shares = finite_float(first_existing(row, ["期末持股-数量", "持仓数量", "持股数", "最新持股数", "shares"]))
    shares_change = finite_float(first_existing(row, ["期末持股-数量变化", "shares_change"]))
    change_ratio = finite_float(first_existing(row, ["期末持股-数量变化比例", "持股比例增幅", "change_ratio"]))
    float_ratio = finite_float(first_existing(row, ["期末持股-持股占流通股比", "占流通股比例", "最新占流通股比例", "float_ratio"]))
    market_value = finite_float(first_existing(row, ["期末持股-流通市值", "持股市值", "market_value"]))
    announce = normalize_optional_date(text_value(first_existing(row, ["公告日", "UPDATE_DATE", "NOTICE_DATE", "announce_date"])))
    rank = text_value(first_existing(row, ["股东排名", "序号", "rank"]))
    raw_payload = json.dumps(json_safe_row(row), ensure_ascii=False, separators=(",", ":"))
    return {
        "stock_code": code,
        "stock_name": name,
        "report_period": period,
        "announce_date": announce,
        "holder_name": holder_name,
        "holder_type": holder_type,
        "holder_code": holder_code,
        "holder_rank": rank,
        "fund_company": fund_company,
        "fund_code": fund_code,
        "disclosure_scope": holding_disclosure_scope(source_type),
        "shares": shares,
        "shares_change": shares_change,
        "change_ratio": change_ratio,
        "float_ratio": float_ratio,
        "market_value": market_value,
        "source_type": source_type,
        "source_url": "https://data.eastmoney.com/gdfx/HoldingAnalyse.html"
        if source_type.startswith("stock_gdfx_")
        else "https://vip.stock.finance.sina.com.cn/",
        "source_key": source_key([source_type, period, code, holder_name, holder_type, holder_code]),
        "raw_payload": raw_payload,
        "fetched_at": utc_now_iso(),
    }


def holding_frame_items(frame: Any, source_type: str, period: str, code: str = "", name: str = "") -> list[dict[str, Any]]:
    items: list[dict[str, Any]] = []
    try:
        iterator = frame.iterrows()
    except Exception:
        return items
    for _, row in iterator:
        item = holding_row_item(row, source_type, period, code, name)
        if item is not None:
            items.append(item)
    return items


def dedupe_holding_items(items: list[dict[str, Any]]) -> list[dict[str, Any]]:
    seen: set[tuple[str, str, str, str, str, str]] = set()
    out: list[dict[str, Any]] = []
    for item in items:
        key = (
            text_value(item.get("source_type")),
            text_value(item.get("report_period")),
            text_value(item.get("stock_code")),
            text_value(item.get("holder_name")),
            text_value(item.get("holder_type")),
            text_value(item.get("holder_code")),
        )
        if key in seen:
            continue
        seen.add(key)
        out.append(item)
    return out


def strip_html_tags(value: str) -> str:
    text = re.sub(r"<[^>]+>", "", value or "")
    return re.sub(r"\s+", " ", html.unescape(text)).strip()


def fund_name_from_report_title(title: str) -> str:
    title = re.sub(r"^[\[\【].*?[\]\】]\s*", "", title).strip()
    match = re.search(r"(20\d{2})", title)
    if match and match.start() > 0:
        return title[: match.start()].strip(" ：:,-_")
    for marker in ["年度报告", "中期报告", "季度报告", "季报"]:
        idx = title.find(marker)
        if idx > 0:
            return title[:idx].strip(" ：:,-_")
    return ""


def holding_report_document_item(title: str, href: str, fallback_period: str, source_type: str) -> dict[str, Any] | None:
    title = strip_html_tags(title)
    href = html.unescape(text_value(href))
    if not title or not href:
        return None
    if not any(keyword in title for keyword in ["季度报告", "季报", "第1季度", "第2季度", "第3季度", "第4季度", "第一季度", "第二季度", "第三季度", "第四季度"]):
        return None
    period = normalize_holding_period(title) or normalize_holding_period(fallback_period)
    if not period:
        return None
    if fallback_period and period != normalize_holding_period(fallback_period):
        return None
    source_url = urllib.parse.urljoin(TIANTIAN_FUND_REGULAR_REPORT_URL, href)
    fund_code_match = re.search(r"(?<!\d)(\d{6})(?!\d)", title)
    fund_code = fund_code_match.group(1) if fund_code_match else ""
    fund_name = fund_name_from_report_title(title)
    raw = {"title": title, "href": href}
    return {
        "source_type": source_type,
        "source_key": source_key([source_type, period, fund_code, fund_name, title, source_url]),
        "report_period": period,
        "fund_code": fund_code,
        "fund_name": fund_name,
        "fund_company": normalize_fund_company("", fund_name),
        "announcement_title": title,
        "announcement_date": normalize_optional_date(title),
        "source_url": source_url,
        "pdf_url": source_url if source_url.lower().split("?", 1)[0].endswith(".pdf") else "",
        "parse_status": "indexed",
        "raw_payload": json.dumps(raw, ensure_ascii=False, separators=(",", ":")),
        "fetched_at": utc_now_iso(),
    }


def parse_tiantian_fund_report_documents(html_text: str, period: str, limit: int) -> list[dict[str, Any]]:
    items: list[dict[str, Any]] = []
    for match in re.finditer(r"<a\b[^>]*href=[\"']([^\"']+)[\"'][^>]*>(.*?)</a>", html_text or "", re.IGNORECASE | re.DOTALL):
        item = holding_report_document_item(match.group(2), match.group(1), period, "tiantian_fund_regular_report")
        if item is None:
            continue
        items.append(item)
        if limit > 0 and len(items) >= limit:
            break
    seen: set[tuple[str, str]] = set()
    out: list[dict[str, Any]] = []
    for item in items:
        key = (text_value(item.get("source_type")), text_value(item.get("source_key")))
        if key in seen:
            continue
        seen.add(key)
        out.append(item)
    return out


def fetch_tiantian_fund_report_documents(period: str, limit: int) -> tuple[list[dict[str, Any]], str]:
    headers = dict(EASTMONEY_HEADERS)
    headers["Accept"] = "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"
    headers["Referer"] = "https://fund.eastmoney.com/"
    request = urllib.request.Request(TIANTIAN_FUND_REGULAR_REPORT_URL, headers=headers)
    try:
        with urllib.request.urlopen(request, timeout=20) as response:
            body = response.read()
    except Exception as exc:  # pragma: no cover - external service variability
        return [], f"tiantian_fund_regular_report {period}: {exc}"
    text = body.decode("utf-8", errors="replace")
    if "�" in text:
        text = body.decode("gbk", errors="replace")
    items = parse_tiantian_fund_report_documents(text, period, limit)
    if not items:
        return items, f"tiantian_fund_regular_report {period}: no report links parsed"
    return items, ""


def holding_worker_count(workers: int, task_count: int) -> int:
    if task_count <= 1:
        return 1
    return max(1, min(max(1, workers), DEFAULT_HOLDING_WORKERS, task_count))


def fetch_market_holding_free_detail(ak: Any, period: str) -> tuple[list[dict[str, Any]], list[str]]:
    try:
        frame = ak.stock_gdfx_free_holding_detail_em(date=period)
        return holding_frame_items(frame, "stock_gdfx_free_holding_detail_em", period), []
    except Exception as exc:  # pragma: no cover - external service variability
        return [], [f"stock_gdfx_free_holding_detail_em {period}: {exc}"]


def fetch_market_holding_detail(ak: Any, period: str, holder_type: str, change: str) -> tuple[list[dict[str, Any]], list[str]]:
    try:
        frame = ak.stock_gdfx_holding_detail_em(date=period, indicator=holder_type, symbol=change)
        return holding_frame_items(frame, "stock_gdfx_holding_detail_em", period), []
    except Exception as exc:  # pragma: no cover - external service variability
        return [], [f"stock_gdfx_holding_detail_em {period} {holder_type}/{change}: {exc}"]


def fetch_market_holdings_for_period(ak: Any, period: str, workers: int = DEFAULT_HOLDING_WORKERS) -> tuple[list[dict[str, Any]], list[str]]:
    items: list[dict[str, Any]] = []
    warnings: list[str] = []
    tasks = [lambda: fetch_market_holding_free_detail(ak, period)]
    for holder_type in HOLDING_DETAIL_TYPES:
        for change in HOLDING_DETAIL_CHANGES:
            tasks.append(lambda holder_type=holder_type, change=change: fetch_market_holding_detail(ak, period, holder_type, change))

    with concurrent.futures.ThreadPoolExecutor(max_workers=holding_worker_count(workers, len(tasks))) as pool:
        futures = [pool.submit(task) for task in tasks]
        for future in concurrent.futures.as_completed(futures):
            try:
                fetched_items, fetched_warnings = future.result()
            except Exception as exc:  # pragma: no cover - defensive isolation
                warnings.append(f"stock holding task {period}: {exc}")
                continue
            items.extend(fetched_items)
            warnings.extend(fetched_warnings)
    return dedupe_holding_items(items), warnings


def fetch_symbol_holdings_for_period(ak: Any, period: str, code: str) -> tuple[list[dict[str, Any]], list[str]]:
    items: list[dict[str, Any]] = []
    warnings: list[str] = []
    sina_quarter = holding_period_to_sina_quarter(period)
    if sina_quarter:
        try:
            frame = ak.stock_institute_hold_detail(stock=code, quarter=sina_quarter)
            items.extend(holding_frame_items(frame, "stock_institute_hold_detail", period, code))
        except Exception as exc:  # pragma: no cover - external service variability
            warnings.append(f"stock_institute_hold_detail {code} {sina_quarter}: {exc}")
    try:
        frame = ak.stock_fund_stock_holder(symbol=code)
        fund_items = holding_frame_items(frame, "stock_fund_stock_holder", period, code)
        normalized_period = normalize_holding_period(period)
        if normalized_period:
            fund_items = [item for item in fund_items if item.get("report_period") == normalized_period]
        items.extend(fund_items)
    except Exception as exc:  # pragma: no cover - external service variability
        warnings.append(f"stock_fund_stock_holder {code}: {exc}")
    return dedupe_holding_items(items), warnings


def dividend_event_row_item(row: Any, code: str, start: str, end: str) -> dict[str, Any] | None:
    data = row.to_dict() if hasattr(row, "to_dict") else dict(row)
    ex_date = normalize_optional_date(
        data.get("除权日")
        or data.get("除权除息日")
        or data.get("ex_date")
        or data.get("除权除息日期")
    )
    dividend_date = normalize_optional_date(
        data.get("派息日")
        or data.get("dividend_date")
        or data.get("派息日期")
    )
    if not optional_date_in_range(ex_date, start, end) and not optional_date_in_range(dividend_date, start, end):
        return None
    return {
        "code": code,
        "name": text_value(data.get("股票简称") or data.get("简称") or data.get("name")),
        "ex_date": ex_date,
        "dividend_date": dividend_date,
        "record_date": normalize_optional_date(data.get("股权登记日") or data.get("record_date")),
        "description": text_value(
            data.get("分红方案")
            or data.get("实施方案")
            or data.get("实施方案分红说明")
            or data.get("派现")
            or data.get("description")
        ),
        "source": "stock_dividend_cninfo",
    }


def fetch_dividend_events_for_symbol(ak: Any, code: str, start: str, end: str) -> tuple[list[dict[str, Any]], str]:
    try:
        frame = ak.stock_dividend_cninfo(symbol=code)
    except Exception as exc:  # pragma: no cover - external service variability
        return [], f"stock_dividend_cninfo {code}: {exc}"
    items: list[dict[str, Any]] = []
    if frame is None:
        return items, ""
    for _, row in frame.iterrows():
        item = dividend_event_row_item(row, code, start, end)
        if item:
            items.append(item)
    return items, ""


class AuctionService:
    def __init__(self, cache_dir: Path, workers: int, default_limit: int):
        self.cache_dir = cache_dir
        self.workers = max(1, workers)
        self.default_limit = max(0, default_limit)

    def health(self) -> dict[str, Any]:
        try:
            load_akshare()
            return {"status": "ok", "akshare": "ok"}
        except Exception as exc:
            return {"status": "degraded", "akshare": "missing", "error": str(exc)}

    def fetch_tdx_quote(self, query: dict[str, list[str]]) -> dict[str, Any]:
        started = time.time()
        codes = parse_tdx_quote_codes(first_query_value(query, "codes") or first_query_value(query, "code"))
        if not codes:
            return {"_http_status": 400, "items": [], "message": "codes required", "fetched_at": utc_now_iso()}
        cached, missing = self.read_tdx_quote_cache(codes)
        fetched: list[dict[str, Any]] = []
        warning = ""
        if missing:
            try:
                fetched = self.request_tdx_quotes(missing)
                self.write_tdx_quote_cache(fetched)
            except Exception as exc:
                warning = str(exc)
        items_by_code: dict[str, dict[str, Any]] = {}
        for item in cached + fetched:
            code = normalize_sh_sz_code(item.get("code"))
            if code:
                items_by_code[code] = item
        items = [items_by_code[code] for code in codes if code in items_by_code]
        payload: dict[str, Any] = {
            "items": items,
            "count": len(items),
            "requested": len(codes),
            "source": "tdx",
            "cache_ttl_sec": tdx_quote_cache_ttl_sec(),
            "elapsed_sec": round(time.time() - started, 3),
            "fetched_at": utc_now_iso(),
        }
        if warning:
            payload["warning"] = warning
        if not items:
            payload["_http_status"] = 502
            payload["message"] = warning or "tdx quote returned no usable data"
        return payload

    def read_tdx_quote_cache(self, codes: list[str]) -> tuple[list[dict[str, Any]], list[str]]:
        now = time.time()
        ttl = tdx_quote_cache_ttl_sec()
        cached: list[dict[str, Any]] = []
        missing: list[str] = []
        with _tdx_quote_cache_lock:
            for code in codes:
                entry = _tdx_quote_cache.get(code)
                if entry and now - float(entry.get("fetched_at_ts", 0.0)) <= ttl:
                    item = dict(entry["item"])
                    item["cached"] = True
                    cached.append(item)
                else:
                    missing.append(code)
        return cached, missing

    def write_tdx_quote_cache(self, items: list[dict[str, Any]]) -> None:
        now = time.time()
        with _tdx_quote_cache_lock:
            for item in items:
                code = normalize_sh_sz_code(item.get("code"))
                if not code or finite_float(item.get("price")) <= 0:
                    continue
                cached_item = dict(item)
                cached_item["cached"] = False
                _tdx_quote_cache[code] = {"fetched_at_ts": now, "item": cached_item}

    def request_tdx_quotes(self, codes: list[str]) -> list[dict[str, Any]]:
        api_cls = load_pytdx_hq_api()
        securities: list[tuple[int, str]] = []
        for code in codes:
            market = tdx_market_for_code(code)
            if market is not None:
                securities.append((market, code))
        if not securities:
            return []
        errors: list[str] = []
        for host, port in tdx_hq_servers():
            api = api_cls()
            connected = False
            try:
                try:
                    connected = bool(api.connect(host, port, time_out=2))
                except TypeError:
                    connected = bool(api.connect(host, port))
                if not connected:
                    errors.append(f"{host}:{port} connect failed")
                    continue
                rows = api.get_security_quotes(securities) or []
                items: list[dict[str, Any]] = []
                for row in rows:
                    item = tdx_quote_item_from_row(row_to_dict(row))
                    if item:
                        item["server"] = f"{host}:{port}"
                        items.append(item)
                if items:
                    return items
                errors.append(f"{host}:{port} returned no usable quote")
            except Exception as exc:
                errors.append(f"{host}:{port} {exc}")
            finally:
                if connected:
                    try:
                        api.disconnect()
                    except Exception:
                        pass
        raise RuntimeError("; ".join(errors) or "tdx quote servers returned no data")

    def fetch_trading_day(self, query: dict[str, list[str]]) -> dict[str, Any]:
        ak = load_akshare()
        return trading_day_status(ak, first_query_value(query, "date") or local_today())

    def fetch(self, query: dict[str, list[str]]) -> dict[str, Any]:
        requested_date = first_query_value(query, "date")
        trade_date = normalize_date(requested_date)
        code_value = first_query_value(query, "code") or ""
        explicit_codes = [item.strip().zfill(6) for item in code_value.split(",") if item.strip()]
        limit = int_value(first_query_value(query, "limit"), self.default_limit)
        force = first_query_value(query, "force") in {"1", "true", "yes"}

        if not force:
            cached = read_cache(self.cache_dir, trade_date)
            if cached is not None and payload_has_usable_items(cached):
                return cached

        ak = load_akshare()
        latest_date = latest_trading_day(ak, local_today())
        if not requested_date:
            trade_date = latest_date
            if not force:
                cached = read_cache(self.cache_dir, trade_date)
                if cached is not None and payload_has_usable_items(cached):
                    return cached
        if trade_date != latest_date:
            return {
                "_http_status": 422,
                "date": trade_date,
                "items": [],
                "message": (
                    "AKShare auction adapter only serves the latest trading day "
                    f"({latest_date}) without a usable local cache for the requested date."
                ),
                "fetched_at": utc_now_iso(),
            }

        started = time.time()
        warning = ""
        code_names, code_name_warning = load_stock_code_names(ak, 0)
        if explicit_codes:
            symbols = load_symbols(ak, explicit_codes, limit, code_names)
            with concurrent.futures.ThreadPoolExecutor(max_workers=self.workers) as pool:
                items = list(pool.map(lambda symbol: fetch_one_auction(ak, symbol, trade_date), symbols))
        else:
            try:
                items = fetch_market_snapshot(ak, trade_date, limit)
                if should_retry_full_market_snapshot_with_eastmoney(limit, items):
                    snapshot_count = len(items)
                    try:
                        eastmoney_items = fetch_eastmoney_snapshot(trade_date, limit)
                        if len(eastmoney_items) > len(items):
                            items = eastmoney_items
                            warning = (
                                f"stock_zh_a_spot_em returned only {snapshot_count} rows for full-market snapshot, "
                                "used direct Eastmoney snapshot."
                            )
                    except Exception as eastmoney_exc:
                        warning = (
                            f"stock_zh_a_spot_em returned only {snapshot_count} rows for full-market snapshot, "
                            f"and direct Eastmoney snapshot fallback failed: {eastmoney_exc}"
                        )
            except Exception as exc:
                try:
                    items = fetch_eastmoney_snapshot(trade_date, limit)
                    warning = f"stock_zh_a_spot_em failed, used direct Eastmoney snapshot: {exc}"
                except Exception as eastmoney_exc:
                    effective_limit = normalize_symbol_limit(limit)
                    try:
                        symbols = load_symbols(ak, [], effective_limit, code_names)
                        with concurrent.futures.ThreadPoolExecutor(max_workers=self.workers) as pool:
                            items = list(pool.map(lambda symbol: fetch_one_auction(ak, symbol, trade_date), symbols))
                        warning = (
                            "stock_zh_a_spot_em and direct Eastmoney snapshot failed, "
                            f"used pre-market fallback: snapshot={exc}; eastmoney={eastmoney_exc}"
                        )
                    except Exception as fallback_exc:
                        items = []
                        warning = (
                            "AKShare market snapshot, direct Eastmoney snapshot, and fallback symbol list all failed: "
                            f"snapshot={exc}; eastmoney={eastmoney_exc}; fallback={fallback_exc}"
                        )
            if items and not any(item_has_usable_amount(item) for item in items):
                try:
                    eastmoney_items = fetch_eastmoney_snapshot(trade_date, limit)
                    if any(item_has_usable_amount(item) for item in eastmoney_items):
                        previous_warning = (warning + "; ") if warning else ""
                        items = eastmoney_items
                        warning = previous_warning + "AKShare snapshot returned no usable amounts, used direct Eastmoney snapshot."
                except Exception as eastmoney_exc:
                    if warning:
                        warning += f"; direct Eastmoney snapshot also failed: {eastmoney_exc}"
                    else:
                        warning = f"AKShare snapshot returned no usable amounts and direct Eastmoney snapshot failed: {eastmoney_exc}"
        if code_name_warning:
            warning = ((warning + "; ") if warning else "") + f"code name universe fallback failed: {code_name_warning}"
        items = normalize_auction_snapshot_units(items)
        payload = {
            "date": trade_date,
            "items": items,
            "code_names": code_names,
            "code_name_count": len(code_names),
            "count": len(items),
            "ok": sum(1 for item in items if item.get("status") == "ok"),
            "elapsed_sec": round(time.time() - started, 3),
            "fetched_at": utc_now_iso(),
        }
        if warning:
            payload["warning"] = warning
        if payload_has_usable_items(payload):
            write_cache(self.cache_dir, trade_date, payload)
        elif items:
            payload["warning"] = (
                (warning + "; ") if warning else ""
            ) + "AKShare returned rows but no usable auction amounts; cache was not updated."
        return payload

    def fetch_evening_snapshot(self, query: dict[str, list[str]]) -> dict[str, Any]:
        requested_date = first_query_value(query, "date")
        trade_date = normalize_date(requested_date)
        limit = int_value(first_query_value(query, "limit"), self.default_limit)
        force = first_query_value(query, "force") in {"1", "true", "yes"}
        ak = load_akshare()
        latest_date = latest_trading_day(ak, local_today())
        if not requested_date:
            trade_date = latest_date
        if trade_date != latest_date and not force:
            return {
                "_http_status": 422,
                "date": trade_date,
                "items": [],
                "message": (
                    "AKShare evening snapshot only serves the latest trading day "
                    f"({latest_date}) unless force=1 is provided."
                ),
                "fetched_at": utc_now_iso(),
            }
        started = time.time()
        warning = ""
        try:
            items = fetch_evening_snapshot(ak, trade_date, limit)
            if not items:
                raise RuntimeError("stock_zh_a_spot_em returned no evening rows")
        except Exception as exc:
            try:
                items = fetch_eastmoney_evening_snapshot(trade_date, limit)
                warning = f"stock_zh_a_spot_em evening snapshot failed, used direct Eastmoney snapshot: {exc}"
            except Exception as eastmoney_exc:
                items = []
                warning = f"evening snapshot failed: akshare={exc}; eastmoney={eastmoney_exc}"
        payload: dict[str, Any] = {
            "date": trade_date,
            "items": items,
            "count": len(items),
            "ok": sum(1 for item in items if item.get("status") == "ok"),
            "elapsed_sec": round(time.time() - started, 3),
            "fetched_at": utc_now_iso(),
        }
        metric_warning = evening_snapshot_metric_warning(items)
        if metric_warning:
            warning = (warning + "; " if warning else "") + metric_warning
        if warning:
            payload["warning"] = warning
        if not items:
            payload["_http_status"] = 502
            payload["message"] = warning or "evening snapshot returned no usable rows"
        return payload

    def fetch_stock_research(self, query: dict[str, list[str]]) -> dict[str, Any]:
        ak = load_akshare()
        start = normalize_optional_date(first_query_value(query, "start"))
        end = normalize_optional_date(first_query_value(query, "end"))
        limit = int_value(first_query_value(query, "limit"), 0)
        symbols, warning = resolve_research_symbols(ak, query)
        if limit <= 0 and not (first_query_value(query, "code") or first_query_value(query, "company")):
            limit = 50
        started = time.time()
        items: list[dict[str, Any]] = []
        errors: list[str] = []
        for symbol in symbols:
            try:
                items.extend(fetch_research_reports_for_symbol(ak, symbol, start, end))
            except Exception as exc:  # pragma: no cover - external service variability
                errors.append(f"{symbol}: {exc}")
        items.sort(key=lambda item: (text_value(item.get("research_date")), text_value(item.get("source_key"))), reverse=True)
        if limit > 0:
            items = items[:limit]
        payload: dict[str, Any] = {
            "items": items,
            "count": len(items),
            "symbols": symbols,
            "start": start,
            "end": end,
            "elapsed_sec": round(time.time() - started, 3),
            "fetched_at": utc_now_iso(),
        }
        warnings = [item for item in [warning] if item]
        if errors:
            warnings.append("; ".join(errors))
        if warnings:
            payload["warning"] = "; ".join(warnings)
        if symbols and not items and errors:
            payload["_http_status"] = 502
        return payload

    def fetch_sector_fund_flow(self, query: dict[str, list[str]]) -> dict[str, Any]:
        ak = None
        akshare_error = ""
        try:
            ak = load_akshare()
        except Exception as exc:
            akshare_error = str(exc)
        trade_date = normalize_date(first_query_value(query, "date"))
        sector_type = normalize_sector_fund_flow_sector_type(first_query_value(query, "sector_type"))
        indicator = normalize_sector_fund_flow_indicator(first_query_value(query, "indicator"))
        source = normalize_fund_flow_source(first_query_value(query, "source") or first_query_value(query, "source_type"))
        limit = int_value(first_query_value(query, "limit"), 0)
        started = time.time()
        source_errors: list[str] = []
        source_items: list[dict[str, Any]] = []
        if source == "all":
            for source_name in FUND_FLOW_SOURCES:
                try:
                    fetched = fetch_sector_fund_flow_source(ak, trade_date, sector_type, indicator, limit, source_name, akshare_error)
                    source_items.extend(fetched)
                except Exception as exc:
                    source_errors.append(f"{source_name}: {exc}")
            items = average_fund_flow_items(source_items, ["trade_date", "sector_type", "indicator", "name"], FUND_FLOW_NUMERIC_FIELDS)
        else:
            try:
                items = fetch_sector_fund_flow_source(ak, trade_date, sector_type, indicator, limit, source, akshare_error)
                source_items = list(items)
            except Exception as exc:
                items = []
                source_errors.append(f"{source}: {exc}")
        payload: dict[str, Any] = {
            "items": items,
            "count": len(items),
            "date": trade_date,
            "sector_type": sector_type,
            "indicator": indicator,
            "source": source,
            "source_errors": source_errors,
            "elapsed_sec": round(time.time() - started, 3),
            "fetched_at": utc_now_iso(),
        }
        if source == "all":
            payload["source_items"] = source_items
            payload["source_count"] = len(set(text_value(item.get("source_type")) for item in source_items if text_value(item.get("source_type"))))
        if not items:
            payload["warning"] = "; ".join(source_errors) or "sector fund flow endpoint returned no rows"
            payload["_http_status"] = 502
        elif source_errors:
            payload["warning"] = "; ".join(source_errors)
        return payload

    def fetch_stock_fund_flow(self, query: dict[str, list[str]]) -> dict[str, Any]:
        ak = None
        akshare_error = ""
        try:
            ak = load_akshare()
        except Exception as exc:
            akshare_error = str(exc)
        trade_date = normalize_date(first_query_value(query, "date"))
        indicator = normalize_sector_fund_flow_indicator(first_query_value(query, "indicator"))
        source = normalize_fund_flow_source(first_query_value(query, "source") or first_query_value(query, "source_type"))
        limit = int_value(first_query_value(query, "limit"), 0)
        started = time.time()
        source_errors: list[str] = []
        source_items: list[dict[str, Any]] = []
        if source == "all":
            source_names = list(STOCK_FUND_FLOW_SOURCES)
            if indicator != "今日":
                source_names = [item for item in source_names if item != "sina"]
            for source_name in source_names:
                try:
                    fetched = fetch_stock_fund_flow_source(ak, trade_date, indicator, limit, source_name, akshare_error)
                    source_items.extend(fetched)
                except Exception as exc:
                    source_errors.append(f"{source_name}: {exc}")
            items = average_fund_flow_items(source_items, ["trade_date", "indicator", "code"], STOCK_FUND_FLOW_NUMERIC_FIELDS)
        else:
            try:
                items = fetch_stock_fund_flow_source(ak, trade_date, indicator, limit, source, akshare_error)
                source_items = list(items)
            except Exception as exc:
                items = []
                source_errors.append(f"{source}: {exc}")
        payload: dict[str, Any] = {
            "items": items,
            "count": len(items),
            "date": trade_date,
            "indicator": indicator,
            "source": source,
            "source_errors": source_errors,
            "elapsed_sec": round(time.time() - started, 3),
            "fetched_at": utc_now_iso(),
        }
        if source == "all":
            payload["source_items"] = source_items
            payload["source_count"] = len(set(text_value(item.get("source_type")) for item in source_items if text_value(item.get("source_type"))))
        if not items:
            payload["warning"] = "; ".join(source_errors) or "stock fund flow endpoint returned no rows"
            payload["_http_status"] = 502
        elif source_errors:
            payload["warning"] = "; ".join(source_errors)
        return payload

    def fetch_margin_trading(self, query: dict[str, list[str]]) -> dict[str, Any]:
        trade_date = normalize_date(first_query_value(query, "date"))
        market = normalize_margin_market(first_query_value(query, "market"))
        limit = int_value(first_query_value(query, "limit"), 0)
        started = time.time()
        summaries: list[dict[str, Any]] = []
        details: list[dict[str, Any]] = []
        source_errors: list[str] = []
        try:
            ak = load_akshare()
        except Exception as exc:
            ak = None
            source_errors.append(f"AKShare load failed: {exc}")
        markets = list(MARGIN_MARKETS) if market == "all" else [market]
        if ak is not None:
            for market_name in markets:
                try:
                    market_summaries, market_details = fetch_margin_trading_market(ak, trade_date, market_name, limit)
                    if market_summaries:
                        summaries.extend(market_summaries)
                    if market_details:
                        details.extend(market_details)
                    if not market_summaries and not market_details:
                        source_errors.append(f"{market_name}: returned no rows")
                except Exception as exc:
                    source_errors.append(f"{market_name}: {exc}")
        payload: dict[str, Any] = {
            "date": trade_date,
            "market": market,
            "summaries": summaries,
            "details": details,
            "summary_count": len(summaries),
            "detail_count": len(details),
            "source_errors": source_errors,
            "elapsed_sec": round(time.time() - started, 3),
            "fetched_at": utc_now_iso(),
        }
        if source_errors:
            payload["warning"] = "; ".join(source_errors)
        if not summaries and not details:
            payload["_http_status"] = 502
            if "warning" not in payload:
                payload["warning"] = "margin trading endpoint returned no rows"
        return payload

    def fetch_sector_constituents(self, query: dict[str, list[str]]) -> dict[str, Any]:
        sector_type = normalize_sector_fund_flow_sector_type(first_query_value(query, "sector_type"))
        sector_name = text_value(
            first_query_value(query, "sector_name")
            or first_query_value(query, "name")
            or first_query_value(query, "symbol")
        )
        indicator = normalize_sector_fund_flow_indicator(first_query_value(query, "indicator"))
        limit = int_value(first_query_value(query, "limit"), 0)
        if not sector_name:
            return {
                "_http_status": 400,
                "items": [],
                "count": 0,
                "sector_type": sector_type,
                "sector_name": sector_name,
                "indicator": indicator,
                "message": "sector_name is required",
                "fetched_at": utc_now_iso(),
            }
        started = time.time()
        try:
            ak = load_akshare()
            items, errors = fetch_sector_constituent_items(ak, sector_type, sector_name, indicator, limit)
        except Exception as exc:
            items = []
            errors = [str(exc)]
        payload: dict[str, Any] = {
            "items": items,
            "count": len(items),
            "sector_type": sector_type,
            "sector_name": sector_name,
            "indicator": indicator,
            "elapsed_sec": round(time.time() - started, 3),
            "fetched_at": utc_now_iso(),
        }
        if errors:
            payload["source_errors"] = errors
        if not items:
            payload["warning"] = "; ".join(errors) or "sector constituents endpoint returned no rows"
            payload["_http_status"] = 502
        elif errors:
            payload["warning"] = "; ".join(errors)
        return payload

    def fetch_holdings(self, query: dict[str, list[str]]) -> dict[str, Any]:
        ak = load_akshare()
        period = normalize_holding_period(first_query_value(query, "period"))
        if not period:
            return {
                "_http_status": 400,
                "items": [],
                "message": "period is required, e.g. 20260331",
                "fetched_at": utc_now_iso(),
            }
        code = compact_stock_code(first_query_value(query, "code") or first_query_value(query, "symbol") or "")
        limit = int_value(first_query_value(query, "limit"), 0)
        report_limit = min(max(int_value(first_query_value(query, "report_limit"), 200), 0), 1000)
        started = time.time()
        if code:
            items, warnings = fetch_symbol_holdings_for_period(ak, period, code)
        else:
            items, warnings = fetch_market_holdings_for_period(ak, period, self.workers)
        reports, report_warning = fetch_tiantian_fund_report_documents(period, report_limit)
        if report_warning:
            warnings.append(report_warning)
        if limit > 0:
            items = items[:limit]
        payload: dict[str, Any] = {
            "items": items,
            "reports": reports,
            "count": len(items),
            "report_count": len(reports),
            "period": period,
            "code": code,
            "elapsed_sec": round(time.time() - started, 3),
            "fetched_at": utc_now_iso(),
        }
        if warnings:
            payload["warning"] = "; ".join(warnings)
        if not items and warnings:
            payload["_http_status"] = 502
        return payload

    def fetch_holding_reports(self, query: dict[str, list[str]]) -> dict[str, Any]:
        period = normalize_holding_period(first_query_value(query, "period"))
        if not period:
            return {
                "_http_status": 400,
                "items": [],
                "message": "period is required, e.g. 20260331",
                "fetched_at": utc_now_iso(),
            }
        limit = min(max(int_value(first_query_value(query, "limit"), 200), 0), 1000)
        started = time.time()
        items, warning = fetch_tiantian_fund_report_documents(period, limit)
        payload: dict[str, Any] = {
            "items": items,
            "reports": items,
            "count": len(items),
            "period": period,
            "elapsed_sec": round(time.time() - started, 3),
            "fetched_at": utc_now_iso(),
        }
        if warning:
            payload["warning"] = warning
        if not items and warning:
            payload["_http_status"] = 502
        return payload

    def fetch_dividend_events(self, query: dict[str, list[str]]) -> dict[str, Any]:
        started = time.time()
        date = normalize_date(first_query_value(query, "date"))
        window_days = max(0, int_value(first_query_value(query, "window_days"), 3))
        start = normalize_optional_date(first_query_value(query, "start")) or date_add(date, -window_days)
        end = normalize_optional_date(first_query_value(query, "end")) or date_add(date, window_days)
        if start > end:
            start, end = end, start
        codes = parse_tdx_quote_codes(first_query_value(query, "codes") or first_query_value(query, "code"))
        if not codes:
            return {"_http_status": 400, "items": [], "message": "codes required", "fetched_at": utc_now_iso()}
        ak = load_akshare()
        items: list[dict[str, Any]] = []
        warnings: list[str] = []
        for code in codes:
            code_items, warning = fetch_dividend_events_for_symbol(ak, code, start, end)
            items.extend(code_items)
            if warning:
                warnings.append(warning)
        payload: dict[str, Any] = {
            "items": items,
            "count": len(items),
            "date": date,
            "start": start,
            "end": end,
            "elapsed_sec": round(time.time() - started, 3),
            "fetched_at": utc_now_iso(),
        }
        if warnings:
            payload["warning"] = "; ".join(warnings)
        return payload


def first_query_value(query: dict[str, list[str]], key: str) -> str | None:
    values = query.get(key)
    if not values:
        return None
    return values[0]


def int_value(value: str | None, default: int) -> int:
    if value is None or value == "":
        return default
    try:
        return int(value)
    except ValueError:
        return default


class RequestHandler(BaseHTTPRequestHandler):
    service: AuctionService

    def log_message(self, fmt: str, *args: Any) -> None:
        sys.stdout.write("%s %s\n" % (dt.datetime.now().isoformat(timespec="seconds"), fmt % args))
        sys.stdout.flush()

    def do_GET(self) -> None:  # noqa: N802
        parsed = urllib.parse.urlparse(self.path)
        query = urllib.parse.parse_qs(parsed.query)
        try:
            if parsed.path in {"/", "/healthz"}:
                self.write_json(200, self.service.health())
                return
            if parsed.path == "/api/a-stock/auction":
                payload = self.service.fetch(query)
                status = int(payload.get("_http_status", 200))
                if "_http_status" in payload:
                    payload = dict(payload)
                    payload.pop("_http_status", None)
                self.write_json(status, payload)
                return
            if parsed.path == "/api/a-stock/evening-snapshot":
                payload = self.service.fetch_evening_snapshot(query)
                status = int(payload.get("_http_status", 200))
                if "_http_status" in payload:
                    payload = dict(payload)
                    payload.pop("_http_status", None)
                self.write_json(status, payload)
                return
            if parsed.path == "/api/a-stock/trading-day":
                payload = self.service.fetch_trading_day(query)
                status = int(payload.get("_http_status", 200))
                if "_http_status" in payload:
                    payload = dict(payload)
                    payload.pop("_http_status", None)
                self.write_json(status, payload)
                return
            if parsed.path == "/api/a-stock/tdx-quote":
                payload = self.service.fetch_tdx_quote(query)
                status = int(payload.get("_http_status", 200))
                if "_http_status" in payload:
                    payload = dict(payload)
                    payload.pop("_http_status", None)
                self.write_json(status, payload)
                return
            if parsed.path == "/api/a-stock/dividend-events":
                payload = self.service.fetch_dividend_events(query)
                status = int(payload.get("_http_status", 200))
                if "_http_status" in payload:
                    payload = dict(payload)
                    payload.pop("_http_status", None)
                self.write_json(status, payload)
                return
            if parsed.path == "/api/a-stock/holdings":
                payload = self.service.fetch_holdings(query)
                status = int(payload.get("_http_status", 200))
                if "_http_status" in payload:
                    payload = dict(payload)
                    payload.pop("_http_status", None)
                self.write_json(status, payload)
                return
            if parsed.path == "/api/a-stock/holding-reports":
                payload = self.service.fetch_holding_reports(query)
                status = int(payload.get("_http_status", 200))
                if "_http_status" in payload:
                    payload = dict(payload)
                    payload.pop("_http_status", None)
                self.write_json(status, payload)
                return
            if parsed.path == "/api/a-stock/sector-fund-flow":
                payload = self.service.fetch_sector_fund_flow(query)
                status = int(payload.get("_http_status", 200))
                if "_http_status" in payload:
                    payload = dict(payload)
                    payload.pop("_http_status", None)
                self.write_json(status, payload)
                return
            if parsed.path == "/api/a-stock/stock-fund-flow":
                payload = self.service.fetch_stock_fund_flow(query)
                status = int(payload.get("_http_status", 200))
                if "_http_status" in payload:
                    payload = dict(payload)
                    payload.pop("_http_status", None)
                self.write_json(status, payload)
                return
            if parsed.path == "/api/a-stock/margin-trading":
                payload = self.service.fetch_margin_trading(query)
                status = int(payload.get("_http_status", 200))
                if "_http_status" in payload:
                    payload = dict(payload)
                    payload.pop("_http_status", None)
                self.write_json(status, payload)
                return
            if parsed.path == "/api/a-stock/sector-constituents":
                payload = self.service.fetch_sector_constituents(query)
                status = int(payload.get("_http_status", 200))
                if "_http_status" in payload:
                    payload = dict(payload)
                    payload.pop("_http_status", None)
                self.write_json(status, payload)
                return
            if parsed.path == "/api/stock-research":
                payload = self.service.fetch_stock_research(query)
                status = int(payload.get("_http_status", 200))
                if "_http_status" in payload:
                    payload = dict(payload)
                    payload.pop("_http_status", None)
                self.write_json(status, payload)
                return
            self.write_json(404, {"error": "not found"})
        except Exception as exc:
            self.write_json(
                500,
                {
                    "error": str(exc),
                    "trace": traceback.format_exc(limit=8),
                },
            )

    def write_json(self, status: int, payload: dict[str, Any]) -> None:
        body = json.dumps(payload, ensure_ascii=False).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="AKShare A-stock auction HTTP adapter")
    parser.add_argument("--host", default=os.getenv("AKSHARE_AUCTION_HOST", DEFAULT_HOST))
    parser.add_argument("--port", type=int, default=int(os.getenv("AKSHARE_AUCTION_PORT", DEFAULT_PORT)))
    parser.add_argument(
        "--workers",
        type=int,
        default=int(os.getenv("AKSHARE_AUCTION_WORKERS", DEFAULT_WORKERS)),
    )
    parser.add_argument(
        "--limit",
        type=int,
        default=int(os.getenv("AKSHARE_AUCTION_LIMIT", "0")),
        help="optional max stock count; 0 means full market",
    )
    parser.add_argument(
        "--cache-dir",
        default=os.getenv("AKSHARE_AUCTION_CACHE_DIR", str(DEFAULT_CACHE_DIR)),
    )
    parser.add_argument("--self-test", action="store_true")
    return parser


def run_self_test() -> None:
    assert normalize_date("20260617") == "2026-06-17"
    assert finite_float("12.3") == 12.3
    assert finite_float("nan") == 0.0
    assert payload_has_usable_items({"items": [{"code": "000001", "name": "平安银行", "status": "ok", "auction_amount": 1}]})
    assert not payload_has_usable_items({"items": [{"code": "000001", "name": "平安银行", "status": "no_auction_amount", "auction_amount": 0}]})
    assert normalize_symbol_limit(0) == 0
    assert normalize_symbol_limit(6000) == 6000
    assert normalize_symbol_limit(-1) == 0
    assert should_retry_full_market_snapshot_with_eastmoney(0, [{"code": "000001"}] * 100)
    assert not should_retry_full_market_snapshot_with_eastmoney(100, [{"code": "000001"}] * 100)
    assert not should_retry_full_market_snapshot_with_eastmoney(0, [{"code": "000001"}] * 1200)
    assert eastmoney_page_size(0) == 100
    assert eastmoney_page_size(50) == 50
    assert eastmoney_page_size(200) == 100
    assert eastmoney_target_row_count(5534, 0) == 5534
    assert eastmoney_target_row_count(5534, 300) == 300
    assert eastmoney_target_row_count(0, 300) == 300
    assert is_sh_sz_code("000001")
    assert is_sh_sz_code("301696")
    assert not is_sh_sz_code("012322")
    assert not is_sh_sz_code("011631")
    assert not is_sh_sz_code("920118")
    assert tdx_market_for_code("000001") == 0
    assert tdx_market_for_code("600000") == 1
    assert parse_tdx_quote_codes("000001,sh600000,920118") == ["000001", "600000"]
    tdx_item = tdx_quote_item_from_row({"code": "000977", "price": 93.67, "last_close": 86.0, "open": 92.0})
    assert tdx_item is not None
    assert tdx_item["code"] == "000977"
    assert tdx_item["price"] == 93.67
    assert round(tdx_item["pct"], 4) == 8.9186
    assert has_resolved_stock_name("301696", "测试股份")
    assert not has_resolved_stock_name("301696", "301696")
    assert not has_resolved_stock_name("000034", "金十数据整理")
    class FakeCodeNameFrame:
        def iterrows(self) -> Any:
            return iter(
                [
                    (0, {"代码": "000034", "名称": "神州数码"}),
                    (1, {"代码": "601995", "名称": "中金公司"}),
                    (2, {"代码": "301696", "名称": "金十数据整理"}),
                ]
            )

    code_name_items = stock_code_name_items_from_frame(FakeCodeNameFrame(), "akshare_code_name")
    assert [item["code"] for item in code_name_items] == ["000034", "601995"]
    assert code_name_items[0]["name"] == "神州数码"
    eastmoney_items = eastmoney_rows_to_items(
        [
            {"f12": "1", "f14": "平安银行", "f2": "12.3", "f5": "1000", "f6": "12300"},
            {"f12": "000001", "f14": "平安银行", "f2": "12.3", "f5": "1000", "f6": "12300"},
            {"f12": "301696", "f14": "301696", "f2": "11.1", "f5": "1000", "f6": "11100"},
        ],
        "2026-06-18",
        0,
    )
    assert len(eastmoney_items) == 1
    assert eastmoney_items[0]["code"] == "000001"
    assert eastmoney_items[0]["name"] == "平安银行"
    assert eastmoney_items[0]["source"] == "eastmoney_clist"
    assert eastmoney_items[0]["status"] == "ok"
    evening_items = eastmoney_rows_to_evening_items(
        [
            {"f12": "000001", "f14": "平安银行", "f2": "12.30", "f3": "4.50", "f6": "300000000", "f8": "3.20", "f10": "4.10", "f22": "0.50"},
            {"f12": "920118", "f14": "太湖远大", "f2": "18", "f3": "4", "f6": "300000000", "f8": "3", "f10": "4", "f22": "1"},
        ],
        "2026-07-22",
        0,
    )
    assert len(evening_items) == 1
    assert evening_items[0]["code"] == "000001"
    assert evening_items[0]["source"] == "eastmoney_clist"
    assert evening_items[0]["price"] == 12.3
    assert evening_items[0]["change_pct"] == 4.5
    assert evening_items[0]["volume_ratio"] == 4.1
    assert evening_items[0]["turnover_pct"] == 3.2
    assert evening_items[0]["amount"] == 300000000
    assert evening_items[0]["speed"] == 0.5
    assert not evening_snapshot_metric_warning(evening_items)

    class FakeEveningSpotFrame:
        def iterrows(self) -> Any:
            return iter(
                [
                    (
                        0,
                        {
                            "代码": "600000",
                            "名称": "浦发银行",
                            "最新价": "8.88",
                            "涨跌幅": "5.20",
                            "量比": "4.30",
                            "换手率": "3.70%",
                            "成交额": "450000000",
                            "涨速": "0.60%",
                        },
                    )
                ]
            )

    class FakeEveningAK:
        def stock_zh_a_spot_em(self) -> Any:
            return FakeEveningSpotFrame()

    ak_evening_items = fetch_evening_snapshot(FakeEveningAK(), "2026-07-22", 0)
    assert len(ak_evening_items) == 1
    assert ak_evening_items[0]["code"] == "600000"
    assert ak_evening_items[0]["source"] == "akshare_spot_em"
    assert ak_evening_items[0]["price"] == 8.88
    assert ak_evening_items[0]["change_pct"] == 5.2
    assert ak_evening_items[0]["volume_ratio"] == 4.3
    assert ak_evening_items[0]["turnover_pct"] == 3.7
    assert ak_evening_items[0]["amount"] == 450000000
    assert ak_evening_items[0]["speed"] == 0.6
    missing_metric_warning = evening_snapshot_metric_warning(
        [
            {
                "code": "000001",
                "name": "平安银行",
                "price": 12.3,
                "change_pct": 4.5,
                "turnover_pct": 3.2,
                "amount": 300000000,
                "volume_ratio": 0,
                "speed": 0,
            }
        ]
    )
    assert "volume_ratio" in missing_metric_warning and "speed" in missing_metric_warning
    oversized_items = [
        {
            "code": f"30{i:04d}",
            "name": f"测试股份{i}",
            "auction_price": 10,
            "auction_volume": 1_400_000,
            "auction_amount": 3_500_000_000,
            "source": "eastmoney_clist",
            "status": "ok",
        }
        for i in range(1001)
    ]
    normalized_oversized = normalize_auction_snapshot_units(oversized_items)
    assert normalized_oversized[0]["auction_volume"] == 14_000
    assert normalized_oversized[0]["auction_amount"] == 35_000_000
    assert eastmoney_items[0]["auction_volume"] == 1000
    assert not eastmoney_rows_to_items(
        [{"f12": "920118", "f14": "太湖远大", "f2": "18", "f5": "1000", "f6": "18000"}],
        "2026-06-18",
        0,
    )
    assert normalize_fund_flow_source("同花顺") == "ths"
    assert parse_amount_to_yuan("1.23亿") == 123000000
    assert parse_amount_to_yuan("456万") == 4560000
    assert parse_amount_to_yuan("1.5", "亿") == 150000000
    assert round(parse_percent_value("0.0123", True), 4) == 1.23
    sector_items = eastmoney_sector_fund_flow_rows_to_items(
        [
            {"f14": "航空机场", "f3": -0.6, "f62": -100, "f184": -1.2, "f66": -60, "f69": -0.7, "f72": -40, "f75": -0.5, "f78": 20, "f81": 0.2, "f84": 80, "f87": 1.0, "f204": "春秋航空"},
            {"f14": "风力发电", "f3": 2.6, "f62": 500, "f184": 4.8, "f66": 300, "f69": 2.5, "f72": 200, "f75": 2.3, "f78": -100, "f81": -0.8, "f84": -400, "f87": -4.0, "f204": "N华润"},
        ],
        "2026-07-02",
        "行业资金流",
        "今日",
        1,
    )
    assert len(sector_items) == 1
    assert sector_items[0]["rank"] == 1
    assert sector_items[0]["name"] == "风力发电"
    assert sector_items[0]["main_net_inflow"] == 500
    assert sector_items[0]["main_net_inflow_pct"] == 4.8
    assert sector_items[0]["top_stock"] == "N华润"
    assert sector_items[0]["source_type"] == "eastmoney"
    ths_sector = ths_sector_fund_flow_frame_to_items(
        type(
            "FakeTHSFrame",
            (),
            {"iterrows": lambda self: iter([(0, {"序号": 1, "行业": "电机", "行业-涨跌幅": "3.30", "净额": "4.00", "领涨股": "大洋电机"})])},
        )(),
        "2026-07-02",
        "行业资金流",
        "今日",
        0,
    )
    assert ths_sector[0]["main_net_inflow"] == 400000000
    assert parse_fund_flow_field_counts(ths_sector[0]["field_counts_json"])["main_net_inflow"] == 1
    sina_sector = sina_sector_fund_flow_rows_to_items(
        [{"name": "机械行业", "avg_changeratio": "0.00706961", "netamount": "3976086740.8500", "ratioamount": "0.106499", "ts_name": "宝塔实业"}],
        "2026-07-02",
        "行业资金流",
        "今日",
        0,
    )
    assert round(sina_sector[0]["change_pct"], 4) == 0.707
    assert round(sina_sector[0]["main_net_inflow_pct"], 4) == 10.6499
    averaged_sector = average_fund_flow_items(
        [
            {"trade_date": "2026-07-02", "sector_type": "行业资金流", "indicator": "今日", "name": "电机", "main_net_inflow": 100, "large_net_inflow": 50, "source_type": "eastmoney", "field_counts_json": '{"main_net_inflow":1,"large_net_inflow":1}'},
            {"trade_date": "2026-07-02", "sector_type": "行业资金流", "indicator": "今日", "name": "电机", "main_net_inflow": 200, "large_net_inflow": 0, "source_type": "ths", "field_counts_json": '{"main_net_inflow":1}'},
        ],
        ["trade_date", "sector_type", "indicator", "name"],
        FUND_FLOW_NUMERIC_FIELDS,
    )
    assert averaged_sector[0]["main_net_inflow"] == 150
    assert averaged_sector[0]["large_net_inflow"] == 50
    assert averaged_sector[0]["source_count"] == 2
    stock_items = eastmoney_stock_fund_flow_frame_to_items(
        type(
            "FakeStockFrame",
            (),
            {"iterrows": lambda self: iter([(0, {"序号": 1, "代码": "300502", "名称": "新易盛", "最新价": "529.04", "今日涨跌幅": "3.94", "今日主力净流入-净额": "810968272", "今日主力净流入-净占比": "8.95"})])},
        )(),
        "2026-07-02",
        "今日",
        0,
    )
    assert stock_items[0]["code"] == "300502"
    assert stock_items[0]["source_type"] == "eastmoney"
    ths_stock_rows = [
        (
            0,
            {
                "序号": 1408,
                "股票代码": "002520",
                "股票简称": "日发精机",
                "最新价": "7.02",
                "阶段涨跌幅": "-0.43%",
                "连续换手率": "71.47%",
                "资金流入净额": "-3.14亿",
            },
        )
    ]
    ths_stock = ths_stock_fund_flow_frame_to_items(
        type(
            "FakeTHSStockFrame",
            (),
            {"iterrows": lambda self: iter(ths_stock_rows)},
        )(),
        "2026-07-08",
        "5日",
        0,
    )
    assert ths_stock[0]["code"] == "002520"
    assert ths_stock[0]["main_net_inflow"] == -314000000
    assert round(ths_stock[0]["change_pct"], 4) == -0.43
    assert round(ths_stock[0]["turnover_pct"], 4) == 71.47
    ths_stock_counts = parse_fund_flow_field_counts(ths_stock[0]["field_counts_json"])
    assert ths_stock_counts["main_net_inflow"] == 1
    assert ths_stock_counts["change_pct"] == 1
    assert ths_stock_counts["turnover_pct"] == 1
    sector_constituents = sector_constituent_frame_to_items(
        type(
            "FakeSectorConstituentFrame",
            (),
            {
                "iterrows": lambda self: iter(
                    [
                        (0, {"代码": "600760", "名称": "中航沈飞"}),
                        (1, {"代码": "012322", "名称": "012322"}),
                        (2, {"代码": "920118", "名称": "太湖远大"}),
                    ]
                )
            },
        )(),
        "eastmoney_industry_constituents",
        0,
    )
    assert [item["code"] for item in sector_constituents] == ["600760"]
    assert sector_constituents[0]["name"] == "中航沈飞"
    sina_stock = sina_stock_fund_flow_rows_to_items(
        [{"symbol": "sz300308", "name": "中际旭创", "trade": "123.45", "changeratio": "0.00613", "turnover": "12.7552", "amount": "1000", "inamount": "700", "outamount": "300", "netamount": "400", "ratioamount": "0.93803"}],
        "2026-07-02",
        "今日",
        0,
    )
    assert sina_stock[0]["code"] == "300308"
    assert round(sina_stock[0]["change_pct"], 4) == 0.613
    assert round(sina_stock[0]["main_net_inflow_pct"], 4) == 93.803
    assert normalize_margin_market("沪市") == "sse"
    assert normalize_margin_market("深市") == "szse"
    assert parse_unit_number("1.23亿", "亿") == 123000000

    class FakeMarginFrame:
        def __init__(self, rows: list[dict[str, Any]]) -> None:
            self.rows = rows

        def iterrows(self) -> Any:
            return iter(enumerate(self.rows))

    class FakeMarginAK:
        def stock_margin_sse(self, start_date: str, end_date: str) -> Any:
            assert start_date == "20260702" and end_date == "20260702"
            return FakeMarginFrame(
                [
                    {
                        "信用交易日期": "20260702",
                        "融资余额": "790606930947",
                        "融资买入额": "31868581207",
                        "融券余量": "5024353370",
                        "融券余量金额": "26428195175",
                        "融券卖出量": "44179298",
                        "融资融券余额": "817035126122",
                    }
                ]
            )

        def stock_margin_detail_sse(self, date: str) -> Any:
            assert date == "20260702"
            return FakeMarginFrame(
                [
                    {
                        "信用交易日期": "20260702",
                        "标的证券代码": "510050",
                        "标的证券简称": "50ETF",
                        "融资余额": "4756033617",
                        "融资买入额": "78369279",
                        "融资偿还额": "91883384",
                        "融券余量": "13696100",
                        "融券卖出量": "1936100",
                        "融券偿还量": "3572000",
                    }
                ]
            )

        def stock_margin_szse(self, date: str) -> Any:
            assert date == "20260702"
            return FakeMarginFrame(
                [
                    {
                        "融资买入额": "321.08",
                        "融资余额": "7077.67",
                        "融券卖出量": "0.28",
                        "融券余量": "24.34",
                        "融券余额": "157.3",
                        "融资融券余额": "7234.97",
                    }
                ]
            )

        def stock_margin_detail_szse(self, date: str) -> Any:
            assert date == "20260702"
            return FakeMarginFrame(
                [
                    {
                        "证券代码": "000001",
                        "证券简称": "平安银行",
                        "融资买入额": "153304267",
                        "融资余额": "4910810456",
                        "融券卖出量": "3600",
                        "融券余量": "593200",
                        "融券余额": "6080300",
                        "融资融券余额": "4916890756",
                    }
                ]
            )

    sse_summaries, sse_details = fetch_margin_trading_market(FakeMarginAK(), "2026-07-02", "sse", 0)
    assert sse_summaries[0]["market"] == "sse"
    assert sse_summaries[0]["trade_date"] == "2026-07-02"
    assert sse_details[0]["code"] == "510050"
    assert sse_details[0]["margin_trading_balance"] is None
    szse_summaries, szse_details = fetch_margin_trading_market(FakeMarginAK(), "2026-07-02", "szse", 0)
    assert szse_summaries[0]["margin_buy_amount"] == 32108000000
    assert szse_summaries[0]["short_balance_volume"] == 2434000000
    assert szse_details[0]["code"] == "000001"
    assert szse_details[0]["margin_trading_balance"] == 4916890756

    class FakeFrame:
        def iterrows(self) -> Any:
            return iter(
                [
                    (0, {"trade_date": "2026-06-12"}),
                    (1, {"trade_date": "2026-06-15"}),
                    (2, {"trade_date": "2026-06-18"}),
                    (3, {"trade_date": "2026-06-19"}),
                    (4, {"trade_date": "2026-06-20"}),
                    (5, {"trade_date": "2026-06-22"}),
                ]
            )

    class FakeAK:
        def __init__(self) -> None:
            self.calls = 0

        def tool_trade_date_hist_sina(self) -> Any:
            self.calls += 1
            return FakeFrame()

    reset_trading_day_calendar_cache()
    latest_fake = FakeAK()
    assert latest_trading_day(latest_fake, "2026-06-17") == "2026-06-15"
    assert latest_fake.calls == 1
    assert latest_trading_day(latest_fake, "2026-06-17") == "2026-06-15"
    assert latest_fake.calls == 1

    reset_trading_day_calendar_cache()
    calendar_fake = FakeAK()
    assert trading_day_calendar(calendar_fake) == ["2026-06-12", "2026-06-15", "2026-06-18", "2026-06-22"]
    assert trading_day_calendar(calendar_fake) == ["2026-06-12", "2026-06-15", "2026-06-18", "2026-06-22"]
    assert calendar_fake.calls == 1
    trading = trading_day_status(calendar_fake, "2026-06-15")
    assert calendar_fake.calls == 1
    assert trading["date"] == "2026-06-15"
    assert trading["is_trading_day"] is True
    assert trading["previous_trading_day"] == "2026-06-12"
    assert trading["next_trading_day"] == "2026-06-18"
    closed = trading_day_status(FakeAK(), "2026-06-17")
    assert closed["is_trading_day"] is False
    assert closed["latest_trading_day"] == "2026-06-15"
    assert closed["previous_trading_day"] == "2026-06-15"
    assert closed["next_trading_day"] == "2026-06-18"
    holiday = trading_day_status(FakeAK(), "2026-06-19")
    assert holiday["is_trading_day"] is False
    assert holiday["previous_trading_day"] == "2026-06-18"
    assert holiday["next_trading_day"] == "2026-06-22"
    report = research_report_item(
        {
            "股票代码": "2230",
            "股票简称": "科大讯飞",
            "报告名称": "科大讯飞深度研究",
            "东财评级": "买入",
            "机构": "中金公司",
            "日期": "2026-06-16",
            "报告PDF链接": "https://example.com/report.pdf",
            "行业": "软件开发",
        },
        "002230",
    )
    assert report is not None
    assert report["code"] == "002230"
    assert report["source_type"] == "akshare_stock_research"
    assert report["pdf_status"] == "pending"
    assert date_in_range("2026-06-16", "2026-06-01", "2026-06-30")
    assert normalize_holding_period("2026-Q1") == "20260331"
    assert normalize_holding_period("2026年第2季度报告") == "20260630"
    assert normalize_holding_period("2026-03-31") == "20260331"
    assert holding_period_to_sina_quarter("20260331") == "20261"
    holding = holding_row_item(
        {
            "股票代码": "2230",
            "股票简称": "科大讯飞",
            "报告期": "2026-03-31",
            "股东名称": "全国社保基金一一八组合",
            "股东类型": "社保",
            "股东排名": 2,
            "期末持股-数量": "1000",
            "期末持股-数量变化": "100",
            "期末持股-持股占流通股比": "1.5",
            "期末持股-流通市值": "50000",
            "公告日": "2026-04-30",
        },
        "stock_gdfx_free_holding_detail_em",
        "20260331",
    )
    assert holding is not None
    assert holding["stock_code"] == "002230"
    assert holding["report_period"] == "20260331"
    assert holding["holder_type"] == "social_security"
    assert holding["shares"] == 1000
    fund_holding = holding_row_item(
        {
            "股票代码": "002230",
            "报告期": "2026-Q2",
            "基金名称": "易方达蓝筹精选混合型证券投资基金",
            "基金代码": "005827",
            "基金公司": "易方达基金管理有限公司",
            "持股数": "2000",
        },
        "stock_fund_stock_holder",
        "20260630",
    )
    assert fund_holding is not None
    assert fund_holding["holder_type"] == "fund"
    assert fund_holding["fund_code"] == "005827"
    assert fund_holding["fund_company"] == "易方达基金管理有限公司"
    assert fund_holding["disclosure_scope"] == "fund_quarterly"
    assert holding_worker_count(12, 25) == DEFAULT_HOLDING_WORKERS
    assert holding_worker_count(2, 25) == 2

    class FakeHoldingFrame:
        def __init__(self, rows: list[dict[str, Any]]) -> None:
            self.rows = rows

        def iterrows(self) -> Any:
            return iter(enumerate(self.rows))

    class FakeHoldingAK:
        def stock_gdfx_free_holding_detail_em(self, date: str) -> Any:
            return FakeHoldingFrame([{"股票代码": "002230", "报告期": date, "股东名称": "全国社保基金一一八组合"}])

        def stock_gdfx_holding_detail_em(self, date: str, indicator: str, symbol: str) -> Any:
            if indicator == "基金" and symbol == "新进":
                return FakeHoldingFrame([{"股票代码": "300059", "报告期": date, "股东名称": "易方达基金"}])
            return FakeHoldingFrame([])

    market_holdings, market_warnings = fetch_market_holdings_for_period(FakeHoldingAK(), "20260331", 4)
    assert not market_warnings
    assert sorted(item["stock_code"] for item in market_holdings) == ["002230", "300059"]
    docs = parse_tiantian_fund_report_documents(
        '<a href="/gonggao/005827,AN202607231650000000.html">易方达蓝筹精选混合型证券投资基金2026年第2季度报告</a>',
        "20260630",
        10,
    )
    assert len(docs) == 1
    assert docs[0]["report_period"] == "20260630"
    assert docs[0]["source_type"] == "tiantian_fund_regular_report"
    print("akshare auction service self-test passed")


def main() -> int:
    parser = build_parser()
    args = parser.parse_args()
    if args.self_test:
        run_self_test()
        return 0

    service = AuctionService(Path(args.cache_dir), args.workers, args.limit)
    RequestHandler.service = service
    server = ThreadingHTTPServer((args.host, args.port), RequestHandler)
    print(
        f"akshare auction service listening on http://{args.host}:{args.port}, "
        f"workers={service.workers}, limit={service.default_limit}, cache={service.cache_dir}",
        flush=True,
    )
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        print("akshare auction service stopping", flush=True)
    finally:
        server.server_close()
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
