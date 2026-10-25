import assert from "node:assert/strict"
import { test } from "node:test"

import { getSchema } from "@tiptap/core"
import { Slice, type Node } from "@tiptap/pm/model"
import { EditorState, TextSelection } from "@tiptap/pm/state"
import { StarterKit } from "@tiptap/starter-kit"

import { keepMarksKey, keepMarksPlugin } from "../src/features/designs/designer/keepMarks.ts"

const schema = getSchema([StarterKit.configure({ link: { autolink: false, openOnClick: false } })])
const href = "https://example.com/news/1"

function start(on = true) {
  const { bold, link } = schema.marks
  const doc = schema.node("doc", null, [
    schema.node("paragraph", null, [schema.text("Read more →", [bold.create(), link.create({ href })])]),
  ])
  const state = EditorState.create({ schema, doc, plugins: [keepMarksPlugin()] })
  return state.apply(state.tr.setMeta(keepMarksKey, on))
}

function select(state: EditorState, from: number, to: number) {
  return state.apply(state.tr.setSelection(TextSelection.create(state.doc, from, to)))
}

function run(state: EditorState, tr: ReturnType<EditorState["tr"]["insertText"]>) {
  return state.applyTransaction(tr).state
}

function texts(doc: Node) {
  const out: { text: string; href?: string; bold: boolean }[] = []
  doc.descendants((n) => {
    if (n.isText)
      out.push({
        text: n.text ?? "",
        href: n.marks.find((m) => m.type.name === "link")?.attrs.href,
        bold: n.marks.some((m) => m.type.name === "bold"),
      })
  })
  return out
}

const end = (s: EditorState) => s.doc.firstChild!.content.size + 1

test("typing over the whole linked text keeps the link and every character", () => {
  let s = start()
  s = select(s, 1, end(s))
  s = run(s, s.tr.insertText("Ler mais →"))
  assert.deepEqual(texts(s.doc), [{ text: "Ler mais →", href, bold: true }])
})

test("pasting over the whole linked text keeps the link and the pasted text", () => {
  let s = start()
  s = select(s, 1, end(s))
  const slice = new Slice(schema.node("paragraph", null, [schema.text("Ler mais →")]).content, 0, 0)
  s = run(s, s.tr.replaceSelection(slice))
  assert.deepEqual(texts(s.doc), [{ text: "Ler mais →", href, bold: true }])
})

test("typing over part of the link keeps one link over the whole text", () => {
  let s = start()
  s = select(s, 1, 1 + "Read more ".length)
  s = run(s, s.tr.insertText("Ler mais "))
  assert.deepEqual(texts(s.doc), [{ text: "Ler mais →", href, bold: true }])
})

test("deleting the linked text and then typing keeps the link", () => {
  let s = start()
  s = select(s, 1, end(s))
  s = run(s, s.tr.deleteSelection())
  s = run(s, s.tr.insertText("Ler mais →"))
  assert.deepEqual(texts(s.doc), [{ text: "Ler mais →", href, bold: true }])
})

test("an editor without it drops the link as before", () => {
  let s = start(false)
  s = select(s, 1, end(s))
  s = run(s, s.tr.insertText("Ler mais →"))
  assert.equal(texts(s.doc)[0].text, "Ler mais →")
  assert.equal(texts(s.doc)[0].href, undefined)
})
