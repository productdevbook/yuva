const shared = Symbol.for("yuva.navigation");

type Watched = Window & { [shared]?: EventTarget };

// Routers call pushState/replaceState without any event, so both are wrapped once per window; the
// wrapper only calls the original and notifies afterwards. Other copies of this module (the chat
// bundle, another version) reuse the same target instead of wrapping again.
function navigation(): EventTarget {
  const win = window as Watched;
  let target = win[shared];
  if (target) return target;
  const created = new EventTarget();
  target = win[shared] = created;
  const notify = () => created.dispatchEvent(new Event("change"));
  for (const name of ["pushState", "replaceState"] as const) {
    const original = history[name];
    history[name] = function (this: History, ...args: Parameters<History["pushState"]>) {
      const result = original.apply(this, args);
      notify();
      return result;
    };
  }
  window.addEventListener("popstate", notify);
  window.addEventListener("hashchange", notify);
  return target;
}

export function pageOf(value: string | null): string {
  try {
    const url = new URL(value || location.href, location.href);
    url.search = "";
    url.hash = "";
    return url.href;
  } catch {
    return value || location.href;
  }
}

export function followPage(listener: () => void): () => void {
  const target = navigation();
  let scheduled = false;
  const changed = () => {
    if (scheduled) return;
    scheduled = true;
    queueMicrotask(() => {
      scheduled = false;
      listener();
    });
  };
  target.addEventListener("change", changed);
  return () => target.removeEventListener("change", changed);
}
