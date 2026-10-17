"""Generate README charts from the public marker API; never persist raw records."""

import argparse
from collections import Counter
from datetime import datetime, timedelta, timezone
import json
from pathlib import Path
import re
import time
import unicodedata
from urllib.error import HTTPError, URLError
from urllib.request import Request, urlopen


ROOT = Path(__file__).resolve().parents[1]
SOURCE = "https://chulbong-kr.fly.dev/api/v1/markers/new-markers?lastMarkerID=0"
KST = timezone(timedelta(hours=9))
PROVINCES = {
    "서울특별시": ("서울", "서울시"),
    "부산광역시": ("부산", "부산시"),
    "대구광역시": ("대구", "대구시"),
    "인천광역시": ("인천", "인천시"),
    "광주광역시": ("광주", "광주시"),
    "대전광역시": ("대전", "대전시"),
    "울산광역시": ("울산", "울산시"),
    "세종특별자치시": ("세종", "세종시"),
    "경기도": ("경기",),
    "강원특별자치도": ("강원", "강원도"),
    "충청북도": ("충북",),
    "충청남도": ("충남",),
    "전북특별자치도": ("전북", "전라북도"),
    "전라남도": ("전남",),
    "경상북도": ("경북",),
    "경상남도": ("경남",),
    "제주특별자치도": ("제주", "제주도"),
}
ALIASES = {alias: name for name, aliases in PROVINCES.items() for alias in (name, *aliases)}
SHORT = {name: aliases[0] for name, aliases in PROVINCES.items()}


def classify_address(address):
    """Keep administrative levels and parent names; do not count roads as districts."""
    if not isinstance(address, str):
        return None, None, None
    parts = unicodedata.normalize("NFKC", address).split()
    if not parts or parts[0] not in ALIASES:
        return None, None, None
    province = ALIASES[parts[0]]
    city = district = None
    if len(parts) > 1:
        local = parts[1]
        if re.fullmatch(r"[가-힣]+[시군]", local) and province != "세종특별자치시":
            city = f"{SHORT[province]} {local}"
            if len(parts) > 2 and re.fullmatch(r"[가-힣]+구", parts[2]):
                district = f"{SHORT[province]} {local} {parts[2]}"
        elif re.fullmatch(r"[가-힣]+구", local):
            district = f"{SHORT[province]} {local}"
    return province, city, district


def summarize(records, date):
    if not isinstance(records, list) or not records:
        raise ValueError("Expected a non-empty array of markers; keeping the previous chart")
    seen = set()
    provinces, cities, districts = Counter(), Counter(), Counter()
    unclassified = 0
    for record in records:
        if not isinstance(record, dict):
            raise ValueError("Invalid marker record")
        marker_id = record.get("markerId")
        if type(marker_id) is not int or marker_id <= 0:
            raise ValueError("Marker IDs must be positive integers")
        if marker_id in seen:
            raise ValueError(f"Duplicate marker ID: {marker_id}; refusing an ambiguous snapshot")
        seen.add(marker_id)
        province, city, district = classify_address(record.get("address"))
        if province:
            provinces[province] += 1
        else:
            unclassified += 1
        if city:
            cities[city] += 1
        if district:
            districts[district] += 1
    if sum(provinces.values()) < len(seen) / 2:
        raise ValueError("Fewer than half the addresses are recognizable; keeping the previous chart")
    ordered = lambda counter: dict(sorted(counter.items(), key=lambda pair: (-pair[1], pair[0])))
    return {
        "date_kst": date,
        "source": SOURCE,
        "total_locations": len(seen),
        "unclassified_addresses": unclassified,
        "provinces": ordered(provinces),
        "cities_and_counties": ordered(cities),
        "districts": ordered(districts),
    }


def fetch_records(url=SOURCE):
    request = Request(url, headers={"User-Agent": "k-pullup-readme-stats/1.0", "Accept": "application/json"})
    for attempt in range(3):
        try:
            with urlopen(request, timeout=30) as response:
                content = response.read(20 * 1024 * 1024 + 1)
            if len(content) > 20 * 1024 * 1024:
                raise ValueError("Marker response exceeds 20 MiB")
            return json.loads(content)
        except (HTTPError, URLError, TimeoutError) as error:
            if isinstance(error, HTTPError) and error.code not in (429, 500, 502, 503, 504):
                raise
            if attempt == 2:
                raise
            time.sleep(2 ** attempt)


def render_chart(summary, output, font):
    import matplotlib

    matplotlib.use("Agg")
    import matplotlib.pyplot as plt
    from matplotlib import font_manager
    from matplotlib.patches import FancyBboxPatch

    font_manager.fontManager.addfont(str(font))
    family = font_manager.FontProperties(fname=font).get_name()
    plt.rcParams.update({
        "font.family": family,
        "font.size": 11,
        "axes.unicode_minus": False,
        "svg.fonttype": "path",
        "svg.hashsalt": "k-pullup-statistics",
    })
    bg, ink, muted, accent = "#F4F6F5", "#182C27", "#64766F", "#15805E"
    fig = plt.figure(figsize=(16, 11.5), facecolor=bg)
    text = lambda x, y, value, **kwargs: fig.text(x, y, value, color=kwargs.pop("color", ink), **kwargs)
    text(0.045, 0.946, "K-PULLUP  /  COMMUNITY ATLAS", size=12, weight="bold", color=accent)
    text(0.045, 0.894, "전국의 철봉을 한눈에", size=29, weight="bold")
    text(0.955, 0.944, f"{summary['date_kst']} KST", size=11, ha="right", color=muted)
    text(0.955, 0.918, "매주 월요일 업데이트", size=10, ha="right", color=muted)
    total = summary["total_locations"]
    unknown = summary["unclassified_addresses"]
    coverage = (total - unknown) / total * 100
    for x, value, label in (
        (0.045, f"{total:,}", "등록된 운동 장소"),
        (0.37, f"{len(summary['provinces'])} / 17", "데이터가 등록된 시·도"),
        (0.68, f"{coverage:.2f}%", "주소 분류율"),
    ):
        text(x, 0.808, value, size=34, weight="bold")
        text(x, 0.775, label, size=12, color=muted)

    def panel(bounds):
        x, y, w, h = bounds
        fig.add_artist(FancyBboxPatch((x, y), w, h, boxstyle="round,pad=0.008,rounding_size=0.012",
                                     transform=fig.transFigure, facecolor="white", edgecolor="none", zorder=0))

    panel((0.035, 0.115, 0.44, 0.59))
    panel((0.505, 0.425, 0.46, 0.28))
    panel((0.505, 0.115, 0.46, 0.28))
    text(0.055, 0.672, "시·도별 등록 현황", size=17, weight="bold")
    text(0.455, 0.673, "장소 수 · 전국 비율", size=10, ha="right", color=muted)
    text(0.525, 0.672, "시·군 TOP 10", size=16, weight="bold")
    text(0.525, 0.362, "구 TOP 10", size=16, weight="bold")
    text(0.945, 0.672, "광역시는 시·도에 집계", size=9, ha="right", color=muted)
    text(0.945, 0.362, "같은 이름도 지역별로 구분", size=9, ha="right", color=muted)

    def bars(bounds, counts, limit, labels_x, values_x, small=False):
        entries = list(counts.items())[:limit]
        ax = fig.add_axes(bounds, facecolor="white")
        if not entries:
            ax.text(0.5, 0.5, "집계 가능한 주소 없음", ha="center", color=muted)
            ax.set_axis_off()
            return
        maximum = entries[0][1]
        for index, (name, count) in enumerate(entries):
            color = accent if index < 3 else "#C5D8D0"
            ax.barh(index, count, height=0.51 if small else 0.58, color=color, zorder=2)
            y = 1 - (index + 0.5) / len(entries)
            ax.text(labels_x, y, name, transform=ax.transAxes, ha="right", va="center",
                    color=ink, size=9.4 if small else 11)
            ax.text(values_x, y, f"{count:,}   {count / total:4.1%}", transform=ax.transAxes,
                    ha="right", va="center", color=ink, size=9.5 if small else 11)
        ax.set_ylim(len(entries) - 0.5, -0.5)
        ax.set_xlim(0, maximum * 1.08)
        ax.set_axis_off()

    bars((0.178, 0.141, 0.175, 0.493), summary["provinces"], 17, -0.07, 1.6)
    bars((0.735, 0.448, 0.1, 0.184), summary["cities_and_counties"], 10, -0.12, 2.05, True)
    bars((0.735, 0.138, 0.1, 0.184), summary["districts"], 10, -0.12, 2.05, True)
    text(0.045, 0.071, "비율은 전체 등록 장소 기준 · 장소 수는 기구 수량과 다릅니다.", size=10, color=muted)
    text(0.045, 0.044, f"주소 미분류 {unknown:,}곳도 전체에 포함합니다.  |  출처: k-pullup 공개 API", size=10, color=muted)
    text(0.955, 0.044, "k-pullup.com", size=12, ha="right", weight="bold", color=accent)
    output.mkdir(parents=True, exist_ok=True)
    svg = output / "coverage.svg"
    fig.savefig(svg, metadata={"Date": None, "Creator": "k-pullup"})
    svg.write_text("\n".join(line.rstrip() for line in svg.read_text(encoding="utf-8").splitlines()) + "\n", encoding="utf-8")
    fig.savefig(output / "coverage.png", dpi=120, metadata={"Software": "k-pullup"})
    plt.close(fig)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--input", type=Path, help="Optional local markers JSON for offline reproduction")
    parser.add_argument("--output", type=Path, default=ROOT / "docs/statistics")
    parser.add_argument("--font", type=Path, default=ROOT / "backend/resource/nanum.ttf")
    parser.add_argument("--date", default=datetime.now(KST).date().isoformat())
    args = parser.parse_args()
    records = json.loads(args.input.read_text(encoding="utf-8")) if args.input else fetch_records()
    summary = summarize(records, args.date)
    render_chart(summary, args.output, args.font)
    (args.output / "summary.json").write_text(json.dumps(summary, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(f"Generated chart: {summary['total_locations']:,} locations; {summary['unclassified_addresses']} unclassified addresses")


if __name__ == "__main__":
    main()
