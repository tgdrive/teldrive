import { t } from "@/lib/localization";
import { QueryClientProvider } from "@tanstack/react-query";
import { createRouter, RouterProvider } from "@tanstack/react-router";
import ReactDOM from "react-dom/client";
import { Toaster } from "sonner";
import { ThemeProvider, useTheme } from "@/lib/theme";
import { applyLanguage } from "@/lib/localization";
import { LocaleProvider, AppI18nProvider } from "@/lib/locale";
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
      containerAriaLabel={t("Notificaciones")}
      theme={resolvedTheme}
      toastOptions={{
        closeButtonAriaLabel: t("Cerrar aviso"),
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
  applyLanguage();
  const rootElement = document.getElementById("root")!;

  if (!rootElement.innerHTML) {
    const root = ReactDOM.createRoot(rootElement);
    root.render(
      <QueryClientProvider client={queryClient}>
        <LocaleProvider><AppI18nProvider>
          <ThemeProvider>
            <RouterProvider router={router} />
            <ThemeToaster />
          </ThemeProvider>
        </AppI18nProvider></LocaleProvider>
      </QueryClientProvider>,
    );
  }
}

startApp();
