import L from "leaflet";
import type { Flow } from "./types";
import { payloadColor } from "./palette";

interface Anim {
  pts: L.LatLng[];
  color: string;
  start: number;
  dur: number;
}

const MAX_ACTIVE = 120;
const TRAVEL_BASE = 750;
const TRAVEL_PER_SEG = 250;
const FADE = 550;

export class FlowLayer {
  private map: L.Map;
  private canvas: HTMLCanvasElement;
  private ctx: CanvasRenderingContext2D;
  private anims: Anim[] = [];
  private dpr = 1;
  private running = false;

  constructor(map: L.Map) {
    this.map = map;
    this.canvas = document.createElement("canvas");
    const s = this.canvas.style;
    s.position = "absolute";
    s.inset = "0";
    s.zIndex = "450";
    s.pointerEvents = "none";
    map.getContainer().appendChild(this.canvas);
    this.ctx = this.canvas.getContext("2d")!;

    this.resize();
    this.map.on("resize", () => this.resize());
    window.addEventListener("resize", () => this.resize());
  }

  add(flow: Flow): void {
    if (flow.waypoints.length < 1) return;
    const pts = flow.waypoints.map(([lat, lon]) => L.latLng(lat, lon));
    this.push({
      pts,
      color: payloadColor(flow.payload_type),
      start: performance.now(),
      dur: TRAVEL_BASE + (pts.length - 2) * TRAVEL_PER_SEG,
    });
  }

  private push(a: Anim): void {
    if (this.anims.length >= MAX_ACTIVE) this.anims.shift();
    this.anims.push(a);
    this.ensureRunning();
  }

  private ensureRunning(): void {
    if (!this.running) {
      this.running = true;
      requestAnimationFrame(this.frame);
    }
  }

  private resize(): void {
    const c = this.map.getContainer();
    this.dpr = window.devicePixelRatio || 1;
    this.canvas.width = c.clientWidth * this.dpr;
    this.canvas.height = c.clientHeight * this.dpr;
    this.canvas.style.width = `${c.clientWidth}px`;
    this.canvas.style.height = `${c.clientHeight}px`;
    this.ctx.setTransform(this.dpr, 0, 0, this.dpr, 0, 0);
  }

  private frame = (now: number): void => {
    const ctx = this.ctx;
    ctx.clearRect(0, 0, this.canvas.width, this.canvas.height);

    this.anims = this.anims.filter((a) => now - a.start < a.dur + FADE);
    for (const a of this.anims) this.draw(a, now);

    if (this.anims.length > 0) {
      requestAnimationFrame(this.frame);
    } else {
      this.running = false;
    }
  };

  private draw(a: Anim, now: number): void {
    const ctx = this.ctx;
    const screen = a.pts.map((p) => this.map.latLngToContainerPoint(p));

    const elapsed = now - a.start;
    const progress = Math.min(elapsed / a.dur, 1);
    const fadeAlpha =
      elapsed <= a.dur ? 1 : Math.max(0, 1 - (elapsed - a.dur) / FADE);

    if (screen.length === 1) {
      const p = screen[0];
      const r = 2 + progress * 18;
      ctx.globalAlpha = 0.7 * fadeAlpha * (1 - progress * 0.5);
      ctx.strokeStyle = a.color;
      ctx.lineWidth = 2;
      ctx.beginPath();
      ctx.arc(p.x, p.y, r, 0, Math.PI * 2);
      ctx.stroke();
      ctx.globalAlpha = fadeAlpha;
      ctx.fillStyle = a.color;
      ctx.shadowColor = a.color;
      ctx.shadowBlur = 10;
      ctx.beginPath();
      ctx.arc(p.x, p.y, 3, 0, Math.PI * 2);
      ctx.fill();
      ctx.shadowBlur = 0;
      ctx.globalAlpha = 1;
      return;
    }

    const seg: number[] = [0];
    let total = 0;
    for (let i = 1; i < screen.length; i++) {
      total += screen[i].distanceTo(screen[i - 1]);
      seg.push(total);
    }
    if (total === 0) return;

    ctx.lineJoin = "round";
    ctx.lineCap = "round";
    ctx.beginPath();
    ctx.moveTo(screen[0].x, screen[0].y);
    for (let i = 1; i < screen.length; i++) ctx.lineTo(screen[i].x, screen[i].y);
    ctx.strokeStyle = a.color;
    ctx.globalAlpha = 0.22 * fadeAlpha;
    ctx.lineWidth = 1.5;
    ctx.stroke();

    const head = pointAt(screen, seg, progress * total);
    ctx.globalAlpha = fadeAlpha;
    ctx.fillStyle = a.color;
    ctx.shadowColor = a.color;
    ctx.shadowBlur = 12;
    ctx.beginPath();
    ctx.arc(head.x, head.y, 3.5, 0, Math.PI * 2);
    ctx.fill();
    ctx.shadowBlur = 0;
    ctx.globalAlpha = 1;
  }
}

function pointAt(pts: L.Point[], cum: number[], d: number): L.Point {
  if (d <= 0) return pts[0];
  const last = cum[cum.length - 1];
  if (d >= last) return pts[pts.length - 1];
  let i = 1;
  while (i < cum.length && cum[i] < d) i++;
  const segLen = cum[i] - cum[i - 1] || 1;
  const t = (d - cum[i - 1]) / segLen;
  return L.point(
    pts[i - 1].x + (pts[i].x - pts[i - 1].x) * t,
    pts[i - 1].y + (pts[i].y - pts[i - 1].y) * t,
  );
}
