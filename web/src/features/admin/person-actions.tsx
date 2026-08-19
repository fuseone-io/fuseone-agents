import { KeyRound, Settings2 } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import type { Person } from "@/features/admin/people-api";

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
    </div>
  );
}

function canUseLocalPassword(person: Person) {
  return person.kind === "user" && !(person.provider ?? "").startsWith("oidc");
}
