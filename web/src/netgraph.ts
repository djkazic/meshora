import { roleColor } from "./palette";
import { clamp } from "./util";
import type { NetworkGraph } from "./types";

interface SimNode {
  pubkey: string;
  name: string;
  role: string;
  x: number;
  y: number;
  vx: number;
  vy: number;
  deg: number;
}

const HEIGHT = 460;

export class NetGraph {
  private canvas: HTMLCanvasElement;
  private ctx: CanvasRenderingContext2D;
  private nodes: SimNode[] = [];
  private links: { a: number; b: number; w: number }[] = [];
  private raf = 0;
  private run = 0;
  private alpha = 1;
  private scale = 1;
  private tx = 0;
  private ty = 0;
  private manual = false;
  private dragging = false;
  private moved = false;
  private lastX = 0;
  private lastY = 0;

  constructor(private host: HTMLElement, private onNode: (pubkey: string) => void) {
    this.canvas = document.createElement("canvas");
    this.canvas.className = "an-graph";
    this.host.appendChild(this.canvas);
    this.ctx = this.canvas.getContext("2d")!;
    this.canvas.addEventListener("wheel", (e) => this.onWheel(e), { passive: false });
    this.canvas.addEventListener("mousedown", (e) => this.onDown(e));
    window.addEventListener("mousemove", (e) => this.onMove(e));
    window.addEventListener("mouseup", () => this.onUp());
  }

  render(data: NetworkGraph): void {
    const run = ++this.run;
    cancelAnimationFrame(this.raf);
    this.resize();
    if (data.nodes.length === 0) {
      this.ctx.clearRect(0, 0, this.canvas.width, this.canvas.height);
      return;
    }
    const idx = new Map<string, number>();
    this.nodes = data.nodes.map((n, i) => {
      idx.set(n.pubkey, i);
      const a = i * 2.39996;
      const rad = 14 * Math.sqrt(i);
      return { ...n, x: Math.cos(a) * rad, y: Math.sin(a) * rad, vx: 0, vy: 0, deg: 0 };
    });
    this.links = [];
    for (const e of data.edges) {
      const a = idx.get(e.source);
      const b = idx.get(e.target);
      if (a === undefined || b === undefined) continue;
      this.links.push({ a, b, w: e.weight });
      this.nodes[a].deg += e.weight;
      this.nodes[b].deg += e.weight;
    }
    this.alpha = 1;
    this.manual = false;
    const tick = () => {
      if (run !== this.run) return;
      this.step();
      if (!this.manual) this.fit();
      this.draw();
      this.alpha *= 0.99;
      if (this.alpha > 0.02) this.raf = requestAnimationFrame(tick);
    };
    tick();
  }

  stop(): void {
    this.run++;
    cancelAnimationFrame(this.raf);
  }

  private resize(): void {
    const dpr = window.devicePixelRatio || 1;
    const w = this.host.clientWidth || 600;
    this.canvas.width = w * dpr;
    this.canvas.height = HEIGHT * dpr;
    this.canvas.style.width = `${w}px`;
    this.canvas.style.height = `${HEIGHT}px`;
    this.ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
  }

  private step(): void {
    const n = this.nodes;
    for (let i = 0; i < n.length; i++) {
      for (let j = i + 1; j < n.length; j++) {
        let dx = n[i].x - n[j].x;
        let dy = n[i].y - n[j].y;
        const d2 = dx * dx + dy * dy || 0.01;
        const d = Math.sqrt(d2);
        const f = 11000 / d2;
        dx = (dx / d) * f;
        dy = (dy / d) * f;
        n[i].vx += dx;
        n[i].vy += dy;
        n[j].vx -= dx;
        n[j].vy -= dy;
      }
    }
    for (const l of this.links) {
      const a = n[l.a];
      const b = n[l.b];
      const dx = b.x - a.x;
      const dy = b.y - a.y;
      const d = Math.sqrt(dx * dx + dy * dy) || 0.01;
      const f = (d - 55) * 0.02;
      const fx = (dx / d) * f;
      const fy = (dy / d) * f;
      a.vx += fx;
      a.vy += fy;
      b.vx -= fx;
      b.vy -= fy;
    }
    for (const node of n) {
      node.vx -= node.x * 0.0035;
      node.vy -= node.y * 0.0035;
      node.vx *= 0.86;
      node.vy *= 0.86;
      node.x += node.vx * this.alpha;
      node.y += node.vy * this.alpha;
    }
  }

  private fit(): void {
    const w = this.canvas.clientWidth;
    let minX = Infinity;
    let minY = Infinity;
    let maxX = -Infinity;
    let maxY = -Infinity;
    for (const nd of this.nodes) {
      minX = Math.min(minX, nd.x);
      minY = Math.min(minY, nd.y);
      maxX = Math.max(maxX, nd.x);
      maxY = Math.max(maxY, nd.y);
    }
    const pad = 36;
    this.scale = Math.min((w - pad * 2) / (maxX - minX || 1), (HEIGHT - pad * 2) / (maxY - minY || 1));
    this.tx = (w - (minX + maxX) * this.scale) / 2;
    this.ty = (HEIGHT - (minY + maxY) * this.scale) / 2;
  }

  private draw(): void {
    const w = this.canvas.clientWidth;
    const ctx = this.ctx;
    ctx.clearRect(0, 0, w, HEIGHT);

    let maxW = 1;
    for (const l of this.links) maxW = Math.max(maxW, l.w);
    for (const l of this.links) {
      const a = this.nodes[l.a];
      const b = this.nodes[l.b];
      ctx.strokeStyle = `rgba(94, 243, 140, ${(0.06 + 0.44 * (l.w / maxW)).toFixed(3)})`;
      ctx.lineWidth = 0.5 + 1.5 * (l.w / maxW);
      ctx.beginPath();
      ctx.moveTo(this.sx(a), this.sy(a));
      ctx.lineTo(this.sx(b), this.sy(b));
      ctx.stroke();
    }

    let maxDeg = 1;
    for (const nd of this.nodes) maxDeg = Math.max(maxDeg, nd.deg);
    for (const nd of this.nodes) {
      const r = 2 + 5 * Math.sqrt(nd.deg / maxDeg);
      ctx.fillStyle = roleColor(nd.role);
      ctx.beginPath();
      ctx.arc(this.sx(nd), this.sy(nd), r, 0, Math.PI * 2);
      ctx.fill();
    }

    ctx.font = '10px ui-monospace, "DejaVu Sans Mono", monospace';
    ctx.lineWidth = 3;
    ctx.strokeStyle = "rgba(5, 8, 10, 0.85)";
    ctx.fillStyle = "#9fe9c0";
    const placed: { x: number; y: number; w: number; h: number }[] = [];
    for (const nd of [...this.nodes].sort((a, b) => b.deg - a.deg)) {
      const px = this.sx(nd);
      const py = this.sy(nd);
      if (px < -20 || px > w + 20 || py < -20 || py > HEIGHT + 20) continue;
      const label = nd.name || nd.pubkey.slice(0, 6);
      const lw = ctx.measureText(label).width;
      const r = 2 + 5 * Math.sqrt(nd.deg / maxDeg);
      const spots = [
        [px + r + 4, py + 3],
        [px - r - 4 - lw, py + 3],
        [px - lw / 2, py - r - 5],
        [px - lw / 2, py + r + 12],
      ];
      const spot = spots.find((s) => {
        const box = { x: s[0], y: s[1] - 9, w: lw, h: 12 };
        return !placed.some((p) => box.x < p.x + p.w && box.x + box.w > p.x && box.y < p.y + p.h && box.y + box.h > p.y);
      });
      if (!spot) continue;
      placed.push({ x: spot[0], y: spot[1] - 9, w: lw, h: 12 });
      ctx.strokeText(label, spot[0], spot[1]);
      ctx.fillText(label, spot[0], spot[1]);
    }

    ctx.font = '9px ui-monospace, monospace';
    ctx.fillStyle = "rgba(58, 125, 87, 0.8)";
    ctx.fillText("scroll to zoom · drag to pan · click a node", 8, HEIGHT - 8);
  }

  private sx(n: SimNode): number {
    return n.x * this.scale + this.tx;
  }

  private sy(n: SimNode): number {
    return n.y * this.scale + this.ty;
  }

  private onWheel(e: WheelEvent): void {
    e.preventDefault();
    const rect = this.canvas.getBoundingClientRect();
    const px = e.clientX - rect.left;
    const py = e.clientY - rect.top;
    const factor = Math.exp(-e.deltaY * 0.0015);
    const next = clamp(this.scale * factor, 0.05, 30);
    this.tx = px - (px - this.tx) * (next / this.scale);
    this.ty = py - (py - this.ty) * (next / this.scale);
    this.scale = next;
    this.manual = true;
    this.draw();
  }

  private onDown(e: MouseEvent): void {
    this.dragging = true;
    this.moved = false;
    this.lastX = e.clientX;
    this.lastY = e.clientY;
    this.canvas.style.cursor = "grabbing";
  }

  private onMove(e: MouseEvent): void {
    if (!this.dragging) return;
    const dx = e.clientX - this.lastX;
    const dy = e.clientY - this.lastY;
    if (dx * dx + dy * dy > 9) this.moved = true;
    this.lastX = e.clientX;
    this.lastY = e.clientY;
    this.tx += dx;
    this.ty += dy;
    this.manual = true;
    this.draw();
  }

  private onUp(): void {
    if (!this.dragging) return;
    this.dragging = false;
    this.canvas.style.cursor = "grab";
    if (!this.moved) this.pick();
  }

  private pick(): void {
    const px = this.lastX - this.canvas.getBoundingClientRect().left;
    const py = this.lastY - this.canvas.getBoundingClientRect().top;
    let best = -1;
    let bestD = 16 * 16;
    for (let i = 0; i < this.nodes.length; i++) {
      const dx = this.sx(this.nodes[i]) - px;
      const dy = this.sy(this.nodes[i]) - py;
      const d = dx * dx + dy * dy;
      if (d < bestD) {
        bestD = d;
        best = i;
      }
    }
    if (best >= 0) this.onNode(this.nodes[best].pubkey);
  }
}
