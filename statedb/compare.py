#!/usr/bin/env python3
import re
import sys

def print_progress_bar(current, total, prefix='', length=40):
    """Print a progress bar that updates in place"""
    percent = current / total if total > 0 else 0
    filled = int(length * percent)
    bar = '█' * filled + '░' * (length - filled)
    sys.stdout.write(f'\r{prefix} [{bar}] {current}/{total} ({percent*100:.1f}%)')
    sys.stdout.flush()

def remove_ansi_codes(text):
    """Remove ANSI escape sequences"""
    ansi_escape = re.compile(r'\x1b\[[0-9;]*m')
    return ansi_escape.sub('', text)

def parse_interpreter_line(line):
    """Parse a line in interpreter format"""
    # First remove ANSI codes
    line = remove_ansi_codes(line)
    # Format: OPCODE counter pc Gas: xxx Registers:[...]
    match = re.search(r'(\w+)\s+(\d+)\s+(\d+)\s+.*Registers:\s*\[([\d\s,]+)\]', line)
    if match:
        opcode = match.group(1)
        counter = match.group(2)
        pc = match.group(3)
        regs_str = match.group(4)
        regs = [int(x.strip()) for x in regs_str.split(',')]
        return {'counter': counter, 'pc': pc, 'opcode': opcode, 'registers': regs}
    return None

def parse_recompiler_line(line):
    """Parse a line in recompiler format"""
    # First remove ANSI codes
    line = remove_ansi_codes(line)
    # [DEBUG] PC=... Opcode=... | r0=... r1=... ...
    match = re.search(r'\[DEBUG\]\s+PC=(\d+)\s+Opcode=0x[0-9a-f]+\s+\((\w+)\)\s+\|\s+(.*)', line)
    if match:
        pc = match.group(1)
        opcode = match.group(2)
        regs_str = match.group(3)

        # Parse registers
        regs = []
        for i in range(13):  # r0-r12
            reg_match = re.search(rf'r{i}=0x([0-9a-f]+)', regs_str)
            if reg_match:
                regs.append(int(reg_match.group(1), 16))
            else:
                # Also try decimal format
                reg_match_dec = re.search(rf'r{i}=(\d+)', regs_str)
                if reg_match_dec:
                    regs.append(int(reg_match_dec.group(1)))
                else:
                    regs.append(None)

        return {'pc': pc, 'opcode': opcode, 'registers': regs}
    return None

def print_raw_context(data, idx, name, raw_lines, context=5):
    """Print raw lines from original file around the given index"""
    start = max(0, idx - context)
    end = min(len(data), idx + context + 1)
    print(f"\n{name} raw context (idx {start} to {end - 1}):")
    print("-" * 120)
    for i in range(start, end):
        inst = data[i]
        line_no = inst.get('line_no')
        marker = " <<<" if i == idx else ""
        if line_no is not None and line_no < len(raw_lines):
            raw = raw_lines[line_no].rstrip()
            # Truncate if too long
            if len(raw) > 150:
                raw = raw[:150] + "..."
            print(f"  [{i:6}] L{line_no:>7}: {raw}{marker}")
        else:
            print(f"  [{i:6}] (no raw line){marker}")
    print("-" * 120)

def main():
    # Read the two files
    with open('interpreter.txt', 'r', errors='ignore') as f:
        interpreter_lines = f.readlines()

    with open('recompiler.txt', 'r', errors='ignore') as f:
        recompiler_lines = f.readlines()

    # Parse the two files
    # Keep all instructions for special handling
    interpreter_data_all = []
    total_interp_lines = len(interpreter_lines)
    for idx, line in enumerate(interpreter_lines):
        if (idx + 1) % 1000 == 0 or idx == total_interp_lines - 1:
            print_progress_bar(idx + 1, total_interp_lines, 'Parsing interpreter.txt')
        parsed = parse_interpreter_line(line)
        if parsed:
            parsed['line_no'] = idx  # Record original line number
            interpreter_data_all.append(parsed)
    print()  # newline after progress bar

    # Build the interpreter list used for comparison
    # Rules: keep ECALLI/SBRK, remove FALLTHROUGH, keep other instructions
    interpreter_data = []
    for i, inst in enumerate(interpreter_data_all):
        if inst['opcode'] in ['ECALLI', 'SBRK']:
            # Keep ECALLI/SBRK and mark if followed by FALLTHROUGH
            inst['has_fallthrough'] = (i + 1 < len(interpreter_data_all) and
                                      interpreter_data_all[i + 1]['opcode'] == 'FALLTHROUGH')
            interpreter_data.append(inst)
        elif inst['opcode'] != 'FALLTHROUGH':
            # Keep non-FALLTHROUGH instructions
            inst['has_fallthrough'] = False
            interpreter_data.append(inst)

    recompiler_data = []
    total_recomp_lines = len(recompiler_lines)
    for idx, line in enumerate(recompiler_lines):
        if (idx + 1) % 1000 == 0 or idx == total_recomp_lines - 1:
            print_progress_bar(idx + 1, total_recomp_lines, 'Parsing recompiler.txt ')
        parsed = parse_recompiler_line(line)
        if parsed and parsed['opcode'] != 'FALLTHROUGH':
            parsed['line_no'] = idx  # Record original line number
            # Skip FALLTHROUGH, only keep real instructions
            recompiler_data.append(parsed)
    print()  # newline after progress bar

    print(f"Parsed interpreter instructions (including ECALLI/SBRK): {len(interpreter_data)}")
    print(f"Parsed recompiler instructions: {len(recompiler_data)}")
    print()

    # Compare registers
    # Strategy: compare in order, but if interpreter's next is ECALLI/SBRK, interpreter skips an extra one
    first_diff = None
    interp_idx = 0
    recomp_idx = 0
    compare_count = 0

    total_compare = min(len(interpreter_data), len(recompiler_data))
    stage = 1
    while interp_idx < len(interpreter_data) - 1 and recomp_idx < len(recompiler_data) - 1:
        compare_count += 1
        if compare_count % 1000 == 0:
            print_progress_bar(compare_count, total_compare, f'Comparing (stage {stage})  ')
        interp_curr = interpreter_data[interp_idx]
        interp_next = interpreter_data[interp_idx + 1]
        recomp_next = recompiler_data[recomp_idx + 1]

        # Detect new execution stage: recompiler PC goes back to 0
        if recomp_next['pc'] == '0' and recomp_idx > 0:
            prev_recomp = recompiler_data[recomp_idx]
            if prev_recomp['pc'] != '0':  # PC jumped to 0 from non-zero
                print(f"\n[Stage {stage} complete] Recompiler PC reset to 0 at recomp_idx={recomp_idx + 1}")
                print(f"  Compared {compare_count} instructions in stage {stage}")
                stage += 1
                # Find next interpreter instruction with PC=0 to re-align
                found_align = False
                for i in range(interp_idx + 1, len(interpreter_data)):
                    if interpreter_data[i]['pc'] == '0':
                        print(f"  Re-aligned: interpreter jumping from idx {interp_idx} to {i}")
                        interp_idx = i
                        recomp_idx += 1
                        found_align = True
                        break
                if found_align:
                    continue
                else:
                    print(f"  Warning: Could not find interpreter PC=0 to re-align")

        # Check if interpreter's next instruction is ECALLI/SBRK
        if interp_next['opcode'] in ['ECALLI', 'SBRK']:
            # interpreter[i+1] (after ECALLI/SBRK executed) vs recompiler[j+1] before execution
            interp_compare = interp_next

            regs_match = True
            diff_regs = []
            for j in range(13):
                if j < len(interp_compare['registers']) and recomp_next['registers'][j] is not None:
                    if interp_compare['registers'][j] != recomp_next['registers'][j]:
                        regs_match = False
                        diff_regs.append(j)

            if not regs_match and first_diff is None:
                first_diff = interp_idx
                print(f"Found first register difference!")
                print(f"Location: Interpreter[{interp_idx + 1}] (after ECALLI/SBRK) → Recompiler[{recomp_idx + 1}]")
                print(f"\nDifferent registers: {diff_regs}")
                print(f"\nInterpreter[{interp_idx + 1}] {interp_compare['opcode']} after execution:")
                print(f"  Counter: {interp_compare['counter']}")
                print(f"  PC: {interp_compare['pc']}")
                for j in range(13):
                    if j < len(interp_compare['registers']):
                        marker = " <--" if j in diff_regs else ""
                        print(f"  r{j}: {interp_compare['registers'][j]:20}{marker}")

                print(f"\nRecompiler[{recomp_idx + 1}] {recomp_next['opcode']} before execution:")
                print(f"  PC: {recomp_next['pc']}")
                for j in range(13):
                    if recomp_next['registers'][j] is not None:
                        marker = " <--" if j in diff_regs else ""
                        print(f"  r{j}: {recomp_next['registers'][j]:20}{marker}")

                # Print raw context from original files
                print_raw_context(interpreter_data, interp_idx + 1, "Interpreter", interpreter_lines)
                print_raw_context(recompiler_data, recomp_idx + 1, "Recompiler", recompiler_lines)
                print("\n" + "="*60)
                break

            # ECALLI/SBRK: interpreter advances by 2 (skip ECALLI/SBRK), recompiler advances by 1
            interp_idx += 2
            recomp_idx += 1
        else:
            # Normal instruction: interpreter[i] after execution vs recompiler[j+1] before execution
            regs_match = True
            diff_regs = []
            for j in range(13):
                if j < len(interp_curr['registers']) and recomp_next['registers'][j] is not None:
                    if interp_curr['registers'][j] != recomp_next['registers'][j]:
                        regs_match = False
                        diff_regs.append(j)

            if not regs_match and first_diff is None:
                first_diff = interp_idx
                print(f"Found first register difference!")
                print(f"Location: Interpreter[{interp_idx}] → Recompiler[{recomp_idx + 1}]")
                print(f"\nDifferent registers: {diff_regs}")
                print(f"\nInterpreter[{interp_idx}] {interp_curr['opcode']} after execution:")
                print(f"  Counter: {interp_curr['counter']}")
                print(f"  PC: {interp_curr['pc']}")
                for j in range(13):
                    if j < len(interp_curr['registers']):
                        marker = " <--" if j in diff_regs else ""
                        print(f"  r{j}: {interp_curr['registers'][j]:20}{marker}")

                print(f"\nRecompiler[{recomp_idx + 1}] {recomp_next['opcode']} before execution:")
                print(f"  PC: {recomp_next['pc']}")
                for j in range(13):
                    if recomp_next['registers'][j] is not None:
                        marker = " <--" if j in diff_regs else ""
                        print(f"  r{j}: {recomp_next['registers'][j]:20}{marker}")

                print(f"\nInterpreter[{interp_idx + 1}] {interp_next['opcode']} after execution (for reference):")
                print(f"  Counter: {interp_next['counter']}")
                print(f"  PC: {interp_next['pc']}")

                # Print raw context from original files
                print_raw_context(interpreter_data, interp_idx, "Interpreter", interpreter_lines)
                print_raw_context(recompiler_data, recomp_idx + 1, "Recompiler", recompiler_lines)
                print("\n" + "="*60)
                break

            # Normal case: both advance by 1
            interp_idx += 1
            recomp_idx += 1

    print()  # newline after progress bar
    if first_diff is None:
        print(f"No register differences found across all {compare_count} instructions")

if __name__ == '__main__':
    main()