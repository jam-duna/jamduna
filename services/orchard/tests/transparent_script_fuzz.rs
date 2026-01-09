use orchard_service::transparent_script::{
    execute_script, execute_script_with_context, ScriptContext, ScriptStack,
};

fn next_u64(seed: &mut u64) -> u64 {
    *seed = seed.wrapping_mul(6364136223846793005).wrapping_add(1);
    *seed
}

fn next_byte(seed: &mut u64) -> u8 {
    (next_u64(seed) >> 32) as u8
}

#[test]
fn test_script_fuzz_does_not_panic() {
    let mut seed = 0x4d595df4d0f33173u64;

    for _ in 0..256 {
        let mut stack = ScriptStack::new();
        let item_count = (next_byte(&mut seed) % 8) as usize;
        for _ in 0..item_count {
            let len = (next_byte(&mut seed) % 16) as usize;
            let mut data = Vec::with_capacity(len);
            for _ in 0..len {
                data.push(next_byte(&mut seed));
            }
            let _ = stack.push(&data);
        }

        let script_len = (next_byte(&mut seed) % 128) as usize;
        let mut script = Vec::with_capacity(script_len);
        for _ in 0..script_len {
            script.push(next_byte(&mut seed));
        }

        let _ = execute_script(&script, &mut stack, &[0u8; 32]);
    }
}

#[test]
fn test_checklocktimeverify_requires_context() {
    let mut stack = ScriptStack::new();
    stack.push(&[0x01]).expect("push locktime");
    let script = [0xb1u8]; // OP_CHECKLOCKTIMEVERIFY
    let err = execute_script_with_context(&script, &mut stack, &[0u8; 32], None)
        .expect_err("missing context should fail");
    assert!(err.to_string().contains("missing script context"));
}

#[test]
fn test_checklocktimeverify_context_ok() {
    let mut stack = ScriptStack::new();
    stack.push(&[0x01]).expect("push locktime");
    let script = [0xb1u8]; // OP_CHECKLOCKTIMEVERIFY
    let ctx = ScriptContext {
        lock_time: 1,
        sequence: 0xFFFF_FFFE,
        tx_version: 5,
    };
    execute_script_with_context(&script, &mut stack, &[0u8; 32], Some(&ctx))
        .expect("locktime satisfied");
}

#[test]
fn test_checksequenceverify_context_ok() {
    let mut stack = ScriptStack::new();
    stack.push(&[0x01]).expect("push sequence");
    let script = [0xb2u8]; // OP_CHECKSEQUENCEVERIFY
    let ctx = ScriptContext {
        lock_time: 0,
        sequence: 1,
        tx_version: 2,
    };
    execute_script_with_context(&script, &mut stack, &[0u8; 32], Some(&ctx))
        .expect("sequence satisfied");
}
