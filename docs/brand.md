# Brand

## Logo

<img src="assets/img/logo.svg" alt="Conduit logo" width="96" height="96">

The mark is five concentric arcs forming a "C" that opens toward the
direction of flow. The colors run from red on the outside to yellow at the
center, a nod to data moving through a conduit.

| File | Use |
|------|-----|
| `ui/static/img/logo.svg` | The mark on light or dark backgrounds, at 24 px and up |
| `ui/static/img/favicon.svg` | The mark on a white rounded tile, for browser tabs and app icons |

Pair the mark with the word **Conduit** set in the UI's sans-serif at 700
weight. Leave clear space of at least a quarter of the mark's width around
it. Don't recolor, rotate, stretch or outline the mark.

## Palette

| | Hex | RGB | Role |
|---|-----|-----|------|
| ![](https://img.shields.io/badge/-%20%20%20%20-ec0000) | `#ec0000` | 236, 0, 0 | Primary: links, primary buttons, errors |
| ![](https://img.shields.io/badge/-%20%20%20%20-ec2400) | `#ec2400` | 236, 36, 0 | Gradient and accents |
| ![](https://img.shields.io/badge/-%20%20%20%20-ec5300) | `#ec5300` | 236, 83, 0 | Accents, chart series |
| ![](https://img.shields.io/badge/-%20%20%20%20-ec9b00) | `#ec9b00` | 236, 155, 0 | Waterfall bars, warnings, highlights |
| ![](https://img.shields.io/badge/-%20%20%20%20-ecca00) | `#ecca00` | 236, 202, 0 | Tags and highlights |
| ![](https://img.shields.io/badge/-%20%20%20%20-ffffff) | `#ffffff` | 255, 255, 255 | Surfaces |

**Supporting neutrals,** needed for legible text since the palette has no dark color:
- `#1c1917` ink, for body text and the nav bar
- `#57534e` secondary text
- `#e7e5e4` borders
- `#fafaf9` page background

The brand gradient runs through all five colors from left to right. Use it
only for the nav rule, card accents and the logo, never behind text.

## Accessibility rules

These come from measured contrast ratios (WCAG 2.1), and the UI follows them.

| Combination | Ratio | Rule |
|-------------|-------|------|
| `#ec0000` on white | 4.6:1 | **The only palette color allowed as body text on white.** |
| White on `#ec0000` | 4.6:1 | OK for button labels |
| `#ec2400` / `#ec5300` on white | 4.4 / 3.6:1 | Large text (18 px and up) and graphics only |
| `#ec9b00` / `#ecca00` on white | 2.3 / 1.6:1 | **Never** for text. Use them as fills with ink text on top (7.7 / 10.8:1). |
| Error text on the red tint `#fef2f2` | | Use `#b80000` (6.3:1). `#ec0000` drops to 4.2:1 there. |
| `#57534e` on white | 7.6:1 | Secondary text |

Never rely on color alone for status. Status badges always include a word
(`OK`, `ERROR`), and error rows also get a red edge.
