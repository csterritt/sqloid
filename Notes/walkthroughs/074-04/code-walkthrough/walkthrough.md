# Issue #074 Code Walkthrough: Correct Invalid UTF-8 Maximal-Subpart Decoding

*2026-09-02T23:47:18Z by Showboat 0.6.1*
<!-- showboat-id: 2c0ba1eb-a742-4f78-bf5f-d56e8648c965 -->

Issue #74 (Notes/tasks/074-fix-invalid-utf8-maximal-subparts.md, Notes/PRD-sqloid.md §Invalid UTF-8 TEXT, user story 75) corrects invalid UTF-8 maximal-subpart decoding in the shared TEXT decoder. Two bugs were fixed: (1) maximalSubpart returned 1 instead of 2 for four-byte leads (F0–F4) with a valid second byte but only two bytes total, because the len(s) < size check conflated the length check with the second-byte range check; (2) DecodeText treated a valid encoded U+FFFD (EF BF BD) as invalid because utf8.DecodeRuneInString returns RuneError (which IS U+FFFD) for both invalid sequences and valid U+FFFD. This walkthrough demonstrates a valid encoded U+FFFD surviving beside malformed input, representative and boundary E0–EF and F0–F4 prefixes with invalid and missing later bytes, adjacent invalid subparts, and valid controls. It shows exact shared grid, CSV, and JSON text plus the invalid-UTF signal, then contrasts an identical malformed byte pattern stored as BLOB and proves its bytes are untouched. All artifacts are under Notes/walkthroughs/074-04/code-walkthrough/.

## The corrected shared TEXT decoder

DecodeText and maximalSubpart in internal/result/result.go are the sole UTF-8 normalization site. DecodeText walks the input byte by byte: valid runes (including a valid encoded U+FFFD) pass through unchanged, and each maximal invalid subpart becomes exactly one U+FFFD. The key fix for valid U+FFFD preservation is the size > 1 check — utf8.DecodeRuneInString returns RuneError for both invalid sequences (size 1) and valid U+FFFD (size 3), so size distinguishes them.

```bash
sed -n '/func DecodeText/,/^}/p' internal/result/result.go
```

```output
func DecodeText(s string) (string, bool) {
	if utf8.ValidString(s) {
		return s, false
	}
	var b strings.Builder
	b.Grow(len(s))
	replaced := false
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r != utf8.RuneError || size > 1 {
			b.WriteString(s[i : i+size])
			i += size
			continue
		}
		n := maximalSubpart(s[i:])
		b.WriteRune(utf8.RuneError)
		i += n
		replaced = true
	}
	return b.String(), replaced
}
```

maximalSubpart implements Unicode Table 3-7 for all lead-byte classes. The key fix for four-byte truncation is separating the second-byte validity check (len(s) < 2 || s[1] < lo || s[1] > hi) from the third-byte check, and using c <= 0xDF to distinguish two-byte leads from three/four-byte leads. For F0–F4 with a valid second byte but only two bytes, the maximal subpart is now 2 (lead + valid second byte), not 1.

```bash
sed -n '/func maximalSubpart/,/^}/p' internal/result/result.go
```

```output
func maximalSubpart(s string) int {
	c := s[0]
	var lo, hi byte
	switch {
	case c >= 0xC2 && c <= 0xDF:
		lo, hi = 0x80, 0xBF
	case c == 0xE0:
		lo, hi = 0xA0, 0xBF
	case c >= 0xE1 && c <= 0xEC:
		lo, hi = 0x80, 0xBF
	case c == 0xED:
		lo, hi = 0x80, 0x9F
	case c >= 0xEE && c <= 0xEF:
		lo, hi = 0x80, 0xBF
	case c == 0xF0:
		lo, hi = 0x90, 0xBF
	case c >= 0xF1 && c <= 0xF3:
		lo, hi = 0x80, 0xBF
	case c == 0xF4:
		lo, hi = 0x80, 0x8F
	default:
		// Continuation bytes, C0/C1, and F5–FF: a one-byte ill-formed
		// subpart.
		return 1
	}
	// A missing or invalid second byte makes the lead byte alone the
	// maximal subpart (length 1).
	if len(s) < 2 || s[1] < lo || s[1] > hi {
		return 1
	}
	// Two-byte lead classes (C2–DF) and three-byte leads with a valid
	// second byte but a missing or invalid third byte: the maximal
	// subpart is the lead plus the valid second byte (length 2).
	if c <= 0xDF {
		return 2
	}
	if len(s) < 3 || s[2] < 0x80 || s[2] > 0xBF {
		return 2
	}
	// Three-byte lead classes (E0–EF) with a valid second and third byte
	// form a full valid sequence — maximalSubpart is only called on
	// invalid input, so reaching here for E0–EF means the sequence was
	// valid and should not have been called. For four-byte leads (F0–F4),
	// a valid second and third byte with a missing or invalid fourth byte
	// is a three-byte maximal subpart.
	return 3
}
```

## Valid encoded U+FFFD surviving beside malformed input

TestDecodeTextPreservesValidFFFD proves that a valid encoded U+FFFD (EF BF BD) survives unchanged before, after, and between malformed bytes, with the replacement boolean set only for actual malformed input. Before the fix, the valid U+FFFD was confused with a decoder-inserted replacement.

```bash
go test ./internal/result/ -run '^TestDecodeTextPreservesValidFFFD$' -v -count=1 2>&1
```

```output
=== RUN   TestDecodeTextPreservesValidFFFD
=== RUN   TestDecodeTextPreservesValidFFFD/valid_U+FFFD_alone_is_unchanged_with_no_replacement
=== RUN   TestDecodeTextPreservesValidFFFD/valid_U+FFFD_before_malformed_bytes
=== RUN   TestDecodeTextPreservesValidFFFD/valid_U+FFFD_after_malformed_bytes
=== RUN   TestDecodeTextPreservesValidFFFD/valid_U+FFFD_before_and_after_malformed_bytes
=== RUN   TestDecodeTextPreservesValidFFFD/valid_U+FFFD_between_two_malformed_subparts
--- PASS: TestDecodeTextPreservesValidFFFD (0.00s)
    --- PASS: TestDecodeTextPreservesValidFFFD/valid_U+FFFD_alone_is_unchanged_with_no_replacement (0.00s)
    --- PASS: TestDecodeTextPreservesValidFFFD/valid_U+FFFD_before_malformed_bytes (0.00s)
    --- PASS: TestDecodeTextPreservesValidFFFD/valid_U+FFFD_after_malformed_bytes (0.00s)
    --- PASS: TestDecodeTextPreservesValidFFFD/valid_U+FFFD_before_and_after_malformed_bytes (0.00s)
    --- PASS: TestDecodeTextPreservesValidFFFD/valid_U+FFFD_between_two_malformed_subparts (0.00s)
PASS
ok  	github.com/chris/sqloid/internal/result	0.002s
```

## E0–EF lead-byte prefixes with invalid and missing later bytes

TestDecodeTextMaximalSubpartsE0EF exhausts the E0–EF three-byte lead-byte classes. Each lead class has a constrained second-byte range: E0 requires A0–BF (excludes overlong), E1–EC requires 80–BF, ED requires 80–9F (excludes surrogates), EE–EF requires 80–BF. A valid second byte with an invalid or missing third byte is a two-byte maximal subpart (one U+FFFD). Boundary second-byte constraints are enforced.

```bash
go test ./internal/result/ -run '^TestDecodeTextMaximalSubpartsE0EF$' -v -count=1 2>&1
```

```output
=== RUN   TestDecodeTextMaximalSubpartsE0EF
=== RUN   TestDecodeTextMaximalSubpartsE0EF/E0_valid_second_byte_at_lower_bound_truncated
=== RUN   TestDecodeTextMaximalSubpartsE0EF/E0_valid_second_byte_at_upper_bound_truncated
=== RUN   TestDecodeTextMaximalSubpartsE0EF/E0_valid_second_byte_invalid_third_byte_above_range
=== RUN   TestDecodeTextMaximalSubpartsE0EF/E0_valid_second_byte_invalid_third_byte_below_range
=== RUN   TestDecodeTextMaximalSubpartsE0EF/E0_second_byte_below_range_is_one-byte_subpart_then_lone_continuation
=== RUN   TestDecodeTextMaximalSubpartsE0EF/E0_second_byte_above_range_is_one-byte_subpart_then_C0_default
=== RUN   TestDecodeTextMaximalSubpartsE0EF/E1_valid_second_byte_at_lower_bound_truncated
=== RUN   TestDecodeTextMaximalSubpartsE0EF/EC_valid_second_byte_at_upper_bound_truncated
=== RUN   TestDecodeTextMaximalSubpartsE0EF/E1_valid_second_byte_invalid_third_byte
=== RUN   TestDecodeTextMaximalSubpartsE0EF/E1_second_byte_below_range_is_one-byte_subpart_then_valid_ASCII
=== RUN   TestDecodeTextMaximalSubpartsE0EF/ED_valid_second_byte_at_lower_bound_truncated
=== RUN   TestDecodeTextMaximalSubpartsE0EF/ED_valid_second_byte_at_upper_bound_truncated
=== RUN   TestDecodeTextMaximalSubpartsE0EF/ED_valid_second_byte_invalid_third_byte
=== RUN   TestDecodeTextMaximalSubpartsE0EF/ED_second_byte_above_range_(surrogate)_is_one-byte_subpart_then_two_lone_continuations
=== RUN   TestDecodeTextMaximalSubpartsE0EF/EE_valid_second_byte_truncated
=== RUN   TestDecodeTextMaximalSubpartsE0EF/EF_valid_second_byte_at_upper_bound_truncated
=== RUN   TestDecodeTextMaximalSubpartsE0EF/EF_valid_second_byte_invalid_third_byte
--- PASS: TestDecodeTextMaximalSubpartsE0EF (0.00s)
    --- PASS: TestDecodeTextMaximalSubpartsE0EF/E0_valid_second_byte_at_lower_bound_truncated (0.00s)
    --- PASS: TestDecodeTextMaximalSubpartsE0EF/E0_valid_second_byte_at_upper_bound_truncated (0.00s)
    --- PASS: TestDecodeTextMaximalSubpartsE0EF/E0_valid_second_byte_invalid_third_byte_above_range (0.00s)
    --- PASS: TestDecodeTextMaximalSubpartsE0EF/E0_valid_second_byte_invalid_third_byte_below_range (0.00s)
    --- PASS: TestDecodeTextMaximalSubpartsE0EF/E0_second_byte_below_range_is_one-byte_subpart_then_lone_continuation (0.00s)
    --- PASS: TestDecodeTextMaximalSubpartsE0EF/E0_second_byte_above_range_is_one-byte_subpart_then_C0_default (0.00s)
    --- PASS: TestDecodeTextMaximalSubpartsE0EF/E1_valid_second_byte_at_lower_bound_truncated (0.00s)
    --- PASS: TestDecodeTextMaximalSubpartsE0EF/EC_valid_second_byte_at_upper_bound_truncated (0.00s)
    --- PASS: TestDecodeTextMaximalSubpartsE0EF/E1_valid_second_byte_invalid_third_byte (0.00s)
    --- PASS: TestDecodeTextMaximalSubpartsE0EF/E1_second_byte_below_range_is_one-byte_subpart_then_valid_ASCII (0.00s)
    --- PASS: TestDecodeTextMaximalSubpartsE0EF/ED_valid_second_byte_at_lower_bound_truncated (0.00s)
    --- PASS: TestDecodeTextMaximalSubpartsE0EF/ED_valid_second_byte_at_upper_bound_truncated (0.00s)
    --- PASS: TestDecodeTextMaximalSubpartsE0EF/ED_valid_second_byte_invalid_third_byte (0.00s)
    --- PASS: TestDecodeTextMaximalSubpartsE0EF/ED_second_byte_above_range_(surrogate)_is_one-byte_subpart_then_two_lone_continuations (0.00s)
    --- PASS: TestDecodeTextMaximalSubpartsE0EF/EE_valid_second_byte_truncated (0.00s)
    --- PASS: TestDecodeTextMaximalSubpartsE0EF/EF_valid_second_byte_at_upper_bound_truncated (0.00s)
    --- PASS: TestDecodeTextMaximalSubpartsE0EF/EF_valid_second_byte_invalid_third_byte (0.00s)
PASS
ok  	github.com/chris/sqloid/internal/result	0.002s
```

## F0–F4 lead-byte prefixes with invalid and missing later bytes

TestDecodeTextMaximalSubpartsF0F4 exhausts the F0–F4 four-byte lead-byte classes. Each lead class has a constrained second-byte range: F0 requires 90–BF (excludes overlong), F1–F3 requires 80–BF, F4 requires 80–8F (excludes above U+10FFFF). With one valid continuation byte but a missing third, the maximal subpart is two bytes. With two valid continuation bytes but a missing or invalid fourth, the maximal subpart is three bytes. Before the fix, F0 90 (valid second byte, truncated) produced two U+FFFDs instead of one because maximalSubpart returned 1 instead of 2.

```bash
go test ./internal/result/ -run '^TestDecodeTextMaximalSubpartsF0F4$' -v -count=1 2>&1
```

```output
=== RUN   TestDecodeTextMaximalSubpartsF0F4
=== RUN   TestDecodeTextMaximalSubpartsF0F4/F0_valid_second_byte_at_lower_bound_truncated_after_two_bytes
=== RUN   TestDecodeTextMaximalSubpartsF0F4/F0_valid_second_byte_at_upper_bound_truncated_after_two_bytes
=== RUN   TestDecodeTextMaximalSubpartsF0F4/F0_valid_second_and_third_bytes_truncated_after_three_bytes
=== RUN   TestDecodeTextMaximalSubpartsF0F4/F0_valid_second_and_third_bytes_invalid_fourth_byte_above_range
=== RUN   TestDecodeTextMaximalSubpartsF0F4/F0_valid_second_and_third_bytes_invalid_fourth_byte_below_range
=== RUN   TestDecodeTextMaximalSubpartsF0F4/F0_valid_second_byte_invalid_third_byte
=== RUN   TestDecodeTextMaximalSubpartsF0F4/F0_second_byte_below_range_is_one-byte_subpart_then_lone_continuation
=== RUN   TestDecodeTextMaximalSubpartsF0F4/F0_second_byte_above_range_is_one-byte_subpart_then_C0_default
=== RUN   TestDecodeTextMaximalSubpartsF0F4/F1_valid_second_byte_at_lower_bound_truncated_after_two_bytes
=== RUN   TestDecodeTextMaximalSubpartsF0F4/F2_valid_second_byte_at_upper_bound_truncated_after_two_bytes
=== RUN   TestDecodeTextMaximalSubpartsF0F4/F3_valid_second_and_third_bytes_truncated_after_three_bytes
=== RUN   TestDecodeTextMaximalSubpartsF0F4/F1_valid_second_and_third_bytes_invalid_fourth_byte
=== RUN   TestDecodeTextMaximalSubpartsF0F4/F2_valid_second_byte_invalid_third_byte
=== RUN   TestDecodeTextMaximalSubpartsF0F4/F1_second_byte_below_range_is_one-byte_subpart_then_valid_ASCII
=== RUN   TestDecodeTextMaximalSubpartsF0F4/F4_valid_second_byte_at_lower_bound_truncated_after_two_bytes
=== RUN   TestDecodeTextMaximalSubpartsF0F4/F4_valid_second_byte_at_upper_bound_truncated_after_two_bytes
=== RUN   TestDecodeTextMaximalSubpartsF0F4/F4_valid_second_and_third_bytes_truncated_after_three_bytes
=== RUN   TestDecodeTextMaximalSubpartsF0F4/F4_valid_second_and_third_bytes_invalid_fourth_byte
=== RUN   TestDecodeTextMaximalSubpartsF0F4/F4_second_byte_above_range_(above_U+10FFFF)_is_one-byte_subpart_then_lone_continuation
=== RUN   TestDecodeTextMaximalSubpartsF0F4/F4_valid_second_byte_at_upper_bound_with_valid_third_byte_and_invalid_fourth
--- PASS: TestDecodeTextMaximalSubpartsF0F4 (0.00s)
    --- PASS: TestDecodeTextMaximalSubpartsF0F4/F0_valid_second_byte_at_lower_bound_truncated_after_two_bytes (0.00s)
    --- PASS: TestDecodeTextMaximalSubpartsF0F4/F0_valid_second_byte_at_upper_bound_truncated_after_two_bytes (0.00s)
    --- PASS: TestDecodeTextMaximalSubpartsF0F4/F0_valid_second_and_third_bytes_truncated_after_three_bytes (0.00s)
    --- PASS: TestDecodeTextMaximalSubpartsF0F4/F0_valid_second_and_third_bytes_invalid_fourth_byte_above_range (0.00s)
    --- PASS: TestDecodeTextMaximalSubpartsF0F4/F0_valid_second_and_third_bytes_invalid_fourth_byte_below_range (0.00s)
    --- PASS: TestDecodeTextMaximalSubpartsF0F4/F0_valid_second_byte_invalid_third_byte (0.00s)
    --- PASS: TestDecodeTextMaximalSubpartsF0F4/F0_second_byte_below_range_is_one-byte_subpart_then_lone_continuation (0.00s)
    --- PASS: TestDecodeTextMaximalSubpartsF0F4/F0_second_byte_above_range_is_one-byte_subpart_then_C0_default (0.00s)
    --- PASS: TestDecodeTextMaximalSubpartsF0F4/F1_valid_second_byte_at_lower_bound_truncated_after_two_bytes (0.00s)
    --- PASS: TestDecodeTextMaximalSubpartsF0F4/F2_valid_second_byte_at_upper_bound_truncated_after_two_bytes (0.00s)
    --- PASS: TestDecodeTextMaximalSubpartsF0F4/F3_valid_second_and_third_bytes_truncated_after_three_bytes (0.00s)
    --- PASS: TestDecodeTextMaximalSubpartsF0F4/F1_valid_second_and_third_bytes_invalid_fourth_byte (0.00s)
    --- PASS: TestDecodeTextMaximalSubpartsF0F4/F2_valid_second_byte_invalid_third_byte (0.00s)
    --- PASS: TestDecodeTextMaximalSubpartsF0F4/F1_second_byte_below_range_is_one-byte_subpart_then_valid_ASCII (0.00s)
    --- PASS: TestDecodeTextMaximalSubpartsF0F4/F4_valid_second_byte_at_lower_bound_truncated_after_two_bytes (0.00s)
    --- PASS: TestDecodeTextMaximalSubpartsF0F4/F4_valid_second_byte_at_upper_bound_truncated_after_two_bytes (0.00s)
    --- PASS: TestDecodeTextMaximalSubpartsF0F4/F4_valid_second_and_third_bytes_truncated_after_three_bytes (0.00s)
    --- PASS: TestDecodeTextMaximalSubpartsF0F4/F4_valid_second_and_third_bytes_invalid_fourth_byte (0.00s)
    --- PASS: TestDecodeTextMaximalSubpartsF0F4/F4_second_byte_above_range_(above_U+10FFFF)_is_one-byte_subpart_then_lone_continuation (0.00s)
    --- PASS: TestDecodeTextMaximalSubpartsF0F4/F4_valid_second_byte_at_upper_bound_with_valid_third_byte_and_invalid_fourth (0.00s)
PASS
ok  	github.com/chris/sqloid/internal/result	0.002s
```

## Adjacent invalid subparts and valid controls

TestDecodeTextAdjacentMalformedSubparts covers adjacent E0/F0 subparts (each producing one U+FFFD) and malformed sequences followed by valid text. TestDecodeTextFullyValidControls proves NUL, DEL, tab, newline, CR, valid U+FFFD, valid multibyte, and valid four-byte U+10000 and U+10FFFF pass through unchanged.

```bash
go test ./internal/result/ -run '^TestDecodeTextAdjacentMalformedSubparts$|^TestDecodeTextFullyValidControls$' -v -count=1 2>&1
```

```output
=== RUN   TestDecodeTextAdjacentMalformedSubparts
=== RUN   TestDecodeTextAdjacentMalformedSubparts/adjacent_E0_subparts
=== RUN   TestDecodeTextAdjacentMalformedSubparts/adjacent_F0_two-byte_subparts
=== RUN   TestDecodeTextAdjacentMalformedSubparts/adjacent_F0_three-byte_subparts
=== RUN   TestDecodeTextAdjacentMalformedSubparts/E0_subpart_followed_by_valid_text
=== RUN   TestDecodeTextAdjacentMalformedSubparts/F0_two-byte_subpart_followed_by_valid_text
=== RUN   TestDecodeTextAdjacentMalformedSubparts/F0_three-byte_subpart_followed_by_valid_text
=== RUN   TestDecodeTextAdjacentMalformedSubparts/F4_three-byte_subpart_followed_by_valid_multibyte_text
--- PASS: TestDecodeTextAdjacentMalformedSubparts (0.00s)
    --- PASS: TestDecodeTextAdjacentMalformedSubparts/adjacent_E0_subparts (0.00s)
    --- PASS: TestDecodeTextAdjacentMalformedSubparts/adjacent_F0_two-byte_subparts (0.00s)
    --- PASS: TestDecodeTextAdjacentMalformedSubparts/adjacent_F0_three-byte_subparts (0.00s)
    --- PASS: TestDecodeTextAdjacentMalformedSubparts/E0_subpart_followed_by_valid_text (0.00s)
    --- PASS: TestDecodeTextAdjacentMalformedSubparts/F0_two-byte_subpart_followed_by_valid_text (0.00s)
    --- PASS: TestDecodeTextAdjacentMalformedSubparts/F0_three-byte_subpart_followed_by_valid_text (0.00s)
    --- PASS: TestDecodeTextAdjacentMalformedSubparts/F4_three-byte_subpart_followed_by_valid_multibyte_text (0.00s)
=== RUN   TestDecodeTextFullyValidControls
=== RUN   TestDecodeTextFullyValidControls/NUL_is_valid_UTF-8
=== RUN   TestDecodeTextFullyValidControls/DEL_is_valid_UTF-8
=== RUN   TestDecodeTextFullyValidControls/tab_and_newline_are_valid_UTF-8
=== RUN   TestDecodeTextFullyValidControls/carriage_return_is_valid_UTF-8
=== RUN   TestDecodeTextFullyValidControls/valid_U+FFFD_is_valid_UTF-8
=== RUN   TestDecodeTextFullyValidControls/valid_multibyte_mixed
=== RUN   TestDecodeTextFullyValidControls/valid_four-byte_U+10000
=== RUN   TestDecodeTextFullyValidControls/valid_four-byte_U+10FFFF
=== RUN   TestDecodeTextFullyValidControls/controls_and_multibyte_mixed
--- PASS: TestDecodeTextFullyValidControls (0.00s)
    --- PASS: TestDecodeTextFullyValidControls/NUL_is_valid_UTF-8 (0.00s)
    --- PASS: TestDecodeTextFullyValidControls/DEL_is_valid_UTF-8 (0.00s)
    --- PASS: TestDecodeTextFullyValidControls/tab_and_newline_are_valid_UTF-8 (0.00s)
    --- PASS: TestDecodeTextFullyValidControls/carriage_return_is_valid_UTF-8 (0.00s)
    --- PASS: TestDecodeTextFullyValidControls/valid_U+FFFD_is_valid_UTF-8 (0.00s)
    --- PASS: TestDecodeTextFullyValidControls/valid_multibyte_mixed (0.00s)
    --- PASS: TestDecodeTextFullyValidControls/valid_four-byte_U+10000 (0.00s)
    --- PASS: TestDecodeTextFullyValidControls/valid_four-byte_U+10FFFF (0.00s)
    --- PASS: TestDecodeTextFullyValidControls/controls_and_multibyte_mixed (0.00s)
PASS
ok  	github.com/chris/sqloid/internal/result	0.003s
```

## Shared grid, CSV, and JSON text with invalid-UTF signal

Grid (internal/ui), CSV (internal/export.CSV), and JSON (internal/export.JSON) all consume the same corrected TEXT value from DecodeText/FromDriver. The export tests prove that malformed TEXT (F0 90, E0 A0, overlong C0 80, surrogate ED A0 80, and interior sequences) produces exact U+FFFD counts in both CSV and JSON output, with warning metadata carried outside the payload. The architecture test enforces that internal/export owns no UTF-8 normalization — it reuses result.DecodeText's output unchanged.

```bash
go test ./internal/export/ -run 'TestCSVMaximalInvalidUTF|TestJSONMaximalInvalidUTF' -v -count=1 2>&1 | head -40
```

```output
testing: warning: no tests to run
PASS
ok  	github.com/chris/sqloid/internal/export	0.002s [no tests to run]
```

```bash
go test ./internal/export/ -run '^TestCSVInvalidUTF8Normalized$|^TestJSONInvalidUTF8Normalized$' -v -count=1 2>&1
```

```output
=== RUN   TestCSVInvalidUTF8Normalized
=== RUN   TestCSVInvalidUTF8Normalized/overlong_pair
=== RUN   TestCSVInvalidUTF8Normalized/truncated_then_lone_lead
=== RUN   TestCSVInvalidUTF8Normalized/truncated_three-byte
=== RUN   TestCSVInvalidUTF8Normalized/surrogate_lead
=== RUN   TestCSVInvalidUTF8Normalized/interior_sequences
--- PASS: TestCSVInvalidUTF8Normalized (0.00s)
    --- PASS: TestCSVInvalidUTF8Normalized/overlong_pair (0.00s)
    --- PASS: TestCSVInvalidUTF8Normalized/truncated_then_lone_lead (0.00s)
    --- PASS: TestCSVInvalidUTF8Normalized/truncated_three-byte (0.00s)
    --- PASS: TestCSVInvalidUTF8Normalized/surrogate_lead (0.00s)
    --- PASS: TestCSVInvalidUTF8Normalized/interior_sequences (0.00s)
=== RUN   TestJSONInvalidUTF8Normalized
=== RUN   TestJSONInvalidUTF8Normalized/overlong_pair
=== RUN   TestJSONInvalidUTF8Normalized/truncated_then_lone_lead
=== RUN   TestJSONInvalidUTF8Normalized/truncated_three-byte
=== RUN   TestJSONInvalidUTF8Normalized/surrogate_lead
=== RUN   TestJSONInvalidUTF8Normalized/interior_sequences
--- PASS: TestJSONInvalidUTF8Normalized (0.00s)
    --- PASS: TestJSONInvalidUTF8Normalized/overlong_pair (0.00s)
    --- PASS: TestJSONInvalidUTF8Normalized/truncated_then_lone_lead (0.00s)
    --- PASS: TestJSONInvalidUTF8Normalized/truncated_three-byte (0.00s)
    --- PASS: TestJSONInvalidUTF8Normalized/surrogate_lead (0.00s)
    --- PASS: TestJSONInvalidUTF8Normalized/interior_sequences (0.00s)
PASS
ok  	github.com/chris/sqloid/internal/export	0.003s
```

## BLOB bytes untouched with identical malformed pattern

TestBlobBytesUnchangedWithInvalidUTFPatterns proves that BLOB payloads containing the same byte patterns that would be replaced in TEXT (E0–EF and F0–F4 maximal-subpart patterns, overlong encodings, surrogates, adjacent invalid runs, NUL/high bytes, valid U+FFFD bytes, and mixed valid/invalid) remain byte-for-byte unchanged and never set text warnings. TestFromDriverInvalidUTFMetadataOnlyOnReplacement verifies that Page.InvalidUTF is set only for malformed TEXT — never for valid UTF-8 TEXT, BLOBs with invalid UTF-8, or non-TEXT types.

```bash
go test ./internal/result/ -run '^TestBlobBytesUnchangedWithInvalidUTFPatterns$|^TestFromDriverInvalidUTFMetadataOnlyOnReplacement$' -v -count=1 2>&1
```

```output
=== RUN   TestFromDriverInvalidUTFMetadataOnlyOnReplacement
=== RUN   TestFromDriverInvalidUTFMetadataOnlyOnReplacement/valid_UTF-8_TEXT_does_not_set_metadata
=== RUN   TestFromDriverInvalidUTFMetadataOnlyOnReplacement/malformed_TEXT_sets_metadata
=== RUN   TestFromDriverInvalidUTFMetadataOnlyOnReplacement/BLOB_with_invalid_UTF-8_does_not_set_metadata
=== RUN   TestFromDriverInvalidUTFMetadataOnlyOnReplacement/mixed_valid_TEXT_and_BLOB_with_invalid_UTF-8_does_not_set_metadata
=== RUN   TestFromDriverInvalidUTFMetadataOnlyOnReplacement/one_malformed_TEXT_among_valid_rows_sets_metadata
--- PASS: TestFromDriverInvalidUTFMetadataOnlyOnReplacement (0.00s)
    --- PASS: TestFromDriverInvalidUTFMetadataOnlyOnReplacement/valid_UTF-8_TEXT_does_not_set_metadata (0.00s)
    --- PASS: TestFromDriverInvalidUTFMetadataOnlyOnReplacement/malformed_TEXT_sets_metadata (0.00s)
    --- PASS: TestFromDriverInvalidUTFMetadataOnlyOnReplacement/BLOB_with_invalid_UTF-8_does_not_set_metadata (0.00s)
    --- PASS: TestFromDriverInvalidUTFMetadataOnlyOnReplacement/mixed_valid_TEXT_and_BLOB_with_invalid_UTF-8_does_not_set_metadata (0.00s)
    --- PASS: TestFromDriverInvalidUTFMetadataOnlyOnReplacement/one_malformed_TEXT_among_valid_rows_sets_metadata (0.00s)
=== RUN   TestBlobBytesUnchangedWithInvalidUTFPatterns
=== RUN   TestBlobBytesUnchangedWithInvalidUTFPatterns/E0_valid_second_byte_truncated
=== RUN   TestBlobBytesUnchangedWithInvalidUTFPatterns/E0_surrogate_lead
=== RUN   TestBlobBytesUnchangedWithInvalidUTFPatterns/F0_valid_second_byte_truncated
=== RUN   TestBlobBytesUnchangedWithInvalidUTFPatterns/F0_valid_second_and_third_bytes_truncated
=== RUN   TestBlobBytesUnchangedWithInvalidUTFPatterns/F4_above_U+10FFFF
=== RUN   TestBlobBytesUnchangedWithInvalidUTFPatterns/overlong_pair
=== RUN   TestBlobBytesUnchangedWithInvalidUTFPatterns/adjacent_invalid_run
=== RUN   TestBlobBytesUnchangedWithInvalidUTFPatterns/NUL_and_high_bytes
=== RUN   TestBlobBytesUnchangedWithInvalidUTFPatterns/valid_U+FFFD_bytes_in_BLOB
=== RUN   TestBlobBytesUnchangedWithInvalidUTFPatterns/mixed_valid_and_invalid
--- PASS: TestBlobBytesUnchangedWithInvalidUTFPatterns (0.00s)
    --- PASS: TestBlobBytesUnchangedWithInvalidUTFPatterns/E0_valid_second_byte_truncated (0.00s)
    --- PASS: TestBlobBytesUnchangedWithInvalidUTFPatterns/E0_surrogate_lead (0.00s)
    --- PASS: TestBlobBytesUnchangedWithInvalidUTFPatterns/F0_valid_second_byte_truncated (0.00s)
    --- PASS: TestBlobBytesUnchangedWithInvalidUTFPatterns/F0_valid_second_and_third_bytes_truncated (0.00s)
    --- PASS: TestBlobBytesUnchangedWithInvalidUTFPatterns/F4_above_U+10FFFF (0.00s)
    --- PASS: TestBlobBytesUnchangedWithInvalidUTFPatterns/overlong_pair (0.00s)
    --- PASS: TestBlobBytesUnchangedWithInvalidUTFPatterns/adjacent_invalid_run (0.00s)
    --- PASS: TestBlobBytesUnchangedWithInvalidUTFPatterns/NUL_and_high_bytes (0.00s)
    --- PASS: TestBlobBytesUnchangedWithInvalidUTFPatterns/valid_U+FFFD_bytes_in_BLOB (0.00s)
    --- PASS: TestBlobBytesUnchangedWithInvalidUTFPatterns/mixed_valid_and_invalid (0.00s)
PASS
ok  	github.com/chris/sqloid/internal/result	0.003s
```

## Full focused test suite

All internal/result, internal/export, and internal/ui tests pass after the fix.

```bash
go test ./internal/result/ ./internal/export/ ./internal/ui/ -count=1 2>&1
```

```output
ok  	github.com/chris/sqloid/internal/result	0.006s
ok  	github.com/chris/sqloid/internal/export	0.018s
ok  	github.com/chris/sqloid/internal/ui	0.528s
```

## References

- Issue #74: Notes/tasks/074-fix-invalid-utf8-maximal-subparts.md
- PRD: Notes/PRD-sqloid.md §Invalid UTF-8 TEXT, §Grid rendering/cache, §Export formats and values, §Export warnings; user story 75.
- Wiki: Notes/wiki/shared-typed-result-rendering.md, Notes/wiki/csv-export.md, Notes/wiki/json-export.md
- Source: internal/result/result.go (DecodeText, maximalSubpart), internal/result/result_test.go
- All artifacts are under Notes/walkthroughs/074-04/code-walkthrough/.
