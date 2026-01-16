import type { ComponentProps } from 'react'
import { DropdownMenuItem } from '@/components/ui/dropdown-menu'

/**
 * DropdownMenuItem wrapper that prevents IME composition issues.
 * Use this for menu items that may be clicked while IME is active.
 */
export function ActionDropdownMenuItem({
  onClick,
  ...props
}: ComponentProps<typeof DropdownMenuItem>) {
  return (
    <DropdownMenuItem
      onMouseDown={e => {
        e.preventDefault()
        onClick?.(e as unknown as React.MouseEvent<HTMLDivElement>)
      }}
      {...props}
    />
  )
}
