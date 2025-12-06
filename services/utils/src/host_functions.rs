#[polkavm_derive::polkavm_import]
extern "C" {
    // general
    #[polkavm_import(index = 0)]
    pub fn gas() -> u64;

    #[polkavm_import(index = 1)]
    pub fn fetch(o: u64, f: u64, l: u64, d_type: u64, index_0: u64, index_1: u64) -> u64;

    #[polkavm_import(index = 2)]
    pub fn lookup(s: u64, h: u64, o: u64, f: u64, l: u64) -> u64;

    #[polkavm_import(index = 3)]
    pub fn read(s: u64, ko: u64, kz: u64, o: u64, f: u64, l: u64) -> u64;

    #[polkavm_import(index = 4)]
    pub fn write(ko: u64, kz: u64, vo: u64, vz: u64) -> u64;

    #[polkavm_import(index = 5)]
    pub fn info(s: u64, bo: u64, f: u64, l: u64) -> u64;

    #[polkavm_import(index = 6)]
    pub fn historical_lookup(s: u64, h: u64, o: u64, f: u64, l: u64) -> u64;

    #[polkavm_import(index = 7)]
    pub fn export(p: u64, z: u64) -> u64;

    #[polkavm_import(index = 8)]
    pub fn machine(po: u64, pz: u64, i: u64) -> u64;

    #[polkavm_import(index = 9)]
    pub fn peek(n: u64, o: u64, s: u64, z: u64) -> u64;

    #[polkavm_import(index = 10)]
    pub fn poke(n: u64, s: u64, o: u64, z: u64) -> u64;

    #[polkavm_import(index = 11)]
    pub fn pages(n: u64, p: u64, c: u64, r: u64) -> u64;

    #[polkavm_import(index = 12)]
    pub fn invoke(n: u64, o: u64) -> (u64, u64);

    #[polkavm_import(index = 13)]
    pub fn expunge(n: u64) -> u64;

    #[polkavm_import(index = 14)]
    pub fn bless(m: u64, a: u64, v: u64, r: u64, o: u64, n: u64) -> u64;

    #[polkavm_import(index = 15)]
    pub fn assign(c: u64, o: u64, a: u64) -> u64;

    #[polkavm_import(index = 16)]
    pub fn designate(o: u64) -> u64;

    #[polkavm_import(index = 17)]
    pub fn checkpoint() -> u64;

    #[polkavm_import(index = 18)]
    pub fn new(o: u64, l: u64, g: u64, m: u64, f: u64, i: u64) -> u64;

    #[polkavm_import(index = 19)]
    pub fn upgrade(o: u64, g: u64, m: u64) -> u64;

    #[polkavm_import(index = 20)]
    pub fn transfer(d: u64, a: u64, l: u64, o: u64) -> u64;

    #[polkavm_import(index = 21)]
    pub fn eject(d: u64, o: u64) -> u64;

    #[polkavm_import(index = 22)]
    pub fn query(o: u64, z: u64) -> u64;

    #[polkavm_import(index = 23)]
    pub fn solicit(o: u64, z: u64) -> u64;

    #[polkavm_import(index = 24)]
    pub fn forget(o: u64, z: u64) -> u64;

    #[polkavm_import(index = 25)]
    pub fn oyield(o: u64) -> u64;

    #[polkavm_import(index = 26)]
    pub fn provide(s: u64, o: u64, z: u64) -> u64;

    #[polkavm_import(index = 100)]
    pub fn log(level: u64, target: u64, target_len: u64, message: u64, message_len: u64) -> u64; //https://hackmd.io/@polkadot/jip1

    #[polkavm_import(index = 63)]
    pub fn zero(n: u64, p: u64, c: u64) -> u64;

    #[polkavm_import(index = 254)]
    pub fn fetch_object(s: u64, ko: u64, kz: u64, o: u64, f: u64, l: u64) -> u64;
}

// AccumulateInstruction represents a deferred accumulate host function call
// Serialization format: [opcode:1][u64_params...][bytes_len:8][bytes...]
#[derive(Debug, Clone)]
pub struct AccumulateInstruction {
    pub opcode: u8, // Host function discriminator
    pub params: alloc::vec::Vec<u64>,
    pub data: alloc::vec::Vec<u8>,
}

impl AccumulateInstruction {
    // Host function opcodes
    pub const BLESS: u8 = 14;
    pub const ASSIGN: u8 = 15;
    pub const DESIGNATE: u8 = 16;
    pub const NEW: u8 = 18;
    pub const UPGRADE: u8 = 19;
    pub const TRANSFER: u8 = 20;
    pub const EJECT: u8 = 21;
    pub const WRITE: u8 = 4; // matches core host function WRITE
    pub const SOLICIT: u8 = 23;
    pub const FORGET: u8 = 24;
    pub const PROVIDE: u8 = 26;

    pub fn new(opcode: u8, params: alloc::vec::Vec<u64>, data: alloc::vec::Vec<u8>) -> Self {
        Self {
            opcode,
            params,
            data,
        }
    }

    /// Serialize instruction to bytes
    pub fn serialize(&self) -> alloc::vec::Vec<u8> {
        let mut result = alloc::vec::Vec::new();

        // Opcode (1 byte)
        result.push(self.opcode);

        // u64 parameters (8 bytes each, little-endian)
        for &param in &self.params {
            result.extend_from_slice(&param.to_le_bytes());
        }

        // Data length (8 bytes, little-endian)
        result.extend_from_slice(&(self.data.len() as u64).to_le_bytes());

        // Data bytes
        result.extend_from_slice(&self.data);

        result
    }

    /// Deserialize instruction from bytes, returns (instruction, bytes_consumed)
    pub fn deserialize(bytes: &[u8]) -> Option<(Self, usize)> {
        if bytes.is_empty() {
            return None;
        }

        let opcode = bytes[0];
        let mut offset = 1;

        // Determine expected parameter count based on opcode
        let param_count = match opcode {
            Self::BLESS => 4,     // m, v, r, n (+ bold_a then bold_z bytes in data)
            Self::ASSIGN => 2,    // c, a (+ queue bytes in data)
            Self::DESIGNATE => 0, // (+ validators bytes in data)
            Self::NEW => 5,       // l, g, m, f, i (+ codeHash bytes in data)
            Self::UPGRADE => 2,   // g, m (+ codeHash bytes in data)
            Self::TRANSFER => 3,  // d, a, g (+ memo bytes in data)
            Self::EJECT => 1,     // d (+ hashData bytes in data)
            Self::WRITE => 2,     // kz, vz (+ key bytes + value bytes in data)
            Self::SOLICIT => 1,   // z (+ hashData bytes in data)
            Self::FORGET => 1,    // z (+ hashData bytes in data)
            Self::PROVIDE => 2,   // s, z (+ data bytes in data)
            _ => return None,
        };

        // Read u64 parameters
        let params_bytes = param_count * 8;
        if offset + params_bytes > bytes.len() {
            return None;
        }

        let mut params = alloc::vec::Vec::with_capacity(param_count);
        for i in 0..param_count {
            let param_offset = offset + i * 8;
            let param_bytes: [u8; 8] = bytes[param_offset..param_offset + 8].try_into().ok()?;
            params.push(u64::from_le_bytes(param_bytes));
        }
        offset += params_bytes;

        // Read data length
        if offset + 8 > bytes.len() {
            return None;
        }
        let data_len_bytes: [u8; 8] = bytes[offset..offset + 8].try_into().ok()?;
        let data_len = u64::from_le_bytes(data_len_bytes) as usize;
        offset += 8;

        // Read data
        if offset + data_len > bytes.len() {
            return None;
        }
        let data = bytes[offset..offset + data_len].to_vec();
        offset += data_len;

        Some((
            Self {
                opcode,
                params,
                data,
            },
            offset,
        ))
    }

    /// Deserialize all instructions from a byte array
    pub fn deserialize_all(mut bytes: &[u8]) -> alloc::vec::Vec<Self> {
        let mut instructions = alloc::vec::Vec::new();

        while !bytes.is_empty() {
            match Self::deserialize(bytes) {
                Some((instruction, consumed)) => {
                    instructions.push(instruction);
                    bytes = &bytes[consumed..];
                }
                None => break,
            }
        }

        instructions
    }

    /// Serialize multiple instructions into a single byte array
    pub fn serialize_all(instructions: &[Self]) -> alloc::vec::Vec<u8> {
        let mut result = alloc::vec::Vec::new();
        for instruction in instructions {
            result.extend_from_slice(&instruction.serialize());
        }
        result
    }
}
