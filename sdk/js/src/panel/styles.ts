export const styles = `
[hidden] { display: none !important; }
.header {
  display: flex;
  align-items: center;
  gap: 10px;
  padding-block: 12px;
  padding-inline: 14px 10px;
  background: var(--yuva-accent);
  color: var(--yuva-on-accent);
}
.heading { flex: 1; min-inline-size: 0; }
.title {
  margin: 0;
  font-size: 16px;
  font-weight: 600;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.status { display: flex; align-items: center; gap: 6px; font-size: 13px; opacity: 0.9; }
.dot { inline-size: 8px; block-size: 8px; border-radius: 50%; background: #22c55e; box-shadow: 0 0 0 2px rgb(255 255 255 / 0.6); }
.dot.away { background: #9ca3af; }
.avatars { display: flex; }
.avatars .avatar { margin-inline-start: -8px; box-shadow: 0 0 0 2px var(--yuva-accent); }
.avatars .avatar:first-child { margin-inline-start: 0; }
.avatar {
  display: grid;
  place-items: center;
  flex: none;
  inline-size: 28px;
  block-size: 28px;
  border-radius: 50%;
  background: var(--yuva-soft);
  color: var(--yuva-fg);
  font-size: 11px;
  font-weight: 700;
  text-transform: uppercase;
}
.icon {
  display: grid;
  place-items: center;
  flex: none;
  inline-size: 32px;
  block-size: 32px;
  padding: 0;
  border: 0;
  border-radius: 8px;
  background: transparent;
  color: inherit;
  cursor: pointer;
}
.icon:hover { background: rgb(127 127 127 / 0.18); }
.icon:focus-visible { outline: 2px solid currentColor; outline-offset: -4px; }
[dir="rtl"] .flip { transform: scaleX(-1); }
.body { position: relative; flex: 1; min-block-size: 0; display: flex; flex-direction: column; }
.scroll { flex: 1; overflow-y: auto; overscroll-behavior: contain; padding: 14px; display: flex; flex-direction: column; gap: 4px; }
.center { flex: 1; display: grid; place-content: center; gap: 6px; padding: 24px; text-align: center; }
.empty { margin: 0; font-weight: 600; }
.hint { margin: 0; color: var(--yuva-muted); font-size: 14px; }
.list { list-style: none; margin: 0; padding: 6px; overflow-y: auto; flex: 1; }
.item {
  display: flex;
  gap: 10px;
  align-items: center;
  inline-size: 100%;
  padding: 10px;
  border: 0;
  border-radius: 10px;
  background: transparent;
  color: inherit;
  font: inherit;
  text-align: start;
  cursor: pointer;
}
.item:hover, .item:focus-visible { background: var(--yuva-soft); outline: none; }
.item-main { flex: 1; min-inline-size: 0; }
.item-top { display: flex; gap: 8px; justify-content: space-between; }
.item-subject, .item-preview { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.item-subject { font-weight: 600; }
.item-preview { color: var(--yuva-muted); font-size: 14px; }
.item.unread .item-preview { color: var(--yuva-fg); font-weight: 600; }
.item-time { flex: none; color: var(--yuva-muted); font-size: 12px; }
.unread-dot { flex: none; inline-size: 9px; block-size: 9px; border-radius: 50%; background: var(--yuva-accent); }
.primary {
  margin: 10px;
  padding: 10px 14px;
  border: 0;
  border-radius: 10px;
  background: var(--yuva-accent);
  color: var(--yuva-on-accent);
  font: inherit;
  font-weight: 600;
  cursor: pointer;
}
.row { display: flex; gap: 8px; align-items: flex-end; max-inline-size: 85%; }
.row.mine { align-self: flex-end; }
.row.theirs .avatar { margin-block-end: 2px; }
.avatar img { inline-size: 100%; block-size: 100%; border-radius: 50%; object-fit: cover; }
.group-gap { margin-block-start: 8px; }
.author { margin: 6px 0 2px; margin-inline-start: 36px; color: var(--yuva-muted); font-size: 12px; }
.bubble {
  padding: 8px 12px;
  border-radius: 16px;
  background: var(--yuva-soft);
  overflow-wrap: anywhere;
  white-space: normal;
}
.mine .bubble { background: var(--yuva-accent); color: var(--yuva-on-accent); border-end-end-radius: 4px; }
.theirs .bubble { border-end-start-radius: 4px; }
.bubble a { color: inherit; text-decoration: underline; }
.bubble code { font-family: ui-monospace, Menlo, monospace; font-size: 13px; padding: 1px 4px; border-radius: 4px; background: rgb(127 127 127 / 0.2); }
.bubble pre { margin: 4px 0; padding: 8px; border-radius: 8px; background: rgb(127 127 127 / 0.2); font-family: ui-monospace, Menlo, monospace; font-size: 13px; white-space: pre-wrap; }
.greeting { align-self: flex-start; max-inline-size: 85%; }
.files { display: flex; flex-direction: column; gap: 4px; margin-block-start: 4px; }
.files img { display: block; max-inline-size: 100%; max-block-size: 220px; border-radius: 10px; cursor: zoom-in; }
.file {
  display: inline-flex;
  gap: 6px;
  align-items: center;
  padding: 6px 10px;
  border: 1px solid currentColor;
  border-radius: 8px;
  background: transparent;
  color: inherit;
  font: inherit;
  font-size: 13px;
  cursor: pointer;
}
.meta { align-self: flex-end; color: var(--yuva-muted); font-size: 12px; margin-block-start: 2px; }
.meta.failed { color: var(--yuva-danger); }
.link { padding: 0; border: 0; background: none; color: inherit; font: inherit; text-decoration: underline; cursor: pointer; }
.typing { display: flex; gap: 6px; align-items: center; color: var(--yuva-muted); font-size: 13px; margin-block-start: 6px; }
.typing i { display: inline-block; inline-size: 6px; block-size: 6px; border-radius: 50%; background: currentColor; animation: blink 1.2s infinite both; }
.typing i:nth-child(2) { animation-delay: 0.2s; }
.typing i:nth-child(3) { animation-delay: 0.4s; }
@keyframes blink { 0%, 80%, 100% { opacity: 0.25; } 40% { opacity: 1; } }
@media (prefers-reduced-motion: reduce) { .typing i { animation: none; } }
.card { margin-block-start: 10px; padding: 12px; border: 1px solid var(--yuva-border); border-radius: 12px; font-size: 14px; }
.card p { margin: 0 0 8px; }
.card form { display: flex; gap: 6px; }
.card input {
  flex: 1;
  min-inline-size: 0;
  padding: 8px 10px;
  border: 1px solid var(--yuva-border);
  border-radius: 8px;
  background: var(--yuva-bg);
  color: inherit;
  font: inherit;
}
.card button { padding: 8px 12px; border: 0; border-radius: 8px; background: var(--yuva-accent); color: var(--yuva-on-accent); font: inherit; font-weight: 600; cursor: pointer; }
.card .error { margin: 6px 0 0; color: var(--yuva-danger); }
.card button:disabled { opacity: 0.6; cursor: default; }
.rate { text-align: center; }
.choices { display: flex; justify-content: center; gap: 10px; }
.card .choice { inline-size: 44px; block-size: 44px; padding: 0; border: 1px solid var(--yuva-border); border-radius: 50%; background: var(--yuva-bg); font-size: 20px; line-height: 1; }
.card .choice:hover:not(:disabled), .card .choice[aria-pressed="true"] { border-color: var(--yuva-accent); box-shadow: 0 0 0 1px var(--yuva-accent); }
.card .choice:disabled { opacity: 1; }
.card .choice:focus-visible { outline: 2px solid var(--yuva-accent); outline-offset: 2px; }
.rate .choices + p { margin: 8px 0 0; }
.rate form { flex-direction: column; margin-block-start: 10px; }
.rate textarea { box-sizing: border-box; inline-size: 100%; padding: 8px 10px; border: 1px solid var(--yuva-border); border-radius: 8px; background: var(--yuva-bg); color: inherit; font: inherit; resize: vertical; }
.rate .actions { display: flex; justify-content: flex-end; align-items: center; gap: 14px; }
.card .link { padding: 0; background: none; color: var(--yuva-muted); font-weight: 400; }
.composer { border-block-start: 1px solid var(--yuva-border); padding: 8px; }
.composer form { display: flex; gap: 4px; align-items: flex-end; }
.composer textarea {
  flex: 1;
  min-inline-size: 0;
  box-sizing: border-box;
  block-size: 38px;
  max-block-size: 140px;
  padding: 8px 10px;
  border: 0;
  border-radius: 10px;
  background: transparent;
  color: inherit;
  font: inherit;
  resize: none;
  outline: none;
}
.composer .icon { color: var(--yuva-muted); }
.composer .send { color: var(--yuva-accent); }
.composer .send:disabled { opacity: 0.4; cursor: default; }
.chips { display: flex; flex-wrap: wrap; gap: 6px; padding: 0 6px 6px; }
.chip { display: inline-flex; gap: 4px; align-items: center; max-inline-size: 100%; padding: 2px 4px 2px 10px; border-radius: 999px; background: var(--yuva-soft); font-size: 13px; }
.chip span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; max-inline-size: 180px; }
.chip .icon { inline-size: 22px; block-size: 22px; }
.banner { padding: 6px 12px; background: var(--yuva-soft); color: var(--yuva-muted); font-size: 13px; text-align: center; }
@media (max-width: 480px) { .composer textarea, .card input, .card textarea { font-size: 16px; } }
.sr { position: absolute; inline-size: 1px; block-size: 1px; overflow: hidden; clip-path: inset(50%); white-space: nowrap; }
`;
