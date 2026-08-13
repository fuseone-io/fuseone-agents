import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Download } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { api, unwrap } from "@/lib/api/client";

/**
 * Downloads the run as a signed export.
 *
 * Straight to a file rather than into a viewer: what an auditor does with this
 * is check it with `agentd verify`, and a document reformatted by a browser on
 * the way is a document whose canonical bytes somebody has to reconstruct.
 */
export function ExportButton({ runId }: { runId: string }) {
  const { t } = useTranslation();
  const [working, setWorking] = useState(false);

  const download = async () => {
    setWorking(true);
    try {
      const bundle = unwrap(
        await api.GET("/audit/export", { params: { query: { runId } } }),
      );
      save(`${runId}.ledger.json`, JSON.stringify(bundle, null, 2));
      toast.success(t("runs.exported"), {
        description: t("runs.exportedHint"),
      });
    } catch (error) {
      toast.error(t("runs.exportFailed"), {
        description: error instanceof Error ? error.message : undefined,
      });
    } finally {
      setWorking(false);
    }
  };

  return (
    <Button
      variant="outline"
      size="sm"
      className="h-8"
      disabled={working}
      onClick={() => void download()}
    >
      <Download className="size-4" aria-hidden />
      {t("runs.export")}
    </Button>
  );
}

function save(name: string, body: string) {
  const url = URL.createObjectURL(
    new Blob([body], { type: "application/json" }),
  );
  const link = document.createElement("a");
  link.href = url;
  link.download = name;
  link.click();
  URL.revokeObjectURL(url);
}
