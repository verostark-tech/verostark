package statements

import (
	"strings"
	"testing"
)

func TestParseSTIM_EmptyInput(t *testing.T) {
	_, err := parseSTIM(strings.NewReader(""))
	if err == nil {
		t.Fatal("expected error for empty input, got nil")
	}
}

func TestParseSTIM_MissingRequiredColumn(t *testing.T) {
	bad := "Work ID,Title,Net Amount\nBM001,Sommarnatt,15.81\n"
	_, err := parseSTIM(strings.NewReader(bad))
	if err == nil {
		t.Fatal("expected error for missing Gross column, got nil")
	}
}
