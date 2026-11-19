import { useLingui } from "@lingui/react/macro"

import { SectionPage } from "@/components/common"

export function HelpLayout({
  aside,
  testId,
  children,
}: {
  aside?: React.ReactNode
  testId?: string
  children: React.ReactNode
}) {
  const { t } = useLingui()
  return (
    <SectionPage title={t`Help center`} aside={aside} testId={testId}>
      {children}
    </SectionPage>
  )
}
