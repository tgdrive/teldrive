import { genThemeConfig } from "@tw-material/theme/config";
import { createContext, useContext, useEffect, useMemo } from "react";
import { useLocalStorage } from "usehooks-ts";

type Theme = "dark" | "light" | "system";

type ColorScheme = {
  color: string;
  preset?: string;
  cssVars?: Record<string, string>;
};

type ThemeProviderProps = {
  children: React.ReactNode;
  defaultTheme?: Theme;
  storageKey?: string;
};

type ThemeProviderState = {
  colorScheme: ColorScheme;
  theme: Theme;
  setTheme: (theme: Theme) => void;
  setColorScheme: (colorScheme: ColorScheme) => void;
  density: "comfortable" | "compact";
  setDensity: (density: "comfortable" | "compact") => void;
};

export const defaultColorScheme: ColorScheme = {
  color: "#1a73e8",
  preset: "drive",
};

const initialState: ThemeProviderState = {
  colorScheme: defaultColorScheme,
  theme: "system",
  setTheme: () => null,
  setColorScheme: () => null,
  density: "comfortable",
  setDensity: () => null,
};

const ThemeProviderContext = createContext<ThemeProviderState>(initialState);

const sheet = typeof CSSStyleSheet !== "undefined" && "replaceSync" in CSSStyleSheet.prototype ? new CSSStyleSheet() : null;

function applyThemeRules(rules: string[]) {
  if (sheet && "adoptedStyleSheets" in document) {
    sheet.replaceSync(rules.join("\n"));
    if (!document.adoptedStyleSheets.includes(sheet)) document.adoptedStyleSheets = [...document.adoptedStyleSheets, sheet];
    return;
  }
  let style = document.getElementById("teldrive-theme");
  if (!style) {
    style = document.createElement("style");
    style.id = "teldrive-theme";
    document.head.appendChild(style);
  }
  style.textContent = rules.join("\n");
}

export function ThemeProvider({
  children,
  defaultTheme = "system",
  storageKey = "theme",
  ...props
}: ThemeProviderProps) {
  const [colorScheme, setColorScheme] = useLocalStorage<ColorScheme>(
    "colorScheme",
    defaultColorScheme,
  );

  const [theme, setTheme] = useLocalStorage<Theme>(storageKey, defaultTheme);
  const [density, setDensity] = useLocalStorage<"comfortable" | "compact">("ui-density", "comfortable");

  useEffect(() => {
    document.documentElement.dataset.uiTheme = colorScheme.preset || "custom";
    document.documentElement.dataset.density = density;
  }, [colorScheme.preset, density]);

  useEffect(() => {
    const root = window.document.documentElement;
    const media = window.matchMedia("(prefers-color-scheme: dark)");
    function updateTheme() {
      root.classList.remove("light", "dark");
      root.classList.add(theme === "system" ? (media.matches ? "dark" : "light") : theme);
    }
    updateTheme();
    if (theme !== "system") return;
    media.addEventListener("change", updateTheme);
    return () => media.removeEventListener("change", updateTheme);
  }, [theme]);

  // Handle Dynamic Theme Generation & Caching
  useEffect(() => {
    if (!colorScheme.color) return;

    // Apply cached CSS if available to avoid regeneration on every reload
    if (colorScheme.cssVars) {
      const rules = Object.entries(colorScheme.cssVars).map(([key, val]) => `${key}{${val}}`);
      applyThemeRules(rules);
      return;
    }

    // Otherwise generate new theme config
    const config = genThemeConfig({
      sourceColor: colorScheme.color,
      customColors: [],
    });

    const cssVars: Record<string, string> = {};
    const rules: string[] = [];
    for (const key in config.utilities) {
      const value = Object.entries(config.utilities[key]).reduce(
        (acc, val) => `${acc}${val[0]}:${val[1]};`,
        "",
      );
      cssVars[key] = value;
      rules.push(`${key}{${value}}`);
    }

    applyThemeRules(rules);

    // Persist the generated vars so they can be reused on next reload
    setColorScheme({ color: colorScheme.color, preset: colorScheme.preset, cssVars });
  }, [colorScheme.color, colorScheme.preset, colorScheme.cssVars, setColorScheme]);

  const value = useMemo(
    () => ({
      theme,
      setTheme,
      colorScheme,
      setColorScheme,
      density,
      setDensity,
    }),
    [theme, setTheme, colorScheme, setColorScheme, density, setDensity],
  );

  return (
    <ThemeProviderContext.Provider {...props} value={value}>
      {children}
    </ThemeProviderContext.Provider>
  );
}

export const useTheme = () => {
  const context = useContext(ThemeProviderContext);

  if (context === undefined) {
    throw new Error("useTheme must be used within a ThemeProvider");
  }

  return context;
};
