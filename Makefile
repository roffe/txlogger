export CC=clang

CANINTERFACES="canlib,canusb,combi,ftdi,j2534,pcan,rcan,socketcan"

default: txlogger

cangateway:
	go build -tags="j2534" -ldflags '-s -w' -o cangateway ../gocangateway

txlogger:
	go build -tags=$(CANINTERFACES) -ldflags '-s -w' -o txlogger .

release:
	fyne package -tags=$(CANINTERFACES) --release

run: cangateway
	@echo Using compiler "$(CC)"
	go run -tags=$(CANINTERFACES) .

clean:
	rm -f cangateway
	rm -f txlogger