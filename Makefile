export GOEXPERIMENT=greenteagc
export CC=clang

default: txlogger

cangateway:
	go build -tags="j2534" -ldflags '-s -w' -o cangateway ../gocangateway

txlogger: cangateway
	go build -tags="canlib,canusb,combi,j2534,pcan,rcan,socketcan" -ldflags '-s -w' -o txlogger .

release:
	fyne package -tags="canlib,canusb,combi,ftdi,j2534,pcan,rcan,socketcan" --release

run: cangateway
	echo $(CC)
	go run -tags="canlib,canusb,combi,ftdi,j2534,pcan,rcan" .

clean:
	rm -f cangateway
	rm -f txlogger