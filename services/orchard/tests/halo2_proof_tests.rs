use orchard_service::{
    crypto::ORCHARD_VK_BYTES,
    errors::OrchardError,
    vk_registry::VkRegistry,
};

const HALO2_VK_MAGIC: &[u8; 8] = b"H2VKv1\0\0";
const VK_BUNDLE_MAGIC: &[u8; 8] = b"RGVKv1\0\0";

#[test]
fn test_vk_bytes_embedded() {
    assert!(!ORCHARD_VK_BYTES.is_empty(), "Orchard VK bytes must be embedded");
    assert!(
        ORCHARD_VK_BYTES.starts_with(HALO2_VK_MAGIC)
            || ORCHARD_VK_BYTES.starts_with(VK_BUNDLE_MAGIC),
        "Orchard VK bytes must start with a recognized Halo2 header"
    );
}

#[test]
fn test_vk_registry_single_entry() {
    let registry = VkRegistry::new();
    let entry = registry.get_vk_entry(1).expect("vk_id=1 should exist");
    assert_eq!(entry.field_count, 9);

    let err = registry.get_vk_entry(2).expect_err("vk_id=2 should be invalid");
    match err {
        OrchardError::InvalidVkId { vk_id } => assert_eq!(vk_id, 2),
        other => panic!("unexpected error: {other:?}"),
    }
}
