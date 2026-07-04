// Pure-CSS loading indicators. Every variant embeds an sr-only <output>
// ("Loading") — an implicit live status region — so assistive tech announces
// the activity while the visual marks stay presentational; the
// motion honors prefers-reduced-motion via the blanket rule in globals.css.
// Keyframes (`lyra-loader-*`) live in styles/globals.css; `pulse-dot` reuses
// the shared `animate-pulse-dot` and `text-shimmer` the shared `animate-shimmer`.
//
// Skeleton (skeleton.tsx) already covers block/placeholder loading, so the
// spinner-style variants from the source library are dropped — these are the
// inline "agent is working" marks that sit next to text.

import { cn } from "@/lib/utils";

export type LoaderVariant =
  "dots" | "typing" | "pulse-dot" | "wave" | "bars" | "terminal" | "text-shimmer";

export type LoaderSize = "sm" | "md" | "lg";

export interface LoaderProps {
  variant?: LoaderVariant;
  size?: LoaderSize;
  /** Label rendered by the `text-shimmer` variant; ignored by the others. */
  text?: string;
  className?: string;
}

const CONTAINER: Record<LoaderSize, string> = {
  sm: "h-4",
  md: "h-5",
  lg: "h-6",
};

const TEXT: Record<LoaderSize, string> = {
  sm: "text-xs",
  md: "text-sm",
  lg: "text-base",
};

// The live status region: `<output>` carries an implicit role=status +
// aria-live=polite (the semantic AT hook), so the visual container elements
// stay presentational — no `role` attribute needed on them.
function Loading() {
  return <output className="sr-only">Loading</output>;
}

function DotsLoader({ size = "md", className }: { size?: LoaderSize; className?: string }) {
  const dot = { sm: "h-1.5 w-1.5", md: "h-2 w-2", lg: "h-2.5 w-2.5" }[size];
  return (
    <div className={cn("flex items-center gap-1", CONTAINER[size], className)}>
      {[0, 1, 2].map((i) => (
        <span
          key={i}
          className={cn(
            "rounded-full bg-fg-muted animate-[lyra-loader-bounce_1.4s_ease-in-out_infinite]",
            dot,
          )}
          style={{ animationDelay: `${i * 160}ms` }}
        />
      ))}
      <Loading />
    </div>
  );
}

function TypingLoader({ size = "md", className }: { size?: LoaderSize; className?: string }) {
  const dot = { sm: "h-1 w-1", md: "h-1.5 w-1.5", lg: "h-2 w-2" }[size];
  return (
    <div className={cn("flex items-center gap-1", CONTAINER[size], className)}>
      {[0, 1, 2].map((i) => (
        <span
          key={i}
          className={cn(
            "rounded-full bg-fg-muted animate-[lyra-loader-typing_1s_ease-in-out_infinite]",
            dot,
          )}
          style={{ animationDelay: `${i * 250}ms` }}
        />
      ))}
      <Loading />
    </div>
  );
}

function PulseDotLoader({ size = "md", className }: { size?: LoaderSize; className?: string }) {
  const dot = { sm: "h-1 w-1", md: "h-2 w-2", lg: "h-3 w-3" }[size];
  return (
    <div className={cn("rounded-full bg-fg-muted animate-pulse-dot", dot, className)}>
      <Loading />
    </div>
  );
}

function WaveLoader({ size = "md", className }: { size?: LoaderSize; className?: string }) {
  const bar = { sm: "w-0.5", md: "w-0.5", lg: "w-1" }[size];
  return (
    <div className={cn("flex items-center gap-0.5", CONTAINER[size], className)}>
      {[0, 1, 2, 3, 4].map((i) => (
        <span
          key={i}
          className={cn(
            "h-full origin-center rounded-full bg-fg-muted animate-[lyra-loader-wave_1s_ease-in-out_infinite]",
            bar,
          )}
          style={{ animationDelay: `${i * 100}ms` }}
        />
      ))}
      <Loading />
    </div>
  );
}

function BarsLoader({ size = "md", className }: { size?: LoaderSize; className?: string }) {
  const bar = { sm: "w-1 gap-1", md: "w-1.5 gap-1.5", lg: "w-2 gap-2" }[size];
  const [width, gap] = bar.split(" ");
  return (
    <div className={cn("flex items-center", gap, CONTAINER[size], className)}>
      {[0, 1, 2].map((i) => (
        <span
          key={i}
          className={cn(
            "h-full origin-center rounded-sm bg-fg-muted animate-[lyra-loader-bars_1.2s_ease-in-out_infinite]",
            width,
          )}
          style={{ animationDelay: `${i * 200}ms` }}
        />
      ))}
      <Loading />
    </div>
  );
}

function TerminalLoader({ size = "md", className }: { size?: LoaderSize; className?: string }) {
  const cursor = { sm: "h-3 w-1.5", md: "h-4 w-2", lg: "h-5 w-2.5" }[size];
  return (
    <div className={cn("flex items-center gap-1", CONTAINER[size], className)}>
      <span className={cn("font-mono text-fg-muted", TEXT[size])}>{">"}</span>
      <span
        className={cn("bg-fg-muted animate-[lyra-loader-blink_1s_step-end_infinite]", cursor)}
      />
      <Loading />
    </div>
  );
}

function TextShimmerLoader({
  text = "Thinking",
  size = "md",
  className,
}: {
  text?: string;
  size?: LoaderSize;
  className?: string;
}) {
  return (
    <span
      className={cn(
        "inline-block bg-clip-text font-medium text-transparent animate-shimmer motion-reduce:animate-none",
        "bg-[linear-gradient(90deg,var(--color-text-muted)_35%,var(--color-text)_50%,var(--color-text-muted)_65%)]",
        "bg-[length:200%_100%]",
        TEXT[size],
        className,
      )}
    >
      {text}
      <Loading />
    </span>
  );
}

export function Loader({ variant = "dots", size = "md", text, className }: LoaderProps) {
  switch (variant) {
    case "typing":
      return <TypingLoader size={size} className={className} />;
    case "pulse-dot":
      return <PulseDotLoader size={size} className={className} />;
    case "wave":
      return <WaveLoader size={size} className={className} />;
    case "bars":
      return <BarsLoader size={size} className={className} />;
    case "terminal":
      return <TerminalLoader size={size} className={className} />;
    case "text-shimmer":
      return <TextShimmerLoader text={text} size={size} className={className} />;
    default:
      return <DotsLoader size={size} className={className} />;
  }
}
