import { useState } from "react";
import { useTranslation } from "react-i18next";
import { ChevronDown } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import type { OAuthCredentialInput } from "@/features/integrations/mcp/oauth-credential";

type Field = keyof OAuthCredentialInput;

/**
 * The inputs of a manual OAuth grant, grouped by what each part is.
 *
 * The one copy, for every form that writes a grant: a server's page, a person's
 * own credential and the connect form. Where the token comes from and whose
 * client it is, then the tokens, then what is rarely needed — the token type,
 * the expiry and the scopes — closed until somebody opens it, unless it
 * already holds something.
 */
export function OAuthGrantFields({
  value,
  onChange,
}: {
  value: OAuthCredentialInput;
  onChange: (field: Field, next: string) => void;
}) {
  const { t } = useTranslation();
  const advancedFilled = [value.oauthTokenType, value.oauthExpiresAtUnix, value.oauthScopes]
    .some((part) => part.trim() !== "");
  const [advanced, setAdvanced] = useState(advancedFilled);
  const input = (field: Field, label: string, secret = false, placeholder?: string) => (
    <GrantInput
      spec={{ id: INPUT_IDS[field], label, secret, placeholder }}
      value={value[field]}
      onChange={(next) => onChange(field, next)}
    />
  );
  return (
    <div className="space-y-4">
      <Group title={t("mcp.oauthGroupEndpoint")}>
        {input("oauthTokenURL", t("mcp.oauthTokenURL"), false, t("mcp.oauthTokenURLPlaceholder"))}
        {input("oauthClientID", t("mcp.oauthClientID"))}
        {input("oauthClientSecret", t("mcp.oauthClientSecret"), true)}
      </Group>
      <Group title={t("mcp.oauthGroupTokens")}>
        {input("oauthAccessToken", t("mcp.oauthAccessToken"), true)}
        {input("oauthRefreshToken", t("mcp.oauthRefreshToken"), true)}
      </Group>
      <Collapsible open={advanced || advancedFilled} onOpenChange={setAdvanced}>
        <CollapsibleTrigger asChild>
          <Button type="button" variant="ghost" size="sm" className="-ml-2">
            <ChevronDown className="size-3.5" />
            {t("mcp.oauthGroupAdvanced")}
          </Button>
        </CollapsibleTrigger>
        <CollapsibleContent className="grid gap-3 pt-2 sm:grid-cols-2">
          {input("oauthTokenType", t("mcp.oauthTokenType"), false, t("mcp.oauthTokenTypePlaceholder"))}
          {input("oauthExpiresAtUnix", t("mcp.oauthExpiresAtUnix"))}
          <div className="space-y-1.5 sm:col-span-2">
            <Label htmlFor="oauth-scopes">{t("mcp.oauthScopes")}</Label>
            <Textarea
              id="oauth-scopes"
              rows={2}
              className="font-mono text-xs"
              placeholder={t("mcp.oauthScopesPlaceholder")}
              value={value.oauthScopes}
              onChange={(e) => onChange("oauthScopes", e.target.value)}
            />
          </div>
        </CollapsibleContent>
      </Collapsible>
    </div>
  );
}

function Group({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="space-y-2">
      <h4 className="text-xs font-medium text-muted-foreground">{title}</h4>
      <div className="grid gap-3 sm:grid-cols-2">{children}</div>
    </div>
  );
}

const INPUT_IDS: Record<Field, string> = {
  oauthAccessToken: "oauth-access-token",
  oauthRefreshToken: "oauth-refresh-token",
  oauthTokenURL: "oauth-token-url",
  oauthClientID: "oauth-client-id",
  oauthClientSecret: "oauth-client-secret",
  oauthTokenType: "oauth-token-type",
  oauthExpiresAtUnix: "oauth-expires-at",
  oauthScopes: "oauth-scopes",
};

function GrantInput({
  spec,
  value,
  onChange,
}: {
  spec: { id: string; label: string; secret: boolean; placeholder?: string };
  value: string;
  onChange: (next: string) => void;
}) {
  return (
    <div className="space-y-1.5">
      <Label htmlFor={spec.id}>{spec.label}</Label>
      <Input
        id={spec.id}
        type={spec.secret ? "password" : "text"}
        autoComplete="off"
        className="font-mono"
        placeholder={spec.placeholder}
        value={value}
        onChange={(e) => onChange(e.target.value)}
      />
    </div>
  );
}
