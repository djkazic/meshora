import type { FlowRec, NodeDetail, NodePath, PacketDetail } from "./types";
import { getNode, getNodePaths, getPacket } from "./api";
import { payloadColor, roleColor } from "./palette";
import { clamp, clock, esc, roleAbbrev } from "./util";

let winZ = 4000;
let cascade = 0;
const openWins: Shell[] = [];

window.addEventListener("keydown", (e) => {
  if (e.key !== "Escape" || !openWins.length) return;
  let top = openWins[0];
  for (const w of openWins) {
    if ((+w.root.style.zIndex || 0) >= (+top.root.style.zIndex || 0)) top = w;
  }
  top.destroy();
});

interface Shell {
  root: HTMLElement;
  panel: HTMLElement;
  destroy(): void;
}

function makeShell(): Shell {
  const root = document.createElement("div");
  root.className = "win show";
  root.innerHTML = `<div class="pm-panel"></div>`;
  document.body.appendChild(root);
  const panel = root.querySelector(".pm-panel") as HTMLElement;

  const mobile = window.innerWidth < 640;
  const off = mobile ? 0 : (cascade++ % 5) * 26;
  const panelW = Math.min(560, window.innerWidth * 0.92);
  const left = Math.max(8, Math.min(window.innerWidth - panelW - 8, (window.innerWidth - panelW) / 2 + off));
  const hud = document.getElementById("hud");
  const top = mobile && hud ? hud.getBoundingClientRect().bottom + 8 : 60 + off;
  panel.style.transform = "none";
  panel.style.left = `${left}px`;
  panel.style.top = `${top}px`;

  const toFront = () => {
    root.style.zIndex = String(++winZ);
  };
  toFront();

  let dragging = false;
  let sx = 0,
    sy = 0,
    ox = 0,
    oy = 0;
  panel.addEventListener("pointerdown", (e) => {
    toFront();
    const t = e.target as HTMLElement;
    if (!t.closest(".pm-head") || t.closest(".pm-close")) return;
    dragging = true;
    panel.setPointerCapture(e.pointerId);
    const r = panel.getBoundingClientRect();
    panel.style.left = `${r.left}px`;
    panel.style.top = `${r.top}px`;
    sx = e.clientX;
    sy = e.clientY;
    ox = r.left;
    oy = r.top;
  });
  panel.addEventListener("pointermove", (e) => {
    if (!dragging) return;
    panel.style.left = `${clamp(ox + e.clientX - sx, 0, window.innerWidth - 80)}px`;
    panel.style.top = `${clamp(oy + e.clientY - sy, 0, window.innerHeight - 40)}px`;
  });
  const end = () => (dragging = false);
  panel.addEventListener("pointerup", end);
  panel.addEventListener("pointercancel", end);

  const shell: Shell = {
    root,
    panel,
    destroy: () => {
      const i = openWins.indexOf(shell);
      if (i >= 0) openWins.splice(i, 1);
      root.remove();
    },
  };
  openWins.push(shell);
  return shell;
}

export class PacketWindow {
  private shell = makeShell();

  constructor(
    hash: string,
    private replayRec: FlowRec | null,
    private onReplay: (rec: FlowRec) => void,
    private onNode: (pubkey: string) => void,
  ) {
    this.shell.panel.innerHTML = headLoad("PACKET");
    this.wireClose();
    this.load(hash);
  }

  private wireClose(): void {
    this.shell.panel.querySelector(".pm-close")!.addEventListener("click", () => this.shell.destroy());
  }

  private async load(hash: string): Promise<void> {
    try {
      this.render(await getPacket(hash));
    } catch (e) {
      this.shell.panel.querySelector(".pm-load")!.textContent = `no detail available: ${String(e)}`;
    }
  }

  private render(d: PacketDetail): void {
    const color = payloadColor(d.payload_type);

    const hopChip = (h: { pubkey: string; name: string; role: string }): string => {
      const label = h.name || h.pubkey;
      const ab = roleAbbrev(h.role);
      const tag = ab ? ` <i class="pm-hoprole">(${ab})</i>` : "";
      return `<span class="pm-hop pm-hop-link" data-pk="${esc(h.pubkey)}" title="${esc(h.pubkey)}">${esc(label)}${tag}</span>`;
    };
    const path = d.resolved.length
      ? d.resolved.map(hopChip).join('<span class="pm-arr">→</span>')
      : d.hops.length
        ? d.hops.map((h) => `<span class="pm-hop">${esc(h)}</span>`).join('<span class="pm-arr">→</span>')
        : '<span class="pm-dim">— no relays (direct / origin) —</span>';

    const advert = d.advert
      ? `<div class="pm-row"><label>ADVERT</label><span class="pm-hop-link" data-pk="${esc(d.advert.pubkey)}">${esc(d.advert.name || "(unnamed)")} · ${esc(d.advert.role)}${
          d.advert.lat != null ? ` · ${d.advert.lat.toFixed(4)}, ${d.advert.lon!.toFixed(4)}` : ""
        }</span></div>`
      : "";
    const message = d.message
      ? `<div class="pm-row"><label>${esc(d.channel || "CHAT")}</label><span class="pm-msg">${esc(d.message)}</span></div>`
      : "";

    const hasSnr = d.observers.some((o) => o.snr != null);
    const hasRssi = d.observers.some((o) => o.rssi != null);
    const cols = 2 + (hasSnr ? 1 : 0) + (hasRssi ? 1 : 0);
    const head = `<th>station</th>${hasSnr ? "<th>snr</th>" : ""}${hasRssi ? "<th>rssi</th>" : ""}<th>time</th>`;
    const obs = d.observers
      .map((o) => {
        const name = esc(o.observer_name || o.observer_id.slice(0, 8));
        const station = o.count > 1 ? `${name} <i class="pm-obscount">×${o.count}</i>` : name;
        return (
          `<tr><td>${station}</td>` +
          (hasSnr ? `<td>${o.snr != null ? o.snr.toFixed(1) : "··"}</td>` : "") +
          (hasRssi ? `<td>${o.rssi != null ? o.rssi.toFixed(0) : "··"}</td>` : "") +
          `<td>${clock(o.ts * 1000)}</td></tr>`
        );
      })
      .join("");

    this.shell.panel.innerHTML = `
      <div class="pm-head" style="border-bottom-color:${color}">
        <span class="pm-type" style="color:${color}">${esc(d.payload_name)}</span>
        <span class="pm-route">${esc(d.route_name)}${d.payload_version != null ? ` · v${d.payload_version}` : ""}</span>
        <button class="pm-close" title="close (Esc)">✕</button>
      </div>
      <div class="pm-body">
        <div class="pm-row"><label>HEARD</label><span>${fmtDate(d.first_seen)} · ${d.observation_count} obs</span></div>
        <div class="pm-row"><label>HASH</label><span class="pm-mono">${esc(d.hash)}</span></div>
        ${advert}
        ${message}
        <div class="pm-row pm-pathrow"><label>PATH</label><span class="pm-path">${path}</span></div>
        <div class="pm-sec">OBSERVERS</div>
        <table class="pm-obs"><thead><tr>${head}</tr></thead>
          <tbody>${obs || `<tr><td colspan="${cols}" class="pm-dim">none</td></tr>`}</tbody></table>
        <div class="pm-sec">RAW</div>
        <div class="pm-raw">${esc(d.raw_hex)}</div>
      </div>
      ${this.replayRec ? '<div class="pm-foot"><button class="pm-replay">⟲ REPLAY ON MAP</button></div>' : ""}`;

    this.wireClose();
    this.shell.panel.querySelector(".pm-replay")?.addEventListener("click", () => {
      const rec = this.replayRec;
      this.shell.destroy();
      if (rec) this.onReplay(rec);
    });
    this.shell.panel.querySelectorAll<HTMLElement>(".pm-hop-link").forEach((el) =>
      el.addEventListener("click", () => this.onNode(el.dataset.pk!)),
    );
  }
}

export class NodeWindow {
  private shell = makeShell();

  constructor(
    private pubkey: string,
    private onPin: (lat: number, lon: number) => void,
    private onNode: (pubkey: string) => void,
  ) {
    this.shell.panel.innerHTML = headLoad("NODE");
    this.shell.panel.querySelector(".pm-close")!.addEventListener("click", () => this.shell.destroy());
    this.load(pubkey);
  }

  private async load(pubkey: string): Promise<void> {
    try {
      this.render(await getNode(pubkey));
      this.loadPaths();
    } catch (e) {
      this.shell.panel.querySelector(".pm-load")!.textContent = `no node detail: ${String(e)}`;
    }
  }

  private async loadPaths(): Promise<void> {
    const el = this.shell.panel.querySelector(".pm-paths");
    if (!el) return;
    try {
      const paths = await getNodePaths(this.pubkey);
      if (!paths.length) {
        el.innerHTML = '<span class="pm-dim">— none observed —</span>';
        return;
      }
      el.innerHTML = paths.map((p) => this.pathLine(p)).join("");
      el.querySelectorAll<HTMLElement>(".pm-hop-link").forEach((x) =>
        x.addEventListener("click", () => this.onNode(x.dataset.pk!)),
      );
    } catch {
      el.innerHTML = '<span class="pm-dim">paths unavailable</span>';
    }
  }

  private pathLine(p: NodePath): string {
    const hops = p.hops
      .map((h, i) => {
        const indent = Math.min(i, 10) * 14;
        const conn = i ? '<span class="pm-pathconn">↳</span>' : "";
        return `<div class="pm-pathhop" style="margin-left:${indent}px">${conn}${this.pathChip(h)}</div>`;
      })
      .join("");
    return `<div class="pm-pathblock"><span class="pm-pathcount">×${p.count} packets</span>${hops}</div>`;
  }

  private pathChip(h: { pubkey: string; name: string; role: string }): string {
    const self = h.pubkey.toLowerCase() === this.pubkey.toLowerCase();
    const ab = roleAbbrev(h.role);
    const tag = ab ? ` <i class="pm-hoprole">(${ab})</i>` : "";
    const label = h.name || h.pubkey;
    if (self) return `<span class="pm-hop pm-hop-self" title="${esc(h.pubkey)}">${esc(label)}${tag}</span>`;
    return `<span class="pm-hop pm-hop-link" data-pk="${esc(h.pubkey)}" title="${esc(h.pubkey)}">${esc(label)}${tag}</span>`;
  }

  private render(d: NodeDetail): void {
    const color = roleColor(d.role);
    const positioned = d.lat != null && d.lon != null;
    const pos = positioned ? `${d.lat!.toFixed(5)}, ${d.lon!.toFixed(5)}` : "— no position —";
    const row = (label: string, val: string) =>
      `<div class="pm-row"><label>${label}</label><span>${val}</span></div>`;
    const stat = (label: string, n: number) => (n > 0 ? row(label, n.toLocaleString()) : "");
    const hashSize =
      d.role === "repeater" && d.hash_size ? row("HASH SIZE", `${d.hash_size}-byte`) : "";
    const repeaterStats =
      d.role === "repeater"
        ? `<div class="pm-sec">RELAYED (ON PATH)</div>` +
          row("LAST HOUR", d.on_path_1h.toLocaleString()) +
          row("LAST 24H", d.on_path_24h.toLocaleString())
        : "";

    this.shell.panel.innerHTML = `
      <div class="pm-head" style="border-bottom-color:${color}">
        <span class="pm-type" style="color:${color}">${esc(d.name || "(unnamed node)")}</span>
        <span class="pm-route">${esc(d.role)}</span>
        <button class="pm-close" title="close (Esc)">✕</button>
      </div>
      <div class="pm-body">
        <div class="pm-row"><label>PUBKEY</label><span class="pm-mono">${esc(d.pubkey)}</span></div>
        ${hashSize}
        ${row("POSITION", pos)}
        ${row("FIRST SEEN", fmtDate(d.first_seen))}
        ${row("LAST SEEN", fmtDate(d.last_seen))}
        ${stat("ADVERTS", d.advert_count)}
        ${stat("HEARD", d.heard_as_observer)}
        ${stat("ORIGINATED", d.originated)}
        ${repeaterStats}
        <div class="pm-sec">KNOWN PATHS</div>
        <div class="pm-paths">// loading…</div>
      </div>
      <div class="pm-foot">${positioned ? '<button class="pm-replay pm-pin">📍 SHOW ON MAP</button>' : ""}</div>`;

    this.shell.panel.querySelector(".pm-close")!.addEventListener("click", () => this.shell.destroy());
    this.shell.panel.querySelector(".pm-pin")?.addEventListener("click", () => {
      this.onPin(d.lat!, d.lon!);
      this.shell.destroy();
    });
  }
}

function headLoad(title: string): string {
  return `<div class="pm-head"><span class="pm-type">${title}</span><button class="pm-close">✕</button></div><div class="pm-load">// loading…</div>`;
}

function fmtDate(unixSec: number): string {
  const d = new Date(unixSec * 1000);
  const p = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${clock(d.getTime())}`;
}
