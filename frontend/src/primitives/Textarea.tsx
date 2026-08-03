import { forwardRef, type TextareaHTMLAttributes } from 'react'

type TextareaProps = TextareaHTMLAttributes<HTMLTextAreaElement>

export const Textarea = forwardRef<HTMLTextAreaElement, TextareaProps>(function Textarea(
  { className, rows = 6, ...rest },
  ref,
) {
  const cls = ['textarea', className].filter(Boolean).join(' ')
  return <textarea ref={ref} rows={rows} className={cls} {...rest} />
})
