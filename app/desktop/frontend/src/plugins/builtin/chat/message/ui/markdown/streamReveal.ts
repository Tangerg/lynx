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

// Reduced-motion gate. Progressive reveal is JS-driven (it grows the visible
// text length over time), so the blanket `prefers-reduced-motion` rule in
// globals.css — which only tones down CSS transition/animation durations —
// cannot reach it. We short-circuit here so reduced-motion users get the full
// text at once, no typewriter / per-word crawl. Honours BOTH the OS media query
// AND the in-app "Motion: Off" setting (which stamps `data-motion="off"` on
// :root, the same signal the CSS rule keys on) so the two stay consistent.
function prefersReducedMotion(): boolean {
  if (typeof document !== "undefined") {
    if (document.documentElement.getAttribute("data-motion") === "off") return true;
  }
  return (
    typeof window !== "undefined" &&
    typeof window.matchMedia === "function" &&
    window.matchMedia("(prefers-reduced-motion: reduce)").matches
  );
}

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
  // Reduced motion collapses the reveal: start fully revealed, never arm the
  // rAF loop, and return the full text on every render (see the guards below).
  const reduce = prefersReducedMotion();
  const active = enabled && !reduce;

  const initialLen = active ? 0 : rawText.length;
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
  const enabledRef = useRef(active);
  enabledRef.current = active;
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
  // Under reduced motion the loop stays parked — nothing to re-arm.
  useEffect(() => {
    if (reduce) return;
    armRef.current();
  }, [rawText, reduce]);

  // Reduced motion returns the full text directly, bypassing displayLen (which
  // is pinned at mount but never advanced, since the loop never arms).
  return reduce ? rawText : rawText.slice(0, displayLen);
}

// Commit-throttle ceiling for the markdown re-parse. `useStreamReveal` advances
// the visible text a few characters per rAF frame, so its output changes
// ~60×/s; re-parsing + re-rendering the whole markdown block tree that often is
// work the eye cannot resolve for a text reveal. This coalesces the input to at
// most one commit per `minMs` (a leading commit, then a trailing one), so a
// burst of tiny tokens cannot trigger a parse every frame. The trailing edge
// ALWAYS flushes the latest value, so the final (settled) text is never left as
// a stale slice. `minMs <= 0` is a passthrough — used once the stream ends (the
// short drain should land promptly) and for instant/user text.
export function useCommitThrottle(value: string, minMs: number): string {
  const [committed, setCommitted] = useState(value);
  const lastCommitRef = useRef(0);

  useEffect(() => {
    if (minMs <= 0) {
      setCommitted(value);
      return;
    }
    const elapsed = performance.now() - lastCommitRef.current;
    if (elapsed >= minMs) {
      lastCommitRef.current = performance.now();
      setCommitted(value);
      return;
    }
    // Inside the window — schedule a trailing commit of this (latest) value.
    // A newer value re-runs the effect, whose cleanup clears this timer and
    // schedules one for the newer value, so the trailing edge is always fresh.
    const id = setTimeout(() => {
      lastCommitRef.current = performance.now();
      setCommitted(value);
    }, minMs - elapsed);
    return () => clearTimeout(id);
  }, [value, minMs]);

  return committed;
}
