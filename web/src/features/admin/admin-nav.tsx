import {
  Archive,
  Building2,
  CircleDollarSign,
  Gauge,
  Inbox,
  KeyRound,
  Layers,
  Palette,
  ScrollText,
  Sparkles,
  Users,
} from "lucide-react";
import type { LucideIcon } from "lucide-react";
import { useTranslation } from "react-i18next";
import {
  type AdminTab,
  type AdminTabValue,
} from "@/features/admin/admin-tabs";
import { useTools } from "@/features/admin/api";
import { waitingFor } from "@/features/admin/waiting-tools";
import { cn } from "@/lib/utils";

const ADMIN_TAB_ICONS: Record<AdminTabValue, LucideIcon> = {
  tools: Inbox,
  events: ScrollText,
  branding: Palette,
  authoring: Sparkles,
  identity: KeyRound,
  companies: Building2,
  areas: Layers,
  people: Users,
  prices: CircleDollarSign,
  budgets: Gauge,
  retention: Archive,
};

export function AdminNav({
  groups,
  value,
  onValueChange,
}: {
  groups: Array<{ label: string; tabs: AdminTab[] }>;
  value: AdminTabValue;
  onValueChange: (value: AdminTabValue) => void;
}) {
  const { t } = useTranslation();

  return (
    <nav
      aria-label={t("admin.sections")}
      className="flex w-full shrink-0 flex-col gap-0.5 lg:sticky lg:top-6 lg:w-[232px]"
      role="tablist"
    >
      {groups.map((group, index) => (
        <div key={group.label} className="contents">
          <span
            className={cn(
              "px-2.5 pb-1 text-2xs font-semibold uppercase tracking-label text-text-disabled",
              index === 0 ? "pt-0" : "pt-3.5",
            )}
          >
            {t(group.label)}
          </span>
          {group.tabs.map((item) => (
            <AdminNavItem
              key={item.value}
              item={item}
              active={item.value === value}
              onPick={() => onValueChange(item.value)}
            />
          ))}
        </div>
      ))}
    </nav>
  );
}

function AdminNavItem({
  item,
  active,
  onPick,
}: {
  item: AdminTab;
  active: boolean;
  onPick: () => void;
}) {
  const { t } = useTranslation();
  const Icon = ADMIN_TAB_ICONS[item.value];

  return (
    <button
      id={`admin-tab-${item.value}`}
      type="button"
      role="tab"
      aria-selected={active}
      aria-controls={`admin-panel-${item.value}`}
      onClick={onPick}
      className={cn(
        "relative flex h-[34px] w-full items-center gap-[9px] rounded-md border-0 bg-transparent px-2.5 pl-[11px] text-left text-sm text-muted-foreground transition-colors hover:bg-surface-hover hover:text-foreground",
        active &&
          "bg-surface-accent font-medium text-foreground hover:bg-surface-accent",
      )}
    >
      {active && (
        <span
          className="absolute bottom-2 left-0 top-2 w-0.5 rounded-full bg-primary"
          aria-hidden
        />
      )}
      <Icon
        className={cn(
          "size-3.5 shrink-0",
          active ? "text-text-accent" : "text-text-disabled",
        )}
        aria-hidden
      />
      <span className="min-w-0 flex-1 truncate">{t(item.label)}</span>
      {item.value === "tools" && <ToolsWaitingBadge active={active} />}
    </button>
  );
}

function ToolsWaitingBadge({ active }: { active: boolean }) {
  const { t } = useTranslation();
  const { data } = useTools();
  const waiting = waitingFor(data?.items ?? []).length;

  if (waiting === 0) return null;

  return (
    <span
      className={cn(
        "ml-auto font-mono text-[11px]",
        active ? "text-text-accent" : "text-warning",
      )}
      title={t("admin.waitingForARuling", { count: waiting })}
    >
      {waiting}
    </span>
  );
}
