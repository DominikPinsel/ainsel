import { render } from '@testing-library/react'
import { TimeSeriesChart, type SeriesStyle } from './TimeSeriesChart'

const styles: SeriesStyle[] = [
  { name: 'a', color: 'var(--ink)', label: 'Series A' },
  { name: 'b', color: 'var(--signal)', label: 'Series B', dashed: true },
]

describe('TimeSeriesChart', () => {
  it('renders a path per styled series', () => {
    const series = [
      {
        name: 'a',
        points: [
          { timestamp: '2026-05-20T00:00:00Z', value: 1 },
          { timestamp: '2026-05-20T01:00:00Z', value: 5 },
        ],
      },
      {
        name: 'b',
        points: [
          { timestamp: '2026-05-20T00:00:00Z', value: 2 },
          { timestamp: '2026-05-20T01:00:00Z', value: 3 },
        ],
      },
    ]
    const { container } = render(<TimeSeriesChart series={series} styles={styles} />)
    const paths = container.querySelectorAll('svg path')
    expect(paths).toHaveLength(2)
  })

  it('renders legend swatch + label per style', () => {
    const { container } = render(<TimeSeriesChart series={[]} styles={styles} />)
    const entries = container.querySelectorAll('.chart-legend .series')
    expect(entries).toHaveLength(2)
  })

  it('handles empty series gracefully', () => {
    const { container } = render(<TimeSeriesChart series={[]} styles={styles} />)
    expect(container.querySelector('svg')).not.toBeNull()
  })
})
