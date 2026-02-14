export CC=clang

CANINTERFACES=canlib,canusb,combi,ftdi,j2534,pcan,rcan,socketcan

BUILDTAGS=$(CANINTERFACES)


default: txlogger

cangateway:
	go build -tags="j2534" -ldflags '-s -w' -o cangateway ../gocangateway

txlogger:
	go build -tags=$(BUILDTAGS) -ldflags '-s -w' -o txlogger .

release:
	fyne package -tags=$(BUILDTAGS) --release

run: cangateway
	@echo Using compiler "$(CC)"
	go run -tags=$(BUILDTAGS) . 2>&1 | tee run.log

clean:
	rm -f cangateway
	rm -f txlogger