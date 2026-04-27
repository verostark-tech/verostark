// Package ai wraps the Claude API and generates plain-English explanations
// of STIM royalty deviations for copyright administrators.
package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"encore.dev/beta/errs"
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

var secrets struct {
	VtClaudeSecretKey string
}

// model is fixed per CLAUDE.md stack — do not change without explicit instruction.
const model anthropic.Model = "claude-sonnet-4-20250514"

//encore:service
type Service struct {
	client anthropic.Client
}

func initService() (*Service, error) {
	client := anthropic.NewClient(option.WithAPIKey(secrets.VtClaudeSecretKey))
	return &Service{client: client}, nil
}

// ExplainRequest carries the deviation data for one detection flag.
type ExplainRequest struct {
	WorkTitle       string  `json:"work_title"`
	ISWC            string  `json:"iswc"`
	RightType       string  `json:"right_type"`
	Period          string  `json:"period"`
	Severity        string  `json:"severity"`
	GrossSEK        float64 `json:"gross_sek"`
	ControlledShare float64 `json:"controlled_share"` // 0–1
	ExpectedSEK     float64 `json:"expected_sek"`
	ReceivedSEK     float64 `json:"received_sek"`
	DeviationSEK    float64 `json:"deviation_sek"` // negative = underpayment
	DeviationPct    float64 `json:"deviation_pct"` // fractional, signed (−0.25 = 25% under)
}

// ExplainResponse carries the why-explanation and one suggested next step.
type ExplainResponse struct {
	Explanation string `json:"explanation"`
	NextStep    string `json:"next_step"`
}

// ExplainDeviation calls the Claude API and returns a plain-English explanation
// and a concrete next-step recommendation for the given deviation flag.
// Called exclusively by the detection service — not exposed externally.
//
//encore:api private
func (s *Service) ExplainDeviation(ctx context.Context, req *ExplainRequest) (*ExplainResponse, error) {
	resp, err := s.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     model,
		MaxTokens: 1024,
		System: []anthropic.TextBlockParam{
			{Text: systemPrompt},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(buildPrompt(req))),
		},
	})
	if err != nil {
		return nil, &errs.Error{
			Code:    errs.Internal,
			Message: "could not generate explanation — please try again",
		}
	}

	var raw string
	for _, block := range resp.Content {
		if tb, ok := block.AsAny().(anthropic.TextBlock); ok {
			raw = tb.Text
			break
		}
	}

	var result ExplainResponse
	if err := json.Unmarshal([]byte(stripCodeFences(raw)), &result); err != nil {
		return nil, &errs.Error{
			Code:    errs.Internal,
			Message: "could not parse explanation — please try again",
		}
	}

	return &result, nil
}

const systemPrompt = `You are a royalty analysis assistant for an independent music publisher.

Your job is to explain why a payment deviation occurred on a royalty statement and suggest what the publisher should investigate next.

The expected payment is always calculated as: gross × controlled share × distribution key. For mechanical rights on a Nordic publisher registration the standard distribution key is one-third.

A deviation means one of three things most likely occurred:
1. A conflicting registration from another society overrode the local registration during the distribution period.
2. The controlled share on record at the collecting society differs from what the publisher holds on file.
3. An unusual distribution rule was applied to this work for this period.

Rules for your response:
- Write in plain English only. A copyright administrator — not an engineer — must understand every word.
- Never name a specific collecting society as having made an error.
- Never name ICE Cube, PRS, GEMA, or any other institution as the cause. Describe the data pattern — not the actor.
- Say "the collecting society" or "the distribution" — never a society name.
- Do not use words like: distribution key, org_id, algorithm, system, database. Use: payment percentage, share, amount, registration, claim.
- Always end with one suggested next step the administrator can take immediately. Frame it as a suggestion not an instruction.

Respond with valid JSON only. No markdown. No code fences.
Exactly two string fields: "explanation" (why this most likely occurred) and "next_step" (one concrete suggested action).`

func buildPrompt(req *ExplainRequest) string {
	direction := "overpayment"
	if req.DeviationSEK < 0 {
		direction = "underpayment"
	}
	absPct := req.DeviationPct
	if absPct < 0 {
		absPct = -absPct
	}

	return fmt.Sprintf(`A %s was detected on this royalty statement line:

Work: %s (ISWC: %s)
Period: %s
Right type: %s

Gross collected: %.2f SEK
Publisher controlled share: %.0f%%
Expected payment: %.2f × %.0f%% × 33.33%% = %.2f SEK
Actual payment received: %.2f SEK
Difference: %.2f SEK (%.1f%% above expected)
Severity: %s

Explain in 2 to 3 sentences why this deviation most likely occurred. Reason through which of the three inputs — the gross amount collected, the controlled share, or the payment percentage applied — is most likely the cause.

Then provide one concrete suggested next step the administrator can take to investigate or resolve this.

Respond with valid JSON only: {"explanation": "...", "next_step": "..."}`,
		direction,
		req.WorkTitle, req.ISWC,
		req.Period,
		req.RightType,
		req.GrossSEK,
		req.ControlledShare*100,
		req.GrossSEK, req.ControlledShare*100, req.ExpectedSEK,
		req.ReceivedSEK,
		req.DeviationSEK, absPct*100,
		req.Severity,
	)
}

// stripCodeFences removes markdown code fences that Claude occasionally wraps around JSON.
func stripCodeFences(text string) string {
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "```") {
		if i := strings.Index(text, "\n"); i != -1 {
			text = text[i+1:]
		}
		text = strings.TrimSuffix(strings.TrimSpace(text), "```")
		text = strings.TrimSpace(text)
	}
	return text
}
