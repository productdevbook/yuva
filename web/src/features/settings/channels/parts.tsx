export function FieldError({ children, id }: { children?: React.ReactNode; id: string }) {
  if (!children) return null
  return (
    <p id={id} role="alert" className="text-xs text-destructive">
      {children}
    </p>
  )
}

export function FormBlock({
  title,
  action,
  children,
  ...props
}: { title: React.ReactNode; action?: React.ReactNode; children: React.ReactNode } & Omit<React.ComponentProps<"section">, "title">) {
  return (
    <section className="flex min-w-0 flex-col gap-5 border-t pt-5" {...props}>
      <div className="flex items-center justify-between gap-3">
        <h3 className="text-sm font-semibold">{title}</h3>
        {action}
      </div>
      {children}
    </section>
  )
}
