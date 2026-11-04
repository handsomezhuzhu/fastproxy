import * as React from "react";
import { Area, AreaChart, CartesianGrid, XAxis, YAxis } from "recharts";
import {
  ChartContainer,
  ChartLegend,
  ChartLegendContent,
  ChartTooltip,
  ChartTooltipContent,
  type ChartConfig,
} from "@/components/ui/chart";
import { bytes } from "@/lib/utils";
import { useI18n } from "@/i18n";
import type { MetricPoint } from "@/api";

/** 24h connections + traffic area chart (shadcn Chart / Recharts). Each series uses its own Y axis. */
export function MetricsChart({ points }: { points: MetricPoint[] }) {
  const { t } = useI18n();
  const chartConfig = React.useMemo(
    () =>
      ({
        conns: { label: t("dashboard.chart.conns"), color: "var(--chart-1)" },
        traffic: { label: t("dashboard.chart.traffic"), color: "var(--chart-2)" },
      }) satisfies ChartConfig,
    [t]
  );
  const rows = React.useMemo(
    () =>
      points.map((p) => ({
        t: p.t,
        conns: p.conns || 0,
        traffic: (p.bytes_in || 0) + (p.bytes_out || 0),
      })),
    [points]
  );

  return (
    <ChartContainer config={chartConfig} className="aspect-auto h-[250px] w-full">
      <AreaChart data={rows} margin={{ left: 0, right: 0, top: 8 }}>
        <defs>
          <linearGradient id="fillConns" x1="0" y1="0" x2="0" y2="1">
            <stop offset="5%" stopColor="var(--color-conns)" stopOpacity={0.8} />
            <stop offset="95%" stopColor="var(--color-conns)" stopOpacity={0.1} />
          </linearGradient>
          <linearGradient id="fillTraffic" x1="0" y1="0" x2="0" y2="1">
            <stop offset="5%" stopColor="var(--color-traffic)" stopOpacity={0.6} />
            <stop offset="95%" stopColor="var(--color-traffic)" stopOpacity={0.05} />
          </linearGradient>
        </defs>
        <CartesianGrid vertical={false} />
        <XAxis dataKey="t" tickLine={false} axisLine={false} tickMargin={8} minTickGap={32} />
        <YAxis yAxisId="conns" hide />
        <YAxis yAxisId="traffic" hide orientation="right" />
        <ChartTooltip
          cursor={false}
          content={
            <ChartTooltipContent
              indicator="dot"
              formatter={(value, name) => (
                <div className="flex w-full items-center justify-between gap-4">
                  <span className="text-muted-foreground">
                    {chartConfig[name as keyof typeof chartConfig]?.label ?? name}
                  </span>
                  <span className="font-mono font-medium tabular-nums text-foreground">
                    {name === "traffic" ? bytes(Number(value)) : Number(value)}
                  </span>
                </div>
              )}
            />
          }
        />
        <Area
          yAxisId="traffic"
          dataKey="traffic"
          type="natural"
          fill="url(#fillTraffic)"
          stroke="var(--color-traffic)"
        />
        <Area yAxisId="conns" dataKey="conns" type="natural" fill="url(#fillConns)" stroke="var(--color-conns)" />
        <ChartLegend content={<ChartLegendContent />} />
      </AreaChart>
    </ChartContainer>
  );
}
