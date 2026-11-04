/**
 * Decorative routing diagram for the login page: local ports on the left pass
 * through FastProxy and leave via the upstream each one is bound to.
 * Pure SVG on theme tokens, so it follows light/dark; the packet animation is
 * CSS (`.fp-packet` in index.css) and stops under prefers-reduced-motion.
 */
const ROWS = [70, 140, 210, 280];
const PORTS = [":10001", ":10002", ":10003", ":10004"];
const EXITS = ["HK", "JP", "SG", "US"];
// port index -> exit index; crossings show that binding is per port.
const ROUTES: [number, number][] = [
  [0, 1],
  [1, 0],
  [2, 3],
  [3, 2],
];
const HUB = { x: 240, y: 175 };

function routePath(from: number, to: number) {
  const y1 = ROWS[from];
  const y2 = ROWS[to];
  return `M104 ${y1} C150 ${y1} 168 ${HUB.y} 204 ${HUB.y} L276 ${HUB.y} C312 ${HUB.y} 330 ${y2} 376 ${y2}`;
}

export function LoginTopology({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 480 350" className={className} aria-hidden="true" focusable="false">
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

      {PORTS.map((label, i) => (
        <g key={label}>
          <rect
            x={20}
            y={ROWS[i] - 14}
            width={84}
            height={28}
            rx={7}
            className="fill-background stroke-border"
          />
          <text
            x={62}
            y={ROWS[i] + 4}
            textAnchor="middle"
            className="fill-foreground font-mono"
            fontSize={11.5}
          >
            {label}
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
            x={376}
            y={ROWS[i] - 14}
            width={84}
            height={28}
            rx={7}
            className="fill-background stroke-border"
          />
          <circle cx={393} cy={ROWS[i]} r={3.5} className="fill-emerald-500" />
          <text x={406} y={ROWS[i] + 4} className="fill-foreground font-mono" fontSize={11.5}>
            {label}
          </text>
        </g>
      ))}
    </svg>
  );
}
