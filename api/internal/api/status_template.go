package api

import "html/template"

const statusStyle = `
:root{color-scheme:light dark;--paper:#ffffff;--deep:#f7f7f8;--ink:#0b0b0f;--soft:#52525b;--cross:#71717a;--rule:#ececef;--strong:#d4d4d8;
--brand:#c2410c;--wash:#fff1eb;--ok:#16a34a;--ok-t:#52525b;--ok-deep:#15803d;--warn:#eab308;--warn-t:#a16207;--warn-wash:#fefce8;
--partial:#f97316;--partial-t:#c2410c;--bad:#dc2626;--bad-t:#b91c1c;--maint:#2563eb}
@media (prefers-color-scheme:dark){:root{--paper:#09090b;--deep:#0f0f12;--ink:#fafafa;--soft:#a1a1aa;--cross:#8a8a93;--rule:#1f1f23;--strong:#2e2e34;
--brand:#ff8a5c;--wash:#2a140c;--ok:#4ade80;--ok-t:#a1a1aa;--ok-deep:#4ade80;--warn:#facc15;--warn-t:#facc15;--warn-wash:#1f1a05;
--partial:#fb923c;--partial-t:#fdba74;--bad:#f87171;--bad-t:#fca5a5;--maint:#60a5fa}}
*{box-sizing:border-box}
[hidden]{display:none!important}
html{-webkit-text-size-adjust:100%}
body{overflow-x:hidden}
@supports (overflow:clip){body{overflow-x:clip}}
body{margin:0;min-height:100vh;background:var(--paper);color:var(--ink);font:15px/1.55 Geist,ui-sans-serif,system-ui,sans-serif;letter-spacing:-.1px}
a{color:inherit;text-decoration:none}
a:hover{text-decoration:underline;text-underline-offset:3px}
a:focus-visible,button:focus-visible,select:focus-visible{outline:2px solid var(--brand);outline-offset:2px}
h1,h2,p{margin:0}
.sr{position:absolute;width:1px;height:1px;overflow:hidden;clip:rect(0 0 0 0);white-space:nowrap}
.mono{font-family:"Geist Mono",ui-monospace,monospace}
.label{font-family:"Geist Mono",ui-monospace,monospace;font-size:11px;text-transform:uppercase;letter-spacing:1px;color:var(--soft)}
.soft{color:var(--soft)}
.sm{font-size:13px}
.accent{color:var(--brand)}
[dir=rtl] .fl{display:inline-block;transform:scaleX(-1)}
.band{border-bottom:1px solid var(--rule)}
.col{position:relative;max-width:1040px;margin:0 auto;border-inline:1px solid var(--rule)}
.x{position:absolute;bottom:-6px;width:11px;height:11px;color:var(--cross);z-index:1}
.x.s{inset-inline-start:-6px}.x.e{inset-inline-end:-6px}
.grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(min(300px,100%),1fr));gap:1px;background:var(--rule)}
.grid>*{background:var(--paper);min-width:0}
.g4{grid-template-columns:repeat(auto-fit,minmax(min(230px,100%),1fr))}
.wide{grid-column:span 2}
.pad{padding:28px;display:flex;flex-direction:column;gap:12px}
.head{display:flex;flex-wrap:wrap;justify-content:space-between;align-items:center;gap:8px 16px;padding:12px 28px;border-bottom:1px solid var(--rule);min-height:47px}
.bar{min-height:64px;padding:10px 28px;display:flex;flex-wrap:wrap;align-items:center;justify-content:space-between;gap:8px 20px}
.brandmark{display:flex;align-items:center;gap:10px}
.brandmark img{display:block;width:auto;height:26px;max-width:120px}
.brandmark .name{font-size:17px;font-weight:600;letter-spacing:-.3px}
.brandmark .label{font-size:12px}
nav.top{display:flex;flex-wrap:wrap;align-items:center;gap:8px 22px;font-size:14px;color:var(--soft)}
.lang{display:inline-flex;align-items:center;gap:6px}
.lang label{position:relative;display:inline-flex;align-items:center}
.lang svg{position:absolute;pointer-events:none}
.lang .globe{inset-inline-start:10px}.lang .chev{inset-inline-end:11px}
select{appearance:none;-webkit-appearance:none;font:500 14px Geist,ui-sans-serif,system-ui,sans-serif;color:var(--ink);background:var(--paper);border:1px solid var(--strong);border-radius:8px;min-height:36px;cursor:pointer}
.lang select{padding:7px 30px;max-width:180px}
.pick select{padding:7px 32px 7px 14px}
.pick{position:relative;display:inline-flex;align-items:center}
.pick svg{position:absolute;inset-inline-end:12px;pointer-events:none}
.btn,.go{font:600 14px Geist,ui-sans-serif,system-ui,sans-serif;background:var(--ink);color:var(--paper);border:0;border-radius:8px;padding:10px 18px;min-height:44px;cursor:pointer}
.go{min-height:36px;padding:6px 12px;font-size:13px}
.hero{padding:72px 28px 56px;display:flex;flex-direction:column;gap:14px}
.hero h1{font-size:52px;font-weight:600;letter-spacing:-1.8px;line-height:1.05;max-width:640px;overflow-wrap:anywhere}
.sub .hero{padding:64px 28px 48px}.sub .hero h1{font-size:46px;letter-spacing:-1.6px;line-height:1.08}
.lead{font-size:17px;color:var(--soft);max-width:560px}
.kick{display:inline-flex;flex-wrap:wrap;align-items:center;gap:8px 16px;font-family:"Geist Mono",ui-monospace,monospace;font-size:12px;text-transform:uppercase;letter-spacing:1px}
.kick>span{display:inline-flex;align-items:center;gap:8px}
.q{display:inline-block;width:8px;height:8px;flex:none}
.cells{display:grid;grid-auto-rows:minmax(0,1fr);gap:1px;background:var(--rule)!important}
.cell{background:var(--paper);padding:18px 24px;display:flex;flex-direction:column;justify-content:center;min-width:0}
.cell .val{font-size:26px;font-weight:600;letter-spacing:-.6px;overflow-wrap:anywhere}
.sub .cell .val{font-size:22px;letter-spacing:-.5px}
.card{padding:28px;display:flex;flex-direction:column;gap:8px}
.card h2{font-size:20px;font-weight:600;letter-spacing:-.4px}
.legend{display:flex;flex-wrap:wrap;gap:6px 14px}
.legend span{display:inline-flex;align-items:center;gap:5px}
.rows{display:flex;flex-direction:column;gap:1px;background:var(--rule)}
.group{background:var(--deep);padding:10px 28px}
.comp{display:grid;grid-template-columns:1fr 2fr;gap:1px;background:var(--rule)}
.comp.nobars{grid-template-columns:1fr}
.comp>div{background:var(--paper);padding:18px 28px;min-width:0}
.cname{display:flex;flex-wrap:wrap;justify-content:space-between;align-items:center;gap:6px 12px}
.cname>span:first-child{min-width:0;overflow-wrap:anywhere}
.cname b{font-weight:600;display:block}
.cname small{display:block;color:var(--soft);font-size:13px}
.st{font-size:13px;font-weight:500;display:inline-flex;align-items:center;gap:7px;white-space:nowrap}
.st .q{width:7px;height:7px}
.cbars{display:flex;flex-direction:column;justify-content:center;gap:6px}
.bars,.strip{display:block;width:100%;max-width:100%;height:auto}
.scale{display:flex;flex-wrap:wrap;justify-content:space-between;gap:4px 12px;font-size:11px;color:var(--soft)}
.d-none{fill:var(--rule)}
.d-operational,.q-operational{fill:var(--ok);background:var(--ok)}
.d-degraded,.q-degraded{fill:var(--warn);background:var(--warn)}
.d-partial_outage,.q-partial_outage{fill:var(--partial);background:var(--partial)}
.d-major_outage,.q-major_outage{fill:var(--bad);background:var(--bad)}
.d-maintenance,.q-maintenance{fill:var(--maint);background:var(--maint)}
.k-operational{color:var(--ok-t)}.k-degraded,.i-degraded{color:var(--warn-t)}.k-partial_outage,.i-partial_outage{color:var(--partial-t)}
.k-major_outage,.i-major_outage{color:var(--bad-t)}.k-maintenance{color:var(--maint)}.i-none{color:var(--soft)}
.kick.k-operational,.kick .k-operational{color:var(--ok-deep)}
.tabs{display:inline-flex;flex-wrap:wrap;max-width:100%;border:1px solid var(--strong);border-radius:8px;overflow:hidden}
.tabs label{font-size:13px;font-weight:500;padding:6px 14px;min-height:34px;display:inline-flex;align-items:center;cursor:pointer}
.tabs label+label{border-inline-start:1px solid var(--strong)}
.rt-in{position:absolute;opacity:0;pointer-events:none}
.rt-tabs .rt-chart{display:none!important}
#rt0:checked~.rt-chart.c0,#rt1:checked~.rt-chart.c1,#rt2:checked~.rt-chart.c2{display:grid!important}
#rt0:checked~.head label[for=rt0],#rt1:checked~.head label[for=rt1],#rt2:checked~.head label[for=rt2]{background:var(--ink);color:var(--paper)}
.rt-in:focus-visible~.head .tabs{outline:2px solid var(--brand);outline-offset:2px}
.rt-chart{display:grid;grid-template-columns:repeat(auto-fit,minmax(min(200px,100%),1fr));gap:1px;background:var(--rule)}
.rt-chart+.rt-chart{border-top:1px solid var(--rule)}
.rt-chart>div{background:var(--paper);padding:24px 28px;display:flex;flex-direction:column;gap:4px;min-width:0}
.rt-chart .plot{grid-column:span 3;gap:8px}
.big{font-size:32px;font-weight:600;letter-spacing:-1px}
.chart{display:block;width:100%;max-width:100%;height:auto}
.chart .off{fill:var(--rule)}
.chart .band{fill:var(--warn);opacity:.2}
.chart .on{fill:var(--brand)}
.chart .hot{fill:var(--warn)}
.chart .w{opacity:.13}
.past{display:grid;grid-template-columns:72px minmax(0,1fr);gap:10px 16px;font-size:14px}
.past .mono{font-size:12px}
.picks{display:flex;flex-wrap:wrap;gap:6px}
.chip{position:relative}
.chip input{position:absolute;opacity:0}
.chip span{display:inline-flex;align-items:center;font-size:13px;font-weight:500;border:1px solid var(--strong);border-radius:6px;padding:5px 10px;min-height:32px;cursor:pointer}
.chip input:checked+span{background:var(--ink);color:var(--paper);border-color:var(--ink)}
.chip input:focus-visible+span{outline:2px solid var(--brand);outline-offset:2px}
form.sub-form{display:flex;flex-direction:column;gap:12px;align-items:flex-start}
.follow{display:flex;flex-direction:column;gap:6px;padding:24px 28px}
.follow b{font-weight:600}
.follow .mono{font-size:12px;overflow-wrap:anywhere}
code{display:block;max-width:100%;font-family:"Geist Mono",ui-monospace,monospace;font-size:11px;background:var(--deep);border:1px solid var(--rule);border-radius:6px;padding:6px 8px;white-space:pre-wrap;overflow-wrap:anywhere;word-break:break-all}
.upd{display:grid;grid-template-columns:repeat(auto-fit,minmax(min(200px,100%),1fr));gap:1px;background:var(--rule)}
.upd>div{background:var(--paper);padding:22px 28px;min-width:0}
.upd .body{grid-column:span 3;overflow-wrap:anywhere}
.upd b{display:inline-flex;align-items:center;gap:8px;font-weight:600}
.upd time,.upd .edited{font-family:"Geist Mono",ui-monospace,monospace;font-size:12px;color:var(--soft)}
.aff{display:flex;flex-wrap:wrap;justify-content:space-between;gap:4px 12px}
.links{display:flex;flex-wrap:wrap;gap:20px;font-size:14px}
.month{display:grid;grid-template-columns:repeat(auto-fit,minmax(min(220px,100%),1fr));gap:1px;background:var(--rule)}
.month>div{background:var(--paper);min-width:0}
.month .items{grid-column:span 3;display:flex;flex-direction:column;gap:1px;background:var(--rule)}
.item{background:var(--paper);padding:20px 28px;display:grid;grid-template-columns:72px minmax(0,1fr) auto;gap:16px;align-items:baseline;flex:1}
.item small{display:block;font-size:13px;color:var(--soft)}
.item b{font-weight:600}
.quiet{background:var(--paper);padding:24px 28px;color:var(--soft);flex:1}
.seg{display:inline-flex;flex-wrap:wrap;max-width:100%;align-self:flex-start;border:1px solid var(--strong);border-radius:8px;overflow:hidden;margin-top:8px}
.seg a{font-size:13px;font-weight:500;padding:6px 14px;min-height:34px;display:inline-flex;align-items:center}
.seg a+a{border-inline-start:1px solid var(--strong)}
.seg a.on{background:var(--ink);color:var(--paper)}
.foot{padding:24px 28px 64px;display:flex;flex-wrap:wrap;justify-content:space-between;gap:12px;font-size:13px;color:var(--soft)}
.foot .links{font-size:13px}
.powered{white-space:nowrap}
.mark{display:block;flex:none}
.powered .mark{display:inline-block;vertical-align:-.2em;vertical-align:calc(.5cap - 8px);margin-inline-end:6px}
@media (max-width:1053px){.x{display:none}}
@media (max-width:700px){.wide,.rt-chart .plot,.upd .body,.month .items{grid-column:auto}.comp{grid-template-columns:1fr}
.hero{padding:48px 20px 40px}.hero h1,.sub .hero h1{font-size:36px;letter-spacing:-1.2px}.bar,.head,.pad,.card,.comp>div,.follow,.rt-chart>div,.upd>div,.item,.quiet,.foot,.cell,.group{padding-inline:16px}
.item{grid-template-columns:56px minmax(0,1fr)}.item .label{grid-column:2}}
`

var statusTemplate = template.Must(template.New("status").Parse(`<!doctype html>
<html lang="{{.W.Lang}}" dir="{{.Dir}}"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Name}} – {{if .Incident}}{{.Incident.Title}}{{else if eq .View "history"}}{{.W.History}}{{else}}{{.Headline}}{{end}}</title>
<link rel="alternate" type="application/atom+xml" href="{{.FeedURL}}" title="{{.W.Feed}}">
<link rel="preload" href="{{.FontSans}}" as="font" type="font/woff2" crossorigin>
<style>
@font-face{font-family:Geist;src:url("{{.FontSans}}") format("woff2");font-weight:100 900;font-display:swap}
@font-face{font-family:"Geist Mono";src:url("{{.FontMono}}") format("woff2");font-weight:100 900;font-display:swap}
` + statusStyle + `{{if .Accent}}:root{--brand:{{.Accent}};--wash:color-mix(in srgb,{{.Accent}} 12%,var(--paper))}{{end}}
</style></head><body{{if ne .View "page"}} class="sub"{{end}}>
{{define "x"}}<svg class="x s" viewBox="0 0 11 11" aria-hidden="true"><path d="M5.5 0v11M0 5.5h11" stroke="currentColor"/></svg><svg class="x e" viewBox="0 0 11 11" aria-hidden="true"><path d="M5.5 0v11M0 5.5h11" stroke="currentColor"/></svg>{{end}}
{{define "cells"}}<div class="cells">{{range .}}<div class="cell"><span class="label">{{.Label}}</span><span class="val">{{if .Time}}<time datetime="{{.Time.ISO}}" title="{{.Time.Text}}">{{.Value}}</time>{{else}}{{.Value}}{{end}}</span></div>{{end}}</div>{{end}}
{{define "response"}}<section class="band"><div class="col{{if .Tabs}} rt-tabs{{end}}">{{template "x"}}
{{if .Tabs}}{{range $i, $c := .Charts}}<input class="rt-in" type="radio" name="rt" id="rt{{$i}}"{{if eq $i 0}} checked{{end}}>{{end}}{{end}}
<div class="head"><span class="label">{{if .Num}}{{.Num}} — {{end}}{{.Title}}</span>
{{if .Tabs}}<div class="tabs" role="group" aria-label="{{$.W.Monitored}}">{{range $i, $c := .Charts}}<label for="rt{{$i}}">{{.Name}}</label>{{end}}</div>{{end}}
{{if .Select}}<label class="pick" hidden><span class="sr">{{$.W.Monitored}}</span><select data-rt>{{range $i, $c := .Charts}}<option value="{{$i}}">{{.Name}}</option>{{end}}</select><svg viewBox="0 0 12 12" width="10" height="10" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M3 4.5l3 3 3-3"/></svg></label>{{end}}
</div>
{{range $i, $c := .Charts}}<div class="rt-chart c{{$i}}" data-rt-chart="{{$i}}">
<div><span class="label">{{.StatLabel}}</span><span class="big">{{.Stat}}</span>{{if .Sub}}<span class="sm soft">{{.Sub}}</span>{{end}}</div>
<div class="plot">{{if .Empty}}<p class="soft">{{$.W.NoChecks}}</p>{{else}}<svg class="chart" viewBox="{{.Grid.ViewBox}}" shape-rendering="crispEdges" role="img" aria-label="{{.Label}}">{{range .Grid.Cells}}<path class="{{.Class}}" d="{{.D}}"/>{{end}}</svg>{{end}}
<div class="scale mono">{{range .Axis}}<span>{{.}}</span>{{end}}</div></div>
</div>{{end}}
</div></section>{{end}}
<header class="band"><div class="col bar">{{template "x"}}
<a class="brandmark" href="{{.Home}}">{{if .Logo}}{{if .LogoDark}}<picture><source media="(prefers-color-scheme: dark)" srcset="{{.LogoDark}}">{{end}}<img src="{{.Logo}}" alt="{{if not .ShowName}}{{.Name}}{{end}}">{{if .LogoDark}}</picture>{{end}}{{end}}{{if .ShowName}}<span class="name">{{.Name}}</span>{{end}}<span class="label">{{.W.Status}}</span></a>
<nav class="top">{{if ne .View "page"}}<a href="{{.Home}}"><span class="fl">←</span> {{.W.Back}}</a>{{end}}{{if ne .View "history"}}<a href="{{.HistoryURL}}">{{.W.History}}</a>{{end}}{{if and (eq .View "page") .ShowFollow}}<a href="#follow">{{.W.Follow}}</a>{{end}}
{{if .Site}}<a href="{{.Site}}" rel="noopener">{{.W.Site}}</a>{{end}}{{if .Support}}<a href="{{.Support}}" rel="noopener">{{.W.Support}}</a>{{end}}
{{if and .Self (gt (len .Langs) 1)}}<form class="lang" method="get" action="{{.Self}}">{{range .Keep}}<input type="hidden" name="{{.Name}}" value="{{.Value}}">{{end}}<label><span class="sr">{{.W.Language}}</span><svg class="globe" viewBox="0 0 16 16" width="14" height="14" fill="none" stroke="currentColor" stroke-width="1.4" aria-hidden="true"><circle cx="8" cy="8" r="6.3"/><path d="M1.7 8h12.6M8 1.7c1.8 1.8 2.6 3.9 2.6 6.3S9.8 12.5 8 14.3M8 1.7C6.2 3.5 5.4 5.6 5.4 8s.8 4.5 2.6 6.3"/></svg><select name="lang" data-lang>{{range .Langs}}<option value="{{.Code}}" lang="{{.Code}}"{{if .On}} selected{{end}}>{{.Name}}</option>{{end}}</select><svg class="chev" viewBox="0 0 12 12" width="10" height="10" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M3 4.5l3 3 3-3"/></svg></label><button class="go" type="submit">{{.W.Apply}}</button></form>{{end}}
</nav></div></header>
<main>
{{if .Incident}}{{with .Incident}}
<section class="band"><div class="col grid">{{template "x"}}
<div class="hero wide"><div class="kick"><span class="k-{{.StateClass}}"><i class="q q-{{.StateClass}}"></i>{{.State}}</span>{{if not .Maintenance}}<span class="i-{{.ImpactClass}}">{{.Impact}}</span>{{end}}<span class="soft">{{.Kind}}</span></div>
<h1>{{.Title}}</h1>{{if .Start}}<p class="lead">{{$.W.Window}}: <time datetime="{{.Start.ISO}}" data-local>{{.Start.Text}}</time> – <time datetime="{{.End.ISO}}" data-local>{{.End.Text}}</time></p>{{end}}</div>
{{template "cells" $.Cells}}
</div></section>
{{end}}
{{with .Response}}{{template "response" .}}{{end}}
<section class="band"><div class="col">{{template "x"}}<div class="head"><span class="label">{{.Num.updates}} — {{.W.Updates}}</span></div>
<div class="rows">{{range .Incident.Updates}}<article class="upd"><div><b><i class="q q-{{.Class}}"></i>{{.State}}</b><br><time datetime="{{.At.ISO}}" data-local>{{.At.Text}}</time>{{if .Edited}} <span class="edited">· {{$.W.Edited}}</span>{{end}}</div>
<div class="body">{{if .Body}}<p>{{.Body}}</p>{{end}}</div></article>{{end}}</div></div></section>
<section class="band"><div class="col grid">{{template "x"}}
<div class="pad"><span class="label">{{.Num.affected}} — {{.W.Affected}}</span>{{range .Affected}}<div class="aff"><b>{{.Name}}</b><span class="sm soft">{{.Text}}</span></div>{{else}}<span class="soft">–</span>{{end}}</div>
{{if .Num.stay}}<div class="pad"><span class="label">{{.Num.stay}} — {{.W.StayInformed}}</span><span class="soft">{{.W.StayText}}</span><span class="links">{{if .SubscribeURL}}<a class="accent" href="{{.SubscribeURL}}" target="_top">{{.W.SubscribeHead}} <span class="fl">→</span></a>{{end}}{{if .ShowFollow}}<a class="accent" href="{{.FeedURL}}">{{.W.Feed}} <span class="fl">→</span></a>{{end}}</span></div>{{end}}
</div></section>
{{else if eq .View "history"}}
<section class="band"><div class="col grid">{{template "x"}}
<div class="hero wide"><span class="label">{{.W.History}}</span><h1>{{.W.HistoryHead}}</h1>
<nav class="seg" aria-label="{{.W.Filter}}">{{range .Filters}}<a href="{{.URL}}"{{if .On}} class="on" aria-current="page"{{end}}>{{.Label}}</a>{{end}}</nav></div>
{{template "cells" .Cells}}
</div></section>
{{range .Months}}<section class="band"><div class="col">{{template "x"}}<div class="month">
<div class="pad"><span class="label">{{.Index}} — {{.Name}}</span>{{if .Uptime}}<span class="big" style="font-size:26px">{{.Uptime}}</span>{{end}}{{.Strip}}</div>
<div class="items">{{range .Items}}<a class="item" href="{{.URL}}"><span class="mono soft sm">{{.Day}}</span><span><b>{{.Title}}</b><small>{{.Meta}}</small></span><span class="label {{.KindClass}}">{{.Kind}} <span class="fl">→</span></span></a>{{else}}<div class="quiet">{{$.W.NoMonth}}</div>{{end}}</div>
</div></div></section>{{else}}<section class="band"><div class="col pad"><p class="soft">{{.NoHistory}}</p></div></section>{{end}}
{{else}}
<section class="band"><div class="col grid">{{template "x"}}
<div class="hero wide"><div class="kick k-{{.StateClass}}"><span><i class="q q-{{.StateClass}}"></i>{{.State}}</span></div><h1>{{.Headline}}</h1>{{if .Description}}<p class="lead">{{.Description}}</p>{{end}}</div>
{{template "cells" .Cells}}
</div></section>
{{if or .Open .Maint}}<section class="band"><div class="col grid">{{template "x"}}
{{range .Open}}<div class="card"><div class="label {{.KindClass}}">{{.Kicker}}</div><h2><a href="{{.URL}}">{{.Title}} <span class="fl">→</span></a></h2>{{if .Text}}<p class="soft">{{.Text}}</p>{{end}}</div>{{end}}
{{range .Maint}}<div class="card"><div class="label k-maintenance">{{.Kicker}}</div><h2><a href="{{.URL}}">{{.Title}} <span class="fl">→</span></a></h2>{{if .Text}}<p class="soft">{{.Text}}</p>{{end}}{{if $.SubscribeURL}}<p><a class="accent" href="#subscribe">{{$.W.Subscribe}}</a></p>{{end}}</div>{{end}}
</div></section>{{end}}
<section class="band"><div class="col">{{template "x"}}
<div class="head"><span class="label">{{.Num.components}} — {{.W.Components}}</span>{{if .ShowBars}}<span class="legend label" aria-hidden="true">{{range .Legend}}<span><i class="q q-{{.Class}}"></i>{{.Label}}</span>{{end}}</span>{{end}}</div>
<div class="rows">{{range .Groups}}{{if .Name}}<div class="group label">{{.Name}}</div>{{end}}{{range .Components}}<div class="comp{{if not .Bars}} nobars{{end}}">
<div class="cname"><span><b>{{.Name}}</b>{{if .Description}}<small>{{.Description}}</small>{{end}}</span><span class="st k-{{.Class}}"><i class="q q-{{.Class}}" aria-hidden="true"></i>{{.State}}</span></div>
{{if .Bars}}<div class="cbars">{{.Bars}}<div class="scale mono"><span>{{$.OldestDay}}</span><span>{{.Uptime}}</span></div></div>{{end}}
</div>{{end}}{{end}}</div>
</div></section>
{{with .Response}}{{template "response" .}}{{end}}
<section class="band"><div class="col grid">{{template "x"}}
<div class="pad" id="history"><span class="label">{{.Num.recent}} — {{.W.Recent}}</span>
<div class="past">{{range .Recent}}<span class="mono soft">{{.Day}}</span><span><a href="{{.URL}}">{{.Title}}</a>{{if .Maintenance}} <span class="soft">· {{$.W.Maintenance}}</span>{{end}}</span>{{else}}<span></span><span class="soft">{{.W.NoRecent}}</span>{{end}}</div>
<a class="accent sm" href="{{.HistoryURL}}">{{.W.FullHistory}} <span class="fl">→</span></a></div>
{{if .SubscribeURL}}<div class="pad" id="subscribe"><span class="label">{{.Num.subscribe}} — {{.W.SubscribeHead}}</span>
<form class="sub-form" method="get" action="{{.SubscribeURL}}" target="_top">{{if .Picks}}<div class="picks">{{range .Picks}}<label class="chip"><input type="checkbox" name="components" value="{{.ID}}"><span>{{.Name}}</span></label>{{end}}</div>{{end}}<button class="btn" type="submit">{{.W.SubscribeGo}}</button></form>
<p class="sm soft">{{if .Picks}}{{.W.SubscribeText}}{{else}}{{.W.Subscribe}}{{end}}</p></div>{{end}}
</div></section>
{{if .ShowFollow}}<section class="band" id="follow"><div class="col">{{template "x"}}<div class="head"><span class="label">{{.Num.follow}} — {{.W.FollowEmbed}}</span></div>
<div class="grid g4">
<div class="follow"><b>{{.W.Feed}}</b><span class="sm soft">{{.W.FeedText}}</span><a class="mono accent" href="{{.FeedURL}}">{{.FeedPath}}</a></div>
<div class="follow"><b>{{.W.Badge}}</b><span class="sm soft">{{.W.BadgeText}}</span><a href="{{.BadgeURL}}"><img src="{{.BadgeURL}}" alt="{{.State}}" height="20"></a></div>
<div class="follow"><b>JSON</b><span class="sm soft">{{.W.JSONText}}</span><a class="mono accent" href="{{.JSONURL}}">{{.JSONPath}}</a></div>
<div class="follow"><b>{{.W.Embed}}</b><span class="sm soft">{{.W.EmbedText}}</span><code>{{.EmbedCode}}</code></div>
</div></div></section>{{end}}
{{end}}
</main>
<footer><div class="col foot"><span>{{.TimesIn}} <a class="powered" href="{{.YuvaSite}}" rel="noopener">{{.Mark}}{{.W.Powered}}</a></span>
<span class="links">{{if .Support}}<a href="{{.Support}}" rel="noopener">{{.W.Support}}</a>{{end}}{{if .ShowFollow}}<a href="{{.FeedURL}}">feed.atom</a><a href="{{.BadgeURL}}">badge.svg</a>{{end}}<span>{{.W.Updated}}: <time datetime="{{.Updated.ISO}}" data-local>{{.Updated.Text}}</time></span></span></div></footer>
<script>{{.Script}}</script>
</body></html>`))
