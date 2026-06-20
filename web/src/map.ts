import L from "leaflet";
import "leaflet/dist/leaflet.css";
import type { Config, NodeT } from "./types";
import { roleColor } from "./palette";
import { esc } from "./util";

function tooltipHTML(n: NodeT): string {
  const name = n.name ? esc(n.name) : "(unnamed)";
  return `<b>${name}</b><br>${esc(n.role || "?")}`;
}

export class MeshMap {
  readonly map: L.Map;
  private markers = new Map<string, L.CircleMarker>();
  private roles = new Map<string, string>();
  private nodes = new Map<string, NodeT>();

  onNodeClick: ((pubkey: string) => void) | null = null;

  constructor(cfg: Config) {
    const coarse = window.matchMedia("(pointer: coarse)").matches;
    this.map = L.map("map", {
      preferCanvas: true,
      renderer: L.canvas({ tolerance: coarse ? 14 : 4 }),
      zoomControl: true,
    }).setView([cfg.center.lat, cfg.center.lon], 11);
    L.tileLayer(
      "https://{s}.basemaps.cartocdn.com/dark_all/{z}/{x}/{y}{r}.png",
      {
        attribution:
          '&copy; <a href="https://www.openstreetmap.org/copyright">OSM</a> &copy; <a href="https://carto.com/attributions">CARTO</a>',
        subdomains: "abcd",
        maxZoom: 19,
      },
    ).addTo(this.map);
    this.map.fitBounds([
      [cfg.bbox.minLat, cfg.bbox.minLon],
      [cfg.bbox.maxLat, cfg.bbox.maxLon],
    ]);
    this.map.on("zoomend", () => this.recomputeRadii());
  }

  private radiusFor(role: string): number {
    const z = this.map.getZoom();
    const base = role === "repeater" || role === "room" ? 2.6 : 1.8;
    const scale = z >= 13 ? 1.25 : z >= 11 ? 1.0 : z >= 9 ? 0.78 : 0.6;
    return Math.max(1.4, base * scale);
  }

  private recomputeRadii(): void {
    for (const [pk, m] of this.markers) {
      m.setRadius(this.radiusFor(this.roles.get(pk) || ""));
    }
  }

  upsertNode(n: NodeT): void {
    if (n.lat == null || n.lon == null) return;
    this.roles.set(n.pubkey, n.role);
    this.nodes.set(n.pubkey, n);
    const color = roleColor(n.role);
    const existing = this.markers.get(n.pubkey);
    if (existing) {
      existing.setLatLng([n.lat, n.lon]);
      existing.setStyle({ color, fillColor: color });
      existing.setRadius(this.radiusFor(n.role));
      existing.setTooltipContent(tooltipHTML(n));
      return;
    }
    const m = L.circleMarker([n.lat, n.lon], {
      radius: this.radiusFor(n.role),
      color,
      fillColor: color,
      fillOpacity: 0.7,
      weight: 1,
    }).addTo(this.map);
    m.bindTooltip(tooltipHTML(n), { direction: "top", opacity: 0.9 });
    m.on("click", () => this.onNodeClick?.(n.pubkey));
    this.markers.set(n.pubkey, m);
  }

  nodeCount(): number {
    return this.markers.size;
  }

  search(query: string, limit: number): NodeT[] {
    const q = query.toLowerCase();
    const out: NodeT[] = [];
    for (const n of this.nodes.values()) {
      if (n.name.toLowerCase().includes(q) || n.pubkey.toLowerCase().startsWith(q)) {
        out.push(n);
        if (out.length >= limit * 3) break;
      }
    }
    out.sort((a, b) => {
      const as = a.name.toLowerCase().startsWith(q) ? 0 : 1;
      const bs = b.name.toLowerCase().startsWith(q) ? 0 : 1;
      return as - bs || a.name.localeCompare(b.name);
    });
    return out.slice(0, limit);
  }

  focus(pubkey: string): void {
    const n = this.nodes.get(pubkey);
    if (!n || n.lat == null || n.lon == null) return;
    this.map.setView([n.lat, n.lon], Math.max(this.map.getZoom(), 14));
    this.markers.get(pubkey)?.openTooltip();
  }
}
