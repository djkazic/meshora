import { getPacketList } from "./api";
import type { PacketListItem } from "./types";
import { payloadColor } from "./palette";
import { clock, esc, payloadAbbrev } from "./util";

const MAX_ROWS = 400;

export class PacketsView {
  private tbody: HTMLElement;
  private ftype = "";
  private fnode = "";
  private nodeTimer: number | undefined;

  constructor(private onRow: (hash: string) => void) {
    const list = document.querySelector("#packets .pk-list")!;
    list.innerHTML =
      `<table class="pk-table"><thead><tr>` +
      `<th>hash</th><th>time</th><th>type</th><th>detail</th><th>route</th><th>hops</th><th>obs</th>` +
      `</tr></thead><tbody></tbody></table>`;
    this.tbody = list.querySelector("tbody")!;

    const typeSel = document.querySelector<HTMLSelectElement>("#packets .pk-ftype");
    typeSel?.addEventListener("change", () => {
      this.ftype = typeSel.value;
      this.refresh();
    });
    const nodeInput = document.querySelector<HTMLInputElement>("#packets .pk-fnode");
    nodeInput?.addEventListener("input", () => {
      clearTimeout(this.nodeTimer);
      this.nodeTimer = window.setTimeout(() => {
        this.fnode = nodeInput.value.trim();
        this.refresh();
      }, 300);
    });
  }

  async refresh(): Promise<void> {
    try {
      const items = await getPacketList(300, this.ftype, this.fnode);
      this.tbody.innerHTML = items.map((p) => this.row(p)).join("");
      this.tbody
        .querySelectorAll<HTMLElement>("tr[data-h]")
        .forEach((tr) => (tr.onclick = () => this.onRow(tr.dataset.h!)));
    } catch {
      return;
    }
  }

  prepend(p: PacketListItem): void {
    if (this.fnode) return;
    if (this.ftype !== "" && String(p.payload_type) !== this.ftype) return;
    this.tbody.insertAdjacentHTML("afterbegin", this.row(p));
    const first = this.tbody.firstElementChild as HTMLElement | null;
    if (first) first.onclick = () => this.onRow(first.dataset.h!);
    while (this.tbody.childElementCount > MAX_ROWS) this.tbody.lastElementChild!.remove();
  }

  private row(p: PacketListItem): string {
    return (
      `<tr data-h="${esc(p.hash)}">` +
      `<td class="pk-hash">${esc(p.hash)}</td>` +
      `<td>${clock(p.first_seen * 1000)}</td>` +
      `<td><i class="pk-dot" style="background:${payloadColor(p.payload_type)}"></i>` +
      `<span class="pk-typef">${esc(p.payload_name)}</span>` +
      `<span class="pk-typea">${esc(payloadAbbrev(p.payload_name))}</span></td>` +
      `<td class="pk-detail" title="${esc(p.detail || "")}">${esc(p.detail || "")}</td>` +
      `<td>${esc(p.route_name)}</td>` +
      `<td>${p.hops}</td>` +
      `<td>${p.observation_count}</td></tr>`
    );
  }
}
