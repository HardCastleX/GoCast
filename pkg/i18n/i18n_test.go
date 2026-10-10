package i18n

import "testing"

func TestCatalogsHaveMatchingKeys(t *testing.T) {
	en := messages[English]
	es := messages[Spanish]
	if len(en) == 0 || len(es) == 0 {
		t.Fatal("catalogs were not loaded")
	}
	for key := range en {
		if _, ok := es[key]; !ok {
			t.Errorf("key %q is missing from the Spanish catalog", key)
		}
	}
	for key := range es {
		if _, ok := en[key]; !ok {
			t.Errorf("key %q is missing from the English catalog", key)
		}
	}
}

func TestTTranslatesAndFallsBackToKey(t *testing.T) {
	SetLanguage(Spanish)
	if got, want := T("btn.start"), messages[Spanish]["btn.start"]; got != want {
		t.Fatalf("T(btn.start) = %q, want %q", got, want)
	}
	if got := T("missing.key"); got != "missing.key" {
		t.Fatalf("expected key fallback, got %q", got)
	}
}

func TestSetLanguageIgnoresUnknown(t *testing.T) {
	SetLanguage(English)
	SetLanguage("fr")
	if Current() != English {
		t.Fatalf("expected %q, got %q", English, Current())
	}
}

func TestTForTranslatesRequestedLanguage(t *testing.T) {
	if got, want := TFor(Spanish, "lang.prompt"), messages[Spanish]["lang.prompt"]; got != want {
		t.Fatalf("TFor(es) = %q, want %q", got, want)
	}
	if got, want := TFor(English, "lang.prompt"), messages[English]["lang.prompt"]; got != want {
		t.Fatalf("TFor(en) = %q, want %q", got, want)
	}
	if got, want := TFor("fr", "lang.prompt"), messages[English]["lang.prompt"]; got != want {
		t.Fatalf("TFor fallback = %q, want %q", got, want)
	}
}

func TestSystemLanguage(t *testing.T) {
	lang := SystemLanguage()
	if lang != English && lang != Spanish {
		t.Fatalf("unexpected system language %q", lang)
	}
	t.Logf("detected system language: %q", lang)
}
