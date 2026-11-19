import { useLingui } from "@lingui/react/macro"

import { InfoColumn, NavList, PageLayout } from "@/components/common/PageLayout"

export function SectionPage({
  title,
  nav,
  aside,
  testId,
  children,
}: {
  title: string
  nav?: React.ReactNode
  aside?: React.ReactNode
  testId?: string
  children: React.ReactNode
}) {
  const { t } = useLingui()
  return (
    <PageLayout
      testId={testId}
      label={title}
      nav={nav && <NavList label={title}>{nav}</NavList>}
      aside={<InfoColumn label={t`Details`}>{aside}</InfoColumn>}
    >
      {children}
    </PageLayout>
  )
}
