import type { UserToken } from '../../api/userTokens'

// A token is dead when revoked OR past its expiry. #152
export function tokenStatus(
  tok: Pick<UserToken, 'revokedAt' | 'expiresAt'>,
  now: Date = new Date(),
): { label: string; className: string } {
  if (tok.revokedAt) return { label: 'Revoked', className: 'label' }
  if (new Date(tok.expiresAt).getTime() < now.getTime()) {
    return { label: 'Expired', className: 'tag warn' }
  }
  return { label: 'Active', className: 'tag tag-green' }
}