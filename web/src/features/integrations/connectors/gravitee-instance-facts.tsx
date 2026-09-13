import type { TFunction } from "i18next";
import type { ConnectorInstance } from "@/features/integrations/api";

export function GraviteeInstanceFacts({
  instance,
  t,
}: {
  instance: ConnectorInstance;
  t: TFunction;
}) {
  const gravitee = instance.gravitee;
  const reference = gravitee?.allowedReferences[0];
  return (
    <>
      <Fact label={t("connectors.graviteeAddress")} value={gravitee?.address ?? "-"} />
      <Fact
        label={t("connectors.graviteeTarget")}
        value={gravitee ? `${gravitee.organization}/${gravitee.environment}` : "-"}
      />
      <Fact label={t("connectors.graviteeAPIReference")} value={reference?.id ?? "-"} />
      <Fact
        label={t("connectors.graviteeExpiryPolicy")}
        value={gravitee
          ? t(
              gravitee.allowNoExpiry
                ? "connectors.graviteeTTLRangeNoExpiry"
                : "connectors.graviteeTTLRange",
              { min: gravitee.minTTLSeconds, max: gravitee.maxTTLSeconds },
            )
          : "-"}
      />
      <Fact
        label={t("connectors.vaultInstance")}
        value={gravitee?.credentialSource.vaultInstance ?? "-"}
      />
    </>
  );
}

function Fact({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0">
      <p className="text-2xs uppercase text-muted-foreground">{label}</p>
      <p className="truncate font-mono">{value}</p>
    </div>
  );
}
