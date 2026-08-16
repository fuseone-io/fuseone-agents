import { useTranslation } from "react-i18next";
import type { UseFormReturn } from "react-hook-form";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible";
import { Button } from "@/components/ui/button";
import { ChevronDown } from "lucide-react";
import { RecipePicker } from "@/features/integrations/mcp/recipe-picker";
import { useRecipes } from "@/features/integrations/mcp/api";
import type { ServerFormValues } from "@/features/integrations/server-schema";

/**
 * The recipes, offered where a server is created.
 *
 * Folded away rather than absent: somebody who knows their address should not
 * have to scroll past a catalogue, and somebody who does not should not have
 * to know one exists.
 *
 * Choosing one fills the form and submits nothing. Everything that follows —
 * accepting local execution, the credential, the surface, every ruling — stays
 * an act somebody performs, which is the difference between a recipe and a
 * connector.
 */
export function Recipes({ form }: { form: UseFormReturn<ServerFormValues> }) {
  const { t } = useTranslation();
  const { data } = useRecipes();
  const recipes = data?.items ?? [];

  if (recipes.length === 0) return null;

  return (
    <Collapsible>
      <CollapsibleTrigger asChild>
        <Button variant="outline" size="sm" className="w-full justify-between">
          {t("mcp.startFromARecipe")}
          <ChevronDown className="size-3.5" />
        </Button>
      </CollapsibleTrigger>
      <CollapsibleContent className="pt-3">
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
      </CollapsibleContent>
    </Collapsible>
  );
}
