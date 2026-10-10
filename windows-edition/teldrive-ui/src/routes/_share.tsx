import { ShareLayout } from "@/layouts/share-layout";
import { createFileRoute } from "@tanstack/react-router";

export const Route = createFileRoute("/_share")({
  component: ShareLayout,
});
