import { forwardRef, type InputHTMLAttributes } from 'react'

type InputProps = InputHTMLAttributes<HTMLInputElement>

export const Input = forwardRef<HTMLInputElement, InputProps>(function Input(
  { className, type = 'text', ...rest },
  ref,
) {
  const cls = ['input', className].filter(Boolean).join(' ')
  return <input ref={ref} type={type} className={cls} {...rest} />
})
