// HisarWeb: the reset link carries e-mail and token in the URL fragment, which
// never reaches a server, proxy log or Referer header (ASVS V8.3.1). Links sent
// before this change still use the query string, so those keep working.
export type HwResetParams = { email?: string; token?: string }

export function hwReadResetParams(
  hash: string,
  fallback: HwResetParams
): HwResetParams {
  const fromHash = new URLSearchParams(hash.replace(/^#/, ''))
  return {
    email: fromHash.get('email') || fallback.email,
    token: fromHash.get('token') || fallback.token,
  }
}

// Removes the secret from the address bar and history entry once it is read.
export function hwClearResetParams(): void {
  if (typeof window === 'undefined') return
  const clean = window.location.pathname
  window.history.replaceState(window.history.state, '', clean)
}
