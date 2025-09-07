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

# Default target
all: directories $(LIBDIR)/$(LIBNAME) $(LIBDIR)/$(SONAME)

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

.PHONY: all directories debug release clean install uninstall test info list-sources check-deps format