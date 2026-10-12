import { useDateFormatter, useLocale, useNumberFormatter } from "react-aria";
import { useMemo } from "react";

// Byte units follow the selected app language through React Aria's locale
// (bridged from LocaleProvider by AppI18nProvider), not the browser default.
// Intl picks the correct short unit per language (MB, Mo, МБ, ...).
const BYTE_UNITS = ["byte", "kilobyte", "megabyte", "gigabyte", "terabyte", "petabyte"] as const;

function normalizeSpace(value: string): string {
  return value.replace(/[\u00a0\u202f]/g, " ");
}

const unitCache = new Map<string, Intl.NumberFormat>();

function unitFormatter(
  locale: string,
  unit: (typeof BYTE_UNITS)[number],
  fractions: number,
): Intl.NumberFormat {
  const key = `${locale}|${unit}|${fractions}`;
  const cached = unitCache.get(key);
  if (cached) return cached;
  const formatter = new Intl.NumberFormat(locale, {
    style: "unit",
    unit,
    unitDisplay: "short",
    minimumFractionDigits: fractions,
    maximumFractionDigits: fractions,
  });
  unitCache.set(key, formatter);
  return formatter;
}

export function formatBytesWithLocale(locale: string, bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) {
    return normalizeSpace(unitFormatter(locale, "byte", 0).format(0));
  }
  const power = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), BYTE_UNITS.length - 1);
  const value = bytes / 1024 ** power;
  if (power === 0) return normalizeSpace(unitFormatter(locale, "byte", 0).format(value));
  const rounded = Math.round(value * 10) / 10;
  return normalizeSpace(unitFormatter(locale, BYTE_UNITS[power], 1).format(rounded));
}

// The unit varies per value, so one useNumberFormatter instance cannot cover
// every size; the locale still comes from React Aria's provider.
export function useFormatBytes(): (bytes: number) => string {
  const { locale } = useLocale();
  return useMemo(() => {
    return (bytes: number) => formatBytesWithLocale(locale, bytes);
  }, [locale]);
}

export function useCountFormatter(): Intl.NumberFormat {
  return useNumberFormatter();
}

// Matches the previous bare toLocaleString() shape in English
// ("1/5/2026, 3:04:33 PM") while following the selected language.
export function useDateTimeFormatter(): Intl.DateTimeFormat {
  return useDateFormatter({
    year: "numeric",
    month: "numeric",
    day: "numeric",
    hour: "numeric",
    minute: "2-digit",
    second: "2-digit",
  });
}

export function useDateOnlyFormatter(): Intl.DateTimeFormat {
  return useDateFormatter({ year: "numeric", month: "numeric", day: "numeric" });
}

export function useTimeOnlyFormatter(): Intl.DateTimeFormat {
  return useDateFormatter({ hour: "numeric", minute: "2-digit", second: "2-digit" });
}
