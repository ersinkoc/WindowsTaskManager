import { zodResolver } from "@hookform/resolvers/zod";
import { Bot, Eye, EyeOff, LoaderCircle, ShieldCheck, Sparkles } from "lucide-react";
import { useState } from "react";
import { useForm } from "react-hook-form";
import { useNavigate } from "react-router";
import { z } from "zod";
import { EmptyState } from "../components/shared/empty-state";
import { PageHeader } from "../components/shared/page-header";
import { PageSkeleton } from "../components/shared/page-skeleton";
import { queueRuleDraftPrefill } from "./rules-page";
import { Badge } from "../components/ui/badge";
import { Button } from "../components/ui/button";
import { Card } from "../components/ui/card";
import { useAIAnalyzeMutation, useAIChatMutation, useAIExecuteMutation, useAIStatusQuery, useSystemSnapshotQuery, useAlertsQuery } from "../lib/api-client";
import { formatPercent } from "../lib/format";
import type { AISuggestion } from "../types/api";

const chatSchema = z.object({
  message: z.string().min(1, "Message is required").max(1500, "Keep it under 1500 characters"),
});

const promptSuggestions = [
  "Which process looks most suspicious right now?",
  "Summarize the biggest CPU and memory hotspots.",
  "What should I investigate first from the active alerts?",
];

type ChatFormValues = z.infer<typeof chatSchema>;

export function AIPage() {
  const navigate = useNavigate();
  const { data, isLoading } = useAIStatusQuery();
  const { data: systemData } = useSystemSnapshotQuery();
  const { data: alertsData } = useAlertsQuery();
  const chatMutation = useAIChatMutation();
  const analyzeMutation = useAIAnalyzeMutation();
  const executeMutation = useAIExecuteMutation();
  const [transcript, setTranscript] = useState<Array<{ role: "user" | "assistant"; text: string }>>([]);
  const [suggestions, setSuggestions] = useState<AISuggestion[]>([]);
  const [showPreview, setShowPreview] = useState(false);
  const MAX_TURNS = 20;
  const form = useForm<ChatFormValues>({
    resolver: zodResolver(chatSchema),
    defaultValues: { message: "" },
    mode: "onChange",
  });

  const onSubmit = form.handleSubmit(async (values) => {
    const newEntry = { role: "user" as const, text: values.message };
    setTranscript((current) => {
      const next = [...current, newEntry];
      return next.length > MAX_TURNS ? next.slice(-MAX_TURNS) : next;
    });
    form.reset();
    const result = await chatMutation.mutateAsync(values.message);
    setTranscript((current) => {
      const withResponse = [...current, { role: "assistant" as const, text: result.answer }];
      return withResponse.length > MAX_TURNS ? withResponse.slice(-MAX_TURNS) : withResponse;
    });
    setSuggestions(result.actions ?? []);
  });

  if (isLoading) {
    return <PageSkeleton />;
  }

  if (!data) {
    return <EmptyState icon={Bot} title="Waiting for AI status" description="WTM is checking whether the local AI advisor is configured and reachable." />;
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title="AI Advisor"
        description="Use the local advisor for triage, summaries, and guarded actions without giving up operator control."
        eyebrow="Assistant"
        icon={Bot}
        meta={
          <>
            <Badge variant={data.enabled ? "success" : "warning"}>{data.enabled ? "Enabled" : "Disabled"}</Badge>
            <Badge variant={data.configured ? "info" : "warning"}>
              {data.provider ? `${data.provider}${data.model ? ` / ${data.model}` : ""}` : data.configured ? "Configured" : "Not configured"}
            </Badge>
          </>
        }
      />

      <div className="grid gap-4 sm:grid-cols-3">
        <StatusCard label="AI status" value={data.enabled ? "Enabled" : "Disabled"} variant={data.enabled ? "success" : "warning"} />
        <StatusCard
          label="Provider"
          value={data.provider ? `${data.provider}${data.model ? ` / ${data.model}` : ""}` : data.configured ? "Configured" : "Not configured"}
          variant={data.configured ? "info" : "warning"}
        />
        <StatusCard label="Messages" value={String(transcript.length)} variant={transcript.length > 0 ? "info" : "neutral"} />
      </div>

      <div className="grid gap-4 xl:grid-cols-[minmax(0,1.85fr)_minmax(17rem,0.75fr)]">
        <Card className="space-y-0 overflow-hidden p-0">
          <div className="flex flex-wrap items-start justify-between gap-3 border-b border-border px-4 py-4 sm:px-5">
            <div className="flex items-center gap-3">
              <div className="rounded-md border border-border bg-background px-3 py-3 text-accent">
                <Sparkles className="h-4.5 w-4.5" />
              </div>
              <div>
                <div className="font-semibold text-foreground">{data.enabled ? "AI enabled" : "AI disabled"}</div>
                <div className="text-sm text-secondary">{data.configured ? "Provider configured" : "Provider not configured"}</div>
              </div>
            </div>
            <Button type="button" size="sm" variant="secondary" onClick={() => navigate("/settings")}>
              Open settings
            </Button>
          </div>

          <div className="grid gap-px border-b border-border bg-border sm:grid-cols-3">
            <QuickGuide title="Settings moved out" description="Provider, endpoint, token, and runtime stay on Settings." />
            <QuickGuide title="Analyze first" description="Use snapshot analysis when you want the current machine state summarized." />
            <QuickGuide title="Approval required" description="Nothing executes until you approve a suggested action." />
          </div>

          <div className="space-y-3 px-4 py-4 sm:px-5">
            <div className="flex flex-wrap items-center gap-2">
              <span className="text-sm font-medium text-secondary">Quick prompts</span>
              {promptSuggestions.map((prompt) => (
                <Button
                  key={prompt}
                  type="button"
                  size="sm"
                  variant="secondary"
                  className="max-w-full truncate"
                  onClick={() => form.setValue("message", prompt, { shouldDirty: true, shouldTouch: true, shouldValidate: true })}
                >
                  {prompt}
                </Button>
              ))}
            </div>

            <div className="space-y-3 rounded-lg border border-border bg-background px-4 py-4">
              <div className="flex items-center justify-between gap-3">
                <div>
                  <h2 className="section-title">Conversation</h2>
                  <p className="mt-1 text-sm text-secondary">Ask for prioritization, anomaly summaries, or the safest next step.</p>
                </div>
                <div className="flex items-center gap-2">
                  {transcript.length > 0 && (
                    <Button type="button" size="sm" variant="ghost" onClick={() => { setTranscript([]); setSuggestions([]); }}>
                      Clear
                    </Button>
                  )}
                  <Badge variant={transcript.length > 0 ? "info" : "neutral"}>{transcript.length} turns</Badge>
                </div>
              </div>
              {transcript.length === 0 ? (
                <EmptyState icon={Bot} title="No conversation yet" description="Ask about noisy processes, spikes, or the safest next step." />
              ) : (
                <div className="max-h-[28rem] space-y-2 overflow-y-auto pr-1">
                  {transcript.map((entry, index) => (
                    <div
                      key={`${entry.role}-${index}`}
                      className={
                        entry.role === "user"
                          ? "ml-auto max-w-3xl rounded-md bg-accent px-3.5 py-3 text-accent-foreground"
                          : "mr-auto max-w-3xl rounded-md border border-border bg-surface px-3.5 py-3 text-foreground"
                      }
                    >
                      <div className="mb-1 text-[0.68rem] font-semibold uppercase tracking-[0.18em] opacity-75">{entry.role}</div>
                      <div className="text-sm leading-6">{entry.text}</div>
                    </div>
                  ))}
                </div>
              )}
            </div>
          </div>

          <form className="space-y-3 border-t border-border px-4 py-4 sm:px-5" onSubmit={onSubmit}>
            <label className="block text-sm font-medium text-foreground" htmlFor="ai-message">
              Message
            </label>
            <textarea
              id="ai-message"
              className="form-textarea min-h-24"
              placeholder="Ask the local advisor to summarize suspicious behavior or recommend your next step."
              {...form.register("message")}
            />
            {form.formState.errors.message ? <p className="text-sm text-error">{form.formState.errors.message.message}</p> : null}
            <div className="flex flex-wrap items-center gap-3">
              <Button disabled={chatMutation.isPending || !form.formState.isValid || !data.enabled} type="submit">
                {chatMutation.isPending ? (
                  <>
                    <LoaderCircle className="mr-2 h-4 w-4 animate-spin motion-reduce:animate-none" />
                    Sending...
                  </>
                ) : (
                  "Send chat"
                )}
              </Button>
              <Button
                type="button"
                variant="secondary"
                disabled={analyzeMutation.isPending || !form.formState.isValid || !data.enabled}
                onClick={form.handleSubmit(async (values) => {
                  const result = await analyzeMutation.mutateAsync(values.message);
                  setTranscript((current) => {
                    const withResponse = [...current, { role: "assistant" as const, text: result.answer }];
                    return withResponse.length > MAX_TURNS ? withResponse.slice(-MAX_TURNS) : withResponse;
                  });
                  setSuggestions(result.actions ?? []);
                })}
              >
                {analyzeMutation.isPending ? "Analyzing..." : "Analyze snapshot"}
              </Button>
              <Button type="button" variant="ghost" size="sm" onClick={() => setShowPreview((v) => !v)}>
                {showPreview ? <EyeOff className="mr-2 h-4 w-4" /> : <Eye className="mr-2 h-4 w-4" />}
                {showPreview ? "Hide" : "Show"} data preview
              </Button>
              {!data.enabled ? <Badge variant="warning">Enable AI in the backend config to chat.</Badge> : null}
            </div>
            {showPreview && systemData && (
              <div className="mt-3 rounded-lg border border-border bg-background px-4 py-3">
                <div className="mb-2 text-xs font-semibold uppercase tracking-wider text-secondary">Data sent to AI</div>
                <div className="grid gap-2 text-xs">
                  <div className="flex justify-between gap-4">
                    <span className="text-secondary">Processes</span>
                    <span className="font-mono text-foreground">{systemData.processes.length}</span>
                  </div>
                  <div className="flex justify-between gap-4">
                    <span className="text-secondary">CPU</span>
                    <span className="font-mono text-foreground">{formatPercent(systemData.cpu.total_percent)}</span>
                  </div>
                  <div className="flex justify-between gap-4">
                    <span className="text-secondary">Memory</span>
                    <span className="font-mono text-foreground">{formatPercent(systemData.memory.used_percent)}</span>
                  </div>
                  <div className="flex justify-between gap-4">
                    <span className="text-secondary">Top process</span>
                    <span className="font-mono text-foreground">{systemData.processes.sort((a, b) => b.cpu_percent - a.cpu_percent)[0]?.name ?? "--"}</span>
                  </div>
                  {alertsData?.active.length ? (
                    <div className="flex justify-between gap-4">
                      <span className="text-secondary">Active alerts</span>
                      <span className="font-mono text-foreground">{alertsData.active.length}</span>
                    </div>
                  ) : null}
                  {systemData.port_bindings?.length ? (
                    <div className="flex justify-between gap-4">
                      <span className="text-secondary">Port bindings</span>
                      <span className="font-mono text-foreground">{systemData.port_bindings.length}</span>
                    </div>
                  ) : null}
                </div>
              </div>
            )}
          </form>
        </Card>

        <Card className="space-y-0 overflow-hidden p-0">
          <div className="flex items-center justify-between gap-3 border-b border-border px-4 py-4 sm:px-5">
            <h2 className="section-title">Suggested actions</h2>
            <Badge variant={suggestions.length > 0 ? "info" : "neutral"}>{suggestions.length} queued</Badge>
          </div>
          <div className="grid gap-px border-b border-border bg-border">
            <QuickGuide title="Approve deliberately" description="All actions still go through backend safety checks." />
            <QuickGuide title="Use Analyze for triage" description="Snapshot analysis is better for current state than open-ended chat." />
            <QuickGuide title="Review rules" description="Rule drafts should still be checked in Rules before saving." />
          </div>
          {suggestions.length === 0 ? (
            <div className="px-4 py-4 text-sm text-secondary sm:px-5">
              No structured actions yet. Ask the advisor for a concrete mitigation plan or click Analyze snapshot.
            </div>
          ) : (
            <div className="space-y-0 px-4 py-4 sm:px-5">
              {suggestions.map((suggestion) => (
                <ActionCard
                  key={suggestion.id}
                  suggestion={suggestion}
                  isPending={executeMutation.isPending}
                  onApprove={() => executeMutation.mutate(suggestion)}
                  onOpenInRules={() => {
                    if (!suggestion.rule) {
                      return;
                    }
                    queueRuleDraftPrefill({
                      name: suggestion.rule.name,
                      enabled: suggestion.rule.enabled ?? true,
                      match: suggestion.rule.match,
                      metric: suggestion.rule.metric,
                      op: suggestion.rule.op,
                      threshold: suggestion.rule.threshold,
                      for_seconds: suggestion.rule.for_seconds ?? suggestion.rule.for ?? 30,
                      action: suggestion.rule.action,
                      cooldown_seconds: suggestion.rule.cooldown_seconds ?? suggestion.rule.cooldown ?? 300,
                    });
                    navigate("/rules");
                  }}
                />
              ))}
            </div>
          )}
        </Card>
      </div>
    </div>
  );
}

interface ActionCardProps {
  suggestion: AISuggestion;
  isPending: boolean;
  onApprove: () => void;
  onOpenInRules: () => void;
}

function ActionCard({ suggestion, isPending, onApprove, onOpenInRules }: ActionCardProps) {
  const target =
    suggestion.type === "add_rule"
      ? suggestion.rule?.name || "rule"
      : suggestion.name || (suggestion.pid ? `PID ${suggestion.pid}` : "target");

  return (
    <div className="border-b border-border py-3 last:border-b-0 first:pt-0 last:pb-0">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <Badge variant={suggestion.type === "kill" || suggestion.type === "suspend" ? "warning" : "info"}>{suggestion.type}</Badge>
            <span className="truncate text-sm font-semibold text-foreground">{target}</span>
          </div>
          {suggestion.reason ? <div className="mt-2 text-sm leading-5 text-secondary">{suggestion.reason}</div> : null}
          {suggestion.rule ? (
            <div className="mt-2 text-xs leading-5 text-secondary">
              {suggestion.rule.match} {suggestion.rule.metric} {suggestion.rule.op} {suggestion.rule.threshold} {"->"} {suggestion.rule.action}
            </div>
          ) : null}
        </div>
        <div className="flex flex-wrap gap-2">
          <Button type="button" size="sm" disabled={isPending} onClick={onApprove}>
            <ShieldCheck className="mr-2 h-4 w-4" />
            Approve
          </Button>
          {suggestion.type === "add_rule" ? (
            <Button type="button" size="sm" variant="secondary" onClick={onOpenInRules}>
              Open in Rules
            </Button>
          ) : null}
        </div>
      </div>
    </div>
  );
}

interface QuickGuideProps {
  title: string;
  description: string;
}

function QuickGuide({ title, description }: QuickGuideProps) {
  return (
    <div className="bg-surface px-4 py-3 sm:px-5">
      <div className="text-sm font-semibold text-foreground">{title}</div>
      <div className="mt-1 text-sm leading-5 text-secondary">{description}</div>
    </div>
  );
}

interface StatusCardProps {
  label: string;
  value: string;
  variant: "neutral" | "info" | "success" | "warning";
}

function StatusCard({ label, value, variant }: StatusCardProps) {
  return (
    <Card>
      <div className="flex min-w-0 items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="metric-label">{label}</div>
          <div className="mt-2 truncate text-xl font-semibold tracking-tight text-foreground sm:text-2xl">{value}</div>
        </div>
        <Badge variant={variant}>{label}</Badge>
      </div>
    </Card>
  );
}
