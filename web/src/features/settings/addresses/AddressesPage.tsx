import { Trans, useLingui } from "@lingui/react/macro"
import { Trash2Icon } from "lucide-react"
import { useState } from "react"

import { ErrorLine, toast, useConfirm } from "@/components/common"
import { AddressState } from "@/components/common/AddressState"
import { useErrorText } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { useAddressActions, useMyAddresses } from "@/features/settings/addresses/queries"
import { Card, EmptyRow, PageHeader, Row, Rows, Section } from "@/features/settings/ui"
import { ApiError, type MemberAddress } from "@/lib/api"

function useAddressError() {
  const { t } = useLingui()
  const errorText = useErrorText()
  return (e: unknown) => {
    if (e instanceof ApiError) {
      if (e.code === "limit_reached") return t`You can keep at most 10 addresses. Remove one first.`
      if (e.code === "address_taken") return t`This address is already used for mail in this workspace.`
      if (e.code === "validation_failed" && e.detail) return e.detail
    }
    return errorText(e)
  }
}

function CodeForm({ address }: { address: MemberAddress }) {
  const { t } = useLingui()
  const text = useAddressError()
  const { verify, add } = useAddressActions()
  const [code, setCode] = useState("")
  const busy = verify.isPending || add.isPending
  const error = verify.error ?? add.error
  return (
    <form
      className="flex w-full flex-col gap-2"
      onSubmit={(e) => {
        e.preventDefault()
        verify.mutate({ id: address.id, code: code.trim() }, { onSuccess: () => toast(t`Address verified`) })
      }}
    >
      <p className="text-caption text-faint">
        <Trans>We mailed a 6-digit code to this address. It is valid for 10 minutes.</Trans>
      </p>
      <div className="flex flex-wrap items-center gap-2">
        <Input
          value={code}
          inputMode="numeric"
          autoComplete="one-time-code"
          maxLength={6}
          aria-label={t`Code`}
          placeholder="123456"
          onChange={(e) => setCode(e.target.value.replace(/\D/g, ""))}
          className="h-8 w-32 font-mono tracking-widest"
          data-testid="address-code"
        />
        <Button type="submit" size="sm" disabled={busy || code.length !== 6} data-testid="address-verify">
          <Trans>Verify</Trans>
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          disabled={busy}
          onClick={() => {
            add.mutate(address.address, { onSuccess: () => toast(t`A new code is on its way`) })
          }}
          data-testid="address-resend"
        >
          <Trans>Send a new code</Trans>
        </Button>
      </div>
      {error && (
        <p role="alert" className="text-caption text-destructive">
          {text(error)}
        </p>
      )}
    </form>
  )
}

function AddressRow({ address: a }: { address: MemberAddress }) {
  const { remove } = useAddressActions()
  const [confirm, confirmDialog] = useConfirm()
  const { t } = useLingui()
  const address = a.address
  return (
    <Row className="flex-wrap" data-testid="address-row">
      <div className="min-w-0 flex-1">
        <p className="truncate">{address}</p>
        {a.verification === "approval" && !a.verified && (
          <p className="text-caption text-faint">
            <Trans>
              An owner or admin must approve this address, because mail to its domain lands in a shared inbox.
            </Trans>
          </p>
        )}
      </div>
      <AddressState address={a} />
      <Button
        variant="ghost"
        size="icon-sm"
        className="text-faint hover:text-destructive"
        aria-label={t`Remove ${address}`}
        onClick={() =>
          confirm({
            title: <Trans>Remove {address}?</Trans>,
            description: <Trans>Mail queued from it fails instead of going out from another address.</Trans>,
            confirm: <Trans>Remove</Trans>,
            run: () => remove.mutate(a.id),
          })
        }
        data-testid="address-remove"
      >
        <Trash2Icon />
      </Button>
      {!a.verified && a.verification === "code" && (
        <div className="basis-full">
          <CodeForm address={a} />
        </div>
      )}
      <ErrorLine error={remove.error} className="basis-full text-caption" />
      {confirmDialog}
    </Row>
  )
}

export function AddressesPage() {
  const { t } = useLingui()
  const text = useAddressError()
  const list = useMyAddresses()
  const { add } = useAddressActions()
  const [address, setAddress] = useState("")
  const items = list.data ?? []
  return (
    <>
      <PageHeader
        title={<Trans>Your addresses</Trans>}
        description={
          <Trans>
            Send mail from your own address instead of the inbox's. Verify it with a code, and the inbox's domain must
            be set up to send as it.
          </Trans>
        }
      />
      {list.isPending && <Skeleton className="h-32 rounded-xl" />}
      <ErrorLine error={list.error} />
      <Section>
        <div className="flex max-w-[760px] flex-col gap-4">
          <Card flush>
            {items.length === 0 && !list.isPending ? (
              <EmptyRow>
                <Trans>You have not added an address yet.</Trans>
              </EmptyRow>
            ) : (
              <Rows>
                {items.map((a) => (
                  <AddressRow key={a.id} address={a} />
                ))}
              </Rows>
            )}
          </Card>
          <form
            className="flex flex-col gap-2"
            onSubmit={(e) => {
              e.preventDefault()
              add.mutate(address.trim(), {
                onSuccess: (saved) => {
                  setAddress("")
                  toast(saved.verification === "approval" ? t`Waiting for approval` : t`Code sent`)
                },
              })
            }}
          >
            <label htmlFor="address-new" className="text-small font-medium">
              <Trans>Add an address</Trans>
            </label>
            <div className="flex flex-wrap items-center gap-2">
              <Input
                id="address-new"
                type="email"
                value={address}
                maxLength={320}
                placeholder={t`you@example.com`}
                onChange={(e) => setAddress(e.target.value)}
                className="h-8 max-w-80 flex-1"
                data-testid="address-new"
              />
              <Button
                type="submit"
                size="sm"
                disabled={add.isPending || address.trim().length < 3}
                data-testid="address-add"
              >
                <Trans>Add</Trans>
              </Button>
            </div>
            {add.error && (
              <p role="alert" className="text-caption text-destructive" data-testid="address-add-error">
                {text(add.error)}
              </p>
            )}
          </form>
        </div>
      </Section>
    </>
  )
}
