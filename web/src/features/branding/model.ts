import type { CSSProperties } from "react";
import type { Branding } from "@/features/branding/api";

export const defaultBranding: Branding = { displayName: "FuseOne Agents" };

export function normaliseBranding(branding: Branding | undefined): Branding {
  return {
    displayName: branding?.displayName?.trim() || defaultBranding.displayName,
    logoUrl: clean(branding?.logoUrl),
    iconUrl: clean(branding?.iconUrl),
    primaryColor: clean(branding?.primaryColor),
  };
}

export function brandCSSVariables(branding: Branding): CSSProperties {
  const hex = branding.primaryColor;
  if (!hex || !/^#[0-9A-Fa-f]{6}$/.test(hex)) return {};

  const rgb = parseHex(hex);
  const foreground = contrastText(rgb);
  return {
    "--fuse-50": mixHex(rgb, white, 0.9),
    "--fuse-100": mixHex(rgb, white, 0.78),
    "--fuse-200": mixHex(rgb, white, 0.58),
    "--fuse-300": mixHex(rgb, white, 0.38),
    "--fuse-400": mixHex(rgb, white, 0.16),
    "--fuse-500": formatHex(rgb),
    "--fuse-600": mixHex(rgb, black, 0.18),
    "--fuse-700": mixHex(rgb, black, 0.32),
    "--fuse-800": mixHex(rgb, black, 0.48),
    "--fuse-900": mixHex(rgb, black, 0.62),
    "--primary-foreground": foreground,
    "--sidebar-primary-foreground": foreground,
    "--text-on-accent": foreground,
    "--surface-accent": rgbAlpha(rgb, 0.12),
  } as CSSProperties;
}

function clean(value: string | undefined): string | undefined {
  const trimmed = value?.trim();
  return trimmed || undefined;
}

type RGB = { r: number; g: number; b: number };

const white: RGB = { r: 255, g: 255, b: 255 };
const black: RGB = { r: 0, g: 0, b: 0 };

function parseHex(hex: string): RGB {
  const raw = hex.slice(1);
  return {
    r: Number.parseInt(raw.slice(0, 2), 16),
    g: Number.parseInt(raw.slice(2, 4), 16),
    b: Number.parseInt(raw.slice(4, 6), 16),
  };
}

function mixHex(from: RGB, to: RGB, amount: number): string {
  return formatHex({
    r: mix(from.r, to.r, amount),
    g: mix(from.g, to.g, amount),
    b: mix(from.b, to.b, amount),
  });
}

function mix(a: number, b: number, amount: number): number {
  return Math.round(a + (b - a) * amount);
}

function formatHex({ r, g, b }: RGB): string {
  return `#${part(r)}${part(g)}${part(b)}`;
}

function part(value: number): string {
  return value.toString(16).padStart(2, "0");
}

function rgbAlpha({ r, g, b }: RGB, alpha: number): string {
  return `rgb(${r} ${g} ${b} / ${alpha})`;
}

function contrastText({ r, g, b }: RGB): string {
  const luminance = (0.2126 * r + 0.7152 * g + 0.0722 * b) / 255;
  return luminance > 0.62 ? "#0b1416" : "#ffffff";
}
