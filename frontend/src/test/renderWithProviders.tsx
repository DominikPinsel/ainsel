import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, type RenderOptions } from '@testing-library/react'
import { MemoryRouter, type MemoryRouterProps } from 'react-router-dom'
import type { ReactElement } from 'react'

type Opts = {
  route?: string
  routerProps?: Omit<MemoryRouterProps, 'initialEntries' | 'children'>
  queryClient?: QueryClient
  renderOptions?: RenderOptions
}

export function renderWithProviders(ui: ReactElement, opts: Opts = {}) {
  const queryClient =
    opts.queryClient ??
    new QueryClient({
      defaultOptions: {
        queries: { retry: false, gcTime: 0, staleTime: 0 },
      },
    })

  const wrapped = (
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[opts.route ?? '/']} {...opts.routerProps}>
        {ui}
      </MemoryRouter>
    </QueryClientProvider>
  )

  return { queryClient, ...render(wrapped, opts.renderOptions) }
}
