import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { IntegrationCard } from "@/features/integrations/integration-card";
import { RemoveButton } from "@/components/shared/remove-button";
import {
  useDeleteProvider,
  type ModelProvider,
} from "@/features/integrations/api";

/**
 * A model provider.
 *
 * Nothing connects to one until a run needs it, so there is no attempt to
 * report and the card does not pretend to have observed anything. A provider
 * with no credential reads as switched off, because that is what it is: no
 * run using it can advance.
 */
export function ProviderCard({
  provider,
  onEdit,
}: {
  provider: ModelProvider;
  onEdit: () => void;
}) {
  const { t } = useTranslation();
  const remove = useDeleteProvider();

  return (
    <IntegrationCard
      name={provider.name}
      kind={provider.kind}
      description={provider.baseUrl || t("integrations.defaultEndpoint")}
      enabled={provider.enabled && provider.hasKey}
      observes={false}
      action={
        <div className="flex items-center gap-1">
          <Button variant="ghost" size="sm" className="h-7" onClick={onEdit}>
            {t("agents.edit")}
          </Button>
          <RemoveButton
            title={`Remover ${provider.name}?`}
            description={t("integrations.removeProvider")}
            onConfirm={() =>
              remove.mutate(provider.name, {
                onSuccess: () => toast.success(`${provider.name} removido`),
                onError: (e) =>
                  toast.error(
                    e instanceof Error ? e.message : t("common.removeFailed"),
                  ),
              })
            }
          />
        </div>
      }
    />
  );
}
