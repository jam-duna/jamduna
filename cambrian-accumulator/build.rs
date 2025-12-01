// Build script to compile CUDA library
use std::env;
use std::path::PathBuf;
use std::process::Command;

fn main() {
    // Only build CUDA library if the feature is enabled
    #[cfg(feature = "gpu")]
    {
        println!("cargo:rerun-if-changed=librsa_cuda/");

        let out_dir = env::var("OUT_DIR").unwrap();
        let manifest_dir = env::var("CARGO_MANIFEST_DIR").unwrap();

        // Create build directory
        let build_dir = PathBuf::from(&out_dir).join("cuda_build");
        std::fs::create_dir_all(&build_dir).expect("Failed to create build directory");

        // Run CMake configure
        let cmake_status = Command::new("cmake")
            .current_dir(&build_dir)
            .arg(format!("{}/librsa_cuda", manifest_dir))
            .arg(format!("-DCMAKE_BUILD_TYPE=Release"))
            .status()
            .expect("Failed to run cmake configure");

        if !cmake_status.success() {
            panic!("CMake configure failed");
        }

        // Run CMake build
        let build_status = Command::new("cmake")
            .current_dir(&build_dir)
            .arg("--build")
            .arg(".")
            .arg("--config")
            .arg("Release")
            .status()
            .expect("Failed to run cmake build");

        if !build_status.success() {
            panic!("CMake build failed");
        }

        // Tell cargo where to find the library
        println!("cargo:rustc-link-search=native={}", build_dir.display());
        println!("cargo:rustc-link-lib=dylib=rsa_cuda");

        // Also link CUDA runtime
        println!("cargo:rustc-link-lib=dylib=cudart");
    }

    // If GPU feature is not enabled, we skip building the CUDA library
    #[cfg(not(feature = "gpu"))]
    {
        // No-op when GPU feature is disabled
    }
}
