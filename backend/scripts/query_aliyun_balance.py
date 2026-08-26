#!/usr/bin/env python3
"""Query Alibaba Cloud account balance without third-party dependencies."""

import argparse
import base64
import getpass
import hashlib
import hmac
import json
import os
import secrets
import sys
import urllib.error
import urllib.parse
import urllib.request
from datetime import datetime, timezone


ENDPOINT = "https://business.aliyuncs.com/"


def percent_encode(value: str) -> str:
    return urllib.parse.quote(str(value), safe="-_.~")


def signed_query(access_key_id: str, access_key_secret: str, timestamp: str, nonce: str) -> str:
    params = {
        "AccessKeyId": access_key_id,
        "Action": "QueryAccountBalance",
        "Format": "JSON",
        "SignatureMethod": "HMAC-SHA1",
        "SignatureNonce": nonce,
        "SignatureVersion": "1.0",
        "Timestamp": timestamp,
        "Version": "2017-12-14",
    }
    canonical = "&".join(
        f"{percent_encode(key)}={percent_encode(value)}"
        for key, value in sorted(params.items())
    )
    string_to_sign = f"GET&%2F&{percent_encode(canonical)}"
    signature = base64.b64encode(
        hmac.new(
            f"{access_key_secret}&".encode(),
            string_to_sign.encode(),
            hashlib.sha1,
        ).digest()
    ).decode()
    return f"{canonical}&Signature={percent_encode(signature)}"


def self_test() -> None:
    assert percent_encode("a b+c/*~") == "a%20b%2Bc%2F%2A~"
    query = signed_query("testid", "testsecret", "2026-08-26T00:00:00Z", "nonce")
    assert query.endswith("Signature=u29Rvqfq%2By6sNDxq504MYPPRHQo%3D")
    print("self-test passed")


def credentials() -> tuple[str, str]:
    access_key_id = os.getenv("ALIBABA_CLOUD_ACCESS_KEY_ID", "").strip()
    access_key_secret = os.getenv("ALIBABA_CLOUD_ACCESS_KEY_SECRET", "").strip()
    if not access_key_id:
        access_key_id = input("AccessKey ID: ").strip()
    if not access_key_secret:
        access_key_secret = getpass.getpass("AccessKey Secret: ").strip()
    if not access_key_id or not access_key_secret:
        raise ValueError("both AccessKey ID and AccessKey Secret are required")
    return access_key_id, access_key_secret


def main() -> int:
    parser = argparse.ArgumentParser(description="Query Alibaba Cloud QueryAccountBalance")
    parser.add_argument("--self-test", action="store_true")
    args = parser.parse_args()
    if args.self_test:
        self_test()
        return 0

    try:
        access_key_id, access_key_secret = credentials()
        timestamp = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
        query = signed_query(access_key_id, access_key_secret, timestamp, secrets.token_hex(16))
        request = urllib.request.Request(f"{ENDPOINT}?{query}", headers={"Accept": "application/json"})
        with urllib.request.urlopen(request, timeout=20) as response:
            payload = json.loads(response.read())
    except urllib.error.HTTPError as error:
        body = error.read().decode(errors="replace")
        print(f"HTTP {error.code}: {body}", file=sys.stderr)
        return 1
    except (urllib.error.URLError, ValueError, json.JSONDecodeError) as error:
        print(f"request failed: {error}", file=sys.stderr)
        return 1

    print(json.dumps(payload, ensure_ascii=False, indent=2))
    data = payload.get("Data") or {}
    if payload.get("Success") is False or payload.get("Code") not in (None, "Success"):
        return 1
    print(
        "balance:",
        data.get("AvailableAmount"),
        data.get("Currency", "CNY"),
        "cash:",
        data.get("AvailableCashAmount"),
        "credit:",
        data.get("CreditAmount"),
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
