import { createLazyFileRoute } from "@tanstack/react-router";

import { Login } from "@/components/login";

export const Route = createLazyFileRoute("/_auth/login")({
  component: Login,
});
