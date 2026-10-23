import type { ClientAttachmentLimits } from "./client/api"

export function allowsType(limits: ClientAttachmentLimits, type: string): boolean {
  const value = type.toLowerCase()
  return limits.types.some((entry) => entry === value || (entry.endsWith("/*") && value.startsWith(entry.slice(0, -1))))
}

export function fileProblem(
  limits: ClientAttachmentLimits,
  file: { size: number; type: string }
): "size" | "type" | null {
  if (!allowsType(limits, file.type)) return "type"
  return file.size > limits.max_bytes ? "size" : null
}

export const canAttach = (limits: ClientAttachmentLimits): boolean => limits.types.length > 0

export const acceptAttribute = (limits: ClientAttachmentLimits): string => limits.types.join(",")
