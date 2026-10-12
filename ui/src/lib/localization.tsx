import catalog from "../../../localization/catalog.json";

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
export function t(source: string, values: unknown[] = []): string {
  const dictionary = catalog[getLanguage()] as Record<string, string>;
  const text = dictionary[source] ?? source;
  return text.replace(/\{(\d+)\}/g, (match, index: string) =>
    Number(index) < values.length ? String(values[Number(index)]) : match,
  );
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
  return (
    <label className="flex flex-wrap items-center gap-3 text-sm">
      {t("Idioma")}
      <select
        aria-label={t("Idioma")}
        className="min-h-11 max-w-full rounded-lg border border-border bg-field px-3 text-field-foreground"
        value={getLanguage()}
        onChange={(event) => setLanguage(event.target.value)}
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
