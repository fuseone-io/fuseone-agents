import { useTranslation } from "react-i18next";
import { Wrench } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Mono } from "@/components/shared/mono";

export function StepReaches({ reaches }: { reaches?: string[] }) {
  const { t } = useTranslation();
  const tools = reaches ?? [];
  if (tools.length === 0) {
    return (
      <span className="text-2xs text-muted-foreground">
        {t("agents.reachesNothing")}
      </span>
    );
  }

  return tools.map((tool) => (
    <Badge key={tool} variant="outline" className="max-w-full gap-1">
      <Wrench aria-hidden />
      <Mono className="truncate text-2xs">{tool}</Mono>
    </Badge>
  ));
}
