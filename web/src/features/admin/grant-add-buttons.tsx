import { useTranslation } from "react-i18next";
import { Plus, ShieldCheck } from "lucide-react";
import { Button } from "@/components/ui/button";
import { firstOffered } from "@/features/scope/scope-select-options";
import type { GrantInput } from "@/features/admin/people-api";
import { useScopeLists } from "@/features/scope/use-scope-lists";

/**
 * The two ways to add a grant. A new one starts on the first company that can
 * take one, over all of it; the installation administrator is its own button
 * so nobody has to know the wildcard that names the installation.
 */
export function GrantAddButtons({
  granted,
  machine,
  onAdd,
}: {
  granted: GrantInput[];
  machine: boolean;
  onAdd: (grant: GrantInput) => void;
}) {
  const { t } = useTranslation();
  const { data } = useScopeLists();
  const hasInstallationAdmin = granted.some(
    (g) => g.company === "*" && g.area === "" && g.role === "admin",
  );
  return (
    <div className="flex flex-wrap gap-2">
      <Button
        variant="outline"
        size="sm"
        className="h-8"
        onClick={() =>
          onAdd({
            company: firstOffered(data.companies),
            area: "",
            role: "author",
          })
        }
      >
        <Plus className="size-4" aria-hidden />
        {t("people.addGrant")}
      </Button>
      {!machine && (
        <Button
          variant="outline"
          size="sm"
          className="h-8"
          disabled={hasInstallationAdmin}
          onClick={() => onAdd({ company: "*", area: "", role: "admin" })}
        >
          <ShieldCheck className="size-4" aria-hidden />
          {t("people.addAdminGrant")}
        </Button>
      )}
    </div>
  );
}
