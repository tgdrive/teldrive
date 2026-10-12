import { I18nProvider } from "@heroui/react";
import {
  createContext,
  type ReactNode,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
} from "react";
import { APP_LOCALES, STRINGS, type AppLocale } from "./locales";

export type { AppLocale };
export { APP_LOCALES };

const STORAGE_KEY = "teldrive-locale";

function detectLocale(): AppLocale {
  try {
    const stored = window.localStorage.getItem(STORAGE_KEY);
    if (stored && (STRINGS as Record<string, unknown>)[stored]) return stored as AppLocale;
  } catch {
    // Private mode etc: fall through to browser detection.
  }
  const language = window.navigator.language.toLowerCase();
  if (language.startsWith("pt")) return "pt-BR";
  if (language.startsWith("es")) return "es";
  if (language.startsWith("ru")) return "ru";
  if (language.startsWith("fr")) return "fr";
  if (language.startsWith("de")) return "de";
  if (language.startsWith("zh")) return "zh-CN";
  if (language.startsWith("en")) return "en";
  return "en";
}

const LocaleContext = createContext<{
  locale: AppLocale;
  setLocale: (locale: AppLocale) => void;
  t: (key: string, vars?: Record<string, string | number>) => string;
}>({
  locale: "en",
  setLocale: () => {},
  t: (key) => key,
});

export function LocaleProvider({ children }: { children: ReactNode }) {
  const [locale, setLocaleState] = useState<AppLocale>(detectLocale);

  useEffect(() => {
    document.documentElement.lang = locale;
  }, [locale]);

  const setLocale = useCallback((next: AppLocale) => {
    setLocaleState(next);
    try {
      window.localStorage.setItem(STORAGE_KEY, next);
    } catch {
      // Ignore write failures; the locale still applies for this session.
    }
  }, []);

  const t = useCallback(
    (key: string, vars?: Record<string, string | number>) => {
      const template = STRINGS[locale][key] ?? STRINGS.en[key] ?? key;
      if (!vars) return template;
      return template.replace(/\{(\w+)\}/g, (_, name: string) =>
        vars[name] === undefined ? `{${name}}` : String(vars[name]),
      );
    },
    [locale],
  );

  const value = useMemo(() => ({ locale, setLocale, t }), [locale, setLocale, t]);
  return <LocaleContext.Provider value={value}>{children}</LocaleContext.Provider>;
}

// Named to avoid clashing with react-aria's useLocale.
export function useAppLocale() {
  return useContext(LocaleContext);
}

// Bridges the app locale into HeroUI / React Aria so calendars, date and
// number formatting follow the selected language.
export function AppI18nProvider({ children }: { children: ReactNode }) {
  const { locale } = useAppLocale();
  return <I18nProvider locale={locale}>{children}</I18nProvider>;
}
