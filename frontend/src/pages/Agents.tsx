import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Bot, Plus, Trash2, Key } from "lucide-react";
import { api, type Agent } from "../lib/api";
import { PageHeader } from "../components/Layout";
import { useToast } from "../components/Toast";
import { Card, SectionHeader, CardHeader, Button, Input, Field, Spinner, EmptyState, ErrorBanner } from "../components/ui";

export function AgentsPage() {
  const qc = useQueryClient();
  const toast = useToast();
  const agents = useQuery({ queryKey: ["agents"], queryFn: () => api.listAgents() });

  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [systemPrompt, setSystemPrompt] = useState("");
  const [model, setModel] = useState("");
  const [error, setError] = useState("");

  const create = useMutation({
    mutationFn: () => api.createAgent({ name, description, system_prompt: systemPrompt, model }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["agents"] });
      setName("");
      setDescription("");
      setSystemPrompt("");
      setModel("");
      setError("");
      toast.success("Agent created", `Agent "${name}" is now available.`);
    },
    onError: (e) => {
      setError((e as Error).message);
      toast.error("Agent creation failed", (e as Error).message);
    },
  });

  const remove = useMutation({
    mutationFn: (id: string) => api.deleteAgent(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["agents"] });
      toast.success("Agent removed", "The agent has been deleted.");
    },
    onError: (e) => toast.error("Agent removal failed", (e as Error).message),
  });

  return (
    <>
      <PageHeader
        title="Custom Agents"
        icon={Bot}
        description="Create and manage tailored AI agents with specific instructions and models."
      />

      <div className="space-y-6">
        <Card>
          <SectionHeader
            title="Create agent"
            description="Configure a new agent with a custom system prompt and specific model routing."
            icon={Plus}
          />
          <div className="space-y-4 border-t border-[var(--border)] px-6 py-5">
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <Field label="Name">
                <Input
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder="Code Assistant"
                />
              </Field>
              <Field label="Description">
                <Input
                  value={description}
                  onChange={(e) => setDescription(e.target.value)}
                  placeholder="Expert at writing unit tests"
                />
              </Field>
            </div>
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <Field label="Default Model (Optional)">
                <Input
                  value={model}
                  onChange={(e) => setModel(e.target.value)}
                  placeholder="claude-3-5-sonnet"
                />
              </Field>
            </div>
            <Field label="System Prompt">
              <textarea
                value={systemPrompt}
                onChange={(e) => setSystemPrompt(e.target.value)}
                rows={4}
                placeholder="You are an expert developer..."
                className="w-full rounded-lg border border-[var(--border)] bg-[var(--bg-elevated)] px-3 py-2 text-sm placeholder:text-[var(--text-muted)] focus:border-accent-400 focus:outline-none focus-visible:ring-2 focus-visible:ring-accent-400/40"
              />
            </Field>
            {error && <ErrorBanner message={error} />}
            <div className="flex items-center gap-3">
              <Button onClick={() => create.mutate()} disabled={!name.trim() || create.isPending}>
                <Plus className="h-4 w-4" />
                {create.isPending ? "Creating…" : "Create agent"}
              </Button>
            </div>
          </div>
        </Card>

        <Card>
          <CardHeader title="Configured agents" />
          {agents.isLoading ? (
            <Spinner />
          ) : !agents.data?.agents?.length ? (
            <EmptyState title="No agents configured" hint="Create your first custom agent above." />
          ) : (
            <div className="divide-y divide-[var(--border)]">
              {agents.data.agents.map((ag) => (
                <AgentRow
                  key={ag.id}
                  agent={ag}
                  onDelete={() => remove.mutate(ag.id)}
                />
              ))}
            </div>
          )}
        </Card>
      </div>
    </>
  );
}

function AgentRow({
  agent,
  onDelete,
}: {
  agent: Agent;
  onDelete: () => void;
}) {
  return (
    <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4 px-6 py-4">
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2">
            <span className="text-sm font-medium">{agent.name}</span>
            {agent.model && (
                <span className="rounded bg-[var(--bg-subtle)] px-2 py-0.5 font-mono text-[10px] text-[var(--text-muted)] border border-[var(--border)]">
                    {agent.model}
                </span>
            )}
        </div>
        {agent.description && (
          <p className="mt-1 text-xs text-[var(--text-muted)]">{agent.description}</p>
        )}
        <div className="mt-3 flex items-center gap-4 text-[11px] text-[var(--text-muted)] font-mono">
           <div className="flex items-center gap-1.5">
              <Key className="h-3 w-3" />
              <span>Endpoint: /v1/agents/{agent.id}/chat/completions</span>
           </div>
        </div>
      </div>
      <div className="flex shrink-0 items-center gap-2 mt-4 sm:mt-0">
        <Button variant="danger" onClick={onDelete}>
          <Trash2 className="h-4 w-4" />
        </Button>
      </div>
    </div>
  );
}
