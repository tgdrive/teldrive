import { createLazyFileRoute } from "@tanstack/react-router"

import { DriveFileBrowser } from "@/components/file-browser"
import { LifecycleView } from "@/components/lifecycle-view"

export const Route = createLazyFileRoute("/_authed/$view")({
  component: View,
})

function View() {
  const { view } = Route.useParams()
  return view === "trash" || view === "spam" ? <LifecycleView key={view} state={view} /> : <DriveFileBrowser />
}
