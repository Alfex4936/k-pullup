# Weekly marker statistics

The main README chart is generated from the existing public endpoint:

`https://chulbong-kr.fly.dev/api/v1/markers/new-markers?lastMarkerID=0`

The API reads current markers from the database. The chart therefore does not depend on a local Bleve export, an index rebuild, or deployment. No database credentials or private repository files are used by GitHub Actions.

## Updates

`.github/workflows/update-marker-stats.yml` runs every Monday around **09:17 Asia/Seoul**, and can also be started with **Actions → Update marker statistics → Run workflow**. It also runs when the chart code changes on `main`. GitHub may delay scheduled runs. See [GitHub's scheduling documentation](https://docs.github.com/en/actions/reference/workflows-and-actions/events-that-trigger-workflows#schedule).

Only `coverage.svg`, `coverage.png`, and `summary.json` are committed by the workflow. `summary.json` contains aggregate regional counts; usernames, user IDs, individual addresses and coordinates are never persisted. If the API fails, returns empty/invalid data, repeats IDs, or more than half of the addresses cannot be classified, generation fails and the previously committed chart remains visible.

## What the chart counts

- One registered marker means one **location**, not the quantity of exercise equipment there.
- Percentages use **all marker locations** as the denominator, including missing/unknown addresses. The unknown count is shown explicitly.
- Province aliases and old names are normalized, including 전라북도 → 전북특별자치도 and 강원도 → 강원특별자치도.
- City/county rankings include `시` and `군` below the province level. Metropolitan cities belong in the province panel. District rankings include `구`, with parent names retained: 서울 중구 and 부산 중구 are separate.
- Numbers show contributed locations, not population coverage, exercise-equipment totals, or a complete census of public facilities.

## Reproduce locally

Run from the repository root with Python 3.11:

```sh
python -m pip install -r scripts/requirements-stats.txt
python -m unittest discover -s scripts -p 'test_marker_stats.py' -v
python scripts/marker_stats.py
```

For an existing local export: `python scripts/marker_stats.py --input /path/to/markers.json`. This reads only `markerId` and `address`. The font already exists in the public repository at `backend/resource/nanum.ttf`; the SVG embeds glyph outlines so Korean labels do not depend on the viewer's fonts.
