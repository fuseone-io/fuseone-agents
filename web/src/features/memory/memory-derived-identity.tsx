import { useTranslation } from "react-i18next";
import { Mono } from "@/components/shared/mono";
import { Badge } from "@/components/ui/badge";
import { derivedMemoryIdentity } from "@/features/memory/memory-authoring-identity";

export function MemoryDerivedIdentity({ subject }: { subject: string }) {
  const { t } = useTranslation();
  const identity = derivedMemoryIdentity(subject);

  return (
    <section
      aria-label={t("memory.derivedIdentity")}
      className="rounded-md border bg-muted/40 px-3 py-2"
    >
      <p className="text-2xs uppercase tracking-label text-muted-foreground">
        {t("memory.derivedIdentity")}
      </p>
      <div className="mt-1 flex min-w-0 items-center gap-2">
        <Badge variant="outline" className="font-mono text-2xs">
          {identity.kind}
        </Badge>
        <Mono className="truncate text-xs">
          {identity.signature || t("memory.derivedIdentityPending")}
        </Mono>
      </div>
      <p className="mt-1 text-2xs text-muted-foreground">
        {t("memory.derivedIdentityHint")}
      </p>
    </section>
  );
}
