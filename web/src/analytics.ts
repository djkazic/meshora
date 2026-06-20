import { getPathHashStats, getHopDistribution, getCentrality, getRoutes, getGraph, getCritical } from "./api";
import type { HashSizeCount, HopBucket, CentralityRow, RouteRow, CriticalNode } from "./types";
import { esc } from "./util";
import { NetGraph } from "./netgraph";

const HASH_COLORS: Record<number, string> = {
  1: "#5ef38c",
  2: "#ffb000",
  3: "#ff5555",
};

const HOP_CAP = 10;

const PERIODS: [string, string][] = [
  ["1d", "1D"],
  ["7d", "7D"],
  ["", "ALL"],
];

const PERIOD_PANES = ["packets", "repeaters", "hops", "centrality", "routes"];

interface Slice {
  label: string;
  count: number;
  color: string;
}

export class AnalyticsView {
  private charts: Map<string, HTMLElement> = new Map();
  private graph: NetGraph | null = null;
  private periods: Map<string, string> = new Map();

  constructor(private onNode: (pubkey: string) => void) {
    for (const el of document.querySelectorAll<HTMLElement>("#analytics .an-chart")) {
      this.charts.set(el.dataset.chart!, el);
    }
    const host = this.charts.get("graph");
    if (host) this.graph = new NetGraph(host, onNode);
    for (const key of PERIOD_PANES) this.buildSelector(key);
  }

  async refresh(): Promise<void> {
    await Promise.all([
      ...PERIOD_PANES.map((key) => this.fetchPane(key)),
      getCritical()
        .then((c) => this.renderCritical(c))
        .catch(() => {}),
      getGraph()
        .then((g) => this.graph?.render(g))
        .catch(() => {}),
    ]);
  }

  private fetchPane(key: string): Promise<void> {
    const p = this.periods.get(key) ?? "";
    switch (key) {
      case "packets":
        return getPathHashStats(p).then((s) => this.renderPie("packets", s.packets)).catch(() => {});
      case "repeaters":
        return getPathHashStats(p).then((s) => this.renderPie("repeaters", s.repeaters)).catch(() => {});
      case "hops":
        return getHopDistribution(p).then((b) => this.renderHops(b)).catch(() => {});
      case "centrality":
        return getCentrality(p).then((r) => this.renderCentrality(r)).catch(() => {});
      case "routes":
        return getRoutes(p).then((r) => this.renderRoutes(r)).catch(() => {});
      default:
        return Promise.resolve();
    }
  }

  private buildSelector(key: string): void {
    const caption = this.charts.get(key)?.parentElement?.querySelector("figcaption");
    if (!caption) return;
    const sel = document.createElement("span");
    sel.className = "an-period";
    sel.innerHTML = PERIODS.map(([v, label]) => `<button data-p="${v}"${v === "" ? ' class="on"' : ""}>${label}</button>`).join("");
    caption.appendChild(sel);
    sel.querySelectorAll<HTMLButtonElement>("button").forEach((btn) => {
      btn.addEventListener("click", () => {
        this.periods.set(key, btn.dataset.p!);
        sel.querySelectorAll("button").forEach((b) => b.classList.toggle("on", b === btn));
        this.fetchPane(key);
      });
    });
  }

  deactivate(): void {
    this.graph?.stop();
  }

  private renderPie(key: string, data: HashSizeCount[]): void {
    const el = this.charts.get(key);
    if (!el) return;
    const slices: Slice[] = data.map((d) => ({
      label: `${d.size}-byte`,
      count: d.count,
      color: HASH_COLORS[d.size] ?? "#7e8b9c",
    }));
    const total = slices.reduce((sum, s) => sum + s.count, 0);
    if (total === 0) {
      el.innerHTML = `<div class="an-empty">no data yet</div>`;
      return;
    }
    el.innerHTML = pie(slices, total) + legend(slices, total) + `<div class="an-total">${total.toLocaleString()} total</div>`;
  }

  private renderHops(buckets: HopBucket[]): void {
    const el = this.charts.get("hops");
    if (!el) return;
    if (buckets.length === 0) {
      el.innerHTML = `<div class="an-empty">no data yet</div>`;
      return;
    }
    const merged = new Map<number, number>();
    for (const b of buckets) {
      const k = Math.min(b.hops, HOP_CAP);
      merged.set(k, (merged.get(k) ?? 0) + b.count);
    }
    const keys = [...merged.keys()].sort((a, b) => a - b);
    const max = Math.max(...merged.values());
    const total = [...merged.values()].reduce((s, n) => s + n, 0);
    const rows = keys
      .map((k) => {
        const count = merged.get(k)!;
        const label = k >= HOP_CAP ? `${HOP_CAP}+` : String(k);
        const w = count > 0 ? Math.max(1.5, (count / max) * 100) : 0;
        return (
          `<div class="an-bar-row"><span class="an-bar-k">${label}</span>` +
          `<span class="an-bar-track"><i style="width:${w.toFixed(1)}%"></i></span>` +
          `<span class="an-bar-v">${count.toLocaleString()}</span></div>`
        );
      })
      .join("");
    el.innerHTML = `<div class="an-bars">${rows}</div>` + `<div class="an-total">${total.toLocaleString()} packets</div>`;
  }

  private renderCentrality(data: CentralityRow[]): void {
    const el = this.charts.get("centrality");
    if (!el) return;
    if (data.length === 0) {
      el.innerHTML = `<div class="an-empty">no data yet</div>`;
      return;
    }
    const rows = data
      .map((r, i) => {
        const name = r.name || r.pubkey.slice(0, 10);
        return (
          `<tr data-pk="${esc(r.pubkey)}"><td class="an-rank">${i + 1}</td>` +
          `<td class="an-ct-name" title="${esc(name)}">${esc(name)}</td>` +
          `<td class="an-ct-n">${r.path_count.toLocaleString()}</td>` +
          `<td class="an-ct-p">${r.percentile.toFixed(1)}</td></tr>`
        );
      })
      .join("");
    el.innerHTML =
      `<div class="an-ct-wrap"><table class="an-ctab">` +
      `<thead><tr><th>#</th><th>repeater</th><th>paths</th><th>pctl</th></tr></thead>` +
      `<tbody>${rows}</tbody></table></div>` +
      `<div class="an-note">observed relay participation, skewed by observer coverage. not physical reach</div>`;
    el.querySelectorAll<HTMLElement>("tr[data-pk]").forEach((tr) => {
      tr.onclick = () => this.onNode(tr.dataset.pk!);
    });
  }

  private renderRoutes(data: RouteRow[]): void {
    const el = this.charts.get("routes");
    if (!el) return;
    if (data.length === 0) {
      el.innerHTML = `<div class="an-empty">no data yet</div>`;
      return;
    }
    const rows = data
      .map((r, i) => {
        const chain = r.hops.map((h) => h.name || h.pubkey.slice(0, 6)).join(" → ");
        return (
          `<tr><td class="an-rank">${i + 1}</td>` +
          `<td class="an-ct-name" title="${esc(chain)}">${esc(chain)}</td>` +
          `<td class="an-ct-n">${r.hops.length}</td>` +
          `<td class="an-ct-p">${r.count.toLocaleString()}</td></tr>`
        );
      })
      .join("");
    el.innerHTML =
      `<div class="an-ct-wrap"><table class="an-ctab">` +
      `<thead><tr><th>#</th><th>route</th><th>hops</th><th>freq</th></tr></thead>` +
      `<tbody>${rows}</tbody></table></div>`;
  }

  private renderCritical(data: CriticalNode[]): void {
    const el = this.charts.get("critical");
    if (!el) return;
    if (data.length === 0) {
      el.innerHTML = `<div class="an-empty">no cut vertices</div>`;
      return;
    }
    const rows = data
      .map((c, i) => {
        const name = c.name || c.pubkey.slice(0, 10);
        return (
          `<tr data-pk="${esc(c.pubkey)}"><td class="an-rank">${i + 1}</td>` +
          `<td class="an-ct-name" title="${esc(name)}">${esc(name)}</td>` +
          `<td class="an-ct-n">${c.fragments}</td>` +
          `<td class="an-ct-p">${c.isolated}</td></tr>`
        );
      })
      .join("");
    el.innerHTML =
      `<div class="an-ct-wrap"><table class="an-ctab">` +
      `<thead><tr><th>#</th><th>repeater</th><th>splits</th><th>cut off</th></tr></thead>` +
      `<tbody>${rows}</tbody></table></div>` +
      `<div class="an-note">cut vertices in observed paths, not physical chokepoints. the mesh likely has redundant links we never saw</div>`;
    el.querySelectorAll<HTMLElement>("tr[data-pk]").forEach((tr) => {
      tr.onclick = () => this.onNode(tr.dataset.pk!);
    });
  }
}

function pie(slices: Slice[], total: number): string {
  const cx = 90;
  const cy = 90;
  const r = 80;
  const drawn = slices.filter((s) => s.count > 0);
  let segs = "";
  if (drawn.length === 1) {
    segs = `<circle cx="${cx}" cy="${cy}" r="${r}" fill="${drawn[0].color}" />`;
  } else {
    let a = -Math.PI / 2;
    for (const s of drawn) {
      const frac = s.count / total;
      const a1 = a + frac * Math.PI * 2;
      const [x0, y0] = point(cx, cy, r, a);
      const [x1, y1] = point(cx, cy, r, a1);
      const large = frac > 0.5 ? 1 : 0;
      segs += `<path d="M${cx},${cy} L${x0},${y0} A${r},${r} 0 ${large} 1 ${x1},${y1} Z" fill="${s.color}" />`;
      a = a1;
    }
  }
  return `<svg viewBox="0 0 180 180" class="an-pie">${segs}</svg>`;
}

function legend(slices: Slice[], total: number): string {
  const rows = slices
    .map((s) => {
      const pct = ((s.count / total) * 100).toFixed(1);
      return (
        `<div class="an-leg"><i style="background:${s.color};color:${s.color}"></i>` +
        `<span class="an-leg-l">${s.label}</span>` +
        `<span class="an-leg-v">${s.count.toLocaleString()} · ${pct}%</span></div>`
      );
    })
    .join("");
  return `<div class="an-legend">${rows}</div>`;
}

function point(cx: number, cy: number, r: number, angle: number): [string, string] {
  return [(cx + r * Math.cos(angle)).toFixed(2), (cy + r * Math.sin(angle)).toFixed(2)];
}
