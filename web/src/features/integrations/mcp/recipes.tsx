import { useEffect, useRef } from "react";
import type { UseFormReturn } from "react-hook-form";
import { RecipePicker } from "@/features/integrations/mcp/recipe-picker";
import { useRecipes, type ServerRecipe } from "@/features/integrations/mcp/api";
import type { ServerFormValues } from "@/features/integrations/server-schema";

/**
 * The recipes, offered where there is room for them.
 *
 * They lived inside the connection dialog first, which was an attempt to save
 * building a screen and cost one instead: a grid of cards has no business in a
 * modal, and it drew straight over the fields underneath. A catalogue is
 * something somebody reads before deciding, and reading needs a page.
 *
 * Choosing one fills the form and submits nothing. Everything that follows —
 * accepting local execution, the credential, the surface, every ruling — stays
 * an act somebody performs, which is the difference between a recipe and a
 * connector.
 */
export function Recipes({
  form,
  chosen,
}: {
  form: UseFormReturn<ServerFormValues>;
  /** A recipe already picked on the catalogue, if the visit came from there. */
  chosen?: string | null;
}) {
  const { data } = useRecipes();
  const recipes = data?.items ?? [];
  const applied = useRef<string | null>(null);

  // Filled once, and only from the visit that named it. Re-applying on every
  // render would overwrite whatever somebody had started typing.
  useEffect(() => {
    if (!chosen || applied.current === chosen) return;
    const one = recipes.find((r) => r.server === chosen);
    if (!one) return;
    applied.current = chosen;
    fill(form, one);
  }, [chosen, recipes, form]);

  if (recipes.length === 0) return null;

  return (
    <RecipePicker
      recipes={recipes}
      onChoose={(recipe) => fill(form, recipe)}
    />
  );
}

/*
fill puts a recipe into the form, and stops there.

Never the acceptance of local execution: a recipe that ticked that box would be
the catalogue deciding to run code inside the worker, which is the one decision
this whole design keeps with a person.
*/
function fill(form: UseFormReturn<ServerFormValues>, recipe: ServerRecipe) {
  form.setValue("name", recipe.server);
  if (recipe.transport) form.setValue("transport", recipe.transport);
  form.setValue("protocolMode", recipe.protocolMode ?? "auto");
  form.setValue("command", recipe.command ?? "");
  form.setValue("args", (recipe.args ?? []).join(" "));
  form.setValue("url", recipe.url ?? "");
}
