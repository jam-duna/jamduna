OUTPUT := libpvm_recompiler.so

.PHONY: all clean

all: $(OUTPUT)

$(OUTPUT):
	go build -buildmode=c-shared -o $(OUTPUT) .

clean:
	rm -f $(OUTPUT) libpvm_recompiler.h
