import { Field, FieldContent, FieldDescription, FieldLabel, FieldTitle } from "@/components/ui/field"
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group"
import { cn } from "@/lib/utils"

export type RadioChoice<T extends string> = {
  value: T
  title: React.ReactNode
  hint?: React.ReactNode
  icon?: React.ReactNode
  disabled?: boolean
  testId?: string
}

export function RadioChoices<T extends string>({
  id,
  value,
  onChange,
  items,
  label,
  columns,
  className,
}: {
  id: string
  value: T
  onChange: (v: T) => void
  items: RadioChoice<T>[]
  label: string
  columns?: boolean
  className?: string
}) {
  return (
    <RadioGroup
      value={value}
      onValueChange={(v) => v !== value && onChange(v as T)}
      aria-label={label}
      className={cn("gap-2", columns && "sm:grid-cols-2", className)}
    >
      {items.map((it) => (
        <FieldLabel key={it.value} htmlFor={`${id}-${it.value}`} className="w-full">
          <Field orientation="horizontal" data-disabled={it.disabled || undefined}>
            <RadioGroupItem value={it.value} id={`${id}-${it.value}`} disabled={it.disabled} data-testid={it.testId} />
            {it.icon && <span className="shrink-0 text-muted-foreground [&_svg]:size-4">{it.icon}</span>}
            <FieldContent>
              <FieldTitle>{it.title}</FieldTitle>
              {it.hint && <FieldDescription className="text-caption">{it.hint}</FieldDescription>}
            </FieldContent>
          </Field>
        </FieldLabel>
      ))}
    </RadioGroup>
  )
}
