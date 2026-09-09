import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { afterEach, beforeEach, vi } from 'vitest'
import { Layout } from './Layout'

vi.mock('../auth/AuthProvider', () => ({
  useAuth: vi.fn(() => ({
    token: 'test-token',
    user: { username: 'kim', email: 'kim@example.com' },
    ready: true,
    signinRedirect: vi.fn(),
    signoutRedirect: vi.fn(),
  })),
}))

vi.mock('../components/ReportButton', () => ({
  ReportButton: () => null,
}))

function renderLayout(route = '/dashboard') {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0, staleTime: 0 } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[route]}>
        <Routes>
          <Route element={<Layout />}>
            <Route path="/dashboard" element={<div>Dashboard page</div>} />
            <Route path="/agents" element={<div>Agents page</div>} />
          </Route>
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

beforeEach(() => {
  vi.stubGlobal(
    'fetch',
    vi.fn(() =>
      Promise.resolve(
        new Response(
          JSON.stringify({ items: [], total: 0, page: 1, pageSize: 200, totalPages: 0 }),
          { status: 200 },
        ),
      ),
    ),
  )
})
afterEach(() => {
  vi.unstubAllGlobals()
})

const menuButton = () => screen.getByRole('button', { name: 'Menu' })

describe('Layout', () => {
  it('renders the mobile menu button in collapsed state', () => {
    renderLayout()
    expect(menuButton()).toHaveAttribute('aria-expanded', 'false')
    expect(document.querySelector('.spine')).not.toHaveClass('open')
    expect(document.querySelector('.spine-backdrop')).toBeNull()
  })

  it('opens the sidebar drawer when the menu button is clicked', async () => {
    const user = userEvent.setup()
    renderLayout()
    await user.click(menuButton())
    expect(menuButton()).toHaveAttribute('aria-expanded', 'true')
    expect(document.querySelector('.spine')).toHaveClass('open')
    expect(document.querySelector('.spine-backdrop')).toBeInTheDocument()
  })

  it('closes the drawer when the backdrop is clicked', async () => {
    const user = userEvent.setup()
    renderLayout()
    await user.click(menuButton())
    const backdrop = document.querySelector('.spine-backdrop') as HTMLElement
    await user.click(backdrop)
    expect(document.querySelector('.spine')).not.toHaveClass('open')
  })

  it('closes the drawer on Escape', async () => {
    const user = userEvent.setup()
    renderLayout()
    await user.click(menuButton())
    expect(document.querySelector('.spine')).toHaveClass('open')
    await user.keyboard('{Escape}')
    expect(document.querySelector('.spine')).not.toHaveClass('open')
  })

  it('closes the drawer after navigating to a page', async () => {
    const user = userEvent.setup()
    renderLayout()
    await user.click(menuButton())
    await user.click(screen.getByRole('link', { name: /Agents/i }))
    expect(await screen.findByText('Agents page')).toBeInTheDocument()
    expect(document.querySelector('.spine')).not.toHaveClass('open')
  })

  it('locks body scroll while the drawer is open and restores it', async () => {
    const user = userEvent.setup()
    renderLayout()
    await user.click(menuButton())
    expect(document.body.style.overflow).toBe('hidden')
    await user.keyboard('{Escape}')
    expect(document.body.style.overflow).toBe('')
  })
})
