interface ActionLocation {
  pathname: string
  search: string
  hash: string
}

interface ActionHistory {
  readonly state: unknown
  replaceState(data: unknown, unused: string, url?: string | URL | null): void
}

export function readActionToken(location: ActionLocation = window.location): string {
  const query = new URLSearchParams(location.search)
  const fragment = new URLSearchParams(location.hash.startsWith('#') ? location.hash.slice(1) : location.hash)
  return fragment.get('token') || query.get('token') || ''
}

// Removes every token copy from the visible URL. New links use the fragment
// so the token is never sent in the HTTP request; query support is retained
// for links already delivered before that change.
export function clearActionTokenFromURL(
  location: ActionLocation = window.location,
  history: ActionHistory = window.history,
): void {
  const query = new URLSearchParams(location.search)
  const fragment = new URLSearchParams(location.hash.startsWith('#') ? location.hash.slice(1) : location.hash)
  const fragmentHasToken = fragment.has('token')
  const queryHasToken = query.has('token')

  if (!fragmentHasToken && !queryHasToken) return

  query.delete('token')
  fragment.delete('token')
  const nextSearch = query.size > 0 ? `?${query.toString()}` : ''
  const nextHash = fragmentHasToken
    ? (fragment.size > 0 ? `#${fragment.toString()}` : '')
    : location.hash
  history.replaceState(history.state, '', `${location.pathname}${nextSearch}${nextHash}`)
}
