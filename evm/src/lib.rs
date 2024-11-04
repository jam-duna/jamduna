use std::collections::HashMap;
use sha3::{Digest, Keccak256};
use rlp::{Rlp, RlpStream};
use tiny_keccak::{Hasher, Keccak};
use std::error::Error;

#[derive(Debug)]
enum OpCode {
    STOP,
    ADD,
    MUL,
    SUB,
    DIV,
    SDIV,
    MOD,
    SMOD,
    ADDMOD,
    MULMOD,
    EXP,
    SIGNEXTEND,
    LT,
    GT,
    SLT,
    SGT,
    EQ,
    ISZERO,
    AND,
    OR,
    XOR,
    NOT,
    BYTE,
    SHL,
    SHR,
    SAR,
    KECCAK256,
    ADDRESS,
    BALANCE,
    ORIGIN,
    CALLER,
    CALLVALUE,
    CALLDATALOAD,
    CALLDATASIZE,
    CALLDATACOPY,
    CODESIZE,
    CODECOPY,
    GASPRICE,
    EXTCODESIZE,
    EXTCODECOPY,
    RETURNDATASIZE,
    RETURNDATACOPY,
    EXTCODEHASH,
    BLOCKHASH,
    COINBASE,
    TIMESTAMP,
    NUMBER,
    PREVRANDAO,
    GASLIMIT,
    CHAINID,
    SELFBALANCE,
    BASEFEE,
    BLOBHASH,
    BLOBBASEFEE,
    POP,
    MLOAD,
    MSTORE,
    MSTORE8,
    SLOAD,
    SSTORE,
    JUMP,
    JUMPI,
    PC,
    MSIZE,
    GAS,
    JUMPDEST,
    TLOAD,
    TSTORE,
    MCOPY,
    PUSH(u8),
    DUP(u8),
    SWAP(u8),
    LOG(usize),
    CREATE,
    CALL,
    CALLCODE,
    RETURN,
    DELEGATECALL,
    CREATE2,
    STATICCALL,
    REVERT,
    INVALID,
    SELFDESTRUCT,
}

#[derive(Debug, Clone)]
struct Account {
    balance: u64,
    nonce: u64,
    storage_root: Vec<u8>, // Merkle root for storage trie
    code_hash: Vec<u8>,
    destroyed: bool,         // Indicates if the account has been destroyed
}

#[derive(Debug, Clone)]
struct Transaction {
    from: u64,
    to: u64,
    value: u64,
    gas: u64,
    data: Vec<u8>, // Code or input data for contract calls
}

struct PatriciaTrie {
    root: Vec<u8>,
    nodes: HashMap<Vec<u8>, Vec<u8>>, // Node hash -> Node data
}

struct Block {
    block_number: u32,                 // Number of the block in the blockchain
    timestamp: u64,                    // Timestamp of the block
    transactions: Vec<Transaction>,    // List of transactions in the block
}


struct EVM {
    stack: Vec<u64>,                  // Stack used to hold intermediate values during execution
    memory: Vec<u8>,                  // Linear memory array for contract execution (initialized to zero)
    accounts: HashMap<u64, Account>,  // Map of account IDs to account data (e.g., balance, nonce, code)
    storage: HashMap<(u64, Vec<u8>), u64>, // Map of account and key pairs to their corresponding storage values
    
    balances: HashMap<u64, u64>,      // Mapping of account IDs to their balances
    calldata: Vec<u8>,                // Input data for the current call
    pc: usize,                        // Program counter, which tracks the current instruction in the code
    code: Vec<u8>,                    // Bytecode of the contract currently being executed
    jumpdests: HashMap<usize, bool>,  // Cache of valid jump destinations within the code
    returndata: Vec<u8>,              // Stores the data returned from the most recent call
    logs: Vec<(u64, Vec<u8>, Vec<u64>)>,    // Stores log entries (address, data, topics)

    balance: u64,                     // Balance of the current contract or account
    chainid: u64,                     // Chain ID (used for transaction signature verification)
    blockhash: u64,                   // Block hash for the current block (can be set dynamically)
    coinbase: u64,                    // Address of the miner/validator of the current block
    timestamp: u64,                   // Unix timestamp of the current block
    block_number: u64,                // Number of the current block
    prevrandao: u64,                  // Randomness value for the current block (replaces difficulty)
    gaslimit: u64,                    // Gas limit of the current block (maximum gas allowed)
    basefee: u64,                     // Base fee for gas in the current block (used in fee calculations)
    gas_remaining: u64,                     // Remaining gas available for the current execution
    next_address: u64,                      // Counter for generating unique contract addresses

    address: u64,                     // Address of the currently executing contract or account
    origin: u64,                      // Origin address of the transaction (initial sender)
    caller: u64,                      // Address of the caller (account that invoked the current call)
    callvalue: u64,                   // Amount of wei sent with the current call
    gasprice: u64,                    // Price per gas unit for the transaction
}

        
impl EVM {

    fn new(accounts: HashMap<u64, Account>) -> Self {
        let mut evm = Self {
            stack: Vec::new(),
            memory: vec![0; 1024],
            accounts,
            storage: HashMap::new(),
            balances: HashMap::new(),
            calldata: Vec::new(),
            pc: 0,
            code: Vec::new(),
            jumpdests: HashMap::new(),
            returndata: Vec::new(),
            logs: Vec::new(),
            balance: 0,
            chainid: 1,           // Default chain ID (e.g., 1 for Ethereum mainnet)
            blockhash: 0,
            coinbase: 0,
            timestamp: 0,
            block_number: 0,
            prevrandao: 0,
            gaslimit: u64::MAX,   // Set a high gas limit by default
            basefee: 0,
            gas_remaining: u64::MAX, // Start with the maximum gas available
            next_address: 1,      // Initialize the counter for unique contract addresses
            address: 0,           // Default address for execution
            origin: 0,            // Default origin of the transaction
            caller: 0,            // Default caller address
            callvalue: 0,         // Default call value in wei
            gasprice: 0,          // Default gas price
        };

        evm.analyze_jumpdests(); // Analyze and cache jump destinations in code
        evm
    }

    
    // Method to initialize the EVM's state with verified account and storage witnesses
    fn initialize_state(
        &mut self,
        accounts: HashMap<u64, Account>,
        balances: HashMap<u64, u64>,
        storage: HashMap<(u64, Vec<u8>), u64>,
    ) {
        self.accounts = accounts;
        self.balances = balances;
        self.storage = storage;
    }

    // Method to simulate execution of transactions in a block
    fn execute_block(
        &mut self,
        block: Block,
    ) -> (HashMap<u64, Account>, HashMap<u64, u64>, HashMap<(u64, Vec<u8>), u64>) {
        let mut changed_accounts = HashMap::new();
        let mut changed_balances = HashMap::new();
        let mut changed_storage = HashMap::new();

        for transaction in block.transactions.iter() {
            if transaction.data.is_empty() {
                // Native transfer
                self.native_transfer(transaction.from, transaction.to, transaction.value);

                // Track account changes
                let from_account = self.accounts.get(&transaction.from).cloned().unwrap();
                let to_account = self.accounts.get(&transaction.to).cloned().unwrap();

                changed_accounts.insert(transaction.from, from_account);
                changed_accounts.insert(transaction.to, to_account);
                changed_balances.insert(transaction.from, self.balances[&transaction.from]);
                changed_balances.insert(transaction.to, self.balances[&transaction.to]);
            } else {
                // Smart contract call
                self.execute_contract(transaction.clone());

                // Track account and storage changes after contract execution
                let from_account = self.accounts.get(&transaction.from).cloned().unwrap();
                let to_account = self.accounts.get(&transaction.to).cloned().unwrap();

                changed_accounts.insert(transaction.from, from_account);
                changed_accounts.insert(transaction.to, to_account);
                changed_balances.insert(transaction.from, self.balances[&transaction.from]);
                changed_balances.insert(transaction.to, self.balances[&transaction.to]);

                // Record any storage changes made by contract execution
                for key in self.storage.keys() {
                    if !changed_storage.contains_key(key) {
                        changed_storage.insert(key.clone(), self.storage[key]);
                    }
                }
            }
        }

        (changed_accounts, changed_balances, changed_storage)
    }
    
    // Method to simulate execution of transactions in a block
    fn execute_block_simple(&mut self, block: Block) {
        for transaction in block.transactions {
            if transaction.data.is_empty() {
                // Native transfer
                self.native_transfer(transaction.from, transaction.to, transaction.value);
            } else {
                // Smart contract call
                self.execute_contract(transaction);
            }
        }
    }

    // Executes a smart contract transaction using opcodes from the data
    fn execute_contract(&mut self, transaction: Transaction) {
        let sender_account = self.accounts.get_mut(&transaction.from);
        if let Some(sender) = sender_account {
            if sender.balance >= transaction.value {
                sender.balance -= transaction.value;
                sender.nonce += 1;
                self.balances.insert(transaction.from, sender.balance);
                self.balances.insert(transaction.to, transaction.value);

                // Load transaction data as code for interpretation
                self.code = transaction.data.clone();
                self.pc = 0; // Reset program counter

                // Interpret the code
                self.interpret();
                
                // Finalize balance
                println!("Contract call completed for account {}. Balance: {}", transaction.to, self.balances[&transaction.to]);
            } else {
                println!("Contract call failed: insufficient balance for account {}", transaction.from);
            }
        }
    }

// Executes a native transfer by adjusting balances and nonces
fn native_transfer(&mut self, from: u64, to: u64, value: u64) {
    let accounts = &mut self.accounts;

    // Separate mutable access to sender and receiver accounts
    let sender_balance;
    let receiver_balance;

    if let Some(sender) = accounts.get_mut(&from) {
        if sender.balance >= value {
            // Deduct from sender's balance and increment nonce
            sender.balance -= value;
            sender.nonce += 1;
            sender_balance = sender.balance;

            // Retrieve or initialize receiver account
            let receiver = accounts.entry(to).or_insert(Account {
                balance: 0,
                nonce: 0,
                storage_root: Vec::new(), // Initialize with an empty storage root
                code_hash: Vec::new(),    // Initialize with an empty code hash
                destroyed: false,
            });

            // Add value to receiver's balance and capture the final balance
            receiver.balance += value;
            receiver_balance = receiver.balance;

            println!(
                "Transferred {} from {} to {}. New balances: {} -> {}, {} -> {}",
                value, from, to, from, sender_balance, to, receiver_balance
            );
        } else {
            println!("Transfer failed: insufficient balance for account {}", from);
        }
    } else {
        println!("Transfer failed: sender account {} does not exist", from);
    }
}

        
    fn interpret(&mut self) {
        loop {
            if self.pc >= self.code.len() {
                break;
            }

            let opcode = self.fetch_opcode();
            match opcode {
                OpCode::STOP => break,
                OpCode::ADD => self.op_add(),
                OpCode::MUL => self.op_mul(),
                OpCode::SUB => self.op_sub(),
                OpCode::DIV => self.op_div(),
                OpCode::SDIV => self.op_sdiv(),
                OpCode::MOD => self.op_mod(),
                OpCode::SMOD => self.op_smod(),
                OpCode::ADDMOD => self.op_addmod(),
                OpCode::MULMOD => self.op_mulmod(),
                OpCode::EXP => self.op_exp(),
                OpCode::SIGNEXTEND => self.op_signextend(),
                OpCode::LT => self.op_lt(),
                OpCode::GT => self.op_gt(),
                OpCode::SLT => self.op_slt(),
                OpCode::SGT => self.op_sgt(),
                OpCode::EQ => self.op_eq(),
                OpCode::ISZERO => self.op_iszero(),
                OpCode::AND => self.op_and(),
                OpCode::OR => self.op_or(),
                OpCode::XOR => self.op_xor(),
                OpCode::NOT => self.op_not(),
                OpCode::BYTE => self.op_byte(),
                OpCode::SHL => self.op_shl(),
                OpCode::SHR => self.op_shr(),
                OpCode::SAR => self.op_sar(),
                OpCode::KECCAK256 => self.op_keccak256(),
                OpCode::ADDRESS => self.op_address(),
                OpCode::BALANCE => self.op_balance(),
                OpCode::ORIGIN => self.op_origin(),
                OpCode::CALLER => self.op_caller(),
                OpCode::CALLVALUE => self.op_callvalue(),
                OpCode::CALLDATALOAD => self.op_calldataload(),
                OpCode::CALLDATASIZE => self.op_calldatasize(),
                OpCode::CALLDATACOPY => self.op_calldatacopy(),
                OpCode::CODESIZE => self.op_codesize(),
                OpCode::CODECOPY => self.op_codecopy(),
                OpCode::GASPRICE => self.op_gasprice(),
                OpCode::EXTCODESIZE => self.op_extcodesize(),
                OpCode::EXTCODECOPY => self.op_extcodecopy(),
                OpCode::RETURNDATASIZE => self.op_returndatasize(),
                OpCode::RETURNDATACOPY => self.op_returndatacopy(),
                OpCode::EXTCODEHASH => self.op_extcodehash(),
                OpCode::BLOCKHASH => self.op_blockhash(),
                OpCode::COINBASE => self.op_coinbase(),
                OpCode::TIMESTAMP => self.op_timestamp(),
                OpCode::NUMBER => self.op_number(),
                OpCode::PREVRANDAO => self.op_prevrandao(),
                OpCode::GASLIMIT => self.op_gaslimit(),
                OpCode::CHAINID => self.op_chainid(),
                OpCode::SELFBALANCE => self.op_selfbalance(),
                OpCode::BASEFEE => self.op_basefee(),
                OpCode::BLOBHASH => self.op_blobhash(),
                OpCode::BLOBBASEFEE => self.op_blobbasefee(),
                OpCode::POP => self.op_pop(),
                OpCode::MLOAD => self.op_mload(),
                OpCode::MSTORE => self.op_mstore(),
                OpCode::MSTORE8 => self.op_mstore8(),
                OpCode::SLOAD => self.op_sload(self.address),
                OpCode::SSTORE => self.op_sstore(),
                OpCode::JUMP => self.op_jump(),
                OpCode::JUMPI => self.op_jumpi(),
                OpCode::PC => self.op_pc(),
                OpCode::MSIZE => self.op_msize(),
                OpCode::GAS => self.op_gas(),
                OpCode::JUMPDEST => {}, // Marker opcode
                OpCode::TLOAD => self.op_tload(),
                OpCode::TSTORE => self.op_tstore(),
                OpCode::MCOPY => self.op_mcopy(),
                OpCode::PUSH(value) => self.op_push(value),
                OpCode::DUP(n) => self.op_dup(n),
                OpCode::SWAP(n) => self.op_swap(n),
                OpCode::LOG(n) => self.op_log(n),
                OpCode::CREATE => self.op_create(),
                OpCode::CALL => self.op_call(),
                OpCode::CALLCODE => self.op_callcode(),
                OpCode::RETURN => self.op_return(),
                OpCode::DELEGATECALL => self.op_delegatecall(),
                OpCode::CREATE2 => self.op_create2(),
                OpCode::STATICCALL => self.op_staticcall(),
                OpCode::REVERT => self.op_revert(),
                OpCode::INVALID => println!("Invalid opcode"),
                OpCode::SELFDESTRUCT => self.op_selfdestruct(),
            }
        }
    }

    fn fetch_opcode(&mut self) -> OpCode {
        let opcode = self.code[self.pc];
        self.pc += 1;
        match opcode {
            0x00 => OpCode::STOP,
            0x01 => OpCode::ADD,
            0x02 => OpCode::MUL,
            0x03 => OpCode::SUB,
            0x04 => OpCode::DIV,
            0x05 => OpCode::SDIV,
            0x06 => OpCode::MOD,
            0x07 => OpCode::SMOD,
            0x08 => OpCode::ADDMOD,
            0x09 => OpCode::MULMOD,
            0x0a => OpCode::EXP,
            0x0b => OpCode::SIGNEXTEND,
            0x10 => OpCode::LT,
            0x11 => OpCode::GT,
            0x12 => OpCode::SLT,
            0x13 => OpCode::SGT,
            0x14 => OpCode::EQ,
            0x15 => OpCode::ISZERO,
            0x16 => OpCode::AND,
            0x17 => OpCode::OR,
            0x18 => OpCode::XOR,
            0x19 => OpCode::NOT,
            0x1a => OpCode::BYTE,
            0x1b => OpCode::SHL,
            0x1c => OpCode::SHR,
            0x1d => OpCode::SAR,
            0x20 => OpCode::KECCAK256,
            0x30 => OpCode::ADDRESS,
            0x31 => OpCode::BALANCE,
            0x32 => OpCode::ORIGIN,
            0x33 => OpCode::CALLER,
            0x34 => OpCode::CALLVALUE,
            0x35 => OpCode::CALLDATALOAD,
            0x36 => OpCode::CALLDATASIZE,
            0x37 => OpCode::CALLDATACOPY,
            0x38 => OpCode::CODESIZE,
            0x39 => OpCode::CODECOPY,
            0x3a => OpCode::GASPRICE,
            0x3b => OpCode::EXTCODESIZE,
            0x3c => OpCode::EXTCODECOPY,
            0x3d => OpCode::RETURNDATASIZE,
            0x3e => OpCode::RETURNDATACOPY,
            0x3f => OpCode::EXTCODEHASH,
            0x40 => OpCode::BLOCKHASH,
            0x41 => OpCode::COINBASE,
            0x42 => OpCode::TIMESTAMP,
            0x43 => OpCode::NUMBER,
            0x44 => OpCode::PREVRANDAO,
            0x45 => OpCode::GASLIMIT,
            0x46 => OpCode::CHAINID,
            0x47 => OpCode::SELFBALANCE,
            0x48 => OpCode::BASEFEE,
            0x49 => OpCode::BLOBHASH,
            0x4a => OpCode::BLOBBASEFEE,
            0x50 => OpCode::POP,
            0x51 => OpCode::MLOAD,
            0x52 => OpCode::MSTORE,
            0x53 => OpCode::MSTORE8,
            0x54 => OpCode::SLOAD,
            0x55 => OpCode::SSTORE,
            0x56 => OpCode::JUMP,
            0x57 => OpCode::JUMPI,
            0x58 => OpCode::PC,
            0x59 => OpCode::MSIZE,
            0x5a => OpCode::GAS,
            0x5b => OpCode::JUMPDEST,
            0x60..=0x7f => OpCode::PUSH(opcode - 0x60 + 1),
            0x80..=0x8f => OpCode::DUP(opcode - 0x7f),
            0x90..=0x9f => OpCode::SWAP(opcode - 0x8f),
            0xa0..=0xa4 => OpCode::LOG((opcode - 0xa0) as usize),
            0xf0 => OpCode::CREATE,
            0xf1 => OpCode::CALL,
            0xf2 => OpCode::CALLCODE,
            0xf3 => OpCode::RETURN,
            0xf4 => OpCode::DELEGATECALL,
            0xf5 => OpCode::CREATE2,
            0xfa => OpCode::STATICCALL,
            0xfd => OpCode::REVERT,
            0xfe => OpCode::INVALID,
            0xff => OpCode::SELFDESTRUCT,
            _ => OpCode::INVALID,
        }
    }

    fn analyze_jumpdests(&mut self) {
        for i in 0..self.code.len() {
            if self.code[i] == 0x5b {
                self.jumpdests.insert(i, true);
            }
        }
    }

    fn op_add(&mut self) {
        if let (Some(a), Some(b)) = (self.stack.pop(), self.stack.pop()) {
            self.stack.push(a + b);
        }
    }

    fn op_mul(&mut self) {
        if let (Some(a), Some(b)) = (self.stack.pop(), self.stack.pop()) {
            self.stack.push(a * b);
        }
    }

    fn op_sub(&mut self) {
        if let (Some(a), Some(b)) = (self.stack.pop(), self.stack.pop()) {
            self.stack.push(a - b);
        }
    }

    fn op_div(&mut self) {
        if let (Some(a), Some(b)) = (self.stack.pop(), self.stack.pop()) {
            if b != 0 {
                self.stack.push(a / b);
            }
        }
    }

    fn op_mod(&mut self) {
        if let (Some(a), Some(b)) = (self.stack.pop(), self.stack.pop()) {
            if b != 0 {
                self.stack.push(a % b);
            }
        }
    }

    fn op_push(&mut self, value: u8) {
        self.stack.push(value as u64);
    }

    fn op_pop(&mut self) {
        self.stack.pop();
    }

    fn op_dup(&mut self, n: u8) {
        let index = self.stack.len().saturating_sub(n as usize);
        if index < self.stack.len() {
            let value = self.stack[index];
            self.stack.push(value);
        }
    }

    fn op_swap(&mut self, n: u8) {
        let len = self.stack.len();
        let a = len.saturating_sub(1);
        let b = len.saturating_sub(n as usize + 1);
        if a < len && b < len {
            self.stack.swap(a, b);
        }
    }

    fn op_jump(&mut self) {
        if let Some(position) = self.stack.pop() {
            if self.jumpdests.contains_key(&(position as usize)) {
                self.pc = position as usize;
            }
        }
    }

    fn op_jumpi(&mut self) {
        if let (Some(position), Some(condition)) = (self.stack.pop(), self.stack.pop()) {
            if condition != 0 && self.jumpdests.contains_key(&(position as usize)) {
                self.pc = position as usize;
            }
        }
    }

    fn op_calldataload(&mut self) {
        if let Some(offset) = self.stack.pop() {
            let offset = offset as usize;
            let data = &self.calldata[offset..(offset + 32).min(self.calldata.len())];
            let mut padded = vec![0; 32];
            padded[..data.len()].copy_from_slice(data);

            // Directly access the first 8 bytes as an array
            let first_8_bytes: [u8; 8] = [
                padded[0], padded[1], padded[2], padded[3],
                padded[4], padded[5], padded[6], padded[7],
            ];
            self.stack.push(u64::from_be_bytes(first_8_bytes));
        }
    }

    
    fn op_calldatasize(&mut self) {
        self.stack.push(self.calldata.len() as u64);
    }

    fn op_calldatacopy(&mut self) {
        if let (Some(dest_offset), Some(offset), Some(size)) = (self.stack.pop(), self.stack.pop(), self.stack.pop()) {
            let dest_offset = dest_offset as usize;
            let offset = offset as usize;
            let size = size as usize;

            for i in 0..size {
                if let Some(byte) = self.calldata.get(offset + i) {
                    self.memory[dest_offset + i] = *byte;
                }
            }
        }
    }

    fn op_mload(&mut self) {
        if let Some(offset) = self.stack.pop() {
            let offset = offset as usize;
            let data = &self.memory[offset..(offset + 32).min(self.memory.len())];
            let mut padded = vec![0; 32];
            padded[..data.len()].copy_from_slice(data);

            // Directly access the first 8 bytes as an array
            let first_8_bytes: [u8; 8] = [
                padded[0], padded[1], padded[2], padded[3],
                padded[4], padded[5], padded[6], padded[7],
            ];
            self.stack.push(u64::from_be_bytes(first_8_bytes));
        }
    }

fn op_mstore(&mut self) {
    if let (Some(offset), Some(value)) = (self.stack.pop(), self.stack.pop()) {
        let offset = offset as usize;
        let bytes = value.to_be_bytes();

        // Calculate the end position separately to avoid immutable borrow
        let end = (offset + 8).min(self.memory.len());

        // Perform the memory write operation
        self.memory[offset..end].copy_from_slice(&bytes);
    }
}

    fn op_mstore8(&mut self) {
        if let (Some(offset), Some(value)) = (self.stack.pop(), self.stack.pop()) {
            let offset = offset as usize;
            self.memory[offset] = (value & 0xff) as u8;
        }
    }

    fn op_sload(&mut self, account_id: u64) {
        if let Some(key) = self.stack.pop() {
            // Convert `key` to a Vec<u8> to use as part of the composite key
            let storage_key = key.to_be_bytes().to_vec();

            // Use the composite key (account_id, storage_key) to access storage
            let value = *self.storage.get(&(account_id, storage_key)).unwrap_or(&0);

            self.stack.push(value);
        }
    }

    fn op_sstore(&mut self) {
        if let (Some(key), Some(value)) = (self.stack.pop(), self.stack.pop()) {
            // Convert `key` to Vec<u8> to use as part of the composite key
            let storage_key = key.to_be_bytes().to_vec();

            // Use the composite key (address, storage_key) to store the value
            self.storage.insert((self.address, storage_key), value);
        }
    }
    
fn op_sha3(&mut self) {
        if let (Some(offset), Some(size)) = (self.stack.pop(), self.stack.pop()) {
            let offset = offset as usize;
            let size = size as usize;
            let data = &self.memory[offset..(offset + size).min(self.memory.len())];
            let hash = Keccak256::digest(data);

            // Manually construct the first 8 bytes as an array to avoid `try_into`
            let first_8_bytes: [u8; 8] = [
                hash[0], hash[1], hash[2], hash[3],
                hash[4], hash[5], hash[6], hash[7],
            ];

            self.stack.push(u64::from_be_bytes(first_8_bytes));
        }
}
    
    fn op_or(&mut self) {
        if let (Some(a), Some(b)) = (self.stack.pop(), self.stack.pop()) {
            self.stack.push(a | b);
        }
    }

    fn op_and(&mut self) {
        if let (Some(a), Some(b)) = (self.stack.pop(), self.stack.pop()) {
            self.stack.push(a & b);
        }
    }

    fn op_not(&mut self) {
        if let Some(a) = self.stack.pop() {
            self.stack.push(!a);
        }
    }
    
    // Signed division (SDIV) operation
    fn op_sdiv(&mut self) {
        if let (Some(a), Some(b)) = (self.stack.pop(), self.stack.pop()) {
            if b != 0 {
                let result = (a as i64).wrapping_div(b as i64);
                self.stack.push(result as u64);
            } else {
                self.stack.push(0); // Per EVM spec, division by zero returns zero
            }
        }
    }

    // Bitwise XOR operation
    fn op_xor(&mut self) {
        if let (Some(a), Some(b)) = (self.stack.pop(), self.stack.pop()) {
            self.stack.push(a ^ b);
        }
    }
    
    
    fn op_smod(&mut self) {
        if let (Some(a), Some(b)) = (self.stack.pop(), self.stack.pop()) {
            if b != 0 {
                let result = (a as i64 % b as i64).abs() as u64;
                self.stack.push(result);
            }
        }
    }

    fn op_addmod(&mut self) {
        if let (Some(a), Some(b), Some(c)) = (self.stack.pop(), self.stack.pop(), self.stack.pop()) {
            if c != 0 {
                let result = (a + b) % c;
                self.stack.push(result);
            }
        }
    }

    fn op_mulmod(&mut self) {
        if let (Some(a), Some(b), Some(c)) = (self.stack.pop(), self.stack.pop(), self.stack.pop()) {
            if c != 0 {
                let result = (a * b) % c;
                self.stack.push(result);
            }
        }
    }

    fn op_exp(&mut self) {
        if let (Some(base), Some(exp)) = (self.stack.pop(), self.stack.pop()) {
            let result = base.pow(exp as u32);
            self.stack.push(result);
        }
    }

    fn op_signextend(&mut self) {
        if let (Some(byte_pos), Some(value)) = (self.stack.pop(), self.stack.pop()) {
            let byte_pos = byte_pos as u8;
            let sign_bit = 1 << (8 * byte_pos + 7);
            let mask = !0 >> (8 * byte_pos);
            let result = if (value & sign_bit) != 0 {
                value | mask
            } else {
                value & !mask
            };
            self.stack.push(result);
        }
    }

    fn op_lt(&mut self) {
        if let (Some(a), Some(b)) = (self.stack.pop(), self.stack.pop()) {
            self.stack.push((a < b) as u64);
        }
    }

    fn op_gt(&mut self) {
        if let (Some(a), Some(b)) = (self.stack.pop(), self.stack.pop()) {
            self.stack.push((a > b) as u64);
        }
    }

    fn op_slt(&mut self) {
        if let (Some(a), Some(b)) = (self.stack.pop(), self.stack.pop()) {
            self.stack.push(((a as i64) < (b as i64)) as u64);
        }
    }

    fn op_sgt(&mut self) {
        if let (Some(a), Some(b)) = (self.stack.pop(), self.stack.pop()) {
            self.stack.push(((a as i64) > (b as i64)) as u64);
        }
    }

    fn op_eq(&mut self) {
        if let (Some(a), Some(b)) = (self.stack.pop(), self.stack.pop()) {
            self.stack.push((a == b) as u64);
        }
    }

    fn op_iszero(&mut self) {
        if let Some(a) = self.stack.pop() {
            self.stack.push((a == 0) as u64);
        }
    }

    fn op_byte(&mut self) {
        if let (Some(pos), Some(value)) = (self.stack.pop(), self.stack.pop()) {
            let byte = if pos < 32 {
                (value >> (8 * (31 - pos))) & 0xFF
            } else {
                0
            };
            self.stack.push(byte);
        }
    }

    fn op_shl(&mut self) {
        if let (Some(shift), Some(value)) = (self.stack.pop(), self.stack.pop()) {
            self.stack.push(value << shift);
        }
    }

    fn op_shr(&mut self) {
        if let (Some(shift), Some(value)) = (self.stack.pop(), self.stack.pop()) {
            self.stack.push(value >> shift);
        }
    }

    fn op_sar(&mut self) {
        if let (Some(shift), Some(value)) = (self.stack.pop(), self.stack.pop()) {
            let result = (value as i64 >> shift) as u64;
            self.stack.push(result);
        }
    }

    
    fn op_keccak256(&mut self) {
        if let (Some(offset), Some(size)) = (self.stack.pop(), self.stack.pop()) {
            let offset = offset as usize;
            let size = size as usize;
            if offset + size <= self.memory.len() {
                let data = &self.memory[offset..offset + size];
                let hash = Keccak256::digest(data);

                // Manually construct the first 8 bytes as an array to avoid `try_into`
                let first_8_bytes: [u8; 8] = [
                    hash[0], hash[1], hash[2], hash[3],
                    hash[4], hash[5], hash[6], hash[7],
                ];

                self.stack.push(u64::from_be_bytes(first_8_bytes));
            }
        }
    }
    
    fn op_address(&mut self) {
        self.stack.push(self.address);
    }

    // BALANCE opcode: Pushes the balance of a given address onto the stack
    fn op_balance(&mut self) {
        if let Some(address) = self.stack.pop() {
            let balance = *self.balances.get(&address).unwrap_or(&0);
            self.stack.push(balance);
        }
    }    
    
    fn op_origin(&mut self) {
        self.stack.push(self.origin);
    }

    fn op_caller(&mut self) {
        self.stack.push(self.caller);
    }

    fn op_callvalue(&mut self) {
        self.stack.push(self.callvalue);
    }

    fn op_codesize(&mut self) {
        self.stack.push(self.code.len() as u64);
    }

    fn op_codecopy(&mut self) {
        if let (Some(dest_offset), Some(offset), Some(size)) = (self.stack.pop(), self.stack.pop(), self.stack.pop()) {
            let dest_offset = dest_offset as usize;
            let offset = offset as usize;
            let size = size as usize;
            for i in 0..size {
                if let Some(byte) = self.code.get(offset + i) {
                    self.memory[dest_offset + i] = *byte;
                }
            }
        }
    }

    fn op_gasprice(&mut self) {
        self.stack.push(self.gasprice);
    }
    
    fn op_extcodesize(&mut self) {
        if let Some(address) = self.stack.pop() {
            let code_size = self.external_code_size(address); // Simulated external call
            self.stack.push(code_size);
        }
    }

    fn op_extcodecopy(&mut self) {
        if let (Some(address), Some(dest_offset), Some(offset), Some(size)) =
            (self.stack.pop(), self.stack.pop(), self.stack.pop(), self.stack.pop())
        {
            let code = self.external_code(address); // Simulated external code retrieval
            let offset = offset as usize;
            let size = size as usize;
            let dest_offset = dest_offset as usize;

            for i in 0..size {
                if offset + i < code.len() && dest_offset + i < self.memory.len() {
                    self.memory[dest_offset + i] = code[offset + i];
                }
            }
        }
    }
    
    fn op_returndatasize(&mut self) {
        self.stack.push(self.returndata.len() as u64);
    }

    fn op_returndatacopy(&mut self) {
        if let (Some(dest_offset), Some(offset), Some(size)) = (self.stack.pop(), self.stack.pop(), self.stack.pop()) {
            let offset = offset as usize;
            let size = size as usize;
            let dest_offset = dest_offset as usize;

            for i in 0..size {
                if offset + i < self.returndata.len() && dest_offset + i < self.memory.len() {
                    self.memory[dest_offset + i] = self.returndata[offset + i];
                }
            }
        }
    }


    fn op_extcodehash(&mut self) {
        if let Some(address) = self.stack.pop() {
            let code = self.external_code(address); // Simulated external code retrieval
            let hash = Keccak256::digest(&code);

            // Manually construct the first 8 bytes as an array to avoid `try_into`
            let first_8_bytes: [u8; 8] = [
                hash[0], hash[1], hash[2], hash[3],
                hash[4], hash[5], hash[6], hash[7],
            ];
            
            let hash_value = u64::from_be_bytes(first_8_bytes);
            self.stack.push(hash_value);
        }
    }
    
    fn op_blockhash(&mut self) {
        if let Some(block_number) = self.stack.pop() {
            if block_number < self.block_number && self.block_number - block_number <= 256 {
                self.stack.push(self.blockhash);
            } else {
                self.stack.push(0); // Invalid block range
            }
        }
    }

    fn op_coinbase(&mut self) {
        self.stack.push(self.coinbase);
    }

    fn op_timestamp(&mut self) {
        self.stack.push(self.timestamp);
    }
    
    fn op_number(&mut self) {
        self.stack.push(self.block_number);
    }

    fn op_prevrandao(&mut self) {
        self.stack.push(self.prevrandao);
    }

    fn op_gaslimit(&mut self) {
        self.stack.push(self.gaslimit);
    }

    fn op_chainid(&mut self) {
        self.stack.push(self.chainid);
    }

    fn op_selfbalance(&mut self) {
        self.stack.push(self.balance);
    }

    fn op_basefee(&mut self) {
        self.stack.push(self.basefee);
    }
    
    fn op_blobhash(&mut self) {
        // EIP-4844 placeholder, not yet functional
        self.stack.push(0);
    }

    fn op_blobbasefee(&mut self) {
        // EIP-4844 placeholder, not yet functional
        self.stack.push(0);
    }

    // Helper function: Simulates retrieval of code size for an external address
    fn external_code_size(&self, address: u64) -> u64 {
        // In an actual implementation, this would query the blockchain state
        println!("Retrieving code size for address: {}", address);
        42 // Placeholder value
    }

    // Helper function: Simulates retrieval of code for an external address
    fn external_code(&self, address: u64) -> Vec<u8> {
        // In an actual implementation, this would return the code bytes
        println!("Retrieving code for address: {}", address);
        vec![0x60, 0x60, 0x60, 0x60] // Placeholder bytecode
    }
    
    fn op_pc(&mut self) {
        self.stack.push(self.pc as u64);
    }

    fn op_msize(&mut self) {
        self.stack.push(self.memory.len() as u64);
    }

    
    fn op_gas(&mut self) {
        // Push remaining gas onto the stack
        self.stack.push(self.gas_remaining);
    }

    fn op_tload(&mut self) {
        if let Some(key) = self.stack.pop() {
            // Convert `key` to bytes for the composite storage key
            let key_as_bytes = key.to_be_bytes().to_vec();

            // Use the composite key (current_account_id, key_as_bytes) to access storage
            let value = *self.storage.get(&(self.address, key_as_bytes)).unwrap_or(&0);

            // Push the retrieved value onto the stack
            self.stack.push(value);
        }
    }

    fn op_tstore(&mut self) {
        if let (Some(key), Some(value)) = (self.stack.pop(), self.stack.pop()) {
            // Convert `key` to bytes to match the composite storage key format
            let key_as_bytes = key.to_be_bytes().to_vec();

            // Use the composite key (current_account_id, key_as_bytes) to store the value
            self.storage.insert((self.address, key_as_bytes), value);
        }
    }
    
fn op_mcopy(&mut self) {
    if let (Some(dest), Some(src), Some(length)) = (self.stack.pop(), self.stack.pop(), self.stack.pop()) {
        let dest = dest as usize;
        let src = src as usize;
        let length = length as usize;

        // Ensure the copy boundaries are within the memory bounds
        if src + length <= self.memory.len() && dest + length <= self.memory.len() {
            // Create a temporary copy of the source slice to avoid borrow conflicts
            let src_data: Vec<u8> = self.memory[src..src + length].to_vec();
            self.memory[dest..dest + length].copy_from_slice(&src_data);
        }
    }
}

    
    fn op_log(&mut self, num_topics: usize) {
        if let Some(data_offset) = self.stack.pop() {
            if let Some(data_size) = self.stack.pop() {
                let mut topics = Vec::with_capacity(num_topics);
                for _ in 0..num_topics {
                    if let Some(topic) = self.stack.pop() {
                        topics.push(topic);
                    }
                }

                let offset = data_offset as usize;
                let size = data_size as usize;
                let data = if offset + size <= self.memory.len() {
                    self.memory[offset..offset + size].to_vec()
                } else {
                    vec![]
                };
                
                self.logs.push((self.address, data, topics));
            }
        }
    }
    

    fn op_create(&mut self) {
        if let (Some(value), Some(offset), Some(size)) = (self.stack.pop(), self.stack.pop(), self.stack.pop()) {
            // Extract contract creation code from memory
            let code = self.memory[offset as usize..(offset as usize + size as usize)].to_vec();

            // Generate a new address for the contract
            let new_address = self.next_address;
            self.next_address += 1;

            // Calculate code hash
            let code_hash = Keccak256::digest(&code).to_vec();

            // Create a new contract transaction with the generated address
            let contract_tx = Transaction {
                from: new_address,
                to: new_address,
                value,
                gas: 0,    // Placeholder gas, can be adjusted
                data: code.clone(), // Contract initialization code
            };

            // Add new account for the contract with initial balance, code hash, and an empty storage root
            self.accounts.insert(
                new_address,
                Account {
                    balance: value,
                    nonce: 0,
                    storage_root: vec![], // Initialize storage root as empty for now
                    code_hash,
                    destroyed: false,
                },
            );

            println!("Creating contract at address {} with balance {}", new_address, value);
            self.execute_contract(contract_tx); // Execute the contract code
            self.stack.push(new_address);       // Push new contract address onto the stack
        }
    }

    fn op_call(&mut self) {
    if let (Some(gas), Some(addr), Some(value), Some(in_offset), Some(in_size), Some(_out_offset), Some(_out_size)) =
        (self.stack.pop(), self.stack.pop(), self.stack.pop(), self.stack.pop(), self.stack.pop(), self.stack.pop(), self.stack.pop())
    {
        // Prepare data for the call
        let data = self.memory[in_offset as usize..(in_offset as usize + in_size as usize)].to_vec();

        // Clone data when creating the transaction to retain it for `println!`
        let call_tx = Transaction {
            from: addr,
            to: addr,
            value,
            gas,
            data: data.clone(), // Clone here to avoid moving `data`
        };

        // Update caller's balance and increment nonce
        if let Some(caller_account) = self.accounts.get_mut(&addr) {
            if caller_account.balance >= value {
                caller_account.balance -= value;
                caller_account.nonce += 1;
            } else {
                println!("CALL failed: insufficient balance for account {}", addr);
                self.stack.push(0); // Push failure indicator onto the stack
                return;
            }
        }

        println!(
            "Calling contract at address {} with gas: {}, value: {}, data: {:?}",
            addr, gas, value, data
        );

        // Execute the call and push success indicator
        self.execute_contract(call_tx);
        self.stack.push(1); // Push success indicator onto the stack
    }
}

// CALLCODE opcode: similar to CALL, but code executes in the caller's context
fn op_callcode(&mut self) {
    if let (Some(gas), Some(addr), Some(value), Some(in_offset), Some(in_size), Some(_out_offset), Some(_out_size)) =
        (self.stack.pop(), self.stack.pop(), self.stack.pop(), self.stack.pop(), self.stack.pop(), self.stack.pop(), self.stack.pop())
    {
        // Prepare data for the call
        let data = self.memory[in_offset as usize..(in_offset as usize + in_size as usize)].to_vec();

        // Create a "callcode" transaction, executing in the caller's context
        let callcode_tx = Transaction {
            from: addr,
            to: addr,
            value,
            gas,
            data, // `data` is moved here, but `data_ref` still holds a reference to it for `println!`
        };

        // Update caller's balance and increment nonce
        if let Some(caller_account) = self.accounts.get_mut(&addr) {
            if caller_account.balance >= value {
                caller_account.balance -= value;
                caller_account.nonce += 1;
            } else {
                println!("CALLCODE failed: insufficient balance for account {}", addr);
                self.stack.push(0); // Push failure indicator onto the stack
                return;
            }
        }

        // Execute the callcode in the caller’s context and push success indicator
        self.execute_contract(callcode_tx);
        self.stack.push(1); // Push success indicator onto the stack
    }
}

    
    // RETURN opcode: Stops execution and returns data from memory
    fn op_return(&mut self) {
        if let (Some(offset), Some(size)) = (self.stack.pop(), self.stack.pop()) {
            let offset = offset as usize;
            let size = size as usize;
            if offset + size <= self.memory.len() {
                self.returndata = self.memory[offset..offset + size].to_vec();
                println!("Returning data: {:?}", self.returndata);
            } else {
                println!("RETURN failed: invalid memory access");
            }
            self.pc = self.code.len(); // Stop execution by setting pc to end of code
        }
    }


    // DELEGATECALL opcode: Calls a contract while retaining caller’s context (no transfer of ownership)
// DELEGATECALL opcode: Calls a contract while retaining caller’s context (no transfer of ownership)
fn op_delegatecall(&mut self) {
    if let (Some(gas), Some(addr), Some(in_offset), Some(in_size), Some(_out_offset), Some(_out_size)) =
        (self.stack.pop(), self.stack.pop(), self.stack.pop(), self.stack.pop(), self.stack.pop(), self.stack.pop())
    {
        // Prepare data for the call
        let data = self.memory[in_offset as usize..(in_offset as usize + in_size as usize)].to_vec();

        // Create a "delegatecall" transaction, executing in the caller's context
        let delegate_tx = Transaction {
            from: addr,
            to: addr,
            value: 0, // Delegatecall does not transfer value
            gas,
            data, // Move `data` here
        };

        // Execute the delegatecall in the caller’s context and push success indicator
        self.execute_contract(delegate_tx);
        self.stack.push(1); // Push success indicator onto the stack
    }
}


    // CREATE2 opcode: Creates a contract with a deterministic address
    fn op_create2(&mut self) {
        if let (Some(value), Some(offset), Some(size), Some(salt)) = (self.stack.pop(), self.stack.pop(), self.stack.pop(), self.stack.pop()) {
            // Extract contract creation code from memory
            let code = self.memory[offset as usize..(offset as usize + size as usize)].to_vec();

            // Calculate the contract address based on CREATE2 specifications
            let mut hasher = Keccak256::new();
            hasher.update([0xff]); // Initial CREATE2 prefix byte
            hasher.update(self.next_address.to_be_bytes()); // Address of the creator
            hasher.update(salt.to_be_bytes()); // Salt
            hasher.update(Keccak256::digest(&code)); // Keccak256 hash of init code
            let hash = hasher.finalize();

            // Construct the new address from the first 8 bytes of the hash
            let first_8_bytes: [u8; 8] = [
                hash[0], hash[1], hash[2], hash[3],
                hash[4], hash[5], hash[6], hash[7],
            ];
            let new_address = u64::from_be_bytes(first_8_bytes);

            // Calculate code hash
            let code_hash = Keccak256::digest(&code).to_vec();

            // Prepare a contract creation transaction for execution
            let create2_tx = Transaction {
                from: new_address,
                to: new_address,
                value,
                gas: 0, // Placeholder gas
                data: code,
            };

            println!(
                "CREATE2 with value: {}, salt: {}, code: {:?}, new address: {}",
                value, salt, create2_tx.data, new_address
            );

            // Update accounts with initial balance, nonce, code_hash, and an empty storage root
            self.accounts.insert(
                new_address,
                Account {
                    balance: value,
                    nonce: 0,
                    storage_root: vec![], // Initialize storage root as empty
                    code_hash,
                    destroyed: false,
                },
            );

            self.execute_contract(create2_tx); // Execute the contract initialization code
            self.stack.push(new_address);      // Push new contract address onto the stack
        }
    }
    

    // STATICCALL opcode: Executes a read-only contract call with no state changes
    fn op_staticcall(&mut self) {
        if let (Some(gas), Some(addr), Some(in_offset), Some(in_size), Some(_out_offset), Some(_out_size)) =
            (self.stack.pop(), self.stack.pop(), self.stack.pop(), self.stack.pop(), self.stack.pop(), self.stack.pop())
        {
            // Extract call data from memory based on in_offset and in_size
            let data = self.memory[in_offset as usize..(in_offset as usize + in_size as usize)].to_vec();

            // Create a "staticcall" transaction with zero value and no state changes
            let static_tx = Transaction {
                from: addr,
                to: addr,
                value: 0, // Static calls cannot transfer value
                gas,
                data,
            };

            // Execute the static call in a read-only context
            self.execute_contract(static_tx);
            self.stack.push(1); // Push success indicator onto the stack
        }
    }

    // REVERT opcode: Stops execution and reverts the transaction, optionally with return data
    fn op_revert(&mut self) {
        if let (Some(offset), Some(size)) = (self.stack.pop(), self.stack.pop()) {
            let offset = offset as usize;
            let size = size as usize;

            if offset + size <= self.memory.len() {
                self.returndata = self.memory[offset..offset + size].to_vec();
                println!("Transaction reverted with data: {:?}", self.returndata);
            } else {
                println!("REVERT failed: invalid memory access");
            }

            // Set program counter to end of code to stop execution and revert state
            self.pc = self.code.len();
        }
    }

// SELFDESTRUCT opcode: Destroys the contract and transfers its balance to a beneficiary
fn op_selfdestruct(&mut self) {
    if let Some(beneficiary) = self.stack.pop() {
        // Get the address of the current contract (assuming it’s stored in the last account created)
        let contract_address = self.balances.keys().last().copied().unwrap_or(0);

        // Step 1: Retrieve the contract's balance and mark it as destroyed
        let contract_balance;
        {
            if let Some(contract_account) = self.accounts.get_mut(&contract_address) {
                contract_balance = contract_account.balance;
                contract_account.balance = 0;
                contract_account.destroyed = true;
            } else {
                // If the contract account doesn't exist, exit early
                println!("Selfdestruct failed: contract account {} does not exist.", contract_address);
                return;
            }
        }

        // Step 2: Transfer the balance to the beneficiary account
        let beneficiary_account = self.accounts.entry(beneficiary).or_insert(Account {
            balance: 0,
            nonce: 0,
            storage_root: vec![], // Default storage root for new accounts
            code_hash: vec![],    // Default empty code hash for new accounts
            destroyed: false,
        });
        beneficiary_account.balance += contract_balance;
    }
}

    
}

impl Block {
    // Function to parse a block's raw bytes into the Block structure
    fn import_block(raw_bytes: &[u8]) -> Result<Block, Box<dyn Error>> {
        let mut offset = 0;

        // Parse block header
        let block_number = u32::from_be_bytes([
            raw_bytes[offset], raw_bytes[offset + 1], raw_bytes[offset + 2], raw_bytes[offset + 3],
        ]);
        offset += 4;

        let timestamp = u64::from_be_bytes([
            raw_bytes[offset], raw_bytes[offset + 1], raw_bytes[offset + 2], raw_bytes[offset + 3],
            raw_bytes[offset + 4], raw_bytes[offset + 5], raw_bytes[offset + 6], raw_bytes[offset + 7],
        ]);
        offset += 8;

        // Parse transaction count
        let transaction_count = u32::from_be_bytes([
            raw_bytes[offset], raw_bytes[offset + 1], raw_bytes[offset + 2], raw_bytes[offset + 3],
        ]);
        offset += 4;

        let mut transactions = Vec::with_capacity(transaction_count as usize);

        for _ in 0..transaction_count {
            // Parse each transaction
            let from = u64::from_be_bytes([
                raw_bytes[offset], raw_bytes[offset + 1], raw_bytes[offset + 2], raw_bytes[offset + 3],
                raw_bytes[offset + 4], raw_bytes[offset + 5], raw_bytes[offset + 6], raw_bytes[offset + 7],
            ]);
            offset += 8;

            let to = u64::from_be_bytes([
                raw_bytes[offset], raw_bytes[offset + 1], raw_bytes[offset + 2], raw_bytes[offset + 3],
                raw_bytes[offset + 4], raw_bytes[offset + 5], raw_bytes[offset + 6], raw_bytes[offset + 7],
            ]);
            offset += 8;

            let value = u64::from_be_bytes([
                raw_bytes[offset], raw_bytes[offset + 1], raw_bytes[offset + 2], raw_bytes[offset + 3],
                raw_bytes[offset + 4], raw_bytes[offset + 5], raw_bytes[offset + 6], raw_bytes[offset + 7],
            ]);
            offset += 8;

            let gas = u32::from_be_bytes([
                raw_bytes[offset], raw_bytes[offset + 1], raw_bytes[offset + 2], raw_bytes[offset + 3],
            ]);
            offset += 4;

            // Parse data length and data
            let data_length = u32::from_be_bytes([
                raw_bytes[offset], raw_bytes[offset + 1], raw_bytes[offset + 2], raw_bytes[offset + 3],
            ]);
            offset += 4;

            let data = raw_bytes[offset..offset + data_length as usize].to_vec();
            offset += data_length as usize;

            // Add transaction to the list
            transactions.push(Transaction { from, to, value, gas: gas.into(), data });

        }

        Ok(Block {
            block_number,
            timestamp,
            transactions,
        })
    }
}



impl PatriciaTrie {
    // Verifies a proof for a given key against the trie root
    fn verify_proof(&self, key: &[u8], expected_value: &[u8]) -> bool {
        let mut current_node_hash = self.root.clone();
        let mut path = key_to_nibbles(key);

        while !path.is_empty() {
            // Retrieve current node data from hash
            let current_node_data = match self.nodes.get(&current_node_hash) {
                Some(data) => data.clone(),
                None => return false,
            };

            // Decode the node
            let rlp_node = Rlp::new(&current_node_data);
            if rlp_node.item_count().unwrap() == 2 {
                // Leaf node: check if it matches the expected value
                let stored_key = rlp_node.at(0).unwrap().data().unwrap();
                let stored_value = rlp_node.at(1).unwrap().data().unwrap();
                return stored_key == path && stored_value == expected_value;
            } else {
                // Branch or extension node
                let (next_node_hash, remaining_path) = self.process_node(&rlp_node, &path);
                current_node_hash = next_node_hash;
                path = remaining_path;
            }
        }

        false
    }

    fn process_node(&self, node: &Rlp, path: &[u8]) -> (Vec<u8>, Vec<u8>) {
        // Determine if node is branch or extension and process accordingly
        if node.item_count().unwrap() == 17 {
            // Branch node
            let index = path[0] as usize;
            let next_node_hash = node.at(index).unwrap().data().unwrap().to_vec();
            let remaining_path = path[1..].to_vec();
            (next_node_hash, remaining_path)
        } else {
            // Extension node
            let partial_path = node.at(0).unwrap().data().unwrap();
            let next_node_hash = node.at(1).unwrap().data().unwrap().to_vec();
            let remaining_path = path[partial_path.len()..].to_vec();
            (next_node_hash, remaining_path)
        }
    }
}


fn keccak256(data: &[u8]) -> Vec<u8> {
    let mut hasher = Keccak::v256();
    let mut output = [0u8; 32];
    hasher.update(data);
    hasher.finalize(&mut output);
    output.to_vec()
}

fn key_to_nibbles(key: &[u8]) -> Vec<u8> {
    let mut nibbles = Vec::new();
    for byte in key {
        nibbles.push(byte >> 4);
        nibbles.push(byte & 0x0F);
    }
    nibbles
}


fn verify_state(
    _state_root: Vec<u8>,
    account_witnesses: Vec<(u64, Account)>,
    storage_witnesses: Vec<(u64, Vec<(Vec<u8>, u64)>)>,
    trie: &PatriciaTrie,
    balances: &mut HashMap<u64, u64>,
    accounts: &mut HashMap<u64, Account>,
    storage: &mut HashMap<(u64, Vec<u8>), u64>,
) -> bool {
    for (address, account) in account_witnesses.iter() {
        let mut account_rlp = RlpStream::new_list(4);
        account_rlp.append(&account.nonce);
        account_rlp.append(&account.balance);
        account_rlp.append(&account.storage_root);
        account_rlp.append(&account.code_hash);
        let account_encoded = account_rlp.out().to_vec();

        let account_key = keccak256(&address.to_be_bytes());
        if !trie.verify_proof(&account_key, &account_encoded) {
            println!("Account witness verification failed for address {}", address);
            return false;
        }

        // Populate accounts and balances
        accounts.insert(*address, account.clone());
        balances.insert(*address, account.balance);
    }

    for (account_address, key_value_pairs) in storage_witnesses.iter() {
        let account_storage_root = accounts.get(account_address).unwrap().storage_root.clone();
        let storage_trie = PatriciaTrie {
            root: account_storage_root,
            nodes: trie.nodes.clone(),
        };

        for (key, value) in key_value_pairs {
            // Convert `value` from u64 to a byte array and create an RLP encoding
            let value_bytes = value.to_be_bytes();
            let encoded_value = Rlp::new(&value_bytes).data().unwrap().to_vec();
            
            if !storage_trie.verify_proof(key, &encoded_value) {
                println!("Storage witness verification failed for key {:?}", key);
                return false;
            }
            storage.insert((*account_address, key.clone()), *value);
        }
    }

    true
}

// Verifies that the updated accounts and storage values in account_changes and storage_changes match the new block’s state root.
fn check_state_witnesses_against_new_state(
    _new_state_root: Vec<u8>,
    trie: &PatriciaTrie,
    account_changes: &HashMap<u64, Account>,
    storage_changes: &HashMap<(u64, Vec<u8>), u64>,
) -> bool {
    for (address, account) in account_changes {
        let account_key = keccak256(&address.to_be_bytes());
        let mut account_rlp = RlpStream::new_list(4);
        account_rlp.append(&account.nonce);
        account_rlp.append(&account.balance);
        account_rlp.append(&account.storage_root);
        account_rlp.append(&account.code_hash);
        let account_encoded = account_rlp.out().to_vec();

        if !trie.verify_proof(&account_key, &account_encoded) {
            println!("New account witness verification failed for address {}", address);
            return false;
        }
    }

    for ((account_address, key), value) in storage_changes {
        let account_storage_root = account_changes.get(account_address).unwrap().storage_root.clone();
        let storage_trie = PatriciaTrie {
            root: account_storage_root,
            nodes: trie.nodes.clone(),
        };

        // Convert `value` to bytes and create an RLP encoding from it
        let value_bytes = value.to_be_bytes();
        let encoded_value = Rlp::new(&value_bytes).data().unwrap().to_vec();

        if !storage_trie.verify_proof(key, &encoded_value) {
            println!("New storage witness verification failed for key {:?}", key);
            return false;
        }
    }

    true
}



#[cfg(test)]
mod tests {
    use super::*;
    use std::collections::HashMap;

    #[test]
    fn test_execute_block() {
        // Sample accounts
        let mut accounts = HashMap::new();
        accounts.insert(
            1,
            Account {
                balance: 1000,
                nonce: 0,
                storage_root: vec![],
                code_hash: vec![],
            },
        );
        accounts.insert(
            2,
            Account {
                balance: 500,
                nonce: 0,
                storage_root: vec![],
                code_hash: vec![],
            },
        );

        // Sample block with transactions
        let block = Block {
            transactions: vec![
                Transaction {
                    from: 1,
                    to: 2,
                    value: 100,
                    gas: 21000,
                    data: vec![],
                }, // Native transfer
                Transaction {
                    from: 1,
                    to: 3,
                    value: 50,
                    gas: 80000,
                    data: vec![0x01, 0x02, 0x31, 0xff],
                }, // Contract call with opcodes
            ],
            state_root: vec![], // Placeholder state root for this example
        };

        // Initialize EVM and execute block
        let mut evm = EVM {
            accounts: HashMap::new(),
            balances: HashMap::new(),
            storage: HashMap::new(),
        };
        evm.initialize_state(accounts.clone(), HashMap::new(), HashMap::new());

        let (changed_accounts, changed_balances, _changed_storage) = evm.execute_block(block);

        // Expected outcomes after execution
        assert_eq!(evm.balances[&1], 850); // Account 1's balance after transferring 100 and 50
        assert_eq!(evm.balances[&2], 600); // Account 2's balance after receiving 100
        assert_eq!(evm.balances.get(&3).unwrap_or(&0), &50); // Account 3's balance after receiving 50

        assert_eq!(changed_accounts.len(), 3); // Three accounts should be changed
        assert_eq!(changed_balances.len(), 3); // Three balances should be changed
    }

    #[test]
    fn test_verify_state() {
        // Assume `state_root` is provided from the block header
        let state_root = vec![0u8; 32]; // Placeholder for the state root hash

        // Account setup
        let account = Account {
            balance: 1000,
            nonce: 1,
            storage_root: keccak256(b"example_storage_root"),
            code_hash: keccak256(b"example_code"),
        };

        // Example Patricia Trie with nodes for the account and storage
        let mut nodes = HashMap::new();
        // Populate `nodes` with RLP-encoded trie nodes (omitted for brevity)
        let trie = PatriciaTrie {
            root: state_root.clone(),
            nodes,
        };

        // Example storage proof entries for verification
        let storage_proofs = vec![
            (keccak256(b"storage_key_1"), keccak256(b"storage_value_1")),
            (keccak256(b"storage_key_2"), keccak256(b"storage_value_2")),
        ];

        // Verify the account and storage values against the state root
        let account_key = keccak256(b"account_address");

        let mut balances = HashMap::new();
        let mut accounts = HashMap::new();
        let mut storage = HashMap::new();
        let account_witnesses = vec![(account_key.clone(), account.clone())];
        let storage_witnesses = vec![(account_key.clone(), storage_proofs.clone())];

        let result = verify_state(
            state_root,
            account_witnesses,
            storage_witnesses,
            &trie,
            &mut balances,
            &mut accounts,
            &mut storage,
        );

        assert!(result, "Account and storage verification failed.");
        assert_eq!(accounts.get(&account_key), Some(&account));
    }
}



#[cfg(test)]
mod tests {
    use super::*;
    use std::collections::HashMap;

    #[test]
    fn test_import_block() {
        // Sample raw bytes for a block
        let raw_block_data: Vec<u8> = vec![
            // Block Header
            0, 0, 0, 1,               // block_number = 1
            0, 0, 0, 0, 0, 0, 0, 10,  // timestamp = 10
            0, 0, 0, 2,               // transaction_count = 2
            // Transaction 1
            0, 0, 0, 0, 0, 0, 0, 1,   // from = 1
            0, 0, 0, 0, 0, 0, 0, 2,   // to = 2
            0, 0, 0, 0, 0, 0, 0, 50,  // value = 50
            0, 0, 10, 0,              // gas = 2560
            0, 0, 0, 3,               // data_length = 3
            0x60, 0x61, 0x62,         // data = [0x60, 0x61, 0x62]
            // Transaction 2
            0, 0, 0, 0, 0, 0, 0, 3,   // from = 3
            0, 0, 0, 0, 0, 0, 0, 4,   // to = 4
            0, 0, 0, 0, 0, 0, 0, 100, // value = 100
            0, 0, 20, 0,              // gas = 5120
            0, 0, 0, 0,               // data_length = 0
        ];

        // Expected Block structure after parsing
        let expected_block = Block {
            block_number: 1,
            timestamp: 10,
            transactions: vec![
                Transaction {
                    from: 1,
                    to: 2,
                    value: 50,
                    gas: 2560,
                    data: vec![0x60, 0x61, 0x62],
                },
                Transaction {
                    from: 3,
                    to: 4,
                    value: 100,
                    gas: 5120,
                    data: vec![],
                },
            ],
        };

        // Parse the raw bytes into a Block
        let parsed_block = Block::import_block(&raw_block_data).expect("Failed to parse block");

        // Assert the parsed block matches the expected structure
        assert_eq!(parsed_block.block_number, expected_block.block_number);
        assert_eq!(parsed_block.timestamp, expected_block.timestamp);
        assert_eq!(parsed_block.transactions.len(), expected_block.transactions.len());

        // Assert each transaction matches the expected transaction data
        for (parsed_tx, expected_tx) in parsed_block.transactions.iter().zip(expected_block.transactions.iter()) {
            assert_eq!(parsed_tx.from, expected_tx.from);
            assert_eq!(parsed_tx.to, expected_tx.to);
            assert_eq!(parsed_tx.value, expected_tx.value);
            assert_eq!(parsed_tx.gas, expected_tx.gas);
            assert_eq!(parsed_tx.data, expected_tx.data);
        }
    }
}

