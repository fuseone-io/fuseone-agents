import type { Branding } from "@/features/branding/api";
import { builtInPrimary, normaliseBranding } from "@/features/branding/model";

export interface BrandingForm {
  displayName: string;
  logoUrl: string;
  iconUrl: string;
  primaryColor: string;
}

const hexColour = /^#[0-9A-Fa-f]{6}$/;

export function formFrom(branding: Branding | undefined): BrandingForm {
  const normal = normaliseBranding(branding);
  return {
    displayName: normal.displayName,
    logoUrl: normal.logoUrl ?? "",
    iconUrl: normal.iconUrl ?? "",
    primaryColor: normal.primaryColor ?? "",
  };
}

export function bodyFrom(form: BrandingForm): Branding {
  return {
    displayName: form.displayName.trim(),
    logoUrl: clean(form.logoUrl),
    iconUrl: clean(form.iconUrl),
    primaryColor: clean(form.primaryColor),
  };
}

function clean(value: string): string | undefined {
  const trimmed = value.trim();
  return trimmed || undefined;
}

/** Empty is allowed: it means the built-in palette. The server re-checks. */
export function colourOK(value: string): boolean {
  const trimmed = value.trim();
  return trimmed === "" || hexColour.test(trimmed);
}

/**
 * What the picker shows for whatever is in the hex field. A native colour
 * input only holds "#rrggbb", so a half-typed or empty value shows the
 * built-in colour rather than a black the operator never chose.
 */
export function pickerValue(hex: string): string {
  const trimmed = hex.trim();
  return hexColour.test(trimmed) ? trimmed.toLowerCase() : builtInPrimary;
}
