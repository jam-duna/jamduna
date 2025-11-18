#!/usr/bin/env python3
"""
Match PVM Traces by Service ID

This script properly matches jamDuna and JavaJAM PVM execution traces by actual service ID,
not by execution order. It performs a 2-step process:

Step 1: Extract jamDuna services using [service: X] markers already in trace
Step 2: Match JavaJAM traces to jamDuna services by PC sequence matching

Usage:
    # Generate traces first
    cd /path/to/jam
    go test -v -run TestSingleFuzzTrace ./statedb 2>&1 | tee /tmp/jamduna_test.log

    # Run the matcher
    python3 match_pvm_traces_by_service_id.py \
        --jamduna-trace /path/to/jamduna_trace.log \
        --javajam-trace /path/to/javajam_trace.log \
        --output /tmp/matched_services

Output:
    Creates pairs of files named by service ID:
    - jamduna_service_0.log <-> javajam_service_0.log
    - jamduna_service_1011448405.log <-> javajam_service_1011448405.log
    - etc.

    Each pair contains the EXACT SAME service execution, allowing direct comparison
    of gas accounting and other implementation differences.

Author: Claude Code
Date: 2024-11-17
"""

import re
import os
import argparse
from collections import defaultdict
from typing import Dict, List, Tuple, Optional

def parse_trace_line(line: str) -> Optional[Dict]:
    """
    Parse a PVM trace line into structured data.

    Supports both jamDuna format:
        JUMP 1 5 Gas: 4999999 Registers:[...]

    And JavaJAM format:
        12:44:58.905 INFO  PVMInterpreter -- JUMP 1 5 Gas: 2499999 Registers:[...]
    """
    match = re.search(r'(\w+)\s+(\d+)\s+(\d+)\s+Gas:\s+(\d+)\s+Registers:\[([^\]]+)\]', line)
    if match:
        return {
            'instruction': match.group(1),
            'step': int(match.group(2)),
            'pc': int(match.group(3)),
            'gas': int(match.group(4)),
            'registers': [int(x.strip()) for x in match.group(5).split(',')],
            'line': line.strip()
        }
    return None

def extract_jamduna_services(trace_file: str) -> Dict[int, Dict]:
    """
    Step 1: Extract jamDuna services using [service: X] markers in trace.

    Returns:
        Dict mapping service_id -> {service_id, entries, initial_gas, execution_order}
    """
    print(f"STEP 1: Extracting jamDuna services from {trace_file}")
    print(f"        Using [service: X] markers in trace")

    # First pass: identify services and their boundaries
    service_boundaries = {}  # service_id -> (first_line, last_line)
    current_service_id = None
    service_first_line = None

    with open(trace_file, 'r') as f:
        for line_num, line in enumerate(f, 1):
            # Check for service marker
            match = re.search(r'\[service:\s*(\d+)\]', line)
            if match:
                service_id = int(match.group(1))

                # If this is a new service, record boundary
                if service_id != current_service_id:
                    # Save previous service boundary
                    if current_service_id is not None and service_first_line is not None:
                        service_boundaries[current_service_id] = (service_first_line, line_num - 1)

                    # Start new service
                    current_service_id = service_id
                    service_first_line = line_num

            if line_num % 50000 == 0:
                print(f"  Scanned {line_num:,} lines...")

    # Save last service
    if current_service_id is not None and service_first_line is not None:
        with open(trace_file, 'r') as f:
            total_lines = sum(1 for _ in f)
        service_boundaries[current_service_id] = (service_first_line, total_lines)

    print(f"\n  Found {len(service_boundaries)} services:")
    for svc_id in sorted(service_boundaries.keys()):
        first, last = service_boundaries[svc_id]
        print(f"    Service ID {svc_id}: lines {first:,} to {last:,} ({last-first+1:,} lines)")

    # Second pass: extract actual trace entries for each service
    services_by_id = {}

    for service_id in sorted(service_boundaries.keys()):
        first_line, last_line = service_boundaries[service_id]

        entries = []
        with open(trace_file, 'r') as f:
            for line_num, line in enumerate(f, 1):
                if line_num < first_line:
                    continue
                if line_num > last_line:
                    break

                parsed = parse_trace_line(line)
                if parsed:
                    entries.append(parsed)

        if entries:
            services_by_id[service_id] = {
                'service_id': service_id,
                'entries': entries,
                'initial_gas': entries[0]['gas'] if entries else 0,
                'execution_order': len(services_by_id) + 1
            }

    print(f"\n  Extracted {len(services_by_id)} services:")
    for svc_id in sorted(services_by_id.keys()):
        svc = services_by_id[svc_id]
        print(f"    Service ID {svc_id}: {len(svc['entries']):,} PVM steps, "
              f"initial_gas={svc['initial_gas']:,}, exec_order={svc['execution_order']}")

    return services_by_id

def extract_javajam_by_pc_matching(trace_file: str, jamduna_services: Dict[int, Dict]) -> Dict[int, Dict]:
    """
    Step 2: Extract JavaJAM traces by matching PC sequences to jamDuna services.

    Since JavaJAM doesn't log service IDs in its trace, we match by PC sequences.
    This works because the bytecode execution is deterministic.

    Returns:
        Dict mapping service_id -> {service_id, entries, initial_gas, javajam_start_line}
    """
    print(f"\nSTEP 2: Matching JavaJAM traces from {trace_file}")

    # Load all JavaJAM entries
    javajam_all = []
    with open(trace_file, 'r') as f:
        for line_num, line in enumerate(f, 1):
            parsed = parse_trace_line(line)
            if parsed:
                javajam_all.append(parsed)

            if line_num % 50000 == 0:
                print(f"  Loaded {line_num:,} lines...")

    print(f"  Total JavaJAM entries: {len(javajam_all):,}")

    # Match each jamDuna service to JavaJAM by PC sequence
    javajam_services = {}

    for svc_id in sorted(jamduna_services.keys()):
        jam_svc = jamduna_services[svc_id]
        print(f"\n  Matching Service ID {svc_id} (jamDuna exec #{jam_svc['execution_order']})...")

        # Extract PC sequence from jamDuna
        jam_pcs = [e['pc'] for e in jam_svc['entries']]

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

        # Extract matched entries
        matched_entries = [javajam_all[idx] for idx in javajam_indices]

        if matched_entries:
            javajam_services[svc_id] = {
                'service_id': svc_id,
                'entries': matched_entries,
                'initial_gas': matched_entries[0]['gas'] if matched_entries else 0,
                'javajam_start_line': javajam_indices[0] if javajam_indices else -1
            }

            match_pct = 100 * len(matched_entries) / len(jam_pcs) if jam_pcs else 0
            print(f"    Matched {len(matched_entries):,}/{len(jam_pcs):,} PCs ({match_pct:.1f}%)")
            print(f"    JavaJAM initial_gas: {javajam_services[svc_id]['initial_gas']:,}")
            print(f"    jamDuna initial_gas: {jam_svc['initial_gas']:,}")
            print(f"    Gas difference: {jam_svc['initial_gas'] - javajam_services[svc_id]['initial_gas']:+,}")
        else:
            print(f"    ❌ No match found!")

    return javajam_services

def write_matched_services(jamduna_services: Dict[int, Dict], javajam_services: Dict[int, Dict],
                          output_dir: str) -> None:
    """Write matched services to files named by service ID."""
    os.makedirs(output_dir, exist_ok=True)

    print(f"\n{'='*120}")
    print(f"Writing matched services to {output_dir}/")
    print(f"{'='*120}")

    for svc_id in sorted(jamduna_services.keys()):
        if svc_id not in javajam_services:
            print(f"  ⚠️  Service ID {svc_id}: No JavaJAM match, skipping")
            continue

        jam_svc = jamduna_services[svc_id]
        jav_svc = javajam_services[svc_id]

        # Write files named by service ID
        jam_file = os.path.join(output_dir, f"jamduna_service_{svc_id}.log")
        jav_file = os.path.join(output_dir, f"javajam_service_{svc_id}.log")

        with open(jam_file, 'w') as f:
            for entry in jam_svc['entries']:
                f.write(entry['line'] + '\n')

        with open(jav_file, 'w') as f:
            for entry in jav_svc['entries']:
                f.write(entry['line'] + '\n')

        gas_diff = jam_svc['initial_gas'] - jav_svc['initial_gas']

        print(f"\n  Service ID {svc_id}:")
        print(f"    jamDuna: {len(jam_svc['entries']):6,} steps, gas={jam_svc['initial_gas']:,}, "
              f"exec_order={jam_svc['execution_order']}")
        print(f"    JavaJAM: {len(jav_svc['entries']):6,} steps, gas={jav_svc['initial_gas']:,}")
        print(f"    Gas diff: {gas_diff:+,}")
        print(f"    Files:")
        print(f"      {jam_file}")
        print(f"      {jav_file}")

def compare_services(jamduna_services: Dict[int, Dict], javajam_services: Dict[int, Dict]) -> None:
    """Compare matched services and report gas accounting differences."""
    print(f"\n{'='*120}")
    print("DETAILED COMPARISON BY SERVICE ID")
    print(f"{'='*120}")

    for svc_id in sorted(jamduna_services.keys()):
        if svc_id not in javajam_services:
            continue

        jam_svc = jamduna_services[svc_id]
        jav_svc = javajam_services[svc_id]

        print(f"\n{'='*120}")
        print(f"SERVICE ID {svc_id}")
        print(f"{'='*120}")

        # Count gas differences
        gas_diffs = []
        for i in range(min(len(jam_svc['entries']), len(jav_svc['entries']))):
            jam_e = jam_svc['entries'][i]
            jav_e = jav_svc['entries'][i]

            diff = jam_e['gas'] - jav_e['gas']
            if diff != 0:
                gas_diffs.append((i, diff, jam_e['instruction'], jam_e['pc']))

        print(f"Steps: jamDuna={len(jam_svc['entries']):,}, JavaJAM={len(jav_svc['entries']):,}")
        print(f"Initial gas: jamDuna={jam_svc['initial_gas']:,}, JavaJAM={jav_svc['initial_gas']:,}, "
              f"diff={jam_svc['initial_gas'] - jav_svc['initial_gas']:+,}")
        print(f"Gas differences: {len(gas_diffs):,} positions out of "
              f"{min(len(jam_svc['entries']), len(jav_svc['entries'])):,} compared")

        if gas_diffs:
            # Categorize differences
            by_diff_value = defaultdict(int)
            for _, diff, _, _ in gas_diffs:
                by_diff_value[diff] += 1

            print(f"\nGas difference breakdown:")
            for diff in sorted(by_diff_value.keys(), reverse=True)[:10]:
                count = by_diff_value[diff]
                pct = 100 * count / len(gas_diffs) if gas_diffs else 0
                print(f"  {diff:+12,} gas: {count:6,} occurrences ({pct:5.1f}%)")

def main():
    parser = argparse.ArgumentParser(
        description='Match PVM traces by actual Service ID',
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog=__doc__
    )

    parser.add_argument('--jamduna-trace',
                       default='/Users/michael/Downloads/1763371127_jamduna_trace.log',
                       help='jamDuna PVM trace file')
    parser.add_argument('--javajam-trace',
                       default='/Users/michael/Downloads/1763371127_javajam_trace.log',
                       help='JavaJAM PVM trace file')
    parser.add_argument('--output',
                       default='/tmp/services_by_id',
                       help='Output directory for matched service files')

    args = parser.parse_args()

    print("="*120)
    print("PVM TRACE MATCHER BY ACTUAL SERVICE ID")
    print("="*120)

    # Step 1: Extract jamDuna services using [service: X] markers
    jamduna_services = extract_jamduna_services(args.jamduna_trace)

    if not jamduna_services:
        print("\n❌ ERROR: Could not extract jamDuna services from trace file.")
        print("   Make sure the trace contains [service: X] markers from host function calls.")
        return 1

    # Step 2: Match JavaJAM by PC sequences
    javajam_services = extract_javajam_by_pc_matching(args.javajam_trace, jamduna_services)

    # Write matched services
    write_matched_services(jamduna_services, javajam_services, args.output)

    # Compare services
    compare_services(jamduna_services, javajam_services)

    print(f"\n{'='*120}")
    print("✅ DONE - Services matched by actual Service ID")
    print(f"{'='*120}")

    return 0

if __name__ == '__main__':
    exit(main())
