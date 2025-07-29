#!/usr/bin/env python3
"""
Extract pure disassembly instructions from TESTCASES.md and create individual test files
This version extracts only the pure assembly instructions without hex bytes
"""

import re
import os

def extract_pure_assembly_cases():
    # Read the TESTCASES.md file
    with open('TESTCASES.md', 'r') as f:
        content = f.read()
    
    # Create output directory if it doesn't exist
    output_dir = 'pure_disassembly_tests'
    os.makedirs(output_dir, exist_ok=True)
    
    # Pattern to match test cases
    pattern = r'^## (inst_\w+|gas_\w+)$.*?```\n(.*?)```'
    
    matches = re.findall(pattern, content, re.MULTILINE | re.DOTALL)
    
    extracted_count = 0
    
    for test_name, disassembly_block in matches:
        # Clean up the disassembly block - extract only the pure assembly instructions
        lines = disassembly_block.strip().split('\n')
        assembly_lines = []
        
        for line in lines:
            line = line.strip()
            # Skip empty lines, comments, and lines that don't contain assembly
            if line and not line.startswith(':') and line != 'invalid':
                # Extract the assembly part (after the hex bytes)
                # Format examples:
                # "     0: be 87 09                 i32 r9 = r7 + r8"
                # "     4: 21 20 00 00 01 00 12 34  u32 [0x20000] = 0x12345678"
                if ':' in line:
                    parts = line.split(':', 1)
                    if len(parts) == 2:
                        hex_and_asm = parts[1].strip()
                        
                        # Method 1: Split by multiple spaces (original method)
                        segments = re.split(r'\s{2,}', hex_and_asm)
                        if len(segments) >= 2:
                            assembly = segments[1].strip()
                        else:
                            # Method 2: If no multiple spaces, try to find assembly after hex pattern
                            # Look for pattern like "xx xx xx ... instruction"
                            hex_pattern = r'^([0-9a-fA-F]{2}\s*)+\s+'
                            match = re.search(hex_pattern, hex_and_asm)
                            if match:
                                assembly = hex_and_asm[match.end():].strip()
                            else:
                                # If no hex pattern found, take the whole thing
                                assembly = hex_and_asm.strip()
                        
                        # Final cleanup: ensure we have pure assembly
                        # Remove any remaining hex bytes at the start
                        assembly = re.sub(r'^([0-9a-fA-F]{2}\s*)+\s*', '', assembly)
                        assembly = assembly.strip()
                        
                        # Only add meaningful assembly lines
                        if (assembly and 
                            assembly != 'invalid' and 
                            not re.match(r'^[0-9a-fA-F\s]+$', assembly) and
                            len(assembly) > 3):  # Ensure it's not just hex remnants
                            assembly_lines.append(assembly)
        
        if assembly_lines:
            # Write to individual test file
            output_file = os.path.join(output_dir, f'{test_name}.txt')
            with open(output_file, 'w') as f:
                for line in assembly_lines:
                    f.write(line + '\n')
            
            extracted_count += 1
            print(f"Extracted {test_name}: {len(assembly_lines)} instructions")
    
    print(f"\nTotal extracted: {extracted_count} test cases")
    print(f"Output directory: {output_dir}")

if __name__ == '__main__':
    extract_pure_assembly_cases()
