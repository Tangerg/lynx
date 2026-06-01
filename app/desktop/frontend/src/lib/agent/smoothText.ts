// Smooth-text engine — paces token reveal at a steady rate so each
// per-word fade-in has time to register, regardless of how irregularly
// the backend chunks arrive. Three-tier adaptive rate based on backlog
// + drain mode when the stream ends + time-based char-debt accounting
// so the visible cadence stays on-target through vsync jitter.

import { useEffect, useRef, useState } from "react";
import { segmentWords } from "@/lib/i18n/segmentWords";

// Rates in chars/sec. The visible cadence picks one based on backlog and
// whether the source is still streaming.
const RATE_CRUISE = 40; // backlog < 20, streaming = true
const RATE_MODERATE = 80; // backlog in [20, 60)
const RATE_CATCHUP = 160; // backlog ≥ 60

const SENTENCE_PAUSE_MS = 80;
const MAX_DEBT = 12;
const MAX_FRAME_STEP_MS = 64;

// Drain-mode rate when streaming = false. Scales with backlog so a tiny
// tail (a few words) doesn't feel slow but a big tail (a missed paragraph)
// doesn't blast onto the screen all at once. Capped so it never feels like
// a dump.
const DRAIN_RATE_MIN = 80;
const DRAIN_RATE_MAX = 280;
const DRAIN_RATE_PER_CHAR = 8;

const SENTENCE_END_RE = /[。！？…!?.]$/;

// Exported so tests can pin the rate selection. Not part of the public
// API otherwise — keep imports through `useSmoothText` from app code.
export function pickRate(backlog: number, streaming: boolean): number {
  if (!streaming) {
    // Drain mode — proportional to remaining backlog, clamped.
    return Math.min(DRAIN_RATE_MAX, Math.max(DRAIN_RATE_MIN, backlog * DRAIN_RATE_PER_CHAR));
  }
  if (backlog >= 60) return RATE_CATCHUP;
  if (backlog >= 20) return RATE_MODERATE;
  return RATE_CRUISE;
}

// Reveals `rawText` at the adaptive rate from pickRate. `enabled` controls
// both the initial state (false → start fully revealed) and the live rate
// (false → drain mode). `typewriter` switches the reveal granularity from
// whole-word + per-word fade (smooth, the default) to crisp character-by-
// character with no sentence pauses (a steady terminal cadence). The rAF loop
// stays mounted so stream-end transitions drain gracefully.
export function useSmoothText(rawText: string, enabled: boolean, typewriter = false): string {
  const initialLen = enabled ? 0 : rawText.length;
  const [displayLen, setDisplayLen] = useState(initialLen);

  // Live state in refs — the rAF tick reads the freshest values without
  // re-subscribing the effect.
  const stateRef = useRef({
    rawText: "",
    words: [] as string[],
    displayLen: initialLen,
    lastTickAt: -1,
    charDebt: 0,
    pauseUntil: 0,
  });

  // Mirror the enabled flag into a ref so the rAF closure picks up the
  // latest value without re-subscribing.
  const enabledRef = useRef(enabled);
  enabledRef.current = enabled;

  const typewriterRef = useRef(typewriter);
  typewriterRef.current = typewriter;

  // Sync rawText into refs on each render. Re-segment only when text
  // actually changed.
  if (stateRef.current.rawText !== rawText) {
    stateRef.current.rawText = rawText;
    stateRef.current.words = segmentWords(rawText);
    if (stateRef.current.displayLen > rawText.length) {
      stateRef.current.displayLen = rawText.length;
    }
  }

  useEffect(() => {
    let rafId = 0;
    let active = true;

    const tick = () => {
      if (!active) return;
      const st = stateRef.current;
      const backlog = st.rawText.length - st.displayLen;

      if (backlog <= 0) {
        // Idle. Reset rate state so the next text-growth event starts a
        // clean cadence (no stale lastTickAt → giant elapsed → debt dump).
        st.lastTickAt = -1;
        st.charDebt = 0;
        rafId = requestAnimationFrame(tick);
        return;
      }

      const now = performance.now();
      if (now < st.pauseUntil) {
        rafId = requestAnimationFrame(tick);
        return;
      }

      // Typewriter cadence — reveal individual characters at the same
      // backlog-adaptive rate, no word snapping, no sentence pauses. The
      // crisp char-by-char appear IS the effect (the caller drops the
      // per-word fade-in), so it reads as a steady terminal type-out.
      if (typewriterRef.current) {
        let reveal: number;
        if (st.lastTickAt < 0) {
          st.lastTickAt = now;
          reveal = 1; // cold start: emit one char so it doesn't look hung
        } else {
          const elapsed = Math.min(now - st.lastTickAt, MAX_FRAME_STEP_MS);
          st.lastTickAt = now;
          st.charDebt += pickRate(backlog, enabledRef.current) * (elapsed / 1000);
          reveal = Math.floor(st.charDebt);
          st.charDebt = Math.max(0, Math.min(st.charDebt - reveal, MAX_DEBT));
        }
        const newLen = Math.min(st.displayLen + reveal, st.rawText.length);
        if (newLen !== st.displayLen) {
          st.displayLen = newLen;
          setDisplayLen(newLen);
        }
        rafId = requestAnimationFrame(tick);
        return;
      }

      // Find the word boundary that aligns with the current displayLen.
      const words = st.words;
      let charCount = 0;
      let wordIdx = 0;
      while (wordIdx < words.length && charCount < st.displayLen) {
        charCount += words[wordIdx]!.length;
        wordIdx++;
      }

      const isFirstFrame = st.lastTickAt < 0;
      if (isFirstFrame) {
        // Cold start: emit at least one word so the bubble doesn't feel
        // like it's hung.
        st.lastTickAt = now;
        if (wordIdx < words.length) {
          charCount += words[wordIdx]!.length;
          wordIdx++;
        }
      } else {
        const elapsed = Math.min(now - st.lastTickAt, MAX_FRAME_STEP_MS);
        st.lastTickAt = now;
        const rate = pickRate(backlog, enabledRef.current);
        st.charDebt += rate * (elapsed / 1000);
        while (st.charDebt >= 1 && wordIdx < words.length) {
          const w = words[wordIdx]!;
          charCount += w.length;
          st.charDebt -= w.length;
          wordIdx++;
        }
        st.charDebt = Math.max(0, Math.min(st.charDebt, MAX_DEBT));
      }

      const lastWord = wordIdx > 0 ? words[wordIdx - 1] : "";
      const newLen = Math.min(charCount, st.rawText.length);
      if (newLen !== st.displayLen) {
        st.displayLen = newLen;
        setDisplayLen(newLen);
      }
      // Sentence-end breathing — only honour while still streaming. During
      // drain we want to flush without artificial pauses.
      if (enabledRef.current && lastWord && SENTENCE_END_RE.test(lastWord.trimEnd())) {
        st.pauseUntil = now + SENTENCE_PAUSE_MS;
      }

      rafId = requestAnimationFrame(tick);
    };

    rafId = requestAnimationFrame(tick);

    return () => {
      active = false;
      cancelAnimationFrame(rafId);
    };
  }, []);

  return rawText.slice(0, displayLen);
}
