package fuzz

import (
	"bytes"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"time"
)

const (
	FUZZ_VERSION = "0.7.0.7"
)

type FlagRegistry struct {
	program     string
	flags       []registeredFlag
	registered  map[string]bool
	validLongs  map[string]struct{}
	validShorts map[string]struct{}
}

type registeredFlag struct {
	short       string
	long        string
	target      interface{}
	defaultVal  interface{}
	description string
}

func NewFlagRegistry(programName string) *FlagRegistry {
	return &FlagRegistry{
		program:     programName,
		registered:  make(map[string]bool),
		validLongs:  make(map[string]struct{}),
		validShorts: make(map[string]struct{}),
	}
}

func (r *FlagRegistry) ProcessRegistry() {
	r.Register()
	flag.Usage = r.Usage
	flag.Parse()
	r.Verify() // check for invalid flags
}

func (r *FlagRegistry) RegisterLongFlag(long interface{}, def interface{}, desc string, target interface{}) {
	r.RegisterFlag(long, nil, def, desc, target)
}

func (r *FlagRegistry) RegisterShortFlag(short interface{}, def interface{}, desc string, target interface{}) {
	r.RegisterFlag(nil, short, def, desc, target)
}

func (r *FlagRegistry) RegisterFlag(long, short, def interface{}, desc string, target interface{}) {
	if target == nil {
		log.Fatal("BadFlags")
	}

	var longStr, shortStr string
	if ls, ok := long.(string); ok && ls != "" {
		longStr = ls
	}
	if ss, ok := short.(string); ok && ss != "" {
		shortStr = ss
	}
	if longStr == "" && shortStr == "" {
		log.Fatal("BadFlags")
	}
	if len(shortStr) > 1 {
		log.Fatal("BadFlags")
	}
	r.checkDuplicate(longStr, shortStr)
	r.flags = append(r.flags, registeredFlag{
		short:       shortStr,
		long:        longStr,
		target:      target,
		defaultVal:  def,
		description: desc,
	})
	if longStr != "" {
		r.validLongs[longStr] = struct{}{}
		r.registered[longStr] = true
	}
	if shortStr != "" {
		r.validShorts[shortStr] = struct{}{}
		r.registered[shortStr] = true
	}
}

func registerFlagVar[T any](short, long string, target *T, defVal interface{}, desc string, regFn func(*T, string, T, string)) {
	d, ok := defVal.(T)
	if !ok {
		log.Fatalf("Type mismatch for flag --%s: expected %T, got %T", long, *target, defVal)
	}
	if short != "" {
		regFn(target, short, d, desc)
	}
	if long != "" {
		regFn(target, long, d, desc)
	}
}

func (r *FlagRegistry) Register() {
	for _, f := range r.flags {
		switch ptr := f.target.(type) {
		case *string:
			registerFlagVar(f.short, f.long, ptr, f.defaultVal, f.description, flag.StringVar)
		case *bool:
			registerFlagVar(f.short, f.long, ptr, f.defaultVal, f.description, flag.BoolVar)
		case *int:
			registerFlagVar(f.short, f.long, ptr, f.defaultVal, f.description, flag.IntVar)
		case *float64:
			registerFlagVar(f.short, f.long, ptr, f.defaultVal, f.description, flag.Float64Var)
		case *int64:
			registerFlagVar(f.short, f.long, ptr, f.defaultVal, f.description, flag.Int64Var)
		case *uint:
			registerFlagVar(f.short, f.long, ptr, f.defaultVal, f.description, flag.UintVar)
		case *uint64:
			registerFlagVar(f.short, f.long, ptr, f.defaultVal, f.description, flag.Uint64Var)
		case *time.Duration:
			registerFlagVar(f.short, f.long, ptr, f.defaultVal, f.description, flag.DurationVar)
		default:
			log.Fatalf("Unsupported flag type for --%s (%T)", f.long, f.target)
		}
	}
}

func (r *FlagRegistry) Verify() {
	r.validateFlagFormat()
	r.validateNoConflict()
}

func (r *FlagRegistry) Usage() {
	fmt.Fprint(flag.CommandLine.Output(), r.usage())
	os.Exit(0)
}

func (r *FlagRegistry) usage() string {
	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("Usage: %s [OPTIONS]\n\n", r.program))
	buf.WriteString("Options:\n")

	var entries []struct {
		names string
		help  string
	}
	maxWidth := 0

	for _, f := range r.flags {
		var nameParts []string
		if f.short != "" {
			nameParts = append(nameParts, fmt.Sprintf("-%s", f.short))
		}
		if f.long != "" {
			nameParts = append(nameParts, fmt.Sprintf("--%s", f.long))
		}
		names := strings.Join(nameParts, ", ")
		if len(names) > maxWidth {
			maxWidth = len(names)
		}
		var defaultVal interface{}
		if str, ok := f.defaultVal.(string); ok && str == "" {
			defaultVal = nil
		} else {
			defaultVal = f.defaultVal
		}
		helpText := fmt.Sprintf("%s (default:%v)", f.description, defaultVal)
		entries = append(entries, struct {
			names string
			help  string
		}{names, helpText})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].names < entries[j].names
	})

	for _, e := range entries {
		buf.WriteString(fmt.Sprintf("  %-*s  %s\n", maxWidth, e.names, e.help))
	}
	return buf.String()
}

func (r *FlagRegistry) checkDuplicate(long, short string) {
	if short != "" && r.registered[short] {
		log.Fatalf("DuplicateFlag: -%s", short)
	}
	if long != "" && r.registered[long] {
		log.Fatalf("DuplicateFlag: --%s", long)
	}
}

func (r *FlagRegistry) validateFlagFormat() {
	args := os.Args[1:]
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			continue
		}
		if strings.HasPrefix(arg, "--") {
			name := strings.TrimPrefix(arg, "--")
			if eq := strings.Index(name, "="); eq > 0 {
				name = name[:eq]
			}
			if _, ok := r.validLongs[name]; !ok {
				if len(name) == 1 {
					for _, f := range r.flags {
						if f.short == name {
							log.Fatalf("UnknownFlag: --%s (Expected '--%s')", name, f.long)
						}
					}
				}
				log.Fatalf("UnknownFlag: --%s", name)
			}
			continue
		}
		trimmed := strings.TrimPrefix(arg, "-")
		if eq := strings.Index(trimmed, "="); eq > 0 {
			trimmed = trimmed[:eq]
		}
		if len(trimmed) == 1 {
			if _, ok := r.validShorts[trimmed]; !ok {
				log.Fatalf("UnknownFlag: -%s", trimmed)
			}
			continue
		}
		if _, inLong := r.validLongs[trimmed]; inLong {
			log.Fatalf("UnknownFlag: -%s (Expected '--%s')", trimmed, trimmed)
		}
		first := string(trimmed[0])
		if _, inShort := r.validShorts[first]; inShort {
			for _, f := range r.flags {
				if f.short == first {
					log.Fatalf("UnknownFlag: -%s (Expected '--%s')", trimmed, f.long)
				}
			}
		}
		log.Fatalf("UnknownFlag: -%s", trimmed)
	}
}

func (r *FlagRegistry) validateNoConflict() {
	visitCount := make(map[string]int)
	flag.Visit(func(f *flag.Flag) {
		visitCount[f.Name]++
	})
	for name, count := range visitCount {
		if count > 1 {
			log.Fatalf("DuplicateFlag found: --%s", name)
		}
	}
	for _, f := range r.flags {
		if f.short != "" && visitCount[f.short] > 0 && visitCount[f.long] > 0 {
			log.Fatalf("DuplicateFlags: -%s & --%s", f.short, f.long)
		}
	}
}
