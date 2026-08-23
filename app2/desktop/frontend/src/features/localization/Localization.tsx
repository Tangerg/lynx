import {
  createContext,
  useContext,
  useLayoutEffect,
  useMemo,
  type ReactNode,
} from "react";

import { englishMessages } from "./messages/en";
import { localeDefinitions, type Locale, type TextDirection } from "./locales";
import { useShellPreferences } from "../preferences/ShellPreferences";
import type {
  MessageDictionary,
  MessageKey,
  MessageValues,
  Translate,
} from "./messages/types";

export type { MessageDictionary, MessageKey, MessageValues, Translate };

interface LocalizationContextValue {
  locale: Locale;
  direction: TextDirection;
  t: Translate;
  formatNumber(value: number, options?: Intl.NumberFormatOptions): string;
  formatDateTime(value: Date, options?: Intl.DateTimeFormatOptions): string;
}

export const translateEnglish: Translate = (key, values) =>
  interpolate(englishMessages[key], values);

const englishLocalization = createLocalization("en");
const LocalizationContext = createContext<LocalizationContextValue>(
  englishLocalization,
);

export function LocalizationProvider({ children }: { children: ReactNode }) {
  const { locale } = useShellPreferences();
  const localization = useMemo(() => createLocalization(locale), [locale]);

  useLayoutEffect(() => {
    const root = document.documentElement;
    root.lang = localization.locale;
    root.dir = localization.direction;
  }, [localization]);

  return (
    <LocalizationContext.Provider value={localization}>
      {children}
    </LocalizationContext.Provider>
  );
}

export function useLocalization() {
  return useContext(LocalizationContext);
}

function createLocalization(locale: Locale): LocalizationContextValue {
  const definition = localeDefinitions[locale];
  return {
    locale,
    direction: definition.direction,
    t: (key, values) => interpolate(definition.messages[key], values),
    formatNumber: (value, options) =>
      new Intl.NumberFormat(locale, options).format(value),
    formatDateTime: (value, options) =>
      new Intl.DateTimeFormat(locale, options).format(value),
  };
}

function interpolate(template: string, values?: MessageValues) {
  if (values === undefined) return template;
  return template.replaceAll(/\{([A-Za-z][A-Za-z0-9_]*)\}/g, (placeholder, name) =>
    Object.hasOwn(values, name) ? String(values[name]) : placeholder,
  );
}
