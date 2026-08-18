.PHONY: j2534proxy snapshots clean appimage

export CC=clang

CANINTERFACES=canlib,canusb,combi,ftdi,j2534,pcan,rcan,socketcan
OTHER=wayland

BUILDTAGS=$(CANINTERFACES),$(OTHER)

ifdef EXTRA_TAGS
BUILDTAGS:=$(CANINTERFACES),$(EXTRA_TAGS)
endif

default: txlogger

clean:
	rm -f cangateway j2534proxy.exe
	rm -f txlogger
	rm -f txlogger-*-x86_64.AppImage
	rm -f txlogger-dx.exe
	rm -f txlogger.exe

pkg/ota/firmware.bin: /home/roffe/Documents/PlatformIO/Projects/txbridge/.pio/build/esp32dev/firmware.bin
	@cp $< $@

txlogger:
	go build -tags=$(BUILDTAGS) -ldflags '-s -w' -o txlogger .

release:
	fyne package -tags=$(BUILDTAGS) --release

debug: clean
	@echo Using compiler "$(CC)"
	-go run -tags=$(BUILDTAGS),debug . 2>&1 | tee run.log

windows:
	CGO_CFLAGS="-Ivcpkg/packages/libusb_x64-windows/include/libusb-1.0" \
	CGO_LDFLAGS="-Lvcpkg/packages/libusb_x64-windows/lib" \
	CGO_ENABLED=1 \
	CC=x86_64-w64-mingw32-gcc \
	GOARCH=amd64 \
	GOOS=windows \
	fyne package --os windows --icon Icon.png -tags=$(BUILDTAGS) --release \
	GOARCH=386 \
	go build -tags=j2534 -ldflags '-s -w' -o j2534proxy.exe ./j2534proxy

windows-dx:
	CGO_CFLAGS="-Ivcpkg/packages/libusb_x64-windows/include/libusb-1.0" \
	CGO_LDFLAGS="-Lvcpkg/packages/libusb_x64-windows/lib" \
	CGO_ENABLED=1 \
	CC=x86_64-w64-mingw32-gcc \
	GOARCH=amd64 \
	GOOS=windows \
	fyne package --os windows --icon Icon.png --name txlogger-dx.exe -tags=$(BUILDTAGS),directx --release \
	GOARCH=386 \
	go build -tags=j2534,dx -ldflags '-s -w' -o j2534proxy.exe ./j2534proxy

run: clean pkg/ota/firmware.bin
	@echo Using compiler "$(CC)"
	-GOEXPERIMENT=simd go run -tags=$(BUILDTAGS) . 2>&1 | tee run.log

snapshots:
	go run -tags combi,canusb,ftdi,j2534,pcan,rcan,socketcan ./cmd/screenshots \
    	-out ../txlogger-webpage/static/screenshots -log cmd/screenshots/t7log.csv \
    	-bin ".tmp/bosse 25mhz.bin"


j2534proxy:
	GOOS=windows GOARCH=386 go build -tags="j2534" -ldflags '-s -w' -o j2534proxy.exe ./j2534proxy

APPIMAGETOOL=.tmp/appimagetool
VERSION=$(shell sed -n 's/^Version = "\(.*\)"/\1/p' FyneApp.toml)

$(APPIMAGETOOL):
	@mkdir -p .tmp
	curl -fsSL -o $@ https://github.com/AppImage/appimagetool/releases/download/continuous/appimagetool-x86_64.AppImage
	chmod +x $@

appimage: txlogger $(APPIMAGETOOL)
	rm -rf .tmp/AppDir
	mkdir -p .tmp/AppDir/usr/bin
	mkdir -p .tmp/AppDir/usr/share/icons/
	cp txlogger .tmp/AppDir/usr/bin/
	cp Icon.png .tmp/AppDir/txlogger.png
	cp Icon.png .tmp/AppDir/usr/share/icons/txlogger.png
	cp Icon.png .tmp/AppDir/.DirIcon
	ln -sf usr/bin/txlogger .tmp/AppDir/AppRun
	printf '[Desktop Entry]\nType=Application\nName=txlogger\nExec=txlogger\nIcon=txlogger\nCategories=Utility;\n' > .tmp/AppDir/txlogger.desktop
	ARCH=x86_64 $(APPIMAGETOOL) --appimage-extract-and-run .tmp/AppDir txlogger-x86_64.AppImage
