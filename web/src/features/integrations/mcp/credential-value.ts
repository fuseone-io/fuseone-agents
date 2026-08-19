import {
  headerCredential,
  multiHeaderCredential,
  type AuthMode,
  type RemoteAuthPlan,
} from "@/features/integrations/mcp/auth-plan";
import { readVariables } from "@/features/integrations/mcp/variables";

export type CredentialValue = {
  token: string;
  headers: Record<string, string>;
  dsn: string;
  env: string;
  configFile: string;
  configFileEnv: string;
  oauthAccessToken: string;
  oauthRefreshToken: string;
  oauthTokenURL: string;
  oauthClientID: string;
  oauthClientSecret: string;
  oauthTokenType: string;
  oauthExpiresAtUnix: string;
  oauthScopes: string;
};

export function blankCredential(configFileEnv = ""): CredentialValue {
  return {
    token: "",
    headers: {},
    dsn: "",
    env: "",
    configFile: "",
    configFileEnv,
    oauthAccessToken: "",
    oauthRefreshToken: "",
    oauthTokenURL: "",
    oauthClientID: "",
    oauthClientSecret: "",
    oauthTokenType: "",
    oauthExpiresAtUnix: "",
    oauthScopes: "",
  };
}

export function remoteCredential(
  value: CredentialValue,
  plan: RemoteAuthPlan,
  headers: string[],
) {
  if (value.token !== "" && plan.secret?.type === "bearer") {
    return { token: value.token };
  }
  if (value.token !== "" && plan.secret) {
    return { headers: headerCredential(plan.secret, value.token) };
  }
  if (headers.length > 0) {
    return { headers: multiHeaderCredential(headers, value.headers) };
  }
  return {};
}

export function localCredential(value: CredentialValue, dsnMode: AuthMode | null) {
  if (value.env === "" && (dsnMode === null || value.dsn === "")) {
    return {};
  }
  const env = value.env === "" ? {} : readVariables(value.env);
  if (dsnMode?.env && value.dsn !== "") {
    env[dsnMode.env] = value.dsn;
  }
  return { env };
}
