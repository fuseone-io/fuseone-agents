import { useTranslation } from "react-i18next";
import type { ReactNode } from "react";
import { Mono } from "@/components/shared/mono";
import { formatRelative } from "@/lib/format";
import { cn } from "@/lib/utils";
import type { IntegrationHealth } from "@/lib/api/client";

/**
 * One connected system, and whether it is answering.
 *
 * Configuration and observation are shown as different things because they are:
 * a server can be enabled, correct and unreachable, and only one of those three
 * is somebody's opinion. A card that reported only the configuration would call
 * a server that has refused connections for a week "enabled".
 */
export function IntegrationCard({
  name,
  kind,
  description,
  enabled,
  health,
  observes = true,
  action,
}: {
  name: string;
  kind: string;
  description: string;
  enabled: boolean;
  health?: IntegrationHealth | null;
  /**
   * Whether the platform ever tries to reach this on its own. False for a
   * model provider: nothing connects to one until a run needs it, so there is
   * no attempt to report and "no contact" would be a false alarm rather than
   * a finding.
   */
  observes?: boolean;
  action?: ReactNode;
}) {
  const state = stateOf(enabled, health, observes);

  return (
    <article className="flex flex-col gap-3 rounded-xl border border-border bg-card p-4 shadow-sm">
      <header className="flex items-start gap-3">
        <span
          aria-hidden
          className={cn(
            "flex size-[30px] shrink-0 items-center justify-center rounded-lg border font-mono text-xs uppercase",
            state.tile,
          )}
        >
          {name.slice(0, 2)}
        </span>

        <div className="min-w-0 flex-1">
          <h3 className="truncate text-sm font-medium">{name}</h3>
          <p className="truncate text-xs text-muted-foreground">{kind}</p>
        </div>

        <span
          className={cn(
            "shrink-0 rounded-pill px-2 py-0.5 text-2xs font-medium",
            state.pill,
          )}
        >
          {state.label}
        </span>
      </header>

      <p className="text-xs text-muted-foreground">{description}</p>

      {observes && <Observation health={health} />}

      {action && <div className="flex justify-end">{action}</div>}
    </article>
  );
}

/**
 * What the last attempt found — including that there has not been one.
 *
 * "Never tried" and "tried and failed" are different facts, and collapsing
 * them would let a server nobody has connected to look healthy.
 */
function Observation({ health }: { health?: IntegrationHealth | null }) {
  const { t } = useTranslation();
  if (!health) {
    return (
      <p className="text-2xs text-muted-foreground">
        {t("integrations.neverTried")}
      </p>
    );
  }

  return (
    <div className="flex flex-col gap-1">
      <p className="text-2xs text-muted-foreground">
        {health.reachable ? (
          <>
            <Mono dim>{health.toolCount}</Mono> ferramentas · visto{" "}
            {formatRelative(health.observedAt)}
          </>
        ) : (
          <>
            Não respondeu · última tentativa {formatRelative(health.observedAt)}
          </>
        )}
        {health.observedBy ? ` · por ${health.observedBy}` : ""}
      </p>

      {/* Shown as-is: the person reading it is the one who fixes the server. */}
      {!health.reachable && health.detail && (
        <p
          className="truncate font-mono text-2xs text-danger"
          title={health.detail}
        >
          {health.detail}
        </p>
      )}
    </div>
  );
}

/**
 * Three states, not two. Disabled is a decision, unreachable is a fact, and a
 * screen that painted both red would send somebody to debug a server that was
 * switched off on purpose.
 */
function stateOf(
  enabled: boolean,
  health?: IntegrationHealth | null,
  observes = true,
) {
  if (!enabled) {
    return {
      label: "desligado",
      pill: "bg-muted text-muted-foreground",
      tile: "border-border bg-muted text-muted-foreground",
    };
  }
  if (health && !health.reachable) {
    return {
      label: "não responde",
      pill: "bg-danger-surface text-danger",
      tile: "border-danger bg-danger-surface text-danger",
    };
  }
  if (!observes) {
    return {
      label: "configurado",
      pill: "bg-success-surface text-success",
      tile: "border-success bg-success-surface text-success",
    };
  }
  if (!health) {
    return {
      label: "sem contato",
      pill: "bg-warning-surface text-warning",
      tile: "border-warning bg-warning-surface text-warning",
    };
  }
  return {
    label: "respondendo",
    pill: "bg-success-surface text-success",
    tile: "border-success bg-success-surface text-success",
  };
}
