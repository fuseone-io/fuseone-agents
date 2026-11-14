import { useTranslation } from "react-i18next";
import {
  areaOptions,
  companyOptions,
  WHOLE_COMPANY,
} from "@/features/admin/grant-scope-options";
import type { GrantInput } from "@/features/admin/people-api";
import { ScopeSelect } from "@/features/admin/scope-select";
import { useGrantScopes } from "@/features/admin/use-grant-scopes";

/**
 * Where one grant applies: a company, then one of its areas or all of them.
 *
 * Chosen from what is registered rather than typed, so a grant cannot name an
 * area that does not exist and never meet the agents filed under the real one.
 */
export function GrantScopeFields({
  grant,
  index,
  labelClass,
  onChange,
}: {
  grant: GrantInput;
  index: number;
  labelClass: string;
  onChange: (patch: Partial<GrantInput>) => void;
}) {
  const { t } = useTranslation();
  const { data, isLoading } = useGrantScopes();
  const companies = companyOptions(data.companies, grant.company);
  const areas = areaOptions(data.scopes, grant.company, grant.area);

  return (
    <>
      <ScopeSelect
        field={{
          id: `grant-company-${index}`,
          label: t("identity.company"),
          labelClass,
        }}
        value={grant.company}
        options={companies}
        disabled={isLoading}
        onChange={(company) => onChange({ company, area: "" })}
      />
      <ScopeSelect
        field={{
          id: `grant-area-${index}`,
          label: t("identity.area"),
          labelClass,
        }}
        value={grant.area || WHOLE_COMPANY}
        options={areas}
        disabled={isLoading || areas.length < 2}
        onChange={(area) =>
          onChange({ area: area === WHOLE_COMPANY ? "" : area })
        }
      />
    </>
  );
}
