import type { ReactNode } from "react"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"

type Tab = { id: string; label: string; title: string }

export default function Snippets({ tabs, ...panels }: { tabs: Tab[] } & Record<string, ReactNode>) {
  return (
    <Tabs defaultValue={tabs[0]!.id} className="min-w-0">
      <div className="-mx-1 overflow-x-auto px-1 pb-1">
        <TabsList>
          {tabs.map((t) => (
            <TabsTrigger key={t.id} value={t.id} className="px-3">
              {t.label}
            </TabsTrigger>
          ))}
        </TabsList>
      </div>
      {tabs.map((t) => (
        <TabsContent key={t.id} value={t.id} keepMounted className="overflow-hidden rounded-xl border bg-card">
          <p className="border-b px-4 py-3 text-sm font-medium text-muted-foreground">{t.title}</p>
          {panels[t.id]}
        </TabsContent>
      ))}
    </Tabs>
  )
}
