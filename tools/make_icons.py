#!/usr/bin/env python3
"""Draw the map template icons of the template picker (icons/*.png).

32x32 pixel art, saved at 64x64 (nearest neighbour) so the client keeps the
pixels sharp. The plugin embeds the files and sends them inline as
"[png:<base64>" textures, so they need no media on the servers.

  python3 tools/make_icons.py          (needs Pillow)
  python3 tools/make_icons.py --europe DIR
      also redraws europe.png from the prepare.py output of the Europe map
      (luanti-tools/countrygen; needs numpy); without it europe.png is kept
"""

import math
import os
import random

from PIL import Image, ImageDraw

OUT = os.path.join(os.path.dirname(os.path.dirname(os.path.abspath(__file__))), "icons")
N = 32
OUTLINE = (24, 22, 30, 255)


def canvas():
    img = Image.new("RGBA", (N, N), (0, 0, 0, 0))
    return img, ImageDraw.Draw(img)


def save(img, name):
    img.resize((N * 2, N * 2), Image.NEAREST).save(os.path.join(OUT, name + ".png"), optimize=True)


# ── Flags ─────────────────────────────────────────────────────────────────────

FX0, FY0, FX1, FY1 = 2, 7, 29, 24   # flag area, inclusive (28x18)


def flag(name, stripes, vertical=False, extra=None):
    img, d = canvas()
    d.rectangle([FX0 + 1, FY0 + 1, FX1 + 1, FY1 + 1], fill=(0, 0, 0, 90))  # shadow
    total = sum(w for w, _ in stripes)
    span = (FX1 - FX0 + 1) if vertical else (FY1 - FY0 + 1)
    pos = 0
    for i, (w, col) in enumerate(stripes):
        a = round(pos * span / total)
        pos += w
        b = round(pos * span / total) - 1
        if vertical:
            d.rectangle([FX0 + a, FY0, FX0 + b, FY1], fill=col)
        else:
            d.rectangle([FX0, FY0 + a, FX1, FY0 + b], fill=col)
    if extra:
        extra(d)
    # light from the top left, a little shade at the bottom right
    for y in range(FY0, FY1 + 1):
        for x in range(FX0, FX1 + 1):
            r, g, b, a = img.getpixel((x, y))
            k = 1.08 - 0.16 * ((x - FX0) / (FX1 - FX0) + (y - FY0) / (FY1 - FY0)) / 2
            img.putpixel((x, y), (min(255, int(r * k)), min(255, int(g * k)), min(255, int(b * k)), a))
    d.rectangle([FX0 - 1, FY0 - 1, FX1 + 1, FY1 + 1], outline=OUTLINE)
    save(img, name)


def spain_arms(d):
    # simplified coat of arms on the yellow band, towards the hoist: crown,
    # shield in four quarters, Granada at the bottom
    d.point([(8, 10), (10, 10), (12, 10)], fill=(150, 104, 0))
    d.line([(8, 11), (12, 11)], fill=(150, 104, 0))
    d.rectangle([7, 12, 13, 18], fill=(120, 20, 24))
    d.point([(7, 18), (13, 18)], fill=(241, 191, 0))
    d.rectangle([8, 13, 9, 15], fill=(206, 34, 34))       # Castile
    d.rectangle([11, 13, 12, 15], fill=(244, 244, 236))   # Leon
    d.point([(10, 13), (10, 14), (10, 15), (10, 16), (10, 17)], fill=(120, 20, 24))
    d.point([(8, 16), (9, 17)], fill=(241, 191, 0))       # Aragon
    d.point([(9, 16), (8, 17)], fill=(206, 34, 34))
    d.rectangle([11, 16, 12, 17], fill=(206, 34, 34))     # Navarre


# ── Cubes ─────────────────────────────────────────────────────────────────────

GRASS = [(98, 160, 64), (88, 148, 56), (110, 172, 72), (80, 138, 52)]
DIRT_L = [(136, 96, 62), (122, 86, 54), (146, 104, 68)]
DIRT_R = [(106, 74, 46), (96, 66, 42), (114, 80, 50)]


def iso_block(d, rng, top_y, height, top_fill=None):
    """Grass block in 2:1 isometric view, 28 px wide, side `height` px."""
    cx = 15.5
    top = [(cx, top_y), (29.5, top_y + 7), (cx, top_y + 14), (1.5, top_y + 7)]
    left = [(1.5, top_y + 7), (cx, top_y + 14), (cx, top_y + 14 + height), (1.5, top_y + 7 + height)]
    right = [(cx, top_y + 14), (29.5, top_y + 7), (29.5, top_y + 7 + height), (cx, top_y + 14 + height)]
    mask = Image.new("L", (N, N), 0)
    md = ImageDraw.Draw(mask)
    md.polygon(left, fill=1)
    md.polygon(right, fill=2)
    md.polygon(top, fill=3)
    px = d._image.load()
    for y in range(N):
        for x in range(N):
            m = mask.getpixel((x, y))
            if m == 3:
                px[x, y] = rng.choice(GRASS) + (255,)
            elif m in (1, 2):
                # grass fringe at the top of the sides
                edge = top_y + 7 + (x - 1.5) / 2 if x < cx else top_y + 14 - (x - cx) / 2
                if y - edge < 2.5 + (rng.random() < 0.4):
                    px[x, y] = (GRASS[3] if m == 1 else (66, 116, 44)) + (255,)
                else:
                    px[x, y] = rng.choice(DIRT_L if m == 1 else DIRT_R) + (255,)
    if top_fill:
        top_fill(px, mask, rng)
    # outline: every opaque pixel next to a transparent one
    edge = [(x, y) for y in range(N) for x in range(N) if mask.getpixel((x, y)) and any(
        not (0 <= x + dx < N and 0 <= y + dy < N) or not mask.getpixel((x + dx, y + dy))
        for dx, dy in ((1, 0), (-1, 0), (0, 1), (0, -1)))]
    for p in edge:
        px[p] = OUTLINE
    # the front vertical edge and the top's lower edges, slightly darker
    for y in range(top_y + 15, top_y + 14 + height):
        px[15, y] = (70, 48, 30, 255)


def flat_icon():
    img, d = canvas()
    iso_block(d, random.Random(3), 8, 5)
    save(img, "flat")


def generic_top(px, mask, rng):
    def top(x, y):
        return mask.getpixel((x, y)) == 3
    # pond on the right
    for y in range(N):
        for x in range(N):
            if top(x, y) and ((x - 20) ** 2) / 16 + ((y - 12) ** 2) / 4 <= 1:
                px[x, y] = (58, 108, 200, 255) if (x + y) % 5 else (96, 150, 228, 255)
            elif top(x, y) and 0.9 < ((x - 20) ** 2) / 25 + ((y - 12) ** 2) / 7 <= 1.5:
                px[x, y] = (222, 206, 150, 255)   # sand shore
    # tree on the left: trunk and a round crown
    for y in range(7, 12):
        px[11, y] = (102, 72, 40, 255)
        px[12, y] = (86, 60, 34, 255)
    crown = [(dx, dy) for dy in range(-4, 4) for dx in range(-5, 6) if dx * dx + dy * dy * 1.7 <= 22]
    for dx, dy in crown:
        x, y = 11 + dx, 4 + dy
        if 0 <= y < N:
            shade = (46, 112, 40) if dx + dy > 1 else (70, 146, 54) if dx + dy > -3 else (96, 170, 70)
            px[x, y] = shade + (255,)
    for dx, dy in crown:
        x, y = 11 + dx, 4 + dy
        if 0 <= y < N and any((ax, ay) not in crown for ax, ay in ((dx + 1, dy), (dx - 1, dy), (dx, dy + 1), (dx, dy - 1))):
            px[x, y] = (30, 70, 28, 255)
    # a flower
    px[7, 12] = (240, 70, 70, 255)
    px[14, 15] = (250, 230, 90, 255)


def generic_icon():
    img, d = canvas()
    iso_block(d, random.Random(5), 4, 13, generic_top)
    save(img, "generic")


# ── Moon and Mars ─────────────────────────────────────────────────────────────

def planet(name, features):
    img, d = canvas()
    r = 13.0
    cx = cy = 15.5
    px = img.load()
    rng = random.Random(name)
    for y in range(N):
        for x in range(N):
            dx, dy = x - cx, y - cy
            dist = math.hypot(dx, dy)
            if dist > r:
                continue
            col = features(x, y, dx / r, dy / r, rng)
            # light from the top left
            k = 1.15 - 0.55 * max(0.0, (dx + dy) / (r * 1.6) + 0.25)
            col = tuple(max(0, min(255, int(c * k))) for c in col)
            px[x, y] = col + (255,)
    # outline
    for y in range(N):
        for x in range(N):
            if px[x, y][3] and any(not px[x + ax, y + ay][3] for ax, ay in ((1, 0), (-1, 0), (0, 1), (0, -1))
                                   if 0 <= x + ax < N and 0 <= y + ay < N):
                px[x, y] = OUTLINE
    save(img, name)


MOON_CRATERS = [(-0.35, -0.3, 0.22), (0.3, 0.25, 0.28), (0.4, -0.45, 0.13), (-0.45, 0.4, 0.15), (0.05, 0.6, 0.1), (-0.05, -0.05, 0.1)]
MOON_MARIA = [(-0.2, -0.15, 0.45), (0.25, -0.1, 0.3)]


def moon_features(x, y, u, v, rng):
    col = (196, 196, 190)
    for mx, my, mr in MOON_MARIA:
        if (u - mx) ** 2 + (v - my) ** 2 < mr * mr:
            col = (140, 142, 144)
    for mx, my, mr in MOON_CRATERS:
        dd = math.hypot(u - mx, v - my) / mr
        if dd < 0.75:
            col = tuple(int(c * 0.78) for c in col)
        elif dd < 1.0:
            col = tuple(min(255, int(c * 1.12)) for c in col)
    return col


def mars_features(x, y, u, v, rng):
    col = (204, 98, 54)
    # darker regions and a canyon line
    if (u + 0.1) ** 2 * 2 + (v - 0.15) ** 2 * 6 < 0.35 or (u - 0.45) ** 2 + (v + 0.1) ** 2 < 0.08:
        col = (150, 70, 44)
    if abs(v - 0.05 - 0.15 * u) < 0.06 and -0.6 < u < 0.4:
        col = (120, 54, 36)
    if (u + 0.45) ** 2 + (v + 0.35) ** 2 < 0.03:
        col = (226, 140, 92)  # volcano
    if v < -0.66 + 0.08 * math.sin(u * 9):
        col = (240, 238, 232)  # polar cap
    if rng.random() < 0.08:
        col = tuple(int(c * 0.9) for c in col)
    return col


# ── World Tools menu ──────────────────────────────────────────────────────────

def outline(img):
    """Darken every opaque pixel that touches a transparent one."""
    px = img.load()
    edge = [(x, y) for y in range(N) for x in range(N) if px[x, y][3] and any(
        not (0 <= x + dx < N and 0 <= y + dy < N) or not px[x + dx, y + dy][3]
        for dx, dy in ((1, 0), (-1, 0), (0, 1), (0, -1)))]
    for p in edge:
        px[p] = OUTLINE


def tool_zones():
    # a flag planted in a fenced patch of grass
    img, d = canvas()
    d.polygon([(15.5, 14), (30, 21), (15.5, 28), (1, 21)], fill=(98, 160, 64))
    for i, (x, y) in enumerate([(15.5, 14), (30, 21), (15.5, 28), (1, 21), (15.5, 14)][:-1]):
        nx, ny = [(30, 21), (15.5, 28), (1, 21), (15.5, 14)][i]
        for t in range(0, 16):
            if t % 4 < 2:
                d.point((round(x + (nx - x) * t / 16), round(y + (ny - y) * t / 16)), fill=(250, 214, 70))
    outline(img)
    d.rectangle([14, 3, 15, 22], fill=(110, 80, 50))
    d.polygon([(16, 3), (27, 7), (16, 11)], fill=(233, 69, 96))
    d.line([(16, 10), (26, 7)], fill=(176, 40, 62))
    d.rectangle([13, 21, 16, 22], fill=(60, 44, 28))
    save(img, "tool_zones")


def tool_map():
    # a folded paper map with a dotted path to an X
    img, d = canvas()
    panels = [(2, 6, 11, 27), (11, 4, 21, 25), (21, 6, 30, 27)]
    colors = [(236, 222, 176), (214, 198, 150), (236, 222, 176)]
    for (x0, y0, x1, y1), col in zip(panels, colors):
        d.polygon([(x0, y0), (x1, y0 - 2 if x0 == 2 else y0 + 2), (x1, y1 - 2 if x0 == 2 else y1 + 2), (x0, y1)], fill=col)
    d.ellipse([4, 15, 10, 21], fill=(110, 170, 90))
    d.ellipse([22, 9, 28, 14], fill=(90, 140, 210))
    for x, y in [(6, 12), (8, 10), (10, 9), (12, 9), (14, 10), (16, 12), (18, 14), (20, 16), (22, 18)]:
        d.point((x, y), fill=(160, 60, 50))
    d.line([(22, 19), (26, 23)], fill=(210, 40, 40), width=2)
    d.line([(26, 19), (22, 23)], fill=(210, 40, 40), width=2)
    outline(img)
    save(img, "tool_map")


def tool_time():
    # sun behind a rain cloud
    img, d = canvas()
    for a in range(8):
        ang = a * math.pi / 4
        d.line([(12 + 7 * math.cos(ang), 11 + 7 * math.sin(ang)), (12 + 10 * math.cos(ang), 11 + 10 * math.sin(ang))],
               fill=(250, 200, 50), width=2)
    d.ellipse([6, 5, 18, 17], fill=(250, 204, 60))
    d.ellipse([8, 7, 13, 11], fill=(255, 236, 140))
    cloud = Image.new("RGBA", (N, N), (0, 0, 0, 0))
    cd = ImageDraw.Draw(cloud)
    for box_ in [(11, 15, 21, 24), (17, 11, 28, 22), (23, 16, 31, 24), (12, 19, 30, 25)]:
        cd.ellipse(box_, fill=(226, 232, 242))
    cd.ellipse([18, 12, 24, 17], fill=(248, 250, 255))
    outline(cloud)
    img.alpha_composite(cloud)
    for x, y in [(15, 27), (21, 28), (27, 27)]:
        d.line([(x, y), (x - 1, y + 2)], fill=(80, 140, 230), width=1)
    save(img, "tool_time")


def tool_rules():
    # clipboard with a checklist
    img, d = canvas()
    d.rectangle([5, 4, 26, 30], fill=(150, 104, 62))
    d.rectangle([8, 7, 23, 28], fill=(246, 244, 236))
    outline(img)
    d.rectangle([11, 2, 20, 6], fill=(150, 156, 170))
    d.rectangle([12, 3, 19, 5], fill=(190, 196, 210))
    for i, y in enumerate((11, 17, 23)):
        col = (60, 170, 80) if i < 2 else (210, 60, 60)
        if i < 2:
            d.line([(10, y), (11, y + 1), (14, y - 2)], fill=col, width=1)
        else:
            d.line([(10, y - 2), (13, y + 1)], fill=col)
            d.line([(13, y - 2), (10, y + 1)], fill=col)
        d.line([(16, y), (21, y)], fill=(120, 124, 140))
    save(img, "tool_rules")


def tool_settings():
    # a gear
    img, d = canvas()
    cx = cy = 15.5
    px = img.load()
    for y in range(N):
        for x in range(N):
            dx, dy = x - cx, y - cy
            r, ang = math.hypot(dx, dy), math.atan2(dy, dx)
            tooth = (math.cos(ang * 8) > 0.35)
            if 4.5 < r <= (13.5 if tooth else 10.5):
                k = 1.15 - 0.4 * max(0.0, (dx + dy) / 20 + 0.3)
                base = (150, 160, 180)
                px[x, y] = tuple(min(255, int(c * k)) for c in base) + (255,)
    outline(img)
    d.ellipse([11, 11, 20, 20], outline=OUTLINE)
    save(img, "tool_settings")


def europe_icon(data_dir):
    """Europe as seen on its map: land, sea and ice from countrygen's rasters."""
    import json

    import numpy as np
    meta = json.load(open(os.path.join(data_dir, "meta.json")))
    mat = np.fromfile(os.path.join(data_dir, "mat.bin"), np.uint8).reshape(meta["height"], meta["width"])
    names = meta["materials"]
    water = np.isin(mat, [names.index(n) for n in ("sea", "lake", "river")])
    snow = mat == names.index("snow")
    img, d = canvas()
    x0, y0, x1, y1 = 1, 4, 30, 27          # map area, inclusive
    w, h = x1 - x0 + 1, y1 - y0 + 1
    H, W = mat.shape
    sea, land, ice = (52, 98, 182), (96, 160, 70), (236, 240, 246)
    for j in range(h):
        for i in range(w):
            r0, r1 = j * H // h, (j + 1) * H // h
            c0, c1 = i * W // w, (i + 1) * W // w
            lf = 1 - water[r0:r1, c0:c1].mean()
            col = sea
            if lf > 0.45:
                col = ice if snow[r0:r1, c0:c1].mean() > 0.4 else land
            img.putpixel((x0 + i, y0 + j), col + (255,))
    d.rectangle([x0 - 1, y0 - 1, x1 + 1, y1 + 1], outline=OUTLINE)
    save(img, "europe")


def main():
    import sys
    os.makedirs(OUT, exist_ok=True)
    if "--europe" in sys.argv:
        europe_icon(sys.argv[sys.argv.index("--europe") + 1])
    tool_zones()
    tool_map()
    tool_time()
    tool_rules()
    tool_settings()
    flag("italy", [(1, (0, 146, 70)), (1, (244, 245, 240)), (1, (206, 43, 55))], vertical=True)
    flag("spain", [(1, (170, 21, 27)), (2, (241, 191, 0)), (1, (170, 21, 27))], extra=spain_arms)
    flag("hungary", [(1, (205, 42, 62)), (1, (244, 245, 240)), (1, (67, 111, 77))])
    flag("germany", [(1, (20, 20, 20)), (1, (221, 0, 0)), (1, (255, 206, 0))])
    flag("poland", [(1, (244, 245, 240)), (1, (220, 20, 60))])
    flat_icon()
    generic_icon()
    planet("moon", moon_features)
    planet("mars", mars_features)
    print("wrote", sorted(os.listdir(OUT)))


if __name__ == "__main__":
    main()
