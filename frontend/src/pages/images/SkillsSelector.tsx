import { useSkills } from '../../api/skills'
import { DualListPicker } from '../../primitives/DualListPicker'
import { Panel } from '../../primitives/Panel'

type SkillsSelectorProps = {
  enabledSkills: string[]
  onChange: (ids: string[]) => void
}

export function SkillsSelector({ enabledSkills, onChange }: SkillsSelectorProps) {
  const { data, isLoading } = useSkills({ pageSize: 200 })
  const skills = data?.items ?? []
  return (
    <div style={{ marginTop: 24 }}>
      <Panel title="Skills">
        <DualListPicker
          items={skills}
          selectedIds={enabledSkills}
          onChange={onChange}
          getId={(s) => s.id}
          getLabel={(s) => s.name}
          getDescription={(s) => s.description}
          isLoading={isLoading}
          emptyLabel="No skills available. Create skills first to enable them on this image."
          enabledTitle="Enabled on this image"
        />
      </Panel>
    </div>
  )
}
