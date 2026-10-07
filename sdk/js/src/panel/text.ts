const inline =
  /(`[^`\n]+`)|(\*\*[^*\n]+\*\*)|(\*[^*\s][^*\n]*\*|_[^_\s][^_\n]*_)|((?:https?:\/\/|www\.)[^\s<>"]*[^\s<>".,:;'!?)\]])/g;

function link(href: string, text: string): HTMLAnchorElement {
  const a = document.createElement("a");
  a.href = href;
  a.textContent = text;
  a.target = "_blank";
  a.rel = "noopener noreferrer nofollow ugc";
  return a;
}

function wrap(tag: string, children: Node[]): HTMLElement {
  const element = document.createElement(tag);
  element.append(...children);
  return element;
}

export function inlineNodes(text: string): Node[] {
  const nodes: Node[] = [];
  let last = 0;
  for (const match of text.matchAll(inline)) {
    const index = match.index ?? 0;
    if (index > last) nodes.push(document.createTextNode(text.slice(last, index)));
    const [whole, code, bold, italic, url] = match;
    if (code) nodes.push(wrap("code", [document.createTextNode(code.slice(1, -1))]));
    else if (bold) nodes.push(wrap("strong", inlineNodes(bold.slice(2, -2))));
    else if (italic) nodes.push(wrap("em", inlineNodes(italic.slice(1, -1))));
    else if (url) {
      const href = url.startsWith("www.") ? `https://${url}` : url;
      try {
        const parsed = new URL(href);
        nodes.push(parsed.protocol === "http:" || parsed.protocol === "https:" ? link(parsed.href, url) : document.createTextNode(url));
      } catch {
        nodes.push(document.createTextNode(url));
      }
    }
    last = index + whole.length;
  }
  if (last < text.length) nodes.push(document.createTextNode(text.slice(last)));
  return nodes;
}

export function richText(body: string): DocumentFragment {
  const fragment = document.createDocumentFragment();
  const parts = body.trim().replace(/\r\n?/g, "\n").split(/^```[^\n]*\n([\s\S]*?)^```[ \t]*$/m);
  parts.forEach((part, i) => {
    if (i % 2 === 1) {
      const pre = document.createElement("pre");
      pre.textContent = part.replace(/\n$/, "");
      fragment.append(pre);
      return;
    }
    const lines = (parts.length > 1 ? part.replace(/^\n|\n$/g, "") : part).split("\n");
    lines.forEach((line, n) => {
      if (n > 0) fragment.append(document.createElement("br"));
      fragment.append(...inlineNodes(line));
    });
  });
  return fragment;
}
