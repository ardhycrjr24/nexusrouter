import { useState, useCallback } from "react";
import { useNavigate } from "react-router-dom";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { KeyRound, Plus, Copy, Check, ToggleLeft, ToggleRight, ArrowLeft, ArrowRight, Trash2, Wallet, Wrench, DollarSign, Gauge, Link2, Activity, Ban, ListFilter } from "lucide-react";
import { api, type APIKey, type CreatedKey, type Plan } from "../lib/api";
import { microsToUSD, formatTokens } from "../lib/format";
import { PageHeader } from "../components/Layout";
import { useToast } from "../components/Toast";
import { formatTokenLimit, FormattedTokenInput, ModelMultiSelect } from "../components/ModelSelect";
import {
  Card,
  CardHeader,
  Button,
  Input,
  Select,
  Field,
  Badge,
  Spinner,
  Toggle,
  Modal,
  StatCard,
} from "../components/ui";

const budgetPeriods = [
  { value: "daily", label: "Daily" },
  { value: "weekly", label: "Weekly" },
  { value: "monthly", label: "Monthly" },
  { value: "total", label: "All time" },
];

type KeySummary = {
  total: number;
  active: number;
  disabled: number;
  restricted: number;
};

function getKeySummary(keys: APIKey[] = []): KeySummary {
  return keys.reduce(
    (acc, key) => {
      acc.total += 1;
      if (key.disabled) acc.disabled += 1;
      else acc.active += 1;
      if ((key.allowed_models ?? []).length > 0) acc.restricted += 1;
      return acc;
    },
    { total: 0, active: 0, disabled: 0, restricted: 0 },
  );
}

function StatusPill({ disabled }: { disabled: boolean }) {
  if (disabled) {
    return (
      <span className="inline-flex items-center gap-1.5 px-1 py-0.5 text-[11px] font-medium text-[var(--text-muted)]">
        <span className="relative flex h-1.5 w-1.5">
          <span className="relative inline-flex h-1.5 w-1.5 rounded-full bg-red-500 opacity-60"></span>
        </span>
        Inactive
      </span>
    );
  }

  return (
    <span className="inline-flex items-center gap-1.5 px-1 py-0.5 text-[11px] font-medium text-emerald-600 dark:text-emerald-400">
      <span className="relative flex h-1.5 w-1.5">
        <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-emerald-400 opacity-75" style={{ animationDuration: '3s' }}></span>
        <span className="relative inline-flex h-1.5 w-1.5 rounded-full bg-emerald-500"></span>
      </span>
      Active
    </span>
  );
}



function KeyCopyButton({
  icon,
  label,
  value,
  copiedMessage,
  className = "",
}: {
  icon: React.ReactNode;
  label: string;
  value: string;
  copiedMessage: string;
  className?: string;
}) {
  const toast = useToast();

  return (
    <button
      type="button"
      onClick={() => {
        navigator.clipboard.writeText(value);
        toast.success(label, copiedMessage);
      }}
      className={`group inline-flex min-w-0 items-center gap-1.5 text-left transition-colors text-[var(--text-muted)] hover:text-[var(--text)] ${className}`}
    >
      <span className="shrink-0">{icon}</span>
      <span className="min-w-0 truncate font-mono text-[11px] font-medium">
        {value}
      </span>
      <Copy className="h-3 w-3 shrink-0 opacity-0 transition-opacity group-hover:opacity-100" />
    </button>
  );
}

function KeyEmptyState({ onCreate }: { onCreate: () => void }) {
  return (
    <div className="px-6 py-14">
      <div className="mx-auto max-w-md text-center">
        <div className="mx-auto flex h-12 w-12 items-center justify-center rounded-2xl bg-secondary-100 text-secondary-700 dark:bg-secondary-900/40 dark:text-secondary-200">
          <KeyRound className="h-5 w-5" />
        </div>
        <h3 className="mt-4 text-base font-semibold tracking-tight text-[var(--text)]">No API keys yet</h3>
        <p className="mt-2 text-sm leading-relaxed text-[var(--text-muted)]">
          Create a key for CLI tools, apps, or teammates. Full secrets are shown once, then stored hashed.
        </p>
        <div className="mt-5 flex justify-center">
          <Button onClick={onCreate}>
            <Plus className="h-4 w-4" />
            Create first key
          </Button>
        </div>
      </div>
    </div>
  );
}

function KeyRow({
  apiKey,
  selected,
  onSelect,
  onToggle,
  onConfigure,
  onRevoke,
  togglePending,
}: {
  apiKey: APIKey;
  selected: boolean;
  onSelect: () => void;
  onToggle: () => void;
  onConfigure: () => void;
  onRevoke: () => void;
  togglePending: boolean;
}) {
  const portalUrl = `${window.location.origin}/portal?id=${apiKey.id}`;
  const modelCount = apiKey.allowed_models?.length ?? 0;

  return (
    <article
      className={`grid gap-3 px-5 py-3.5 transition-colors sm:grid-cols-[minmax(0,1fr)_minmax(200px,0.75fr)_auto] sm:items-center sm:px-6 ${
        selected ? "bg-secondary-50/50 dark:bg-secondary-950/20" : "hover:bg-[var(--bg-subtle)]/70"
      }`}
    >
      <div className="flex min-w-0 items-center gap-4">
        <label className="flex shrink-0 cursor-pointer">
          <input
            type="checkbox"
            checked={selected}
            onChange={onSelect}
            className="h-4 w-4 rounded border-[var(--border)] accent-[var(--color-accent)]"
            aria-label={`Select ${apiKey.name}`}
          />
        </label>
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2.5">
            <h3 className="truncate text-sm font-semibold tracking-tight text-[var(--text)]">{apiKey.name}</h3>
            <StatusPill disabled={apiKey.disabled} />
            <span className="shrink-0 text-[11px] font-medium text-[var(--text-muted)]">{apiKey.plan_name || "Custom plan"}</span>
            {modelCount > 0 && (
              <span className="shrink-0 text-[11px] font-medium text-amber-600 dark:text-amber-400">
                {modelCount} model{modelCount > 1 ? "s" : ""}
              </span>
            )}
          </div>
        </div>
      </div>

      <div className="grid min-w-0 gap-1.5 pl-8 sm:pl-0">
        <KeyCopyButton icon={<KeyRound className="h-3 w-3" />} label="Key copied" value={apiKey.display} copiedMessage="Masked key identifier copied." />
        <KeyCopyButton icon={<Link2 className="h-3 w-3" />} label="Portal link copied" value={portalUrl} copiedMessage="Owner usage portal link copied." />
      </div>

      <div className="flex items-center gap-1 sm:justify-end pl-8 sm:pl-0">
        <button
          onClick={onConfigure}
          className="p-1.5 rounded hover:bg-ink-200 dark:hover:bg-ink-800 text-[var(--text-muted)] hover:text-[var(--text)] transition-colors"
          title="Configure key"
        >
          <Wrench className="h-4 w-4" />
        </button>
        <button
          onClick={onToggle}
          disabled={togglePending}
          className="p-1.5 rounded hover:bg-ink-200 dark:hover:bg-ink-800 text-[var(--text-muted)] hover:text-[var(--text)] transition-colors disabled:opacity-50"
          title={apiKey.disabled ? "Enable key" : "Disable key"}
        >
          {apiKey.disabled ? <ToggleLeft className="h-4 w-4" /> : <ToggleRight className="h-4 w-4" />}
        </button>
        <button
          onClick={onRevoke}
          className="p-1.5 rounded hover:bg-red-100 dark:hover:bg-red-900/30 text-[var(--text-muted)] hover:text-red-600 transition-colors"
          title="Revoke key"
        >
          <Trash2 className="h-4 w-4" />
        </button>
      </div>
    </article>
  );
}

export function KeysPage() {
  const qc = useQueryClient();
  const toast = useToast();
  const navigate = useNavigate();
  const keys = useQuery({ queryKey: ["keys"], queryFn: () => api.listKeys() });

  const [modalOpen, setModalOpen] = useState(false);
  const [step, setStep] = useState<1 | 2 | 3 | 4>(1);
  const [statusFilter, setStatusFilter] = useState<"all" | "active" | "inactive">("all");

  // Step 1 — name
  const [name, setName] = useState("");

  // Step 2 — budget
  const [budgetLimit, setBudgetLimit] = useState("");
  const [budgetLimitTokens, setBudgetLimitTokens] = useState("");
  const [budgetPeriod, setBudgetPeriod] = useState("monthly");
  const [budgetAlertPct, setBudgetAlertPct] = useState(80);
  const [budgetHardCutoff, setBudgetHardCutoff] = useState(true);
  const [allowedModels, setAllowedModels] = useState<string[]>([]);

  // Step 3 — result
  const [created, setCreated] = useState<CreatedKey | null>(null);
  const [copied, setCopied] = useState(false);

  const openModal = () => {
    setName("");
    setSelectedPlanId("custom");
    setCustomizePlan(false);
    setBudgetLimit("");
    setBudgetLimitTokens("");
    setBudgetPeriod("monthly");
    setBudgetAlertPct(80);
    setBudgetHardCutoff(true);
    setAllowedModels([]);
    setCreated(null);
    setCopied(false);
    setStep(1);
    setModalOpen(true);
  };

  const closeModal = () => {
    setModalOpen(false);
    if (created) {
      setCreated(null);
      setCopied(false);
    }
  };

  // Plan selection
  const plans = useQuery({ queryKey: ["plans"], queryFn: () => api.listPlans() });
  const [selectedPlanId, setSelectedPlanId] = useState<string>("custom");
  const [customizePlan, setCustomizePlan] = useState(false);

  const create = useMutation({
    mutationFn: () => {
      const isCustom = selectedPlanId === "custom";
      const hasLimit = parseFloat(budgetLimit) > 0;
      const hasTokenLimit = parseInt(budgetLimitTokens) > 0;

      const opts: Record<string, unknown> = {};
      if (!isCustom && selectedPlanId) {
        opts.plan_id = selectedPlanId;
      }
      if (isCustom || customizePlan) {
        if (hasLimit) opts.budget_limit_usd = parseFloat(budgetLimit);
        if (hasTokenLimit) opts.budget_limit_tokens = parseInt(budgetLimitTokens);
        if (hasLimit || hasTokenLimit) {
          opts.budget_period = budgetPeriod;
          opts.budget_alert_pct = budgetAlertPct;
          opts.budget_hard_cutoff = budgetHardCutoff;
        }
        if (allowedModels.length > 0) opts.allowed_models = allowedModels;
      }
      return api.createKey(name, Object.keys(opts).length > 0 ? opts as any : undefined);
    },
    onSuccess: (data) => {
      qc.invalidateQueries({ queryKey: ["keys"] });
      qc.invalidateQueries({ queryKey: ["budgets"] });
      qc.invalidateQueries({ queryKey: ["budget-status"] });
      qc.invalidateQueries({ queryKey: ["plans"] });
      setCreated(data);
      setStep(4);
      const planMsg = data.plan ? ` Plan: ${data.plan.name}.` : "";
      const parts = [];
      if (data.budget && data.budget.limit_micros > 0) parts.push(`$${(data.budget.limit_micros / 1_000_000).toFixed(2)}`);
      if (data.budget && data.budget.limit_tokens > 0) parts.push(`${(data.budget.limit_tokens / 1_000_000).toFixed(0)}M tokens`);
      const budgetMsg = parts.length > 0 ? ` Budget: ${parts.join(" + ")} / ${data.budget?.period}.` : "";
      const modelMsg = data.allowed_models?.length ? ` Models: ${data.allowed_models.join(", ")}.` : "";
      toast.success("Key created", `Copy the key below — it won't be shown again.${planMsg}${budgetMsg}${modelMsg}`);
    },
    onError: (e: Error) => toast.error("Key creation failed", e.message),
  });

  // Multi-select state
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());

  const toggleSelect = useCallback((id: string) => {
    setSelectedIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }, []);

  const toggleSelectAll = useCallback(() => {
    if (!keys.data?.keys) return;
    setSelectedIds((prev) => {
      if (prev.size === keys.data!.keys!.length) return new Set();
      return new Set(keys.data!.keys!.map((k) => k.id));
    });
  }, [keys.data]);

  const clearSelection = useCallback(() => setSelectedIds(new Set()), []);

  const bulkRemove = useMutation({
    mutationFn: (ids: string[]) => api.deleteKeys(ids),
    onSuccess: (_, ids) => {
      qc.invalidateQueries({ queryKey: ["keys"] });
      clearSelection();
      toast.success(`${ids.length} key${ids.length > 1 ? "s" : ""} revoked`, "All selected keys have been permanently deleted.");
    },
    onError: (e: Error) => toast.error("Bulk revocation failed", e.message),
  });

  const handleBulkDelete = () => {
    const ids = Array.from(selectedIds);
    if (!ids.length) return;
    if (!confirm(`Revoke ${ids.length} key${ids.length > 1 ? "s" : ""}? This cannot be undone.`)) return;
    bulkRemove.mutate(ids);
  };

  const remove = useMutation({
    mutationFn: (id: string) => api.deleteKey(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["keys"] });
      toast.success("Key revoked", "The key has been permanently deleted and can no longer authenticate requests.");
    },
    onError: (e: Error) => toast.error("Revocation failed", e.message),
  });

  const toggleDisabled = useMutation({
    mutationFn: ({ id, disabled }: { id: string; disabled: boolean }) => api.updateKey(id, { disabled }),
    onSuccess: (data) => {
      qc.invalidateQueries({ queryKey: ["keys"] });
      toast.success(
        data.disabled ? "Key disabled" : "Key enabled",
        data.disabled
          ? "Requests using this key will be rejected until re-enabled."
          : "This key can now authenticate requests again.",
      );
    },
    onError: (e: Error) => toast.error("Key update failed", e.message),
  });

  return (
    <>
      <PageHeader
        title="API Keys"
        icon={KeyRound}
        description="Manage authentication keys, owner portal links, model access, and spend controls."
        action={
          <Button onClick={openModal}>
            <Plus className="h-4 w-4" />
            New key
          </Button>
        }
      />

      <Modal
        open={modalOpen}
        onClose={closeModal}
        title={step === 4 ? "Key created" : "Create API key"}
        subtitle={
          step === 1
            ? "Name your key so you can identify it later."
            : step === 2
              ? "Choose a plan or set custom limits."
              : step === 3
                ? "Optionally override plan settings for this key."
                : undefined
        }
      >
        {step === 1 && <StepName name={name} setName={setName} onNext={() => setStep(2)} />}
        {step === 2 && (
          <StepPlanSelect
            plans={plans.data?.plans ?? []}
            selectedPlanId={selectedPlanId}
            setSelectedPlanId={setSelectedPlanId}
            onBack={() => setStep(1)}
            onNext={() => setStep(3)}
          />
        )}
        {step === 3 && (
          <StepConfigure
            selectedPlanId={selectedPlanId}
            plans={plans.data?.plans ?? []}
            customizePlan={customizePlan}
            setCustomizePlan={setCustomizePlan}
            budgetLimit={budgetLimit}
            setBudgetLimit={setBudgetLimit}
            budgetLimitTokens={budgetLimitTokens}
            setBudgetLimitTokens={setBudgetLimitTokens}
            budgetPeriod={budgetPeriod}
            setBudgetPeriod={setBudgetPeriod}
            budgetAlertPct={budgetAlertPct}
            setBudgetAlertPct={setBudgetAlertPct}
            budgetHardCutoff={budgetHardCutoff}
            setBudgetHardCutoff={setBudgetHardCutoff}
            allowedModels={allowedModels}
            setAllowedModels={setAllowedModels}
            onBack={() => setStep(2)}
            onCreate={() => create.mutate()}
            isPending={create.isPending}
          />
        )}
        {step === 4 && created && (
          <StepSuccess
            created={created}
            copied={copied}
            setCopied={setCopied}
            onClose={closeModal}
          />
        )}
      </Modal>

      {(() => {
        const summary = getKeySummary(keys.data?.keys ?? []);
        return (
          <div className="mb-5 grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
            <StatCard label="Total keys" value={String(summary.total)} icon={KeyRound} />
            <StatCard label="Active" value={String(summary.active)} icon={Activity} />
            <StatCard label="Disabled" value={String(summary.disabled)} icon={Ban} iconTone="danger" />
            <StatCard label="Restricted" value={String(summary.restricted)} icon={ListFilter} iconTone="warning" />
          </div>
        );
      })()}

      <Card className="overflow-hidden">
        <CardHeader
          title={selectedIds.size > 0 ? `${selectedIds.size} selected` : "Key inventory"}
          description="Copy identifiers, share owner portals, and control access from one place."
          action={
            selectedIds.size > 0 ? (
              <div className="flex items-center gap-2">
                <button
                  onClick={clearSelection}
                  className="rounded-lg px-3 py-1.5 text-xs font-medium text-[var(--text-muted)] transition-colors hover:text-[var(--text)]"
                >
                  Clear
                </button>
                <Button variant="danger" onClick={handleBulkDelete} disabled={bulkRemove.isPending}>
                  <Trash2 className="h-3.5 w-3.5" />
                  Revoke {selectedIds.size}
                </Button>
              </div>
            ) : undefined
          }
        />
        {keys.isLoading ? (
          <Spinner />
        ) : !keys.data?.keys?.length ? (
          <KeyEmptyState onCreate={openModal} />
        ) : (
          <div>
            {keys.data.keys.length > 0 && (
              <div className="flex items-center justify-between gap-4 border-b border-[var(--border)] bg-[var(--bg-subtle)] px-5 py-3 sm:px-6">
                <label className="flex cursor-pointer items-center gap-3">
                  <input
                    type="checkbox"
                    checked={selectedIds.size > 0 && selectedIds.size === keys.data.keys.length}
                    onChange={toggleSelectAll}
                    className="h-4 w-4 rounded border-[var(--border)] accent-[var(--color-accent)]"
                  />
                  <span className="text-[11px] font-semibold uppercase tracking-[0.16em] text-[var(--text-muted)]">
                    Select all
                  </span>
                </label>
                <div className="flex items-center gap-2 text-[11px] font-medium text-[var(--text-muted)]">
                  <span>Filter:</span>
                  <select
                    className="bg-transparent font-semibold text-[var(--text)] outline-none cursor-pointer"
                    value={statusFilter}
                    onChange={(e) => setStatusFilter(e.target.value as any)}
                  >
                    <option value="all">All</option>
                    <option value="active">Active</option>
                    <option value="inactive">Inactive</option>
                  </select>
                </div>
              </div>
            )}
            <div className="divide-y divide-[var(--border)]">
              {keys.data.keys
                .filter(k => {
                  if (statusFilter === "active") return !k.disabled;
                  if (statusFilter === "inactive") return k.disabled;
                  return true;
                })
                .map((k) => (
                <KeyRow
                  key={k.id}
                  apiKey={k}
                  selected={selectedIds.has(k.id)}
                  onSelect={() => toggleSelect(k.id)}
                  onToggle={() => toggleDisabled.mutate({ id: k.id, disabled: !k.disabled })}
                  onConfigure={() => navigate(`/keys/${k.id}`)}
                  onRevoke={() => remove.mutate(k.id)}
                  togglePending={toggleDisabled.isPending}
                />
              ))}
            </div>
          </div>
        )}
      </Card>
    </>
  );
}

/* ── Step 1: Name ───────────────────────────────────────────────── */

function StepName({
  name,
  setName,
  onNext,
}: {
  name: string;
  setName: (v: string) => void;
  onNext: () => void;
}) {
  return (
    <div className="space-y-4 px-6 py-5">
      <Field label="Key name">
        <Input
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="laptop"
          autoFocus
          onKeyDown={(e) => {
            if (e.key === "Enter" && name.trim()) {
              e.preventDefault();
              onNext();
            }
          }}
        />
      </Field>
      <div className="flex gap-2 pt-1">
        <Button className="flex-1" onClick={onNext} disabled={!name.trim()}>
          Next
          <ArrowRight className="h-4 w-4" />
        </Button>
      </div>
    </div>
  );
}

/* ── Step 2: Plan Select ────────────────────────────────────────── */

function StepPlanSelect({
  plans,
  selectedPlanId,
  setSelectedPlanId,
  onBack,
  onNext,
}: {
  plans: Plan[];
  selectedPlanId: string;
  setSelectedPlanId: (v: string) => void;
  onBack: () => void;
  onNext: () => void;
}) {
  return (
    <div className="space-y-4 px-6 py-5">
      <p className="text-xs text-[var(--text-muted)]">
        Select a plan to inherit its budget rules, or choose Custom to set everything yourself.
      </p>

      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
        {plans.map((p) => (
          <button
            key={p.id}
            type="button"
            onClick={() => setSelectedPlanId(p.id)}
            className={`rounded-xl border px-4 py-3 text-left transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-accent-400/40 ${
              selectedPlanId === p.id
                ? "border-accent-400 bg-accent-500/10"
                : "border-[var(--border)] bg-[var(--bg-subtle)] hover:bg-[var(--bg)]"
            }`}
          >
            <div className="flex items-center gap-2">
              <Wallet className="h-4 w-4 text-[var(--text-muted)]" />
              <span className="text-sm font-medium">{p.name}</span>
            </div>
            <div className="mt-1.5 space-y-0.5 text-xs text-[var(--text-muted)]">
              {p.limit_micros > 0 && <p>{microsToUSD(p.limit_micros)} / {p.period}</p>}
              {p.limit_tokens > 0 && <p>{formatTokens(p.limit_tokens)} tokens / {p.period}</p>}
              {p.limit_micros === 0 && p.limit_tokens === 0 && <p>No spend limit</p>}
              {(p.allowed_models ?? []).length > 0 && <p>{(p.allowed_models ?? []).length} model restriction(s)</p>}
              <p>{p.key_count} key{p.key_count !== 1 ? "s" : ""}</p>
            </div>
          </button>
        ))}

        {/* Custom option */}
        <button
          type="button"
          onClick={() => setSelectedPlanId("custom")}
          className={`rounded-xl border border-dashed px-4 py-3 text-left transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-accent-400/40 ${
            selectedPlanId === "custom"
              ? "border-accent-400 bg-accent-500/10"
              : "border-[var(--border)] bg-[var(--bg-subtle)] hover:bg-[var(--bg)]"
          }`}
        >
          <div className="flex items-center gap-2">
            <Wrench className="h-4 w-4 text-[var(--text-muted)]" />
            <span className="text-sm font-medium">Custom</span>
          </div>
          <div className="mt-1.5 text-xs text-[var(--text-muted)]">
            <p>No preset</p>
            <p>Set everything yourself</p>
          </div>
        </button>
      </div>

      <div className="flex gap-2 pt-2 border-t border-[var(--border)]">
        <Button variant="ghost" onClick={onBack}>
          <ArrowLeft className="h-4 w-4" />
          Back
        </Button>
        <div className="flex-1" />
        <Button onClick={onNext}>
          Next
          <ArrowRight className="h-4 w-4" />
        </Button>
      </div>
    </div>
  );
}

/* ── Step 3: Configure (plan details / custom) ──────────────────── */

function StepConfigure({
  selectedPlanId,
  plans,
  customizePlan,
  setCustomizePlan,
  budgetLimit,
  setBudgetLimit,
  budgetLimitTokens,
  setBudgetLimitTokens,
  budgetPeriod,
  setBudgetPeriod,
  budgetAlertPct,
  setBudgetAlertPct,
  budgetHardCutoff,
  setBudgetHardCutoff,
  allowedModels,
  setAllowedModels,
  onBack,
  onCreate,
  isPending,
}: {
  selectedPlanId: string;
  plans: Plan[];
  customizePlan: boolean;
  setCustomizePlan: (v: boolean) => void;
  budgetLimit: string;
  setBudgetLimit: (v: string) => void;
  budgetLimitTokens: string;
  setBudgetLimitTokens: (v: string) => void;
  budgetPeriod: string;
  setBudgetPeriod: (v: string) => void;
  budgetAlertPct: number;
  setBudgetAlertPct: (v: number) => void;
  budgetHardCutoff: boolean;
  setBudgetHardCutoff: (v: boolean) => void;
  allowedModels: string[];
  setAllowedModels: (v: string[]) => void;
  onBack: () => void;
  onCreate: () => void;
  isPending: boolean;
}) {
  const isCustom = selectedPlanId === "custom";
  const selectedPlan = plans.find((p) => p.id === selectedPlanId);
  const models = selectedPlan?.allowed_models ?? [];

  if (isCustom) {
    // Full custom config (same as old StepBudget)
    return (
      <div className="space-y-4 px-6 py-5">
        <div className="flex gap-3">
          <div className="flex-1">
            <Field label="Limit (USD)">
              <Input
                type="number"
                min="0"
                step="0.01"
                value={budgetLimit}
                onChange={(e) => setBudgetLimit(e.target.value)}
                placeholder="50.00"
              />
            </Field>
          </div>
          <div className="flex-1">
            <Field label="Limit (Tokens)">
              <FormattedTokenInput
                value={budgetLimitTokens}
                onChange={setBudgetLimitTokens}
                placeholder="100000000"
              />
            </Field>
          </div>
          <div className="w-36">
            <Field label="Period">
              <Select value={budgetPeriod} onChange={(e) => setBudgetPeriod(e.target.value)}>
                {budgetPeriods.map((p) => (
                  <option key={p.value} value={p.value}>{p.label}</option>
                ))}
              </Select>
            </Field>
          </div>
        </div>

        <Field label="Allowed models">
          <ModelMultiSelect value={allowedModels} onChange={setAllowedModels} />
          <p className="mt-1 text-[10px] text-[var(--text-muted)]">
            Select models or add custom patterns with * wildcard (e.g. claude-*)
          </p>
        </Field>

        <div className="flex items-end gap-6">
          <div className="w-40">
            <Field label="Alert threshold (%)">
              <Input
                type="number"
                min="1"
                max="100"
                value={budgetAlertPct}
                onChange={(e) => setBudgetAlertPct(parseInt(e.target.value) || 80)}
              />
            </Field>
          </div>
          <div className="flex items-center gap-2 pb-0.5">
            <Toggle checked={budgetHardCutoff} onChange={setBudgetHardCutoff} />
            <span className="text-sm">Hard cutoff (block when exhausted)</span>
          </div>
        </div>

        <div className="flex gap-2 pt-2 border-t border-[var(--border)]">
          <Button variant="ghost" onClick={onBack}>
            <ArrowLeft className="h-4 w-4" />
            Back
          </Button>
          <div className="flex-1" />
          <Button variant="ghost" onClick={onCreate} disabled={isPending}>
            {isPending ? "Creating…" : "Skip budget"}
          </Button>
          <Button onClick={onCreate} disabled={isPending}>
            {isPending ? "Creating…" : "Create key"}
          </Button>
        </div>
      </div>
    );
  }

  // Plan selected — show summary + optional override toggle
  return (
    <div className="space-y-4 px-6 py-5">
      {/* Plan summary card */}
      {selectedPlan && (
        <div className="rounded-xl border border-[var(--border)] bg-[var(--bg-subtle)] p-4">
          <div className="flex items-center gap-2">
            <Wallet className="h-4 w-4 text-accent-500" />
            <span className="text-sm font-medium">{selectedPlan.name}</span>
            <Badge>{selectedPlan.period}</Badge>
            {selectedPlan.hard_cutoff ? <Badge tone="danger">hard cutoff</Badge> : <Badge tone="neutral">advisory</Badge>}
          </div>
          {selectedPlan.description && (
            <p className="mt-1 text-xs text-[var(--text-muted)]">{selectedPlan.description}</p>
          )}
          <div className="mt-2 flex flex-wrap items-center gap-3 text-xs text-[var(--text-muted)]">
            {selectedPlan.limit_micros > 0 && (
              <span className="flex items-center gap-1"><DollarSign className="h-3 w-3" />{microsToUSD(selectedPlan.limit_micros)}</span>
            )}
            {selectedPlan.limit_tokens > 0 && (
              <span className="flex items-center gap-1"><Gauge className="h-3 w-3" />{formatTokens(selectedPlan.limit_tokens)} tok</span>
            )}
            {selectedPlan.limit_micros === 0 && selectedPlan.limit_tokens === 0 && <span>No spend limit</span>}
            <span>Alert at {selectedPlan.alert_pct}%</span>
          </div>
          {models.length > 0 && (
            <p className="mt-1.5 text-xs text-[var(--text-muted)]">Models: {models.join(", ")}</p>
          )}
        </div>
      )}

      {/* Override toggle */}
      <div className="rounded-xl border border-[var(--border)] bg-[var(--bg-subtle)] px-4 py-3">
        <div className="flex items-center justify-between gap-4">
          <div>
            <p className="text-sm font-medium">Customize for this key</p>
            <p className="mt-0.5 text-xs text-[var(--text-muted)]">
              Override plan limits with per-key settings.
            </p>
          </div>
          <Toggle checked={customizePlan} onChange={setCustomizePlan} />
        </div>
      </div>

      {/* Override fields */}
      {customizePlan && (
        <div className="space-y-4 rounded-xl border border-[var(--border)] bg-[var(--bg-subtle)] p-4">
          <div className="flex gap-3">
            <div className="flex-1">
              <Field label="Override USD limit">
                <Input
                  type="number"
                  min="0"
                  step="0.01"
                  value={budgetLimit}
                  onChange={(e) => setBudgetLimit(e.target.value)}
                  placeholder="Leave empty to use plan"
                />
              </Field>
            </div>
            <div className="flex-1">
              <Field label="Override token limit">
                <FormattedTokenInput
                  value={budgetLimitTokens}
                  onChange={setBudgetLimitTokens}
                  placeholder="Leave empty to use plan"
                />
              </Field>
            </div>
          </div>
          <Field label="Override allowed models">
            <ModelMultiSelect value={allowedModels} onChange={setAllowedModels} />
            <p className="mt-1 text-[10px] text-[var(--text-muted)]">
              Leave empty to use plan's model restrictions.
            </p>
          </Field>
        </div>
      )}

      <div className="flex gap-2 pt-2 border-t border-[var(--border)]">
        <Button variant="ghost" onClick={onBack}>
          <ArrowLeft className="h-4 w-4" />
          Back
        </Button>
        <div className="flex-1" />
        <Button onClick={onCreate} disabled={isPending}>
          {isPending ? "Creating…" : "Create key"}
        </Button>
      </div>
    </div>
  );
}

/* ── Step 4: Success / Copy ─────────────────────────────────────── */

function StepSuccess({
  created,
  copied,
  setCopied,
  onClose,
}: {
  created: CreatedKey;
  copied: boolean;
  setCopied: (v: boolean) => void;
  onClose: () => void;
}) {
  const [copiedUrl, setCopiedUrl] = useState(false);
  const portalUrl = `${window.location.origin}/portal?id=${created.id}`;

  return (
    <div className="space-y-4 px-6 py-5">
      <div className="rounded-xl border border-[var(--border)] bg-[var(--bg-subtle)] p-4">
        <p className="text-xs font-medium text-[var(--text-muted)]">Your new key — copy it now, it won't be shown again.</p>
        <div className="mt-2 flex items-center gap-2">
          <code className="flex-1 overflow-x-auto rounded-lg bg-[var(--bg-elevated)] px-3 py-2.5 font-mono text-sm">
            {created.key}
          </code>
          <Button
            onClick={() => {
              navigator.clipboard.writeText(created.key);
              setCopied(true);
              setTimeout(() => setCopied(false), 1500);
            }}
          >
            {copied ? <Check className="h-4 w-4" /> : <Copy className="h-4 w-4" />}
            {copied ? "Copied" : "Copy"}
          </Button>
        </div>
      </div>

      <div className="rounded-xl border border-[var(--border)] bg-[var(--bg-subtle)] p-4">
        <p className="text-xs font-medium text-[var(--text-muted)]">Telemetry Portal URL — share this with the key owner to let them track their usage.</p>
        <div className="mt-2 flex items-center gap-2">
          <code className="flex-1 overflow-x-auto rounded-lg bg-[var(--bg-elevated)] px-3 py-2.5 font-mono text-xs whitespace-nowrap">
            {portalUrl}
          </code>
          <Button
            onClick={() => {
              navigator.clipboard.writeText(portalUrl);
              setCopiedUrl(true);
              setTimeout(() => setCopiedUrl(false), 1500);
            }}
          >
            {copiedUrl ? <Check className="h-4 w-4" /> : <Copy className="h-4 w-4" />}
            {copiedUrl ? "Copied" : "Copy"}
          </Button>
        </div>
      </div>

      {created.budget && (
        <div className="rounded-lg border border-[var(--border)] bg-[var(--bg-subtle)] px-4 py-3">
          <p className="text-xs font-medium text-[var(--text-muted)]">Plan attached</p>
          <p className="mt-0.5 text-sm">
            {created.budget.limit_micros > 0 && `$${(created.budget.limit_micros / 1_000_000).toFixed(2)}`}
            {created.budget.limit_micros > 0 && created.budget.limit_tokens > 0 && " + "}
            {created.budget.limit_tokens > 0 &&
              `${formatTokenLimit(String(created.budget.limit_tokens))} tokens`}
            {` / ${created.budget.period}`}
            {created.budget.hard_cutoff ? " (hard cutoff)" : ""}
          </p>
        </div>
      )}

      {created.allowed_models && created.allowed_models.length > 0 && (
        <div className="rounded-lg border border-[var(--border)] bg-[var(--bg-subtle)] px-4 py-3">
          <p className="text-xs font-medium text-[var(--text-muted)]">Allowed models</p>
          <p className="mt-0.5 text-sm">{created.allowed_models.join(", ")}</p>
        </div>
      )}

      <Button className="w-full" onClick={onClose}>
        Done
      </Button>
    </div>
  );
}