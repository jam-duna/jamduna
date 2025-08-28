# Install dependencies

CoreVM monitor uses SDL2 library that needs to be installed in a platform-specific way.

```bash
# Ubuntu
apt-get install libsdl2 libsdl2-ttf

# MacOS
brew install SDL2 SDL2_ttf
```

On MacOS you might also need to add SDL2 to the library search path
(e.g. if Homebrew is installed in a directory other than `/opt/homebrew`).

```bash
export DYLD_LIBRARY_PATH=<homebrew-dir>/lib
```


# Run Doom

Running `doom.corevm` should be straighforward while `doom-with-audio.corevm` would most likely need Linux (specifically `env POLKAVM_BACKEND=compiler ./polkajam-testnet`) to run at full speed. 

```bash
# Run local testnet.
./polkajam-testnet

# Create CoreVM.
./jamt vm new ./doom.corevm 1000000000

# Run CoreVM builder (SERVICE_ID is in jamt's output).
./corevm-builder --temp --chain dev --gas 1000000000 SERVICE_ID

# Run CoreVM monitor (SERVICE_ID is in jamt's output).
./corevm-monitor SERVICE_ID
```

You can use `export RUST_LOG=corevm` for debugging.
