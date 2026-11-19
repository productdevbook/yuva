import { useParams } from "react-router"

import { ContactsPage } from "@/features/contact/ContactsPage"

export function ContactPage() {
  const { contactId = "" } = useParams()
  return <ContactsPage key="contacts" contactId={contactId} />
}
