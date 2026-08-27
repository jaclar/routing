#!/usr/bin/env python3
"""Convert GSHHG high-resolution binary shoreline data (gshhs_h.b) into an optimized binary format with tuned geometric precision for ultra-fast routing and rendering."""

import struct
import os
import sys
import math
from typing import List, Tuple

def rdp(points: List[Tuple[float, float]], epsilon: float) -> List[Tuple[float, float]]:
    """Ramer-Douglas-Peucker polygon simplification."""
    if len(points) <= 2:
        return points
    dmax = 0.0
    index = 0
    end = len(points) - 1

    line_start = points[0]
    line_end = points[-1]
    dx = line_end[0] - line_start[0]
    dy = line_end[1] - line_start[1]
    norm = math.sqrt(dx * dx + dy * dy)

    for i in range(1, end):
        if norm == 0:
            d = math.hypot(points[i][0] - line_start[0], points[i][1] - line_start[1])
        else:
            d = abs(dy * points[i][0] - dx * points[i][1] + line_end[0] * line_start[1] - line_end[1] * line_start[0]) / norm
        if d > dmax:
            index = i
            dmax = d

    if dmax > epsilon:
        rec1 = rdp(points[:index + 1], epsilon)
        rec2 = rdp(points[index:], epsilon)
        return rec1[:-1] + rec2
    else:
        return [points[0], points[-1]]


def get_landmass_name(pid: int, s: float, nr: float, w: float, e: float) -> str:
    """Assign human-readable geographic names to major landmasses."""
    if pid == 0: return 'Eurasia'
    if pid == 1: return 'Africa'
    if pid == 2: return 'North America'
    if pid == 3: return 'South America'
    if pid in (4, 5): return 'Antarctica'
    if pid == 6: return 'Australia'
    if pid == 7: return 'Greenland'
    if pid == 8: return 'New Guinea'
    if pid == 9: return 'Borneo'
    if pid == 10: return 'Madagascar'
    if pid == 11: return 'Baffin Island'
    if pid == 12: return 'Sumatra'
    if pid == 13: return 'Honshu (Japan)'
    if pid == 14: return 'Victoria Island'
    if pid == 15: return 'Great Britain'
    if pid == 22: return 'Cuba'
    if pid == 126 or (10.0 <= s <= 10.9 and -62.0 <= w <= -60.8): return 'Trinidad'
    if pid == 714 or (11.9 <= s <= 12.3 and -61.9 <= w <= -61.5): return 'Grenada'
    if 11.1 <= s <= 11.4 and -60.9 <= w <= -60.5: return 'Tobago'
    if 32.2 <= s <= 32.5 and -64.9 <= w <= -64.6: return 'Bermuda'
    if 51.3 <= s <= 55.5 and -10.6 <= w <= -5.5: return 'Ireland'
    if 18.9 <= s <= 22.3 and -160.3 <= w <= -154.8: return 'Hawaii'
    if 32.6 <= s <= 32.9 and -17.3 <= w <= -16.6: return 'Madeira'
    if 36.9 <= s <= 39.8 and -31.3 <= w <= -25.0: return 'Azores'
    if 27.6 <= s <= 29.5 and -18.2 <= w <= -13.3: return 'Canary Islands'
    if 13.0 <= s <= 13.4 and -59.7 <= w <= -59.4: return 'Barbados'
    if 13.7 <= s <= 14.2 and -61.2 <= w <= -60.8: return 'St. Lucia'
    if 14.3 <= s <= 15.0 and -61.3 <= w <= -60.8: return 'Martinique'
    if 15.8 <= s <= 16.6 and -61.9 <= w <= -61.1: return 'Guadeloupe'
    if 17.8 <= s <= 18.6 and -65.1 <= w <= -64.2: return 'Virgin Islands'
    if 17.8 <= s <= 18.6 and -67.4 <= w <= -65.5: return 'Puerto Rico'
    if 18.0 <= s <= 20.1 and -74.6 <= w <= -68.3: return 'Hispaniola'

    return f'Land #{pid}'


def main():
    possible_paths = [
        'external assets/gshhg-bin-2/gshhs_h.b',
        'assets/gshhg-bin-2/gshhs_h.b',
        'assets/gshhs_h.b',
    ]
    input_file = None
    for p in possible_paths:
        if os.path.exists(p):
            input_file = p
            break

    if not input_file:
        print(f'Error: Could not find gshhs_h.b in {possible_paths}', file=sys.stderr)
        sys.exit(1)

    output_dir1 = 'services/routing/data'
    output_dir2 = 'services/routing/landmask/data'
    os.makedirs(output_dir1, exist_ok=True)
    os.makedirs(output_dir2, exist_ok=True)
    output_bin1 = os.path.join(output_dir1, 'gshhg_landmask.bin')
    output_bin2 = os.path.join(output_dir2, 'gshhg_landmask.bin')

    print(f'Reading GSHHG high-resolution source: {input_file}')
    polygons = []
    total_raw_points = 0
    total_simp_points = 0

    with open(input_file, 'rb') as f:
        while True:
            hdr = f.read(44)
            if len(hdr) < 44:
                break
            pid, n, flag, west, east, south, north, area, area_full, container, ancestor = struct.unpack('>11i', hdr)
            level = flag & 255
            pts_raw = f.read(n * 8)

            # Keep level 1 (continental/island shoreline) and level 6 (antarctica grounding line)
            if level not in (1, 6):
                continue

            # Retain islands with >= 6 points or named features; filter micro-noise
            if n < 6 and pid > 50:
                continue

            s_lat = south / 1e6
            n_lat = north / 1e6
            w_lon = west / 1e6
            if w_lon > 180.0:
                w_lon -= 360.0
            e_lon = east / 1e6
            if e_lon > 180.0:
                e_lon -= 360.0

            if w_lon > e_lon:
                w_lon, e_lon = -180.0, 180.0

            pts: List[Tuple[float, float]] = []
            for i in range(n):
                x, y = struct.unpack('>2i', pts_raw[i * 8:(i + 1) * 8])
                lon = x / 1e6
                if lon > 180.0:
                    lon -= 360.0
                lat = y / 1e6
                pts.append((lat, lon))

            total_raw_points += len(pts)

            # High-performance adaptive simplification:
            # - Giant continents (pid 0..5): eps = 0.020 (~2km) gives ~1,500-4,000 pts with complete outline
            # - Large islands (>1,000 pts): eps = 0.004 (~400m)
            # - Sailing passages & coastal islands: eps = 0.0008 (~80m) for high-precision bays & channels
            if pid in (0, 1, 2, 3, 4, 5):
                eps = 0.022
            elif len(pts) > 1000:
                eps = 0.004
            elif len(pts) > 100:
                eps = 0.0012
            else:
                eps = 0.0008

            simp = rdp(pts, eps)
            if len(simp) < 3:
                simp = pts[:3] if len(pts) >= 3 else pts

            if simp[0] != simp[-1]:
                simp.append(simp[0])

            total_simp_points += len(simp)

            name = get_landmass_name(pid, s_lat, n_lat, w_lon, e_lon)
            polygons.append({
                'id': pid,
                'name': name,
                'min_lat': min(p[0] for p in simp),
                'max_lat': max(p[0] for p in simp),
                'min_lon': min(p[1] for p in simp),
                'max_lon': max(p[1] for p in simp),
                'vertices': simp,
            })

    print(f'Extracted {len(polygons)} landmass polygons.')
    print(f'Points reduced from {total_raw_points:,} to {total_simp_points:,} ({(total_simp_points / total_raw_points) * 100:.1f}%).')

    # Encode binary format (Little-Endian)
    for out_path in [output_bin1, output_bin2]:
        with open(out_path, 'wb') as out:
            out.write(b'GSHH')
            out.write(struct.pack('<HI', 1, len(polygons)))

            for p in polygons:
                name_bytes = p['name'].encode('utf-8')[:255]
                out.write(struct.pack('<I', p['id']))
                out.write(struct.pack('<B', len(name_bytes)))
                out.write(name_bytes)
                out.write(struct.pack('<4f', p['min_lat'], p['max_lat'], p['min_lon'], p['max_lon']))
                out.write(struct.pack('<I', len(p['vertices'])))
                for lat, lon in p['vertices']:
                    out.write(struct.pack('<2f', lat, lon))

        bin_size = os.path.getsize(out_path)
        print(f'Successfully wrote binary dataset: {out_path} ({bin_size / 1024 / 1024:.2f} MB)')


if __name__ == '__main__':
    main()
