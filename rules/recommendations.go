package rules

// recommendations maps {severity, patternType} to a concrete action for a
// copyright administrator. These are deterministic — always available regardless
// of AI availability.
//
// patternType is either "underpayment" or "overpayment".
// severity is one of the Severity* constants.
//
// Note: in V0.1 only HIGH and CRITICAL are reachable because flagThreshold is
// 0.25. LOW and MEDIUM are included so this does not break if the threshold changes.
var recommendations = map[string]map[string]string{
	SeverityCritical: {
		"underpayment": "Open a formal dispute with STIM immediately. The shortfall exceeds 50% of the expected payment. In your dispute letter, include the ISWC, the affected distribution period, the expected amount, and the received amount. Request a written explanation of the distribution calculation.",
		"overpayment":  "Contact STIM's publisher relations team before the next distribution cycle. Overpayments of this size are typically reclaimed automatically — notify them now so you can plan for the adjustment. Verify that your registered controlled share for this work is correct.",
	},
	SeverityHigh: {
		"underpayment": "Contact STIM's publisher relations team and request a review of the distribution calculation for this work. Provide the ISWC, the distribution period, and the expected versus received amounts. If the calculation cannot be explained, escalate to a formal dispute.",
		"overpayment":  "Contact STIM to confirm whether a correction is forthcoming. Document the overpayment now. Verify that your manuscript share registration for this work matches your agreement.",
	},
	SeverityMedium: {
		"underpayment": "Review the ISWC, IPI numbers, and controlled manuscript share registered for this work with STIM. If all data is correct and the discrepancy persists in the next period, contact STIM's publisher relations team.",
		"overpayment":  "Verify your manuscript share registration for this work. Minor overpayments are sometimes corrected automatically in the following distribution. If not corrected within two periods, contact STIM.",
	},
	SeverityLow: {
		"underpayment": "Check that the ISWC and controlled manuscript share are correctly registered. Small underpayments can result from rounding in STIM's distribution. Monitor the next distribution before escalating.",
		"overpayment":  "Monitor the next distribution. Small overpayments are typically corrected automatically by STIM in subsequent periods.",
	},
}

// Recommend returns a concrete, actionable recommendation for a copyright
// administrator given the severity and pattern of a detected deviation.
// Falls back to a generic message for unrecognised inputs.
func Recommend(severity, patternType string) string {
	if byPattern, ok := recommendations[severity]; ok {
		if rec, ok := byPattern[patternType]; ok {
			return rec
		}
	}
	return "Review this deviation and contact STIM's publisher relations team if you cannot identify the cause."
}
