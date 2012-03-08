T6G=6g
T6L=6l
FMT=gofmt

GOGFLAGS=
GOLFLAGS=
FMTFLAGS=-w

SRC=$(wildcard *.go)
PRODUCT=../testbed/ftclient

.PHONY: all clean fmt

all:
	$(T6G) $(GOGFLAGS) -o main.6 *.go
	$(T6L) $(GOLFLAGS) -o $(PRODUCT) main.6

clean:
	- rm main.6 $(PRODUCT)

fmt: $(SRC)
	$(FMT) $(FMTFLAGS) *.go
