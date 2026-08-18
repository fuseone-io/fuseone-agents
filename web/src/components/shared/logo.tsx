import { cn } from "@/lib/utils";

/**
 * The Fuseone Agents mark: an agent with its status light.
 *
 * Inlined rather than loaded as an image for two reasons the design system
 * makes unavoidable. Line weight is a function of size, not a constant — a
 * 48-grid mark scaled to 20px would take its stroke down with it and dissolve —
 * and the mono variant paints in `currentColor`, which an <img> cannot inherit.
 */
export function LogoMark({
  size = 24,
  mono,
  className,
  ariaLabel = "FuseOne Agents",
}: {
  size?: number;
  /** Paints in currentColor, for a surface that supplies its own colour. */
  mono?: boolean;
  className?: string;
  ariaLabel?: string;
}) {
  const stroke = strokeFor(size, mono);

  // Semantic, never palette: these invert between themes rather than shifting
  // one step, so a mark painted from the ramp directly is the same dark teal
  // on white and on near-black — present in one and gone in the other.
  const line = mono ? "currentColor" : "var(--logo-line)";
  const shell = mono ? "none" : "var(--logo-shell)";
  const ear = mono ? "none" : "var(--logo-ear)";
  // The eyes are the line: they are holes in the head, not a third colour.
  const eye = mono ? "currentColor" : "var(--logo-line)";

  return (
    <svg
      viewBox="0 0 48 48"
      width={size}
      height={size}
      fill="none"
      role="img"
      aria-label={ariaLabel}
      className={cn("shrink-0", className)}
    >
      <g transform="translate(0 5)">
        <rect
          x="8"
          y="13"
          width="4.5"
          height="10"
          rx="2.25"
          fill={ear}
          stroke={line}
          strokeWidth={stroke}
        />
        <rect
          x="35.5"
          y="13"
          width="4.5"
          height="10"
          rx="2.25"
          fill={ear}
          stroke={line}
          strokeWidth={stroke}
        />
        <rect
          x="11.5"
          y="8"
          width="25"
          height="24"
          rx="10"
          fill={shell}
          stroke={line}
          strokeWidth={stroke}
        />
        {/* The light is detached because a status here is something reported,
            never something built in. */}
        <path
          d="M24 8V5.6"
          stroke={line}
          strokeWidth={stroke}
          strokeLinecap="round"
        />
        <circle
          cx="24"
          cy="3.6"
          r="2.6"
          fill={mono ? "none" : "var(--logo-light)"}
          stroke={line}
          strokeWidth={stroke}
        />
        <rect x="17.8" y="16.6" width="3.6" height="6.4" rx="1.8" fill={eye} />
        <rect x="26.6" y="16.6" width="3.6" height="6.4" rx="1.8" fill={eye} />
      </g>
    </svg>
  );
}

/**
 * strokeFor implements the design system's rule that weight rises as the mark
 * shrinks. The shipped SVGs hardcode 2.4, which is correct at 32–64px and
 * disappears at the 20px floor the same document sets.
 */
function strokeFor(size: number, mono?: boolean): number {
  const base = size <= 20 ? 3.6 : size <= 28 ? 3.2 : size <= 64 ? 2.4 : 2;
  return mono ? base + 0.4 : base;
}

/**
 * The name, set with the weight break that is the type's whole idea.
 *
 * Never one weight: `Fuse` light, `One` semibold, `Agents` in aqua. Written as
 * live text rather than an image so it renders in Geist, which the console
 * already loads, and so it inherits the surface's colour.
 */
export function LogoLockup({ className }: { className?: string }) {
  return (
    /* The product's name, not a phrase: it reads the same in both languages,
       and translating it would rename the product for half its users. */
    /* eslint-disable i18next/no-literal-string */
    <span className={cn("whitespace-nowrap tracking-[-0.035em]", className)}>
      <span className="font-light">Fuse</span>
      <span className="font-semibold">One</span>
      <span className="ml-[0.34em] font-normal text-text-accent">Agents</span>
    </span>
    /* eslint-enable i18next/no-literal-string */
  );
}

/** Mark and name together, for a surface with room for the full lockup. */
export function LogoWordmark({ className }: { className?: string }) {
  return (
    <span className={cn("flex items-center gap-2", className)}>
      <LogoMark size={28} />
      <LogoLockup className="text-xl" />
    </span>
  );
}
