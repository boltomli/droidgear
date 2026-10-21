'use client'

import type * as React from 'react'
import { GripVerticalIcon } from 'lucide-react'
import * as ResizablePrimitive from 'react-resizable-panels'

import { cn } from '@/lib/utils'

// react-resizable-panels v4 interprets numeric sizes as pixels; v3 treated
// them as percentages. Keep the v3 semantics by converting numbers to
// percentage strings at the wrapper boundary. Covers every size prop that
// v4 measures in `SizeUnit` (defaultSize, minSize, maxSize, collapsedSize,
// collapsedThreshold).
function toPercentSize(value: number | string | undefined) {
  return typeof value === 'number' ? `${value}%` : value
}

function ResizablePanelGroup({
  className,
  ...props
}: React.ComponentProps<typeof ResizablePrimitive.Group>) {
  // Note: since v4.5.5 the Group root no longer renders aria-orientation,
  // so orientation-specific selectors never match here. The library sets
  // flex-direction inline based on the orientation prop.
  return (
    <ResizablePrimitive.Group
      data-slot="resizable-panel-group"
      className={cn('flex h-full w-full', className)}
      {...props}
    />
  )
}

function ResizablePanel({
  defaultSize,
  minSize,
  maxSize,
  collapsedSize,
  collapsedThreshold,
  ...props
}: React.ComponentProps<typeof ResizablePrimitive.Panel>) {
  return (
    <ResizablePrimitive.Panel
      data-slot="resizable-panel"
      defaultSize={toPercentSize(defaultSize)}
      minSize={toPercentSize(minSize)}
      maxSize={toPercentSize(maxSize)}
      collapsedSize={toPercentSize(collapsedSize)}
      collapsedThreshold={toPercentSize(collapsedThreshold)}
      {...props}
    />
  )
}

function ResizableHandle({
  withHandle,
  className,
  ...props
}: React.ComponentProps<typeof ResizablePrimitive.Separator> & {
  withHandle?: boolean
}) {
  return (
    <ResizablePrimitive.Separator
      data-slot="resizable-handle"
      className={cn(
        'bg-border focus-visible:ring-ring relative flex w-px items-center justify-center after:absolute after:inset-y-0 after:left-1/2 after:w-1 after:-translate-x-1/2 focus-visible:ring-1 focus-visible:ring-offset-1 focus-visible:outline-hidden [aria-orientation=vertical]:h-px [aria-orientation=vertical]:w-full [aria-orientation=vertical]:after:left-0 [aria-orientation=vertical]:after:h-1 [aria-orientation=vertical]:after:w-full [aria-orientation=vertical]:after:translate-x-0 [aria-orientation=vertical]:after:-translate-y-1/2 [[aria-orientation=vertical]>div]:rotate-90',
        className
      )}
      {...props}
    >
      {withHandle && (
        <div className="bg-border z-10 flex h-4 w-3 items-center justify-center rounded-xs border">
          <GripVerticalIcon className="size-2.5" />
        </div>
      )}
    </ResizablePrimitive.Separator>
  )
}

export { ResizablePanelGroup, ResizablePanel, ResizableHandle }
