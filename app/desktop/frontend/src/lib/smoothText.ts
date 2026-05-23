// Smooth-text engine — decouples render cadence from backend chunk cadence.
//
// Why this exists: backend SSE chunks arrive irregularly (5-9 runes every
// 40-100ms in our mock; real models cluster bursts of tokens then pause).
// Rendering raw makes the fade-in feel like "word pop, char spit, word pop".
// Smoothing buffers the raw text into a queue and reveals it at a steady,
// adaptive rate so each fade-in has time to register before the next word
// arrives.
//
// Three improvements over the previous version (inspired by Proma's
// useSmoothStream and our own backlog observations):
//   - Three-tier adaptive rate based on remaining backlog (was two-tier).
//     A small backlog cruises at 40 chars/sec; mid-sized at 80; large at
//     160. That gives a more natural feel than the old binary 40/120
//     switch.
//   - Stream-end drain: when `enabled` flips false, instead of either
//     freezing at the slow rate (laggy) or snapping to full text (jarring),
//     we drain the remaining backlog at a rate proportional to its size —
//     small tail catches up smoothly, big tail catches up quickly.
//   - Time-based char-debt: each frame we add `rate × elapsed` to a debt
//     counter and consume whole words while debt ≥ next-word-length. Keeps
//     visible rate exactly on-target regardless of vsync jitter.

import { useEffect, useRef, useState } from "react";
import { segmentWords } from "./segmentWords";

// Rates in chars/sec. The visible cadence picks one based on backlog and
// whether the source is still streaming.
const RATE_CRUISE   = 40;   // backlog < 20, streaming = true
const RATE_MODERATE = 80;   // backlog in [20, 60)
const RATE_CATCHUP  = 160;  // backlog ≥ 60

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
    return Math.min(
      DRAIN_RATE_MAX,
      Math.max(DRAIN_RATE_MIN, backlog * DRAIN_RATE_PER_CHAR),
    );
  }
  if (backlog >= 60) return RATE_CATCHUP;
  if (backlog >= 20) return RATE_MODERATE;
  return RATE_CRUISE;
}

// useSmoothText paces `rawText` reveal to a steady, adaptive rhythm.
// Returns the visible prefix; consumers re-tokenize and render that. The
// reveal advances by whole words only, so the suffix never lands mid-word.
//
// `enabled` controls TWO things:
//   - initial displayLen (true → start at 0; false → start fully revealed)
//   - the live rate selection (false → drain mode, see pickRate)
//
// The rAF loop is always-on after mount: a block that transitions from
// streaming → finished mid-play continues draining gracefully; future text
// growth on the same block restarts the cruise.
export function useSmoothText(rawText: string, enabled: boolean): string {
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

      // Find the word boundary that aligns with the current displayLen.
      const words = st.words;
      let charCount = 0;
      let wordIdx = 0;
      while (wordIdx < words.length && charCount < st.displayLen) {
        charCount += words[wordIdx].length;
        wordIdx++;
      }

      const isFirstFrame = st.lastTickAt < 0;
      if (isFirstFrame) {
        // Cold start: emit at least one word so the bubble doesn't feel
        // like it's hung.
        st.lastTickAt = now;
        if (wordIdx < words.length) {
          charCount += words[wordIdx].length;
          wordIdx++;
        }
      } else {
        const elapsed = Math.min(now - st.lastTickAt, MAX_FRAME_STEP_MS);
        st.lastTickAt = now;
        const rate = pickRate(backlog, enabledRef.current);
        st.charDebt += rate * (elapsed / 1000);
        while (st.charDebt >= 1 && wordIdx < words.length) {
          charCount += words[wordIdx].length;
          st.charDebt -= words[wordIdx].length;
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
