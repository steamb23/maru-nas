#!/usr/bin/env python3
"""로그인 대기 이스터에그(traceroute 지구본)의 육지 점 비트마스크를 만든다.

지도 원본: Natural Earth 1:110m Physical Vectors, Land (퍼블릭 도메인, 출처 표기 의무 없음)
  https://www.naturalearthdata.com/
  https://raw.githubusercontent.com/nvkelso/natural-earth-vector/master/geojson/ne_110m_land.geojson

사용법(저장소 루트에서, GeoJSON은 저장소 밖에 내려받아 둔다):
  python3 -I scripts/gen-globe-dots.py <ne_110m_land.geojson> > frontend/src/auth/easterEgg/globeDots.gen.ts

격자는 frontend/src/auth/easterEgg/globe.ts의 dotGrid와 같은 규칙이어야 한다.
- 줄 수 rows = floor(180 / step + 0.5), 위도는 남쪽부터 lat = -90 + (i + 0.5) * step
- 줄마다 점 수 n = max(1, floor(360 * cos(lat) / step + 0.5)), 경도는 lon = -180 + (j + 0.5) * 360 / n
- Python round(은행가 반올림)와 JS Math.round가 다르므로 둘 다 floor(x + 0.5)를 쓴다
비트마스크는 격자 순서대로 점 k를 바이트 k >> 3의 비트 k & 7(LSB부터)에 담아 base64로 내보낸다.
"""

import base64
import json
import math
import sys

STEP_DEG = 2.5


def dot_grid(step):
    rows = math.floor(180 / step + 0.5)
    for i in range(rows):
        lat = -90 + (i + 0.5) * step
        n = max(1, math.floor(360 * math.cos(math.radians(lat)) / step + 0.5))
        for j in range(n):
            yield -180 + (j + 0.5) * 360 / n, lat


def load_rings(path):
    with open(path, encoding="utf-8") as f:
        data = json.load(f)
    rings = []
    for feature in data["features"]:
        geom = feature["geometry"]
        if geom["type"] == "Polygon":
            polygons = [geom["coordinates"]]
        elif geom["type"] == "MultiPolygon":
            polygons = geom["coordinates"]
        else:
            continue
        for polygon in polygons:
            for ring in polygon:
                xs = [p[0] for p in ring]
                ys = [p[1] for p in ring]
                rings.append(((min(xs), min(ys), max(xs), max(ys)), ring))
    return rings


def inside(lon, lat, rings):
    # 모든 고리에 짝홀 규칙을 적용하므로 구멍(안쪽 고리)도 처리된다.
    hit = False
    for (x0, y0, x1, y1), ring in rings:
        if lon < x0 or lon > x1 or lat < y0 or lat > y1:
            continue
        for a, b in zip(ring, ring[1:] + ring[:1]):
            (ax, ay), (bx, by) = a[:2], b[:2]
            if (ay > lat) != (by > lat) and lon < ax + (lat - ay) * (bx - ax) / (by - ay):
                hit = not hit
    return hit


def main():
    if len(sys.argv) != 2:
        sys.exit("usage: gen-globe-dots.py <ne_110m_land.geojson>")
    rings = load_rings(sys.argv[1])
    bits = [inside(lon, lat, rings) for lon, lat in dot_grid(STEP_DEG)]
    mask = bytearray((len(bits) + 7) // 8)
    for k, on in enumerate(bits):
        if on:
            mask[k >> 3] |= 1 << (k & 7)
    b64 = base64.b64encode(bytes(mask)).decode("ascii")
    chunks = [b64[i : i + 100] for i in range(0, len(b64), 100)]

    out = sys.stdout
    out.write("// 생성물: scripts/gen-globe-dots.py로 만든다. 직접 수정하지 않는다.\n")
    out.write("// 지도 원본: Natural Earth 1:110m Land(퍼블릭 도메인). 격자 규칙은 globe.ts의 dotGrid.\n\n")
    out.write(f"export const DOT_STEP_DEG = {STEP_DEG}\n")
    out.write(f"export const DOT_COUNT = {len(bits)}\n")
    out.write(f"export const LAND_COUNT = {sum(bits)}\n\n")
    out.write("// 격자 순서대로 점 k가 육지면 바이트 k >> 3의 비트 k & 7이 1이다.\n")
    out.write("export const LAND_MASK_BASE64 = [\n")
    for c in chunks:
        out.write(f"  '{c}',\n")
    out.write("].join('')\n")


if __name__ == "__main__":
    main()
