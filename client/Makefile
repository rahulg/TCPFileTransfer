PRODUCT=../testbed/client

.PHONY: all clean fmt

all:
	go build
	cp client $(PRODUCT)

clean:
	- rm client $(PRODUCT)

fmt:
	go fmt
