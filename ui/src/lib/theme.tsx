import { createContext, useCallback, useContext, useEffect, useState, type ReactNode } from "react";

type Theme = "light" | "dark";
export type ThemeMode = Theme | "system";
export const visualThemes = [
  { id: "drive", label: "Azul Drive", color: "#1a73e8", background: "#f6f8fc" },
  { id: "mega", label: "Rojo Mega", color: "#c62828", background: "#fff7f7" },
  { id: "forest", label: "Bosque", color: "#237843", background: "#f3f8f4" },
  { id: "violet", label: "Violeta", color: "#7c3aed", background: "#f8f5ff" },
  { id: "ocean", label: "Océano", color: "#007c91", background: "#f1f9fb" },
  { id: "amber", label: "Ámbar", color: "#a16207", background: "#fffaf0" },
] as const;
type Palette = (typeof visualThemes)[number]["id"];
type Preferences = { mode: ThemeMode; palette: Palette; customColor: string; density: "comfortable" | "compact"; storageLimitGiB: number | null };
const defaults: Preferences = { mode: "system", palette: "drive", customColor: "", density: "comfortable", storageLimitGiB: null };
const ThemeContext = createContext({ ...defaults, resolvedTheme: "light" as Theme, setTheme: (_: ThemeMode) => {}, setPreferences: (_: Partial<Preferences>) => {} });

function readPreferences(): Preferences {
  try {
    const saved = JSON.parse(localStorage.getItem("teldrive-v2-appearance") || "{}");
    return {
      mode: ["light", "dark", "system"].includes(saved.mode) ? saved.mode : defaults.mode,
      palette: visualThemes.some(theme => theme.id === saved.palette) ? saved.palette : defaults.palette,
      customColor: /^#[\da-f]{6}$/i.test(saved.customColor || "") ? saved.customColor : "",
      density: saved.density === "compact" ? "compact" : "comfortable",
      storageLimitGiB: Number.isFinite(saved.storageLimitGiB) && saved.storageLimitGiB > 0 ? saved.storageLimitGiB : null,
    };
  } catch { return defaults; }
}

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [preferences, update] = useState(readPreferences);
  const [systemDark, setSystemDark] = useState(() => matchMedia("(prefers-color-scheme: dark)").matches);
  const resolvedTheme: Theme = preferences.mode === "system" ? (systemDark ? "dark" : "light") : preferences.mode;
  useEffect(() => {
    const media = matchMedia("(prefers-color-scheme: dark)");
    const changed = () => setSystemDark(media.matches);
    media.addEventListener("change", changed);
    return () => media.removeEventListener("change", changed);
  }, []);
  useEffect(() => {
    const root = document.documentElement;
    const selected = visualThemes.find(theme => theme.id === preferences.palette) || visualThemes[0];
    const accent = preferences.customColor || selected.color;
    const rgb = [1, 3, 5].map(offset => parseInt(accent.slice(offset, offset + 2), 16) / 255).map(value => value <= 0.04045 ? value / 12.92 : ((value + 0.055) / 1.055) ** 2.4);
    root.classList.toggle("dark", resolvedTheme === "dark");
    root.dataset.theme = resolvedTheme;
    root.dataset.density = preferences.density;
    root.style.colorScheme = resolvedTheme;
    root.style.setProperty("--accent", accent);
    root.style.setProperty("--accent-foreground", rgb[0]! * 0.2126 + rgb[1]! * 0.7152 + rgb[2]! * 0.0722 > 0.4 ? "#172033" : "#ffffff");
    root.style.setProperty("--background", resolvedTheme === "dark" ? "#10141c" : selected.background);
    root.style.setProperty("--surface", resolvedTheme === "dark" ? "#191f2b" : "#ffffff");
    root.style.setProperty("--sidebar", resolvedTheme === "dark" ? "#151b25" : selected.background);
    try { localStorage.setItem("teldrive-v2-appearance", JSON.stringify(preferences)); localStorage.setItem("theme", resolvedTheme); } catch { /* Session preferences still apply. */ }
  }, [preferences, resolvedTheme]);
  const setPreferences = useCallback((next: Partial<Preferences>) => update(previous => ({ ...previous, ...next })), []);
  const setTheme = useCallback((mode: ThemeMode) => update(previous => ({ ...previous, mode })), []);
  return <ThemeContext.Provider value={{ ...preferences, resolvedTheme, setTheme, setPreferences }}>{children}</ThemeContext.Provider>;
}
export function useTheme() { return useContext(ThemeContext); }
