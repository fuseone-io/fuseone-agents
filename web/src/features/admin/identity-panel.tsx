import { useState } from "react";
import { useTranslation } from "react-i18next";
import { KeyRound, Plus } from "lucide-react";
import { toast } from "sonner";
import { Panel } from "@/components/shared/panel";
import { Button } from "@/components/ui/button";
import {
  EmptyState,
  ErrorState,
  LoadingRows,
} from "@/components/shared/states";
import { IdentityProviderForm } from "@/features/admin/provider-identity-form";
import { IdentityProviderRow } from "@/features/admin/identity-provider-row";
import {
  useDeleteIdentityProvider,
  useIdentityProviders,
  type IdentityProvider,
} from "@/features/admin/identity-api";

/**
 * How people sign in.
 *
 * An installation with none has exactly one person in it — whoever claimed the
 * setup token — so this is the screen that makes a second colleague possible.
 * It is also where the difference between authenticating and being allowed to
 * do something is made visible: a provider with no group mapping lets people
 * in and grants them nothing.
 */
export function IdentityPanel() {
  const { t } = useTranslation();
  const { data, isLoading, error, refetch } = useIdentityProviders();
  const remove = useDeleteIdentityProvider();
  const [editing, setEditing] = useState<IdentityProvider | null>(null);
  const [adding, setAdding] = useState(false);

  const providers = data?.items ?? [];

  if (adding || editing) {
    return (
      <Panel title={t(editing ? "identity.edit" : "identity.new")}>
        <IdentityProviderForm
          provider={editing ?? undefined}
          onDone={() => {
            setAdding(false);
            setEditing(null);
          }}
        />
      </Panel>
    );
  }

  return (
    <Panel
      title={t("identity.title")}
      action={
        <Button size="sm" onClick={() => setAdding(true)}>
          <Plus className="size-4" aria-hidden />
          {t("identity.newProvider")}
        </Button>
      }
    >
      {isLoading ? (
        <LoadingRows rows={3} />
      ) : error ? (
        <ErrorState error={error} onRetry={() => void refetch()} />
      ) : providers.length === 0 ? (
        <EmptyState
          icon={<KeyRound className="size-6" />}
          title={t("identity.emptyTitle")}
          hint={t("identity.emptyHint")}
        />
      ) : (
        <ul className="flex flex-col gap-3">
          {providers.map((provider) => (
            <li key={provider.id}>
              <IdentityProviderRow
                provider={provider}
                onEdit={() => setEditing(provider)}
                onRemove={() =>
                  remove.mutate(provider.id, {
                    onSuccess: () =>
                      toast.success(
                        t("identity.removed", { name: provider.id }),
                      ),
                    onError: (e) =>
                      toast.error(
                        e instanceof Error
                          ? e.message
                          : t("common.removeFailed"),
                      ),
                  })
                }
              />
            </li>
          ))}
        </ul>
      )}
    </Panel>
  );
}
