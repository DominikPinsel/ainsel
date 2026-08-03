import { useQuery } from '@tanstack/react-query'
import { useAuth } from 'react-oidc-context'
import { getUser, type HubUser } from '../api/users'

export function useCurrentUser() {
  const auth = useAuth()
  const sub = auth.user?.profile?.sub

  return useQuery<HubUser>({
    queryKey: ['currentUser', sub],
    queryFn: () => getUser(sub!),
    enabled: !!sub,
  })
}
