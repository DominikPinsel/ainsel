import { useParams } from 'react-router-dom'
import { ImageFormContainer } from './ImageFormContainer'

export function ImageDetailPage() {
  const { id } = useParams<{ id?: string }>()
  return <ImageFormContainer id={id} />
}
