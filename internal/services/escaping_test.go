package services

import (
	"html"
	"strings"
	"testing"
	"testing/quick"
)

func TestProperty3_BoldFormattingConversion(t *testing.T) {
	property := func(text string) bool {
		result := FormatBold(text)
		expectedPrefix := "<b>"
		expectedSuffix := "</b>"
		expectedContent := html.EscapeString(text)
		expected := expectedPrefix + expectedContent + expectedSuffix

		return result == expected
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 100}); err != nil {
		t.Errorf("Property 3 failed: %v", err)
	}
}

func TestProperty4_ItalicFormattingConversion(t *testing.T) {
	property := func(text string) bool {
		result := FormatItalic(text)
		expectedPrefix := "<i>"
		expectedSuffix := "</i>"
		expectedContent := html.EscapeString(text)
		expected := expectedPrefix + expectedContent + expectedSuffix

		return result == expected
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 100}); err != nil {
		t.Errorf("Property 4 failed: %v", err)
	}
}

// Property 5: Code Formatting Conversion
func TestProperty5_CodeFormattingConversion(t *testing.T) {
	property := func(text string) bool {
		result := FormatCode(text)
		expectedPrefix := "<pre>"
		expectedSuffix := "</pre>"
		expectedContent := html.EscapeString(text)
		expected := expectedPrefix + expectedContent + expectedSuffix

		return result == expected
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 100}); err != nil {
		t.Errorf("Property 5 failed: %v", err)
	}
}

// Unit tests for specific examples
func TestFormatBold(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello", "<b>hello</b>"},
		{"test & data", "<b>test &amp; data</b>"},
		{"<script>", "<b>&lt;script&gt;</b>"},
		{"", "<b></b>"},
	}

	for _, test := range tests {
		result := FormatBold(test.input)
		if result != test.expected {
			t.Errorf("FormatBold(%q) = %q, want %q", test.input, result, test.expected)
		}
	}
}

func TestFormatItalic(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello", "<i>hello</i>"},
		{"test & data", "<i>test &amp; data</i>"},
		{"<script>", "<i>&lt;script&gt;</i>"},
		{"", "<i></i>"},
	}

	for _, test := range tests {
		result := FormatItalic(test.input)
		if result != test.expected {
			t.Errorf("FormatItalic(%q) = %q, want %q", test.input, result, test.expected)
		}
	}
}

func TestFormatCode(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello", "<pre>hello</pre>"},
		{"test & data", "<pre>test &amp; data</pre>"},
		{"<script>", "<pre>&lt;script&gt;</pre>"},
		{"", "<pre></pre>"},
	}

	for _, test := range tests {
		result := FormatCode(test.input)
		if result != test.expected {
			t.Errorf("FormatCode(%q) = %q, want %q", test.input, result, test.expected)
		}
	}
}

func TestFormatLink(t *testing.T) {
	tests := []struct {
		text     string
		url      string
		expected string
	}{
		{"Google", "https://google.com", "<a href=\"https://google.com\">Google</a>"},
		{"test & data", "https://example.com?a=1&b=2", "<a href=\"https://example.com?a=1&amp;b=2\">test &amp; data</a>"},
		{"<script>", "javascript:alert(1)", "<a href=\"javascript:alert(1)\">&lt;script&gt;</a>"},
		{"", "", "<a href=\"\"></a>"},
	}

	for _, test := range tests {
		result := FormatLink(test.text, test.url)
		if result != test.expected {
			t.Errorf("FormatLink(%q, %q) = %q, want %q", test.text, test.url, result, test.expected)
		}
	}
}

func TestProperty7_HTMLCharacterEscaping(t *testing.T) {
	property := func(text string) bool {
		// Test that HTML-specific characters are properly escaped in all formatting functions
		htmlChars := []rune{'&', '<', '>', '"'}

		// Check if the input contains any HTML characters
		containsHTMLChars := false
		for _, char := range text {
			for _, htmlChar := range htmlChars {
				if char == htmlChar {
					containsHTMLChars = true
					break
				}
			}
			if containsHTMLChars {
				break
			}
		}

		if !containsHTMLChars {
			return true // Skip if no HTML characters to test
		}

		// Test all formatting functions properly escape HTML characters
		boldResult := FormatBold(text)
		italicResult := FormatItalic(text)
		codeResult := FormatCode(text)
		linkResult := FormatLink(text, "http://example.com")

		expectedEscaped := html.EscapeString(text)

		// Verify that the escaped content appears in the formatted results
		return strings.Contains(boldResult, expectedEscaped) &&
			strings.Contains(italicResult, expectedEscaped) &&
			strings.Contains(codeResult, expectedEscaped) &&
			strings.Contains(linkResult, expectedEscaped)
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 100}); err != nil {
		t.Errorf("Property 7 failed: %v", err)
	}
}

// Unit tests for HTML character escaping
func TestHTMLCharacterEscaping(t *testing.T) {
	tests := []struct {
		input    string
		expected map[string]string
	}{
		{
			"test & data",
			map[string]string{
				"bold":   "<b>test &amp; data</b>",
				"italic": "<i>test &amp; data</i>",
				"code":   "<pre>test &amp; data</pre>",
			},
		},
		{
			"<script>alert('xss')</script>",
			map[string]string{
				"bold":   "<b>&lt;script&gt;alert(&#39;xss&#39;)&lt;/script&gt;</b>",
				"italic": "<i>&lt;script&gt;alert(&#39;xss&#39;)&lt;/script&gt;</i>",
				"code":   "<pre>&lt;script&gt;alert(&#39;xss&#39;)&lt;/script&gt;</pre>",
			},
		},
		{
			"\"quoted text\"",
			map[string]string{
				"bold":   "<b>&#34;quoted text&#34;</b>",
				"italic": "<i>&#34;quoted text&#34;</i>",
				"code":   "<pre>&#34;quoted text&#34;</pre>",
			},
		},
	}

	for _, test := range tests {
		boldResult := FormatBold(test.input)
		if boldResult != test.expected["bold"] {
			t.Errorf("FormatBold(%q) = %q, want %q", test.input, boldResult, test.expected["bold"])
		}

		italicResult := FormatItalic(test.input)
		if italicResult != test.expected["italic"] {
			t.Errorf("FormatItalic(%q) = %q, want %q", test.input, italicResult, test.expected["italic"])
		}

		codeResult := FormatCode(test.input)
		if codeResult != test.expected["code"] {
			t.Errorf("FormatCode(%q) = %q, want %q", test.input, codeResult, test.expected["code"])
		}
	}
}
