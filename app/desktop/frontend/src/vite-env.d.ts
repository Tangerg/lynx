/// <reference types="vite/client" />

// @total-typescript/ts-reset — tightens TS stdlib types:
//   - .filter(Boolean) narrows correctly (kills the tab-strip filter bug
//     class for good — pre-reset, [string|null].filter(Boolean) stayed
//     (string | null)[] and silently dropped nulls without type help)
//   - JSON.parse() returns unknown not any (forces Zod / type guards)
//   - Array.isArray() narrows readonly arrays correctly
//   - fetch().json() returns unknown not any
import "@total-typescript/ts-reset";
