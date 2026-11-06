import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import type { ConnectorEvidenceConfig } from "@/features/integrations/api";
import { evidenceExamples } from "@/features/integrations/connectors/evidence-examples";
import { parseEvidenceDeclaration } from "@/features/integrations/connectors/evidence-instance-model";

export function EvidenceDeclarationImport({
  onApply,
}: {
  onApply: (declaration: ConnectorEvidenceConfig) => void;
}) {
  const { t } = useTranslation();
  const [text, setText] = useState("");
  const [refused, setRefused] = useState(false);

  function applyPasted() {
    const declaration = parseEvidenceDeclaration(text);
    setRefused(declaration === null);
    if (declaration) onApply(declaration);
  }

  return (
    <section className="grid gap-3 rounded-lg border p-3">
      <div className="grid gap-2">
        <Label id="evidence-example-label">
          {t("connectors.evidenceStartFrom")}
        </Label>
        <Select
          value=""
          onValueChange={(id) => {
            const example = evidenceExamples.find((e) => e.id === id);
            if (example) onApply(example.declaration);
          }}
        >
          <SelectTrigger
            aria-labelledby="evidence-example-label"
            className="w-full"
          >
            <SelectValue
              placeholder={t("connectors.evidenceStartFromPlaceholder")}
            />
          </SelectTrigger>
          <SelectContent>
            {evidenceExamples.map((example) => (
              <SelectItem key={example.id} value={example.id}>
                {t(example.label)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <p className="text-xs text-muted-foreground">
          {t("connectors.evidenceStartFromHint")}
        </p>
      </div>
      <Collapsible className="grid gap-2">
        <CollapsibleTrigger asChild>
          <Button type="button" variant="outline" size="sm" className="w-fit">
            {t("connectors.evidencePasteJson")}
          </Button>
        </CollapsibleTrigger>
        <CollapsibleContent className="grid gap-2">
          <Label htmlFor="evidence-declaration">
            {t("connectors.evidenceDeclarationJson")}
          </Label>
          <Textarea
            id="evidence-declaration"
            value={text}
            onChange={(event) => setText(event.target.value)}
            className="min-h-32 font-mono text-xs"
            spellCheck={false}
          />
          {refused && (
            <Alert variant="destructive">
              <AlertDescription>
                {t("connectors.evidenceDeclarationInvalid")}
              </AlertDescription>
            </Alert>
          )}
          <Button
            type="button"
            size="sm"
            className="w-fit"
            onClick={applyPasted}
          >
            {t("connectors.evidenceApplyJson")}
          </Button>
        </CollapsibleContent>
      </Collapsible>
    </section>
  );
}
