#!/usr/bin/env python3
# 地址规则同步 / Address Rule Snapshot Sync
# 功能：维护者显式刷新公开国家级地址元数据；运行/构建不联网，不上传顾客资料
# 作者：仗键天涯(daxing)
# 邮箱：3442535897@qq.com
# 时间：2026-10-25 17:14:28

import concurrent.futures
import hashlib
import json
import pathlib
import re
import urllib.request
import urllib.error
from datetime import datetime, timezone

ROOT = pathlib.Path(__file__).resolve().parents[1]
SOURCE = 'https://www.gstatic.com/chrome/autofill/libaddressinput/chromium-i18n/ssl-address/data/'
FIELDS = ('fmt', 'lfmt', 'require', 'zip', 'zipex', 'state_name_type', 'locality_name_type', 'sublocality_name_type', 'zip_name_type')
FORMAT_SOURCE = 'https://raw.githubusercontent.com/google/libaddressinput/master/common/src/main/java/com/google/i18n/addressinput/common/RegionDataConstants.java'


def format_fallback(raw):
    # Parse string literals only. Never run downloaded Java/source code.
    result = {}
    for code, expression in re.findall(r'map\.put\(\s*"([A-Z]{2})",(.*?)\);', raw.decode(), re.S):
        tokens = re.findall(r'"(?:\\.|[^"\\])*"', expression)
        remainder = re.sub(r'"(?:\\.|[^"\\])*"', '', expression)
        if re.sub(r'[\s+]', '', remainder):
            raise ValueError('Unexpected format source expression')
        data = json.loads(''.join(json.loads(token) for token in tokens))
        result[code] = {key: data[key] for key in FIELDS if key in data}
    if 'MY' not in result or 'ZZ' not in result:
        raise ValueError('Incomplete format source')
    return result


def fetch(code):
    try:
        with urllib.request.urlopen(SOURCE + code, timeout=20) as response:
            raw = response.read(262145)
    except urllib.error.HTTPError as error:
        if error.code == 404 and code != 'ZZ':
            # Existing catalog destinations without published metadata keep
            # the explicit default profile, not invented postal requirements.
            return code, {}, None
        raise RuntimeError('Cannot fetch country metadata: ' + code) from error
    if len(raw) > 262144:
        raise ValueError('Country metadata exceeds limit: ' + code)
    data = json.loads(raw)
    if data.get('id') != 'data/' + code:
        raise ValueError('Unexpected country metadata: ' + code)
    # The current static service serializes regex escapes twice. Decode only
    # doubled backslashes in the regex, never the address labels or examples.
    item = {key: data[key] for key in FIELDS if key in data}
    if 'zip' in item:
        item['zip'] = item['zip'].replace('\\\\', '\\')
    return code, item, hashlib.sha256(raw).hexdigest()


def main():
    catalog = (ROOT / 'internal/order/countries.go').read_text()
    codes = sorted(set(re.findall(r'\{"([A-Z]{2})",', catalog)) | {'ZZ'})
    with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:
        results = list(pool.map(fetch, codes))
    fallback_source = None
    if any(digest is None for _, _, digest in results):
        with urllib.request.urlopen(FORMAT_SOURCE, timeout=20) as response:
            source = response.read(1048577)
        if len(source) > 1048576:
            raise ValueError('Format source exceeds limit')
        formats = format_fallback(source)
        results = [(code, formats.get(code, {}) if digest is None else item, digest) for code, item, digest in results]
        fallback_source = {'url': FORMAT_SOURCE, 'sha256': hashlib.sha256(source).hexdigest()}
    snapshot = {
        '_meta': {
            'name': '国家地址规则 / Country Address Rules',
            'purpose': 'Required fields, labels and postal syntax; not deliverability verification',
            'author': '仗键天涯(daxing)', 'email': '3442535897@qq.com',
            'time': datetime.now(timezone.utc).isoformat(timespec='seconds'),
            'source': SOURCE, 'license': 'CC-BY-4.0',
            'attribution': 'Google libaddressinput address metadata, reduced and normalized by Kartwo; see NOTICE.md',
            'source_sha256': {code: digest for code, _, digest in results},
            'unpublished_countries': [code for code, _, digest in results if digest is None],
            'format_fallback_source': fallback_source,
        },
        'countries': {code: item for code, item, _ in results},
    }
    target = ROOT / 'internal/address/rules.json'
    target.parent.mkdir(parents=True, exist_ok=True)
    target.write_text(json.dumps(snapshot, ensure_ascii=False, indent=2) + '\n')
    print('Saved', len(codes) - 1, 'country rules and one default; no customer data or settings accessed')


if __name__ == '__main__':
    main()
