const ROUTE_NAMES = ["TFLOOD", "FLOOD", "DIRECT", "TDIRECT"];

export function routeName(rt: number): string {
  return ROUTE_NAMES[rt] ?? "?";
}

export function clock(ms: number): string {
  const d = new Date(ms);
  return [d.getHours(), d.getMinutes(), d.getSeconds()]
    .map((n) => String(n).padStart(2, "0"))
    .join(":");
}

export function roleAbbrev(role: string): string {
  return { repeater: "R", room: "Rm", companion: "C", sensor: "S" }[role] ?? "";
}

const PAYLOAD_ABBREV: Record<string, string> = {
  ADVERT: "Advt",
  TXT_MSG: "Txt",
  GRP_TXT: "GTxt",
  GRP_DATA: "GDat",
  ACK: "Ack",
  REQ: "Req",
  RESPONSE: "Resp",
  ANON_REQ: "Anon",
  PATH: "Path",
  TRACE: "Trce",
  MULTIPART: "Mult",
  CONTROL: "Ctrl",
  RAW_CUSTOM: "Raw",
};

export function payloadAbbrev(name: string): string {
  return PAYLOAD_ABBREV[name] ?? name.slice(0, 4);
}

export function clamp(v: number, lo: number, hi: number): number {
  return Math.max(lo, Math.min(v, hi));
}

const ESCAPES: Record<string, string> = {
  "&": "&amp;",
  "<": "&lt;",
  ">": "&gt;",
  '"': "&quot;",
  "'": "&#39;",
};

export function esc(s: string): string {
  return s.replace(/[&<>"']/g, (c) => ESCAPES[c]);
}
