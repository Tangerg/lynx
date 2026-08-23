import type { englishMessages } from "./en";

export type MessageKey = keyof typeof englishMessages;
export type MessageValues = Readonly<Record<string, string | number>>;
export type MessageDictionary = Record<MessageKey, string>;
export type Translate = (key: MessageKey, values?: MessageValues) => string;
