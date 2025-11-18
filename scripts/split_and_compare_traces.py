#!/usr/bin/env python3
"""
Split and Compare PVM Traces: jamDuna vs JavaJAM

This script:
1. Extracts all services from jamDuna trace (sequential execution)
2. Extracts matching services from JavaJAM trace (interleaved execution)
3. Splits both traces into separate per-service files
4. Compares each service pair and reports differences
"""

import re
import sys
from collections import defaultdict

def parse_trace_line(line):
    """Parse a PVM trace line into structured data"""
    match = re.search(r'(\w+)\s+(\d+)\s+(\d+)\s+Gas:\s+(\d+)\s+Registers:\[([\d,\s]+)\]', line)
    if match:
        registers = [int(x.strip()) for x in match.group(5).split(',')]
        return {
            'instruction': match.group(1),
            'step': int(match.group(2)),
            'pc': int(match.group(3)),
            'gas': int(match.group(4)),
            'registers': registers,
            'line': line.strip()
        }
    return None

def extract_jamduna_services(filename):
    """Extract all services from jamDuna trace (sequential)"""
    print(f"Reading jamDuna trace from {filename}...")

    services = []
    current_service = []

    with open(filename, 'r') as f:
        for line in f:
            parsed = parse_trace_line(line)
            if parsed:
                # Service boundary: JUMP 1 5
                if parsed['instruction'] == 'JUMP' and parsed['step'] == 1 and parsed['pc'] == 5:
                    if current_service:
                        services.append(current_service)
                        current_service = []
                current_service.append(parsed)
            else:
                # Include non-parsed lines (host function calls, etc)
                if current_service and line.strip():
                    current_service.append({'line': line.strip()})

        if current_service:
            services.append(current_service)

    print(f"  Found {len(services)} services")
    for i, svc in enumerate(services, 1):
        pvm_steps = sum(1 for e in svc if 'pc' in e)
        print(f"    Service {i}: {len(svc)} total lines, {pvm_steps} PVM steps")

    return services

def extract_javajam_services(filename, jamduna_services):
    """Extract services from JavaJAM trace by matching jamDuna PC sequences"""
    print(f"\nReading JavaJAM trace from {filename}...")

    # Load all JavaJAM entries
    javajam_all = []
    with open(filename, 'r') as f:
        for line in f:
            parsed = parse_trace_line(line)
            if parsed:
                javajam_all.append(parsed)

    print(f"  Loaded {len(javajam_all)} JavaJAM entries")

    # For each jamDuna service, find matching entries in JavaJAM
    javajam_services = []

    for svc_num, jam_svc in enumerate(jamduna_services, 1):
        print(f"  Matching jamDuna Service {svc_num}...")

        # Extract PC sequence from jamDuna
        jam_pcs = [e['pc'] for e in jam_svc if 'pc' in e]

        # Find these PCs in JavaJAM (in order, allowing gaps for interleaving)
        javajam_indices = []
        jav_idx = 0

        for jam_pc in jam_pcs:
            while jav_idx < len(javajam_all):
                if javajam_all[jav_idx]['pc'] == jam_pc:
                    javajam_indices.append(jav_idx)
                    jav_idx += 1
                    break
                jav_idx += 1

        # Extract the matched entries
        matched_service = [javajam_all[idx] for idx in javajam_indices]
        javajam_services.append(matched_service)

        print(f"    Matched {len(matched_service)}/{len(jam_pcs)} PCs")

    return javajam_services

def write_service_files(services, prefix, output_dir="/tmp"):
    """Write each service to a separate file"""
    print(f"\nWriting {prefix} service files to {output_dir}/...")

    filenames = []
    for svc_num, svc in enumerate(services, 1):
        filename = f"{output_dir}/{prefix}_service_{svc_num}.log"

        with open(filename, 'w') as f:
            for entry in svc:
                f.write(entry['line'] + '\n')

        filenames.append(filename)
        print(f"  Service {svc_num}: {len(svc)} lines -> {filename}")

    return filenames

def compare_services(jam_services, jav_services):
    """Compare each service pair and report differences"""
    print("\n" + "="*120)
    print("SERVICE COMPARISON SUMMARY")
    print("="*120)

    for svc_num, (jam_svc, jav_svc) in enumerate(zip(jam_services, jav_services), 1):
        print(f"\nService {svc_num}:")

        # Count PVM instruction entries
        jam_pvm = [e for e in jam_svc if 'pc' in e]
        jav_pvm = [e for e in jav_svc if 'pc' in e]

        print(f"  jamDuna: {len(jam_pvm)} PVM instructions")
        print(f"  JavaJAM: {len(jav_pvm)} PVM instructions")

        # Compare PC sequences
        pc_matches = 0
        inst_matches = 0
        gas_diffs = []

        for i in range(min(len(jam_pvm), len(jav_pvm))):
            jam = jam_pvm[i]
            jav = jav_pvm[i]

            if jam['pc'] == jav['pc']:
                pc_matches += 1

            if jam['instruction'] == jav['instruction']:
                inst_matches += 1

            gas_diff = jam['gas'] - jav['gas']
            if gas_diff != 0:
                gas_diffs.append((i, gas_diff, jam['step']))

        print(f"  PC matches: {pc_matches}/{min(len(jam_pvm), len(jav_pvm))} ({100*pc_matches/min(len(jam_pvm), len(jav_pvm)):.1f}%)")
        print(f"  Instruction matches: {inst_matches}/{min(len(jam_pvm), len(jav_pvm))} ({100*inst_matches/min(len(jam_pvm), len(jav_pvm)):.1f}%)")

        if gas_diffs:
            print(f"  Gas differences: {len(gas_diffs)} positions")
            if len(gas_diffs) <= 5:
                for pos, diff, step in gas_diffs:
                    print(f"    Position {pos} (step {step}): {diff:+d} gas")
            else:
                print(f"    First 3: {gas_diffs[:3]}")
                print(f"    Last 3: {gas_diffs[-3:]}")
        else:
            print(f"  Gas: All match perfectly")

def main():
    import argparse

    parser = argparse.ArgumentParser(description='Split and compare PVM traces')
    parser.add_argument('--jamduna', default='/Users/michael/Downloads/1763371379_jamduna_trace.log',
                      help='Path to jamDuna trace file')
    parser.add_argument('--javajam', default='/Users/michael/Downloads/1763371379_javajam_trace.log',
                      help='Path to JavaJAM trace file')
    parser.add_argument('--output', default='/tmp',
                      help='Output directory for split files')
    parser.add_argument('--compare-only', action='store_true',
                      help='Only compare, don\'t write files')

    args = parser.parse_args()

    print("="*120)
    print("PVM TRACE SPLITTER AND COMPARATOR")
    print("="*120)

    # Extract services
    jamduna_services = extract_jamduna_services(args.jamduna)
    javajam_services = extract_javajam_services(args.javajam, jamduna_services)

    # Write service files
    if not args.compare_only:
        jam_files = write_service_files(jamduna_services, 'jamduna', args.output)
        jav_files = write_service_files(javajam_services, 'javajam', args.output)

        print("\n" + "="*120)
        print("FILES CREATED")
        print("="*120)
        print(f"\n{'Service':<10} {'jamDuna File':<45} {'JavaJAM File':<45}")
        print("-"*100)
        for i, (jf, jvf) in enumerate(zip(jam_files, jav_files), 1):
            print(f"Service {i:<3} {jf:<45} {jvf:<45}")

    # Compare services
    compare_services(jamduna_services, javajam_services)

    print("\n" + "="*120)
    print("DONE")
    print("="*120)

if __name__ == '__main__':
    main()
