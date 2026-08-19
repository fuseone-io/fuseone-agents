import { Trash2, TriangleAlert } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Mono } from "@/components/shared/mono";
import { formatRelative } from "@/lib/format";
import { cn } from "@/lib/utils";
import type { components } from "@/lib/api/schema.gen";

type Identity = components["schemas"]["ChannelIdentity"];
type Seen = components["schemas"]["ChannelSeenAccount"];

export function IdentityChips({
  identities,
  onRemove,
}: {
  identities: Identity[];
  onRemove: (id: Identity) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="mt-1 flex flex-wrap gap-1.5">
      {identities.map((id) => (
        <span
          key={id.account}
          className={cn(
            "inline-flex min-h-6 max-w-56 items-center gap-1.5 rounded-full border bg-background px-2 py-0.5 text-xs",
            id.unreadable && "border-danger/40 text-danger",
          )}
        >
          {id.unreadable && <TriangleAlert className="size-3" aria-hidden />}
          <span className="truncate">
            {id.unreadable
              ? t("channels.bindingUnreadable")
              : id.display || id.principal}
          </span>
          <Mono dim className="shrink-0 text-[10px]">
            {id.account}
          </Mono>
          <button
            type="button"
            className="text-muted-foreground hover:text-danger"
            aria-label={t("common.remove")}
            onClick={() => onRemove(id)}
          >
            <Trash2 className="size-3" aria-hidden />
          </button>
        </span>
      ))}
    </div>
  );
}

export function SeenAccounts({
  accounts,
  onPick,
}: {
  accounts: Seen[];
  onPick: (account: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="mt-2 flex flex-wrap items-center gap-1.5">
      <span className="text-2xs text-muted-foreground">
        {t("channels.seenAccounts")}:
      </span>
      {accounts.slice(0, 4).map((seen) => (
        <button
          key={seen.account}
          type="button"
          className="rounded-full border bg-background px-2 py-0.5 font-mono text-[10px] text-muted-foreground hover:text-foreground"
          title={t("channels.seenAt", { when: formatRelative(seen.lastSeen) })}
          onClick={() => onPick(seen.account)}
        >
          {seen.account}
        </button>
      ))}
    </div>
  );
}
