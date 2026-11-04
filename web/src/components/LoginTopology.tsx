import { useI18n } from "@/i18n";

/**
 * Decorative routing diagram for the login page:
 * Ports are entry listeners; accounts on a port (default or custom) decide
 * which upstream proxy the traffic exits through.
 */
const ROWS = [82, 152, 222, 292];
const INBOUNDS: { port: string; account: string }[] = [
  { port: ":10001", account: "default" },
  { port: ":10001", account: "alice" },
  { port: ":10002", account: "bob" },
  { port: ":10003", account: "dev" },
];
const EXITS = ["HK · VLESS", "JP · SOCKS5", "SG · SS", "US · HTTP"];
// inbound index -> exit index; crossings show per-account binding (even on the same port :10001).
const ROUTES: [number, number][] = [
  [0, 1],
  [1, 0],
  [2, 3],
  [3, 2],
];
const HUB = { x: 260, y: 187 };

function routePath(from: number, to: number) {
  const y1 = ROWS[from];
  const y2 = ROWS[to];
  return `M152 ${y1} C186 ${y1} 198 ${HUB.y} 224 ${HUB.y} L296 ${HUB.y} C322 ${HUB.y} 334 ${y2} 368 ${y2}`;
}

export function LoginTopology({ className }: { className?: string }) {
  const { t } = useI18n();
  return (
    <svg viewBox="0 0 520 340" className={className} aria-hidden="true" focusable="false">
      <text
        x={84}
        y={42}
        textAnchor="middle"
        className="fill-muted-foreground font-sans"
        fontSize={11.5}
        fontWeight={500}
      >
        {t("login.diagram.inbound")}
      </text>
      <text
        x={436}
        y={42}
        textAnchor="middle"
        className="fill-muted-foreground font-sans"
        fontSize={11.5}
        fontWeight={500}
      >
        {t("login.diagram.outbound")}
      </text>

      <g fill="none" strokeWidth={1.25} strokeLinecap="round">
        {ROUTES.map(([a, b]) => (
          <path key={`l${a}`} d={routePath(a, b)} className="stroke-foreground/20" />
        ))}
        {ROUTES.map(([a, b], i) => (
          <path
            key={`p${a}`}
            d={routePath(a, b)}
            pathLength={100}
            className="fp-packet stroke-foreground"
            strokeWidth={2}
            style={{ animationDelay: `${i * -0.9}s` }}
          />
        ))}
      </g>

      {INBOUNDS.map(({ port, account }, i) => (
        <g key={`${port}-${account}`}>
          <rect
            x={16}
            y={ROWS[i] - 16}
            width={136}
            height={32}
            rx={8}
            className="fill-background stroke-border"
          />
          <text
            x={84}
            y={ROWS[i] + 4}
            textAnchor="middle"
            className="font-mono"
            fontSize={11.5}
          >
            <tspan className="fill-muted-foreground">{port}</tspan>
            <tspan className="fill-muted-foreground/60"> · </tspan>
            <tspan className="fill-foreground" fontWeight={500}>
              {account}
            </tspan>
          </text>
        </g>
      ))}

      <g>
        <rect
          x={HUB.x - 36}
          y={HUB.y - 36}
          width={72}
          height={72}
          rx={16}
          className="fill-primary"
        />
        {/* lucide "zap", scaled into the hub */}
        <svg x={HUB.x - 14} y={HUB.y - 14} width={28} height={28} viewBox="0 0 24 24">
          <path
            d="M4 14a1 1 0 0 1-.78-1.63l9.9-10.2a.5.5 0 0 1 .86.46l-1.92 6.02A1 1 0 0 0 13 10h7a1 1 0 0 1 .78 1.63l-9.9 10.2a.5.5 0 0 1-.86-.46l1.92-6.02A1 1 0 0 0 11 14z"
            className="fill-primary-foreground"
          />
        </svg>
      </g>

      {EXITS.map((label, i) => (
        <g key={label}>
          <rect
            x={368}
            y={ROWS[i] - 16}
            width={136}
            height={32}
            rx={8}
            className="fill-background stroke-border"
          />
          <circle cx={386} cy={ROWS[i]} r={3.5} className="fill-emerald-500" />
          <text x={398} y={ROWS[i] + 4} className="fill-foreground font-mono" fontSize={11.5}>
            {label}
          </text>
        </g>
      ))}
    </svg>
  );
}
