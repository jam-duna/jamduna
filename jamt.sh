#!/bin/bash

code_file=./services/fib.pvm
default_balance=1000000
jamt_bin_file=bin/jamt
# get the hex of bytecode
default_code=$(xxd -p -c 999999 "$code_file" | tr -d '\n')

"$jamt_bin_file" --rpc ws://localhost:19803 create-service "$default_code" "$default_balance"
