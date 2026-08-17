import { useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Panel } from "@/components/shared/panel";
import { Button } from "@/components/ui/button";
import { Mono } from "@/components/shared/mono";
import { problemMessage } from "@/lib/api/problem-message";
import { CredentialFields } from "@/features/integrations/mcp/credential-fields";
import { readVariables } from "@/features/integrations/mcp/variables";
import {
  usePutMCPServer,
  type MCPServer,
} from "@/features/integrations/api";

/**
 * How this server is reached, and what it is given to reach it with.
 *
 * The credential is the part with a gesture of its own. Leaving a field empty
 * means "keep what is stored", because correcting an address must not demand
 * re-entering a secret nobody has to hand — and with only that rule a
 * credential could be written and never taken back, which is the half that
 * matters on the day it leaks.
 */
export function ConnectionPanel({ server }: { server: MCPServer }) {
  const { t } = useTranslation();
  const put = usePutMCPServer();
  const [value, setValue] = useState({
    token: "",
    env: "",
    configFile: "",
    configFileEnv: server.configFileEnv ?? "",
  });
  const local = (server.transport ?? "stdio") === "stdio";
  const storedConfigEnv = server.configFileEnv ?? "";

  async function write(credential: {
    token?: string;
    env?: Record<string, string>;
    configFile?: string;
    configFileEnv?: string;
  }) {
    // Passed through exactly as given. An undefined token means this write is
    // not about the token, and an empty one means somebody is removing it —
    // collapsing the two is how a revoke button stops revoking.
    try {
      await put.mutateAsync({
        name: server.name,
        transport: server.transport ?? "stdio",
        command: server.command ?? "",
        args: server.args ?? [],
        url: server.url ?? "",
        enabled: server.enabled,
        acceptsLocalExecution: server.acceptsLocalExecution ?? false,
        token: credential.token,
        env: credential.env,
        configFile: credential.configFile,
        configFileEnv: credential.configFileEnv,
      });
      setValue({
        token: "",
        env: "",
        configFile: "",
        configFileEnv: credential.configFileEnv ?? storedConfigEnv,
      });
      toast.success(t("mcp.credentialSaved"));
    } catch (problem) {
      toast.error(problemMessage(problem, t));
    }
  }

  return (
    <Panel title={t("mcp.connection")}>
      <div className="space-y-4">
        <dl className="grid gap-1 text-xs">
          <Row label={t("integrations.transport")} value={server.transport ?? "stdio"} />
          {local ? (
            <Row label={t("integrations.command")} value={[server.command, ...(server.args ?? [])].join(" ")} />
          ) : (
            <Row label={t("integrations.url")} value={server.url ?? ""} />
          )}
        </dl>

        <CredentialFields
          local={local}
          hasSecret={local ? server.hasVariables === true : server.hasSecret === true}
          hasConfigFile={server.hasConfigFile === true}
          value={value}
          onChange={setValue}
          onRevoke={() =>
            // Explicit, and empty rather than absent: the two are different
            // requests and only one of them is somebody revoking.
            void write(local ? { env: {} } : { token: "" })
          }
          onRevokeConfigFile={() => void write({ configFile: "" })}
        />

        <div className="flex justify-end">
          <Button
            onClick={() =>
              void write(
                local
                  ? {
                      ...(value.env === "" ? {} : { env: readVariables(value.env) }),
                      ...(value.configFile === ""
                        ? {}
                        : { configFile: value.configFile }),
                      configFileEnv: value.configFileEnv,
                    }
                  : { token: value.token },
              )
            }
            disabled={
              put.isPending ||
              (local
                ? value.env === "" &&
                  value.configFile === "" &&
                  value.configFileEnv === storedConfigEnv
                : value.token === "")
            }
          >
            {t("mcp.saveCredential")}
          </Button>
        </div>
      </div>
    </Panel>
  );
}

function Row({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex gap-2">
      <dt className="w-28 shrink-0 text-muted-foreground">{label}</dt>
      <dd className="min-w-0 truncate">
        <Mono className="text-xs">{value}</Mono>
      </dd>
    </div>
  );
}
