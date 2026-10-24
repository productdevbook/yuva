import { Trans, useLingui } from "@lingui/react/macro"
import { CheckIcon, ImageIcon, ImageUpIcon, LoaderIcon, Trash2Icon, TriangleAlertIcon } from "lucide-react"
import { useRef, useState } from "react"
import { Link } from "react-router"

import { Notice } from "@/components/common"
import { RowCheck, SelectToggle } from "@/components/common/Selection"
import { formatBytes, useErrorText } from "@/components/common/text"
import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Skeleton } from "@/components/ui/skeleton"
import { ImageBulkBar } from "@/features/designs/Bulk"
import { designLink, useDeleteImage, useImages, useUploadImage } from "@/features/designs/queries"
import { useSelection } from "@/hooks/use-selection"
import { ApiError, type EmailImage, type EmailImageProblem } from "@/lib/api"
import { cn } from "@/lib/utils"

const ACCEPT = "image/png,image/jpeg,image/gif,image/webp"

function useImageErrorText() {
  const { t } = useLingui()
  const base = useErrorText()
  return (err: unknown) => {
    if (err instanceof ApiError) {
      if (err.code === "image_type")
        return t`This file type is not allowed. Use PNG, JPEG, GIF or WebP; SVG files can carry scripts, so they are refused.`
      if (err.code === "image_too_large")
        return t`This image is too large. Use one under 40 megapixels and 8,000 pixels on a side, and at most 4,000 pixels tall once it is 1,200 wide.`
      if (err.code === "gif_too_wide") return t`Animated GIFs can be at most 1,200 pixels wide.`
      if (err.code === "image_quota")
        return t`The workspace’s images take all the room this server allows. Delete images you no longer use.`
      if (err.status === 413) return t`Images can be at most 10 MB.`
    }
    return base(err)
  }
}

type Deleting = { image: EmailImage; inUse?: { id: string; name: string }[]; sent?: boolean }

export function ImageLibrary({
  open,
  onOpenChange,
  current,
  onPick,
  finalFocus,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  current?: string
  onPick: (image: EmailImage) => void
  finalFocus?: () => HTMLElement | boolean
}) {
  const { t, i18n } = useLingui()
  const images = useImages(open)
  const upload = useUploadImage()
  const remove = useDeleteImage()
  const errorText = useImageErrorText()
  const input = useRef<HTMLInputElement>(null)
  const [error, setError] = useState<string | null>(null)
  const [over, setOver] = useState(false)
  const [deleting, setDeleting] = useState<Deleting | null>(null)
  const items = images.data?.pages.flatMap((p) => p.items) ?? []
  const selection = useSelection(
    items.map((i) => i.id),
    open
  )

  const send = async (files: File[]) => {
    setError(null)
    let last: EmailImage | null = null
    for (const f of files) {
      try {
        last = await upload.mutateAsync(f)
      } catch (err) {
        setError(`${f.name}: ${errorText(err)}`)
        return
      }
    }
    if (last && files.length === 1) onPick(last)
  }

  const tryDelete = (d: Deleting, force = false) => {
    remove.mutate(
      { id: d.image.id, force },
      {
        onSuccess: () => setDeleting(null),
        onError: (err) => {
          if (err instanceof ApiError && err.code === "image_in_use") {
            const body = err.body as EmailImageProblem | undefined
            setDeleting({ ...d, inUse: body?.used_by?.designs ?? [] })
          } else if (err instanceof ApiError && err.code === "image_sent") setDeleting({ ...d, sent: true })
          else {
            setDeleting(null)
            setError(errorText(err))
          }
        },
      }
    )
  }

  return (
    <>
      <Dialog open={open} onOpenChange={onOpenChange}>
        <DialogContent
          className="flex max-h-[88svh] flex-col gap-4 sm:max-w-3xl"
          finalFocus={finalFocus}
          data-testid="image-library"
          onDragOver={(e) => {
            if (e.dataTransfer.types.includes("Files")) {
              e.preventDefault()
              setOver(true)
            }
          }}
          onDragLeave={(e) => {
            if (e.currentTarget === e.target) setOver(false)
          }}
          onDrop={(e) => {
            e.preventDefault()
            setOver(false)
            void send(Array.from(e.dataTransfer.files))
          }}
        >
          <DialogHeader>
            <DialogTitle>
              <Trans>Images</Trans>
            </DialogTitle>
            <DialogDescription>
              <Trans>
                PNG, JPEG, GIF or WebP up to 10 MB. Uploads are made at most 1,200 pixels wide and lose their location
                and camera details. SVG files are not allowed.
              </Trans>
            </DialogDescription>
          </DialogHeader>
          <input
            ref={input}
            type="file"
            accept={ACCEPT}
            multiple
            className="hidden"
            onChange={(e) => {
              void send(Array.from(e.target.files ?? []))
              e.target.value = ""
            }}
            data-testid="image-upload-input"
          />
          <button
            type="button"
            onClick={() => input.current?.click()}
            disabled={upload.isPending}
            className={cn(
              "flex items-center justify-center gap-3 rounded-2xl border-2 border-dashed px-4 py-5 text-body text-muted-foreground transition-colors hover:border-faint hover:text-foreground",
              over && "border-brand bg-brand-wash text-brand"
            )}
            data-testid="image-upload"
          >
            {upload.isPending ? <LoaderIcon className="size-5 animate-spin" /> : <ImageUpIcon className="size-5" />}
            {upload.isPending ? <Trans>Uploading…</Trans> : <Trans>Upload images, or drop them here</Trans>}
          </button>
          <ImageBulkBar items={items} selection={selection} />
          {!selection.selecting && items.length > 0 && (
            <div className="flex justify-end">
              <SelectToggle selection={selection} />
            </div>
          )}
          {error && (
            <Notice tone="danger" icon={TriangleAlertIcon} role="alert" data-testid="image-error">
              <p>{error}</p>
            </Notice>
          )}
          <div className="-mx-1 min-h-0 flex-1 overflow-y-auto px-1 pb-1">
            {images.isPending ? (
              <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
                {Array.from({ length: 8 }, (_, i) => (
                  <Skeleton key={i} className="aspect-square rounded-xl" />
                ))}
              </div>
            ) : items.length === 0 ? (
              <div className="flex flex-col items-center gap-2 py-10 text-center text-body text-faint">
                <ImageIcon className="size-6" />
                <Trans>No images yet. Upload the first one.</Trans>
              </div>
            ) : (
              <ul className="grid grid-cols-2 gap-3 sm:grid-cols-4" data-testid="image-grid">
                {items.map((img) => {
                  const name = img.filename
                  return (
                    <li key={img.id} className="group relative">
                      <button
                        type="button"
                        onClick={(e) => (selection.selecting ? selection.toggle(img.id, e.shiftKey) : onPick(img))}
                        className={cn(
                          "flex w-full flex-col overflow-hidden rounded-xl border bg-card text-start outline-none transition-colors hover:border-faint focus-visible:ring-2 focus-visible:ring-ring",
                          current === img.id && "border-brand ring-2 ring-brand/30"
                        )}
                        aria-pressed={selection.selecting ? selection.has(img.id) : current === img.id}
                        data-testid="image-tile"
                      >
                        <span className="grid aspect-square place-items-center bg-[repeating-conic-gradient(var(--muted)_0_25%,transparent_0_50%)] bg-[length:16px_16px]">
                          <img src={img.url} alt="" loading="lazy" className="max-h-full max-w-full object-contain" />
                        </span>
                        <span className="flex flex-col gap-0.5 border-t px-2.5 py-2">
                          <span className="truncate text-small font-medium">{img.filename}</span>
                          <span className="text-caption text-faint tabular-nums">
                            {img.width}×{img.height} · {formatBytes(img.size_bytes, i18n.locale)}
                          </span>
                        </span>
                      </button>
                      {selection.selecting && (
                        <RowCheck
                          className="absolute top-2 start-2"
                          checked={selection.has(img.id)}
                          name={name}
                          onToggle={(range) => selection.toggle(img.id, range)}
                        />
                      )}
                      {current === img.id && !selection.selecting && (
                        <span className="absolute top-2 start-2 grid size-6 place-items-center rounded-full bg-brand text-white">
                          <CheckIcon className="size-3.5" />
                        </span>
                      )}
                      {!selection.selecting && (
                        <Button
                          variant="secondary"
                          size="icon-xs"
                          aria-label={t`Delete ${name}`}
                          className="absolute top-2 end-2 opacity-0 shadow-sm group-hover:opacity-100 focus-visible:opacity-100"
                          onClick={() => setDeleting({ image: img })}
                          data-testid="image-delete"
                        >
                          <Trash2Icon />
                        </Button>
                      )}
                    </li>
                  )
                })}
              </ul>
            )}
            {images.hasNextPage && (
              <div className="mt-3 flex justify-center">
                <Button
                  variant="ghost"
                  size="sm"
                  disabled={images.isFetchingNextPage}
                  onClick={() => void images.fetchNextPage()}
                >
                  <Trans>Load more</Trans>
                </Button>
              </div>
            )}
          </div>
        </DialogContent>
      </Dialog>
      <AlertDialog open={!!deleting} onOpenChange={(o) => !o && setDeleting(null)}>
        <AlertDialogContent data-testid="image-delete-dialog">
          <AlertDialogHeader>
            <AlertDialogTitle>
              {deleting?.inUse ? <Trans>This image is in use</Trans> : <Trans>Delete this image?</Trans>}
            </AlertDialogTitle>
            <AlertDialogDescription render={<div />}>
              {deleting?.inUse ? (
                <div className="flex flex-col gap-2">
                  <p>
                    <Trans>Take it out of these designs first, or archive them:</Trans>
                  </p>
                  <ul className="flex flex-col gap-1">
                    {deleting.inUse.map((d) => (
                      <li key={d.id}>
                        <Link to={designLink(d.id)} className="text-brand underline-offset-4 hover:underline">
                          {d.name}
                        </Link>
                      </li>
                    ))}
                  </ul>
                </div>
              ) : deleting?.sent ? (
                <p>
                  <Trans>
                    This image went out in a broadcast. If you delete it, people who open that mail see a broken image.
                  </Trans>
                </p>
              ) : (
                <p>
                  <Trans>Its address stops working at once.</Trans>
                </p>
              )}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{deleting?.inUse ? <Trans>Close</Trans> : <Trans>Cancel</Trans>}</AlertDialogCancel>
            {!deleting?.inUse && (
              <Button
                variant="destructive"
                disabled={remove.isPending}
                onClick={() => deleting && tryDelete(deleting, !!deleting.sent)}
                data-testid="image-delete-confirm"
              >
                {deleting?.sent ? <Trans>Delete anyway</Trans> : <Trans>Delete</Trans>}
              </Button>
            )}
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}
