import { useAgents } from '../../api/agents'
import { useConnectors } from '../../api/connectors'
import { useObservabilitySummary } from '../../api/observability'
import { useTriggers } from '../../api/triggers'
import { Titleblock } from '../../layout/Titleblock'
import { ConnectorRegister } from './ConnectorRegister'
import { KpiRow } from './KpiRow'
import { RecentActivityStream } from './RecentActivityStream'
import { ThroughputChart } from './ThroughputChart'
import './Dashboard.css'

export function Dashboard() {
  const agents = useAgents({ pageSize: 1 })
  const connectors = useConnectors({ pageSize: 1 })
  const triggers = useTriggers({ pageSize: 1 })
  const summary = useObservabilitySummary()

  const errorCount = summary.data?.routingErrors ?? 0

  const plates = [
    {
      key: 'agents',
      label: 'Agents',
      code: 'AG·01',
      to: '/agents',
      count: agents.data?.total,
      sub: 'Active AI agents',
      spark: [12, 14, 13, 14, 14, 14],
    },
    {
      key: 'connectors',
      label: 'Connectors',
      code: 'CN·02',
      to: '/connectors',
      count: connectors.data?.total,
      sub: 'Platform connectors',
      spark: [6, 6, 6, 6, 6, 6],
    },
    {
      key: 'triggers',
      label: 'Triggers',
      code: 'TR·03',
      to: '/triggers',
      count: triggers.data?.total,
      sub: 'Routing rules',
      spark: [18, 20, 21, 22, 23, 23],
    },
    {
      key: 'errors',
      label: 'Errors · 1h',
      code: 'ER·04',
      to: '/error-log',
      count: summary.data?.routingErrors,
      sub: errorCount > 0 ? 'Recent errors — investigate' : 'No recent errors',
      spark: [0, 1, 0, 2, 1, errorCount],
      alert: errorCount > 0,
    },
  ]

  return (
    <>
      <Titleblock
        crumbs={
          <>
            Operations / <b>Dashboard</b>
          </>
        }
        title={
          <>
            Operations <em>Console</em>
          </>
        }
      />
      <div className="dashboard-page">
        <KpiRow plates={plates} />
        <div className="dashboard-grid">
          <ConnectorRegister />
          <div className="dashboard-grid-right">
            <ThroughputChart />
            <RecentActivityStream />
          </div>
        </div>
      </div>
    </>
  )
}
