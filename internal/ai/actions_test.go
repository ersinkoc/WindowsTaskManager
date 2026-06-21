package ai

import (
	"strings"
	"testing"
)

func TestParseActionsBlockNoBlock(t *testing.T) {
	got, acts := parseActionsBlock("just text without actions")
	if got != "just text without actions" {
		t.Errorf("got = %q, want unchanged", got)
	}
	if acts != nil {
		t.Errorf("expected nil actions, got %v", acts)
	}
}

func TestParseActionsBlockUnclosedReturnsRaw(t *testing.T) {
	raw := "intro\n<actions>not closed"
	got, acts := parseActionsBlock(raw)
	if got != raw {
		t.Errorf("got = %q, want raw input", got)
	}
	if acts != nil {
		t.Errorf("expected nil actions, got %v", acts)
	}
}

func TestParseActionsBlockValidJSON(t *testing.T) {
	raw := `Here is some advice.
<actions>[{"type":"ignore","name":"node.exe","reason":"known"}]</actions>`
	got, acts := parseActionsBlock(raw)
	if strings.Contains(got, "<actions>") {
		t.Errorf("block not stripped from answer: %q", got)
	}
	if len(acts) != 1 || acts[0].Type != "ignore" || acts[0].Name != "node.exe" {
		t.Errorf("unexpected actions: %+v", acts)
	}
	if acts[0].ID == "" {
		t.Error("expected non-empty ID")
	}
}

func TestParseActionsBlockFencedJSON(t *testing.T) {
	raw := "advice\n<actions>```json\n[{\"type\":\"protect\",\"name\":\"chrome.exe\",\"reason\":\"browser\"}]\n```</actions>"
	got, acts := parseActionsBlock(raw)
	if strings.Contains(got, "<actions>") {
		t.Errorf("fenced block not stripped: %q", got)
	}
	if len(acts) != 1 || acts[0].Type != "protect" {
		t.Errorf("unexpected actions: %+v", acts)
	}
}

func TestParseActionsBlockEmptyArray(t *testing.T) {
	raw := "advice\n<actions>[]</actions>"
	got, acts := parseActionsBlock(raw)
	if strings.Contains(got, "<actions>") {
		t.Errorf("empty array block not stripped: %q", got)
	}
	if acts != nil {
		t.Errorf("expected nil actions for empty array, got %v", acts)
	}
}

func TestParseActionsBlockEmptyBody(t *testing.T) {
	raw := "advice\n<actions></actions>"
	got, acts := parseActionsBlock(raw)
	if strings.Contains(got, "<actions>") {
		t.Errorf("empty block not stripped: %q", got)
	}
	if acts != nil {
		t.Errorf("expected nil actions for empty body, got %v", acts)
	}
}

func TestParseActionsBlockMalformedJSONLeavesMarker(t *testing.T) {
	raw := "advice\n<actions>not json</actions>"
	got, acts := parseActionsBlock(raw)
	if !strings.Contains(got, "(actions block parse error:") {
		t.Errorf("expected parse error marker in output, got %q", got)
	}
	if acts != nil {
		t.Errorf("expected nil actions on parse error, got %v", acts)
	}
}

func TestParseActionsBlockUnknownTypeFiltered(t *testing.T) {
	raw := "advice\n<actions>[{\"type\":\"unknown\",\"name\":\"x\"},{\"type\":\"ignore\",\"name\":\"y\"}]</actions>"
	_, acts := parseActionsBlock(raw)
	if len(acts) != 1 {
		t.Fatalf("expected 1 valid action, got %d: %+v", len(acts), acts)
	}
	if acts[0].Type != "ignore" {
		t.Errorf("unexpected kept action: %+v", acts[0])
	}
}

func TestParseActionsBlockUppercaseTypeLowercased(t *testing.T) {
	raw := "advice\n<actions>[{\"type\":\"IGNORE\",\"name\":\"x\"}]</actions>"
	_, acts := parseActionsBlock(raw)
	if len(acts) != 1 || acts[0].Type != "ignore" {
		t.Errorf("expected lowercase type, got %+v", acts)
	}
}

func TestParseActionsBlockAllKnownTypesAccepted(t *testing.T) {
	for _, typ := range []string{"kill", "suspend", "protect", "ignore", "add_rule"} {
		raw := "a\n<actions>[{\"type\":\"" + typ + "\",\"name\":\"x\"}]</actions>"
		_, acts := parseActionsBlock(raw)
		if len(acts) != 1 || acts[0].Type != typ {
			t.Errorf("type %q: got %+v", typ, acts)
		}
	}
}

func TestHashSuggestionStableAndUnique(t *testing.T) {
	a := Suggestion{Type: "ignore", Name: "node.exe", PID: 123}
	b := Suggestion{Type: "ignore", Name: "node.exe", PID: 123}
	if hashSuggestion(a) != hashSuggestion(b) {
		t.Error("same input should yield same hash")
	}
	c := Suggestion{Type: "ignore", Name: "other.exe", PID: 123}
	if hashSuggestion(a) == hashSuggestion(c) {
		t.Error("different name should yield different hash")
	}
	d := Suggestion{Type: "protect", Name: "node.exe", PID: 123}
	if hashSuggestion(a) == hashSuggestion(d) {
		t.Error("different type should yield different hash")
	}
	e := Suggestion{Type: "ignore", Name: "node.exe", PID: 999}
	if hashSuggestion(a) == hashSuggestion(e) {
		t.Error("different pid should yield different hash")
	}
}

func TestHashSuggestionWithRule(t *testing.T) {
	rule := &RuleSuggestion{Name: "cpu-rule", Match: "node.exe", Threshold: 90.0}
	withRule := Suggestion{Type: "add_rule", Name: "node.exe", Rule: rule}
	withoutRule := Suggestion{Type: "add_rule", Name: "node.exe"}
	if hashSuggestion(withRule) == hashSuggestion(withoutRule) {
		t.Error("with-rule vs without-rule should yield different hashes")
	}
	// Different rule threshold
	rule2 := &RuleSuggestion{Name: "cpu-rule", Match: "node.exe", Threshold: 80.0}
	withRule2 := Suggestion{Type: "add_rule", Name: "node.exe", Rule: rule2}
	if hashSuggestion(withRule) == hashSuggestion(withRule2) {
		t.Error("different rule threshold should yield different hashes")
	}
	// Different rule name
	rule3 := &RuleSuggestion{Name: "mem-rule", Match: "node.exe", Threshold: 90.0}
	withRule3 := Suggestion{Type: "add_rule", Name: "node.exe", Rule: rule3}
	if hashSuggestion(withRule) == hashSuggestion(withRule3) {
		t.Error("different rule name should yield different hashes")
	}
	// Different rule match
	rule4 := &RuleSuggestion{Name: "cpu-rule", Match: "chrome.exe", Threshold: 90.0}
	withRule4 := Suggestion{Type: "add_rule", Name: "node.exe", Rule: rule4}
	if hashSuggestion(withRule) == hashSuggestion(withRule4) {
		t.Error("different rule match should yield different hashes")
	}
}

func TestHashSuggestionLength12(t *testing.T) {
	got := hashSuggestion(Suggestion{Type: "ignore", Name: "x"})
	if len(got) != 12 {
		t.Errorf("hash length = %d, want 12", len(got))
	}
}
