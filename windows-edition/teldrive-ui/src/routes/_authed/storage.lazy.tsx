import { createLazyFileRoute } from "@tanstack/react-router"

import { StorageView } from "@/components/storage-view"

export const Route = createLazyFileRoute("/_authed/storage")({
  component: StorageView,
})
