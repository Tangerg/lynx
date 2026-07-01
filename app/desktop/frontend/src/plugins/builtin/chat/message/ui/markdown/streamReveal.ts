// Stream-reveal engine — paces assistant token reveal at a steady, backlog-
// adaptive rate regardless of how irregularly the backend chunks arrive. One
// rAF loop + time-based char-debt accounting drives two reveal modes that share
// the same cadence math and differ only in their per-frame advance:
//
//   - smooth     — reveals whole words and breathes at sentence ends, so each
//                  per-word fade-in (applied by the caller) has time to land.
//   - typewriter — reveals raw characters, no snapping, no pauses: a steady
//                  terminal type-out (the caller drops the fade + adds a caret).
//
// Three-tier rate by backlog + a drain mode when the stream ends keep the
// visible cadence on-target through vsync jitter and bursty chunking.

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

// Exported so tests can pin the rate selection. Not part of the markdown
// package surface otherwise.
export function pickRate(backlog: number, streaming: boolean): number {
  if (!streaming) {
    // Drain mode — proportional to remaining backlog, clamped.
    return Math.min(DRAIN_RATE_MAX, Math.max(DRAIN_RATE_MIN, backlog * DRAIN_RATE_PER_CHAR));
  }
  if (backlog >= 60) return RATE_CATCHUP;
  if (backlog >= 20) return RATE_MODERATE;
  return RATE_CRUISE;
}

// Reveals `rawText` at the adaptive rate from pickRate. `enabled` controls both
// the initial state (false → start fully revealed) and the live rate (false →
// drain mode). `typewriter` switches the per-frame advance from whole-word +
// sentence pauses (smooth, the default) to raw character-by-character. The rAF
// loop stays mounted so stream-end transitions drain gracefully.
export function useStreamReveal(rawText: string, enabled: boolean, typewriter = false): string {
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

  // Mirror the live flags into refs so the rAF closure picks up the latest
  // values without re-subscribing.
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

  // rAF bookkeeping. 0 = parked. The loop PARKS at zero backlog instead of
  // self-rescheduling forever: every mounted message owns one of these hooks,
  // so a long session would otherwise keep hundreds of 60fps callbacks
  // spinning while fully idle. The rawText-keyed effect below re-arms the
  // loop on growth — same next-frame reveal latency as the perpetual loop.
  const rafRef = useRef(0);
  const armRef = useRef(() => {});

  useEffect(() => {
    const tick = () => {
      rafRef.current = 0;
      const st = stateRef.current;
      const backlog = st.rawText.length - st.displayLen;

      if (backlog <= 0) {
        // Park. Reset rate state so the next text-growth event starts a
        // clean cadence (no stale lastTickAt → giant elapsed → debt dump).
        st.lastTickAt = -1;
        st.charDebt = 0;
        return;
      }

      const now = performance.now();
      if (now < st.pauseUntil) {
        rafRef.current = requestAnimationFrame(tick);
        return;
      }

      if (st.lastTickAt < 0) {
        // Cold start: seed one unit so the first frame isn't empty (no elapsed
        // to integrate yet). The advance below consumes it — one word / char.
        st.lastTickAt = now;
        st.charDebt = 1;
      } else {
        const elapsed = Math.min(now - st.lastTickAt, MAX_FRAME_STEP_MS);
        st.lastTickAt = now;
        st.charDebt += pickRate(backlog, enabledRef.current) * (elapsed / 1000);
      }

      let newLen: number;
      let lastWord = "";
      if (typewriterRef.current) {
        // Raw characters: spend the integer part of the debt.
        const reveal = Math.floor(st.charDebt);
        st.charDebt -= reveal;
        newLen = st.displayLen + reveal;
      } else {
        // Whole words: walk to the current boundary, then reveal words while
        // the debt covers them (debt paid in characters).
        const words = st.words;
        let charCount = 0;
        let wordIdx = 0;
        while (wordIdx < words.length && charCount < st.displayLen) {
          charCount += words[wordIdx]!.length;
          wordIdx++;
        }
        while (st.charDebt >= 1 && wordIdx < words.length) {
          const w = words[wordIdx]!;
          charCount += w.length;
          st.charDebt -= w.length;
          wordIdx++;
        }
        lastWord = wordIdx > 0 ? words[wordIdx - 1]! : "";
        newLen = charCount;
      }
      st.charDebt = Math.max(0, Math.min(st.charDebt, MAX_DEBT));

      newLen = Math.min(newLen, st.rawText.length);
      // Don't stop between a surrogate pair — typewriter mode spends raw UTF-16
      // units, so a mid-emoji boundary would slice a lone high surrogate (renders
      // "�" for a frame). Pull in the low half.
      if (newLen > st.displayLen && newLen < st.rawText.length) {
        const code = st.rawText.charCodeAt(newLen - 1);
        if (code >= 0xd800 && code <= 0xdbff) newLen += 1;
      }
      if (newLen !== st.displayLen) {
        st.displayLen = newLen;
        setDisplayLen(newLen);
      }

      // Sentence-end breathing — smooth mode only (lastWord is "" in
      // typewriter), and only while still streaming (drain flushes clean).
      if (lastWord && enabledRef.current && SENTENCE_END_RE.test(lastWord.trimEnd())) {
        st.pauseUntil = now + SENTENCE_PAUSE_MS;
      }

      rafRef.current = requestAnimationFrame(tick);
    };

    armRef.current = () => {
      if (rafRef.current === 0) rafRef.current = requestAnimationFrame(tick);
    };
    armRef.current(); // initial backlog (enabled mount) starts revealing

    return () => {
      if (rafRef.current !== 0) cancelAnimationFrame(rafRef.current);
      rafRef.current = 0;
    };
  }, []);

  // Re-arm on every text growth (one cheap effect per delta on the single
  // streaming message; settled messages never re-render so never re-run it).
  useEffect(() => {
    armRef.current();
  }, [rawText]);

  return rawText.slice(0, displayLen);
}
