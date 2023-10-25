lzhuf.dll: lzhuf.o
	gcc -m32 -shared -o lzhuf.dll lzhuf.o

lzhuf.o:
	gcc -m32 -c lzhuf.c -o lzhuf.o

clean:
	@rm -f lzhuf.o
	@rm -f lzhuf.dll