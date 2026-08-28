import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Mono } from "@/components/shared/mono";
import { useReactivateMemoryAssertion } from "@/features/memory/api";
import {
  OWN_STATE,
  sharedIsImprovable,
} from "@/features/memory/memory-match-state";
import { problemMessage } from "@/lib/api/problem-message";
import type { MemoryAssertion, MemoryMatch } from "@/features/memory/api";

/**
 * What the platform already holds about the identity being taught.
 *
 * Shown before saving rather than answered with a conflict afterwards. The
 * point is not to block: teaching a fact that already exists corrects it, which
 * is usually what somebody means. The point is that they should know which of
 * the two they are doing.
 *
 * Every state offers only what the server will accept. A disabled memory cannot
 * be merged into, so saving is not offered and reactivating is; an erased one
 * offers nothing at all, because nothing is honest there.
 */
export function MemoryDuplicateNotice({
  match,
  reason,
  onImproveShared,
}: {
  match: MemoryMatch | undefined;
  /** The reason typed in the form, which reactivation records as its own. */
  reason: string;
  /** Switching the form to the shared namespace, so improving the memory every
   *  agent reads is a decision rather than a side effect. */
  onImproveShared: () => void;
}) {
  const { t } = useTranslation();
  if (!match?.own && !match?.shared && !match?.pending) return null;

  return (
    <div className="grid gap-2">
      {match.own && <Own assertion={match.own} reason={reason} />}
      {match.shared && (
        <Alert>
          <AlertTitle>{t("memory.matchShared")}</AlertTitle>
          <AlertDescription className="grid gap-2">
            <Identity assertion={match.shared} />
            <span>{t("memory.matchSharedHint")}</span>
            {sharedIsImprovable(match.shared) && (
              <Button
                type="button"
                size="sm"
                variant="outline"
                className="justify-self-start"
                onClick={onImproveShared}
              >
                {t("memory.improveShared")}
              </Button>
            )}
          </AlertDescription>
        </Alert>
      )}
      {match.pending && (
        <Alert>
          <AlertTitle>{t("memory.matchPending")}</AlertTitle>
          <AlertDescription>{t("memory.matchPendingHint")}</AlertDescription>
        </Alert>
      )}
    </div>
  );
}

function Own({
  assertion,
  reason,
}: {
  assertion: MemoryAssertion;
  reason: string;
}) {
  const { t } = useTranslation();
  const state = OWN_STATE[assertion.status];
  const reactivate = useReactivateMemoryAssertion();

  async function bringBack() {
    try {
      await reactivate.mutateAsync({
        id: assertion.id,
        company: assertion.scope.company,
        area: assertion.scope.area,
        reason,
      });
      toast.success(t("memory.reactivated"));
    } catch (error) {
      toast.error(problemMessage(error, t));
    }
  }

  return (
    <Alert>
      <AlertTitle>{t(state.says)}</AlertTitle>
      <AlertDescription className="grid gap-2">
        <Identity assertion={assertion} />
        {state.saving && <span>{t(state.saving)}</span>}
        {state.reactivable && (
          <Button
            type="button"
            size="sm"
            variant="outline"
            className="justify-self-start"
            // The server requires a reason and the form already asks for one.
            // Enabling this without it would send a request whose only possible
            // answer is a refusal.
            disabled={!reason.trim() || reactivate.isPending}
            onClick={bringBack}
          >
            {t(reason.trim() ? "memory.reactivate" : "memory.reactivateNeedsReason")}
          </Button>
        )}
      </AlertDescription>
    </Alert>
  );
}

/** The claim as it stands, so the difference from what is being typed is
 *  visible rather than implied by the identity matching. */
function Identity({ assertion }: { assertion: MemoryAssertion }) {
  return (
    <>
      <Mono dim className="block truncate">
        {assertion.signature}
      </Mono>
      <span className="line-clamp-3">{assertion.claim}</span>
    </>
  );
}
