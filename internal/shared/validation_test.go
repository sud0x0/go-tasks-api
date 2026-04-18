package shared

import (
	"testing"

	"github.com/go-playground/validator/v10"
)

func TestRuneCountLen(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int
	}{
		{"empty string", "", 0},
		{"ascii only", "hello", 5},
		{"multi-byte chars", "日本語", 3},         // 3 Japanese characters = 3 runes
		{"mixed ascii and unicode", "ab日c", 4}, // 2 ascii + 1 Japanese + 1 ascii = 4 runes
		{"combining chars", "e\u0301", 2},      // e + combining acute = 2 runes (not 1 grapheme)
		{"cyrillic", "Привет", 6},              // 6 Cyrillic characters
		{"arabic", "مرحبا", 5},                 // 5 Arabic characters
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RuneCountLen(tt.input)
			if got != tt.want {
				t.Errorf("RuneCountLen(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestRuneCountLen_VsByteLen(t *testing.T) {
	// Demonstrate the difference between rune count and byte length
	tests := []struct {
		input   string
		byteLen int
		runeLen int
		desc    string
	}{
		{"hello", 5, 5, "ascii: same length"},
		{"日本語", 9, 3, "Japanese: 3 bytes per char"},
		{"é", 2, 1, "accented char: 2 bytes, 1 rune"},
		{"a日b", 5, 3, "mixed: 1+3+1 bytes = 5, 3 runes"},
		{"Привет", 12, 6, "Cyrillic: 2 bytes per char"},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			gotBytes := len(tt.input)
			gotRunes := RuneCountLen(tt.input)

			if gotBytes != tt.byteLen {
				t.Errorf("len(%q) = %d, want %d", tt.input, gotBytes, tt.byteLen)
			}
			if gotRunes != tt.runeLen {
				t.Errorf("RuneCountLen(%q) = %d, want %d", tt.input, gotRunes, tt.runeLen)
			}
		})
	}
}

func TestRegisterRuneLenValidators(t *testing.T) {
	v := validator.New()
	if err := RegisterRuneLenValidators(v); err != nil {
		t.Fatalf("RegisterRuneLenValidators() error = %v", err)
	}

	// rune_max tests
	t.Run("rune_max", func(t *testing.T) {
		type TestStruct struct {
			Value string `validate:"rune_max=5"`
		}

		tests := []struct {
			name    string
			value   string
			wantErr bool
		}{
			{"ascii within limit", "hello", false},
			{"ascii at limit", "12345", false},
			{"ascii over limit", "123456", true},
			{"unicode within limit", "日本語", false}, // 3 runes
			{"unicode at limit", "日本語ab", false},   // 5 runes
			{"unicode over limit", "日本語abc", true}, // 6 runes
			{"empty string", "", false},
			{"cyrillic within limit", "Прив", false}, // 4 runes
			{"cyrillic at limit", "Приве", false},    // 5 runes
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				err := v.Struct(TestStruct{Value: tt.value})
				if (err != nil) != tt.wantErr {
					t.Errorf("validate(%q) error = %v, wantErr %v", tt.value, err, tt.wantErr)
				}
			})
		}
	})

	// rune_min tests
	t.Run("rune_min", func(t *testing.T) {
		type TestStruct struct {
			Value string `validate:"rune_min=3"`
		}

		tests := []struct {
			name    string
			value   string
			wantErr bool
		}{
			{"ascii at min", "abc", false},
			{"ascii above min", "abcd", false},
			{"ascii below min", "ab", true},
			{"unicode at min", "日本語", false},  // 3 runes
			{"unicode below min", "日本", true}, // 2 runes
			{"empty string", "", true},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				err := v.Struct(TestStruct{Value: tt.value})
				if (err != nil) != tt.wantErr {
					t.Errorf("validate(%q) error = %v, wantErr %v", tt.value, err, tt.wantErr)
				}
			})
		}
	})

	// rune_len tests
	t.Run("rune_len", func(t *testing.T) {
		type TestStruct struct {
			Value string `validate:"rune_len=3"`
		}

		tests := []struct {
			name    string
			value   string
			wantErr bool
		}{
			{"ascii exact", "abc", false},
			{"ascii too short", "ab", true},
			{"ascii too long", "abcd", true},
			{"unicode exact", "日本語", false}, // 3 runes
			{"unicode too short", "日本", true},
			{"unicode too long", "日本語あ", true},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				err := v.Struct(TestStruct{Value: tt.value})
				if (err != nil) != tt.wantErr {
					t.Errorf("validate(%q) error = %v, wantErr %v", tt.value, err, tt.wantErr)
				}
			})
		}
	})

	// Combined validators
	t.Run("combined rune_min and rune_max", func(t *testing.T) {
		type TestStruct struct {
			Value string `validate:"rune_min=2,rune_max=5"`
		}

		tests := []struct {
			name    string
			value   string
			wantErr bool
		}{
			{"at min", "ab", false},
			{"at max", "abcde", false},
			{"in range", "abc", false},
			{"below min", "a", true},
			{"above max", "abcdef", true},
			{"unicode in range", "日本語", false}, // 3 runes
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				err := v.Struct(TestStruct{Value: tt.value})
				if (err != nil) != tt.wantErr {
					t.Errorf("validate(%q) error = %v, wantErr %v", tt.value, err, tt.wantErr)
				}
			})
		}
	})
}

func TestRegisterRuneLenValidators_WithRequired(t *testing.T) {
	v := validator.New()
	if err := RegisterRuneLenValidators(v); err != nil {
		t.Fatalf("RegisterRuneLenValidators() error = %v", err)
	}

	type TestStruct struct {
		Value string `validate:"required,rune_max=100"`
	}

	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"valid", "hello", false},
		{"empty fails required", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Struct(TestStruct{Value: tt.value})
			if (err != nil) != tt.wantErr {
				t.Errorf("validate(%q) error = %v, wantErr %v", tt.value, err, tt.wantErr)
			}
		})
	}
}

func TestIsValidUUID(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"valid UUID v4", "550e8400-e29b-41d4-a716-446655440000", true},
		{"valid UUID lowercase", "550e8400-e29b-41d4-a716-446655440000", true},
		{"valid UUID uppercase", "550E8400-E29B-41D4-A716-446655440000", true},
		{"empty string", "", false},
		{"too short", "550e8400-e29b-41d4-a716", false},
		{"invalid chars", "550e8400-e29b-41d4-a716-44665544000g", false},
		{"no dashes", "550e8400e29b41d4a716446655440000", true}, // uuid.Parse accepts this
		{"random string", "not-a-uuid", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsValidUUID(tt.input)
			if got != tt.want {
				t.Errorf("IsValidUUID(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestSanitiseNullBytes(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"no null bytes", "hello world", "hello world"},
		{"single null byte", "hello\x00world", "helloworld"},
		{"multiple null bytes", "a\x00b\x00c", "abc"},
		{"only null bytes", "\x00\x00\x00", ""},
		{"empty string", "", ""},
		{"null at start", "\x00hello", "hello"},
		{"null at end", "hello\x00", "hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitiseNullBytes(tt.input)
			if got != tt.want {
				t.Errorf("SanitiseNullBytes(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestValidatePagination(t *testing.T) {
	tests := []struct {
		name      string
		limitStr  string
		offsetStr string
		wantLimit int
		wantOff   int
		wantErr   bool
	}{
		{"defaults", "", "", DefaultPageSize, 0, false},
		{"valid limit", "50", "", 50, 0, false},
		{"valid offset", "", "100", DefaultPageSize, 100, false},
		{"both set", "25", "50", 25, 50, false},
		{"max limit", "100", "", 100, 0, false},
		{"over max limit", "101", "", 0, 0, true},
		{"negative limit", "-1", "", 0, 0, true},
		{"zero limit", "0", "", 0, 0, true},
		{"negative offset", "", "-1", 0, 0, true},
		{"offset capped at max", "", "20000", DefaultPageSize, MaxPageOffset, false},
		{"invalid limit", "abc", "", 0, 0, true},
		{"invalid offset", "", "xyz", 0, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limit, offset, err := ValidatePagination(tt.limitStr, tt.offsetStr)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePagination() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if limit != tt.wantLimit {
					t.Errorf("limit = %d, want %d", limit, tt.wantLimit)
				}
				if offset != tt.wantOff {
					t.Errorf("offset = %d, want %d", offset, tt.wantOff)
				}
			}
		})
	}
}
