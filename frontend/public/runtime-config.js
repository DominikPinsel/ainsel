// Default runtime config — used in `pnpm dev`. In production this file is
// overwritten by frontend/docker-entrypoint-runtime-config.sh at container
// startup from env vars (OIDC_ISSUER, OIDC_CLIENT_ID, OIDC_PROJECT_ID).
window.__AINSEL_CONFIG__ = {
  oidcIssuer: "https://oidc.example.com",
  oidcClientId: "your-oidc-client-id",
  oidcProjectId: "your-oidc-project-id",
  forgejoApiBase: "https://forgejo.example.com/api/v1",
  forgejoRepo: "owner/repo",
};
