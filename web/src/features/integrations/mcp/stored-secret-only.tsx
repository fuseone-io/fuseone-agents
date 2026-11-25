import { useTranslation } from "react-i18next";
import { Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";

/**
 * A token or header stored before the recipe said the server takes something
 * else. No shape offered here can show it, and it can still be taken back.
 */
export function StoredSecretOnly({ onRevoke }: { onRevoke: () => void }) {
  const { t } = useTranslation();
  return (
    <div className="space-y-3 border-t pt-3">
      <p className="rounded-lg border border-warning/30 bg-warning-surface px-3 py-2 text-xs text-warning">
        {t("mcp.storedBearerOutsideRecipe")}
      </p>
      <Button type="button" variant="outline" size="sm" onClick={onRevoke} className="text-danger">
        <Trash2 className="size-3.5" />
        {t("mcp.revoke")}
      </Button>
    </div>
  );
}
