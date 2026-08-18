import { Trans, useTranslation } from "react-i18next";
import { TriangleAlert } from "lucide-react";
import { Mono } from "@/components/shared/mono";

/**
 * The arguments as recorded, pretty-printed when they are JSON and verbatim
 * when they are not. An approver has to see what will actually be sent, not a
 * summary of it.
 */
export function DecisionArguments({ body }: { body?: string }) {
  const { t } = useTranslation();
  if (!body) {
    return (
      <p className="rounded-lg border border-border bg-muted p-3 text-sm text-muted-foreground">
        {t("runs.argumentsGone")}
      </p>
    );
  }
  return (
    <pre className="max-h-[min(48vh,28rem)] max-w-full overflow-auto rounded-lg border border-border bg-muted p-3 font-mono text-xs whitespace-pre-wrap break-words">
      {pretty(body)}
    </pre>
  );
}

function pretty(body: string): string {
  try {
    return JSON.stringify(JSON.parse(body), null, 2);
  } catch {
    return body;
  }
}

/**
 * Why this call was escalated: the taint the arguments carry.
 *
 * Usually the whole reason a human was asked at all, so it sits next to the
 * arguments rather than behind a tooltip.
 */
export function DecisionProvenance({ labels }: { labels?: string[] }) {
  if (!labels?.length) return null;
  return (
    <p className="mt-2.5 flex items-start gap-2 text-xs text-muted-foreground">
      <TriangleAlert
        className="mt-px size-3.5 shrink-0 text-warning"
        aria-hidden
      />
      <span>
        <Trans
          i18nKey="runs.taintedArguments"
          values={{ labels: labels.join(", ") }}
          components={{ l: <Mono /> }}
        />
      </span>
    </p>
  );
}
