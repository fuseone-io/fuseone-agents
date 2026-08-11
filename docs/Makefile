# Regeneração dos documentos do repositório.
# `make pdf` reconstrói o PDF a partir do HTML — o HTML é a fonte de verdade.

CHROME ?= $(shell command -v chromium 2>/dev/null || command -v google-chrome-stable 2>/dev/null)
DOCS   := $(CURDIR)

SOURCES := $(wildcard *.html)
PDFS    := $(SOURCES:.html=.pdf)

.PHONY: pdf clean

pdf: $(PDFS)

%.pdf: %.html
	@test -n "$(CHROME)" || { echo "chromium/chrome não encontrado no PATH"; exit 1; }
	$(CHROME) --headless --disable-gpu --no-sandbox --no-pdf-header-footer \
	  --run-all-compositor-stages-before-draw --virtual-time-budget=8000 \
	  --print-to-pdf=$(DOCS)/$@ file://$(DOCS)/$<
	@echo "gerado: $(DOCS)/$@"

clean:
	rm -f $(PDFS)
