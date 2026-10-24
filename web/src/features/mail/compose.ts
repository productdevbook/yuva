export const COMPOSE_EVENT = "yuva:compose-mail"

export function openCompose() {
  window.dispatchEvent(new Event(COMPOSE_EVENT))
}
