use orchard_service::errors::OrchardError;
use orchard_service::transparent_script::{execute_script, ScriptStack};
use serde::Deserialize;

#[derive(Deserialize)]
struct ParityVector {
    name: String,
    script: String,
    initial_stack: Vec<String>,
    expected_stack: Vec<String>,
    expected_error: String,
}

fn map_error(err: Option<OrchardError>) -> String {
    match err {
        None => "ok".to_string(),
        Some(OrchardError::ParseError(msg)) => {
            if msg.contains("OP_RETURN") {
                "op_return".to_string()
            } else if msg.contains("stack underflow") || msg.contains("alt stack underflow") {
                "stack_underflow".to_string()
            } else if msg.contains("unsupported opcode") || msg.contains("unknown opcode") {
                "invalid_opcode".to_string()
            } else if msg.contains("VERIFY") {
                "verify_failed".to_string()
            } else {
                "error".to_string()
            }
        }
        Some(_) => "error".to_string(),
    }
}

#[test]
fn test_transparent_script_parity_vectors() {
    let vectors: Vec<ParityVector> =
        serde_json::from_str(include_str!("../../../test_vectors/transparent_script_parity.json"))
            .expect("parse parity vectors");

    for vector in vectors {
        let script = hex::decode(&vector.script)
            .unwrap_or_else(|err| panic!("vector {} script decode failed: {}", vector.name, err));
        let mut stack = ScriptStack::new();
        for item in &vector.initial_stack {
            let data = hex::decode(item)
                .unwrap_or_else(|err| panic!("vector {} stack decode failed: {}", vector.name, err));
            stack
                .push(&data)
                .unwrap_or_else(|err| panic!("vector {} stack push failed: {}", vector.name, err));
        }

        let result = execute_script(&script, &mut stack, &[0u8; 32]);
        let code = map_error(result.err());
        assert_eq!(
            code, vector.expected_error,
            "vector {} error mismatch",
            vector.name
        );

        if code == "ok" {
            let actual: Vec<String> = stack.items().iter().map(|item| hex::encode(item)).collect();
            assert_eq!(
                actual, vector.expected_stack,
                "vector {} stack mismatch",
                vector.name
            );
        }
    }
}
