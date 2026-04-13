package rules

import (
	"strings"
	"testing"
)

func TestRecommend_FullMatrix(t *testing.T) {
	// All reachable combinations in V0.1. Only HIGH and CRITICAL are flagged
	// at the current 25% threshold, but all four severity levels must return
	// a non-empty, non-fallback recommendation.
	cases := []struct {
		severity    string
		patternType string
	}{
		{SeverityCritical, "underpayment"},
		{SeverityCritical, "overpayment"},
		{SeverityHigh, "underpayment"},
		{SeverityHigh, "overpayment"},
		{SeverityMedium, "underpayment"},
		{SeverityMedium, "overpayment"},
		{SeverityLow, "underpayment"},
		{SeverityLow, "overpayment"},
	}

	fallback := "Review this deviation and contact STIM's publisher relations team if you cannot identify the cause."

	for _, tc := range cases {
		t.Run(tc.severity+"_"+tc.patternType, func(t *testing.T) {
			got := Recommend(tc.severity, tc.patternType)
			if got == "" {
				t.Fatal("recommendation is empty")
			}
			if got == fallback {
				t.Errorf("got fallback for known input {%s, %s}", tc.severity, tc.patternType)
			}
		})
	}
}

func TestRecommend_Fallback(t *testing.T) {
	cases := []struct{ severity, patternType string }{
		{"", "underpayment"},
		{SeverityCritical, ""},
		{"UNKNOWN", "underpayment"},
		{SeverityHigh, "sync"},
	}
	for _, tc := range cases {
		got := Recommend(tc.severity, tc.patternType)
		if got == "" {
			t.Errorf("{%q, %q}: fallback must not be empty", tc.severity, tc.patternType)
		}
	}
}

func TestRecommend_CriticalUnderpaymentMentionsDispute(t *testing.T) {
	// CRITICAL underpayment must always direct the administrator toward a formal dispute.
	got := Recommend(SeverityCritical, "underpayment")
	if !strings.Contains(strings.ToLower(got), "dispute") {
		t.Errorf("CRITICAL underpayment recommendation should mention a dispute, got: %s", got)
	}
}

func TestRecommend_CriticalOverpaymentMentionsContact(t *testing.T) {
	// CRITICAL overpayment must direct the administrator to contact STIM.
	got := Recommend(SeverityCritical, "overpayment")
	if !strings.Contains(strings.ToLower(got), "contact") {
		t.Errorf("CRITICAL overpayment recommendation should mention contacting STIM, got: %s", got)
	}
}
