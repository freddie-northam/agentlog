package render

import (
	"testing"

	"github.com/Ryheff24/agentlog/internal/transcript"
)

func TestAnsweredQuestionShowsTheDecision(t *testing.T) {
	e := transcript.Event{Kind: transcript.KindTool, Tool: "AskUserQuestion", Input: "2 questions", Human: true,
		Answers: []transcript.Answer{{Header: "Scope", Choice: "Only the parser"}}}
	want := "AskUserQuestion answered: Scope: Only the parser"
	if got := eventBody(e, Options{}); got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
	e.Human = false
	if got := eventBody(e, Options{}); got == want {
		t.Errorf("a subagent's answers rendered as human: %q", got)
	}
}
