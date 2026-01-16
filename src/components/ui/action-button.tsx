import type { ComponentProps } from 'react'
import { Button } from '@/components/ui/button'

/**
 * Button wrapper that prevents IME composition issues.
 * Use this for buttons that may be clicked while IME is active.
 */
export function ActionButton({
  onClick,
  ...props
}: ComponentProps<typeof Button>) {
  return (
    <Button
      onMouseDown={e => {
        e.preventDefault()
        onClick?.(e as unknown as React.MouseEvent<HTMLButtonElement>)
      }}
      {...props}
    />
  )
}
