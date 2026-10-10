import { createFileRoute, Navigate } from "@tanstack/react-router";

export const Route = createFileRoute("/_authed/")({
  component: () => (
    <Navigate from="/" to="./$view" params={{ view: "my-drive" }} search={{ path: "/" }} replace />
  ),
});
