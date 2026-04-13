package validators

import "testing"

// Known valid ISWC: T-010.269.050-5 (Happy Birthday To You)
// Check: digits=0102690505, total=1+0+2+0+8+30+54+0+40+0=135, checksum=(10-5)%10=5 ✓

func TestValidateISWC(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid — Happy Birthday", "T0102690505", false},
		{"invalid check digit", "T0102690506", true},
		{"too short", "T012345678", true},
		{"wrong prefix", "A0102690505", true},
		{"non-numeric", "TABCDEFGHIJ", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateISWC(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateISWC(%q) error=%v, wantErr=%v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestNormaliseISWC(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"T-010.269.050-5", "T0102690505"},
		{"T-010269050-5", "T0102690505"},
		{"T0102690505", "T0102690505"},
		{"t-010.269.050-5", "T0102690505"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := NormaliseISWC(tt.input)
			if got != tt.want {
				t.Errorf("NormaliseISWC(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// IPI Base I-000000000-8:
// digits=0000000008, total=2+0=2, checksum=(10-2)%10=8 ✓

func TestValidateIPIBase(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid", "I-000000000-8", false},
		{"invalid check digit", "I-000000000-0", true},
		{"wrong format", "000000000", true},
		{"lowercase I", "i-000000000-8", true}, // must be normalised first
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateIPIBase(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateIPIBase(%q) error=%v, wantErr=%v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestNormaliseIPIBase(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"I-000000000-8", "I-000000000-8"},
		{"I.000000000.8", "I-000000000-8"},
		{"i-000000000-8", "I-000000000-8"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := NormaliseIPIBase(tt.input)
			if got != tt.want {
				t.Errorf("NormaliseIPIBase(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// IPI Name "00000000000": digits[:9]="000000000", total=0 → check="00" ✓
// IPI Name "00000000199": digits[:9]="000000001", total=2 → (101-2)%100=99 → check="99" ✓

func TestValidateIPIName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid all zeros", "00000000000", false},
		{"valid computed", "00000000199", false},
		{"wrong check digits", "00000000001", true},
		{"too short", "0000000000", true},
		{"non-numeric", "0000000000A", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateIPIName(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateIPIName(%q) error=%v, wantErr=%v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestNormaliseIPIName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"00000000000", "00000000000"},
		{"199", "00000000199"},
		{"0", "00000000000"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := NormaliseIPIName(tt.input)
			if got != tt.want {
				t.Errorf("NormaliseIPIName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
