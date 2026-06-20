import type { WSMessage } from "./types";

export function connectWS(
  onMessage: (m: WSMessage) => void,
  onStatus: (connected: boolean) => void,
): void {
  let backoff = 1000;

  const open = () => {
    const proto = location.protocol === "https:" ? "wss:" : "ws:";
    const ws = new WebSocket(`${proto}//${location.host}/ws`);

    ws.onopen = () => {
      backoff = 1000;
      onStatus(true);
    };
    ws.onclose = () => {
      onStatus(false);
      setTimeout(open, backoff);
      backoff = Math.min(backoff * 2, 15000);
    };
    ws.onerror = () => ws.close();
    ws.onmessage = (ev) => {
      try {
        onMessage(JSON.parse(ev.data) as WSMessage);
      } catch {
        return;
      }
    };
  };

  open();
}
