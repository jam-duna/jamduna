# PVM Recompiler C Implementation Makefile (Simplified - Compiler Only)

# Compiler settings
CC = gcc
AR ?= ar
# Allow CFLAGS to be overridden from command line, otherwise use default
CFLAGS ?= -Wall -Wextra -std=c99 -fPIC
CFLAGS_DEBUG = $(CFLAGS) -g -O0 -DDEBUG
CFLAGS_ASAN = $(CFLAGS) -g -O0 -DDEBUG -fsanitize=address -fno-omit-frame-pointer
CFLAGS_RELEASE = $(CFLAGS) -O3 -DNDEBUG -march=native -g

# No external library dependencies (removed unicorn, capstone)
LDFLAGS =
LDFLAGS_ASAN = -fsanitize=address

# Directories
SRC_DIR = src
INCLUDE_DIR = include
LIB_DIR = lib
FFI_DIR = ../../ffi

# Output
LIBRARY_NAME = libcompiler
SHARED_LIB = $(LIB_DIR)/$(LIBRARY_NAME).so
STATIC_LIB = $(LIB_DIR)/$(LIBRARY_NAME).a

# Source files (simplified - compiler only)
SOURCES = $(SRC_DIR)/pvm_const.c \
          $(SRC_DIR)/basic_block.c \
          $(SRC_DIR)/compiler.c \
          $(SRC_DIR)/x86_codegen.c \
          $(SRC_DIR)/x86_translators.c

OBJECTS = $(SOURCES:$(SRC_DIR)/%.c=$(LIB_DIR)/%.o)
OBJECTS_DEBUG = $(SOURCES:$(SRC_DIR)/%.c=$(LIB_DIR)/%-debug.o)
OBJECTS_ASAN = $(SOURCES:$(SRC_DIR)/%.c=$(LIB_DIR)/%-asan.o)

# Header dependencies
HEADERS = $(wildcard $(INCLUDE_DIR)/*.h)

# Default target
.PHONY: all clean debug release asan help install

all: release

# Install to ffi directory for cgo linking
install: release
	cp $(STATIC_LIB) $(FFI_DIR)/$(LIBRARY_NAME).linux_amd64.a
	@echo "Installed to $(FFI_DIR)/$(LIBRARY_NAME).linux_amd64.a"

# Release build
release: CFLAGS := $(CFLAGS_RELEASE)
release: $(SHARED_LIB) $(STATIC_LIB)

# Debug build
debug: CFLAGS := $(CFLAGS_DEBUG)
debug: $(LIB_DIR)/$(LIBRARY_NAME)-debug.so $(LIB_DIR)/$(LIBRARY_NAME)-debug.a

# ASan build
asan: CFLAGS := $(CFLAGS_ASAN)
asan: LDFLAGS := $(LDFLAGS_ASAN)
asan: $(LIB_DIR)/$(LIBRARY_NAME)-asan.so $(LIB_DIR)/$(LIBRARY_NAME)-asan.a

# Shared library (release)
$(SHARED_LIB): $(OBJECTS) | $(LIB_DIR)
	$(CC) -shared -o $@ $^ $(LDFLAGS)
	@echo "Built release shared library: $@"

# Static library (release)
$(STATIC_LIB): $(OBJECTS) | $(LIB_DIR)
	$(AR) rcs $@ $^
	@echo "Built release static library: $@"

# Shared library (debug)
$(LIB_DIR)/$(LIBRARY_NAME)-debug.so: $(OBJECTS_DEBUG) | $(LIB_DIR)
	$(CC) -shared -o $@ $^ $(LDFLAGS)
	@echo "Built debug shared library: $@"

# Static library (debug)
$(LIB_DIR)/$(LIBRARY_NAME)-debug.a: $(OBJECTS_DEBUG) | $(LIB_DIR)
	$(AR) rcs $@ $^
	@echo "Built debug static library: $@"

# Shared library (asan)
$(LIB_DIR)/$(LIBRARY_NAME)-asan.so: $(OBJECTS_ASAN) | $(LIB_DIR)
	$(CC) -shared -o $@ $^ $(LDFLAGS_ASAN)
	@echo "Built asan shared library: $@"

# Static library (asan)
$(LIB_DIR)/$(LIBRARY_NAME)-asan.a: $(OBJECTS_ASAN) | $(LIB_DIR)
	$(AR) rcs $@ $^
	@echo "Built asan static library: $@"

# Object files (release)
$(LIB_DIR)/%.o: $(SRC_DIR)/%.c $(HEADERS) | $(LIB_DIR)
	$(CC) $(CFLAGS) -I$(INCLUDE_DIR) -c $< -o $@

# Object files (debug)
$(LIB_DIR)/%-debug.o: $(SRC_DIR)/%.c $(HEADERS) | $(LIB_DIR)
	$(CC) $(CFLAGS_DEBUG) -I$(INCLUDE_DIR) -c $< -o $@

# Object files (asan)
$(LIB_DIR)/%-asan.o: $(SRC_DIR)/%.c $(HEADERS) | $(LIB_DIR)
	$(CC) $(CFLAGS_ASAN) -I$(INCLUDE_DIR) -c $< -o $@

# Create directories
$(LIB_DIR):
	mkdir -p $(LIB_DIR)

# Clean build artifacts
clean:
	rm -rf $(LIB_DIR)/*.o $(LIB_DIR)/*.so $(LIB_DIR)/*.a
	@echo "Cleaned build artifacts"

# Development helpers
format:
	@echo "Formatting C code..."
	find $(SRC_DIR) $(INCLUDE_DIR) -name "*.c" -o -name "*.h" | xargs clang-format -i

# Help target
help:
	@echo "PVM JIT Compiler Build System (Simplified)"
	@echo "=========================================="
	@echo ""
	@echo "Build Targets:"
	@echo "  all              - Build release version (default)"
	@echo "  release          - Build optimized release version"
	@echo "  debug            - Build debug version"
	@echo "  install          - Build release and copy to ffi directory"
	@echo "  clean            - Remove all build artifacts"
	@echo ""
	@echo "Development:"
	@echo "  format           - Format code with clang-format"
	@echo "  help             - Show this help message"
	@echo ""
	@echo "Usage Examples:"
	@echo "  make                    # Build release"
	@echo "  make install            # Build and install to ffi/"
	@echo "  make debug              # Build debug version"
	@echo "  make clean && make      # Clean rebuild"
	@echo ""
	@echo "Note: This is a simplified compiler-only build."
	@echo "      VM management and execution are handled by Go."

# Phony targets
.PHONY: all clean debug release help format

# Keep intermediate files
.PRECIOUS: $(LIB_DIR)/%.o $(LIB_DIR)/%-debug.o
