import type { Locale } from "./locales";
import { de } from "./messages/de";
import { en, type MessageKey, type Messages } from "./messages/en";
import { fr } from "./messages/fr";
import { it } from "./messages/it";

const messages: Record<Locale, Messages> = { fr, de, it, en };

// The message for `key` in `locale`, with each {name} replaced by the value
// given for it. A placeholder without a value is left as it is, so that a
// missing value shows instead of vanishing.
export function translate(
  locale: Locale,
  key: MessageKey,
  values: Record<string, string> = {},
): string {
  return messages[locale][key].replace(
    /\{(\w+)\}/g,
    (placeholder, name: string) => values[name] ?? placeholder,
  );
}

// A function that translates into one language, as the pages and components
// call it: t("sheet.close").
export type Translate = (
  key: MessageKey,
  values?: Record<string, string>,
) => string;

export function translator(locale: Locale): Translate {
  return (key, values) => translate(locale, key, values);
}

// The names of the languages, each written in its own language, for the
// language switcher.
export const languageNames: Record<Locale, string> = {
  fr: "Français",
  de: "Deutsch",
  it: "Italiano",
  en: "English",
};
