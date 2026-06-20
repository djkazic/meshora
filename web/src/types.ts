export interface NodeT {
  pubkey: string;
  name: string;
  role: string;
  lat: number | null;
  lon: number | null;
  first_seen: number;
  last_seen: number;
  advert_count: number;
}

export interface Flow {
  type: "flow";
  hash: string;
  payload_type: number;
  payload_name: string;
  route_type: number;
  snr: number | null;
  waypoints: [number, number][];
  ts: number;
}

export interface NodeEvent {
  type: "node";
  node: NodeT;
}

export interface PacketEvent {
  type: "packet";
  hash: string;
  payload_type: number;
  payload_name: string;
  route_name: string;
  first_seen: number;
  observation_count: number;
  hops: number;
  detail: string;
}

export type WSMessage = Flow | NodeEvent | PacketEvent;

export interface FlowRec extends Flow {
  recvAt: number;
}

export interface PacketDetail {
  hash: string;
  raw_hex: string;
  route_type: number;
  route_name: string;
  payload_type: number;
  payload_name: string;
  payload_version?: number;
  first_seen: number;
  last_seen: number;
  observation_count: number;
  hops: string[];
  resolved: { pubkey: string; name: string; role: string }[];
  observers: {
    observer_id: string;
    observer_name: string;
    snr: number | null;
    rssi: number | null;
    ts: number;
    count: number;
  }[];
  advert: { pubkey: string; name: string; role: string; lat: number | null; lon: number | null } | null;
  channel: string;
  message: string;
}

export interface NodeDetail {
  pubkey: string;
  name: string;
  role: string;
  lat: number | null;
  lon: number | null;
  first_seen: number;
  last_seen: number;
  advert_count: number;
  heard_as_observer: number;
  originated: number;
  on_path_1h: number;
  on_path_24h: number;
  hash_size: number | null;
}

export interface NodePath {
  count: number;
  last_seen: number;
  hops: { pubkey: string; name: string; role: string }[];
}

export interface PacketListItem {
  hash: string;
  payload_type: number;
  payload_name: string;
  route_name: string;
  first_seen: number;
  observation_count: number;
  hops: number;
  detail: string;
}

export interface HashSizeCount {
  size: number;
  count: number;
}

export interface PathHashStats {
  packets: HashSizeCount[];
  repeaters: HashSizeCount[];
}

export interface HopBucket {
  hops: number;
  count: number;
}

export interface CentralityRow {
  pubkey: string;
  name: string;
  path_count: number;
  percentile: number;
}

export interface RouteHop {
  pubkey: string;
  name: string;
  role: string;
}

export interface RouteRow {
  count: number;
  last_seen: number;
  hops: RouteHop[];
}

export interface GraphNode {
  pubkey: string;
  name: string;
  role: string;
}

export interface GraphEdge {
  source: string;
  target: string;
  weight: number;
}

export interface NetworkGraph {
  nodes: GraphNode[];
  edges: GraphEdge[];
}

export interface CriticalNode {
  pubkey: string;
  name: string;
  role: string;
  fragments: number;
  isolated: number;
}

export interface Config {
  bbox: { minLat: number; maxLat: number; minLon: number; maxLon: number };
  center: { lat: number; lon: number };
}
