import { IconInfoCircle } from "@tabler/icons-react"

import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip"

/**
 * A small info icon that shows a tooltip on hover.
 * A TooltipProvider is mounted at the app root (app-layout.tsx) so no
 * additional provider is needed here.
 */
export function InfoHint({ text }: { text: string }) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span className="inline-flex cursor-help items-center">
          <IconInfoCircle className="size-3.5 text-muted-foreground/60 hover:text-muted-foreground transition-colors" />
        </span>
      </TooltipTrigger>
      <TooltipContent className="max-w-64 text-xs leading-relaxed">
        {text}
      </TooltipContent>
    </Tooltip>
  )
}

/**
 * A muted label followed by an info-hint icon.
 * Used for metric labels where a formula explanation is helpful.
 */
export function LabelWithHint({
  label,
  hint,
  className = "",
}: {
  label: string
  hint: string
  className?: string
}) {
  return (
    <span className={`inline-flex items-center gap-1 ${className}`}>
      {label}
      <InfoHint text={hint} />
    </span>
  )
}
