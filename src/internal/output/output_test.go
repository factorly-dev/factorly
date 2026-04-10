package output

import (
	"strings"
	"testing"
)

// --- Truncation tests ---

func TestTruncateUnderLimit(t *testing.T) {
	input := "hello world"
	got := Truncate(input, 100)
	if got != input {
		t.Errorf("expected unchanged string, got %q", got)
	}
}

func TestTruncateOverLimit(t *testing.T) {
	input := strings.Repeat("x", 1000)
	got := Truncate(input, 200)
	if len(got) > 200 {
		t.Errorf("expected len <= 200, got %d", len(got))
	}
	if !strings.Contains(got, "[... truncated") {
		t.Errorf("expected truncation marker in output")
	}
}

func TestTruncatePreservesHeadTail(t *testing.T) {
	// Build a string with identifiable head and tail.
	head := "HEAD" + strings.Repeat("a", 496)
	tail := strings.Repeat("b", 496) + "TAIL"
	input := head + tail
	if len(input) != 1000 {
		t.Fatalf("expected input len 1000, got %d", len(input))
	}

	got := Truncate(input, 500)
	if !strings.HasPrefix(got, "HEAD") {
		t.Errorf("expected output to start with HEAD")
	}
	if !strings.HasSuffix(got, "TAIL") {
		t.Errorf("expected output to end with TAIL")
	}
}

func TestTruncateMarker(t *testing.T) {
	input := strings.Repeat("x", 1000)
	got := Truncate(input, 200)
	if !strings.Contains(got, "truncated") {
		t.Errorf("expected marker with byte count")
	}
	if !strings.Contains(got, "bytes") {
		t.Errorf("expected marker to mention bytes")
	}
}

func TestTruncateZeroLimit(t *testing.T) {
	input := "hello"
	got := Truncate(input, 0)
	if got != input {
		t.Errorf("expected unchanged string for zero limit, got %q", got)
	}
}

func TestTruncateNegativeLimit(t *testing.T) {
	input := "hello"
	got := Truncate(input, -1)
	if got != input {
		t.Errorf("expected unchanged string for negative limit, got %q", got)
	}
}

func TestTruncateExactLimit(t *testing.T) {
	input := "hello"
	got := Truncate(input, 5)
	if got != input {
		t.Errorf("expected unchanged string at exact limit, got %q", got)
	}
}

// --- Compression tests ---

func TestCompressStripANSI(t *testing.T) {
	input := "\x1b[31mred\x1b[0m"
	got := Compress(input)
	if got != "red" {
		t.Errorf("expected %q, got %q", "red", got)
	}
}

func TestCompressNormalizeWhitespace(t *testing.T) {
	input := "a\n\n\n\n\nb"
	got := Compress(input)
	if got != "a\n\nb" {
		t.Errorf("expected %q, got %q", "a\n\nb", got)
	}
}

func TestCompressJSONCompact(t *testing.T) {
	input := "{\n  \"key\": \"value\",\n  \"num\": 42\n}"
	got := Compress(input, HintJSON)
	expected := `{"key":"value","num":42}`
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestCompressJSONNoHint(t *testing.T) {
	input := "{\n  \"key\": \"value\"\n}"
	got := Compress(input)
	if got != input {
		t.Errorf("expected JSON left alone without hint, got %q", got)
	}
}

func TestCompressLogDedup(t *testing.T) {
	lines := make([]string, 5)
	for i := range lines {
		lines[i] = "same line"
	}
	input := strings.Join(lines, "\n")
	got := Compress(input, HintLogs)
	expected := "same line [repeated 5 times]"
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestCompressLogDedupMixed(t *testing.T) {
	input := "a\nb\na\nb\na"
	got := Compress(input, HintLogs)
	if got != input {
		t.Errorf("expected alternating lines unchanged, got %q", got)
	}
}

func TestCompressAll(t *testing.T) {
	input := "\x1b[32m{\n  \"key\": \"value\"\n}\x1b[0m"
	got := Compress(input, HintAll)
	expected := `{"key":"value"}`
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestCompressEmpty(t *testing.T) {
	got := Compress("")
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

// --- Process tests ---

func TestProcessCompressThenTruncate(t *testing.T) {
	// Create a large pretty-printed JSON.
	var sb strings.Builder
	sb.WriteString("{\n")
	for i := 0; i < 100; i++ {
		sb.WriteString("  \"key")
		sb.WriteString(strings.Repeat("x", 50))
		sb.WriteString("\": \"value\",\n")
	}
	sb.WriteString("  \"last\": true\n}")
	input := sb.String()

	got := Process(input, 500, HintJSON)

	// Should be compacted first (no pretty whitespace), then truncated.
	if len(got) > 500 {
		t.Errorf("expected len <= 500, got %d", len(got))
	}
	// The compacted JSON shouldn't have indentation.
	if strings.Contains(got, "  \"key") {
		t.Errorf("expected JSON to be compacted before truncation")
	}
}
