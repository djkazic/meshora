import type {
  Config,
  Flow,
  NodeDetail,
  NodePath,
  NodeT,
  PacketDetail,
  PacketListItem,
} from "./types";

async function getJSON<T>(path: string): Promise<T> {
  const res = await fetch(path);
  if (!res.ok) throw new Error(`${path}: ${res.status}`);
  return res.json() as Promise<T>;
}

export const getConfig = () => getJSON<Config>("/api/config");
export const getNodes = () => getJSON<NodeT[]>("/api/nodes");
export const getRecentFlows = (mins = 5) =>
  getJSON<Flow[]>(`/api/flows/recent?mins=${mins}`);
export const getPacket = (hash: string) =>
  getJSON<PacketDetail>(`/api/packets/${encodeURIComponent(hash)}`);
export const getNode = (pubkey: string) =>
  getJSON<NodeDetail>(`/api/nodes/${encodeURIComponent(pubkey)}`);
export const getNodePaths = (pubkey: string) =>
  getJSON<NodePath[]>(`/api/nodes/${encodeURIComponent(pubkey)}/paths`);
export const getPacketList = (limit = 200, type = "", node = "") => {
  const q = new URLSearchParams({ limit: String(limit) });
  if (type !== "") q.set("type", type);
  if (node !== "") q.set("node", node);
  return getJSON<PacketListItem[]>(`/api/packets?${q.toString()}`);
};
export const getStats = () =>
  getJSON<{ transmissions: number; nodes_with_pos: number }>("/api/stats");
