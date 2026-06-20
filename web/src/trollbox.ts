import type { FlowRec } from "./types";
import { payloadColor } from "./palette";
import { clamp, clock, esc, routeName } from "./util";

const MAX_LINES = 250;
const POS_KEY = "meshora.trollbox.pos";

export class Trollbox {
  private root: HTMLElement;
  private list: HTMLElement;
  private onSeek: (rec: FlowRec) => void;

  constructor(onSeek: (rec: FlowRec) => void) {
    this.onSeek = onSeek;
    this.root = document.createElement("div");
    this.root.id = "trollbox";
    this.root.innerHTML = `
      <div class="tb-bar">
        <span class="tb-grip">⠿</span>
        <span class="tb-title">// TRAFFIC</span>
        <span class="tb-min" title="minimize">[─]</span>
      </div>
      <div class="tb-list"></div>`;
    document.body.appendChild(this.root);
    this.list = this.root.querySelector(".tb-list")!;

    const minBtn = this.root.querySelector(".tb-min") as HTMLElement;
    minBtn.addEventListener("pointerdown", (e) => e.stopPropagation());
    minBtn.addEventListener("click", (e) => {
      e.stopPropagation();
      this.root.classList.toggle("min");
    });

    this.restorePosition();
    this.wireDrag(this.root.querySelector(".tb-bar") as HTMLElement);
  }

  push(rec: FlowRec): void {
    this.list.insertBefore(this.makeLine(rec), this.list.firstChild);
    while (this.list.childElementCount > MAX_LINES) {
      this.list.removeChild(this.list.lastChild!);
    }
  }

  setEvents(recs: FlowRec[]): void {
    const slice = recs.slice(-MAX_LINES);
    this.list.replaceChildren();
    for (let i = slice.length - 1; i >= 0; i--) {
      this.list.appendChild(this.makeLine(slice[i]));
    }
  }

  private makeLine(rec: FlowRec): HTMLElement {
    const line = document.createElement("div");
    line.className = "tb-line";
    line.style.borderLeftColor = payloadColor(rec.payload_type);
    const hops = rec.waypoints.length;
    const snr = rec.snr != null ? `<span class="tb-s">${rec.snr.toFixed(0)}dB</span>` : "";
    line.innerHTML =
      `<span class="tb-t">${clock(rec.recvAt)}</span>` +
      `<span class="tb-p">${esc(rec.payload_name)}</span>` +
      `<span class="tb-r">${routeName(rec.route_type)}</span>` +
      `<span class="tb-h">${hops}↗</span>` +
      snr;
    line.addEventListener("click", () => this.onSeek(rec));
    return line;
  }

  private wireDrag(bar: HTMLElement): void {
    let startX = 0,
      startY = 0,
      originX = 0,
      originY = 0,
      dragging = false;

    bar.addEventListener("pointerdown", (e) => {
      dragging = true;
      bar.setPointerCapture(e.pointerId);
      const r = this.root.getBoundingClientRect();
      this.root.style.left = `${r.left}px`;
      this.root.style.top = `${r.top}px`;
      this.root.style.right = "auto";
      this.root.style.bottom = "auto";
      startX = e.clientX;
      startY = e.clientY;
      originX = r.left;
      originY = r.top;
    });

    bar.addEventListener("pointermove", (e) => {
      if (!dragging) return;
      const x = clamp(originX + e.clientX - startX, 0, window.innerWidth - this.root.offsetWidth);
      const y = clamp(originY + e.clientY - startY, 0, window.innerHeight - 40);
      this.root.style.left = `${x}px`;
      this.root.style.top = `${y}px`;
    });

    const end = () => {
      if (!dragging) return;
      dragging = false;
      localStorage.setItem(
        POS_KEY,
        JSON.stringify({ x: this.root.offsetLeft, y: this.root.offsetTop }),
      );
    };
    bar.addEventListener("pointerup", end);
    bar.addEventListener("pointercancel", end);
  }

  private restorePosition(): void {
    if (window.innerWidth < 640) return;
    try {
      const raw = localStorage.getItem(POS_KEY);
      if (!raw) return;
      const { x, y } = JSON.parse(raw) as { x: number; y: number };
      this.root.style.left = `${clamp(x, 0, window.innerWidth - 80)}px`;
      this.root.style.top = `${clamp(y, 0, window.innerHeight - 40)}px`;
      this.root.style.right = "auto";
      this.root.style.bottom = "auto";
    } catch {
      return;
    }
  }
}
