import { render } from '@testing-library/react'
import { Markdown } from './Markdown'

describe('Markdown', () => {
  it('renders inline bold markdown', () => {
    const { container } = render(<Markdown source="hello **world**" />)
    const strong = container.querySelector('strong')
    expect(strong).not.toBeNull()
    expect(strong?.textContent).toBe('world')
  })

  it('strips dangerous HTML', () => {
    const { container } = render(<Markdown source={'safe <script>alert(1)</script>'} />)
    expect(container.querySelector('script')).toBeNull()
  })

  it('renders code blocks', () => {
    const src = '```\nconst x = 1\n```'
    const { container } = render(<Markdown source={src} />)
    expect(container.querySelector('pre code')).not.toBeNull()
  })

  it('applies .md-body class', () => {
    const { container } = render(<Markdown source="x" />)
    expect(container.firstChild).toHaveClass('md-body')
  })

  it('handles empty source without error', () => {
    const { container } = render(<Markdown source="" />)
    expect(container.firstChild).toHaveClass('md-body')
  })
})
