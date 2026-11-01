import { useTranslation } from "react-i18next";
import { useState } from "react";
import { toast } from "sonner";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import { Mono } from "@/components/shared/mono";
import { RotateButton } from "@/features/agents/webhook-rotate-button";
import { WebhookSecretDialog } from "@/features/agents/webhook-secret-dialog";
import {
  useRotateWebhookSecret,
  useWebhooks,
} from "@/features/agents/webhooks-api";
import { formatInstant } from "@/lib/format";
import type { Webhook } from "@/lib/api/client";
import { problemMessage } from "@/lib/api/problem-message";

/**
 * The doors into this agent, and whether each one is open.
 *
 * A declared path with no secret refuses everything, which is the safe state
 * but a confusing one to discover through silence — so it says so, next to the
 * one action that changes it.
 */
export function WebhooksPanel({ agentId }: { agentId: string }) {
  const { t } = useTranslation();
  const { data, isLoading } = useWebhooks(agentId);
  const rotate = useRotateWebhookSecret(agentId);
  const [issued, setIssued] = useState<{
    secret: string;
    url: string;
    fromGitHub: boolean;
  }>();

  const hooks = data?.items ?? [];
  if (!isLoading && hooks.length === 0) return null;

  const generate = (hook: Webhook) => {
    rotate.mutate(hook.path, {
      onSuccess: (result) =>
        setIssued({ ...result, fromGitHub: hook.from === "github" }),
      onError: (error) =>
        toast.error(t("agents.keyFailed"), {
          description: problemMessage(error, t),
        }),
    });
  };

  return (
    <section className="flex flex-col gap-3 rounded-xl border border-border bg-card p-4 shadow-sm">
      <h2 className="text-2xs uppercase tracking-label text-muted-foreground">
        {t("agents.webhooks")}
      </h2>

      {isLoading ? (
        <Skeleton className="h-12 rounded-lg" />
      ) : (
        <ul className="flex flex-col gap-3">
          {hooks.map((hook) => (
            <li key={hook.path} className="flex flex-col gap-1.5">
              <div className="flex items-center gap-2">
                {/* A route, not a phrase. */}
                {/* eslint-disable-next-line i18next/no-literal-string */}
                <Mono className="truncate">/hooks/{hook.path}</Mono>
                {hook.from === "github" && (
                  <Badge variant="outline">{t("agents.fromGitHub")}</Badge>
                )}
              </div>
              <div className="flex items-center gap-2">
                <State hook={hook} />
                <RotateButton
                  hook={hook}
                  pending={rotate.isPending}
                  onGenerate={() => generate(hook)}
                />
              </div>
            </li>
          ))}
        </ul>
      )}

      <WebhookSecretDialog
        secret={issued?.secret}
        url={issued?.url}
        fromGitHub={issued?.fromGitHub}
        onClose={() => setIssued(undefined)}
      />
    </section>
  );
}

/** Closed is the safe state, and worth saying rather than leaving to silence. */
function State({ hook }: { hook: Webhook }) {
  const { t } = useTranslation();
  if (!hook.armed) {
    return (
      <span className="rounded-pill bg-warning-surface px-2 py-0.5 text-2xs font-medium text-warning">
        {t("agents.noKeyNoFire")}
      </span>
    );
  }
  return (
    <span className="truncate text-2xs text-muted-foreground">
      {t("agents.keyGenerated", {
        at: hook.rotatedAt ? formatInstant(hook.rotatedAt) : "",
        by: hook.rotatedBy ?? "",
      })}
    </span>
  );
}
