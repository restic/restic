include $(GOROOT)/src/Make.inc

TARG=launchpad.net/gocheck

GOFILES=\
	gocheck.go\
	helpers.go\
	run.go\
	checkers.go\
	printer.go\

#TARGDIR=$(GOPATH)/pkg/$(GOOS)_$(GOARCH)
#GCIMPORTS=$(patsubst %,-I %/pkg/$(GOOS)_$(GOARCH),$(subst :, ,$(GOPATH)))
#LDIMPORTS=$(patsubst %,-L %/pkg/$(GOOS)_$(GOARCH),$(subst :, ,$(GOPATH)))

include $(GOROOT)/src/Make.pkg

GOFMT=gofmt

BADFMT=$(shell $(GOFMT) -l $(GOFILES) $(filter-out printer_test.go,$(wildcard *_test.go)))

gofmt: $(BADFMT)
	@for F in $(BADFMT); do $(GOFMT) -w $$F && echo $$F; done

ifneq ($(BADFMT),)
ifneq ($(MAKECMDGOALS),gofmt)
#$(warning WARNING: make gofmt: $(BADFMT))
endif
endif

