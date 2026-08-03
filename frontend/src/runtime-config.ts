type AinselConfig = {
  oidcIssuer: string
  oidcClientId: string
  oidcProjectId: string
  forgejoApiBase?: string
  forgejoRepo?: string
}

declare global {
  interface Window {
    __AINSEL_CONFIG__?: AinselConfig
  }
}

export function runtimeConfig(): AinselConfig {
  const c = window.__AINSEL_CONFIG__
  if (!c || !c.oidcIssuer || !c.oidcClientId || !c.oidcProjectId) {
    throw new Error(
      "runtime-config.js missing or incomplete: " +
        "window.__AINSEL_CONFIG__ must have oidcIssuer, oidcClientId, and oidcProjectId",
    )
  }
  return c
}
