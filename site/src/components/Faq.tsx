import { Accordion, AccordionContent, AccordionItem, AccordionTrigger } from "@/components/ui/accordion"

export default function Faq({ items }: { items: { q: string; a: string }[] }) {
  return (
    <Accordion className="rounded-xl border bg-card px-4 sm:px-6">
      {items.map((f) => (
        <AccordionItem key={f.q} value={f.q}>
          <AccordionTrigger className="text-base">{f.q}</AccordionTrigger>
          <AccordionContent keepMounted className="text-muted-foreground">
            <p>{f.a}</p>
          </AccordionContent>
        </AccordionItem>
      ))}
    </Accordion>
  )
}
