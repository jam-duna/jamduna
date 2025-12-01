cmake_minimum_required(VERSION 3.18)
project(rsa_cuda LANGUAGES CXX CUDA)

# Require CUDA
find_package(CUDA REQUIRED)

# Set CUDA standard
set(CMAKE_CUDA_STANDARD 14)
set(CMAKE_CUDA_STANDARD_REQUIRED ON)

# CGBN requires compute capability 7.0 or higher
set(CMAKE_CUDA_ARCHITECTURES 70 75 80 86)

# Add CGBN include directory (assumes CGBN is installed or available)
# You may need to adjust this path based on your CGBN installation
set(CGBN_INCLUDE_DIR "${CMAKE_CURRENT_SOURCE_DIR}/../cgbn/include" CACHE PATH "Path to CGBN include directory")

if(NOT EXISTS ${CGBN_INCLUDE_DIR}/cgbn/cgbn.h)
    message(WARNING "CGBN not found at ${CGBN_INCLUDE_DIR}. You may need to download CGBN from NVIDIA.")
endif()

include_directories(${CGBN_INCLUDE_DIR})

# Build shared library
add_library(rsa_cuda SHARED rsa_cuda_cgbn.cu)

# Set CUDA flags
target_compile_options(rsa_cuda PRIVATE
    $<$<COMPILE_LANGUAGE:CUDA>:
        --expt-relaxed-constexpr
        -lineinfo
    >
)

# Link CUDA runtime
target_link_libraries(rsa_cuda ${CUDA_LIBRARIES})

# Installation rules
install(TARGETS rsa_cuda
    LIBRARY DESTINATION lib
    ARCHIVE DESTINATION lib
    RUNTIME DESTINATION bin
)
