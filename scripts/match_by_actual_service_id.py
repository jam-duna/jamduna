#!/usr/bin/env python3
"""
Proper 2-step service matching:
1. Extract services from jamDuna using actual service IDs from logs
2. Match JavaJAM's PC sequences to the correct jamDuna service
"""

import re
from collections import defaultdict

def parse_trace_line(line):
    """Parse a PVM trace line"""
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

def extract_jamduna_with_service_ids(trace_file, log_file):
    """
    Step 1: Extract jamDuna services using actual service IDs from debug logs.
    Returns dict: service_id -> list of trace entries
    """
    print(f"STEP 1: Extracting jamDuna services from {trace_file}")

    # Parse the log file to find service IDs and their execution order
    service_execution_order = []

    with open(log_file, 'r') as f:
        for line in f:
            # Look for our debug output: "!!! SingleAccumulate: ServiceID=..."
            match = re.search(r'!!! SingleAccumulate: ServiceID=(\d+), TotalGas=(\d+)', line)
            if match:
                service_id = int(match.group(1))
                total_gas = int(match.group(2))
                service_execution_order.append((service_id, total_gas))

    print(f"  Found {len(service_execution_order)} services in execution order:")
    for i, (svc_id, gas) in enumerate(service_execution_order, 1):
        print(f"    Execution #{i}: Service ID {svc_id}, TotalGas={gas:,}")

    # Now extract the actual trace entries for each service
    # Service boundaries are marked by "JUMP 1 5"
    services_by_id = {}
    current_service_entries = []
    service_index = 0

    with open(trace_file, 'r') as f:
        for line_num, line in enumerate(f, 1):
            parsed = parse_trace_line(line)
            if parsed:
                # Service boundary
                if parsed['instruction'] == 'JUMP' and parsed['step'] == 1 and parsed['pc'] == 5:
                    # Save previous service
                    if current_service_entries and service_index < len(service_execution_order):
                        service_id, expected_gas = service_execution_order[service_index]
                        services_by_id[service_id] = {
                            'service_id': service_id,
                            'entries': current_service_entries,
                            'initial_gas': current_service_entries[0]['gas'] if current_service_entries else 0,
                            'expected_gas': expected_gas,
                            'execution_order': service_index + 1
                        }
                        service_index += 1

                    # Start new service
                    current_service_entries = [parsed]
                else:
                    current_service_entries.append(parsed)

            if line_num % 50000 == 0:
                print(f"  Processed {line_num} lines...")

    # Save last service
    if current_service_entries and service_index < len(service_execution_order):
        service_id, expected_gas = service_execution_order[service_index]
        services_by_id[service_id] = {
            'service_id': service_id,
            'entries': current_service_entries,
            'initial_gas': current_service_entries[0]['gas'] if current_service_entries else 0,
            'expected_gas': expected_gas,
            'execution_order': service_index + 1
        }

    print(f"\n  Extracted {len(services_by_id)} services:")
    for svc_id in sorted(services_by_id.keys()):
        svc = services_by_id[svc_id]
        print(f"    Service ID {svc_id}: {len(svc['entries'])} steps, initial_gas={svc['initial_gas']:,}, exec_order={svc['execution_order']}")

    return services_by_id

def extract_javajam_by_pc_matching(trace_file, jamduna_services):
    """
    Step 2: Extract JavaJAM traces by matching PC sequences to jamDuna services.
    Returns dict: service_id -> list of trace entries
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
                print(f"  Loaded {line_num} lines...")

    print(f"  Total JavaJAM entries: {len(javajam_all)}")

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

            print(f"    Matched {len(matched_entries)}/{len(jam_pcs)} PCs")
            print(f"    JavaJAM initial_gas: {javajam_services[svc_id]['initial_gas']:,}")
            print(f"    jamDuna initial_gas: {jam_svc['initial_gas']:,}")
            print(f"    Gas difference: {jam_svc['initial_gas'] - javajam_services[svc_id]['initial_gas']:+,}")
        else:
            print(f"    ❌ No match found!")

    return javajam_services

def write_matched_services(jamduna_services, javajam_services, output_dir):
    """Write matched services to files named by service ID"""
    import os
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
        jam_file = f"{output_dir}/jamduna_service_{svc_id}.log"
        jav_file = f"{output_dir}/javajam_service_{svc_id}.log"

        with open(jam_file, 'w') as f:
            for entry in jam_svc['entries']:
                f.write(entry['line'] + '\n')

        with open(jav_file, 'w') as f:
            for entry in jav_svc['entries']:
                f.write(entry['line'] + '\n')

        gas_diff = jam_svc['initial_gas'] - jav_svc['initial_gas']

        print(f"\n  Service ID {svc_id}:")
        print(f"    jamDuna: {len(jam_svc['entries']):6} steps, gas={jam_svc['initial_gas']:,}, exec_order={jam_svc['execution_order']}")
        print(f"    JavaJAM: {len(jav_svc['entries']):6} steps, gas={jav_svc['initial_gas']:,}")
        print(f"    Gas diff: {gas_diff:+,}")
        print(f"    Files:")
        print(f"      {jam_file}")
        print(f"      {jav_file}")

def compare_services(jamduna_services, javajam_services):
    """Compare matched services in detail"""
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
        print(f"Initial gas: jamDuna={jam_svc['initial_gas']:,}, JavaJAM={jav_svc['initial_gas']:,}, diff={jam_svc['initial_gas'] - jav_svc['initial_gas']:+,}")
        print(f"Gas differences: {len(gas_diffs):,} positions out of {min(len(jam_svc['entries']), len(jav_svc['entries'])):,} compared")

        if gas_diffs:
            # Categorize differences
            by_diff_value = defaultdict(int)
            for _, diff, _, _ in gas_diffs:
                by_diff_value[diff] += 1

            print(f"\nGas difference breakdown:")
            for diff in sorted(by_diff_value.keys(), reverse=True)[:10]:
                count = by_diff_value[diff]
                pct = 100 * count / len(gas_diffs)
                print(f"  {diff:+12,} gas: {count:6,} occurrences ({pct:5.1f}%)")

def main():
    import argparse

    parser = argparse.ArgumentParser(description='Match services by actual Service ID')
    parser.add_argument('--jamduna-trace', default='/Users/michael/Downloads/1763371127_jamduna_trace.log',
                      help='jamDuna trace file')
    parser.add_argument('--jamduna-log', default='/tmp/jamduna_test.log',
                      help='jamDuna test output with service ID logs')
    parser.add_argument('--javajam-trace', default='/Users/michael/Downloads/1763371127_javajam_trace.log',
                      help='JavaJAM trace file')
    parser.add_argument('--output', default='/tmp/services_by_id',
                      help='Output directory')

    args = parser.parse_args()

    print("="*120)
    print("PVM TRACE MATCHER BY ACTUAL SERVICE ID")
    print("="*120)

    # Step 1: Extract jamDuna services with real service IDs
    jamduna_services = extract_jamduna_with_service_ids(args.jamduna_trace, args.jamduna_log)

    # Step 2: Match JavaJAM by PC sequences
    javajam_services = extract_javajam_by_pc_matching(args.javajam_trace, jamduna_services)

    # Write matched services
    write_matched_services(jamduna_services, javajam_services, args.output)

    # Compare services
    compare_services(jamduna_services, javajam_services)

    print(f"\n{'='*120}")
    print("DONE - Services matched by actual Service ID")
    print(f"{'='*120}")

if __name__ == '__main__':
    main()
