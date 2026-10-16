import { createContext, type ReactNode, useCallback, useContext, useEffect, useState } from "react";
import { hasCachedPreferences, usePreferences, useUpdatePreferences } from "@/lib/preferences";

type ResolvedTheme = "light" | "dark";
type Theme = ResolvedTheme | "system";

// Same storage key as the next-themes default, so existing browser
// preferences carry over. Outside an account this is the only theme source;
// signed-in pages use the server value and mirror it here for first paint.
const STORAGE_KEY = "theme";

const ThemeContext = createContext<{
  theme: Theme;
  resolvedTheme: ResolvedTheme;
  setTheme: (theme: Theme) => void;
}>({
  theme: "system",
  resolvedTheme: "light",
  setTheme: () => {},
});

function applyTheme(theme: ResolvedTheme) {
  const root = document.documentElement;
  root.classList.toggle("dark", theme === "dark");
  root.dataset.theme = theme;
  root.style.colorScheme = theme;
}

function initialTheme(): Theme {
  try {
    const stored = window.localStorage.getItem(STORAGE_KEY);
    if (stored === "light" || stored === "dark" || stored === "system") return stored;
  } catch {
    // Private mode etc: fall through to the default.
  }
  return "system";
}

function writeStoredTheme(theme: Theme) {
  try {
    window.localStorage.setItem(STORAGE_KEY, theme);
  } catch {
    // Ignore write failures; the theme still applies for this session.
  }
}

export function ThemeProvider({ children }: { children: ReactNode }) {
  const serverTheme = usePreferences()?.appearance.theme;
  const { mutate: updatePreferences } = useUpdatePreferences();
  const [cachedTheme, setCachedTheme] = useState<Theme>(initialTheme);
  const theme = serverTheme ?? cachedTheme;
  const [systemTheme, setSystemTheme] = useState<ResolvedTheme>(() =>
    window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light",
  );
  const resolvedTheme = theme === "system" ? systemTheme : theme;

  useEffect(() => {
    const media = window.matchMedia("(prefers-color-scheme: dark)");
    const update = () => setSystemTheme(media.matches ? "dark" : "light");
    update();
    media.addEventListener("change", update);
    return () => media.removeEventListener("change", update);
  }, []);

  useEffect(() => {
    applyTheme(resolvedTheme);
  }, [resolvedTheme]);

  // Keep the last server value so logout leaves the theme in place.
  useEffect(() => {
    if (!serverTheme) return;
    setCachedTheme(serverTheme);
    writeStoredTheme(serverTheme);
  }, [serverTheme]);

  const setTheme = useCallback(
    (next: Theme) => {
      setCachedTheme(next);
      writeStoredTheme(next);
      if (hasCachedPreferences()) updatePreferences({ appearance: { theme: next } });
    },
    [updatePreferences],
  );

  return (
    <ThemeContext.Provider value={{ theme, resolvedTheme, setTheme }}>
      {children}
    </ThemeContext.Provider>
  );
}

export function useTheme() {
  return useContext(ThemeContext);
}
