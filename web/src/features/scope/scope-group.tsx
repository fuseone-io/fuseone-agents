import { useTranslation } from "react-i18next";
import { DropdownMenuSeparator } from "@/components/ui/dropdown-menu";
import { ScopeChoice } from "@/features/scope/scope-choice";
import type { Scope } from "@/features/scope/active-scope";
import type { RegisteredScope } from "@/features/scope/api";

/** One company and the areas declared inside it. */
export function ScopeGroup({
  company,
  areas,
  active,
  onChoose,
}: {
  company: string;
  areas: RegisteredScope[];
  active: Scope;
  onChoose: (scope: Scope) => void;
}) {
  const { t } = useTranslation();

  return (
    <>
      <DropdownMenuSeparator />
      <ScopeChoice
        label={t("scope.wholeCompany", { company })}
        chosen={active.company === company && active.area === ""}
        onChoose={() => onChoose({ company, area: "" })}
      />
      {areas.map((a) => (
        <ScopeChoice
          key={a.area}
          label={a.label || a.area}
          indented
          chosen={active.company === company && active.area === a.area}
          onChoose={() => onChoose({ company, area: a.area })}
        />
      ))}
      {/* Silence here would read as "this company has no areas" when it may
          only mean nobody has declared one yet. */}
      {areas.length === 0 && (
        <p className="px-2 py-1.5 pl-8 text-xs text-muted-foreground">
          {t("scope.noAreas")}
        </p>
      )}
    </>
  );
}
