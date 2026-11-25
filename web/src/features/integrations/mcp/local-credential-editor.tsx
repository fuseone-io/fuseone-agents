import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { CredentialFields } from "@/features/integrations/mcp/credential-fields";
import { dsnEnvMode } from "@/features/integrations/mcp/auth-plan";
import type { ServerRecipe } from "@/features/integrations/mcp/api";
import {
  blankCredential,
  localCredential,
} from "@/features/integrations/mcp/credential-value";
import type {
  ServerConnection,
  ServerCredentialWrite,
} from "@/features/integrations/mcp/use-server-credential-write";
import type { MCPServer } from "@/features/integrations/api";

/** The variables and configuration file a local server is started with. */
export function LocalCredentialEditor({
  server,
  recipe,
  connection,
}: {
  server: MCPServer;
  recipe?: ServerRecipe | null;
  connection: ServerConnection;
}) {
  const { t } = useTranslation();
  const dsnMode = dsnEnvMode(recipe?.authModes);
  const storedConfigEnv = server.configFileEnv ?? "";
  const [value, setValue] = useState(() => blankCredential(storedConfigEnv));
  const dsnChanged = dsnMode !== null && value.dsn !== "";

  async function write(credential: ServerCredentialWrite) {
    if (await connection.write(credential)) {
      setValue(blankCredential(credential.configFileEnv ?? storedConfigEnv));
    }
  }

  return (
    <>
      <CredentialFields
        local
        hasSecret={server.hasVariables === true}
        hasConfigFile={server.hasConfigFile === true}
        dsnLabel={dsnMode ? (dsnMode.label ?? t("mcp.dsn")) : undefined}
        dsnHint={dsnMode ? t("mcp.dsnHint", { env: dsnMode.env ?? "DATABASE_URL" }) : undefined}
        value={value}
        onChange={(next) => setValue((current) => ({ ...current, ...next }))}
        // Explicit, and empty rather than absent: the two are different
        // requests and only one of them is somebody revoking.
        onRevoke={() => void write({ env: {} })}
        onRevokeConfigFile={() => void write({ configFile: "" })}
      />
      <div className="flex justify-end">
        <Button
          onClick={() =>
            void write({
              ...localCredential(value, dsnMode),
              ...(value.configFile === "" ? {} : { configFile: value.configFile }),
              configFileEnv: value.configFileEnv,
            })
          }
          disabled={
            connection.pending ||
            (value.env === "" &&
              !dsnChanged &&
              value.configFile === "" &&
              value.configFileEnv === storedConfigEnv)
          }
        >
          {t("mcp.saveCredential")}
        </Button>
      </div>
    </>
  );
}
