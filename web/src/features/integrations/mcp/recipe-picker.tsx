import { useTranslation } from "react-i18next";
import { BookOpen } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Separator } from "@/components/ui/separator";
import type { ServerRecipe } from "@/features/integrations/mcp/api";

/**
 * What the platform has read about servers other people publish.
 *
 * Deliberately not a marketplace. Every card says who publishes the server and
 * whose page this was read from, and neither is FuseOne: nothing here is
 * hosted, supported or endorsed. Choosing one fills the form below and commits
 * to nothing — the connection, the surface and every ruling stay separate acts
 * somebody performs.
 */
export function RecipePicker({
  recipes,
  onChoose,
}: {
  recipes: ServerRecipe[];
  onChoose: (recipe: ServerRecipe) => void;
}) {
  const { t } = useTranslation();

  return (
    <div className="space-y-3">
      <p className="text-xs text-muted-foreground">{t("mcp.recipesAre")}</p>
      <ScrollArea className="max-h-[320px]">
        <div className="grid gap-2 pr-3 sm:grid-cols-2">
          {recipes.map((recipe) => (
            <RecipeCard key={recipe.server} recipe={recipe} onChoose={onChoose} />
          ))}
        </div>
      </ScrollArea>
      <Separator />
    </div>
  );
}

function RecipeCard({
  recipe,
  onChoose,
}: {
  recipe: ServerRecipe;
  onChoose: (recipe: ServerRecipe) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="rounded-xl border p-3 shadow-sm">
      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0">
          <p className="truncate text-sm font-medium">{recipe.title}</p>
          {/* Who publishes it, always. A recipe with no name behind it is an
              anonymous opinion wearing this platform's chrome. */}
          <p className="truncate text-xs text-muted-foreground">
            {t("mcp.publishedBy", { publisher: recipe.publisher })}
          </p>
        </div>
        <Button size="sm" variant="outline" onClick={() => onChoose(recipe)}>
          {t("mcp.useRecipe")}
        </Button>
      </div>

      <div className="mt-2 flex flex-wrap items-center gap-1.5">
        <Badge
          variant="outline"
          className="rounded-pill border-transparent bg-muted text-2xs font-normal text-muted-foreground"
        >
          {t(`mcp.docsFrom.${recipe.docsFrom}`)}
        </Badge>
        <Badge
          variant="outline"
          className="rounded-pill border-transparent bg-muted text-2xs font-normal text-muted-foreground"
        >
          {t(`mcp.provenance.${recipe.provenance}`)}
        </Badge>
        {recipe.docs && (
          <a
            href={recipe.docs}
            target="_blank"
            rel="noreferrer noopener"
            className="inline-flex items-center gap-1 text-2xs text-muted-foreground underline underline-offset-2"
          >
            <BookOpen className="size-3" />
            {t("mcp.readTheDocs")}
          </a>
        )}
      </div>

      {recipe.note && (
        <p className="mt-2 line-clamp-3 text-xs text-muted-foreground">
          {recipe.note}
        </p>
      )}
    </div>
  );
}
