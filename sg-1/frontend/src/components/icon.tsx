import type { CSSProperties } from "react";

const paths = {
  grid: "M3 3h7v7H3z M14 3h7v7h-7z M3 14h7v7H3z M14 14h7v7h-7z",
  plus: "M12 5v14 M5 12h14",
  arrow: "M5 12h14 M13 6l6 6-6 6",
  chevron: "M9 5l7 7-7 7",
  lock: "M5 10h14v11H5z M8 10V7a4 4 0 0 1 8 0v3 M12 14v3",
  search: "M21 21l-5-5 M18 10a8 8 0 1 1-16 0 8 8 0 0 1 16 0",
  layers: "M12 3 2 8l10 5 10-5-10-5z M2 12l10 5 10-5 M2 16l10 5 10-5",
  clock: "M12 8v5l3 2 M22 12a10 10 0 1 1-20 0 10 10 0 0 1 20 0",
  code: "m8 6-6 6 6 6 M16 6l6 6-6 6 M14 3l-4 18",
  spark: "m12 3 2.5 6.5L21 12l-6.5 2.5L12 21l-2.5-6.5L3 12l6.5-2.5L12 3z",
  close: "M6 6l12 12 M6 18 18 6",
  external: "M14 3h7v7 M21 3 10 14 M10 3H3v18h18v-7",
  shield: "M12 2 3 6v6c0 5 9 10 9 10s9-5 9-10V6l-9-4z M8 12l3 3 5-6",
  logout: "M9 3H3v18h6 M9 12h12 M16 7l5 5-5 5",
  message: "M3 3h18v14H8l-5 4V3z M7 8h10 M7 12h6",
  key: "M15 3a6 6 0 1 1-4.8 9.6L3 20v1h4v-3h3v-3l1.2-1.2 M16 7h.01",
} as const;

export function Icon({
  name,
  size = 18,
  style,
  className,
}: {
  name: keyof typeof paths;
  size?: number;
  style?: CSSProperties;
  className?: string;
}) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.6"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      style={style}
      className={className}
    >
      <path d={paths[name]} />
    </svg>
  );
}
