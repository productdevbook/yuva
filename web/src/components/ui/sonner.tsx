import { Toaster as Sonner, type ToasterProps } from "sonner"

import { useTheme } from "@/lib/theme"

function Toaster(props: ToasterProps) {
  const theme = useTheme()
  return (
    <Sonner
      theme={theme}
      position="bottom-center"
      className="toaster group"
      toastOptions={{
        unstyled: true,
        classNames: {
          toast:
            "flex w-auto max-w-[calc(100vw-2rem)] items-center gap-3 rounded-full bg-foreground py-2 ps-4 pe-3 text-small text-background shadow-[0_12px_32px_-12px_rgb(0_0_0/0.35)] mx-auto",
          title: "min-w-0 truncate",
          actionButton: "shrink-0 underline underline-offset-3 bg-transparent! text-inherit! p-0!",
        },
      }}
      {...props}
    />
  )
}

export { Toaster }
