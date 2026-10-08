import yuvaMark from "@/assets/yuva-mark.svg"
import { cn } from "@/lib/utils"

export function YuvaMark({ className }: { className?: string }) {
  return <img src={yuvaMark} alt="" className={cn("rounded-[22%]", className)} />
}
