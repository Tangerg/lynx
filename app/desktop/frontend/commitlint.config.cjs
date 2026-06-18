// Conventional Commits enforcement. Lyra's recent commit history is
// already on this format (`feat: ...`, `refactor(scope): ...`, etc) —
// this config locks the convention in so it stays consistent.
//
// The full type list lives in @commitlint/config-conventional; the only
// project-specific override is bumping the body line limit since some
// commit messages here legitimately need multi-paragraph rationale.
module.exports = {
  extends: ["@commitlint/config-conventional"],
  rules: {
    "body-max-line-length": [2, "always", 100],
    "footer-max-line-length": [2, "always", 100],
  },
};
