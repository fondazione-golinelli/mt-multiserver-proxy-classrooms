#!/usr/bin/env python3
"""Draw the map template icons of the template picker (icons/*.png).

32x32 pixel art, saved at 64x64 (nearest neighbour) so the client keeps the
pixels sharp. The plugin embeds the files and sends them inline as
"[png:<base64>" textures, so they need no media on the servers.

  python3 tools/make_icons.py          (needs Pillow)
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


def main():
    os.makedirs(OUT, exist_ok=True)
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
