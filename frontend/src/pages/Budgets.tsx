import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  AlertTriangle,
  Building2,
  Check,
  ChevronDown,
  Clock3,
  DollarSign,
  Gauge,
  KeyRound,
  Lock,
  Pencil,
  Plus,
  Search,
  ShieldCheck,
  Trash2,
  Wallet,
} from "lucide-react";
import { api, type BudgetStatus, type APIKey } from "../lib/api";
import { microsToUSD, formatTokens } from "../lib/format";
import { PageHeader } from "../components/Layout";
import { useToast } from "../components/Toast";
import { FormattedTokenInput } from "../components/ModelSelect";
import {
  Card,
  SectionHeader,
  Button,
  Input,
  Select,
  Field,
  Badge,
  Spinner,
  EmptyState,
  ErrorBanner,
  Toggle,
  Modal,
} from "../components/ui";

const periods = [
  { value: "daily", label: "Daily" },
  { value: "weekly", label: "Weekly" },
  { value: "monthly", label: "Monthly" },
  { value: "total", label: "All time" },
];

function progressColor(pct: number, alertPct: number): string {
  if (pct >= 100) return "bg-red-500";
  if (pct >= alertPct) return "bg-amber-500";
  if (pct >= alertPct * 0.75) return "bg-amber-400";
  return "bg-emerald-500";
}

function parseUSD(value: string): number {
  const n = parseFloat(value);
  return Number.isFinite(n) && n > 0 ? n : 0;
}

function parseTokens(value: string): number {
  const n = parseInt(value, 10);
  return Number.isFinite(n) && n > 0 ? n : 0;
}

function clampAlertPct(value: number): number {
  if (!Number.isFinite(value)) return 80;
  return Math.min(100, Math.max(1, value));
}

export function BudgetsPage() {
  const qc = useQueryClient();
  const toast = useToast();

  const status = useQuery({
    queryKey: ["budget-status"],
    queryFn: () => api.budgetStatus(),
    refetchInterval: 30_000,
  });

  const keys = useQuery({
    queryKey: ["keys"],
    queryFn: () => api.listKeys(),
  });

  const [showCreate, setShowCreate] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);

  const remove = useMutation({
    mutationFn: (id: string) => api.deleteBudget(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["budget-status"] });
      qc.invalidateQueries({ queryKey: ["budgets"] });
      toast.success("Plan removed", "The plan has been deleted. Requests matching this scope are no longer capped.");
    },
    onError: (e: Error) => toast.error("Plan removal failed", e.message),
  });

  const budgets = status.data?.budgets ?? [];
  const editingBudget = budgets.find((b) => b.id === editingId);

  return (
    <>
      <PageHeader
        title="Plans"
        icon={Wallet}
        description="Spend and token limits per key or tenant. Requests are auto-blocked when a plan is exhausted."
        action={
          <Button onClick={() => setShowCreate(true)}>
            <Plus className="h-4 w-4" />
            New plan
          </Button>
        }
      />

      {/* ── Alerts ─────────────────────────────────────────────── */}
      {budgets.some((b) => b.pct_used >= 100 && b.hard_cutoff) && (
        <Card className="mb-6 border-red-300 bg-red-50 dark:border-red-800 dark:bg-red-950/30">
          <div className="flex items-center gap-3 px-6 py-4">
            <AlertTriangle className="h-5 w-5 shrink-0 text-red-600 dark:text-red-400" />
            <div>
              <p className="text-sm font-medium text-red-800 dark:text-red-200">
                Plan limit reached — requests are being blocked
              </p>
              <p className="text-xs text-red-600 dark:text-red-400">
                {budgets
                  .filter((b) => (b.limit_micros > 0 && b.pct_used >= 100) || (b.limit_tokens > 0 && b.tokens_pct_used >= 100))
                  .filter((b) => b.hard_cutoff)
                  .map((b) => `${b.scope_name} (${b.period})`)
                  .join(", ")}
              </p>
            </div>
          </div>
        </Card>
      )}

      {budgets.some((b) => {
        const usdAlert = b.limit_micros > 0 && b.pct_used >= b.alert_pct && b.pct_used < 100;
        const tokAlert = b.limit_tokens > 0 && b.tokens_pct_used >= b.alert_pct && b.tokens_pct_used < 100;
        return usdAlert || tokAlert;
      }) && (
        <Card className="mb-6 border-amber-300 bg-amber-50 dark:border-amber-800 dark:bg-amber-950/30">
          <div className="flex items-center gap-3 px-6 py-4">
            <AlertTriangle className="h-5 w-5 shrink-0 text-amber-600 dark:text-amber-400" />
            <div>
              <p className="text-sm font-medium text-amber-800 dark:text-amber-200">
                Plan alert threshold reached
              </p>
              <p className="text-xs text-amber-600 dark:text-amber-400">
                {budgets
                  .filter((b) => {
                    const usdAlert = b.limit_micros > 0 && b.pct_used >= b.alert_pct && b.pct_used < 100;
                    const tokAlert = b.limit_tokens > 0 && b.tokens_pct_used >= b.alert_pct && b.tokens_pct_used < 100;
                    return usdAlert || tokAlert;
                  })
                  .map((b) => {
                    const parts = [];
                    if (b.limit_micros > 0) parts.push(`$${b.pct_used.toFixed(0)}%`);
                    if (b.limit_tokens > 0) parts.push(`tok ${b.tokens_pct_used.toFixed(0)}%`);
                    return `${b.scope_name}: ${parts.join(" / ")}`;
                  })
                  .join(", ")}
              </p>
            </div>
          </div>
        </Card>
      )}

      {/* ── Create Modal ────────────────────────────────────────── */}
      <Modal
        open={showCreate}
        onClose={() => setShowCreate(false)}
        title="Create plan"
        subtitle="Set spend and token limits for a scope and period."
        maxWidth="max-w-xl"
      >
        <CreateBudgetForm
          keys={keys.data?.keys ?? []}
          onClose={() => setShowCreate(false)}
        />
      </Modal>

      {/* ── Edit Modal ──────────────────────────────────────────── */}
      <Modal
        open={!!editingId}
        onClose={() => setEditingId(null)}
        title="Edit plan"
        subtitle={editingBudget ? `Editing ${editingBudget.scope_name} ${editingBudget.period} plan` : undefined}
        maxWidth="max-w-xl"
      >
        {editingBudget && (
          <EditBudgetForm
            key={editingBudget.id}
            budget={editingBudget}
            onClose={() => setEditingId(null)}
          />
        )}
      </Modal>

      {/* ── Budget list ────────────────────────────────────────── */}
      <Card>
        <SectionHeader
          title="Active plans"
          description="Spend limits with live usage tracking."
          icon={ShieldCheck}
        />
        {status.isLoading ? (
          <div className="px-6 pb-6">
            <Spinner />
          </div>
        ) : budgets.length === 0 ? (
          <div className="px-6 pb-6">
            <EmptyState title="No plans set" hint="Spending is unlimited until you add a plan." />
          </div>
        ) : (
          <div className="divide-y divide-[var(--border)]">
            {budgets.map((b) => (
              <BudgetRow
                key={b.id}
                budget={b}
                onEdit={() => setEditingId(b.id)}
                onDelete={() => {
                  if (confirm(`Remove this ${microsToUSD(b.limit_micros)} ${b.period} plan?`)) {
                    remove.mutate(b.id);
                  }
                }}
              />
            ))}
          </div>
        )}
      </Card>
    </>
  );
}

/* ── Budget row ──────────────────────────────────────────────────── */

function BudgetRow({
  budget: b,
  onEdit,
  onDelete,
}: {
  budget: BudgetStatus;
  onEdit: () => void;
  onDelete: () => void;
}) {
  const remaining = Math.max(0, b.limit_micros - b.spent_micros);
  const overLimit = b.pct_used >= 100;

  return (
    <div className="px-6 py-5">
      <div className="flex items-start justify-between gap-4">
        <div className="min-w-0 flex-1">
          {/* Header badges */}
          <div className="flex flex-wrap items-center gap-2">
              <span className="text-sm font-medium">
                {b.limit_micros > 0 ? microsToUSD(b.limit_micros) : "—"}
                {b.limit_tokens > 0 && (
                  <span className="ml-2 text-[var(--text-muted)]">{formatTokens(b.limit_tokens)} tok</span>
                )}
              </span>
            <Badge>{b.period}</Badge>
            <Badge tone={b.scope_kind === "api_key" ? "accent" : "neutral"}>
              {b.scope_kind === "api_key" ? (
                <span className="flex items-center gap-1">
                  <KeyRound className="h-3 w-3" />
                  {b.scope_name}
                </span>
              ) : (
                <span className="flex items-center gap-1">
                  <Building2 className="h-3 w-3" />
                  {b.scope_name}
                </span>
              )}
            </Badge>
            {b.hard_cutoff ? (
              <Badge tone="danger">hard cutoff</Badge>
            ) : (
              <Badge tone="neutral">advisory</Badge>
            )}
            {overLimit && b.hard_cutoff && (
              <Badge tone="danger">BLOCKING</Badge>
            )}
          </div>

          {/* USD Progress bar */}
          {b.limit_micros > 0 && (
            <div className="mt-3">
              <div className="flex items-center justify-between text-xs">
                <span className="text-[var(--text-muted)]">
                  {microsToUSD(b.spent_micros)} spent
                </span>
                <span className={overLimit ? "font-medium text-red-600 dark:text-red-400" : "text-[var(--text-muted)]"}>
                  {b.pct_used.toFixed(1)}% used
                </span>
              </div>
              <div className="mt-1.5 relative h-2.5 overflow-hidden rounded-full bg-[var(--bg-subtle)]">
                <div
                  className="absolute top-0 bottom-0 w-px bg-amber-400/60 z-10"
                  style={{ left: `${Math.min(b.alert_pct, 100)}%` }}
                  title={`Alert at ${b.alert_pct}%`}
                />
                <div
                  className={`h-full rounded-full transition-all duration-700 ${progressColor(b.pct_used, b.alert_pct)}`}
                  style={{ width: `${Math.min(b.pct_used, 100)}%` }}
                />
              </div>
              <div className="mt-1.5 flex items-center justify-between text-xs text-[var(--text-muted)]">
                <span>{microsToUSD(remaining)} remaining</span>
                <span>alert at {b.alert_pct}%</span>
              </div>
            </div>
          )}
          {/* Token Progress bar */}
          {b.limit_tokens > 0 && (
            <div className="mt-3">
              <div className="flex items-center justify-between text-xs">
                <span className="text-[var(--text-muted)]">
                  {formatTokens(b.spent_tokens)} tokens used
                </span>
                <span className={b.tokens_pct_used >= 100 ? "font-medium text-red-600 dark:text-red-400" : "text-[var(--text-muted)]"}>
                  {b.tokens_pct_used.toFixed(1)}% used
                </span>
              </div>
              <div className="mt-1.5 relative h-2.5 overflow-hidden rounded-full bg-[var(--bg-subtle)]">
                <div
                  className="absolute top-0 bottom-0 w-px bg-amber-400/60 z-10"
                  style={{ left: `${Math.min(b.alert_pct, 100)}%` }}
                  title={`Alert at ${b.alert_pct}%`}
                />
                <div
                  className={`h-full rounded-full transition-all duration-700 ${progressColor(b.tokens_pct_used, b.alert_pct)}`}
                  style={{ width: `${Math.min(b.tokens_pct_used, 100)}%` }}
                />
              </div>
              <div className="mt-1.5 flex items-center justify-between text-xs text-[var(--text-muted)]">
                <span>{formatTokens(Math.max(0, b.limit_tokens - b.spent_tokens))} remaining</span>
                <span>alert at {b.alert_pct}%</span>
              </div>
            </div>
          )}
        </div>

        {/* Actions */}
        <div className="flex shrink-0 items-center gap-1.5">
          <Button variant="ghost" onClick={onEdit} className="px-2" title="Edit plan">
            <Pencil className="h-4 w-4" />
          </Button>
          <Button variant="danger" onClick={onDelete} className="px-2" title="Remove plan">
            <Trash2 className="h-4 w-4" />
          </Button>
        </div>
      </div>
    </div>
  );
}

/* ── API key searchable select ────────────────────────────────────── */

function APIKeySearchSelect({
  keys,
  value,
  onChange,
}: {
  keys: APIKey[];
  value: string;
  onChange: (id: string) => void;
}) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const triggerRef = useRef<HTMLDivElement>(null);
  const dropdownRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const [rect, setRect] = useState<DOMRect | null>(null);

  const selected = useMemo(() => keys.find((k) => k.id === value), [keys, value]);
  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return keys;
    return keys.filter((k) => {
      const allowed = (k.allowed_models ?? []).join(" ");
      return `${k.name} ${k.display} ${allowed}`.toLowerCase().includes(q);
    });
  }, [keys, query]);

  const updateRect = useCallback(() => {
    if (triggerRef.current) setRect(triggerRef.current.getBoundingClientRect());
  }, []);

  useEffect(() => {
    if (!open) return;
    updateRect();
    const onScroll = () => updateRect();
    const onResize = () => updateRect();
    window.addEventListener("scroll", onScroll, true);
    window.addEventListener("resize", onResize);
    return () => {
      window.removeEventListener("scroll", onScroll, true);
      window.removeEventListener("resize", onResize);
    };
  }, [open, updateRect]);

  useEffect(() => {
    if (!open) return;
    const handleClick = (e: MouseEvent) => {
      const target = e.target as globalThis.Node;
      if (triggerRef.current?.contains(target)) return;
      if (dropdownRef.current?.contains(target)) return;
      setOpen(false);
      setQuery("");
    };
    const handleKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        setOpen(false);
        setQuery("");
      }
    };
    document.addEventListener("mousedown", handleClick);
    document.addEventListener("keydown", handleKey);
    return () => {
      document.removeEventListener("mousedown", handleClick);
      document.removeEventListener("keydown", handleKey);
    };
  }, [open]);

  useEffect(() => {
    if (open) inputRef.current?.focus();
  }, [open]);

  const choose = (id: string) => {
    onChange(id);
    setOpen(false);
    setQuery("");
  };

  const dropdown = open && rect
    ? createPortal(
        <div
          ref={dropdownRef}
          onMouseDown={(e) => e.stopPropagation()}
          className="fixed z-[100] overflow-hidden rounded-xl border border-[var(--border)] bg-[var(--bg-elevated)] shadow-[var(--shadow-float)]"
          style={{ top: rect.bottom + 6, left: rect.left, width: Math.max(rect.width, 360), maxHeight: 420 }}
        >
          <div className="border-b border-[var(--border)] p-2">
            <div className="flex items-center gap-2 rounded-lg bg-[var(--bg-subtle)] px-2.5 py-2">
              <Search className="h-3.5 w-3.5 shrink-0 text-[var(--text-muted)]" />
              <input
                ref={inputRef}
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder="Search API keys…"
                className="w-full bg-transparent text-sm outline-none placeholder:text-[var(--text-muted)]"
              />
            </div>
          </div>
          <div className="max-h-72 overflow-y-auto p-1">
            {filtered.length === 0 ? (
              <p className="px-3 py-6 text-center text-xs text-[var(--text-muted)]">No API keys found</p>
            ) : (
              filtered.map((k) => {
                const active = k.id === value;
                const modelCount = k.allowed_models?.length ?? 0;
                return (
                  <button
                    key={k.id}
                    type="button"
                    onClick={() => choose(k.id)}
                    className={`flex w-full items-start gap-3 rounded-lg px-3 py-2.5 text-left transition-colors hover:bg-[var(--bg-subtle)] ${
                      active ? "bg-accent-500/10" : ""
                    }`}
                    role="option"
                    aria-selected={active}
                  >
                    <span
                      className={`mt-0.5 flex h-5 w-5 shrink-0 items-center justify-center rounded-md border ${
                        active ? "border-accent-500 bg-accent-500 text-white" : "border-[var(--border)]"
                      }`}
                    >
                      {active && <Check className="h-3.5 w-3.5" />}
                    </span>
                    <span className="min-w-0 flex-1">
                      <span className="flex items-center gap-2">
                        <span className="truncate text-sm font-medium">{k.name}</span>
                        {k.disabled && <Badge tone="danger">disabled</Badge>}
                      </span>
                      <span className="mt-0.5 block truncate font-mono text-xs text-[var(--text-muted)]">{k.display}</span>
                      {modelCount > 0 && (
                        <span className="mt-1 block truncate text-xs text-[var(--text-muted)]">
                          {modelCount} model rule{modelCount > 1 ? "s" : ""}: {k.allowed_models?.join(", ")}
                        </span>
                      )}
                    </span>
                  </button>
                );
              })
            )}
          </div>
        </div>,
        document.body,
      )
    : null;

  return (
    <div ref={triggerRef} className="relative">
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        className="flex min-h-11 w-full items-center justify-between gap-3 rounded-xl border border-[var(--border)] bg-[var(--bg-elevated)] px-3 py-2 text-left text-sm transition-colors hover:bg-[var(--bg-subtle)] focus:outline-none focus-visible:ring-2 focus-visible:ring-accent-400/40"
        aria-haspopup="listbox"
        aria-expanded={open}
      >
        {selected ? (
          <span className="min-w-0">
            <span className="block truncate font-medium">{selected.name}</span>
            <span className="block truncate font-mono text-xs text-[var(--text-muted)]">{selected.display}</span>
          </span>
        ) : (
          <span className="text-[var(--text-muted)]">Select an API key…</span>
        )}
        <ChevronDown className={`h-4 w-4 shrink-0 text-[var(--text-muted)] transition-transform ${open ? "rotate-180" : ""}`} />
      </button>
      {dropdown}
    </div>
  );
}

function SelectedKeySummary({ keyRecord }: { keyRecord?: APIKey }) {
  if (!keyRecord) {
    return (
      <div className="rounded-xl border border-dashed border-[var(--border)] bg-[var(--bg-subtle)] px-4 py-3">
        <p className="text-sm font-medium">No API key selected</p>
        <p className="mt-1 text-xs text-[var(--text-muted)]">Pick a key to attach this plan to one credential only.</p>
      </div>
    );
  }

  const models = keyRecord.allowed_models ?? [];
  return (
    <div className="rounded-xl border border-[var(--border)] bg-[var(--bg-subtle)] px-4 py-3">
      <div className="flex flex-wrap items-center gap-2">
        <p className="text-sm font-medium">{keyRecord.name}</p>
        {keyRecord.disabled && <Badge tone="danger">disabled</Badge>}
        {models.length > 0 ? <Badge tone="accent">{models.length} model rule{models.length > 1 ? "s" : ""}</Badge> : <Badge>all models</Badge>}
      </div>
      <p className="mt-1 truncate font-mono text-xs text-[var(--text-muted)]">{keyRecord.display}</p>
      {models.length > 0 && (
        <p className="mt-2 line-clamp-2 text-xs text-[var(--text-muted)]">
          Allowed models: {models.join(", ")}
        </p>
      )}
    </div>
  );
}

function LimitFields({
  limit,
  setLimit,
  limitTokens,
  setLimitTokens,
  period,
  setPeriod,
  usdPlaceholder = "50.00",
  tokenPlaceholder = "100000000",
}: {
  limit: string;
  setLimit: (value: string) => void;
  limitTokens: string;
  setLimitTokens: (value: string) => void;
  period: string;
  setPeriod: (value: string) => void;
  usdPlaceholder?: string;
  tokenPlaceholder?: string;
}) {
  return (
    <div className="grid gap-3 sm:grid-cols-3">
      <Field label="Limit (USD)">
        <div className="relative">
          <div className="pointer-events-none absolute inset-y-0 left-0 flex items-center pl-3">
            <DollarSign className="h-4 w-4 text-[var(--text-muted)]" />
          </div>
          <Input
            type="number"
            min="0"
            step="0.01"
            value={limit}
            onChange={(e) => setLimit(e.target.value)}
            placeholder={usdPlaceholder}
            className="pl-9"
          />
        </div>
      </Field>
      <Field label="Limit (Tokens)">
        <FormattedTokenInput
          value={limitTokens}
          onChange={setLimitTokens}
          placeholder={tokenPlaceholder}
        />
      </Field>
      <Field label="Period">
        <Select value={period} onChange={(e) => setPeriod(e.target.value)}>
          {periods.map((p) => (
            <option key={p.value} value={p.value}>
              {p.label}
            </option>
          ))}
        </Select>
      </Field>
    </div>
  );
}

function GuardFields({
  alertPct,
  setAlertPct,
  hardCutoff,
  setHardCutoff,
}: {
  alertPct: number;
  setAlertPct: (value: number) => void;
  hardCutoff: boolean;
  setHardCutoff: (value: boolean) => void;
}) {
  return (
    <div className="grid gap-3 sm:grid-cols-2">
      <Field label="Alert threshold (%)">
        <div className="relative">
          <Input
            type="number"
            min="1"
            max="100"
            value={alertPct}
            onChange={(e) => setAlertPct(clampAlertPct(parseInt(e.target.value, 10)))}
            className="pr-8"
          />
          <div className="pointer-events-none absolute inset-y-0 right-0 flex items-center pr-3">
            <span className="text-sm text-[var(--text-muted)]">%</span>
          </div>
        </div>
      </Field>
      <div className="flex items-center justify-between gap-4 rounded-xl border border-[var(--border)] bg-[var(--bg-subtle)] px-4 py-3">
        <div className="min-w-0">
          <p className="text-sm font-medium">Hard cutoff</p>
          <p className="mt-0.5 text-xs text-[var(--text-muted)]">
            {hardCutoff ? "Requests stop when the plan is exhausted." : "Usage is tracked without blocking requests."}
          </p>
        </div>
        <Toggle checked={hardCutoff} onChange={setHardCutoff} />
      </div>
    </div>
  );
}

function BudgetPreview({
  scopeLabel,
  limit,
  limitTokens,
  period,
  hardCutoff,
}: {
  scopeLabel: string;
  limit: string;
  limitTokens: string;
  period: string;
  hardCutoff: boolean;
}) {
  const usd = parseUSD(limit);
  const tokens = parseTokens(limitTokens);
  return (
    <div className="rounded-xl border border-accent-200/40 bg-accent-50/30 dark:border-accent-900/40 dark:bg-accent-900/10 p-4">
      <h4 className="mb-3 text-xs font-semibold uppercase tracking-wider text-[var(--text-muted)]">Configuration Summary</h4>
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <div>
          <p className="flex items-center gap-1.5 text-xs text-[var(--text-muted)]">
            <KeyRound className="h-3.5 w-3.5" />
            Scope
          </p>
          <p className="mt-1 truncate text-sm font-medium">{scopeLabel}</p>
        </div>
        <div>
          <p className="flex items-center gap-1.5 text-xs text-[var(--text-muted)]">
            <DollarSign className="h-3.5 w-3.5" />
            USD Cap
          </p>
          <p className="mt-1 text-sm font-medium">{usd > 0 ? `$${usd.toFixed(2)}` : "No USD cap"}</p>
        </div>
        <div>
          <p className="flex items-center gap-1.5 text-xs text-[var(--text-muted)]">
            <Clock3 className="h-3.5 w-3.5" />
            Window
          </p>
          <p className="mt-1 text-sm font-medium">{periods.find((p) => p.value === period)?.label ?? period}</p>
        </div>
        <div>
          <p className="flex items-center gap-1.5 text-xs text-[var(--text-muted)]">
            <Lock className="h-3.5 w-3.5" />
            Enforcement
          </p>
          <p className="mt-1 text-sm font-medium">
            {hardCutoff ? (
              <span className="text-red-600 dark:text-red-400">Blocking</span>
            ) : (
              <span className="text-emerald-600 dark:text-emerald-400">Advisory</span>
            )}
          </p>
        </div>
        {tokens > 0 && (
          <div className="sm:col-span-2 lg:col-span-4 pt-2.5 border-t border-[var(--border)] border-dashed">
            <p className="flex items-center gap-1.5 text-xs text-[var(--text-muted)]">
              <Gauge className="h-3.5 w-3.5" />
              Token Cap
            </p>
            <p className="mt-1 text-sm font-medium">{formatTokens(tokens)} tokens</p>
          </div>
        )}
      </div>
    </div>
  );
}

/* ── Create form ─────────────────────────────────────────────────── */

function CreateBudgetForm({ keys, onClose }: { keys: APIKey[]; onClose: () => void }) {
  const qc = useQueryClient();
  const toast = useToast();

  const [scopeKind, setScopeKind] = useState<string>("tenant");
  const [scopeId, setScopeId] = useState("");
  const [limit, setLimit] = useState("");
  const [limitTokens, setLimitTokens] = useState("");
  const [period, setPeriod] = useState("monthly");
  const [alertPct, setAlertPct] = useState(80);
  const [hardCutoff, setHardCutoff] = useState(true);
  const [error, setError] = useState("");
  
  const selectedKey = useMemo(() => keys.find((k) => k.id === scopeId), [keys, scopeId]);
  const usdLimit = parseUSD(limit);
  const tokenLimit = parseTokens(limitTokens);
  const canSubmit = (usdLimit > 0 || tokenLimit > 0) && (scopeKind !== "api_key" || !!scopeId);

  const create = useMutation({
    mutationFn: () =>
      api.createBudget({
        scope_kind: scopeKind,
        scope_id: scopeKind === "api_key" && scopeId ? scopeId : undefined,
        limit_usd: usdLimit > 0 ? usdLimit : undefined,
        limit_tokens: tokenLimit > 0 ? tokenLimit : undefined,
        period,
        alert_pct: alertPct,
        hard_cutoff: hardCutoff,
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["budget-status"] });
      qc.invalidateQueries({ queryKey: ["budgets"] });
      const parts = [];
      if (usdLimit > 0) parts.push(`$${usdLimit.toFixed(2)}`);
      if (tokenLimit > 0) parts.push(`${formatTokens(tokenLimit)} tokens`);
      toast.success(
        "Plan created",
        `${parts.join(" + ")} ${period} limit set for ${scopeKind === "api_key" ? "API key" : "tenant"}.${hardCutoff ? " Requests will be blocked when exhausted." : ""}`,
      );
      onClose();
    },
    onError: (e: Error) => {
      setError(e.message);
      toast.error("Plan creation failed", e.message);
    },
  });

  return (
    <form
      className="flex flex-col max-h-[calc(100vh-10rem)]"
      onSubmit={(e) => {
        e.preventDefault();
        setError("");
        if (scopeKind === "api_key" && !scopeId) {
          setError("Select an API key before creating this plan.");
          return;
        }
        if (usdLimit <= 0 && tokenLimit <= 0) {
          setError("Set at least one positive USD or token limit.");
          return;
        }
        create.mutate();
      }}
    >
      <div className="px-5 py-4 space-y-6 overflow-y-auto min-h-0">
        
        {/* Section 1: Scope */}
        <section className="space-y-3">
          <div>
            <h3 className="text-sm font-semibold text-[var(--text-strong)]">1. Target Scope</h3>
            <p className="text-xs text-[var(--text-muted)]">Choose the scope this plan applies to.</p>
          </div>
          <div className="grid grid-cols-2 gap-3">
            <button
              type="button"
              onClick={() => {
                setScopeKind("tenant");
                setScopeId("");
              }}
              className={`rounded-xl border px-4 py-3 text-left transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-accent-400/40 ${
                scopeKind === "tenant"
                  ? "border-accent-400 bg-accent-500/10"
                  : "border-[var(--border)] bg-[var(--bg-subtle)] hover:bg-[var(--bg)]"
              }`}
            >
              <Building2 className="h-5 w-5 text-[var(--text-muted)]" />
              <span className="mt-2 block text-sm font-medium">Tenant Default</span>
              <span className="mt-0.5 block text-xs text-[var(--text-muted)]">Applies globally to the tenant</span>
            </button>
            <button
              type="button"
              onClick={() => setScopeKind("api_key")}
              className={`rounded-xl border px-4 py-3 text-left transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-accent-400/40 ${
                scopeKind === "api_key"
                  ? "border-accent-400 bg-accent-500/10"
                  : "border-[var(--border)] bg-[var(--bg-subtle)] hover:bg-[var(--bg)]"
              }`}
            >
              <KeyRound className="h-5 w-5 text-[var(--text-muted)]" />
              <span className="mt-2.5 block text-sm font-medium">Specific API Key</span>
              <span className="mt-0.5 block text-xs text-[var(--text-muted)]">Isolate spending for one credential</span>
            </button>
          </div>

          {scopeKind === "api_key" && (
            <div className="mt-3 rounded-xl border border-dashed border-[var(--border)] bg-[var(--bg-subtle)] p-3 space-y-2.5 shadow-sm">
              <p className="text-xs font-medium text-[var(--text-muted)]">Select API Key</p>
              <APIKeySearchSelect keys={keys} value={scopeId} onChange={setScopeId} />
              <SelectedKeySummary keyRecord={selectedKey} />
            </div>
          )}
        </section>

        <div className="h-px bg-[var(--border)] w-full" />

        {/* Section 2: Limits */}
        <section className="space-y-3">
          <div>
            <h3 className="text-sm font-semibold text-[var(--text-strong)]">2. Spending Limits</h3>
            <p className="text-xs text-[var(--text-muted)]">Set maximum thresholds for USD spend or token consumption.</p>
          </div>
          <LimitFields
            limit={limit}
            setLimit={setLimit}
            limitTokens={limitTokens}
            setLimitTokens={setLimitTokens}
            period={period}
            setPeriod={setPeriod}
          />
        </section>

        <div className="h-px bg-[var(--border)] w-full" />

        {/* Section 3: Enforcement */}
        <section className="space-y-3">
          <div>
            <h3 className="text-sm font-semibold text-[var(--text-strong)]">3. Enforcement Rules</h3>
            <p className="text-xs text-[var(--text-muted)]">Configure how to handle exhaustion and approaching limits.</p>
          </div>
          <GuardFields
            alertPct={alertPct}
            setAlertPct={setAlertPct}
            hardCutoff={hardCutoff}
            setHardCutoff={setHardCutoff}
          />
        </section>
        
        {/* Preview Summary */}
        <BudgetPreview
          scopeLabel={scopeKind === "api_key" ? selectedKey?.name ?? "API key" : "Default tenant"}
          limit={limit}
          limitTokens={limitTokens}
          period={period}
          hardCutoff={hardCutoff}
        />

        {error && <ErrorBanner message={error} />}
      </div>

      <div className="shrink-0 flex gap-3 border-t border-[var(--border)] bg-[var(--bg-subtle)] px-6 py-4 rounded-b-xl">
        <div className="flex-1" />
        <Button variant="ghost" type="button" onClick={onClose}>
          Cancel
        </Button>
        <Button type="submit" disabled={create.isPending || !canSubmit}>
          {create.isPending ? "Creating…" : "Create plan"}
        </Button>
      </div>
    </form>
  );
}

/* ── Edit form ───────────────────────────────────────────────────── */

function EditBudgetForm({ budget, onClose }: { budget: BudgetStatus; onClose: () => void }) {
  const qc = useQueryClient();
  const toast = useToast();

  const [limit, setLimit] = useState(budget.limit_micros > 0 ? (budget.limit_micros / 1_000_000).toFixed(2) : "");
  const [limitTokens, setLimitTokens] = useState(budget.limit_tokens > 0 ? budget.limit_tokens.toString() : "");
  const [period, setPeriod] = useState(budget.period);
  const [alertPct, setAlertPct] = useState(budget.alert_pct);
  const [hardCutoff, setHardCutoff] = useState(budget.hard_cutoff);
  const [error, setError] = useState("");
  
  const usdLimit = parseUSD(limit);
  const tokenLimit = parseTokens(limitTokens);
  const canSubmit = usdLimit > 0 || tokenLimit > 0;

  const update = useMutation({
    mutationFn: () =>
      api.updateBudget(budget.id, {
        limit_usd: usdLimit,
        limit_tokens: tokenLimit,
        period,
        alert_pct: alertPct,
        hard_cutoff: hardCutoff,
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["budget-status"] });
      qc.invalidateQueries({ queryKey: ["budgets"] });
      const parts = [];
      if (usdLimit > 0) parts.push(`$${usdLimit.toFixed(2)}`);
      if (tokenLimit > 0) parts.push(`${formatTokens(tokenLimit)} tokens`);
      toast.success(
        "Plan updated",
        `Limit changed to ${parts.join(" + ")} ${period}. ${hardCutoff ? "Hard cutoff is active." : "Advisory mode — requests won't be blocked."}`,
      );
      onClose();
    },
    onError: (e: Error) => {
      setError(e.message);
      toast.error("Plan update failed", e.message);
    },
  });

  return (
    <form
      className="flex flex-col max-h-[calc(100vh-10rem)]"
      onSubmit={(e) => {
        e.preventDefault();
        setError("");
        if (!canSubmit) {
          setError("Set at least one positive USD or token limit.");
          return;
        }
        update.mutate();
      }}
    >
      <div className="px-5 py-4 space-y-6 overflow-y-auto min-h-0">
        
        {/* Section 1: Scope */}
        <section className="space-y-3">
          <div>
            <h3 className="text-sm font-semibold text-[var(--text-strong)]">1. Target Scope</h3>
            <p className="text-xs text-[var(--text-muted)]">The scope this plan applies to (immutable).</p>
          </div>
          <div className="grid gap-3 sm:grid-cols-2">
            <div className="rounded-xl border border-[var(--border)] bg-[var(--bg-subtle)] px-4 py-3">
              <div className="flex items-center gap-2 text-xs text-[var(--text-muted)]">
                {budget.scope_kind === "api_key" ? <KeyRound className="h-3.5 w-3.5" /> : <Building2 className="h-3.5 w-3.5" />}
                Scope
              </div>
              <p className="mt-1 truncate text-sm font-medium">{budget.scope_name}</p>
              <p className="mt-1 font-mono text-xs text-[var(--text-muted)]">{budget.scope_kind}</p>
            </div>
            <div className="rounded-xl border border-[var(--border)] bg-[var(--bg-subtle)] px-4 py-3">
              <p className="text-xs font-medium text-[var(--text-muted)]">Current usage</p>
              <div className="mt-2 space-y-2">
                {budget.limit_micros > 0 && (
                  <div className="flex items-center justify-between gap-3 text-sm">
                    <span className="text-[var(--text-muted)]">USD</span>
                    <span className="font-medium tabular-nums">{budget.pct_used.toFixed(1)}%</span>
                  </div>
                )}
                {budget.limit_tokens > 0 && (
                  <div className="flex items-center justify-between gap-3 text-sm">
                    <span className="text-[var(--text-muted)]">Tokens</span>
                    <span className="font-medium tabular-nums">{budget.tokens_pct_used.toFixed(1)}%</span>
                  </div>
                )}
              </div>
            </div>
          </div>
        </section>

        <div className="h-px bg-[var(--border)] w-full" />

        {/* Section 2: Limits */}
        <section className="space-y-3">
          <div>
            <h3 className="text-sm font-semibold text-[var(--text-strong)]">2. Spending Limits</h3>
            <p className="text-xs text-[var(--text-muted)]">Set maximum thresholds for USD spend or token consumption.</p>
          </div>
          <LimitFields
            limit={limit}
            setLimit={setLimit}
            limitTokens={limitTokens}
            setLimitTokens={setLimitTokens}
            period={period}
            setPeriod={setPeriod}
            usdPlaceholder="0 = no USD cap"
            tokenPlaceholder="0"
          />
        </section>

        <div className="h-px bg-[var(--border)] w-full" />

        {/* Section 3: Enforcement */}
        <section className="space-y-3">
          <div>
            <h3 className="text-sm font-semibold text-[var(--text-strong)]">3. Enforcement Rules</h3>
            <p className="text-xs text-[var(--text-muted)]">Configure how to handle exhaustion and approaching limits.</p>
          </div>
          <GuardFields
            alertPct={alertPct}
            setAlertPct={setAlertPct}
            hardCutoff={hardCutoff}
            setHardCutoff={setHardCutoff}
          />
        </section>

        <BudgetPreview
          scopeLabel={budget.scope_name}
          limit={limit}
          limitTokens={limitTokens}
          period={period}
          hardCutoff={hardCutoff}
        />

        {error && <ErrorBanner message={error} />}
      </div>

      <div className="shrink-0 flex gap-3 border-t border-[var(--border)] bg-[var(--bg-subtle)] px-6 py-4 rounded-b-xl">
        <div className="flex-1" />
        <Button variant="ghost" type="button" onClick={onClose}>
          Cancel
        </Button>
        <Button type="submit" disabled={update.isPending || !canSubmit}>
          {update.isPending ? "Saving…" : "Save changes"}
        </Button>
      </div>
    </form>
  );
}
