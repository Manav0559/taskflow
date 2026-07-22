import type { ButtonHTMLAttributes } from 'react'
import clsx from 'clsx'

type Variant = 'primary' | 'secondary' | 'ghost' | 'danger'

interface Props extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: Variant
}

const VARIANT_CLASSES: Record<Variant, string> = {
  primary: 'bg-series-1 text-white hover:opacity-90 disabled:opacity-50',
  secondary:
    'border border-border bg-card text-ink hover:bg-surface disabled:opacity-50',
  ghost: 'text-ink-secondary hover:bg-surface disabled:opacity-50',
  danger: 'bg-critical text-white hover:opacity-90 disabled:opacity-50',
}

export function Button({ variant = 'secondary', className, ...props }: Props) {
  return (
    <button
      className={clsx(
        'inline-flex items-center justify-center gap-1.5 rounded-lg px-3 py-1.5 text-sm font-medium transition-colors disabled:cursor-not-allowed',
        VARIANT_CLASSES[variant],
        className,
      )}
      {...props}
    />
  )
}
