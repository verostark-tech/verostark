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
	WorkTitle    string  `json:"work_title"`
	ISWC         string  `json:"iswc"`
	RightType    string  `json:"right_type"`
	Period       string  `json:"period"`
	Severity     string  `json:"severity"`
	ExpectedSEK  float64 `json:"expected_sek"`
	ReceivedSEK  float64 `json:"received_sek"`
	DeviationSEK float64 `json:"deviation_sek"` // negative = underpayment
	DeviationPct float64 `json:"deviation_pct"` // fractional, signed (−0.25 = 25% under)
}

// ExplainResponse is stored on the detection_flags row (explanation + recommendation).
type ExplainResponse struct {
	Explanation    string `json:"explanation"`
	Recommendation string `json:"recommendation"`
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

const systemPrompt = `You are a royalty statement analyst for a Nordic music publisher.
Your job is to explain payment deviations to copyright administrators who manage the publisher's catalogue.
Write in plain English. No technical jargon. A copyright administrator who is not a software engineer must understand every word.
Always respond with valid JSON only — no markdown, no code fences — with exactly two string fields: "explanation" and "recommendation".`

func buildPrompt(req *ExplainRequest) string {
	direction := "overpayment"
	if req.DeviationSEK < 0 {
		direction = "underpayment"
	}
	absPct := req.DeviationPct
	if absPct < 0 {
		absPct = -absPct
	}

	return fmt.Sprintf(`A %s was detected on the following STIM royalty line:

Work:       %s (ISWC: %s)
Period:     %s
Right type: %s
Expected:   %.2f SEK
Received:   %.2f SEK
Difference: %.2f SEK (%.1f%%)
Severity:   %s

Write:
- "explanation": 2–3 sentences explaining what this deviation means and what may have caused it. Write for a copyright administrator — no technical language.
- "recommendation": 1–2 sentences telling the administrator exactly what to do next.

Respond with valid JSON only.`,
		direction,
		req.WorkTitle, req.ISWC,
		req.Period,
		req.RightType,
		req.ExpectedSEK,
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
