import { Outlet } from "react-router"

export function SettingsIndex() {
  return null
}

export function SettingsLayout() {
  return (
    <main className="mx-auto flex w-full max-w-[640px] flex-col gap-8 px-6 pt-6 pb-24 phone:px-4 phone:pt-4" data-testid="settings">
      <Outlet />
    </main>
  )
}
