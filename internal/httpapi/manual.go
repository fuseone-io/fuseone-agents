package httpapi

import (
	"context"
	"fmt"
	"net/http"

	"github.com/fuseone/agents/docs"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

// defaultLocale is what a reader gets when they ask for nothing, or for a
// translation the manual does not have.
const defaultLocale = "pt-BR"

/*
GetManual lists the pages, in the order the index shows them.

Without bodies. The manual is meant to grow, and an index that carries every
page turns opening the menu into downloading the book — on a screen whose whole
job is to let somebody choose one thing to read.
*/
func (s *Server) GetManual(
	_ context.Context, req openapi.GetManualRequestObject,
) (openapi.GetManualResponseObject, error) {
	locale := localeOf(string(deref(req.Params.Locale)))
	pages, err := docs.Manual(locale)
	if err != nil {
		return nil, fmt.Errorf("manual %s: %w", locale, err)
	}

	entries := make([]openapi.ManualEntry, 0, len(pages))
	for _, page := range pages {
		entries = append(entries, openapi.ManualEntry{
			Slug: page.Slug, Title: page.Title,
			Summary: page.Summary, Section: page.Section, Tags: page.Tags,
			Order: page.Order, Headings: headingsOut(page.Headings),
		})
	}
	return openapi.GetManual200JSONResponse{Locale: locale, Pages: entries}, nil
}

/*
GetManualPage returns one page as it was written.

The slug is matched against the pages the manual has rather than joined onto a
path. A name that is looked up cannot escape the directory it is looked up in,
which is a property of the lookup and not of how carefully the string was
cleaned.
*/
func (s *Server) GetManualPage(
	_ context.Context, req openapi.GetManualPageRequestObject,
) (openapi.GetManualPageResponseObject, error) {
	locale := localeOf(string(deref(req.Params.Locale)))
	pages, err := docs.Manual(locale)
	if err != nil {
		return nil, fmt.Errorf("manual %s: %w", locale, err)
	}

	for _, page := range pages {
		if page.Slug != req.Slug {
			continue
		}
		return openapi.GetManualPage200JSONResponse{
			Slug: page.Slug, Title: page.Title, Summary: page.Summary,
			Section: page.Section, Tags: page.Tags, Order: page.Order,
			Headings: headingsOut(page.Headings), Body: page.Body,
		}, nil
	}
	return openapi.GetManualPage404ApplicationProblemPlusJSONResponse(
		refusal(http.StatusNotFound, CodeNotFound, "Not found",
			fmt.Sprintf("the manual has no page %q in %s", req.Slug, locale)),
	), nil
}

// localeOf keeps the served locale to the ones that exist. Falling back rather
// than refusing: a reader whose browser asks for something the manual has not
// been translated into wants the manual, not an error.
func localeOf(asked string) string {
	for _, known := range []string{"pt-BR", "en-US"} {
		if asked == known {
			return asked
		}
	}
	return defaultLocale
}

func headingsOut(in []docs.Heading) []openapi.ManualHeading {
	out := make([]openapi.ManualHeading, 0, len(in))
	for _, h := range in {
		out = append(out, openapi.ManualHeading{Id: h.ID, Title: h.Title, Level: h.Level})
	}
	return out
}

func deref[T any](p *T) T {
	if p == nil {
		var zero T
		return zero
	}
	return *p
}
