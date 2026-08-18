export type AuthMode = 'oidc' | 'local' | 'none'

type AinselConfig = {
  authMode?: AuthMode
  oidcIssuer: string
  oidcClientId: string
  oidcProjectId: string
  forgejoApiBase?: string
  forgejoRepo?: string
}

export type ResolvedConfig = AinselConfig & { authMode: AuthMode }

declare global {
  interface Window {
    __AINSEL_CONFIG__?: AinselConfig
  }
}

// runtimeConfig resolves the runtime configuration, including the effective
// auth mode:
//   - explicit authMode from the deployment wins ('local' and 'none' need no
//     OIDC fields),
//   - otherwise a complete OIDC triple means 'oidc',
//   - all-empty OIDC fields mean 'none' (port-forward / edge-proxy installs),
//   - a *partially* filled OIDC triple is a broken deploy and throws.
export function runtimeConfig(): ResolvedConfig {
  const c = window.__AINSEL_CONFIG__
  if (!c) {
    throw new Error(
      "runtime-config.js missing: window.__AINSEL_CONFIG__ is not defined",
    )
  }

  if (c.authMode === 'local' || c.authMode === 'none') {
    return { ...c, authMode: c.authMode }
  }

  const { oidcIssuer, oidcClientId, oidcProjectId } = c
  const allEmpty = !oidcIssuer && !oidcClientId && !oidcProjectId
  if (c.authMode === 'oidc' || !allEmpty) {
    if (!oidcIssuer || !oidcClientId || !oidcProjectId) {
      throw new Error(
        "runtime-config.js incomplete: " +
          "window.__AINSEL_CONFIG__ must have oidcIssuer, oidcClientId, and oidcProjectId",
      )
    }
    return { ...c, authMode: 'oidc' }
  }

  return { ...c, authMode: 'none' }
}
