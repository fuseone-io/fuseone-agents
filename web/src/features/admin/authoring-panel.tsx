import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Sparkles } from "lucide-react";
import { toast } from "sonner";
import { Panel } from "@/components/shared/panel";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  EmptyState,
  ErrorState,
  LoadingRows,
} from "@/components/shared/states";
import { Labelled } from "@/features/policies/section";
import { useAuthoring, useSetAuthoring } from "@/features/admin/authoring-api";
import { useIntegrations } from "@/features/integrations/api";

/**
 * The model the interview uses.
 *
 * A choice, not a second registry: the connection lives in Integrações and
 * this points at it. Which is why the provider is a select over what is
 * already connected and never a field somebody types — a name typed here that
 * nobody connected fails at the moment an author is halfway through
 * describing a process.
 */
export function AuthoringPanel() {
  const { t } = useTranslation();
  const { data, isLoading, error, refetch } = useAuthoring();
  const providers = useIntegrations().data?.providers ?? [];
  const save = useSetAuthoring();

  const [draft, setDraft] = useState<{
    provider: string;
    model: string;
    dailyMicros: number;
  } | null>(null);
  const current = draft ?? {
    provider: data?.provider ?? "",
    model: data?.model ?? "",
    dailyMicros: data?.dailyMicros ?? 0,
  };

  const submit = (enabled: boolean) =>
    save.mutate(
      {
        provider: current.provider,
        model: current.model,
        dailyMicros: current.dailyMicros,
        enabled,
      },
      {
        onSuccess: () =>
          toast.success(
            t(enabled ? "admin.authoringSet" : "admin.authoringOff"),
          ),
        onError: (e) =>
          toast.error(e instanceof Error ? e.message : t("common.saveFailed")),
      },
    );

  if (isLoading) return <LoadingRows rows={2} />;
  if (error) return <ErrorState error={error} onRetry={() => void refetch()} />;

  return (
    <Panel title={t("admin.authoring")}>
      {providers.length === 0 ? (
        <EmptyState
          icon={<Sparkles className="size-6" />}
          title={t("admin.noProviderForAuthoring")}
          hint={t("admin.connectFirst")}
        />
      ) : (
        <div className="flex flex-col gap-4">
          <p className="text-xs text-muted-foreground">
            {t("admin.authoringHint")}
          </p>

          <div className="grid gap-3 sm:grid-cols-[1fr_1fr_1fr_auto]">
            <Labelled label={t("agents.provider")} htmlFor="authoring-provider">
              <Select
                value={current.provider || undefined}
                onValueChange={(provider) => setDraft({ ...current, provider })}
              >
                <SelectTrigger
                  id="authoring-provider"
                  className="w-full font-mono"
                >
                  <SelectValue placeholder={t("admin.choose")} />
                </SelectTrigger>
                <SelectContent>
                  {providers.map((p) => (
                    <SelectItem
                      key={p.name}
                      value={p.name}
                      className="font-mono"
                    >
                      {p.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Labelled>

            <Labelled label={t("admin.model")} htmlFor="authoring-model">
              <Input
                id="authoring-model"
                value={current.model}
                onChange={(e) =>
                  setDraft({ ...current, model: e.target.value })
                }
                className="font-mono"
                placeholder="claude-opus-5"
              />
            </Labelled>

            {/* Required, not optional. This is the only place the platform
                spends outside a run, so the bound is part of configuring it. */}
            <Labelled label={t("admin.dailyCeiling")} htmlFor="authoring-cap">
              <Input
                id="authoring-cap"
                inputMode="decimal"
                value={
                  current.dailyMicros
                    ? String(current.dailyMicros / 1_000_000)
                    : ""
                }
                onChange={(e) =>
                  setDraft({
                    ...current,
                    dailyMicros:
                      Math.round(
                        Number(e.target.value.replace(",", ".")) * 1_000_000,
                      ) || 0,
                  })
                }
                className="font-mono"
                placeholder="5"
              />
            </Labelled>

            <div className="flex items-end">
              <Button
                size="sm"
                disabled={
                  !current.provider ||
                  !current.model ||
                  !current.dailyMicros ||
                  save.isPending
                }
                onClick={() => submit(true)}
              >
                {t("common.save")}
              </Button>
            </div>
          </div>

          {/* Off is a state somebody chooses, not a failure. An installation
              with no strong model still publishes agents through the form. */}
          <label className="flex items-center gap-2.5 text-sm">
            <Switch
              checked={data?.enabled ?? false}
              disabled={!data?.provider}
              onCheckedChange={(on) => submit(on)}
            />
            {t("admin.authoringEnabled")}
          </label>
        </div>
      )}
    </Panel>
  );
}
