import { Trash2, UserRoundCheck } from "lucide-react";
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
import { useBindIdentity, useUnbindIdentity } from "@/features/channels/api";
import { usePeople } from "@/features/admin/people-api";
import { problemMessage } from "@/lib/api/problem-message";
import { Mono } from "@/components/shared/mono";
import type { components } from "@/lib/api/schema.gen";

/**
 * Who each account in this channel speaks for.
 *
 * Binding one grants that person's authority — their grants, their permission
 * to decide — to whoever holds the account, and by the time a decision is being
 * sealed the principal is simply the principal. So the person is chosen from
 * the directory and never typed, and the account is typed because only the
 * channel knows it.
 *
 * An account nobody has bound acts as nobody at all, which is why this list
 * being empty is a working state and not an error.
 */
export function IdentityRows({
  channel,
  identities,
}: {
  channel: string;
  identities: components["schemas"]["ChannelIdentity"][];
}) {
  const { t } = useTranslation();
  const [account, setAccount] = useState("");
  const [principal, setPrincipal] = useState("");
  const bind = useBindIdentity();
  const unbind = useUnbindIdentity();
  const people = usePeople().data?.items ?? [];

  async function add() {
    try {
      await bind.mutateAsync({
        channel,
        account: account.trim(),
        principal,
      });
      setAccount("");
      setPrincipal("");
      toast.success(t("channels.bound"));
    } catch (error) {
      toast.error(problemMessage(error, t));
    }
  }

  return (
    <div className="flex flex-col gap-1 p-2">
      <p className="px-2 pt-1 text-2xs font-medium uppercase tracking-label text-muted-foreground">
        {t("channels.whoCanDecide")}
      </p>

      {identities.length === 0 ? (
        <p className="px-2 py-1.5 text-xs text-muted-foreground">
          {t("channels.nobodyBound")}
        </p>
      ) : (
        identities.map((id) => (
          <div
            key={id.account}
            className="flex items-center gap-2 rounded-md px-2 py-1.5 hover:bg-muted/50"
          >
            <UserRoundCheck
              className="size-3.5 shrink-0 text-muted-foreground"
              aria-hidden
            />
            <span className="min-w-0 flex-1 truncate text-sm">
              {id.display || id.principal}
            </span>
            <Mono dim className="shrink-0 text-2xs">
              {id.account}
            </Mono>
            <Button
              variant="ghost"
              size="icon"
              aria-label={t("common.remove")}
              onClick={() => unbind.mutate({ channel, account: id.account })}
            >
              <Trash2 className="size-4" />
            </Button>
          </div>
        ))
      )}

      <div className="flex flex-wrap items-center gap-2 px-2 pt-1">
        <Input
          value={account}
          onChange={(e) => setAccount(e.target.value)}
          placeholder="U0123ABCDEF"
          aria-label={t("channels.account")}
          className="h-8 w-40 font-mono"
        />
        <Select value={principal} onValueChange={setPrincipal}>
          <SelectTrigger
            className="h-8 min-w-0 flex-1"
            aria-label={t("channels.person")}
          >
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
          {t("channels.bind")}
        </Button>
      </div>
    </div>
  );
}
