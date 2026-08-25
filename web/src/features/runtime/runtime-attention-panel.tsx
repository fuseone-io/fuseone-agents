import {
  AlertTriangle,
  Clock3,
  type LucideIcon,
  MessageSquareWarning,
  ShieldAlert,
  Wrench,
} from "lucide-react";
import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { LoadMore } from "@/components/shared/load-more";
import { Panel } from "@/components/shared/panel";
import { failureLabel } from "@/features/runtime/failure-labels";
import {
  runtimeAttention,
  type RuntimeAttentionItem,
  type RuntimeAttentionKind,
} from "@/features/runtime/runtime-attention";
import { useVisibleItems } from "@/hooks/use-visible-items";
import { formatRelative } from "@/lib/format";
import type { RuntimeHealth } from "@/lib/api/client";

const PAGE_SIZE = 8;
const ICONS: Record<RuntimeAttentionKind, LucideIcon> = {
  provider: AlertTriangle,
  coordination: Clock3,
  tool: Wrench,
  channel: MessageSquareWarning,
  egress: ShieldAlert,
  queue: Clock3,
};

export function RuntimeAttentionPanel({ health }: { health: RuntimeHealth }) {
  const { t } = useTranslation();
  const items = useMemo(() => runtimeAttention(health), [health]);
  const page = useVisibleItems(items, PAGE_SIZE);

  return (
    <Panel title={t("runtime.attentionTitle")} className="xl:col-span-2">
      {page.visible.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          {t("runtime.attentionEmpty")}
        </p>
      ) : (
        <>
          <p className="mb-3 max-w-[80ch] text-xs text-muted-foreground">
            {t("runtime.attentionHint")}
          </p>
          <ol className="grid gap-2 lg:grid-cols-2">
            {page.visible.map((item) => (
              <li key={item.id}>
                <AttentionRow item={item} />
              </li>
            ))}
          </ol>
          <LoadMore
            loaded={page.loaded}
            total={page.total}
            hasMore={page.hasMore}
            isLoading={false}
            onLoad={page.loadMore}
          />
        </>
      )}
    </Panel>
  );
}

function AttentionRow({ item }: { item: RuntimeAttentionItem }) {
  const { t } = useTranslation();
  const Icon = ICONS[item.kind];
  return (
    <div className="flex min-w-0 gap-3 rounded-md border bg-background px-3 py-2">
      <Icon className="mt-0.5 size-4 shrink-0 text-warning" aria-hidden />
      <div className="min-w-0 flex-1">
        <div className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
          <p className="min-w-0 truncate text-sm font-medium">
            {titleOf(item, t)}
          </p>
          <span className="rounded-pill bg-muted px-2 py-0.5 font-mono text-2xs tabular-nums text-muted-foreground">
            {item.count}
          </span>
        </div>
        <p className="mt-1 text-2xs text-muted-foreground">
          {detailOf(item, t)}
          {item.lastAt && (
            <>
              {" · "}
              {t("runtime.attentionLastSeen", {
                seen: formatRelative(item.lastAt),
              })}
            </>
          )}
        </p>
      </div>
    </div>
  );
}

function titleOf(
  item: RuntimeAttentionItem,
  t: (key: string, values?: Record<string, unknown>) => string,
) {
  if (item.kind === "queue") {
    return t(`runtime.attentionQueue${queueKey(item.code)}`);
  }
  return t(failureLabel(item.code));
}

function detailOf(
  item: RuntimeAttentionItem,
  t: (key: string, values?: Record<string, unknown>) => string,
) {
  if (item.kind === "queue") {
    return t("runtime.attentionQueueDetail");
  }
  const key = `runtime.attention${capitalise(item.kind)}Detail`;
  return t(key, { count: item.count, secondary: item.secondary ?? 0, ...item.values });
}

function queueKey(code: string) {
  return code === "expired_leases" ? "ExpiredLeases" : "BackingOff";
}

function capitalise(value: string) {
  return `${value.slice(0, 1).toUpperCase()}${value.slice(1)}`;
}
