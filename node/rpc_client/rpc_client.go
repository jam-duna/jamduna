package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/rpc"
	"strings"

	"github.com/chzyer/readline"
	"github.com/dop251/goja"
)

// Usage: go run rpc_client.go -rpc=localhost:10900

func main() {
	// Define and parse the rpc flag.
	rpcEndpoint := flag.String("rpc", "localhost:21100", "RPC server")
	flag.Parse()

	client, err := rpc.Dial("tcp", *rpcEndpoint)
	if err != nil {
		fmt.Println("❌ Failed to connect to RPC server:", err)
		return
	}
	defer client.Close()

	// Initialize readline, supporting arrow keys and command history
	rl, err := readline.NewEx(&readline.Config{
		Prompt:      "> ",
		HistoryFile: "/tmp/jam_console_history.txt",
	})
	if err != nil {
		fmt.Println("❌ Failed to start readline:", err)
		return
	}
	defer rl.Close()

	// Initialize Goja JavaScript VM
	vm := goja.New()

	// Register `rpc_call(method, param1, param2, ...)`
	vm.Set("rpc_call", func(method string, args ...string) goja.Value {
		var result string
		err := client.Call(method, args, &result)
		if err != nil {
			return vm.ToValue("❌ RPC Call Failed: " + err.Error())
		}

		// Try to parse JSON
		var jsonData interface{}
		if json.Unmarshal([]byte(result), &jsonData) == nil {
			return vm.ToValue(jsonData) // Return the parsed JSON object
		}
		return vm.ToValue(result)
	})

	vm.Set("print", func(args ...goja.Value) {
		for _, arg := range args {
			fmt.Println(arg.Export())
		}
	})

	// Use JavaScript Proxy to automatically bind `jam.xxx()`
	_, err = vm.RunString(`
		var jam = new Proxy({}, {
			get: function(target, method) {
				return function(...args) {
					return rpc_call("jam." + method, ...args);
				};
			}
		});
	`)
	if err != nil {
		fmt.Println("❌ JavaScript Error:", err)
		return
	}

	// 🟡 Automatically call jam.GetFunctions() at startup
	startVal, err := vm.RunString(`jam.GetFunctions()`)
	if err != nil {
		fmt.Println("❌ Startup JS Error:", err)
	} else {
		fmt.Println("▶️ jam.GetFunctions() =>", startVal)
	}

	// Enter Console interactive mode
	fmt.Println("✅ Jam Console Started (Readline Mode)")
	fmt.Println("Use JavaScript to call RPC methods, e.g.:")
	fmt.Println("Type 'exit' to quit.")

	for {
		line, err := rl.Readline()
		if err != nil {
			fmt.Println("🔴 Exiting Jam Console.")
			break
		}

		line = strings.TrimSpace(line)

		if line == "exit" {
			fmt.Println("🔴 Exiting Jam Console.")
			break
		}

		// Execute JavaScript
		value, err := vm.RunString(line)
		if err != nil {
			fmt.Println("❌ JavaScript Error:", err)
		} else {
			fmt.Println("✅", value)
		}
	}
}
