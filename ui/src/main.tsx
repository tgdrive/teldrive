import { QueryClientProvider } from "@tanstack/react-query";
import { createRouter, RouterProvider } from "@tanstack/react-router";
import ReactDOM from "react-dom/client";
import { I18nProvider } from "react-aria-components";
import { Toaster } from "sonner";
import { ThemeProvider, useTheme } from "@/lib/theme";
import { getQueryClient } from "./lib/queryClient";
import { routeTree } from "./routeTree.gen";
import "./styles/globals.css";

const queryClient = getQueryClient();

function ThemeToaster() {
  const { resolvedTheme } = useTheme();
  return (
    <Toaster
      position="bottom-right"
      richColors
      closeButton
      containerAriaLabel="Notificaciones"
      theme={resolvedTheme}
      toastOptions={{
        closeButtonAriaLabel: "Cerrar aviso",
        style: {
          background: "var(--background)",
          border: "1px solid var(--border)",
          color: "var(--foreground)",
          backdropFilter: "blur(16px)",
        },
      }}
    />
  );
}

const router = createRouter({
  routeTree,
  context: { queryClient },
  defaultPreload: "intent",
});

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}

async function startApp() {
  const rootElement = document.getElementById("root")!;

  if (!rootElement.innerHTML) {
    const root = ReactDOM.createRoot(rootElement);
    root.render(
      <QueryClientProvider client={queryClient}>
        <I18nProvider locale="es-ES">
          <ThemeProvider>
            <RouterProvider router={router} />
            <ThemeToaster />
          </ThemeProvider>
        </I18nProvider>
      </QueryClientProvider>,
    );
  }
}

startApp();
