type Listener = () => void

let listener: Listener | null = null

export function onUnauthorized(fn: Listener) {
  listener = fn
}

export function notifyUnauthorized() {
  listener?.()
}
