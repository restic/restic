# Build doc.go from template and snippets.

SHELL=/bin/bash

../doc.go: doc-snippets.go doc.template snipdoc.awk
	@tmp=$$(mktemp) && \
	awk -f snipdoc.awk doc-snippets.go doc.template > $$tmp && \
	chmod +w ../doc.go && \
	mv $$tmp ../doc.go && \
	chmod -w ../doc.go
