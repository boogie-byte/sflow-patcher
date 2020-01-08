OUTDIR := $(CURDIR)/out
BINPATH := $(OUTDIR)/sflow-patcher

.PHONY: all clean

$(BINPATH): *.go
	go build -o $(BINPATH) $(GO_BUILDFLAGS)

all: $(BINPATH)

clean:
	rm -rf $(OUTDIR)