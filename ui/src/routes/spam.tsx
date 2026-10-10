import { createFileRoute } from "@tanstack/react-router";
import { QuarantinePage } from "./trash";
export const Route = createFileRoute("/spam")({ component: () => <QuarantinePage spam /> });
