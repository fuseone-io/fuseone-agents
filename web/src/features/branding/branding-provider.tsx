import {
  createContext,
  useContext,
  useEffect,
  useMemo,
  type ReactNode,
} from "react";
import { LogoLockup, LogoMark, LogoWordmark } from "@/components/shared/logo";
import { cn } from "@/lib/utils";
import { useBranding, type Branding } from "@/features/branding/api";
import {
  brandCSSVariables,
  defaultBranding,
  normaliseBranding,
} from "@/features/branding/model";

const BrandingContext = createContext<Branding>(defaultBranding);

export function BrandingProvider({ children }: { children: ReactNode }) {
  const { data } = useBranding();
  const branding = useMemo(() => normaliseBranding(data), [data]);

  useEffect(() => applyBranding(branding), [branding]);

  return (
    <BrandingContext.Provider value={branding}>
      {children}
    </BrandingContext.Provider>
  );
}

function useInstallationBranding() {
  return useContext(BrandingContext);
}

export function BrandLogoMark({
  size = 24,
  className,
  mono,
}: {
  size?: number;
  className?: string;
  mono?: boolean;
}) {
  const branding = useInstallationBranding();
  if (branding.iconUrl) {
    return (
      <img
        src={branding.iconUrl}
        alt={branding.displayName}
        width={size}
        height={size}
        className={cn("shrink-0 object-contain", className)}
      />
    );
  }
  return (
    <LogoMark
      size={size}
      mono={mono}
      ariaLabel={branding.displayName}
      className={className}
    />
  );
}

export function BrandLogoLockup({ className }: { className?: string }) {
  const branding = useInstallationBranding();
  if (branding.logoUrl) {
    return (
      <img
        src={branding.logoUrl}
        alt={branding.displayName}
        className={cn("h-5 max-w-full object-contain object-left", className)}
      />
    );
  }
  if (branding.displayName === defaultBranding.displayName) {
    return <LogoLockup className={className} />;
  }
  return (
    <span className={cn("truncate font-medium tracking-normal", className)}>
      {branding.displayName}
    </span>
  );
}

export function BrandLogoWordmark({ className }: { className?: string }) {
  const branding = useInstallationBranding();
  if (branding.logoUrl) {
    return (
      <img
        src={branding.logoUrl}
        alt={branding.displayName}
        className={cn("h-8 max-w-[240px] object-contain object-left", className)}
      />
    );
  }
  if (branding.displayName === defaultBranding.displayName) {
    return <LogoWordmark className={className} />;
  }
  return (
    <span className={cn("flex items-center gap-2", className)}>
      <BrandLogoMark size={28} />
      <span className="truncate text-xl font-medium tracking-normal">
        {branding.displayName}
      </span>
    </span>
  );
}

function applyBranding(branding: Branding) {
  const root = document.documentElement;
  const vars = brandCSSVariables(branding);
  for (const [name, value] of Object.entries(vars)) {
    root.style.setProperty(name, String(value));
  }
  document.title = branding.displayName;

  return () => {
    for (const name of Object.keys(vars)) {
      root.style.removeProperty(name);
    }
    document.title = defaultBranding.displayName;
  };
}
