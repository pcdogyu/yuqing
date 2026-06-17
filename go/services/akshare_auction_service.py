#!/usr/bin/env python3
"""Small AKShare HTTP adapter for A-share opening auction amounts.

The Go scheduler calls:
  GET /api/a-stock/auction?date=YYYY-MM-DD

This service uses AKShare's Eastmoney pre-market minute API and returns the
JSON contract consumed by scheduler-service. It intentionally stays on the
standard library so only AKShare and its normal data dependencies are needed.
"""

from __future__ import annotations

import argparse
import concurrent.futures
import datetime as dt
import json
import math
import os
import sys
import threading
import time
import traceback
import urllib.parse
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from typing import Any


DEFAULT_HOST = "127.0.0.1"
DEFAULT_PORT = 19091
DEFAULT_WORKERS = 12
DEFAULT_CACHE_DIR = Path("data") / "akshare-cache" / "a-stock-auction"

_akshare_module: Any | None = None
_akshare_error: str | None = None
_akshare_lock = threading.Lock()


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
    return str(value).strip()


def first_existing(row: Any, names: list[str]) -> Any:
    for name in names:
        try:
            value = row[name]
        except Exception:
            continue
        if text_value(value) != "":
            return value
    return None


def row_time_text(row: Any) -> str:
    value = first_existing(row, ["时间", "time", "datetime", "日期"])
    return text_value(value)


def select_auction_row(frame: Any) -> Any | None:
    if frame is None or getattr(frame, "empty", False):
        return None
    rows = []
    try:
        iterator = frame.iterrows()
    except Exception:
        return None
    for _, row in iterator:
        time_text = row_time_text(row)
        if "09:25" in time_text:
            return row
        rows.append((time_text, row))
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


def load_symbols(ak: Any, explicit_codes: list[str], limit: int) -> list[dict[str, str]]:
    if explicit_codes:
        return [{"code": code, "name": ""} for code in explicit_codes]
    try:
        frame = ak.stock_zh_a_spot_em()
    except Exception:
        frame = ak.stock_info_a_code_name()
    symbols: list[dict[str, str]] = []
    for _, row in frame.iterrows():
        code = text_value(first_existing(row, ["代码", "code", "股票代码"]))
        name = text_value(first_existing(row, ["名称", "name", "股票名称"]))
        if not code:
            continue
        symbols.append({"code": code.zfill(6), "name": name})
        if limit > 0 and len(symbols) >= limit:
            break
    return symbols


def fetch_market_snapshot(ak: Any, trade_date: str, limit: int) -> list[dict[str, Any]]:
    frame = ak.stock_zh_a_spot_em()
    items: list[dict[str, Any]] = []
    fetched_at = utc_now_iso()
    for _, row in frame.iterrows():
        code = text_value(first_existing(row, ["代码", "code", "股票代码"]))
        name = text_value(first_existing(row, ["名称", "name", "股票名称"]))
        if not code:
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
    status = text_value(item.get("status")).lower()
    if status and status != "ok":
        return False
    return finite_float(item.get("auction_amount")) > 0 or finite_float(item.get("auction_volume")) > 0


def payload_has_usable_items(payload: dict[str, Any]) -> bool:
    items = payload.get("items")
    if not isinstance(items, list):
        return False
    return any(isinstance(item, dict) and item_has_usable_amount(item) for item in items)


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

    def fetch(self, query: dict[str, list[str]]) -> dict[str, Any]:
        trade_date = normalize_date(first_query_value(query, "date"))
        code_value = first_query_value(query, "code") or ""
        explicit_codes = [item.strip().zfill(6) for item in code_value.split(",") if item.strip()]
        limit = int_value(first_query_value(query, "limit"), self.default_limit)
        force = first_query_value(query, "force") in {"1", "true", "yes"}

        if not force:
            cached = read_cache(self.cache_dir, trade_date)
            if cached is not None and payload_has_usable_items(cached):
                return cached

        if trade_date != local_today():
            return {
                "_http_status": 422,
                "date": trade_date,
                "items": [],
                "message": "AKShare auction adapter only serves the current trading day without a usable local cache for the requested date.",
                "fetched_at": utc_now_iso(),
            }

        started = time.time()
        ak = load_akshare()
        warning = ""
        if explicit_codes:
            symbols = load_symbols(ak, explicit_codes, limit)
            with concurrent.futures.ThreadPoolExecutor(max_workers=self.workers) as pool:
                items = list(pool.map(lambda symbol: fetch_one_auction(ak, symbol, trade_date), symbols))
        else:
            try:
                items = fetch_market_snapshot(ak, trade_date, limit)
            except Exception as exc:
                fallback_limit = int(os.getenv("AKSHARE_AUCTION_FALLBACK_LIMIT", "300"))
                effective_limit = limit if limit > 0 else max(0, fallback_limit)
                try:
                    symbols = load_symbols(ak, [], effective_limit)
                    with concurrent.futures.ThreadPoolExecutor(max_workers=self.workers) as pool:
                        items = list(pool.map(lambda symbol: fetch_one_auction(ak, symbol, trade_date), symbols))
                    warning = f"stock_zh_a_spot_em failed, used pre-market fallback: {exc}"
                except Exception as fallback_exc:
                    items = []
                    warning = (
                        "AKShare market snapshot and fallback symbol list both failed: "
                        f"snapshot={exc}; fallback={fallback_exc}"
                    )
        payload = {
            "date": trade_date,
            "items": items,
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
            if parsed.path == "/healthz":
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
    assert payload_has_usable_items({"items": [{"status": "ok", "auction_amount": 1}]})
    assert not payload_has_usable_items({"items": [{"status": "no_auction_amount", "auction_amount": 0}]})
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
