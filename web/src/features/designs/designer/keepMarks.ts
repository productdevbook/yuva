import { Extension } from "@tiptap/core"
import type { Mark, Node, Slice } from "@tiptap/pm/model"
import { Plugin, PluginKey, type Transaction } from "@tiptap/pm/state"
import { Mapping, ReplaceStep } from "@tiptap/pm/transform"

export const keepMarksKey = new PluginKey<boolean>("keepMarks")

function sharedMarks(doc: Node, from: number, to: number): readonly Mark[] {
  let marks: readonly Mark[] | null = null
  doc.nodesBetween(from, to, (node) => {
    if (!node.isText) return true
    marks = marks ? marks.filter((m) => m.isInSet(node.marks)) : node.marks
    return false
  })
  return marks ?? []
}

function needsKeeping(marks: readonly Mark[]) {
  return marks.some((m) => m.type.spec.inclusive === false)
}

function continued(doc: Node, pos: number, slice: Slice): readonly Mark[] {
  if (slice.openStart || slice.openEnd || !slice.size) return []
  let inline = true
  const types = new Set<string>()
  slice.content.forEach((n) => {
    if (!n.isInline) inline = false
    n.marks.forEach((m) => types.add(m.type.name))
  })
  const before = doc.resolve(pos).nodeBefore
  if (!inline || !before?.isText) return []
  return before.marks.filter((m) => m.type.spec.inclusive === false && !types.has(m.type.name))
}

export function keepMarksPlugin() {
  return new Plugin<boolean>({
    key: keepMarksKey,
    state: {
      init: () => false,
      apply: (tr, on) => tr.getMeta(keepMarksKey) ?? on,
    },
    appendTransaction(trs, _old, state) {
      if (!keepMarksKey.getState(state)) return null
      const fixes: { from: number; to: number; marks: readonly Mark[] }[] = []
      let stored: readonly Mark[] | null = null
      trs.forEach((tr: Transaction, i) => {
        if (tr.getMeta("composition") !== undefined) return
        tr.steps.forEach((step, j) => {
          if (!(step instanceof ReplaceStep)) return
          const insert = step.from === step.to
          const marks = insert
            ? continued(tr.docs[j], step.from, step.slice)
            : sharedMarks(tr.docs[j], step.from, step.to)
          if (!needsKeeping(marks)) return
          if (step.slice.size === 0) {
            stored = marks
            return
          }
          const rest = new Mapping(tr.mapping.maps.slice(j + 1))
          for (const later of trs.slice(i + 1)) rest.appendMapping(later.mapping)
          const from = rest.map(step.from, 1)
          const to = rest.map(step.from + step.slice.size, -1)
          if (to > from) fixes.push({ from, to, marks })
          stored = null
        })
      })
      if (!fixes.length && !stored) return null
      const tr = state.tr
      for (const f of fixes) for (const m of f.marks) tr.addMark(f.from, f.to, m)
      if (stored && !fixes.length) tr.setStoredMarks(stored)
      return tr
    },
  })
}

export const KeepMarks = Extension.create({
  name: "keepMarks",
  addProseMirrorPlugins() {
    return [keepMarksPlugin()]
  },
})
