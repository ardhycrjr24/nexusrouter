import { useState, useEffect, useRef, useCallback, useMemo } from "react";
import { useNavigate } from "react-router-dom";
import {
  Search,
  LayoutGrid,
  Boxes,
  Network,
  Layers,
  BarChart3,
  Clock,
  TerminalSquare,
  Image,
  Waypoints,
  Sparkles,
  ScrollText,
  Wallet,
  Zap,
  Key,
  Plus,
  Settings,
  type LucideIcon,
} from "lucide-react";

interface CommandItem {
  id: string;
  label: string;
  description?: string;
  icon: LucideIcon;
  section: string;
  action: () => void;
  keywords?: string[];
}

interface CommandPaletteProps {
  open: boolean;
  onClose: () => void;
}

export function CommandPalette({ open, onClose }: CommandPaletteProps) {
  const [query, setQuery] = useState("");
  const [selectedIndex, setSelectedIndex] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);
  const listRef = useRef<HTMLDivElement>(null);
  const navigate = useNavigate();

  const go = useCallback(
    (path: string) => {
      navigate(path);
      onClose();
    },
    [navigate, onClose],
  );

  const items: CommandItem[] = useMemo(
    () => [
      // Navigation
      { id: "nav-overview", label: "Overview", icon: LayoutGrid, section: "Overview", action: () => go("/"), keywords: ["home", "dashboard"] },
      { id: "nav-endpoints", label: "Endpoints", icon: Network, section: "Traffic & Logic", action: () => go("/endpoints"), keywords: ["proxy", "settings"] },
      { id: "nav-chains", label: "Chains", icon: Layers, section: "Traffic & Logic", action: () => go("/chains"), keywords: ["chain", "routing", "fallback"] },
      { id: "nav-skills", label: "Skills", icon: Sparkles, section: "Traffic & Logic", action: () => go("/skills"), keywords: ["prompt", "custom"] },
      { id: "nav-providers", label: "Providers", icon: Boxes, section: "Connections", action: () => go("/providers"), keywords: ["accounts", "openai", "anthropic"] },
      { id: "nav-media", label: "Media", icon: Image, section: "Connections", action: () => go("/media"), keywords: ["image", "video", "tts", "stt"] },
      { id: "nav-proxy-pools", label: "Proxy Pools", icon: Waypoints, section: "Connections", action: () => go("/proxy-pools"), keywords: ["proxy", "residential", "egress"] },
      { id: "nav-usage", label: "Usage", icon: BarChart3, section: "Cost & Analytics", action: () => go("/usage"), keywords: ["analytics", "stats", "tokens"] },
      { id: "nav-plans", label: "Plans", icon: Wallet, section: "Cost & Analytics", action: () => go("/plans"), keywords: ["cost", "limit", "spend", "budget"] },
      { id: "nav-quota", label: "Quota Tracker", icon: Clock, section: "Cost & Analytics", action: () => go("/quota"), keywords: ["limits", "upstream"] },
      { id: "nav-settings", label: "Token Saving", icon: Zap, section: "Cost & Analytics", action: () => go("/settings"), keywords: ["rtk", "caveman", "terse", "cache"] },
      { id: "nav-keys", label: "API Keys", icon: Key, section: "Developer", action: () => go("/keys"), keywords: ["auth", "token", "secret"] },
      { id: "nav-console", label: "Console Log", icon: ScrollText, section: "Developer", action: () => go("/console"), keywords: ["logs", "debug", "output"] },
      { id: "nav-cli-tools", label: "CLI Tools", icon: TerminalSquare, section: "Developer", action: () => go("/cli-tools"), keywords: ["claude", "codex", "cline", "copilot"] },

      // Quick actions
      { id: "action-new-key", label: "Create API Key", icon: Plus, section: "Actions", action: () => go("/keys"), keywords: ["new", "generate"] },
      { id: "action-new-account", label: "Add Provider Account", icon: Plus, section: "Actions", action: () => go("/providers"), keywords: ["new", "connect"] },
      { id: "action-new-chain", label: "Create Chain", icon: Plus, section: "Actions", action: () => go("/chains"), keywords: ["new", "chain", "routing"] },
      { id: "action-new-pool", label: "Add Proxy Pool", icon: Plus, section: "Actions", action: () => go("/proxy-pools"), keywords: ["new", "proxy"] },
      { id: "action-settings", label: "Open Settings", icon: Settings, section: "Actions", action: () => go("/settings"), keywords: ["config", "preferences"] },
    ],
    [go],
  );

  const filtered = useMemo(() => {
    if (!query.trim()) return items;
    const q = query.toLowerCase();
    return items.filter(
      (item) =>
        item.label.toLowerCase().includes(q) ||
        item.description?.toLowerCase().includes(q) ||
        item.section.toLowerCase().includes(q) ||
        item.keywords?.some((k) => k.includes(q)),
    );
  }, [items, query]);

  // Group by section
  const grouped = useMemo(() => {
    const groups: { section: string; items: typeof filtered }[] = [];
    for (const item of filtered) {
      const last = groups[groups.length - 1];
      if (last && last.section === item.section) {
        last.items.push(item);
      } else {
        groups.push({ section: item.section, items: [item] });
      }
    }
    return groups;
  }, [filtered]);

  // Flat list for keyboard navigation
  const flatItems = filtered;

  // Reset selection when query changes
  useEffect(() => {
    setSelectedIndex(0);
  }, [query]);

  // Focus input when opened
  useEffect(() => {
    if (open) {
      setQuery("");
      setSelectedIndex(0);
      // Small delay to ensure the input is mounted
      requestAnimationFrame(() => inputRef.current?.focus());
    }
  }, [open]);

  // Scroll selected item into view
  useEffect(() => {
    if (!listRef.current) return;
    const selected = listRef.current.querySelector(`[data-index="${selectedIndex}"]`);
    selected?.scrollIntoView({ block: "nearest" });
  }, [selectedIndex]);

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      switch (e.key) {
        case "ArrowDown":
          e.preventDefault();
          setSelectedIndex((i) => (i + 1) % flatItems.length);
          break;
        case "ArrowUp":
          e.preventDefault();
          setSelectedIndex((i) => (i - 1 + flatItems.length) % flatItems.length);
          break;
        case "Enter":
          e.preventDefault();
          if (flatItems[selectedIndex]) {
            flatItems[selectedIndex].action();
          }
          break;
        case "Escape":
          e.preventDefault();
          onClose();
          break;
      }
    },
    [flatItems, selectedIndex, onClose],
  );

  if (!open) return null;

  let flatIndex = 0;

  return (
    <div className="fixed inset-0 z-[100] flex items-start justify-center pt-[15vh]" role="dialog" aria-modal="true" aria-label="Command palette">
      {/* Backdrop */}
      <div className="fixed inset-0 bg-black/40 backdrop-blur-sm" onClick={onClose} />

      {/* Panel */}
      <div
        className="relative w-full max-w-lg overflow-hidden rounded-2xl border border-[var(--border)] bg-[var(--bg-elevated)] shadow-[var(--shadow-float)]"
        style={{ animation: "page-in 0.15s ease-out" }}
        onKeyDown={handleKeyDown}
      >
        {/* Input */}
        <div className="flex items-center gap-3 border-b border-[var(--border)] px-4">
          <Search className="h-4 w-4 shrink-0 text-[var(--text-muted)]" />
          <input
            ref={inputRef}
            type="text"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Search pages, actions…"
            className="h-12 flex-1 bg-transparent text-sm text-[var(--text)] placeholder:text-[var(--text-muted)] focus:outline-none"
            autoComplete="off"
            spellCheck={false}
          />
          <kbd className="shrink-0 rounded border border-[var(--border)] bg-[var(--bg-subtle)] px-1.5 py-0.5 font-mono text-[10px] text-[var(--text-muted)]">
            esc
          </kbd>
        </div>

        {/* Results */}
        <div ref={listRef} className="max-h-80 overflow-y-auto py-2">
          {grouped.length === 0 ? (
            <div className="px-4 py-8 text-center text-sm text-[var(--text-muted)]">
              No results for "{query}"
            </div>
          ) : (
            grouped.map((group) => (
              <div key={group.section}>
                <p className="px-4 pb-1 pt-2 text-[10px] font-semibold uppercase tracking-wider text-[var(--text-muted)]">
                  {group.section}
                </p>
                {group.items.map((item) => {
                  const idx = flatIndex++;
                  const selected = idx === selectedIndex;
                  return (
                    <button
                      key={item.id}
                      data-index={idx}
                      onClick={item.action}
                      onMouseEnter={() => setSelectedIndex(idx)}
                      className={`flex w-full items-center gap-3 px-4 py-2 text-left text-sm transition-colors ${
                        selected
                          ? "bg-accent-100 text-accent-700 dark:bg-accent-800/40 dark:text-accent-200"
                          : "text-[var(--text)] hover:bg-[var(--bg-subtle)]"
                      }`}
                    >
                      <item.icon className="h-4 w-4 shrink-0 text-[var(--text-muted)]" strokeWidth={2} />
                      <span className="flex-1 truncate">{item.label}</span>
                      {item.description && (
                        <span className="text-xs text-[var(--text-muted)]">{item.description}</span>
                      )}
                    </button>
                  );
                })}
              </div>
            ))
          )}
        </div>

        {/* Footer hint */}
        <div className="flex items-center gap-4 border-t border-[var(--border)] px-4 py-2 text-[10px] text-[var(--text-muted)]">
          <span className="flex items-center gap-1">
            <kbd className="rounded border border-[var(--border)] bg-[var(--bg-subtle)] px-1 py-px font-mono">↑↓</kbd>
            navigate
          </span>
          <span className="flex items-center gap-1">
            <kbd className="rounded border border-[var(--border)] bg-[var(--bg-subtle)] px-1 py-px font-mono">↵</kbd>
            select
          </span>
          <span className="flex items-center gap-1">
            <kbd className="rounded border border-[var(--border)] bg-[var(--bg-subtle)] px-1 py-px font-mono">esc</kbd>
            close
          </span>
        </div>
      </div>
    </div>
  );
}
