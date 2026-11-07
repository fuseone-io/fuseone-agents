import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { GrantEditor } from "@/features/admin/grant-editor";
import { ServiceAccountGrants } from "@/features/admin/service-account-grants";
import { ServiceAccountSection } from "@/features/admin/service-account-section";
import type { ServiceAccount } from "@/features/admin/service-accounts-api";
import type { Person } from "@/features/admin/people-api";

function asPerson(account: ServiceAccount): Person {
  return {
    id: account.id,
    kind: "service",
    display: account.name,
    disabled: account.disabled,
    grants: account.grants.map((grant) => ({ ...grant, asserted: false })),
  };
}

export function ServiceAccountAccess({ account }: { account: ServiceAccount }) {
  const { t } = useTranslation();
  const [changing, setChanging] = useState(false);
  return (
    <ServiceAccountSection
      title={t("serviceAccounts.accessTitle")}
      explains={t("serviceAccounts.accessExplains")}
      action={
        !changing && (
          <Button variant="outline" size="sm" onClick={() => setChanging(true)}>
            {t("serviceAccounts.changeAccess")}
          </Button>
        )
      }
    >
      {changing ? (
        <GrantEditor
          person={asPerson(account)}
          onDone={() => setChanging(false)}
        />
      ) : (
        <ServiceAccountGrants account={account} />
      )}
    </ServiceAccountSection>
  );
}
