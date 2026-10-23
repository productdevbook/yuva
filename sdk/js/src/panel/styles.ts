const cross = (side: string) =>
  `.sec::${side}{content:"";position:absolute;top:-7px;inset-inline-${side === "before" ? "start" : "end"}:14px;inline-size:13px;block-size:13px;z-index:3;pointer-events:none;background:linear-gradient(var(--yuva-strong),var(--yuva-strong)) center/1px 100% no-repeat,linear-gradient(var(--yuva-strong),var(--yuva-strong)) center/100% 1px no-repeat}`

export const styles = `
[hidden] { display: none !important; }
*, *::before, *::after { box-sizing: border-box; }
.panel::before, .panel::after {
  content: "";
  position: absolute;
  inset-block: 0;
  inline-size: 1px;
  background: var(--yuva-border);
  pointer-events: none;
  z-index: 1;
}
.panel::before { inset-inline-start: 20px; }
.panel::after { inset-inline-end: 20px; }
${cross("before")}
${cross("after")}
button, input, textarea { font: inherit; color: inherit; }
button:focus-visible, a:focus-visible, input:focus-visible, textarea:focus-visible, select:focus-visible { outline: 2px solid var(--yuva-brand); outline-offset: 2px; }
.flip { display: block; }
[dir="rtl"] .flip { transform: scaleX(-1); }
.mono { font-family: var(--yuva-mono); }
.label { margin: 0; font-family: var(--yuva-mono); font-size: 11px; letter-spacing: 0.08em; text-transform: uppercase; color: var(--yuva-faint); }
.sec { position: relative; flex: none; padding: 18px 32px 20px; border-block-start: 1px solid var(--yuva-border); display: flex; flex-direction: column; gap: 12px; }
.header {
  position: relative;
  z-index: 2;
  flex: none;
  display: flex;
  align-items: center;
  gap: 6px;
  min-block-size: 64px;
  padding-block: 0;
  padding-inline: 20px 32px;
  background: var(--yuva-bg);
}
.header.home { z-index: auto; min-block-size: 56px; padding-inline: 32px 20px; background: transparent; }
.headbrand { display: flex; align-items: center; gap: 10px; flex: 1; min-inline-size: 0; }
.mark { display: grid; place-items: center; flex: none; inline-size: 26px; block-size: 26px; border-radius: 7px; background: var(--yuva-fg); color: var(--yuva-bg); font-size: 13px; font-weight: 600; overflow: hidden; }
.mark img { inline-size: 100%; block-size: 100%; object-fit: cover; }
.heading { flex: 1; min-inline-size: 0; display: flex; flex-direction: column; margin-inline-start: 6px; }
.title { margin: 0; font-size: 15px; font-weight: 600; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.status { display: flex; align-items: center; gap: 6px; font-size: 12px; color: var(--yuva-muted); }
.who { position: relative; flex: none; }
.who .dot { position: absolute; inset-inline-end: -1px; inset-block-end: -1px; inline-size: 10px; block-size: 10px; border: 2px solid var(--yuva-bg); border-radius: 50%; background: var(--yuva-ok); }
.who .dot.away { background: var(--yuva-faint); }
.avatar {
  display: grid;
  place-items: center;
  flex: none;
  inline-size: 28px;
  block-size: 28px;
  border-radius: 50%;
  background: var(--yuva-wash);
  color: var(--yuva-brand);
  font-size: 11px;
  font-weight: 600;
  text-transform: uppercase;
  overflow: hidden;
}
.avatar.lg { inline-size: 32px; block-size: 32px; font-size: 12px; }
.avatar img { inline-size: 100%; block-size: 100%; object-fit: cover; }
.avatars { display: flex; }
.avatars .avatar { inline-size: 30px; block-size: 30px; margin-inline-start: -8px; border: 2px solid var(--yuva-bg); }
.avatars .avatar:first-child { margin-inline-start: 0; }
.avatars .avatar:nth-child(2) { background: var(--yuva-deep); color: var(--yuva-muted); }
.avatars .avatar:nth-child(3) { background: var(--yuva-fg); color: var(--yuva-bg); }
.icon {
  display: grid;
  place-items: center;
  flex: none;
  inline-size: 44px;
  block-size: 44px;
  padding: 0;
  border: 0;
  border-radius: 10px;
  background: transparent;
  color: var(--yuva-muted);
  cursor: pointer;
}
.icon:hover { color: var(--yuva-fg); }
.icon.sm { inline-size: 24px; block-size: 24px; border-radius: 6px; }
.icon.tool { inline-size: 36px; block-size: 36px; border-radius: 8px; }
.icon.tool:hover { background: var(--yuva-deep); }
.body { position: relative; flex: 1; min-block-size: 0; display: flex; flex-direction: column; border-block-start: 1px solid var(--yuva-border); }
.header.home + .body, .header.home + .banner + .body { border-block-start: 0; }
.scroll { flex: 1; min-block-size: 0; overflow-y: auto; overscroll-behavior: contain; padding: 20px 32px 16px; display: flex; flex-direction: column; gap: 14px; -webkit-mask-image: linear-gradient(to bottom, transparent 0, #000 22px); mask-image: linear-gradient(to bottom, transparent 0, #000 22px); }
.scroll > :first-child { margin-block-start: auto; }
.page { flex: 1; min-block-size: 0; overflow-y: auto; overscroll-behavior: contain; display: flex; flex-direction: column; }
.page > .fill { flex: 1; }
.center { flex: 1; display: grid; place-content: center; gap: 6px; padding: 24px 32px; text-align: center; }
.empty { margin: 0; font-weight: 600; }
.search { display: flex; align-items: center; gap: 10px; min-block-size: 44px; padding: 0 14px; border: 1px solid var(--yuva-strong); border-radius: 10px; background: var(--yuva-deep); color: var(--yuva-faint); }
.search:focus-within { border-color: var(--yuva-brand); }
.search svg { flex: none; }
.search input { flex: 1; min-inline-size: 0; border: 0; outline: 0; background: transparent; font: inherit; color: var(--yuva-fg); }
.article-card { display: flex; align-items: center; gap: 10px; max-inline-size: 88%; padding: 10px 12px; border: 1px solid var(--yuva-border); border-radius: 12px; background: var(--yuva-bg); color: var(--yuva-fg); text-decoration: none; }
.article-card:hover { background: var(--yuva-deep); }
.article-card > svg:first-child { flex: none; color: var(--yuva-brand); }
.article-card > svg:last-child { flex: none; color: var(--yuva-faint); }
.article-card-text { flex: 1; min-inline-size: 0; display: flex; flex-direction: column; gap: 2px; }
.article-card-text b { font-weight: 500; }
.article-card-text span { font-size: 12px; color: var(--yuva-muted); display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; }
.hint { margin: 0; color: var(--yuva-muted); font-size: 14px; }
.hint.resolved { text-align: center; }
.hero { padding-block: 28px 24px; gap: 14px; }
.hero h1 { margin: 0; font-size: 28px; line-height: 1.15; font-weight: 600; letter-spacing: -0.02em; }
.primary {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  min-block-size: 44px;
  padding: 0 16px;
  border: 0;
  border-radius: 10px;
  background: var(--yuva-accent);
  color: var(--yuva-on-accent);
  font-weight: 500;
  cursor: pointer;
}
.primary:disabled { opacity: 0.5; cursor: default; }
.center .primary { margin-block-start: 8px; }
.quiet { min-block-size: 44px; padding: 0 16px; border: 1px solid var(--yuva-strong); border-radius: 10px; background: transparent; cursor: pointer; }
.item {
  display: flex;
  gap: 12px;
  align-items: flex-start;
  inline-size: calc(100% + 24px);
  margin-inline: -12px;
  padding: 12px;
  border: 0;
  border-radius: 10px;
  background: transparent;
  text-align: start;
  cursor: pointer;
}
.item:hover { background: var(--yuva-deep); }
.item-main { flex: 1; min-inline-size: 0; display: flex; flex-direction: column; gap: 2px; }
.item-top { display: flex; gap: 8px; justify-content: space-between; }
.item-subject, .item-preview { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.item-subject { font-weight: 500; }
.item-preview { color: var(--yuva-muted); }
.item.unread .item-preview { color: var(--yuva-fg); font-weight: 600; }
.item-state { flex: none; color: var(--yuva-faint); font-size: 12px; }
.item-time { flex: none; font-family: var(--yuva-mono); font-size: 11px; color: var(--yuva-faint); }
.unread-dot { flex: none; inline-size: 8px; block-size: 8px; margin-block-start: 6px; border-radius: 50%; background: var(--yuva-brand); }
.list { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; }
.list .item { inline-size: 100%; margin-inline: 0; border-radius: 0; border-block-end: 1px solid var(--yuva-border); }
.link { padding: 0; border: 0; background: none; color: inherit; text-decoration: underline; cursor: pointer; }
.link.soft { color: var(--yuva-muted); text-decoration: none; min-block-size: 28px; }
.link.soft:hover { color: var(--yuva-fg); }
.link.brand { color: var(--yuva-brand); font-weight: 500; text-decoration: none; min-block-size: 28px; }
.tiles { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); border: 1px solid var(--yuva-border); border-radius: 12px; overflow: hidden; }
.tile { display: flex; flex-direction: column; gap: 8px; min-block-size: 44px; padding: 14px; border: 0; background: transparent; text-align: start; cursor: pointer; color: inherit; }
.tile:hover { background: var(--yuva-deep); }
.tile + .tile { border-inline-start: 1px solid var(--yuva-border); }
.tile:only-child { grid-column: 1 / -1; }
.tile svg { color: var(--yuva-brand); }
.tile .tile-top { display: flex; align-items: center; justify-content: space-between; font-size: inherit; color: inherit; }
.tile .tile-count { min-inline-size: 20px; padding: 1px 6px; border-radius: 10px; background: var(--yuva-brand); color: var(--yuva-on-accent); font-family: var(--yuva-mono); font-size: 11px; font-weight: 500; text-align: center; }
.tile b { font-weight: 500; }
.tile span { font-size: 12px; color: var(--yuva-muted); }
.statusrow { display: flex; align-items: center; gap: 10px; inline-size: 100%; min-block-size: 44px; padding: 0 14px; border: 1px solid var(--yuva-border); border-radius: 12px; background: transparent; text-align: start; cursor: pointer; }
.statusrow:hover { background: var(--yuva-deep); }
.statusrow .grow { flex: 1; }
.statusrow svg { color: var(--yuva-faint); }
.tone { flex: none; inline-size: 8px; block-size: 8px; border-radius: 50%; background: var(--yuva-ok); }
.tone.warn { background: var(--yuva-warn); }
.tone.bad { background: var(--yuva-danger); }
.tone.maint { background: var(--yuva-info); }
.foot { min-block-size: 40px; padding: 0; display: flex; align-items: center; justify-content: center; font-family: var(--yuva-mono); font-size: 11px; color: var(--yuva-faint); }
.page .sec.foot { padding: 0 32px; }
.day { display: flex; align-items: center; gap: 10px; font-family: var(--yuva-mono); font-size: 11px; letter-spacing: 0.08em; text-transform: uppercase; color: var(--yuva-faint); }
.day::before, .day::after { content: ""; flex: 1; block-size: 1px; background: var(--yuva-border); }
.sys { display: flex; align-items: center; gap: 8px; font-size: 13px; color: var(--yuva-muted); }
.group { display: flex; flex-direction: column; gap: 4px; }
.group.mine { align-items: flex-end; }
.row { display: flex; gap: 8px; align-items: flex-end; max-inline-size: 100%; }
.row.mine { justify-content: flex-end; }
.stack { flex: 1; min-inline-size: 0; display: flex; flex-direction: column; gap: 4px; align-items: flex-start; }
.author { padding-inline-start: 12px; font-size: 12px; font-weight: 500; color: var(--yuva-muted); }
.bubble { max-inline-size: 88%; padding: 8px 12px; border-radius: 12px; overflow-wrap: anywhere; white-space: normal; }
.mine .bubble { background: var(--yuva-accent); color: var(--yuva-on-accent); max-inline-size: 80%; }
.theirs .bubble { border: 1px solid var(--yuva-border); background: var(--yuva-deep); }
.mine .bubble.removed, .theirs .bubble.removed { background: transparent; border: 1px dashed var(--yuva-border); color: var(--yuva-faint); font-style: italic; }
.bubble a { color: inherit; text-decoration: underline; }
.bubble code { font-family: var(--yuva-mono); font-size: 13px; padding: 1px 4px; border-radius: 4px; background: rgb(127 127 127 / 0.2); }
.bubble pre { margin: 4px 0; padding: 8px; border-radius: 8px; background: rgb(127 127 127 / 0.2); font-family: var(--yuva-mono); font-size: 13px; white-space: pre-wrap; }
.bubble.greeting { align-self: flex-start; border: 1px solid var(--yuva-border); background: var(--yuva-deep); color: var(--yuva-fg); }
.bubble .files { margin-block-start: 6px; }
.bubble:has(.files):not(:has(> :not(.files))) { padding: 0; border: 0; background: transparent; }
.files { display: flex; flex-direction: column; gap: 4px; align-items: flex-end; }
.theirs .files { align-items: flex-start; }
.att { display: flex; align-items: center; gap: 10px; inline-size: 220px; max-inline-size: 100%; padding: 8px; border: 1px solid var(--yuva-strong); border-radius: 12px; background: var(--yuva-bg); color: var(--yuva-fg); font: inherit; text-align: start; cursor: pointer; }
.att:hover { background: var(--yuva-deep); }
.att .thumb { display: grid; place-items: center; flex: none; inline-size: 40px; block-size: 40px; border: 1px solid var(--yuva-border); border-radius: 8px; background: var(--yuva-deep); color: var(--yuva-muted); overflow: hidden; }
.att .thumb img { inline-size: 100%; block-size: 100%; object-fit: cover; }
.att .info { flex: 1; min-inline-size: 0; line-height: 1.2; }
.att .name, .att .sub { display: block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.att .name { font-size: 13px; font-weight: 500; }
.att .sub { margin-block-start: 2px; font-size: 12px; color: var(--yuva-faint); }
.att.busy { cursor: default; }
.att.busy .thumb { animation: pulse 1.2s infinite; }
.att.bad { border-color: var(--yuva-danger); }
.att.bad .sub { color: var(--yuva-danger); }
.preview { display: block; max-inline-size: 100%; max-block-size: 220px; border-radius: 12px; border: 1px solid var(--yuva-border); cursor: zoom-in; }
.meta { display: flex; align-items: center; gap: 4px; font-size: 12px; color: var(--yuva-faint); }
.meta.failed { color: var(--yuva-danger); }
.meta.seen svg { color: var(--yuva-brand); }
.theirs-meta { padding-inline-start: 36px; }
.typing { display: flex; gap: 8px; align-items: flex-end; }
.typing .dots { display: flex; gap: 4px; padding: 12px 14px; border: 1px solid var(--yuva-border); border-radius: 12px; background: var(--yuva-deep); }
.typing i { inline-size: 6px; block-size: 6px; border-radius: 50%; background: var(--yuva-muted); animation: blink 1.2s infinite both; }
.typing i:nth-child(2) { animation-delay: 0.15s; }
.typing i:nth-child(3) { animation-delay: 0.3s; }
@keyframes blink { 0%, 80%, 100% { opacity: 0.25; } 40% { opacity: 1; } }
@keyframes pulse { 50% { opacity: 0.5; } }
@media (prefers-reduced-motion: reduce) { .typing i, .att.busy .thumb { animation: none; } }
.card { padding: 12px; border: 1px solid var(--yuva-border); border-radius: 12px; font-size: 14px; }
.card p { margin: 0 0 8px; }
.mail { display: flex; gap: 12px; align-items: flex-start; padding: 12px; border: 1px dashed var(--yuva-strong); border-radius: 12px; font-size: 13px; }
.mail > svg { flex: none; margin-block-start: 1px; color: var(--yuva-brand); }
.mail .text { flex: 1; min-inline-size: 0; display: flex; flex-direction: column; gap: 2px; }
.mail .text > b { font-size: 14px; font-weight: 500; }
.mail .soft { color: var(--yuva-muted); overflow-wrap: anywhere; }
.mail .soft b { color: var(--yuva-fg); font-weight: 500; }
.mail .acts { display: flex; gap: 16px; margin-block-start: 2px; }
.mail.bad { flex-direction: column; gap: 8px; border: 1px solid var(--yuva-brand); background: var(--yuva-wash); }
.mail.bad .head { display: flex; gap: 8px; align-items: center; font-size: 14px; font-weight: 500; }
.mail.bad .head svg { color: var(--yuva-brand); }
.mail.ok { border: 0; padding: 0; align-items: center; gap: 8px; color: var(--yuva-muted); }
.mail.ok > svg { color: var(--yuva-ok); margin: 0; }
.inline-form { display: flex; gap: 8px; margin: 0; }
.inline-form input { flex: 1; min-inline-size: 0; }
.field { min-block-size: 40px; padding: 0 12px; border: 1px solid var(--yuva-strong); border-radius: 8px; background: var(--yuva-bg); color: var(--yuva-fg); }
textarea.field { padding: 10px 12px; min-block-size: 96px; resize: vertical; inline-size: 100%; }
.btn { min-block-size: 40px; padding: 0 14px; border: 0; border-radius: 8px; background: var(--yuva-accent); color: var(--yuva-on-accent); font-weight: 500; cursor: pointer; }
.btn:disabled { opacity: 0.5; cursor: default; }
.error { margin: 0; font-size: 13px; color: var(--yuva-danger); }
.rate { text-align: center; }
.choices { display: flex; justify-content: center; gap: 10px; }
.card .choice { inline-size: 44px; block-size: 44px; padding: 0; border: 1px solid var(--yuva-border); border-radius: 50%; background: var(--yuva-bg); font-size: 20px; line-height: 1; cursor: pointer; }
.card .choice:hover:not(:disabled), .card .choice[aria-pressed="true"] { border-color: var(--yuva-accent); box-shadow: 0 0 0 1px var(--yuva-accent); }
.card .choice:disabled { opacity: 1; cursor: default; }
.rate .choices + p { margin: 8px 0 0; }
.rate form { display: flex; flex-direction: column; gap: 8px; margin-block-start: 10px; }
.rate textarea { inline-size: 100%; padding: 8px 10px; border: 1px solid var(--yuva-strong); border-radius: 8px; background: var(--yuva-bg); resize: vertical; }
.rate .actions { display: flex; justify-content: flex-end; align-items: center; gap: 14px; }
.rate .actions .link { color: var(--yuva-muted); text-decoration: none; }
.rate .actions button:not(.link) { min-block-size: 40px; padding: 0 14px; border: 0; border-radius: 8px; background: var(--yuva-accent); color: var(--yuva-on-accent); font-weight: 500; cursor: pointer; }
.composer { padding-block: 12px 14px; gap: 0; background: var(--yuva-bg); }
.box { border: 1px solid var(--yuva-strong); border-radius: 14px; background: var(--yuva-bg); display: flex; flex-direction: column; }
.box:focus-within { border-color: var(--yuva-brand); }
.chips { display: flex; flex-wrap: wrap; gap: 6px; padding: 8px 8px 0; }
.chip { display: flex; align-items: center; gap: 8px; max-inline-size: 100%; padding: 6px 6px 6px 8px; border: 1px solid var(--yuva-border); border-radius: 10px; background: var(--yuva-deep); color: var(--yuva-muted); }
.chip .two { display: flex; flex-direction: column; min-inline-size: 0; line-height: 1.2; }
.chip .two b { font-size: 12px; font-weight: 500; color: var(--yuva-fg); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; max-inline-size: 150px; }
.chip .two span { font-size: 11px; color: var(--yuva-faint); }
.box textarea { display: block; inline-size: 100%; block-size: 44px; max-block-size: 140px; padding: 10px 12px 4px; border: 0; background: transparent; resize: none; outline: none; }
.tools { display: flex; align-items: center; gap: 2px; padding: 4px 6px 6px; }
.tools .grow { flex: 1; }
.hintkey { display: flex; align-items: center; gap: 4px; margin-inline-end: 6px; font-size: 11px; color: var(--yuva-faint); }
.hintkey kbd { font-family: var(--yuva-mono); padding: 1px 5px; border: 1px solid var(--yuva-strong); border-radius: 4px; background: var(--yuva-deep); }
.send { inline-size: 36px; block-size: 36px; display: grid; place-items: center; padding: 0; border: 0; border-radius: 9px; background: var(--yuva-accent); color: var(--yuva-on-accent); cursor: pointer; }
.send:disabled { opacity: 0.4; cursor: default; }
.jump { position: absolute; inset-inline-start: 50%; inset-block-start: -48px; transform: translateX(-50%); z-index: 4; display: flex; align-items: center; gap: 6px; min-block-size: 32px; padding: 0 12px; border: 1px solid var(--yuva-strong); border-radius: 999px; background: var(--yuva-bg); color: var(--yuva-fg); font-size: 12px; font-weight: 500; box-shadow: 0 6px 16px -8px rgb(0 0 0 / 0.25); cursor: pointer; white-space: nowrap; }
[dir="rtl"] .jump { transform: translateX(50%); }
.banner { padding: 6px 12px; background: var(--yuva-deep); color: var(--yuva-muted); font-size: 13px; text-align: center; border-block-start: 1px solid var(--yuva-border); }
@media (max-width: 480px) {
  .box textarea, .field, textarea.field, .inline-form input, .rate textarea { font-size: 16px; }
  .hintkey { display: none; }
}
@media (hover: none) and (pointer: coarse) { .hintkey { display: none; } }
.notice { position: relative; z-index: 2; flex: none; display: flex; align-items: center; gap: 6px; margin: 8px 32px; padding-block: 6px; padding-inline: 10px 4px; border-inline-start: 3px solid var(--yuva-faint); border-radius: 8px; background: var(--yuva-deep); font-size: 13px; }
.sec > .notice { margin: 0; }
.notice-text { flex: 1; min-inline-size: 0; overflow-wrap: anywhere; }
.notice a { color: inherit; text-decoration: underline; }
.notice .icon { color: var(--yuva-muted); }
.notice.impact-minor { border-inline-start-color: var(--yuva-warn); }
.notice.impact-major { border-inline-start-color: #ea580c; }
.notice.impact-critical { border-inline-start-color: var(--yuva-danger); }
.sr { position: absolute; inline-size: 1px; block-size: 1px; overflow: hidden; clip-path: inset(50%); white-space: nowrap; }
`
