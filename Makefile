.PHONY: all server client fmt

all: server client

server:
	cd server && make
client:
	cd client && make

clean:
	cd server && make clean
	cd client && make clean

fmt:
	cd server && make fmt
	cd client && make fmt

