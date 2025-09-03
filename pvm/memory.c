#include "pvm.h"


uint32_t vm_get_current_heap_pointer(VM* vm) {
    return vm->current_heap_pointer;
}

void vm_set_current_heap_pointer(VM* vm, uint32_t pointer) {
    vm->current_heap_pointer = pointer;
}

void vm_allocate_pages(VM* vm, uint32_t start_page, uint32_t page_count) {
    uint32_t required = (start_page + page_count) * Z_P;
    if (vm->rw_data_address_end - vm->rw_data_address < required) {
        // In a real implementation, this would need to grow the rw_data buffer
        // For now, we'll just update the end address
        vm->rw_data_address_end = vm->rw_data_address + required;
    }
}

static inline void put_uint16_le(uint8_t* buf, uint16_t val) {
    buf[0] = (uint8_t)(val & 0xFF);
    buf[1] = (uint8_t)((val >> 8) & 0xFF);
}

static inline void put_uint32_le(uint8_t* buf, uint32_t val) {
    buf[0] = (uint8_t)(val & 0xFF);
    buf[1] = (uint8_t)((val >> 8) & 0xFF);
    buf[2] = (uint8_t)((val >> 16) & 0xFF);
    buf[3] = (uint8_t)((val >> 24) & 0xFF);
}

static inline void put_uint64_le(uint8_t* buf, uint64_t val) {
    buf[0] = (uint8_t)(val & 0xFF);
    buf[1] = (uint8_t)((val >> 8) & 0xFF);
    buf[2] = (uint8_t)((val >> 16) & 0xFF);
    buf[3] = (uint8_t)((val >> 24) & 0xFF);
    buf[4] = (uint8_t)((val >> 32) & 0xFF);
    buf[5] = (uint8_t)((val >> 40) & 0xFF);
    buf[6] = (uint8_t)((val >> 48) & 0xFF);
    buf[7] = (uint8_t)((val >> 56) & 0xFF);
}

static inline uint16_t get_uint16_le(const uint8_t* buf) {
    return (uint16_t)buf[0] | ((uint16_t)buf[1] << 8);
}

static inline uint32_t get_uint32_le(const uint8_t* buf) {
    return (uint32_t)buf[0] | ((uint32_t)buf[1] << 8) | 
           ((uint32_t)buf[2] << 16) | ((uint32_t)buf[3] << 24);
}

static inline uint64_t get_uint64_le(const uint8_t* buf) {
    return (uint64_t)buf[0] | ((uint64_t)buf[1] << 8) | 
           ((uint64_t)buf[2] << 16) | ((uint64_t)buf[3] << 24) |
           ((uint64_t)buf[4] << 32) | ((uint64_t)buf[5] << 40) |
           ((uint64_t)buf[6] << 48) | ((uint64_t)buf[7] << 56);
}

// Memory write functions
uint64_t vm_write_ram_bytes_64(VM* vm, uint32_t address, uint64_t data) {
    uint32_t heap_end = p_func(vm->current_heap_pointer);

    // RW data / heap region (writable up to heap_end)
    if (address >= vm->rw_data_address && address <= heap_end - 8) {
        uint32_t offset = address - vm->rw_data_address;
        // Safety guard for dynamic growth
        if (offset + 8 > vm->rw_data_address_end - vm->rw_data_address) {
            return OOB;
        }
        put_uint64_le(vm->rw_data + offset, data);
        return OK;
    }

    // Output region (writable)
    if (address >= vm->output_address && address <= vm->output_end - 8) {
        uint32_t offset = address - vm->output_address;
        put_uint64_le(vm->output + offset, data);
        return OK;
    }

    // Stack region (writable)
    if (address >= vm->stack_address && address <= vm->stack_address_end - 8) {
        uint32_t offset = address - vm->stack_address;
        put_uint64_le(vm->stack + offset, data);
        return OK;
    }

    // RO data region (writes are OOB but we mimic prior behavior of copying then OOB)
    if (address >= vm->ro_data_address && address <= vm->ro_data_address_end - 8) {
        uint32_t offset = address - vm->ro_data_address;
        put_uint64_le(vm->ro_data + offset, data);
        return OOB;
    }

    return OOB;
}

uint64_t vm_write_ram_bytes_32(VM* vm, uint32_t address, uint32_t data) {
    uint32_t heap_end = p_func(vm->current_heap_pointer);

    // RW data / heap region (writable up to heap_end)
    if (address >= vm->rw_data_address && address <= heap_end - 4) {
        uint32_t offset = address - vm->rw_data_address;
        if (offset + 4 > vm->rw_data_address_end - vm->rw_data_address) {
            return OOB;
        }
        put_uint32_le(vm->rw_data + offset, data);
        return OK;
    }

    // Output region (writable)
    if (address >= vm->output_address && address <= vm->output_end - 4) {
        uint32_t offset = address - vm->output_address;
        put_uint32_le(vm->output + offset, data);
        return OK;
    }

    // Stack region (writable)
    if (address >= vm->stack_address && address <= vm->stack_address_end - 4) {
        uint32_t offset = address - vm->stack_address;
        put_uint32_le(vm->stack + offset, data);
        return OK;
    }

    // RO data region (writes are OOB but we mimic prior behavior of copying then OOB)
    if (address >= vm->ro_data_address && address <= vm->ro_data_address_end - 4) {
        uint32_t offset = address - vm->ro_data_address;
        put_uint32_le(vm->ro_data + offset, data);
        return OOB;
    }

    return OOB;
}

uint64_t vm_write_ram_bytes_16(VM* vm, uint32_t address, uint16_t data) {
    uint32_t heap_end = p_func(vm->current_heap_pointer);

    // RW data / heap region (writable up to heap_end)
    if (address >= vm->rw_data_address && address <= heap_end - 2) {
        uint32_t offset = address - vm->rw_data_address;
        if (offset + 2 > vm->rw_data_address_end - vm->rw_data_address) {
            return OOB;
        }
        put_uint16_le(vm->rw_data + offset, data);
        return OK;
    }

    // Output region (writable)
    if (address >= vm->output_address && address <= vm->output_end - 2) {
        uint32_t offset = address - vm->output_address;
        put_uint16_le(vm->output + offset, data);
        return OK;
    }

    // Stack region (writable)
    if (address >= vm->stack_address && address <= vm->stack_address_end - 2) {
        uint32_t offset = address - vm->stack_address;
        put_uint16_le(vm->stack + offset, data);
        return OK;
    }

    // RO data region (writes are OOB but we mimic prior behavior of copying then OOB)
    if (address >= vm->ro_data_address && address <= vm->ro_data_address_end - 2) {
        uint32_t offset = address - vm->ro_data_address;
        put_uint16_le(vm->ro_data + offset, data);
        return OOB;
    }

    return OOB;
}

uint64_t vm_write_ram_bytes_8(VM* vm, uint32_t address, uint8_t data) {
    uint32_t heap_end = p_func(vm->current_heap_pointer);

    // RW data / heap region (writable up to heap_end)
    if (address >= vm->rw_data_address && address < heap_end) {
        uint32_t offset = address - vm->rw_data_address;
        if (offset >= vm->rw_data_address_end - vm->rw_data_address) {
            return OOB;
        }
        vm->rw_data[offset] = data;
        return OK;
    }

    // Output region (writable)
    if (address >= vm->output_address && address < vm->output_end) {
        uint32_t offset = address - vm->output_address;
        vm->output[offset] = data;
        return OK;
    }

    // Stack region (writable)
    if (address >= vm->stack_address && address < vm->stack_address_end) {
        uint32_t offset = address - vm->stack_address;
        vm->stack[offset] = data;
        return OK;
    }

    // RO data region (writes are OOB but we mimic prior behavior of copying then OOB)
    if (address >= vm->ro_data_address && address < vm->ro_data_address_end) {
        uint32_t offset = address - vm->ro_data_address;
        vm->ro_data[offset] = data;
        return OOB;
    }

    return OOB;
}

uint64_t vm_write_ram_bytes(VM* vm, uint32_t address, uint8_t* data, uint32_t length) {
    if (length == 0) {
        return OK;
    }
    
    uint32_t heap_end = p_func(vm->current_heap_pointer);

    // RW data / heap region (writable up to heap_end)
    if (address >= vm->rw_data_address && address <= heap_end - length) {
        uint32_t offset = address - vm->rw_data_address;
        if (offset + length > vm->rw_data_address_end - vm->rw_data_address) {
            return OOB;
        }
        memcpy(vm->rw_data + offset, data, length);
        return OK;
    }

    // Output region (writable)
    if (address >= vm->output_address && address <= vm->output_end - length) {
        uint32_t offset = address - vm->output_address;
        memcpy(vm->output + offset, data, length);
        return OK;
    }

    // Stack region (writable)
    if (address >= vm->stack_address && address <= vm->stack_address_end - length) {
        uint32_t offset = address - vm->stack_address;
        memcpy(vm->stack + offset, data, length);
        return OK;
    }

    // RO data region (writes are OOB but we mimic prior behavior of copying then OOB)
    if (address >= vm->ro_data_address && address <= vm->ro_data_address_end - length) {
        uint32_t offset = address - vm->ro_data_address;
        memcpy(vm->ro_data + offset, data, length);
        return OOB;
    }

    return OOB;
}

// Memory read functions
uint64_t vm_read_ram_bytes(VM* vm, uint32_t address, uint32_t length, uint8_t* buffer) {
    uint32_t heap_end = p_func(vm->current_heap_pointer);

    // RW data / heap
    if (address >= vm->rw_data_address && address <= heap_end - length) {
        uint32_t offset = address - vm->rw_data_address;
        memcpy(buffer, vm->rw_data + offset, length);
        return OK;
    }

    // RO data
    if (address >= vm->ro_data_address && address <= vm->ro_data_address_end - length) {
        uint32_t offset = address - vm->ro_data_address;
        memcpy(buffer, vm->ro_data + offset, length);
        return OK;
    }

    // Output region
    if (address >= vm->output_address && address <= vm->output_end - length) {
        uint32_t offset = address - vm->output_address;
        memcpy(buffer, vm->output + offset, length);
        return OK;
    }

    // Stack
    if (address >= vm->stack_address && address <= vm->stack_address_end - length) {
        uint32_t offset = address - vm->stack_address;
        memcpy(buffer, vm->stack + offset, length);
        return OK;
    }

    return OOB;
}

uint8_t vm_read_ram_bytes_8(VM* vm, uint32_t addr, int* err_code) {
    uint32_t heap_end = p_func(vm->current_heap_pointer);

    // RW data / heap
    if (addr >= vm->rw_data_address && addr < heap_end) {
        uint32_t offset = addr - vm->rw_data_address;
        if (offset >= vm->rw_data_address_end - vm->rw_data_address) {
            *err_code = OOB;
            return 0;
        }
        *err_code = OK;
        return vm->rw_data[offset];
    }

    // RO data
    if (addr >= vm->ro_data_address && addr < vm->ro_data_address_end) {
        uint32_t offset = addr - vm->ro_data_address;
        *err_code = OK;
        return vm->ro_data[offset];
    }

    // Output region
    if (addr >= vm->output_address && addr < vm->output_end) {
        uint32_t offset = addr - vm->output_address;
        *err_code = OK;
        return vm->output[offset];
    }

    // Stack
    if (addr >= vm->stack_address && addr < vm->stack_address_end) {
        uint32_t offset = addr - vm->stack_address;
        *err_code = OK;
        return vm->stack[offset];
    }

    *err_code = OOB;
    return 0;
}

uint16_t vm_read_ram_bytes_16(VM* vm, uint32_t addr, int* err_code) {
    uint32_t heap_end = p_func(vm->current_heap_pointer);

    // RW data / heap
    if (addr >= vm->rw_data_address && addr <= heap_end - 2) {
        uint32_t offset = addr - vm->rw_data_address;
        *err_code = OK;
        return get_uint16_le(vm->rw_data + offset);
    }

    // RO data
    if (addr >= vm->ro_data_address && addr <= vm->ro_data_address_end - 2) {
        uint32_t offset = addr - vm->ro_data_address;
        *err_code = OK;
        return get_uint16_le(vm->ro_data + offset);
    }

    // Output region
    if (addr >= vm->output_address && addr <= vm->output_end - 2) {
        uint32_t offset = addr - vm->output_address;
        *err_code = OK;
        return get_uint16_le(vm->output + offset);
    }

    // Stack
    if (addr >= vm->stack_address && addr <= vm->stack_address_end - 2) {
        uint32_t offset = addr - vm->stack_address;
        *err_code = OK;
        return get_uint16_le(vm->stack + offset);
    }

    *err_code = OOB;
    return 0;
}

uint32_t vm_read_ram_bytes_32(VM* vm, uint32_t addr, int* err_code) {
    uint32_t heap_end = p_func(vm->current_heap_pointer);

    // RW data / heap
    if (addr >= vm->rw_data_address && addr <= heap_end - 4) {
        uint32_t offset = addr - vm->rw_data_address;
        *err_code = OK;
        return get_uint32_le(vm->rw_data + offset);
    }

    // RO data
    if (addr >= vm->ro_data_address && addr <= vm->ro_data_address_end - 4) {
        uint32_t offset = addr - vm->ro_data_address;
        *err_code = OK;
        return get_uint32_le(vm->ro_data + offset);
    }

    // Output region
    if (addr >= vm->output_address && addr <= vm->output_end - 4) {
        uint32_t offset = addr - vm->output_address;
        *err_code = OK;
        return get_uint32_le(vm->output + offset);
    }

    // Stack
    if (addr >= vm->stack_address && addr <= vm->stack_address_end - 4) {
        uint32_t offset = addr - vm->stack_address;
        *err_code = OK;
        return get_uint32_le(vm->stack + offset);
    }

    *err_code = OOB;
    return 0;
}

uint64_t vm_read_ram_bytes_64(VM* vm, uint32_t addr, int* err_code) {
    uint32_t heap_end = p_func(vm->current_heap_pointer);

    // RW data / heap
    if (addr >= vm->rw_data_address && addr <= heap_end - 8) {
        uint32_t offset = addr - vm->rw_data_address;
        *err_code = OK;
        return get_uint64_le(vm->rw_data + offset);
    }

    // RO data
    if (addr >= vm->ro_data_address && addr <= vm->ro_data_address_end - 8) {
        uint32_t offset = addr - vm->ro_data_address;
        *err_code = OK;
        return get_uint64_le(vm->ro_data + offset);
    }

    // Output region
    if (addr >= vm->output_address && addr <= vm->output_end - 8) {
        uint32_t offset = addr - vm->output_address;
        *err_code = OK;
        return get_uint64_le(vm->output + offset);
    }

    // Stack
    if (addr >= vm->stack_address && addr <= vm->stack_address_end - 8) {
        uint32_t offset = addr - vm->stack_address;
        *err_code = OK;
        return get_uint64_le(vm->stack + offset);
    }

    *err_code = OOB;
    return 0;
}