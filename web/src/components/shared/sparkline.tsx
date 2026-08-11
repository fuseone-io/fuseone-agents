/**
 * A trend, small enough to sit inside a figure.
 *
 * No axes, no labels, no tooltip: it says whether a number has been climbing
 * or falling, and the number beside it says how much. A sparkline that tried
 * to be readable on its own would be a chart in the wrong place.
 */
export function Sparkline({
  points,
  className = "text-primary",
  width = 64,
  height = 20,
}: {
  points: number[];
  className?: string;
  width?: number;
  height?: number;
}) {
  // Two points make a line; one makes nothing, and drawing a flat stroke for
  // it would claim a trend that was never measured.
  if (points.length < 2) return null;

  const max = Math.max(...points);
  const min = Math.min(...points);
  const span = max - min || 1;

  const d = points
    .map((p, i) => {
      const x = (i / (points.length - 1)) * width;
      const y = height - ((p - min) / span) * (height - 3) - 1.5;
      return `${i ? "L" : "M"}${x.toFixed(1)} ${y.toFixed(1)}`;
    })
    .join(" ");

  return (
    <svg
      aria-hidden
      width={width}
      height={height}
      viewBox={`0 0 ${width} ${height}`}
      className={className}
    >
      <path
        d={d}
        fill="none"
        stroke="currentColor"
        strokeWidth="1.5"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}
