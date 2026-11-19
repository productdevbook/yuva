import { useLingui } from "@lingui/react/macro"

import { SectionPage } from "@/components/common"

export function ChangelogLayout({
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
    <SectionPage title={t`Updates`} aside={aside} testId={testId}>
      {children}
    </SectionPage>
  )
}
