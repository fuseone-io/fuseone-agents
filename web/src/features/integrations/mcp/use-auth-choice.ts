import { useState } from "react";
import type { RemoteAuthPlan } from "@/features/integrations/mcp/auth-plan";
import {
  effectiveChoice,
  type AuthChoice,
  type StoredShape,
} from "@/features/integrations/mcp/auth-choice";

/**
 * The shape a form shows: the one somebody picked while it is still offered,
 * else the one stored or the first offered.
 *
 * Worked out on every render rather than fixed at the first. A page reads its
 * recipe after it renders, and a choice made before the recipe arrived is a
 * guess: a recipe offering only OAuth must not leave the form on a token field
 * whose save would send nothing it can store.
 */
export function useAuthChoice(
  choices: AuthChoice[],
  stored: StoredShape,
  plan: RemoteAuthPlan,
): [AuthChoice | null, (choice: AuthChoice) => void] {
  const [picked, setPicked] = useState<AuthChoice | null>(null);
  return [effectiveChoice(picked, choices, stored, plan), setPicked];
}

/** Whether a stored token or header has a shape here that can show it. */
export function secretShapeOffered(choices: AuthChoice[]) {
  return choices.some((choice) => choice === "token" || choice === "headers" || choice === "customHeader");
}
