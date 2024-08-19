

assemble:
	cargo run -p polkatool -- assemble  hostfunctions/host_read_write.txt  -o hostfunctions/read_write.polkavm
	cargo run -p polkatool -- assemble  hostfunctions/host_import_export.txt  -o hostfunctions/import_export.polkavm
	cargo run -p polkatool -- assemble  hostfunctions/host_solicit.txt  -o hostfunctions/solicit.polkavm
	cargo run -p polkatool -- assemble  hostfunctions/host_forget.txt  -o hostfunctions/forget.polkavm
	cargo run -p polkatool -- assemble  hostfunctions/host_historical_lookup.txt  -o hostfunctions/historical_lookup.polkavm
	cargo run -p polkatool -- assemble  hostfunctions/host_lookup.txt  -o hostfunctions/lookup.polkavm
	cargo run -p polkatool -- assemble  hostfunctions/host_assign.txt  -o hostfunctions/assign.polkavm
	cargo run -p polkatool -- assemble  hostfunctions/host_checkpoint.txt  -o hostfunctions/checkpoint.polkavm
	cargo run -p polkatool -- assemble  hostfunctions/host_designate.txt  -o hostfunctions/designate.polkavm
	cargo run -p polkatool -- assemble  hostfunctions/host_egas.txt  -o hostfunctions/egas.polkavm
	cargo run -p polkatool -- assemble  hostfunctions/host_empower.txt  -o hostfunctions/empower.polkavm
	cargo run -p polkatool -- assemble  hostfunctions/host_expunge.txt  -o hostfunctions/expunge.polkavm
	cargo run -p polkatool -- assemble  hostfunctions/host_info.txt  -o hostfunctions/info.polkavm
	cargo run -p polkatool -- assemble  hostfunctions/host_invoke.txt  -o hostfunctions/invoke.polkavm
	cargo run -p polkatool -- assemble  hostfunctions/host_machine.txt  -o hostfunctions/machine.polkavm
	cargo run -p polkatool -- assemble  hostfunctions/host_new.txt  -o hostfunctions/new.polkavm
	cargo run -p polkatool -- assemble  hostfunctions/host_peek.txt  -o hostfunctions/peek.polkavm
	cargo run -p polkatool -- assemble  hostfunctions/host_poke.txt  -o hostfunctions/poke.polkavm
	cargo run -p polkatool -- assemble  hostfunctions/host_quit.txt  -o hostfunctions/quit.polkavm
	cargo run -p polkatool -- assemble  hostfunctions/host_transfer.txt  -o hostfunctions/transfer.polkavm
	cargo run -p polkatool -- assemble  hostfunctions/host_upgrade.txt  -o hostfunctions/upgrade.polkavm

disassemble:
	cargo run -p polkatool disassemble --show-raw-bytes hostfunctions/read_write.polkavm
	cargo run -p polkatool disassemble --show-raw-bytes hostfunctions/import_export.polkavm
	cargo run -p polkatool disassemble --show-raw-bytes hostfunctions/solicit.polkavm
	cargo run -p polkatool disassemble --show-raw-bytes hostfunctions/forget.polkavm
	cargo run -p polkatool disassemble --show-raw-bytes hostfunctions/historical_lookup.polkavm
	cargo run -p polkatool disassemble --show-raw-bytes hostfunctions/lookup.polkavm
	cargo run -p polkatool disassemble --show-raw-bytes hostfunctions/assign.polkavm
	cargo run -p polkatool disassemble --show-raw-bytes hostfunctions/checkpoint.polkavm
	cargo run -p polkatool disassemble --show-raw-bytes hostfunctions/designate.polkavm
	cargo run -p polkatool disassemble --show-raw-bytes hostfunctions/egas.polkavm
	cargo run -p polkatool disassemble --show-raw-bytes hostfunctions/empower.polkavm
	cargo run -p polkatool disassemble --show-raw-bytes hostfunctions/expunge.polkavm
	cargo run -p polkatool disassemble --show-raw-bytes hostfunctions/info.polkavm
	cargo run -p polkatool disassemble --show-raw-bytes hostfunctions/invoke.polkavm
	cargo run -p polkatool disassemble --show-raw-bytes hostfunctions/machine.polkavm
	cargo run -p polkatool disassemble --show-raw-bytes hostfunctions/new.polkavm
	cargo run -p polkatool disassemble --show-raw-bytes hostfunctions/peek.polkavm
	cargo run -p polkatool disassemble --show-raw-bytes hostfunctions/poke.polkavm
	cargo run -p polkatool disassemble --show-raw-bytes hostfunctions/quit.polkavm
	cargo run -p polkatool disassemble --show-raw-bytes hostfunctions/transfer.polkavm
	cargo run -p polkatool disassemble --show-raw-bytes hostfunctions/upgrade.polkavm
