import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
import { CustomHeaderFields } from "@/features/integrations/mcp/custom-header-fields";
import {
  customHeaderCredential,
  customHeaderError,
  customHeaderState,
  type CustomHeader,
  type WriteHeaders,
} from "@/features/integrations/mcp/custom-header";

const blank: CustomHeader = { name: "", value: "" };

/**
 * The custom header of an existing server with no recipe, with its own save.
 *
 * Apart from the bearer field because it is a different act: saved, the header
 * replaces a stored token or OAuth grant, and saving both from one button would
 * make "which one did I just store" a question. Removing it is the panel's own
 * revoke, which clears every header.
 */
export function CustomHeaderSection({
  pending,
  kept,
  write,
}: {
  pending: boolean;
  kept: boolean;
  write: WriteHeaders;
}) {
  const { t } = useTranslation();
  const [value, setValue] = useState(blank);
  const state = customHeaderState(value);

  async function save() {
    if (await write({ headers: customHeaderCredential(value) })) setValue(blank);
  }

  return (
    <div className="space-y-3">
      <Separator />
      <CustomHeaderFields
        value={value}
        kept={kept}
        error={state === "empty" ? undefined : customHeaderError(value)}
        onChange={setValue}
      />
      <Button
        type="button"
        variant="outline"
        size="sm"
        disabled={pending || state !== "complete"}
        onClick={() => void save()}
      >
        {t("mcp.saveCustomHeader")}
      </Button>
    </div>
  );
}
