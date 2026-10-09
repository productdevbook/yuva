import { createCn } from "cn/config"

export const cn = createCn({
  extend: { classGroups: { "font-size": [{ text: ["caption", "small", "body", "reading", "title", "page"] }] } },
})
