import { Plus, UserRoundCheck, UserRoundX } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { usePeople } from "@/features/admin/people-api";
import { useBindIdentity, useUnbindIdentity } from "@/features/channels/api";
import {
  IdentityChips,
  SeenAccounts,
} from "@/features/channels/channel-identity-pills";
import { problemMessage } from "@/lib/api/problem-message";
import { cn } from "@/lib/utils";
import type { components } from "@/lib/api/schema.gen";

type Identity = components["schemas"]["ChannelIdentity"];
type Seen = components["schemas"]["ChannelSeenAccount"];

export function ChannelIdentityStrip({
  channel,
  identities,
  seenAccounts,
}: {
  channel: string;
  identities: Identity[];
  seenAccounts: Seen[];
}) {
  const { t } = useTranslation();
  const [account, setAccount] = useState("");
  const [principal, setPrincipal] = useState("");
  const people = usePeople().data?.items ?? [];
  const bind = useBindIdentity();
  const unbind = useUnbindIdentity();

  async function add() {
    try {
      await bind.mutateAsync({ channel, account: account.trim(), principal });
      setAccount("");
      setPrincipal("");
      toast.success(t("channels.bound"));
    } catch (error) {
      toast.error(problemMessage(error, t));
    }
  }

  return (
    <div
      className={cn(
        "flex flex-wrap items-center gap-3 px-4 py-3",
        identities.length === 0 ? "bg-warning/10" : "bg-muted/40",
      )}
    >
      {identities.length === 0 ? (
        <UserRoundX className="size-4 shrink-0 text-warning" aria-hidden />
      ) : (
        <UserRoundCheck
          className="size-4 shrink-0 text-muted-foreground"
          aria-hidden
        />
      )}
      <div className="min-w-0 flex-1">
        <p className="text-2xs font-medium uppercase tracking-label text-muted-foreground">
          {t("channels.whoCanDecide")}
        </p>
        {identities.length === 0 ? (
          <p className="text-xs text-muted-foreground">
            {t("channels.noDeciderConsequence")}
          </p>
        ) : (
          <IdentityChips
            identities={identities}
            onRemove={(id) =>
              unbind.mutate({ channel, account: id.account })
            }
          />
        )}
        {seenAccounts.length > 0 && (
          <SeenAccounts accounts={seenAccounts} onPick={setAccount} />
        )}
      </div>
      <div className="flex min-w-[260px] flex-wrap items-center gap-2">
        <Input
          value={account}
          onChange={(event) => setAccount(event.target.value)}
          placeholder="U0123ABCDEF"
          aria-label={t("channels.account")}
          className="h-8 w-36 font-mono"
        />
        <Select value={principal} onValueChange={setPrincipal}>
          <SelectTrigger className="h-8 min-w-36 flex-1" aria-label={t("channels.person")}>
            <SelectValue placeholder={t("channels.pickPerson")} />
          </SelectTrigger>
          <SelectContent>
            {people.map((person) => (
              <SelectItem key={person.id} value={person.id}>
                {person.display}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Button
          variant="outline"
          size="sm"
          disabled={!account.trim() || !principal || bind.isPending}
          onClick={() => void add()}
        >
          <Plus className="size-3.5" aria-hidden />
          {t("channels.bind")}
        </Button>
      </div>
    </div>
  );
}
