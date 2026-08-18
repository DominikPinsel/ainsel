-- Local password credentials for users (auth mode "local"). Empty string
-- means the user authenticates via OIDC only. The hash is argon2id in
-- self-describing PHC format ($argon2id$v=19$m=...,t=...,p=...$salt$hash).
ALTER TABLE users ADD COLUMN password_hash TEXT NOT NULL DEFAULT '';
