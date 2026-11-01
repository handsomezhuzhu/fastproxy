import * as React from "react";
import { bytes } from "@/lib/utils";
import type { MetricPoint } from "@/api";

const W = 1000;
const H = 220;
const PAD = 4;

type Series = { key: string; color: string; values: number[]; max: number };

function smoothPath(xs: number[], ys: number[]): string {
  if (xs.length < 2) return "";
  let d = `M ${xs[0].toFixed(2)} ${ys[0].toFixed(2)}`;
  for (let i = 1; i < xs.length; i++) {
    const mx = ((xs[i - 1] + xs[i]) / 2).toFixed(2);
    d += ` C ${mx} ${ys[i - 1].toFixed(2)}, ${mx} ${ys[i].toFixed(2)}, ${xs[i].toFixed(2)} ${ys[i].toFixed(2)}`;
  }
  return d;
}

/** Pure-SVG smoothed area chart for 24h metrics. Each series is normalized to its own peak. */
export function MetricsChart({ points }: { points: MetricPoint[] }) {
  const series = React.useMemo<Series[]>(() => {
    const conns = points.map((p) => p.conns || 0);
    const traffic = points.map((p) => (p.bytes_in || 0) + (p.bytes_out || 0));
    return [
      { key: "conns", color: "var(--chart-1)", values: conns, max: Math.max(1, ...conns) },
      { key: "traffic", color: "var(--chart-2)", values: traffic, max: Math.max(1, ...traffic) },
    ];
  }, [points]);

  const n = points.length;
  const xs = React.useMemo(
    () => points.map((_, i) => PAD + (i / Math.max(1, n - 1)) * (W - PAD * 2)),
    [points, n]
  );

  const paths = series.map((s) => {
    const ys = s.values.map((v) => H - PAD - (v / s.max) * (H - PAD * 2 - 24));
    const line = smoothPath(xs, ys);
    const area = `${line} L ${xs[xs.length - 1].toFixed(2)} ${H} L ${xs[0].toFixed(2)} ${H} Z`;
    return { line, area };
  });

  const labelIdx = n > 0 ? [...new Set([0, Math.floor((n - 1) / 2), n - 1])] : [];

  return (
    <div>
      <svg
        viewBox={`0 0 ${W} ${H}`}
        className="h-[220px] w-full"
        role="img"
        aria-label="近 24 小时连接数与流量趋势图"
        preserveAspectRatio="none"
      >
        <defs>
          {series.map((s) => (
            <linearGradient key={s.key} id={`grad-${s.key}`} x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor={s.color} stopOpacity="0.28" />
              <stop offset="100%" stopColor={s.color} stopOpacity="0.02" />
            </linearGradient>
          ))}
        </defs>
        {[0.25, 0.5, 0.75].map((f) => (
          <line
            key={f}
            x1="0"
            x2={W}
            y1={H * f}
            y2={H * f}
            stroke="var(--border)"
            strokeWidth="1"
            vectorEffect="non-scaling-stroke"
          />
        ))}
        {paths.map((p, i) => (
          <path key={`a-${i}`} d={p.area} fill={`url(#grad-${series[i].key})`} />
        ))}
        {paths.map((p, i) => (
          <path
            key={`l-${i}`}
            d={p.line}
            fill="none"
            stroke={series[i].color}
            strokeWidth="2"
            vectorEffect="non-scaling-stroke"
            strokeLinejoin="round"
            strokeLinecap="round"
          />
        ))}
        {points.map((p, i) => (
          <rect
            key={`h-${i}`}
            x={xs[i] - (xs[1] - xs[0]) / 2 || 0}
            y={0}
            width={xs.length > 1 ? xs[1] - xs[0] : W}
            height={H}
            fill="transparent"
          >
            <title>{`${p.t} · 连接 ${p.conns || 0} · ↑ ${bytes(p.bytes_out || 0)} ↓ ${bytes(p.bytes_in || 0)}`}</title>
          </rect>
        ))}
      </svg>
      {n > 0 ? (
        <div className="flex justify-between pt-2 text-xs tabular-nums text-muted-foreground">
          {labelIdx.map((i) => (
            <span key={i}>{points[i].t}</span>
          ))}
        </div>
      ) : null}
    </div>
  );
}
