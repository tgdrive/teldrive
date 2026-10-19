import catalog from "../../../localization/catalog.json";
import { STRINGS } from "./locales";
import { useAppLocale } from "./locale";

const originalKeys = new Map(Object.entries(STRINGS.en ?? {}).map(([key, value]) => [value, key]));
const catalogKeys = new Map(Object.entries(catalog.en).map(([key, value]) => [value, key]));

export const languages = [
  { id: "es", label: "Español", locale: "es-ES" },
  { id: "en", label: "English", locale: "en-US" },
  { id: "pt", label: "Português", locale: "pt-BR" },
  { id: "fr", label: "Français", locale: "fr-FR" },
  { id: "de", label: "Deutsch", locale: "de-DE" },
  { id: "it", label: "Italiano", locale: "it-IT" },
  { id: "ru", label: "Русский", locale: "ru-RU" },
  { id: "zh-CN", label: "简体中文", locale: "zh-CN" },
  { id: "ja", label: "日本語", locale: "ja-JP" },
  { id: "ar", label: "العربية", locale: "ar" },
] as const;
export type Language = (typeof languages)[number]["id"];
export function getLanguage(): Language {
  try {
    const saved = localStorage.getItem("teldrive-language");
    return languages.find((language) => language.id === saved)?.id ?? "es";
  } catch {
    return "es";
  }
}
export function t(source: string, values: unknown[] | Record<string, unknown> = []): string {
  const language = getLanguage();
  const dictionary = catalog[language] as Record<string, string>;
  const locale = language === "pt" ? "pt-BR" : language;
  const english = STRINGS.en?.[source] ?? source;
  const originalKey = STRINGS.en?.[source] ? source : originalKeys.get(source);
  const catalogKey = catalogKeys.get(english) ?? source;
  const text = dictionary[source] ?? dictionary[catalogKey] ??
    (originalKey ? STRINGS[locale]?.[originalKey] : undefined) ?? english;
  return text.replace(/\{(\w+)\}/g, (match, key: string) => {
    const value = Array.isArray(values) ? values[Number(key)] : values[key];
    return value === undefined ? match : String(value);
  });
}
export function applyLanguage() {
  const language = getLanguage();
  document.documentElement.lang = languages.find((value) => value.id === language)!.locale;
  document.documentElement.dir = language === "ar" ? "rtl" : "ltr";
}
export function setLanguage(language: string) {
  if (!languages.some((value) => value.id === language)) return;
  localStorage.setItem("teldrive-language", language);
  // Recreate static menu definitions as well as the current route. File paths,
  // credentials and user content never pass through the translation catalog.
  location.reload();
}
export function LanguageSelector() {
  const { setLocale } = useAppLocale();
  return (
    <label className="flex flex-wrap items-center gap-3 text-sm">
      {t("Idioma")}
      <select
        aria-label={t("Idioma")}
        className="min-h-11 max-w-full rounded-lg border border-border bg-field px-3 text-field-foreground"
        value={getLanguage()}
        onChange={(event) => {
          const language = event.target.value;
          const locale = language === "pt" ? "pt-BR" : language;
          void setLocale(locale as import("./locales").AppLocale).then(() => location.reload()).catch(() => {});
        }}
      >
        {languages.map((language) => (
          <option key={language.id} value={language.id}>
            {language.label}
          </option>
        ))}
      </select>
    </label>
  );
}
