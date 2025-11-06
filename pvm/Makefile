# PVM FFI Library Makefile

# Compiler and flags
CC = gcc
AR = ar
CFLAGS = -Wall -Wextra -O3 -std=c99 -fPIC 
INCLUDES = -Iinclude
SRCDIR = src
OBJDIR = obj
LIBDIR = lib
INCDIR = include

# Library name
LIBNAME = libpvm.a
SONAME = libpvm.so

# Source files
SOURCES = $(wildcard $(SRCDIR)/*.c)
OBJECTS = $(SOURCES:$(SRCDIR)/%.c=$(OBJDIR)/%.o)

# Default target (static library only - no dynamic builds allowed)
all: directories $(LIBDIR)/$(LIBNAME)

# Create necessary directories
directories:
	@mkdir -p $(OBJDIR) $(LIBDIR)

# Static library
$(LIBDIR)/$(LIBNAME): $(OBJECTS)
	$(AR) rcs $@ $(OBJECTS)
	@echo "Static library created: $@"

# Shared library
$(LIBDIR)/$(SONAME): $(OBJECTS)
	$(CC) -shared -o $@ $(OBJECTS)
	@echo "Shared library created: $@"

# Compile object files
$(OBJDIR)/%.o: $(SRCDIR)/%.c
	$(CC) $(CFLAGS) $(INCLUDES) -c $< -o $@

# Debug build
debug: CFLAGS += -g -DDEBUG 
debug: clean all

# Release build with optimization
release: CFLAGS += -O3 -DNDEBUG 
release: clean all

# Clean build artifacts
clean:
	rm -rf $(OBJDIR) $(LIBDIR)
	@echo "Cleaned build artifacts"

# Install (copy headers and library to system directories)
install: all
	@echo "Installing headers to /usr/local/include..."
	cp -r $(INCDIR)/*.h /usr/local/include/
	@echo "Installing library to /usr/local/lib..."
	cp $(LIBDIR)/$(LIBNAME) /usr/local/lib/
	cp $(LIBDIR)/$(SONAME) /usr/local/lib/
	@echo "Installation complete"

# Uninstall
uninstall:
	rm -f /usr/local/include/pvm.h
	rm -f /usr/local/lib/$(LIBNAME)
	rm -f /usr/local/lib/$(SONAME)
	@echo "Uninstallation complete"

# Test compilation (requires test files)
test: all
	@echo "Building tests..."
	@if [ -d tests ]; then \
		$(CC) $(CFLAGS) $(INCLUDES) -Llib -lpvm tests/*.c -o tests/test_runner; \
		echo "Test executable created: tests/test_runner"; \
	else \
		echo "No tests directory found. Skipping test build."; \
	fi

# Print build information
info:
	@echo "PVM FFI Library Build Configuration:"
	@echo "  CC: $(CC)"
	@echo "  CFLAGS: $(CFLAGS)"
	@echo "  INCLUDES: $(INCLUDES)"
	@echo "  Sources: $(SOURCES)"
	@echo "  Objects: $(OBJECTS)"
	@echo "  Target Library: $(LIBDIR)/$(LIBNAME)"

# List all C files that will be compiled
list-sources:
	@echo "Source files to be compiled:"
	@for src in $(SOURCES); do echo "  $$src"; done

# Check for missing dependencies
check-deps:
	@echo "Checking for required tools..."
	@which $(CC) > /dev/null || (echo "ERROR: $(CC) not found" && exit 1)
	@which $(AR) > /dev/null || (echo "ERROR: $(AR) not found" && exit 1)
	@echo "All required tools found"

# Format code (if clang-format is available)
format:
	@if which clang-format > /dev/null 2>&1; then \
		echo "Formatting C source files..."; \
		find $(SRCDIR) $(INCDIR) -name "*.c" -o -name "*.h" | xargs clang-format -i; \
		echo "Code formatting complete"; \
	else \
		echo "clang-format not found, skipping code formatting"; \
	fi

pvm: all-platforms

# Build for all platforms (cross-compilation)
# Note: This target only creates platform-specific .a files (*.linux_amd64.a, *.mac_arm64.a, etc.)
# It does NOT create the base libpvm.a or libcompiler.a to avoid confusion about which arch they are
all-platforms:
	@echo "Building PVM library statically for all platforms..."
	@mkdir -p lib_tmp
	@echo "  Building for Linux amd64..."
	@CC=x86_64-linux-musl-gcc AR=x86_64-linux-musl-ar CFLAGS="-Wall -Wextra -O3 -std=c99 -fPIC" $(MAKE) clean
	@CC=x86_64-linux-musl-gcc AR=x86_64-linux-musl-ar CFLAGS="-Wall -Wextra -O3 -std=c99 -fPIC" $(MAKE) all CC=x86_64-linux-musl-gcc AR=x86_64-linux-musl-ar CFLAGS="-Wall -Wextra -O3 -std=c99 -fPIC"
	@cp $(LIBDIR)/$(LIBNAME) lib_tmp/libpvm.linux_amd64.a
	@echo "  Building for Linux arm64..."
	@if command -v aarch64-linux-musl-gcc >/dev/null 2>&1; then \
		CC=aarch64-linux-musl-gcc AR=aarch64-linux-musl-ar CFLAGS="-Wall -Wextra -O3 -std=c99 -fPIC" $(MAKE) clean; \
		CC=aarch64-linux-musl-gcc AR=aarch64-linux-musl-ar CFLAGS="-Wall -Wextra -O3 -std=c99 -fPIC" $(MAKE) all CC=aarch64-linux-musl-gcc AR=aarch64-linux-musl-ar CFLAGS="-Wall -Wextra -O3 -std=c99 -fPIC"; \
		cp $(LIBDIR)/$(LIBNAME) lib_tmp/libpvm.linux_arm64.a; \
	else \
		echo "    Skipping Linux arm64 (no aarch64-linux-musl-gcc)"; \
	fi
	@echo "  Building for macOS amd64..."
	@CC=clang AR=ar CFLAGS="-Wall -Wextra -O3 -std=c99 -fPIC" $(MAKE) clean
	@CC=clang AR=ar CFLAGS="-Wall -Wextra -O3 -std=c99 -fPIC -target x86_64-apple-macos10.12" $(MAKE) all CC=clang AR=ar CFLAGS="-Wall -Wextra -O3 -std=c99 -fPIC -target x86_64-apple-macos10.12"
	@cp $(LIBDIR)/$(LIBNAME) lib_tmp/libpvm.mac_amd64.a
	@echo "  Building for macOS arm64..."
	@CC=clang AR=ar CFLAGS="-Wall -Wextra -O3 -std=c99 -fPIC" $(MAKE) clean
	@CC=clang AR=ar CFLAGS="-Wall -Wextra -O3 -std=c99 -fPIC -target arm64-apple-macos11" $(MAKE) all CC=clang AR=ar CFLAGS="-Wall -Wextra -O3 -std=c99 -fPIC -target arm64-apple-macos11"
	@cp $(LIBDIR)/$(LIBNAME) lib_tmp/libpvm.mac_arm64.a

	@echo "  Building recompiler C library for Linux amd64..."
	@CC=x86_64-linux-musl-gcc AR=x86_64-linux-musl-ar $(MAKE) -C recompiler_c clean
	@CC=x86_64-linux-musl-gcc AR=x86_64-linux-musl-ar CFLAGS="-Wall -Wextra -std=c99 -fPIC -O3 -DNDEBUG" $(MAKE) -C recompiler_c all CC=x86_64-linux-musl-gcc AR=x86_64-linux-musl-ar CFLAGS="-Wall -Wextra -std=c99 -fPIC -O3 -DNDEBUG"
	@cp recompiler_c/lib/libcompiler.a lib_tmp/libcompiler.linux_amd64.a || true
	@echo "  Building recompiler C library for Linux arm64..."
	@if command -v aarch64-linux-musl-gcc >/dev/null 2>&1; then \
		CC=aarch64-linux-musl-gcc AR=aarch64-linux-musl-ar $(MAKE) -C recompiler_c clean; \
		CC=aarch64-linux-musl-gcc AR=aarch64-linux-musl-ar CFLAGS="-Wall -Wextra -std=c99 -fPIC -O3 -DNDEBUG" $(MAKE) -C recompiler_c all CC=aarch64-linux-musl-gcc AR=aarch64-linux-musl-ar CFLAGS="-Wall -Wextra -std=c99 -fPIC -O3 -DNDEBUG"; \
		cp recompiler_c/lib/libcompiler.a lib_tmp/libcompiler.linux_arm64.a; \
	else \
		echo "    Skipping recompiler Linux arm64 (no aarch64-linux-musl-gcc)"; \
	fi
	@echo "  Building recompiler C library for macOS amd64..."
	@CC=clang AR=ar $(MAKE) -C recompiler_c clean
	@CC=clang AR=ar CFLAGS="-Wall -Wextra -std=c99 -fPIC -O3 -DNDEBUG -target x86_64-apple-macos10.12" $(MAKE) -C recompiler_c all CC=clang AR=ar CFLAGS="-Wall -Wextra -std=c99 -fPIC -O3 -DNDEBUG -target x86_64-apple-macos10.12"
	@cp recompiler_c/lib/libcompiler.a lib_tmp/libcompiler.mac_amd64.a || true
	@echo "  Building recompiler C library for macOS arm64..."
	@CC=clang AR=ar $(MAKE) -C recompiler_c clean
	@CC=clang AR=ar CFLAGS="-Wall -Wextra -std=c99 -fPIC -O3 -DNDEBUG -target arm64-apple-macos11" $(MAKE) -C recompiler_c all CC=clang AR=ar CFLAGS="-Wall -Wextra -std=c99 -fPIC -O3 -DNDEBUG -target arm64-apple-macos11"
	@cp recompiler_c/lib/libcompiler.a lib_tmp/libcompiler.mac_arm64.a || true

	@echo "  Copying platform libraries to final locations..."
	@cp lib_tmp/libpvm.*.a $(LIBDIR)/ || true
	@cp lib_tmp/libcompiler.*.a recompiler_c/lib/ || true
	@echo "  Copying platform libraries to ffi/ directory..."
	@mkdir -p ffi
	@cp lib_tmp/libpvm.*.a ffi/ || true
	@cp lib_tmp/libcompiler.*.a ffi/ || true
	@rm -rf lib_tmp
	@echo "  Cleaning up non-platform-specific files to avoid confusion..."
	@rm -f $(LIBDIR)/libpvm.a recompiler_c/lib/libcompiler.a recompiler_c/lib/libcompiler.so
	@echo "libpvm.*.a and libcompiler.*.a files ready for all platforms in lib/, recompiler_c/lib/, and ffi/."

.PHONY: all directories debug release clean install uninstall test info list-sources check-deps format all-platforms