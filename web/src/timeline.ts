import type { FlowRec } from "./types";
import { payloadColor } from "./palette";
import { clamp, clock } from "./util";

const WINDOW = 5 * 60 * 1000;
const PRUNE = WINDOW * 1.5;

export interface Feed {
  setEvents(recs: FlowRec[]): void;
  push(rec: FlowRec): void;
}

export class TimeMachine {
  private buffer: FlowRec[] = [];
  private mode: "live" | "replay" = "live";
  private playing = false;
  private playhead = Date.now();
  private axisMax = Date.now();
  private lastFrame = 0;
  private dragging = false;
  private scrubPrev = 0;
  private feedDirty = false;

  private animate: (r: FlowRec) => void;
  private feed: Feed | null = null;

  private root: HTMLElement;
  private canvas: HTMLCanvasElement;
  private ctx: CanvasRenderingContext2D;
  private head: HTMLElement;
  private btnPlay: HTMLElement;
  private btnLive: HTMLElement;
  private label: HTMLElement;

  constructor(animate: (r: FlowRec) => void) {
    this.animate = animate;
    this.root = document.createElement("div");
    this.root.id = "timeline";
    this.root.innerHTML = `
      <div class="tl-ctrls">
        <button class="tl-btn tl-play" title="play / pause">▶</button>
        <button class="tl-btn tl-live on" title="resume live">● LIVE</button>
      </div>
      <div class="tl-track">
        <canvas class="tl-canvas"></canvas>
        <div class="tl-head"></div>
        <div class="tl-label"></div>
      </div>`;
    document.body.appendChild(this.root);

    this.canvas = this.root.querySelector(".tl-canvas")!;
    this.ctx = this.canvas.getContext("2d")!;
    this.head = this.root.querySelector(".tl-head")!;
    this.label = this.root.querySelector(".tl-label")!;
    this.btnPlay = this.root.querySelector(".tl-play")!;
    this.btnLive = this.root.querySelector(".tl-live")!;

    this.btnPlay.addEventListener("click", () => this.togglePlay());
    this.btnLive.addEventListener("click", () => this.goLive());

    const track = this.root.querySelector(".tl-track") as HTMLElement;
    track.addEventListener("pointerdown", (e) => this.onDown(track, e));
    window.addEventListener("pointermove", (e) => this.onMove(e));
    window.addEventListener("pointerup", () => this.onUp());

    new ResizeObserver(() => this.resize()).observe(this.canvas);
    this.resize();
    requestAnimationFrame(this.frame);
  }

  setFeed(feed: Feed): void {
    this.feed = feed;
    this.feedDirty = true;
  }

  push(rec: FlowRec): void {
    this.buffer.push(rec);
    const cutoff = Date.now() - PRUNE;
    while (this.buffer.length && this.buffer[0].recvAt < cutoff) this.buffer.shift();
    if (this.mode === "live") {
      this.animate(rec);
      this.feed?.push(rec);
    }
  }

  seed(recs: FlowRec[]): void {
    this.buffer.push(...recs);
    this.buffer.sort((a, b) => a.recvAt - b.recvAt);
    this.feedDirty = true;
  }

  seek(t: number): void {
    this.mode = "replay";
    this.axisMax = Date.now();
    this.playhead = clamp(t, this.tMin(), this.axisMax);
    this.scrubPrev = this.playhead;
    this.playing = true;
    this.feedDirty = true;
    this.animateAround(this.playhead);
    this.sync();
  }

  private tMax(): number {
    return this.mode === "live" ? Date.now() : this.axisMax;
  }
  private tMin(): number {
    return this.tMax() - WINDOW;
  }
  private visibleUpTo(): number {
    return this.mode === "live" ? Date.now() : this.playhead;
  }

  private goLive(): void {
    this.mode = "live";
    this.playing = false;
    this.feedDirty = true;
    this.sync();
  }

  private togglePlay(): void {
    if (this.mode === "live") {
      this.seek(this.tMin());
      return;
    }
    this.playing = !this.playing;
    this.sync();
  }

  private sync(): void {
    this.btnLive.classList.toggle("on", this.mode === "live");
    this.btnPlay.textContent = this.playing ? "⏸" : "▶";
  }

  private timeAtX(clientX: number): number {
    const r = this.canvas.getBoundingClientRect();
    const frac = clamp((clientX - r.left) / r.width, 0, 1);
    return this.tMin() + frac * WINDOW;
  }

  private onDown(track: HTMLElement, e: PointerEvent): void {
    this.dragging = true;
    track.setPointerCapture(e.pointerId);
    if (this.mode === "live") this.axisMax = Date.now();
    this.mode = "replay";
    this.playing = false;
    const t = this.timeAtX(e.clientX);
    this.playhead = t;
    this.scrubPrev = t;
    this.feedDirty = true;
    this.animateAround(t);
    this.sync();
  }

  private onMove(e: PointerEvent): void {
    if (!this.dragging) return;
    this.scrubTo(this.timeAtX(e.clientX));
  }

  private onUp(): void {
    if (!this.dragging) return;
    this.dragging = false;
    this.playing = true;
    this.sync();
  }

  private scrubTo(t: number): void {
    const lo = Math.min(this.scrubPrev, t);
    const hi = Math.max(this.scrubPrev, t);
    for (const f of this.buffer) {
      if (f.recvAt > lo && f.recvAt <= hi) this.animate(f);
    }
    this.playhead = t;
    this.scrubPrev = t;
    this.feedDirty = true;
  }

  private animateAround(t: number): void {
    for (const f of this.buffer) {
      if (f.recvAt > t - 60 && f.recvAt <= t) this.animate(f);
    }
  }

  private resize(): void {
    const dpr = window.devicePixelRatio || 1;
    const w = this.canvas.clientWidth;
    const h = this.canvas.clientHeight;
    this.canvas.width = w * dpr;
    this.canvas.height = h * dpr;
    this.ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
  }

  private frame = (now: number): void => {
    const dt = this.lastFrame ? now - this.lastFrame : 0;
    this.lastFrame = now;

    if (this.mode === "live") {
      this.playhead = Date.now();
    } else if (this.playing && !this.dragging) {
      const prev = this.playhead;
      this.playhead = Math.min(prev + dt, this.axisMax);
      for (const f of this.buffer) {
        if (f.recvAt > prev && f.recvAt <= this.playhead) {
          this.animate(f);
          this.feed?.push(f);
        }
      }
      if (this.playhead >= this.axisMax) this.goLive();
    }

    if (this.feedDirty && this.feed) {
      const upto = this.visibleUpTo();
      this.feed.setEvents(this.buffer.filter((f) => f.recvAt <= upto));
      this.feedDirty = false;
    }

    this.render();
    requestAnimationFrame(this.frame);
  };

  private render(): void {
    const ctx = this.ctx;
    const w = this.canvas.clientWidth;
    const h = this.canvas.clientHeight;
    ctx.clearRect(0, 0, w, h);

    const tMin = this.tMin();
    let count = 0;
    for (const f of this.buffer) {
      if (f.recvAt < tMin) continue;
      count++;
      const x = ((f.recvAt - tMin) / WINDOW) * w;
      ctx.globalAlpha = 0.85;
      ctx.strokeStyle = payloadColor(f.payload_type);
      ctx.lineWidth = 1;
      ctx.beginPath();
      ctx.moveTo(x, h);
      ctx.lineTo(x, h - 6 - Math.min(f.waypoints.length * 3, 18));
      ctx.stroke();
    }
    ctx.globalAlpha = 1;

    const hx = ((this.playhead - tMin) / WINDOW) * w;
    this.head.style.left = `${clamp((hx / w) * 100, 0, 100)}%`;

    const tag = this.mode === "live" ? "LIVE" : clock(this.playhead);
    this.label.textContent = `${clock(tMin)} ── ${tag} ──▶ ${count} pkt`;
  }
}
