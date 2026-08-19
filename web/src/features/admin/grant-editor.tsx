import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Plus, ShieldCheck } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { GrantRow } from "@/features/admin/grant-row";
import { GrantBadge } from "@/features/admin/grant-badge";
import {
  useSetGrants,
  type GrantInput,
  type Person,
} from "@/features/admin/people-api";
import { problemMessage } from "@/lib/api/problem-message";

/**
 * What somebody was given directly.
 *
 * The asserted grants are shown above and cannot be touched: revoking one here
 * would last until its holder signs in again, and the group is what to change.
 */
export function GrantEditor({
  person,
  onDone,
}: {
  person: Person;
  onDone: () => void;
}) {
  const { t } = useTranslation();
  const save = useSetGrants();
  const held = person.grants ?? [];
  const [granted, setGranted] = useState<GrantInput[]>(
    held
      .filter((g) => !g.asserted)
      .map(({ company, area, role }) => ({
        company,
        area,
        role,
      })),
  );

  const asserted = held.filter((g) => g.asserted);
  const hasInstallationAdmin = granted.some(
    (g) => g.company === "*" && g.area === "" && g.role === "admin",
  );
  const update = (index: number, patch: Partial<GrantInput>) =>
    setGranted(granted.map((g, i) => (i === index ? { ...g, ...patch } : g)));

  const submit = () =>
    save.mutate(
      { principalId: person.id, grants: granted },
      {
        onSuccess: () => {
          toast.success(t("people.saved", { name: person.display }));
          onDone();
        },
        onError: (error) =>
          toast.error(t("people.saveFailed"), {
            description: problemMessage(error, t),
          }),
      },
    );

  return (
    <div className="flex flex-col gap-4">
      {asserted.length > 0 && (
        <div className="flex flex-col gap-1.5">
          <p className="text-xs text-muted-foreground">
            {t("people.assertedHere", { provider: person.provider ?? "" })}
          </p>
          <div className="flex flex-wrap gap-1.5">
            {asserted.map((grant, i) => (
              <GrantBadge key={i} grant={grant} />
            ))}
          </div>
        </div>
      )}

      <div className="flex flex-col gap-2">
        <p className="text-xs text-muted-foreground">
          {t("people.grantedHere")}
        </p>

        {granted.map((grant, index) => (
          <GrantRow
            key={index}
            grant={grant}
            index={index}
            onChange={(patch) => update(index, patch)}
            onRemove={() => setGranted(granted.filter((_, i) => i !== index))}
          />
        ))}

        <div className="flex flex-wrap gap-2">
          <Button
            variant="outline"
            size="sm"
            className="h-8"
            onClick={() =>
              setGranted([
                ...granted,
                { company: "default", area: "", role: "author" },
              ])
            }
          >
            <Plus className="size-4" aria-hidden />
            {t("people.addGrant")}
          </Button>
          <Button
            variant="outline"
            size="sm"
            className="h-8"
            disabled={hasInstallationAdmin}
            onClick={() =>
              setGranted([
                ...granted,
                { company: "*", area: "", role: "admin" },
              ])
            }
          >
            <ShieldCheck className="size-4" aria-hidden />
            {t("people.addAdminGrant")}
          </Button>
        </div>
      </div>

      <div className="flex gap-2">
        <Button onClick={submit} disabled={save.isPending}>
          {t("common.save")}
        </Button>
        <Button variant="ghost" onClick={onDone}>
          {t("common.cancel")}
        </Button>
      </div>
    </div>
  );
}
