import { useTranslation } from "react-i18next";
import { Badge } from "@/components/ui/badge";

export function ServiceAccountStatus({ disabled }: { disabled: boolean }) {
  const { t } = useTranslation();
  return (
    <Badge variant={disabled ? "destructive" : "secondary"}>
      {t(disabled ? "serviceAccounts.statusDisabled" : "serviceAccounts.statusActive")}
    </Badge>
  );
}
