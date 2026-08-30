## Issue 67: Correct invalid UTF-8 maximal-subpart decoding

**Type**: AFK
**Blocked by**: None — can start immediately

### Parent PRD

`PRD-sqloid.md`

### What to build

Correct the shared TEXT decoder so it preserves a valid three-byte U+FFFD rune even when later bytes are malformed and emits exactly one U+FFFD for each maximal invalid sequence. Complete the E0–EF three-byte maximal-subpart checks, including a valid lead and second byte followed by an invalid or missing third byte, and the analogous F0–F4 four-byte cases with valid continuation prefixes followed by an invalid or missing later byte. Keep BLOB bytes unchanged and retain the existing invalid-UTF metadata signal used by grid, CSV, and JSON.

### How to verify

- **Verification sequencing**: Package/seam-level automated verification can proceed before Issue 57; manual/end-to-end steps that drive the shipped TUI must be re-run after Issue 57 lands.
- **Manual**: View and export rows containing a valid encoded U+FFFD followed by malformed bytes and rows containing truncated or invalid E0–EF and F0–F4 sequences; confirm grid, CSV, and JSON agree, preserve the valid U+FFFD, and show one replacement rune per maximal invalid sequence.
- **Automated**: Table-driven result-decoder tests assert the exact output bytes and invalid-UTF flag for mixed valid-U+FFFD/malformed text; E0–EF sequences with a valid lead/second byte and an invalid or missing third byte; F0–F4 sequences with valid continuation prefixes and an invalid or missing later byte; adjacent invalid sequences; valid text; and unchanged BLOB payloads.

### Acceptance criteria

- [ ] Given a valid three-byte U+FFFD followed elsewhere by malformed UTF-8, when the TEXT is decoded, then the valid rune is preserved and only malformed maximal subparts add replacement runes.
- [ ] Given an E0–EF lead and valid second byte followed by an invalid or missing third byte, then exactly one U+FFFD is emitted for that maximal invalid sequence.
- [ ] Given an F0–F4 lead and one or more valid continuation bytes followed by an invalid or missing later byte, then exactly one U+FFFD is emitted for that maximal invalid sequence.
- [ ] Given malformed TEXT is rendered or exported, then grid, CSV, and JSON receive identical corrected text and invalid-UTF metadata remains true; BLOB bytes are not decoded or changed.

### User stories addressed

- User story 75: Replace every maximal invalid UTF-8 TEXT sequence consistently

---
