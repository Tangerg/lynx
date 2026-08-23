import {
  createContext,
  useContext,
  useLayoutEffect,
  useMemo,
  type ReactNode,
} from "react";

import { englishMessages } from "./messages/en";

export type MessageKey = keyof typeof englishMessages;
export type MessageValues = Readonly<Record<string, string | number>>;
export type Translate = (key: MessageKey, values?: MessageValues) => string;

interface LocalizationContextValue {
  locale: "en";
  direction: "ltr";
  t: Translate;
  formatNumber(value: number): string;
  formatDateTime(value: Date, options?: Intl.DateTimeFormatOptions): string;
}

export const translateEnglish: Translate = (key, values) =>
  interpolate(englishMessages[key], values);

const englishLocalization = createLocalization();
const LocalizationContext = createContext<LocalizationContextValue>(
  englishLocalization,
);

export function LocalizationProvider({ children }: { children: ReactNode }) {
  const localization = useMemo(createLocalization, []);

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

function createLocalization(): LocalizationContextValue {
  const locale = "en" as const;
  return {
    locale,
    direction: "ltr",
    t: translateEnglish,
    formatNumber: (value) => new Intl.NumberFormat(locale).format(value),
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
