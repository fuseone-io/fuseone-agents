import type { RuntimeHealth } from "@/lib/api/client";

export type RuntimeAttentionKind =
  | "provider"
  | "coordination"
  | "tool"
  | "channel"
  | "egress"
  | "queue";

export interface RuntimeAttentionItem {
  id: string;
  kind: RuntimeAttentionKind;
  code: string;
  count: number;
  secondary?: number;
  lastAt?: string;
  values?: Record<string, unknown>;
}

/**
 * One operational queue over several low-cardinality projections.
 *
 * The items carry stable codes and counts only. They never include run ids,
 * tool arguments, Slack text, URLs, headers or provider diagnostics.
 */
export function runtimeAttention(health: RuntimeHealth): RuntimeAttentionItem[] {
  const items: RuntimeAttentionItem[] = [];
  collectQueue(items, health);
  collectProviders(items, health);
  collectTools(items, health);
  collectChannels(items, health);
  collectEgress(items, health);
  return items.sort(compareAttention);
}

function collectQueue(items: RuntimeAttentionItem[], health: RuntimeHealth) {
  if (health.queue.expiredLeases > 0) {
    items.push(queueItem("expired_leases", health.queue.expiredLeases));
  }
  if (health.queue.backingOff > 0) {
    items.push(queueItem("backing_off", health.queue.backingOff));
  }
}

function collectProviders(items: RuntimeAttentionItem[], health: RuntimeHealth) {
  for (const failure of health.failures) {
    if (failure.code === "dedupe_in_flight") {
      items.push({
        id: `coordination:${failure.code}`,
        kind: "coordination",
        code: failure.code,
        count: failure.runs,
        lastAt: failure.lastAt,
      });
      continue;
    }
    items.push({
      id: `provider:${failure.code}:${failure.provider ?? ""}:${failure.status ?? 0}`,
      kind: "provider",
      code: failure.code,
      count: failure.runs,
      lastAt: failure.lastAt,
      values: {
        provider: failure.provider || undefined,
        status: failure.status || undefined,
      },
    });
  }
}

function collectTools(items: RuntimeAttentionItem[], health: RuntimeHealth) {
  for (const failure of health.toolFailures) {
    items.push({
      id: `tool:${failure.code}`,
      kind: "tool",
      code: failure.code,
      count: failure.calls,
      secondary: failure.runs,
      lastAt: failure.lastAt,
    });
  }
}

function collectChannels(items: RuntimeAttentionItem[], health: RuntimeHealth) {
  for (const failure of health.channelFailures) {
    items.push({
      id: `channel:${failure.code}`,
      kind: "channel",
      code: failure.code,
      count: failure.attempts,
      secondary: failure.runs,
      lastAt: failure.lastAt,
      values: {
        conversations: failure.scopeWide ? undefined : failure.conversations,
        scopeWide: failure.scopeWide,
      },
    });
  }
}

function collectEgress(items: RuntimeAttentionItem[], health: RuntimeHealth) {
  for (const denial of health.egressDenials) {
    items.push({
      id: `egress:${denial.code}`,
      kind: "egress",
      code: denial.code,
      count: denial.attempts,
      secondary: denial.servers,
      lastAt: denial.lastAt,
      values: {
        destinations: denial.destinations,
      },
    });
  }
}

function queueItem(code: string, count: number): RuntimeAttentionItem {
  return {
    id: `queue:${code}`,
    kind: "queue",
    code,
    count,
  };
}

function compareAttention(left: RuntimeAttentionItem, right: RuntimeAttentionItem) {
  const current = right.count - left.count;
  if (current !== 0) return current;
  return timestamp(right.lastAt) - timestamp(left.lastAt);
}

function timestamp(value?: string) {
  if (!value) return 0;
  const parsed = Date.parse(value);
  return Number.isNaN(parsed) ? 0 : parsed;
}
