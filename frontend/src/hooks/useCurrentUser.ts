import { useQuery } from '@tanstack/react-query'
import { useAuth } from '../auth/AuthProvider'
import { getUser, type HubUser } from '../api/users'

// useCurrentUser resolves the authenticated user's registry record (role,
// admin flag) from the hub. Works in oidc and local modes; in no-auth mode
// there is no identity, so the query stays disabled.
export function useCurrentUser() {
  const auth = useAuth()
  const sub = auth.user?.sub

  return useQuery<HubUser>({
    queryKey: ['currentUser', sub],
    queryFn: () => getUser(sub!),
    enabled: !!sub,
  })
}
