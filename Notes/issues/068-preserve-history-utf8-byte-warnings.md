## Issue 68: Preserve invalid-UTF and byte-cap warnings in result history

**Type**: AFK
**Blocked by**: Issue 60

### Parent PRD

`PRD-sqloid.md`

### What to build

Carry an active page's invalid-UTF flag through SELECT finalization into immutable snapshot metadata. When projecting a historical tabular entry, restore both that invalid-UTF flag on the reconstructed page and `truncated-by-byte-cap` on the result view so browsing and exporting history disclose the same warnings that were true at execution. Warning metadata must remain outside CSV/JSON records and fields.

### How to verify

- **Verification sequencing**: Package/seam-level automated verification can proceed before Issue 57; manual/end-to-end steps that drive the shipped TUI must be re-run after Issue 57 lands.
- **Manual**: Finalize one SELECT containing malformed TEXT and another whose cache was truncated by the 64 MiB byte cap; browse and export each historical entry and confirm its original warning remains visible without appearing in exported data.
- **Automated**: UI lifecycle tests drive settlement → finalization → history projection → export capture separately for invalid UTF-8 and byte-cap truncation, asserting snapshot metadata, reconstructed view metadata, displayed/export-flow warnings, immutable rows, and absence of warning records or properties in serializer input.

### Acceptance criteria

- [ ] Given an active SELECT page reports invalid UTF-8 replacement, when it is finalized, then its immutable snapshot records `InvalidUTF` as true.
- [ ] Given historical snapshot metadata records invalid UTF-8 or byte-cap truncation, when the entry is projected, then the reconstructed page/view restores the corresponding warning metadata.
- [ ] Given either warned historical entry is exported, then the warning is disclosed before destination selection and remains outside CSV/JSON data.

### User stories addressed

- User story 55: Preserve truthful snapshot and export metadata
- User story 64: Browse immutable result snapshots without re-fetching
- User story 70: Disclose UTF and truncation warnings outside exported data

---
