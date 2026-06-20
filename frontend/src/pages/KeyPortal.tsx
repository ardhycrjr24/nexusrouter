import { useState } from "react";
import { useSearchParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import {
  AreaChart, Area, XAxis, YAxis, Tooltip, ResponsiveContainer, CartesianGrid
} from "recharts";
import { fetchKeyUsage, fetchKeyUsageById, APIError } from "../lib/api";
import { useBranding } from "../contexts/BrandingContext";
import { AlertTriangle, CheckCircle2, Activity, Hash, DollarSign, LogOut, Layers, Key } from "lucide-react";
import { Card, Button, Input, Spinner, ErrorCard, Badge } from "../components/ui";

export function KeyPortalPage() {
  const { branding, logoSrc } = useBranding();
  const [params, setParams] = useSearchParams();
  const activeId = params.get("id") || "";
  const activeKey = params.get("key") || "";
  const [apiKeyInput, setApiKeyInput] = useState(activeKey || activeId);

  const authValue = activeId || activeKey;
  const isIdMode = !!activeId;

  const handleLogin = (e: React.FormEvent) => {
    e.preventDefault();
    const val = apiKeyInput.trim();
    if (val) {
      if (val.startsWith("sk-")) {
        setParams({ key: val });
      } else {
        setParams({ id: val });
      }
    }
  };

  const handleLogout = () => {
    setParams({});
    setApiKeyInput("");
  };

  const { data, isLoading, isError, error } = useQuery({
    queryKey: ["key-usage", authValue, isIdMode],
    queryFn: () => isIdMode ? fetchKeyUsageById(authValue) : fetchKeyUsage(authValue),
    enabled: !!authValue,
    retry: false,
    refetchInterval: 30000,
  });

  if (!authValue) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-[var(--bg)] p-4 md:p-8">
        <div className="w-full max-w-md animate-[page-in_0.3s_ease-out]">
          <Card className="p-8 md:p-10 text-center shadow-float border-0 ring-1 ring-[var(--border)]">
            <div className="mx-auto mb-6 flex h-16 w-16 items-center justify-center rounded-2xl bg-[var(--bg-subtle)] ring-1 ring-inset ring-[var(--border)]">
              <img src={logoSrc} alt={branding.name || "NexusRouter"} className="h-8 object-contain" />
            </div>
            <h1 className="mb-2 text-2xl font-display tracking-tight text-[var(--text)]">Portal Access</h1>
            <p className="mb-8 text-sm text-[var(--text-muted)]">
              {branding.tagline || "Enter your Key or Portal ID to monitor your real-time usage and budgets."}
            </p>

            <form onSubmit={handleLogin} className="space-y-5 text-left">
              <div className="space-y-1.5">
                <label className="text-xs font-semibold uppercase tracking-widest text-[var(--text-muted)]">
                  Identifier
                </label>
                <div className="relative">
                  <div className="pointer-events-none absolute inset-y-0 left-0 flex items-center pl-3 text-[var(--text-muted)]">
                    <Key size={16} />
                  </div>
                  <Input
                    type="password"
                    value={apiKeyInput}
                    onChange={(e) => setApiKeyInput(e.target.value)}
                    placeholder="sk-... or key_..."
                    className="pl-10 h-11 bg-[var(--bg)]"
                    autoFocus
                  />
                </div>
              </div>
              <Button 
                type="submit" 
                className="w-full h-11 text-base font-medium shadow-sm transition-all hover:-translate-y-px"
                disabled={!apiKeyInput.trim()}
              >
                View Dashboard
              </Button>
            </form>
          </Card>
        </div>
      </div>
    );
  }

  if (isLoading) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-[var(--bg)]">
        <Spinner />
      </div>
    );
  }

  if (isError) {
    let msg = "Authentication failed or server error.";
    if (error instanceof APIError) {
      msg = error.message;
    }
    return (
      <div className="flex min-h-screen items-center justify-center bg-[var(--bg)] p-4">
        <div className="w-full max-w-md space-y-4 text-center animate-[page-in_0.3s_ease-out]">
          <ErrorCard message={msg} />
          <Button variant="ghost" onClick={handleLogout} className="rounded-xl">
            Return to Login
          </Button>
        </div>
      </div>
    );
  }

  const d = data!;

  return (
    <div className="min-h-screen bg-[var(--bg)] p-4 md:p-10 animate-[page-in_0.4s_ease-out]">
      <div className="mx-auto max-w-[1040px] space-y-10">
        
        {/* ── Header Section ────────────────────────────────────────── */}
        <header className="flex flex-col gap-6 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex items-center gap-5">
            <div className="flex h-16 w-16 shrink-0 items-center justify-center rounded-2xl bg-[var(--bg-elevated)] border border-[var(--border)] shadow-sm">
              <img src={logoSrc} alt={branding.name || "NexusRouter"} className="h-8 object-contain" />
            </div>
            <div>
              <h1 className="text-2xl md:text-3xl font-display font-semibold text-[var(--text)] tracking-tight">Usage Dashboard</h1>
              <div className="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1">
                <span className="text-sm text-[var(--text-muted)]">Monitoring for <strong className="font-medium text-[var(--text)]">{d.key_name}</strong></span>
                <span className="hidden h-1 w-1 rounded-full bg-[var(--border-strong)] sm:inline-block"></span>
                <span className="font-mono text-[13px] text-[var(--text-muted)] tracking-tight">ID: {d.key_id}</span>
              </div>
            </div>
          </div>
          <Button variant="ghost" onClick={handleLogout} className="shrink-0 self-start sm:self-auto hover:bg-[var(--bg-elevated)] rounded-xl px-4 py-2 border-[var(--border)] shadow-sm">
            <LogOut size={16} /> <span className="ml-1 font-medium">Disconnect</span>
          </Button>
        </header>

        {/* ── Unified Overview Panel (Budget + Stats) ───────────────── */}
        <section>
          <div className="overflow-hidden rounded-2xl border border-[var(--border)] bg-[var(--bg-elevated)] shadow-sm ring-1 ring-inset ring-white/50 dark:ring-0">
            <div className="grid grid-cols-1 lg:grid-cols-12 divide-y lg:divide-y-0 lg:divide-x divide-[var(--border)]">
              
              {/* Left Side: Allocations */}
              <div className="lg:col-span-5 flex flex-col p-8 md:p-10 relative">
                <div className="mb-10 flex items-center justify-between">
                  <h2 className="text-xs font-semibold tracking-widest text-[var(--text-muted)] uppercase">Allocations</h2>
                </div>
                
                <div className="flex-1 flex flex-col justify-center space-y-12">
                  {d.budgets && d.budgets.length > 0 ? (
                    d.budgets.map((b, i) => (
                      <div key={i} className="relative">
                        <div className="mb-8 flex items-center justify-between">
                          <div className="flex items-center gap-3.5">
                            <div className={`h-2.5 w-2.5 rounded-full ${b.alert ? 'bg-[color:var(--color-danger)] shadow-[0_0_12px_var(--color-danger)]' : 'bg-accent-500 shadow-[0_0_12px_var(--color-accent-500)]'}`} />
                            <h3 className="text-2xl font-display font-semibold tracking-tight text-[var(--text)]">
                              {b.period === 'total' ? 'All-Time' : b.period.charAt(0).toUpperCase() + b.period.slice(1)} Limit
                            </h3>
                          </div>
                          {b.alert && (
                            <Badge tone="danger">
                              <span className="flex items-center gap-1.5 py-0.5 px-1 font-medium">
                                <AlertTriangle size={14} /> Exceeded
                              </span>
                            </Badge>
                          )}
                        </div>

                        <div className="space-y-8">
                          {b.limit_tokens > 0 && (
                            <BudgetProgress 
                              label="Tokens Usage" 
                              used={b.tokens_used} limit={b.limit_tokens} pct={b.tokens_pct_used} alert={b.alert} 
                              format={formatTokens}
                            />
                          )}
                          {b.limit_usd > 0 && (
                            <BudgetProgress 
                              label="Spend (USD)" 
                              used={b.spent_usd} limit={b.limit_usd} pct={b.usd_pct_used} alert={b.alert} 
                              format={(v: number) => `$${v.toFixed(4)}`}
                              limitFormat={(v: number) => `$${v.toFixed(2)}`}
                            />
                          )}
                        </div>
                      </div>
                    ))
                  ) : (
                    <div className="flex flex-col items-center justify-center text-center pb-8">
                       <div className="mb-5 flex h-16 w-16 items-center justify-center rounded-full bg-accent-50 text-accent-600 ring-4 ring-accent-50/50">
                          <CheckCircle2 size={32} strokeWidth={1.5} />
                       </div>
                       <h3 className="text-2xl font-display font-semibold text-[var(--text)]">Unrestricted</h3>
                       <p className="mt-2 text-[var(--text-muted)] max-w-xs mx-auto">This key has no configured budget limits and can be used indefinitely.</p>
                    </div>
                  )}
                </div>
              </div>

              {/* Right Side: Cycle Activity */}
              <div className="lg:col-span-7 bg-[var(--bg-subtle)]/40 p-8 md:p-10 flex flex-col">
                <h2 className="mb-10 text-xs font-semibold tracking-widest text-[var(--text-muted)] uppercase">Current Cycle Activity</h2>
                <div className="flex-1 grid grid-cols-2 gap-x-8 gap-y-12 content-center">
                   <UnifiedStat 
                      icon={Activity} 
                      label="Total Requests" 
                      value={d.current_period.total_requests.toLocaleString()} 
                      tone="neutral"
                   />
                   <UnifiedStat 
                      icon={Hash} 
                      label="Prompt Tokens" 
                      value={formatTokens(d.current_period.prompt_tokens)} 
                      tone="neutral"
                   />
                   <UnifiedStat 
                      icon={Hash} 
                      label="Completion Tokens" 
                      value={formatTokens(d.current_period.completion_tokens)} 
                      tone="neutral"
                   />
                   <UnifiedStat 
                      icon={DollarSign} 
                      label="Accrued Cost" 
                      value={`$${d.current_period.cost_usd.toFixed(4)}`} 
                      tone={d.current_period.cost_usd > 0 ? "accent" : "neutral"}
                   />
                </div>
              </div>

            </div>
          </div>
        </section>

        {/* ── Daily Usage Chart ─────────────────────────────────────── */}
        {d.daily && d.daily.length > 0 && (
          <section className="space-y-6 pt-4">
            <SectionTitle title="30-Day Trajectory" />
            <div className="rounded-2xl border border-[var(--border)] bg-[var(--bg-elevated)] p-8 shadow-sm transition-all">
              <div className="h-[320px] w-full">
                <ResponsiveContainer width="100%" height="100%">
                  <AreaChart data={d.daily.map(dp => ({ ...dp, label: dp.date.slice(5) }))} margin={{ top: 10, right: 10, left: -20, bottom: 0 }}>
                    <defs>
                      <linearGradient id="promptTokensFill" x1="0" y1="0" x2="0" y2="1">
                        <stop offset="5%" stopColor="var(--color-chart-1)" stopOpacity={0.25} />
                        <stop offset="95%" stopColor="var(--color-chart-1)" stopOpacity={0} />
                      </linearGradient>
                      <linearGradient id="completionTokensFill" x1="0" y1="0" x2="0" y2="1">
                        <stop offset="5%" stopColor="var(--color-chart-2)" stopOpacity={0.25} />
                        <stop offset="95%" stopColor="var(--color-chart-2)" stopOpacity={0} />
                      </linearGradient>
                    </defs>
                    <CartesianGrid vertical={false} stroke="var(--border)" strokeDasharray="4 4" opacity={0.6} />
                    <XAxis dataKey="label" tick={{ fontSize: 12, fill: "var(--text-muted)", fontFamily: "var(--font-sans)" }} tickLine={false} axisLine={false} dy={14} />
                    <YAxis tick={{ fontSize: 12, fill: "var(--text-muted)", fontFamily: "var(--font-sans)" }} tickLine={false} axisLine={false} tickFormatter={formatTokens} width={60} />
                    <Tooltip
                      contentStyle={{ fontSize: 14, background: "var(--bg-elevated)", border: "1px solid var(--border)", borderRadius: 16, boxShadow: "var(--shadow-pop)", padding: "12px 16px" }}
                      formatter={(value: number, name: string) => [
                        <span className="font-semibold text-[var(--text)]">{formatTokens(value)}</span>, 
                        name === "prompt_tokens" ? "Input Tokens" : name === "completion_tokens" ? "Output Tokens" : name
                      ]}
                      labelStyle={{ color: "var(--text-muted)", marginBottom: 8, fontWeight: 500 }}
                    />
                    <Area type="monotone" dataKey="prompt_tokens" stackId="1" stroke="var(--color-chart-1)" strokeWidth={3} fill="url(#promptTokensFill)" name="Input Tokens" />
                    <Area type="monotone" dataKey="completion_tokens" stackId="1" stroke="var(--color-chart-2)" strokeWidth={3} fill="url(#completionTokensFill)" name="Output Tokens" />
                  </AreaChart>
                </ResponsiveContainer>
              </div>
            </div>
          </section>
        )}

        {/* ── Per-Model Breakdown ────────────────────────────────────── */}
        {d.models && d.models.length > 0 && (
          <section className="space-y-6 pt-4">
            <SectionTitle title="Model Matrix" icon={<Layers size={18} />} />
            <div className="rounded-2xl border border-[var(--border)] bg-[var(--bg-elevated)] overflow-hidden shadow-sm">
              <div className="overflow-x-auto">
                <table className="w-full text-sm">
                  <thead className="bg-[var(--bg-subtle)]/50 border-b border-[var(--border)]">
                    <tr>
                      <th className="px-8 py-5 text-left font-semibold tracking-wide text-xs text-[var(--text-muted)]">MODEL</th>
                      <th className="px-8 py-5 text-right font-semibold tracking-wide text-xs text-[var(--text-muted)]">REQUESTS</th>
                      <th className="px-8 py-5 text-right font-semibold tracking-wide text-xs text-[var(--text-muted)]">INPUT</th>
                      <th className="px-8 py-5 text-right font-semibold tracking-wide text-xs text-[var(--text-muted)]">OUTPUT</th>
                      <th className="px-8 py-5 text-right font-semibold tracking-wide text-xs text-[var(--text-muted)]">COST (USD)</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-[var(--border)]">
                    {d.models.map((m, i) => (
                      <tr key={i} className="transition-colors hover:bg-[var(--bg-subtle)]/30 group">
                        <td className="px-8 py-5">
                          <div className="flex items-center gap-4">
                            <ProviderIcon provider={m.provider} />
                            <div className="flex flex-col">
                              <span className="font-semibold text-[var(--text)] text-[15px]">{m.model}</span>
                              <span className="text-[13px] text-[var(--text-muted)] capitalize">{m.provider}</span>
                            </div>
                          </div>
                        </td>
                        <td className="px-8 py-5 text-right tabular-nums text-[var(--text)] font-medium text-[15px]">{m.total_requests.toLocaleString()}</td>
                        <td className="px-8 py-5 text-right tabular-nums font-mono text-[14px] text-[var(--text-muted)]">{formatTokens(m.prompt_tokens)}</td>
                        <td className="px-8 py-5 text-right tabular-nums font-mono text-[14px] text-[var(--text-muted)]">{formatTokens(m.completion_tokens)}</td>
                        <td className="px-8 py-5 text-right tabular-nums font-semibold text-[var(--text)] text-[15px] group-hover:text-accent-600 transition-colors">${m.cost_usd.toFixed(4)}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          </section>
        )}

        {/* ── Allowed Models Panel ──────────────────────────────────── */}
        {d.allowed_models && d.allowed_models.length > 0 && (
          <section className="space-y-6 pt-4 pb-12">
            <SectionTitle title="Authorized Routes" />
            <div className="rounded-2xl border border-[var(--border)] bg-[var(--bg-elevated)] p-8 shadow-sm">
              <div className="flex flex-wrap gap-3">
                {d.allowed_models.map(m => (
                  <div key={m} className="flex items-center gap-2 rounded-full border border-[var(--border)] bg-[var(--bg-subtle)]/50 px-4 py-2 text-sm font-medium text-[var(--text)] shadow-sm transition-colors hover:bg-[var(--bg-subtle)]">
                    <CheckCircle2 size={16} className="text-accent-500" />
                    <span className="font-mono text-[13px] tracking-tight">{m}</span>
                  </div>
                ))}
              </div>
            </div>
          </section>
        )}

      </div>
    </div>
  );
}

function SectionTitle({ title, icon }: { title: string, icon?: React.ReactNode }) {
  return (
    <div className="flex items-center gap-3 pl-2">
      {icon && <div className="text-[var(--text-muted)]">{icon}</div>}
      <h2 className="text-sm font-semibold tracking-widest text-[var(--text-muted)] uppercase">{title}</h2>
    </div>
  );
}

function BudgetProgress({ label, used, limit, pct, alert, format, limitFormat }: any) {
  if (limit <= 0) return null;
  const safePct = Math.min(Math.max(pct, 0), 100);
  const isWarning = safePct > 80 && !alert;
  const barColor = alert 
    ? "bg-[color:var(--color-danger)] shadow-[0_0_10px_var(--color-danger)]" 
    : isWarning ? "bg-[color:var(--color-warning)] shadow-[0_0_10px_var(--color-warning)]" : "bg-accent-500 shadow-[0_0_10px_var(--color-accent-500)]";
    
  return (
    <div className="group">
      <div className="mb-3 flex items-end justify-between">
        <span className="text-sm font-medium text-[var(--text)]">{label}</span>
        <div className="text-right">
          <span className="text-[19px] font-display font-semibold text-[var(--text)] tabular-nums tracking-tight">{format(used)}</span>
          <span className="ml-1.5 text-sm font-medium text-[var(--text-muted)]">/ {limitFormat ? limitFormat(limit) : format(limit)}</span>
        </div>
      </div>
      <div className="h-3 w-full overflow-hidden rounded-full bg-[var(--bg-subtle)] ring-1 ring-inset ring-[var(--border)] shadow-inner">
        <div 
          className={`h-full rounded-full ${barColor} transition-all duration-1000 ease-out`}
          style={{ width: `${safePct}%` }}
        />
      </div>
    </div>
  );
}

function UnifiedStat({ icon: Icon, label, value, tone = "neutral" }: { icon: any, label: string, value: string, tone?: string }) {
  const isAccent = tone === 'accent';
  return (
    <div className="flex flex-col gap-4">
      <div className={`flex h-12 w-12 items-center justify-center rounded-xl border shadow-sm ${
        isAccent 
          ? 'bg-accent-50/50 text-accent-600 border-accent-100 dark:bg-accent-900/20 dark:border-accent-900/50' 
          : 'bg-[var(--bg-elevated)] text-[var(--text-muted)] border-[var(--border)]'
      }`}>
        <Icon size={20} strokeWidth={2} />
      </div>
      <div>
        <h3 className={`text-4xl font-display font-semibold tracking-tight tabular-nums ${isAccent ? 'text-accent-600 dark:text-accent-400' : 'text-[var(--text)]'}`}>{value}</h3>
        <p className="mt-2 text-xs font-semibold uppercase tracking-widest text-[var(--text-muted)]">{label}</p>
      </div>
    </div>
  );
}

function formatTokens(n: number): string {
  if (n >= 1_000_000_000) return `${(n / 1_000_000_000).toFixed(1)}B`;
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`;
  return n.toLocaleString();
}

function ProviderIcon({ provider, className }: { provider: string, className?: string }) {
  const [errored, setErrored] = useState(false);
  const sizeClass = className || "h-10 w-10";
  if (errored) {
    return (
      <div className={`flex shrink-0 items-center justify-center rounded-xl bg-[var(--bg-elevated)] border border-[var(--border)] shadow-sm text-[11px] font-bold text-[var(--text-muted)] uppercase tracking-wider ${sizeClass}`}>
        {provider.slice(0, 2)}
      </div>
    );
  }
  return (
    <div className={`flex shrink-0 items-center justify-center rounded-xl bg-[var(--bg-elevated)] border border-[var(--border)] shadow-sm ${sizeClass} p-1.5`}>
      <img src={`/providers/${provider}.png`} alt={provider} onError={() => setErrored(true)}
        className="h-full w-full object-contain" />
    </div>
  );
}
