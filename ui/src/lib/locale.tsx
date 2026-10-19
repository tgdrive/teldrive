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
import { APP_LOCALES, type AppLocale, isAppLocale, translate } from "./locales";
import { hasCachedPreferences, usePreferences, useUpdatePreferences } from "./preferences";

export type { AppLocale };
export { APP_LOCALES };

// Outside an account this is the only locale source; signed-in pages use the
// server value and mirror it here for first paint.
const STORAGE_KEY = "teldrive-locale";

function detectLocale(): AppLocale {
  try {
    const custom = window.localStorage.getItem("teldrive-language") ?? "es";
    if (custom === "pt") return "pt-BR";
    if (isAppLocale(custom)) return custom;
    const stored = window.localStorage.getItem(STORAGE_KEY);
    if (isAppLocale(stored)) return stored;
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

function writeStoredLocale(locale: AppLocale) {
  try {
    window.localStorage.setItem(STORAGE_KEY, locale);
    window.localStorage.setItem("teldrive-language", locale === "pt-BR" ? "pt" : locale);
  } catch {
    // Ignore write failures; the locale still applies for this session.
  }
}

const LocaleContext = createContext<{
  locale: AppLocale;
  setLocale: (locale: AppLocale) => Promise<void>;
  t: (key: string, vars?: Record<string, string | number>) => string;
}>({
  locale: "es",
  setLocale: async () => {},
  t: (key) => key,
});

export function LocaleProvider({ children }: { children: ReactNode }) {
  const serverLocale = usePreferences()?.appearance.locale;
  const { mutateAsync: updatePreferences } = useUpdatePreferences();
  const [cachedLocale, setCachedLocale] = useState<AppLocale>(detectLocale);
  // An absent server locale means "auto-detect", so the cached/browser value applies.
  const locale = serverLocale ?? cachedLocale;

  useEffect(() => {
    document.documentElement.lang = locale;
    document.documentElement.dir = locale === "ar" ? "rtl" : "ltr";
    writeStoredLocale(locale);
  }, [locale]);

  // Keep the last server value so logout leaves the language in place.
  useEffect(() => {
    if (!serverLocale) return;
    setCachedLocale(serverLocale);
    writeStoredLocale(serverLocale);
  }, [serverLocale]);

  const setLocale = useCallback(
    async (next: AppLocale) => {
      setCachedLocale(next);
      writeStoredLocale(next);
      if (hasCachedPreferences()) await updatePreferences({ appearance: { locale: next } });
    },
    [updatePreferences],
  );

  const t = useCallback(
    (key: string, vars?: Record<string, string | number>) => translate(locale, key, vars),
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
