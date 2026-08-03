import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom'
import { AuthProvider } from './auth/AuthProvider'
import { RequireAuth } from './auth/RequireAuth'
import { Layout } from './layout/Layout'
import { LoginLayout } from './layout/LoginLayout'
import { AgentDetail } from './pages/agents/AgentDetail'
import { AgentForm } from './pages/agents/AgentForm'
import { AgentList } from './pages/agents/AgentList'
import { Callback } from './pages/auth/Callback'
import { ConnectorDetail } from './pages/connectors/ConnectorDetail'
import { ConnectorForm } from './pages/connectors/ConnectorForm'
import { ConnectorList } from './pages/connectors/ConnectorList'
import { Dashboard } from './pages/dashboard/Dashboard'
import { Activity } from './pages/activity/Activity'
import { DocsPage } from './pages/docs/DocsPage'
import { ChatList } from './pages/chat/ChatList'
import { ChatView } from './pages/chat/ChatView'
import { ImageDetailPage as ImageDetail } from './pages/images/ImageDetailPage'
import { ImageList } from './pages/images/ImageList'
import { Login } from './pages/login/Login'
import { Observability } from './pages/observability/Observability'
import { EventsDetail } from './pages/observability/events/EventsDetail'
import { EventView } from './pages/observability/events/EventView'
import { RoutingDetail } from './pages/observability/routing/RoutingDetail'
import { ErrorsDetail } from './pages/observability/errors/ErrorsDetail'
import { TokensDetail } from './pages/observability/tokens/TokensDetail'
import { PersonaDetail } from './pages/personas/PersonaDetail'
import { PersonaForm } from './pages/personas/PersonaForm'
import { PersonaList } from './pages/personas/PersonaList'
import { SettingsLayout } from './pages/settings/SettingsLayout'
import { Profile } from './pages/profile/Profile'
import { UserList } from './pages/users/UserList'
import { GroupDetail } from './pages/groups/GroupDetail'
import { GroupForm } from './pages/groups/GroupForm'
import { GroupList } from './pages/groups/GroupList'
import { MCPServerList } from './pages/mcp-servers/MCPServerList'
import { MCPServerForm } from './pages/mcp-servers/MCPServerForm'
import { SkillList } from './pages/skills/SkillList'
import { SkillForm } from './pages/skills/SkillForm'
import { SkillDetail } from './pages/skills/SkillDetail'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: false,
      staleTime: 30_000,
      refetchOnWindowFocus: false,
    },
  },
})

// React Router needs basename WITHOUT trailing slash; BASE_URL has one.
const routerBasename = import.meta.env.BASE_URL.replace(/\/$/, '') || undefined

export default function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <BrowserRouter basename={routerBasename}>
        <AuthProvider>
          <Routes>
            <Route element={<LoginLayout />}>
              <Route path="/login" element={<Login />} />
              <Route path="/auth/callback" element={<Callback />} />
            </Route>

            <Route
              element={
                <RequireAuth>
                  <Layout />
                </RequireAuth>
              }
            >
              <Route path="/dashboard" element={<Dashboard />} />

              <Route path="/chat" element={<ChatList />} />
              <Route path="/chat/:id" element={<ChatView />} />

              <Route path="/agents" element={<AgentList />} />
              <Route path="/agents/new" element={<AgentForm />} />
              <Route path="/agents/:id" element={<AgentDetail />} />
              <Route path="/agents/:id/edit" element={<AgentForm />} />

              <Route path="/personas" element={<PersonaList />} />
              <Route path="/personas/new" element={<PersonaForm />} />
              <Route path="/personas/:id" element={<PersonaDetail />} />
              <Route path="/personas/:id/edit" element={<PersonaForm />} />

              <Route path="/agent-images" element={<ImageList />} />
              <Route path="/agent-images/new" element={<ImageDetail />} />
              <Route path="/agent-images/:id" element={<ImageDetail />} />
              <Route path="/agent-images/:id/edit" element={<ImageDetail />} />

              <Route path="/connectors" element={<ConnectorList />} />
              <Route path="/connectors/new" element={<ConnectorForm />} />
              <Route path="/connectors/:id" element={<ConnectorDetail />} />
              <Route path="/connectors/:id/edit" element={<ConnectorForm />} />

              <Route path="/skills" element={<SkillList />} />
              <Route path="/skills/new" element={<SkillForm />} />
              <Route path="/skills/:id" element={<SkillDetail />} />
              <Route path="/skills/:id/edit" element={<SkillForm />} />

              <Route path="/profile" element={<Profile />} />
              <Route path="/users" element={<UserList />} />
              <Route path="/groups" element={<GroupList />} />
              <Route path="/groups/new" element={<GroupForm />} />
              <Route path="/groups/:id" element={<GroupDetail />} />
              <Route path="/groups/:id/edit" element={<GroupForm />} />

              <Route path="/error-log" element={<Navigate to="/observability/errors" replace />} />

              <Route path="/activity" element={<Activity />} />
              <Route path="/observability" element={<Observability />} />
              <Route path="/observability/events" element={<EventsDetail />} />
              <Route path="/observability/events/:id" element={<EventView />} />
              <Route path="/observability/routing" element={<RoutingDetail />} />
              <Route path="/observability/errors" element={<ErrorsDetail />} />
              <Route path="/observability/tokens" element={<TokensDetail />} />
              <Route path="/docs" element={<DocsPage />} />
              <Route path="/docs/*" element={<DocsPage />} />
              <Route path="/settings" element={<SettingsLayout />}>
                <Route index element={<Navigate to="mcp-servers" replace />} />
                <Route path="mcp-servers" element={<MCPServerList />} />
                <Route path="mcp-servers/new" element={<MCPServerForm />} />
                <Route path="mcp-servers/:name" element={<MCPServerForm />} />
              </Route>
            </Route>

            <Route path="/" element={<Navigate to="/dashboard" replace />} />
            <Route path="*" element={<Navigate to="/dashboard" replace />} />
          </Routes>
        </AuthProvider>
      </BrowserRouter>
    </QueryClientProvider>
  )
}