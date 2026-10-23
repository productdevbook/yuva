const svg = (paths: string, size = 18, width = 1.75, flip = false) =>
  `<svg${flip ? ' class="flip"' : ""} width="${size}" height="${size}" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="${width}" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${paths}</svg>`

export const icons = {
  close: svg('<path d="M6 6l12 12M18 6L6 18"/>'),
  closeSmall: svg('<path d="M6 6l12 12M18 6L6 18"/>', 12, 2.2),
  back: svg('<path d="M15 6l-6 6 6 6"/>', 18, 1.75, true),
  chevron: svg('<path d="M9 6l6 6-6 6"/>', 14, 2, true),
  arrow: svg('<path d="M5 12h14M13 6l6 6-6 6"/>', 16, 1.75, true),
  attach: svg(
    '<path d="M20 11.5l-7.8 7.8a5 5 0 0 1-7-7l8.4-8.4a3.3 3.3 0 0 1 4.7 4.7l-8.4 8.4a1.7 1.7 0 0 1-2.4-2.4l7.7-7.7"/>'
  ),
  shot: svg(
    '<path d="M4 8V5.5A1.5 1.5 0 0 1 5.5 4H8M16 4h2.5A1.5 1.5 0 0 1 20 5.5V8M20 16v2.5a1.5 1.5 0 0 1-1.5 1.5H16M8 20H5.5A1.5 1.5 0 0 1 4 18.5V16"/><circle cx="12" cy="12" r="3"/>'
  ),
  send: svg('<path d="M12 19V5M6 11l6-6 6 6"/>', 17, 2),
  file: svg(
    '<path d="M14 3H6.5A1.5 1.5 0 0 0 5 4.5v15A1.5 1.5 0 0 0 6.5 21h11a1.5 1.5 0 0 0 1.5-1.5V8z"/><path d="M14 3v5h5"/>',
    16
  ),
  down: svg('<path d="M12 5v14M6 13l6 6 6-6"/>', 14, 2),
  seen: svg('<path d="M2 12.5l4.5 4.5L15 8.5M10 16l1 1 9.5-9.5"/>', 14, 2),
  check: svg('<path d="M5 12.5l4.5 4.5L19 7.5"/>', 15, 2),
  person: svg('<circle cx="12" cy="8" r="4"/><path d="M4 21a8 8 0 0 1 16 0"/>', 15),
  mail: svg('<rect x="3" y="5" width="18" height="14" rx="2"/><path d="M3.5 6.5l8.5 6 8.5-6"/>'),
  alert: svg('<circle cx="12" cy="12" r="9"/><path d="M12 7.5v5.5M12 16.5v.01"/>', 18, 1.9),
  bubble: svg('<path d="M4 5h16v11H9l-5 4z"/><path d="M9 10.5h6"/>', 20),
  search: svg('<circle cx="11" cy="11" r="7"/><path d="M20 20l-3.5-3.5"/>', 16),
  book: svg(
    '<path d="M4 5.5A1.5 1.5 0 0 1 5.5 4H11v16H5.5A1.5 1.5 0 0 1 4 18.5z"/><path d="M20 5.5A1.5 1.5 0 0 0 18.5 4H13v16h5.5a1.5 1.5 0 0 0 1.5-1.5z"/>',
    20
  ),
  vote: svg('<path d="M12 19V5M6 11l6-6 6 6"/>', 20),
  plus: svg('<path d="M12 5v14M5 12h14"/>', 14, 2),
  sparkle: svg('<path d="M12 3l2.2 5.3L20 9l-4.4 3.8L17 18.5 12 15.5 7 18.5l1.4-5.7L4 9l5.8-.7z"/>', 20),
  bell: svg('<path d="M6 8a6 6 0 0 1 12 0c0 7 3 9 3 9H3s3-2 3-9"/><path d="M10.3 21a1.9 1.9 0 0 0 3.4 0"/>', 20),
  idea: svg(
    '<path d="M9.5 18h5M10.5 21h3"/><path d="M12 3a6 6 0 0 0-3.6 10.8c.6.5 1.1 1.3 1.1 2.1V16h5v-.1c0-.8.5-1.6 1.1-2.1A6 6 0 0 0 12 3z"/>'
  ),
  planned: svg('<rect x="4" y="5" width="16" height="15" rx="2"/><path d="M4 10h16M9 3v4M15 3v4"/>'),
  progress: svg('<circle cx="12" cy="12" r="8"/><path d="M12 4a8 8 0 0 1 0 16z" fill="currentColor"/>'),
  shipped: svg('<circle cx="12" cy="12" r="8.5"/><path d="M8.5 12.5l2.5 2.5 4.5-5"/>'),
  tag: svg(
    '<path d="M12.6 2.6A2 2 0 0 0 11.2 2H4a2 2 0 0 0-2 2v7.2a2 2 0 0 0 .6 1.4l8.7 8.7a2.4 2.4 0 0 0 3.4 0l6.6-6.6a2.4 2.4 0 0 0 0-3.4z"/><circle cx="7.5" cy="7.5" r=".5"/>',
    16
  ),
  bug: svg(
    '<path d="M8 2l1.9 1.9M14.1 3.9L16 2M9 7.1v-1a3 3 0 1 1 6 0v1"/><path d="M12 20c-3.3 0-6-2.7-6-6v-3a4 4 0 0 1 4-4h4a4 4 0 0 1 4 4v3c0 3.3-2.7 6-6 6zM12 20v-9M6.5 9C4.6 8.8 3 7.1 3 5M6 13H2M3 21c0-2.1 1.7-3.9 3.8-4M21 5c0 2.1-1.6 3.8-3.5 4M22 13h-4M17.2 17c2.1.1 3.8 1.9 3.8 4"/>',
    16
  ),
  heart: svg(
    '<path d="M19 14c1.5-1.5 3-3.2 3-5.5A5.5 5.5 0 0 0 16.5 3c-1.8 0-3 .5-4.5 2-1.5-1.5-2.7-2-4.5-2A5.5 5.5 0 0 0 2 8.5c0 2.3 1.5 4 3 5.5l7 7z"/>',
    16
  ),
  messageCircle: svg('<path d="M7.9 20A9 9 0 1 0 4 16.1L2 22z"/>', 16),
  star: svg('<path d="M12 2l3.1 6.3 6.9 1-5 4.9 1.2 6.9-6.2-3.3-6.2 3.3L7 14.2 2 9.3l6.9-1z"/>', 16),
  zap: svg('<path d="M13 2L3 14h9l-1 8 10-12h-9z"/>', 16),
}

const categoryIcons: Record<string, string> = {
  bug: icons.bug,
  lightbulb: icons.idea,
  heart: icons.heart,
  "message-circle": icons.messageCircle,
  star: icons.star,
  zap: icons.zap,
}

export const categoryIcon = (name: string): string => categoryIcons[name] ?? icons.tag
