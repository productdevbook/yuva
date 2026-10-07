import type { MessageDescriptor, Messages } from "@lingui/core";
import { messages as en } from "./locales/en/messages";
import { messages as tr } from "./locales/tr/messages";

const catalogs: Record<string, Messages> = { en, tr };
const rtlLanguages = new Set(["ar", "fa", "he", "ur"]);

export const sourceLocale = "en";

export function resolveLocale(...candidates: (string | null | undefined)[]): string {
  for (const candidate of candidates) {
    if (!candidate) continue;
    const tag = candidate.toLowerCase();
    if (catalogs[tag]) return tag;
    const language = tag.split(/[-_]/)[0] ?? "";
    if (catalogs[language]) return language;
  }
  return sourceLocale;
}

export function directionOf(locale: string): "ltr" | "rtl" {
  return rtlLanguages.has(locale.split(/[-_]/)[0] ?? "") ? "rtl" : "ltr";
}

export function catalogFor(locale: string): Messages {
  return catalogs[locale] ?? en;
}

// Keeps @lingui/core out of the loader; only plain messages (no placeholders or plurals) belong here.
export function translatePlain(messages: Messages, descriptor: MessageDescriptor): string {
  const entry = messages[descriptor.id] ?? en[descriptor.id];
  if (typeof entry === "string") return entry;
  if (Array.isArray(entry) && entry.every((part) => typeof part === "string")) return entry.join("");
  return descriptor.message ?? descriptor.id;
}
