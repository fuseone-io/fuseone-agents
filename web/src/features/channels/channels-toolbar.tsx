import { Plus, Search } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import type { ChannelView } from "@/features/channels/channel-model";

export function ChannelsToolbar({
  query,
  view,
  onQuery,
  onView,
  onAdd,
}: {
  query: string;
  view: ChannelView;
  onQuery: (value: string) => void;
  onView: (value: ChannelView) => void;
  onAdd: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-wrap items-start gap-3">
      <div className="min-w-0 flex-1">
        <h2 className="text-sm font-medium">{t("channels.channels")}</h2>
        <p className="mt-0.5 text-xs text-muted-foreground">
          {t("channels.workspaceExplains")}
        </p>
      </div>
      <div className="relative min-w-48 flex-1 sm:max-w-64">
        <Search
          className="pointer-events-none absolute left-2.5 top-2.5 size-3.5 text-muted-foreground"
          aria-hidden
        />
        <Input
          value={query}
          onChange={(event) => onQuery(event.target.value)}
          placeholder={t("channels.search")}
          className="h-8 pl-8 text-sm"
        />
      </div>
      <Tabs value={view} onValueChange={(next) => onView(next as ChannelView)}>
        <TabsList className="h-8" aria-label={t("channels.viewFilter")}>
          <TabsTrigger value="all">{t("channels.views.all")}</TabsTrigger>
          <TabsTrigger value="attention">
            {t("channels.views.attention")}
          </TabsTrigger>
          <TabsTrigger value="approvals">
            {t("channels.views.approvals")}
          </TabsTrigger>
        </TabsList>
      </Tabs>
      <Button size="sm" onClick={onAdd}>
        <Plus className="size-4" aria-hidden />
        {t("channels.newChannel")}
      </Button>
    </div>
  );
}
