import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  Activity,
  DollarSign,
  Database,
  Zap,
  Calendar,
  Clock,
  CheckCircle2,
  Timer,
  TrendingUp,
  AlertTriangle,
  Wallet,
} from "lucide-react";
import { BarChart, Bar, XAxis, YAxis, ResponsiveContainer, Tooltip } from "recharts";
import { api, type UsageInsights, type RecentActivity, type ProviderUsage } from "../lib/api";
import { microsToUSD } from "../lib/format";
import { PageHeader } from "../components/Layout";
import { Card, SectionHeader, Spinner, StatCard, ErrorCard } from "../components/ui";

const periods = [
  { value: "today", label: "Today" },
  { value: "week", label: "Last 7 days" },
  { value: "month", label: "Last 30 days" },
];

export function OverviewPage() {
  const [period, setPeriod] = useState("week");
  const insights = useQuery({
    queryKey: ["usage-insights", period],
    queryFn: () => api.usageInsights(period),
  });

  const budgets = useQuery({
    queryKey: ["budget-status"],
    queryFn: () => api.budgetStatus(),
    refetchInterval: 60_000,
  });

  const alerts = (budgets.data?.budgets ?? []).filter((b) => b.pct_used >= b.alert_pct);
  const blocked = alerts.filter((b) => b.pct_used >= 100 && b.hard_cutoff);
  const warnings = alerts.filter((b) => b.pct_used < 100 || !b.hard_cutoff);

  return (
    <>
      {/* ── Plan alerts ──────────────────────────────────────────── */}
      {blocked.length > 0 && (
        <div className="mb-6 flex items-center gap-3 rounded-xl border border-red-300 bg-red-50 px-4 py-3 dark:border-red-800 dark:bg-red-950/30">
          <AlertTriangle className="h-5 w-5 shrink-0 text-red-600 dark:text-red-400" />
          <div className="flex-1">
            <p className="text-sm font-medium text-red-800 dark:text-red-200">Plan limit reached — requests blocked</p>
            <p className="text-xs text-red-600 dark:text-red-400">
              {blocked.map((b) => `${b.scope_name} (${microsToUSD(b.limit_micros)} ${b.period})`).join(", ")}
            </p>
          </div>
          <a
            href="/plans"
            className="shrink-0 rounded-lg bg-red-100 px-3 py-1.5 text-xs font-medium text-red-700 transition-colors hover:bg-red-200 dark:bg-red-900/40 dark:text-red-300 dark:hover:bg-red-900/60"
          >
            Manage
          </a>
        </div>
      )}
      {warnings.length > 0 && blocked.length === 0 && (
        <div className="mb-6 flex items-center gap-3 rounded-xl border border-amber-300 bg-amber-50 px-4 py-3 dark:border-amber-800 dark:bg-amber-950/30">
          <Wallet className="h-5 w-5 shrink-0 text-amber-600 dark:text-amber-400" />
          <div className="flex-1">
            <p className="text-sm font-medium text-amber-800 dark:text-amber-200">Plan alert</p>
            <p className="text-xs text-amber-600 dark:text-amber-400">
              {warnings.map((b) => `${b.scope_name}: ${b.pct_used.toFixed(0)}% used`).join(", ")}
            </p>
          </div>
          <a
            href="/plans"
            className="shrink-0 rounded-lg bg-amber-100 px-3 py-1.5 text-xs font-medium text-amber-700 transition-colors hover:bg-amber-200 dark:bg-amber-900/40 dark:text-amber-300 dark:hover:bg-amber-900/60"
          >
            Manage
          </a>
        </div>
      )}

      <PageHeader
        title="Overview"
        icon={Activity}
        description="Usage and performance across all providers."
        action={
          <div className="flex items-center gap-2 rounded-lg border border-[var(--border)] bg-[var(--bg-elevated)] px-3 py-2">
            <Calendar className="h-4 w-4 text-[var(--text-muted)]" />
            <select
              value={period}
              onChange={(e) => setPeriod(e.target.value)}
              className="bg-transparent text-sm font-medium focus:outline-none cursor-pointer"
            >
              {periods.map((p) => (
                <option key={p.value} value={p.value}>
                  {p.label}
                </option>
              ))}
            </select>
          </div>
        }
      />

      {insights.isLoading ? (
        <Spinner />
      ) : insights.isError ? (
        <ErrorCard message="Failed to load usage data. Is the backend running?" />
      ) : (
        insights.data ? <InsightsDashboard data={insights.data} /> : null
      )}
    </>
  );
}

function InsightsDashboard({ data }: { data: UsageInsights }) {
  const { summary, providers, recent, series } = data;

  return (
    <div className="space-y-6">
      {/* ── Key metrics ──────────────────────────────────────────── */}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <StatCard
          icon={Activity}
          iconTone="accent"
          label="Requests"
          value={summary.total_requests.toLocaleString()}
        />
        <StatCard
          icon={DollarSign}
          iconTone="warning"
          label="Cost"
          value={`$${summary.cost_usd.toFixed(2)}`}
        />
        <StatCard
          icon={CheckCircle2}
          iconTone="accent"
          label="Success rate"
          value={`${(summary.success_rate * 100).toFixed(1)}%`}
        />
        <StatCard
          icon={Timer}
          iconTone="accent"
          label="Avg TTFT"
          value={`${Math.round(summary.avg_ttft_ms)}ms`}
        />
      </div>

      {/* ── Activity chart + Provider breakdown ──────────────────── */}
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-3">
        <Card className="lg:col-span-2">
          <SectionHeader
            title="Request volume"
            description="Requests over the selected period."
            icon={TrendingUp}
          />
          <div className="px-6 pb-6">
            <ActivityChart series={series} />
          </div>
        </Card>

        <Card>
          <SectionHeader
            title="Providers"
            description="Usage by provider."
            icon={Database}
          />
          <div className="px-6 pb-6">
            <ProviderBreakdown providers={providers} />
          </div>
        </Card>
      </div>

      {/* ── Token stats + Recent activity ────────────────────────── */}
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-3">
        <Card>
          <SectionHeader
            title="Token breakdown"
            description="Input, output, and cached tokens."
            icon={Zap}
          />
          <div className="px-6 pb-6">
            <TokenStats
              prompt={summary.prompt_tokens}
              completion={summary.completion_tokens}
              cached={summary.cached_tokens}
              cacheHits={summary.cache_hits}
            />
          </div>
        </Card>

        <Card className="lg:col-span-2">
          <SectionHeader
            title="Recent activity"
            description="Latest requests through the proxy."
            icon={Clock}
            action={
              recent.length > 0 ? (
                <span className="text-xs text-[var(--text-muted)]">
                  Last {recent.length} requests
                </span>
              ) : undefined
            }
          />
          <RecentActivityTable recent={recent} providers={providers} />
        </Card>
      </div>
    </div>
  );
}

/* ── Activity sparkline chart ────────────────────────────────────── */

function ActivityChart({ series }: { series: UsageInsights["series"] }) {
  if (series.length === 0) {
    return (
      <div className="flex h-48 items-center justify-center text-sm text-[var(--text-muted)]">
        No activity recorded for this period.
      </div>
    );
  }

  return (
    <div className="h-48">
      <ResponsiveContainer width="100%" height="100%">
        <BarChart data={series} barCategoryGap="20%">
          <XAxis
            dataKey="label"
            axisLine={false}
            tickLine={false}
            tick={{ fontSize: 11, fill: "var(--color-ink-400)" }}
            interval="preserveStartEnd"
          />
          <YAxis
            axisLine={false}
            tickLine={false}
            tick={{ fontSize: 11, fill: "var(--color-ink-400)" }}
            width={36}
          />
          <Tooltip
            cursor={{ fill: "var(--color-ink-100)" }}
            contentStyle={{
              background: "var(--bg-elevated)",
              border: "1px solid var(--border)",
              borderRadius: "8px",
              fontSize: "12px",
              padding: "6px 10px",
            }}
          />
          <Bar
            dataKey="count"
            fill="var(--color-accent-500)"
            radius={[4, 4, 0, 0]}
          />
        </BarChart>
      </ResponsiveContainer>
    </div>
  );
}

/* ── Provider breakdown ──────────────────────────────────────────── */

function ProviderBreakdown({ providers }: { providers: ProviderUsage[] }) {
  if (providers.length === 0) {
    return (
      <div className="py-8 text-center text-sm text-[var(--text-muted)]">
        No provider usage yet.
      </div>
    );
  }

  const maxRequests = Math.max(...providers.map((p) => p.total_requests));

  return (
    <div className="divide-y divide-[var(--border)]/50">
      {providers.map((p) => (
        <div key={p.provider} className="py-3 first:pt-0 last:pb-0">
          <div className="flex items-center justify-between text-sm mb-2">
            <div className="flex items-center gap-2">
              <SmallProviderIcon p={p} />
              <span className="font-medium text-[var(--text)]">{p.display_name}</span>
            </div>
            <span className="font-mono text-xs tabular-nums text-[var(--text)]">
              {p.total_requests.toLocaleString()} req
            </span>
          </div>
          <div className="h-1 bg-[var(--bg-subtle)] relative rounded-full overflow-hidden">
            <div
              className="absolute inset-y-0 left-0 bg-[var(--color-ink-400)] dark:bg-[var(--color-ink-500)] transition-all duration-500"
              style={{ width: `${maxRequests > 0 ? (p.total_requests / maxRequests) * 100 : 0}%` }}
            />
          </div>
          <div className="flex items-center justify-between text-xs text-[var(--text-muted)] mt-2 font-mono">
            <span>{compact(p.prompt_tokens + p.completion_tokens)} tks</span>
            <span>${p.cost_usd.toFixed(4)}</span>
          </div>
        </div>
      ))}
    </div>
  );
}

/* ── Token stats (no chart, just clean numbers) ──────────────────── */

function TokenStats({
  prompt,
  completion,
  cached,
  cacheHits,
}: {
  prompt: number;
  completion: number;
  cached: number;
  cacheHits: number;
}) {
  // Cached tokens are a *subset* of prompt tokens (prompt cache hits).
  // Use prompt + completion as the true total; show non-cached input vs cached
  // as separate, mutually exclusive breakdown rows.
  const nonCachedInput = Math.max(prompt - cached, 0);
  const total = prompt + completion;

  if (total === 0) {
    return (
      <div className="py-8 text-center text-sm text-[var(--text-muted)]">
        No token usage recorded yet.
      </div>
    );
  }

  const rows = [
    { label: "Input", value: nonCachedInput },
    { label: "Output", value: completion },
    { label: "Cached", value: cached },
  ];

  return (
    <div className="flex flex-col h-full">
      <div className="pb-6 mb-6 border-b border-[var(--border)]">
        <div className="flex items-baseline gap-2">
          <span className="font-display text-4xl font-semibold tracking-tight">{compact(total)}</span>
          <span className="text-xs uppercase tracking-wider text-[var(--text-muted)]">tokens</span>
        </div>
      </div>

      <div className="space-y-4 flex-1">
        {rows.map((row) => (
          <div key={row.label} className="flex items-center justify-between">
            <span className="text-sm text-[var(--text-muted)]">{row.label}</span>
            <div className="flex items-center gap-4">
              <span className="text-sm font-mono tabular-nums text-[var(--text)]">
                {row.value.toLocaleString()}
              </span>
              <span className="w-10 text-right text-xs font-mono text-[var(--text-muted)]">
                {total > 0 ? `${((row.value / total) * 100).toFixed(0)}%` : "0%"}
              </span>
            </div>
          </div>
        ))}
      </div>

      <div className="pt-4 mt-6 border-t border-[var(--border)]">
        <div className="flex items-center justify-between">
          <span className="text-sm text-[var(--text-muted)]">Cache hits</span>
          <span className="text-sm font-mono tabular-nums text-[var(--text)]">{cacheHits.toLocaleString()}</span>
        </div>
      </div>
    </div>
  );
}

/* ── Recent activity table ───────────────────────────────────────── */

function RecentActivityTable({ recent, providers }: { recent: RecentActivity[], providers: ProviderUsage[] }) {
  if (recent.length === 0) {
    return (
      <div className="px-6 py-10 text-center text-sm text-[var(--text-muted)]">
        No recent activity. Make a request through the proxy to see it here.
      </div>
    );
  }

  return (
    <div className="overflow-x-auto">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b border-[var(--border)] text-left text-xs uppercase tracking-wider text-[var(--text-muted)]">
            <th className="px-6 py-3 font-medium">Model</th>
            <th className="px-4 py-3 font-medium">Provider</th>
            <th className="px-4 py-3 text-right font-medium">Tokens</th>
            <th className="px-4 py-3 text-right font-medium">Cost</th>
            <th className="px-4 py-3 text-right font-medium">Latency</th>
            <th className="px-6 py-3 text-right font-medium">Cache</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-[var(--border)]/50">
          {recent.map((row) => (
            <tr
              key={row.id}
              className="group hover:bg-[var(--bg-subtle)]/50 transition-colors"
            >
              <td className="px-6 py-3">
                <span className="font-mono text-xs text-[var(--text)]">{row.model}</span>
              </td>
              <td className="px-4 py-3 text-xs text-[var(--text-muted)]">
                <div className="flex items-center gap-2">
                  <SmallProviderIcon p={providers.find((p) => p.provider === row.provider)} />
                  {row.provider}
                </div>
              </td>
              <td className="px-4 py-3 text-right font-mono text-xs text-[var(--text)]">
                {row.tokens.toLocaleString()}
              </td>
              <td className="px-4 py-3 text-right font-mono text-xs text-[var(--text-muted)]">
                ${row.cost_usd.toFixed(4)}
              </td>
              <td className="px-4 py-3 text-right font-mono text-xs text-[var(--text-muted)]">
                {row.latency_ms}ms
              </td>
              <td className="px-6 py-3 text-right">
                {row.cache_hit ? (
                  <span className="inline-flex items-center gap-1.5 text-xs text-emerald-600 dark:text-emerald-400 font-medium">
                    <span className="h-1.5 w-1.5 rounded-full bg-current" />
                    Hit
                  </span>
                ) : (
                  <span className="text-xs text-[var(--text-muted)]">—</span>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

/* ── Helpers ─────────────────────────────────────────────────────── */

function compact(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`;
  return String(n);
}

function SmallProviderIcon({ p }: { p?: { display_name: string; icon: string; color: string } }) {
  const [errored, setErrored] = useState(false);
  if (!p) return <div className="h-4 w-4 shrink-0 rounded-sm bg-[var(--border)]" />;
  if (errored || !p.icon) {
    return (
      <div
        className="flex h-4 w-4 shrink-0 items-center justify-center rounded-sm text-[8px] font-bold text-white"
        style={{ backgroundColor: p.color || "var(--text-muted)" }}
      >
        {p.display_name.slice(0, 1).toUpperCase()}
      </div>
    );
  }
  return (
    <img
      src={p.icon}
      alt={p.display_name}
      onError={() => setErrored(true)}
      className="h-4 w-4 shrink-0 rounded-sm object-contain"
    />
  );
}
