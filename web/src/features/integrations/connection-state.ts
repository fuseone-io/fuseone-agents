import type { IntegrationHealth } from "@/features/integrations/api";

/**
 * What a connection is, in one place.
 *
 * Four states, and collapsing them is how a screen starts lying: switched off,
 * configured but never reached, reached and refusing, answering. A server can
 * be enabled, correct and unreachable all at once, and only one of those three
 * is somebody's opinion.
 *
 * Shared because two cards asking the same question must not answer it
 * differently — the catalogue drew every configured server as running, which
 * painted a switched-off one green.
 */
export function stateOf(
  enabled: boolean,
  health?: IntegrationHealth | null,
  observes = true,
) {
  if (!enabled) {
    return {
      label: "integrations.stateOff",
      pill: "bg-muted text-muted-foreground",
      tile: "border-border bg-muted text-muted-foreground",
    };
  }
  if (health && !health.reachable) {
    return {
      label: "integrations.notAnswering",
      pill: "bg-danger-surface text-danger",
      tile: "border-danger bg-danger-surface text-danger",
    };
  }
  if (!observes) {
    return {
      label: "integrations.stateConfigured",
      pill: "bg-success-surface text-success",
      tile: "border-success bg-success-surface text-success",
    };
  }
  if (!health) {
    return {
      label: "integrations.noContact",
      pill: "bg-warning-surface text-warning",
      tile: "border-warning bg-warning-surface text-warning",
    };
  }
  return {
    label: "integrations.stateAnswering",
    pill: "bg-success-surface text-success",
    tile: "border-success bg-success-surface text-success",
  };
}
