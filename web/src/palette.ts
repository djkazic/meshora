export const ROLE_COLORS: Record<string, string> = {
  repeater: "#4ea6b5",
  room: "#9085c9",
  companion: "#cda450",
  sensor: "#56ad86",
  none: "#7e8b9c",
};

export function roleColor(role: string): string {
  return ROLE_COLORS[role] ?? "#94a3b8";
}

export function payloadColor(payloadType: number): string {
  switch (payloadType) {
    case 4:
      return "#34d399";
    case 2:
    case 5:
    case 6:
      return "#60a5fa";
    case 3:
      return "#94a3b8";
    case 8:
    case 9:
      return "#a78bfa";
    case 0:
    case 1:
    case 7:
      return "#fbbf24";
    default:
      return "#f472b6";
  }
}

export const LEGEND_ROLES: [string, string][] = [
  ["repeater", ROLE_COLORS.repeater],
  ["room", ROLE_COLORS.room],
  ["companion", ROLE_COLORS.companion],
  ["sensor", ROLE_COLORS.sensor],
];
