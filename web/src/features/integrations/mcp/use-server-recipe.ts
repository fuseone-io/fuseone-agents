import { useMemo } from "react";
import { useRecipes, type ServerRecipe } from "@/features/integrations/mcp/api";

/**
 * The recipe that describes a server, by its name: null when none does, and
 * undefined until the catalogue has been read, or when it could not be. The
 * difference matters: a field offered only to servers no recipe describes must
 * not appear while nobody knows yet.
 */
export function useServerRecipe(name: string | undefined): ServerRecipe | null | undefined {
  const recipes = useRecipes(name !== undefined);
  return useMemo(() => {
    if (name === undefined) return null;
    if (!recipes.data) return undefined;
    return (recipes.data.items ?? []).find((recipe) => recipe.server === name) ?? null;
  }, [name, recipes.data]);
}
