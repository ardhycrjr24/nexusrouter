// Reusable UI primitives styled with the NexusRouter design system. Calm,
// generously spaced, soft shadows and rounded surfaces — no gradients or neon.
import { useEffect, type ReactNode } from "react";
import type {
  ButtonHTMLAttributes,
  InputHTMLAttributes,
  SelectHTMLAttributes,
} from "react";
import { AlertCircle, X, type LucideIcon } from "lucide-react";

export function Card({ children, className = "" }: { children: ReactNode; className?: string }) {
  return (
    <div
      className={`rounded-2xl border border-[var(--border)] bg-[var(--bg-elevated)] shadow-sm ring-1 ring-inset ring-white/50 dark:ring-0 overflow-hidden ${className}`}
    >
      {children}
    </div>
  );
}

// SectionHeader is the in-card header with an optional rounded icon chip, used
// across Settings/Endpoints-style panels in the attachment.
export function SectionHeader({
  title,
  description,
  icon: Icon,
  iconTone = "accent",
  action,
}: {
  title: ReactNode;
  description?: string;
  icon?: LucideIcon;
  iconTone?: "accent" | "neutral" | "danger" | "secondary";
  action?: ReactNode;
}) {
  const toneClasses: Record<string, string> = {
    accent: "bg-accent-100 text-accent-700 dark:bg-accent-800/40 dark:text-accent-200",
    neutral: "bg-ink-100 text-ink-600 dark:bg-ink-800 dark:text-ink-300",
    danger: "bg-[color:var(--color-danger)]/15 text-[color:var(--color-danger)]",
    secondary: "bg-secondary-100 text-secondary-700 dark:bg-secondary-800/40 dark:text-secondary-200",
  };
  return (
    <div className="flex items-start justify-between gap-4 px-6 pt-5 pb-4">
      <div className="flex items-start gap-3">
        {Icon && (
          <div className={`flex h-9 w-9 shrink-0 items-center justify-center rounded-xl ${toneClasses[iconTone]}`}>
            <Icon className="h-[18px] w-[18px]" strokeWidth={2} />
          </div>
        )}
        <div>
          <h2 className="text-base font-semibold tracking-tight">{title}</h2>
          {description && <p className="mt-0.5 text-sm text-[var(--text-muted)]">{description}</p>}
        </div>
      </div>
      {action}
    </div>
  );
}

// CardHeader keeps the lighter divider-style header for list cards.
export function CardHeader({
  title,
  description,
  action,
}: {
  title: string;
  description?: string;
  action?: ReactNode;
}) {
  return (
    <div className="flex items-start justify-between gap-4 border-b border-[var(--border)] px-6 py-4">
      <div>
        <h2 className="text-sm font-semibold tracking-tight">{title}</h2>
        {description && <p className="mt-0.5 text-xs text-[var(--text-muted)]">{description}</p>}
      </div>
      {action}
    </div>
  );
}

// SettingsSection groups related cards under a labeled heading with a subtle divider.
// Improves scanability on long settings pages.
export function SettingsSection({
  title,
  icon: Icon,
  children,
}: {
  title: string;
  icon?: LucideIcon;
  children: ReactNode;
}) {
  return (
    <div className="space-y-4">
      <div className="flex items-center gap-2.5 pt-2">
        {Icon && (
          <div className="flex h-7 w-7 items-center justify-center rounded-lg bg-[var(--bg-subtle)]">
            <Icon className="h-4 w-4 text-[var(--text-muted)]" strokeWidth={2} />
          </div>
        )}
        <h3 className="text-sm font-semibold uppercase tracking-widest text-[var(--text-muted)]">
          {title}
        </h3>
        <div className="flex-1 border-t border-[var(--border)]" />
      </div>
      <div className="space-y-4">
        {children}
      </div>
    </div>
  );
}

type ButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: "primary" | "secondary" | "ghost" | "danger";
};

export function Button({ variant = "primary", className = "", ...props }: ButtonProps) {
  const base =
    "inline-flex items-center justify-center gap-1.5 rounded-xl px-3.5 py-2 text-sm font-medium transition-colors disabled:cursor-not-allowed disabled:opacity-50 focus:outline-none focus-visible:ring-2 focus-visible:ring-secondary-400/60";
  const variants = {
    primary: "bg-secondary-600 text-white hover:bg-secondary-700 dark:bg-secondary-500 dark:hover:bg-secondary-400 shadow-sm",
    secondary: "bg-accent-600 text-white hover:bg-accent-700 dark:bg-accent-500 dark:hover:bg-accent-400 shadow-sm",
    ghost:
      "border border-[var(--border)] bg-[var(--bg-elevated)] text-[var(--text)] hover:bg-ink-100 dark:hover:bg-ink-800",
    danger:
      "border border-[color:var(--color-danger)]/30 text-[color:var(--color-danger)] hover:bg-[color:var(--color-danger)]/10",
  };
  return <button className={`${base} ${variants[variant]} ${className}`} {...props} />;
}

export function Input({ className = "", ...props }: InputHTMLAttributes<HTMLInputElement>) {
  return (
    <input
      className={`w-full rounded-xl border border-[var(--border)] bg-[var(--bg-elevated)] px-3 py-2 text-sm placeholder:text-[var(--text-muted)] focus:border-accent-400 focus:outline-none focus-visible:ring-2 focus-visible:ring-accent-400/40 ${className}`}
      {...props}
    />
  );
}

export function Select({ className = "", children, ...props }: SelectHTMLAttributes<HTMLSelectElement>) {
  return (
    <select
      className={`w-full rounded-xl border border-[var(--border)] bg-[var(--bg-elevated)] px-3 py-2 text-sm focus:border-accent-400 focus:outline-none focus-visible:ring-2 focus-visible:ring-accent-400/40 ${className}`}
      {...props}
    >
      {children}
    </select>
  );
}

export function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <label className="block space-y-1.5">
      <span className="text-xs font-medium text-[var(--text-muted)]">{label}</span>
      {children}
    </label>
  );
}

export function Badge({
  children,
  tone = "neutral",
  title,
}: {
  children: ReactNode;
  tone?: "neutral" | "accent" | "secondary" | "danger" | "warning" | "success";
  title?: string;
}) {
  const tones = {
    neutral: "bg-ink-100 text-ink-600 dark:bg-ink-800 dark:text-ink-300",
    accent: "bg-accent-100 text-accent-700 dark:bg-accent-800/40 dark:text-accent-200",
    secondary: "bg-secondary-100 text-secondary-700 dark:bg-secondary-800/40 dark:text-secondary-200",
    danger: "bg-[color:var(--color-danger)]/15 text-[color:var(--color-danger)]",
    warning: "bg-[color:var(--color-warning)]/15 text-[color:var(--color-warning)]",
    success: "bg-accent-100 text-accent-700 dark:bg-accent-800/40 dark:text-accent-200",
  };
  return (
    <span
      className={`inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium ${tones[tone]}`}
      title={title}
    >
      {children}
    </span>
  );
}

// StatusDot is the small filled circle used next to "Healthy" / "Active" labels.
export function StatusDot({ tone = "success", label }: { tone?: "success" | "danger" | "warning" | "secondary"; label?: string }) {
  const colors = {
    success: "bg-accent-500",
    secondary: "bg-secondary-500",
    danger: "bg-[color:var(--color-danger)]",
    warning: "bg-[color:var(--color-warning)]",
  };
  return <span className={`inline-block h-1.5 w-1.5 rounded-full ${colors[tone]}`} role="img" aria-label={label || tone} />;
}

export function EmptyState({ title, hint }: { title: string; hint?: string }) {
  return (
    <div className="px-6 py-14 text-center">
      <p className="text-sm text-[var(--text-muted)]">{title}</p>
      {hint && <p className="mt-1 text-xs text-[var(--text-muted)]">{hint}</p>}
    </div>
  );
}

// ErrorBanner is the consistent inline error surface used inside forms and
// cards. For transient feedback prefer a toast; use this for persistent,
// in-context errors (failed loads, validation summaries).
export function ErrorBanner({ message, className = "" }: { message: string; className?: string }) {
  return (
    <div
      role="alert"
      className={`flex items-start gap-2.5 rounded-lg border border-[color:var(--color-danger)]/30 bg-[color:var(--color-danger)]/10 px-3.5 py-2.5 ${className}`}
    >
      <AlertCircle className="mt-0.5 h-4 w-4 shrink-0 text-[color:var(--color-danger)]" strokeWidth={2} />
      <p className="text-sm leading-snug break-words overflow-hidden text-[color:var(--color-danger)]">{message}</p>
    </div>
  );
}

// ErrorCard is a full-card error state for failed data loads.
export function ErrorCard({ message }: { message: string }) {
  return (
    <Card className="flex flex-col items-center gap-2 px-6 py-12 text-center">
      <AlertCircle className="h-6 w-6 text-[color:var(--color-danger)]" strokeWidth={2} />
      <p className="text-sm text-[color:var(--color-danger)]">{message}</p>
    </Card>
  );
}

export function Spinner() {
  return (
    <div className="flex items-center justify-center py-10" role="status" aria-label="Loading">
      <div className="h-5 w-5 animate-spin rounded-full border-2 border-ink-300 border-t-accent-500 dark:border-ink-600 dark:border-t-accent-400" />
    </div>
  );
}

export function StatCard({
  icon: Icon,
  iconTone = "accent",
  label,
  value,
  delta,
}: {
  icon: LucideIcon;
  iconTone?: "accent" | "warning" | "danger";
  label: string;
  value: string;
  delta?: { text: string; direction?: "up" | "down" | "flat" };
}) {
  const deltaColor =
    delta?.direction === "up"
      ? "text-emerald-600 dark:text-emerald-400"
      : delta?.direction === "down"
        ? "text-red-500 dark:text-red-400"
        : "text-[var(--text-muted)]";
  const arrow = delta?.direction === "up" ? "↑" : delta?.direction === "down" ? "↓" : "";

  const tone = iconTone === "accent"
    ? { marker: "bg-secondary-500", icon: "text-secondary-600 dark:text-secondary-300", iconBg: "bg-secondary-50 ring-secondary-200/70 dark:bg-secondary-950/30 dark:ring-secondary-900/60" }
    : iconTone === "warning"
      ? { marker: "bg-amber-500", icon: "text-amber-700 dark:text-amber-300", iconBg: "bg-amber-50 ring-amber-200/70 dark:bg-amber-950/30 dark:ring-amber-900/60" }
      : { marker: "bg-red-500", icon: "text-red-700 dark:text-red-300", iconBg: "bg-red-50 ring-red-200/70 dark:bg-red-950/30 dark:ring-red-900/60" };

  return (
    <div className="rounded-2xl border border-[var(--border)] bg-[var(--bg-elevated)] p-4 shadow-sm">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <span className={`inline-flex h-7 w-7 shrink-0 items-center justify-center rounded-lg ring-1 ${tone.iconBg}`}>
              <Icon className={`h-3.5 w-3.5 ${tone.icon}`} strokeWidth={2} />
            </span>
            <p className="truncate text-[11px] font-semibold uppercase tracking-[0.18em] text-[var(--text-muted)]">
              {label}
            </p>
          </div>
          <p className="mt-3 text-3xl font-semibold tracking-tight tabular-nums text-[var(--text)]">
            {value}
          </p>
        </div>
        <span className={`h-9 w-1.5 shrink-0 rounded-full ${tone.marker}`} aria-hidden="true" />
      </div>
      {delta && (
        <p className={`mt-3 text-xs font-medium ${deltaColor}`}>
          {arrow} {delta.text}
        </p>
      )}
    </div>
  );
}

// SegmentedControl renders the Gentle / Balanced / Strong style toggle group.
export function SegmentedControl<T extends string>({
  value,
  onChange,
  options,
}: {
  value: T;
  onChange: (v: T) => void;
  options: { value: T; label: string }[];
}) {
  return (
    <div className="inline-flex rounded-lg border border-[var(--border)] bg-[var(--bg-subtle)] p-0.5" role="radiogroup">
      {options.map((opt) => (
        <button
          key={opt.value}
          type="button"
          role="radio"
          aria-checked={value === opt.value}
          onClick={() => onChange(opt.value)}
          className={`rounded-md px-3 py-2 text-xs font-medium transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-accent-400/60 ${
            value === opt.value
              ? "bg-[var(--bg-elevated)] text-[var(--text)] shadow-sm"
              : "text-[var(--text-muted)] hover:text-[var(--text)]"
          }`}
        >
          {opt.label}
        </button>
      ))}
    </div>
  );
}

// Toggle is a small accessible switch.
// Modal is a reusable dialog overlay. Escape and backdrop click close it.
export function Modal({
  open,
  onClose,
  title,
  subtitle,
  children,
  maxWidth = "max-w-lg",
}: {
  open: boolean;
  onClose: () => void;
  title: ReactNode;
  subtitle?: string;
  children: ReactNode;
  maxWidth?: string;
}) {
  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [open, onClose]);

  if (!open) return null;

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4 backdrop-blur-sm"
      onClick={onClose}
      role="dialog"
      aria-modal="true"
      aria-label={typeof title === "string" ? title : undefined}
    >
      <div
        className={`w-full ${maxWidth} rounded-2xl border border-[var(--border)] bg-[var(--bg-elevated)] shadow-[var(--shadow-float)]`}
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center justify-between border-b border-[var(--border)] px-6 py-4">
          <div>
            <h2 className="text-base font-semibold tracking-tight">{title}</h2>
            {subtitle && <p className="mt-0.5 text-sm text-[var(--text-muted)]">{subtitle}</p>}
          </div>
          <button
            onClick={onClose}
            aria-label="Close"
            className="flex h-9 w-9 items-center justify-center rounded-xl text-[var(--text-muted)] transition-colors hover:bg-ink-100 hover:text-[var(--text)] dark:hover:bg-ink-800 focus:outline-none focus-visible:ring-2 focus-visible:ring-accent-400/60"
          >
            <X className="h-4 w-4" />
          </button>
        </div>
        {children}
      </div>
    </div>
  );
}

// TabBar renders a horizontal tab navigation strip.
export function TabBar<T extends string>({
  tabs,
  active,
  onChange,
}: {
  tabs: { value: T; label: string; icon?: LucideIcon }[];
  active: T;
  onChange: (v: T) => void;
}) {
  return (
    <div className="flex gap-1 overflow-x-auto border-b border-[var(--border)] px-1 pb-px" role="tablist">
      {tabs.map((tab) => {
        const isActive = tab.value === active;
        return (
          <button
            key={tab.value}
            role="tab"
            aria-selected={isActive}
            onClick={() => onChange(tab.value)}
            className={`relative flex items-center gap-2 whitespace-nowrap rounded-t-lg px-4 py-2.5 text-sm font-medium transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-accent-400/60 ${
              isActive
                ? "text-accent-700 dark:text-accent-200"
                : "text-[var(--text-muted)] hover:text-[var(--text)]"
            }`}
          >
            {tab.icon && <tab.icon className="h-4 w-4 shrink-0" strokeWidth={2} />}
            {tab.label}
            {isActive && (
              <span className="absolute inset-x-0 -bottom-px h-0.5 rounded-full bg-accent-600 dark:bg-accent-400" />
            )}
          </button>
        );
      })}
    </div>
  );
}

export function Toggle({ checked, onChange }: { checked: boolean; onChange: (v: boolean) => void }) {
  return (
    <button
      role="switch"
      aria-checked={checked}
      onClick={() => onChange(!checked)}
      className={`relative inline-flex h-7 w-12 shrink-0 items-center rounded-full transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-secondary-400/60 ${
        checked ? "bg-secondary-600" : "bg-ink-300 dark:bg-ink-700"
      }`}
    >
      <span
        className={`inline-block h-5 w-5 transform rounded-full bg-white shadow-sm transition-transform ${
          checked ? "translate-x-[22px]" : "translate-x-1"
        }`}
      />
    </button>
  );
}