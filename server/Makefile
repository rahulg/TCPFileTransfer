PRODUCT=../testbed/server

.PHONY: all clean fmt

all:
	go build
	cp server $(PRODUCT)

clean:
	- rm server $(PRODUCT)

fmt:
	go fmt
