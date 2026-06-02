import { IconLoader2 } from "@tabler/icons-react"
import { useQuery } from "@tanstack/react-query"
import {
  CategoryScale,
  Chart as ChartJS,
  Legend,
  LinearScale,
  LineElement,
  PointElement,
  Title,
  Tooltip,
} from "chart.js"
import dayjs from "dayjs"
import { useState } from "react"
import { Line } from "react-chartjs-2"

import {
  type DeltaNeutralSeriesRange,
  getDeltaNeutralSnapshotSeries,
} from "@/api/agent-delta-neutral"

ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, Title, Tooltip, Legend)

const RANGES: { label: string; value: DeltaNeutralSeriesRange }[] = [
  { label: "7D", value: "7d" },
  { label: "14D", value: "14d" },
  { label: "30D", value: "30d" },
  { label: "3M", value: "3m" },
  { label: "6M", value: "6m" },
  { label: "All", value: "all" },
]

export function DeltaNeutralYieldChart({ planId }: { planId: number }) {
  const [range, setRange] = useState<DeltaNeutralSeriesRange>("7d")

  const { data, isLoading } = useQuery({
    queryKey: ["dn-series", planId, range],
    queryFn: () => getDeltaNeutralSnapshotSeries(planId, { range }),
  })

  const shortRange = range === "7d" || range === "14d"
  const labels =
    data?.map((d) => dayjs(d.t).format(shortRange ? "M/D HH:mm" : "MMM D")) ?? []

  const chartData = {
    labels,
    datasets: [
      {
        label: "Funding Rate",
        data: data?.map((d) => d.funding_rate) ?? [],
        yAxisID: "yLeft",
        borderColor: "#6366f1",
        backgroundColor: "rgba(99,102,241,0.08)",
        borderWidth: 1.5,
        pointRadius: 0,
        pointHoverRadius: 4,
        tension: 0.3,
      },
      {
        label: "Funding APY%",
        data: data?.map((d) => d.funding_apy) ?? [],
        yAxisID: "yRight",
        borderColor: "#10b981",
        backgroundColor: "rgba(16,185,129,0.08)",
        borderWidth: 1.5,
        pointRadius: 0,
        pointHoverRadius: 4,
        tension: 0.3,
      },
      {
        label: "Earn APY%",
        data: data?.map((d) => d.earn_apy) ?? [],
        yAxisID: "yRight",
        borderColor: "#f59e0b",
        backgroundColor: "rgba(245,158,11,0.08)",
        borderWidth: 1.5,
        pointRadius: 0,
        pointHoverRadius: 4,
        tension: 0.3,
      },
      {
        label: "Combined APY%",
        data: data?.map((d) => d.combined_apy) ?? [],
        yAxisID: "yRight",
        borderColor: "#ef4444",
        backgroundColor: "rgba(239,68,68,0.08)",
        borderWidth: 2,
        pointRadius: 0,
        pointHoverRadius: 4,
        tension: 0.3,
      },
    ],
  }

  const options: Parameters<typeof Line>[0]["options"] = {
    responsive: true,
    maintainAspectRatio: false,
    interaction: {
      mode: "index",
      intersect: false,
    },
    plugins: {
      legend: {
        position: "bottom",
        labels: {
          boxWidth: 20,
          boxHeight: 2,
          padding: 12,
          font: { size: 11 },
          color: "rgba(156,163,175,1)",
          usePointStyle: true,
          pointStyle: "line",
        },
      },
      tooltip: {
        backgroundColor: "rgba(15,15,20,0.92)",
        borderColor: "rgba(255,255,255,0.1)",
        borderWidth: 1,
        padding: 10,
        titleFont: { size: 11 },
        bodyFont: { size: 11 },
        titleColor: "rgba(156,163,175,1)",
        bodyColor: "rgba(229,231,235,1)",
        callbacks: {
          label(ctx) {
            const raw = ctx.parsed.y ?? 0
            if (ctx.dataset.yAxisID === "yLeft") {
              return ` ${ctx.dataset.label}: ${raw.toFixed(6)}`
            }
            return ` ${ctx.dataset.label}: ${raw.toFixed(4)}%`
          },
        },
      },
    },
    scales: {
      x: {
        grid: { color: "rgba(255,255,255,0.05)" },
        ticks: {
          color: "rgba(156,163,175,0.8)",
          font: { size: 10 },
          maxRotation: 0,
          maxTicksLimit: 8,
        },
      },
      yLeft: {
        type: "linear",
        position: "left",
        grid: { color: "rgba(255,255,255,0.05)" },
        ticks: {
          color: "#6366f1",
          font: { size: 10 },
          callback(v) {
            const n = Number(v)
            if (Math.abs(n) >= 0.01) return n.toFixed(3)
            if (Math.abs(n) >= 0.001) return n.toFixed(4)
            return n.toFixed(5)
          },
        },
        title: {
          display: true,
          text: "Funding Rate",
          color: "rgba(99,102,241,0.7)",
          font: { size: 10 },
        },
      },
      yRight: {
        type: "linear",
        position: "right",
        grid: { drawOnChartArea: false },
        ticks: {
          color: "rgba(156,163,175,0.8)",
          font: { size: 10 },
          callback(v) {
            return `${Number(v).toFixed(1)}%`
          },
        },
        title: {
          display: true,
          text: "APY %",
          color: "rgba(156,163,175,0.7)",
          font: { size: 10 },
        },
      },
    },
  }

  return (
    <div className="overflow-hidden rounded-lg border">
      <div className="border-border/50 flex items-center justify-between border-b px-3 py-2">
        <span className="text-foreground/80 text-sm font-medium">Yield History</span>
        <div className="flex gap-0.5">
          {RANGES.map((r) => (
            <button
              key={r.value}
              onClick={() => setRange(r.value)}
              className={`rounded px-2 py-0.5 text-xs font-medium transition-colors ${
                range === r.value
                  ? "bg-accent text-foreground"
                  : "text-muted-foreground hover:bg-muted/60"
              }`}
            >
              {r.label}
            </button>
          ))}
        </div>
      </div>

      <div className="p-3">
        {isLoading ? (
          <div className="flex h-56 items-center justify-center">
            <IconLoader2 className="text-muted-foreground size-4 animate-spin" />
          </div>
        ) : !data || data.length === 0 ? (
          <div className="flex h-56 items-center justify-center">
            <span className="text-muted-foreground text-sm">
              No data for this range yet. Collected on each monitoring interval.
            </span>
          </div>
        ) : (
          <div className="h-56">
            <Line data={chartData} options={options} />
          </div>
        )}
      </div>
    </div>
  )
}
