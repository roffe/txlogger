export CC=clang

CANINTERFACES=canlib,canusb,combi,ftdi,j2534,pcan,rcan,socketcan
OTHER=wayland

BUILDTAGS=$(CANINTERFACES),$(OTHER)

ifdef EXTRA_TAGS
BUILDTAGS:=$(CANINTERFACES),$(EXTRA_TAGS)
endif

default: txlogger

cangateway:
	go build -tags="j2534" -ldflags '-s -w' -o cangateway ../gocangateway

txlogger:
	go build -tags=$(BUILDTAGS) -ldflags '-s -w' -o txlogger .

release:
	fyne package -tags=$(BUILDTAGS) --release

debug: clean cangateway
	@echo Using compiler "$(CC)"
	-go run -tags=$(BUILDTAGS),debug . 2>&1 | tee run.log


run: clean cangateway
	@echo Using compiler "$(CC)"
	-GOEXPERIMENT=simd go run -tags=$(BUILDTAGS) . 2>&1 | tee run.log

clean:
	rm -f cangateway
	rm -f txlogger