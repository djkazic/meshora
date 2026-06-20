import "./style.css";
import { getConfig, getNodes, getRecentFlows, getStats } from "./api";
import { MeshMap } from "./map";
import { FlowLayer } from "./flows";
import { Trollbox } from "./trollbox";
import { TimeMachine } from "./timeline";
import { connectWS } from "./ws";
import { NodeWindow, PacketWindow } from "./modal";
import { PacketsView } from "./packets";
import { AnalyticsView } from "./analytics";
import { NodeSearch } from "./search";
import { LEGEND_ROLES } from "./palette";
import type { FlowRec, WSMessage } from "./types";

async function boot(): Promise<void> {
  const cfg = await getConfig();
  const mesh = new MeshMap(cfg);
  const flows = new FlowLayer(mesh.map);

  buildLegend();
  wireCoordReadout(mesh);

  const time = new TimeMachine((rec) => flows.add(rec));
  function openNode(pubkey: string): void {
    new NodeWindow(pubkey, (lat, lon) => mesh.map.setView([lat, lon], 14), openNode);
  }
  const openPacket = (hash: string, replayRec: FlowRec | null = null) =>
    new PacketWindow(hash, replayRec, (rec) => flows.add(rec), openNode);
  mesh.onNodeClick = openNode;
  new NodeSearch(mesh, openNode);
  const trollbox = new Trollbox((rec) => openPacket(rec.hash, rec));
  time.setFeed(trollbox);

  const packetsView = new PacketsView((hash) => openPacket(hash));
  const analyticsView = new AnalyticsView(openNode);
  wireTabs(packetsView, analyticsView);
  packetsView.refresh();
  positionPacketsBelowHud();

  for (const n of await getNodes()) mesh.upsertNode(n);
  updateStat("stat-nodes", mesh.nodeCount());

  const stats = await getStats().catch(() => null);
  let packets = stats?.transmissions ?? 0;
  updateStat("stat-packets", packets);

  const history = await getRecentFlows(5).catch(() => []);
  time.seed(history.map((f) => ({ ...f, recvAt: f.ts * 1000 })));

  const flowTimes: number[] = [];

  connectWS(
    (msg: WSMessage) => {
      if (msg.type === "node") {
        mesh.upsertNode(msg.node);
        updateStat("stat-nodes", mesh.nodeCount());
        return;
      }
      if (msg.type === "packet") {
        packetsView.prepend(msg);
        packets++;
        updateStat("stat-packets", packets);
        return;
      }
      const rec: FlowRec = { ...msg, recvAt: Date.now() };
      time.push(rec);
      const now = performance.now();
      flowTimes.push(now);
      while (flowTimes.length && now - flowTimes[0] > 60000) flowTimes.shift();
      updateStat("stat-fpm", flowTimes.length);
    },
    (connected) => {
      const el = document.getElementById("conn")!;
      el.classList.toggle("off", !connected);
      el.classList.toggle("on", connected);
      el.textContent = connected ? "ONLINE" : "OFFLINE";
    },
  );

  setInterval(async () => {
    const s = await getStats().catch(() => null);
    if (s) {
      packets = Math.max(packets, s.transmissions);
      updateStat("stat-packets", packets);
    }
  }, 30000);
}

function updateStat(id: string, value: number): void {
  document.getElementById(id)!.textContent = value.toLocaleString();
}

function buildLegend(): void {
  const el = document.getElementById("legend")!;
  el.innerHTML = LEGEND_ROLES.map(
    ([role, color]) =>
      `<span class="lg"><i style="background:${color}"></i>${role}</span>`,
  ).join("");
}

function positionPacketsBelowHud(): void {
  const hud = document.getElementById("hud")!;
  const packets = document.getElementById("packets")!;
  const analytics = document.getElementById("analytics")!;
  const search = document.getElementById("nodesearch")!;
  const place = () => {
    const bottom = hud.getBoundingClientRect().bottom;
    packets.style.top = `${bottom + 8}px`;
    analytics.style.top = `${bottom + 8}px`;
    const zoom = document.querySelector<HTMLElement>(".leaflet-top.leaflet-left");
    if (window.innerWidth < 640) {
      search.style.top = `${bottom + 8}px`;
      search.style.left = "6px";
      search.style.right = "6px";
      search.style.width = "auto";
      search.style.transform = "none";
      if (zoom) zoom.style.top = `${search.getBoundingClientRect().bottom + 8}px`;
    } else {
      search.style.top = search.style.left = search.style.right = search.style.width = search.style.transform = "";
      if (zoom) zoom.style.top = `${bottom + 8}px`;
    }
  };
  place();
  window.addEventListener("resize", place);
  new ResizeObserver(place).observe(hud);
}

function wireTabs(packets: PacketsView, analytics: AnalyticsView): void {
  document.body.classList.add("tab-map");
  for (const btn of document.querySelectorAll<HTMLElement>(".tab")) {
    btn.addEventListener("click", () => {
      const tab = btn.dataset.tab!;
      document.querySelectorAll(".tab").forEach((b) => b.classList.toggle("active", b === btn));
      document.body.classList.toggle("tab-packets", tab === "packets");
      document.body.classList.toggle("tab-analytics", tab === "analytics");
      document.body.classList.toggle("tab-map", tab === "map");
      if (tab === "packets") packets.refresh();
      if (tab === "analytics") analytics.refresh();
      else analytics.deactivate();
    });
  }
}

function wireCoordReadout(mesh: MeshMap): void {
  const el = document.getElementById("coords")!;
  mesh.map.on("mousemove", (e) => {
    el.textContent = `${e.latlng.lat.toFixed(4)}N ${e.latlng.lng.toFixed(4)}W`;
  });
}

boot().catch((err) => {
  document.body.insertAdjacentHTML(
    "beforeend",
    `<div class="fatal">!! boot failure: ${String(err)}</div>`,
  );
});
