// Completion chime — a soft two-note arpeggio synthesized via Web Audio, the
// optional audible companion to the OS completion notification (lib/osNotify).
// No audio asset: two short sine envelopes, so there's nothing to bundle or
// decode. The CALLER owns the gate (toggle + focus) exactly like osNotify.
//
// A single AudioContext is created lazily and reused. Browsers start it
// "suspended" until a user gesture, but by run-completion the user has already
// clicked send, so resume() succeeds; if it's somehow still suspended the notes
// just don't sound (no error surfaces).

let ctx: AudioContext | null = null;

function audioContext(): AudioContext | null {
  if (typeof AudioContext === "undefined") return null;
  ctx ??= new AudioContext();
  return ctx;
}

// playCompletionChime plays one soft two-note chime when Web Audio is
// available; otherwise a no-op.
export function playCompletionChime(): void {
  const ac = audioContext();
  if (!ac) return;
  void ac.resume();

  const now = ac.currentTime;
  // E5 then B5 — a gentle rising fifth. Each note is a sine with a fast attack
  // and exponential decay so it reads as a chime, not a beep.
  [659.25, 987.77].forEach((freq, i) => {
    const osc = ac.createOscillator();
    const gain = ac.createGain();
    osc.type = "sine";
    osc.frequency.value = freq;
    const start = now + i * 0.11;
    gain.gain.setValueAtTime(0, start);
    gain.gain.linearRampToValueAtTime(0.11, start + 0.02);
    gain.gain.exponentialRampToValueAtTime(0.0001, start + 0.26);
    osc.connect(gain).connect(ac.destination);
    osc.start(start);
    osc.stop(start + 0.3);
  });
}
