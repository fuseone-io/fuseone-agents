import { useEffect } from "react";
import { ChevronsUpDown } from "lucide-react";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { SidebarMenuButton, useSidebar } from "@/components/ui/sidebar";
import { LogoLockup, LogoMark } from "@/components/shared/logo";
import { useActiveScope } from "@/features/scope/active-scope";
import { useScopes, type RegisteredScope } from "@/features/scope/api";
import { ScopeChoice } from "@/features/scope/scope-choice";
import { ScopeGroup } from "@/features/scope/scope-group";
import { useMe } from "@/features/session/api";

const EVERYTHING = "Tudo o que eu alcanço";

/**
 * Which company and area the console is reading.
 *
 * It opens on everything the caller reaches rather than on a context chosen
 * for them: a screen narrowed by a decision nobody made looks like an outage
 * rather than like a filter. The choice is remembered across reloads, and
 * dropped the moment the grant behind it is.
 */
export function ScopeSwitcher() {
  const { data: me } = useMe();
  const { data } = useScopes();
  const { company, area, choose, reconcile } = useActiveScope();
  const { isMobile } = useSidebar();

  // Grants change when somebody's access does, and a stored scope that
  // outlives its grant filters every screen to a refusal.
  useEffect(() => {
    if (me) reconcile(me.grants);
  }, [me, reconcile]);

  const companies = [...new Set(me?.grants.map((g) => g.company) ?? [])].sort();
  const areas = data?.items ?? [];

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <SidebarMenuButton size="lg" className="h-[46px] gap-[9px]">
          <span className="flex size-8 shrink-0 items-center justify-center">
            <LogoMark size={26} />
          </span>
          <div className="grid flex-1 text-left leading-tight group-data-[collapsible=icon]:hidden">
            <LogoLockup className="text-base text-sidebar-accent-foreground" />
            <span className="truncate text-xs text-muted-foreground">
              {currentLabel({ company, area }, areas)}
            </span>
          </div>
          <ChevronsUpDown className="ml-auto size-4 shrink-0 opacity-60 group-data-[collapsible=icon]:hidden" />
        </SidebarMenuButton>
      </DropdownMenuTrigger>

      <DropdownMenuContent align="start" side={isMobile ? "bottom" : "right"} className="w-[248px]">
        <DropdownMenuLabel className="text-2xs uppercase tracking-label text-muted-foreground">
          Contexto
        </DropdownMenuLabel>
        <ScopeChoice
          label={EVERYTHING}
          chosen={company === ""}
          onChoose={() => choose({ company: "", area: "" })}
        />
        {companies.map((c) => (
          <ScopeGroup
            key={c}
            company={c}
            areas={areas.filter((a) => a.company === c)}
            active={{ company, area }}
            onChoose={choose}
          />
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function currentLabel(
  { company, area }: { company: string; area: string },
  areas: RegisteredScope[],
): string {
  if (company === "") return EVERYTHING;
  if (area === "") return company;
  return areas.find((a) => a.company === company && a.area === area)?.label || area;
}
