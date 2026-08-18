package httpapi

import (
	"context"
	"testing"

	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
)

func manualServer() *Server { return NewServer(ledger.NewMemory(), "test") }

func TestGetManual_unknownLocale_fallsBackRatherThanFailing(t *testing.T) {
	t.Parallel()

	// A browser asking for a translation the manual does not have should get
	// the manual. Refusing would make the language header a way to break a
	// screen that carries no data at all.
	unknown := openapi.GetManualParamsLocale("fr-FR")
	got, err := manualServer().GetManual(context.Background(), openapi.GetManualRequestObject{
		Params: openapi.GetManualParams{Locale: &unknown},
	})
	if err != nil {
		t.Fatalf("index: %v", err)
	}

	index, ok := got.(openapi.GetManual200JSONResponse)
	if !ok {
		t.Fatalf("got %T, want the index", got)
	}
	if len(index.Pages) == 0 {
		t.Fatal("fell back to a locale with no pages")
	}
	if index.Locale == "fr-FR" {
		t.Error("says it served fr-FR, which it does not have")
	}
}

func TestGetManual_indexCarriesNoBodies(t *testing.T) {
	t.Parallel()

	// The index is a list to choose from, and the manual will grow. Shipping
	// every body with it turns opening the menu into downloading the book.
	got, err := manualServer().GetManual(context.Background(), openapi.GetManualRequestObject{})
	if err != nil {
		t.Fatalf("index: %v", err)
	}
	index := got.(openapi.GetManual200JSONResponse)

	for i, page := range index.Pages {
		if page.Title == "" || page.Summary == "" {
			t.Errorf("page %d has no title or summary to choose it by", i)
		}
		if i > 0 && index.Pages[i-1].Order > page.Order {
			t.Errorf("page %d comes after a higher order", i)
		}
	}
}

func TestGetManualPage_slugTheManualDoesNotHave_is404(t *testing.T) {
	t.Parallel()

	got, err := manualServer().GetManualPage(context.Background(), openapi.GetManualPageRequestObject{
		Slug: "../../etc/passwd",
	})
	if err != nil {
		t.Fatalf("page: %v", err)
	}
	if _, ok := got.(openapi.GetManualPage404ApplicationProblemPlusJSONResponse); !ok {
		t.Fatalf("got %T, want a 404 — a slug is a name, never a path", got)
	}
}

func TestGetManualPage_returnsMarkdownRatherThanMarkup(t *testing.T) {
	t.Parallel()

	got, err := manualServer().GetManualPage(context.Background(), openapi.GetManualPageRequestObject{
		Slug: "agents-and-runs",
	})
	if err != nil {
		t.Fatalf("page: %v", err)
	}
	page, ok := got.(openapi.GetManualPage200JSONResponse)
	if !ok {
		t.Fatalf("got %T, want the page", got)
	}
	if page.Body == "" {
		t.Fatal("the page has no body")
	}
	// Rendering belongs to the reader. A server that had already turned this
	// into markup would be asking the console to trust it about what it put in.
	if page.Body[0] == '<' {
		t.Error("the body opens as markup, not as Markdown")
	}
}
