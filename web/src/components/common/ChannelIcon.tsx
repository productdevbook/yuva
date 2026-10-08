import { CodeXmlIcon, MailIcon, MessageCircleIcon, SmartphoneIcon } from "lucide-react"

import type { ChannelKind } from "@/lib/api"

const icons: Record<ChannelKind, React.ComponentType<{ className?: string }>> = {
  email: MailIcon,
  chat: MessageCircleIcon,
  app: SmartphoneIcon,
  api: CodeXmlIcon,
}

export function ChannelIcon({ kind, className }: { kind?: ChannelKind; className?: string }) {
  const Icon = kind ? icons[kind] : MessageCircleIcon
  return <Icon className={className} />
}
