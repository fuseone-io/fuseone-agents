import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import { Compass } from "lucide-react";
import { Button } from "@/components/ui/button";

/**
 * A route that matches nothing.
 *
 * Without it React Router renders an empty content area with no error at all,
 * so a link pointing at a screen nobody built looks exactly like a screen that
 * loaded and had nothing to show.
 */
export function NotFoundPage() {
  const { t } = useTranslation();

  return (
    <div className="flex flex-col items-center gap-3 rounded-lg border border-dashed px-6 py-16 text-center">
      <Compass className="size-6 text-muted-foreground" aria-hidden />
      <p className="font-medium">{t("notFound.title")}</p>
      <p className="max-w-md text-sm text-muted-foreground">
        {t("notFound.hint")}
      </p>
      <Button asChild variant="outline" size="sm" className="mt-1">
        <Link to="/overview">{t("notFound.goToOverview")}</Link>
      </Button>
    </div>
  );
}
