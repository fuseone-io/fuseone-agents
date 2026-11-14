import { useState } from "react";
import { KeyRound, Settings2, UserMinus } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import type { Person } from "@/features/admin/people-api";
import { isRemovableHere } from "@/features/admin/people-filters";
import { RemovePersonDialog } from "@/features/admin/remove-person-dialog";
import { useMe } from "@/features/session/api";

export function PersonActions({
  person,
  onEdit,
  onSetPassword,
}: {
  person: Person;
  onEdit: () => void;
  onSetPassword: () => void;
}) {
  const { t } = useTranslation();
  const me = useMe();
  const [removing, setRemoving] = useState(false);
  // Offered once the caller is known: before then this could be their own row.
  const removable = me.isSuccess && isRemovableHere(person, me.data?.id);
  return (
    <div className="flex flex-wrap items-center gap-2 bg-surface-sunken px-4 pb-4 lg:pl-16">
      <Button variant="outline" size="sm" onClick={onEdit}>
        <Settings2 className="size-4" aria-hidden />
        {t("people.manage")}
      </Button>
      {canUseLocalPassword(person) && (
        <Button variant="outline" size="sm" onClick={onSetPassword}>
          <KeyRound className="size-4" aria-hidden />
          {person.username
            ? t("people.changePassword")
            : t("people.setPassword")}
        </Button>
      )}
      {removable && (
        <Button variant="outline" size="sm" onClick={() => setRemoving(true)}>
          <UserMinus className="size-4" aria-hidden />
          {t("people.remove")}
        </Button>
      )}
      {removing && (
        <RemovePersonDialog person={person} onClose={() => setRemoving(false)} />
      )}
    </div>
  );
}

function canUseLocalPassword(person: Person) {
  return person.kind === "user" && !(person.provider ?? "").startsWith("oidc");
}
