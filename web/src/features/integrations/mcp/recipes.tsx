import type { UseFormReturn } from "react-hook-form";
import { RecipePicker } from "@/features/integrations/mcp/recipe-picker";
import { useRecipes } from "@/features/integrations/mcp/api";
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
export function Recipes({ form }: { form: UseFormReturn<ServerFormValues> }) {
  const { data } = useRecipes();
  const recipes = data?.items ?? [];

  if (recipes.length === 0) return null;

  return (
    <RecipePicker
      recipes={recipes}
      onChoose={(recipe) => {
        form.setValue("name", recipe.server);
        if (recipe.transport) form.setValue("transport", recipe.transport);
        form.setValue("command", recipe.command ?? "");
        form.setValue("args", (recipe.args ?? []).join(" "));
        form.setValue("url", recipe.url ?? "");
        // Never the acceptance. A recipe that ticked the box would be the
        // catalogue deciding to run code inside the worker.
      }}
    />
  );
}
