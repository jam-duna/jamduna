# Enhanced Sessions API

This document provides examples and usage patterns for the enhanced sessions API in Go BMT.

## Overview

The enhanced sessions API provides:

- **Transaction semantics** - Begin/Prepare/Commit lifecycle
- **Root generation** - Access state roots at every stage
- **Witness generation** - Cryptographic proofs for state transitions
- **Snapshot isolation** - Consistent reads during writes
- **Optimistic concurrency** - Stale transaction detection
- **Overlay composition** - Build complex transaction chains

## API Comparison

### Simple API (Backward Compatible)

```go
// Direct database operations
db.Insert(key, value)
db.Delete(key)
value, _ := db.Get(key)
root := db.Root()
session, _ := db.Commit()
```

### Enhanced Session API

```go
// Session-based operations with full control
session, _ := db.BeginWrite()
session.Insert(key, value)
sessionRoot := session.Root()
prepared, _ := session.Prepare()
newRoot := prepared.Root()
prepared.Commit()
```

## Usage Patterns

### Pattern 1: Simple Transaction

Basic transactional writes with prepare/commit:

```go
func SimpleTransaction(db *bmt.Nomt) error {
    // Begin write session
    session, err := db.BeginWrite()
    if err != nil {
        return err
    }

    // Make changes
    key1 := [32]byte{1}
    key2 := [32]byte{2}
    key3 := [32]byte{3}

    session.Insert(key1, []byte("value1"))
    session.Insert(key2, []byte("value2"))
    session.Delete(key3)

    // Prepare freezes the session and computes final state
    prepared, err := session.Prepare()
    if err != nil {
        return err
    }

    // Check the new root before committing
    newRoot := prepared.Root()
    fmt.Printf("New root: %x\n", newRoot)

    // Commit to database
    return prepared.Commit()
}
```

**Use case**: Atomic batch updates with root verification before commit.

### Pattern 2: Transaction with Witness

Generate cryptographic proofs for state transitions:

```go
func TransactionWithWitness(db *bmt.Nomt) (*bmt.Witness, error) {
    // Begin write session with witness tracking enabled
    session, err := db.BeginWriteWithWitness()
    if err != nil {
        return nil, err
    }

    // Perform operations (all tracked for witness)
    key := [32]byte{1}
    session.Insert(key, []byte("value"))

    // Read operations are also tracked in witness mode
    value, _ := session.Get(key)
    fmt.Printf("Read back: %s\n", value)

    // Prepare session
    prepared, err := session.Prepare()
    if err != nil {
        return nil, err
    }

    // Generate witness before commit
    witness, err := prepared.Witness()
    if err != nil {
        return nil, err
    }

    fmt.Printf("Witness contains %d keys\n", len(witness.Keys))
    fmt.Printf("State transition: %x -> %x\n",
        witness.PrevRoot, witness.Root)

    // Commit the transaction
    if err := prepared.Commit(); err != nil {
        return nil, err
    }

    return witness, nil
}
```

**Use case**: Blockchain state transitions requiring verifiable proofs.

### Pattern 3: Multi-Session Overlay Building

Build complex transaction chains with overlays:

```go
func OverlayWorkflow(db *bmt.Nomt) error {
    // Session 1: Create base overlay
    session1, err := db.BeginWrite()
    if err != nil {
        return err
    }

    key1 := [32]byte{1}
    session1.Insert(key1, []byte("base"))

    prepared1, err := session1.Prepare()
    if err != nil {
        return err
    }

    // Convert to overlay without committing
    overlay1 := prepared1.ToOverlay()

    // Session 2: Build on overlay1 (layered changes)
    session2, err := db.BeginWriteWithOverlay([]*overlay.Overlay{overlay1})
    if err != nil {
        return err
    }

    key2 := [32]byte{2}
    session2.Insert(key2, []byte("derived"))

    prepared2, err := session2.Prepare()
    if err != nil {
        return err
    }

    // Commit both sessions atomically
    return prepared2.Commit()
}
```

**Use case**: Speculative execution, transaction simulation, or staging complex state changes.

### Pattern 4: Read-Only Snapshot

Consistent reads with snapshot isolation:

```go
func ReadOnlyOperations(db *bmt.Nomt) error {
    // Create read session (snapshot at current state)
    session, err := db.BeginRead()
    if err != nil {
        return err
    }
    defer session.Close()

    // All reads see consistent snapshot
    key1 := [32]byte{1}
    key2 := [32]byte{2}

    value1, _ := session.Get(key1)
    value2, _ := session.Get(key2)

    // Snapshot root
    fmt.Printf("Reading from root: %x\n", session.Root())
    fmt.Printf("Values: %s, %s\n", value1, value2)

    return nil
}
```

**Use case**: Long-running queries or analytics that need consistent view of data.

### Pattern 5: Root Generation Workflow

Track roots at every stage of transaction lifecycle:

```go
func RootGenerationExample(db *bmt.Nomt) ([32]byte, error) {
    // 1. Get current root before changes
    prevRoot := db.Root()
    fmt.Printf("Previous root: %x\n", prevRoot)

    // 2. Create write session
    session, err := db.BeginWrite()
    if err != nil {
        return [32]byte{}, err
    }

    // 3. Make changes
    key1 := [32]byte{1}
    key2 := [32]byte{2}
    session.Insert(key1, []byte("new_value"))
    session.Delete(key2)

    // 4. Check intermediate root (before prepare)
    intermediateRoot := session.Root()
    fmt.Printf("Intermediate root: %x\n", intermediateRoot)

    // 5. Prepare session (freezes state and computes final root)
    prepared, err := session.Prepare()
    if err != nil {
        return [32]byte{}, err
    }

    // 6. Get final root before committing
    newRoot := prepared.Root()
    preparedPrevRoot := prepared.PrevRoot()
    fmt.Printf("New root (before commit): %x\n", newRoot)
    fmt.Printf("Prev root (from prepared): %x\n", preparedPrevRoot)

    // Verify roots match expectations
    if preparedPrevRoot != prevRoot {
        return [32]byte{}, fmt.Errorf("root mismatch")
    }

    // 7. Commit and verify root in database
    if err := prepared.Commit(); err != nil {
        return [32]byte{}, err
    }

    committedRoot := db.Root()
    fmt.Printf("Committed root: %x\n", committedRoot)

    return newRoot, nil
}
```

**Use case**: State root tracking for blockchain consensus or debugging.

### Pattern 6: Read at Specific Root

Read historical state at a specific root:

```go
func ReadAtHistoricalRoot(db *bmt.Nomt, historicalRoot [32]byte) error {
    // Create read session at historical root
    session, err := db.BeginReadAt(historicalRoot)
    if err != nil {
        return err
    }
    defer session.Close()

    // Read from historical state
    key := [32]byte{1}
    value, err := session.Get(key)
    if err != nil {
        return err
    }

    fmt.Printf("Value at root %x: %s\n",
        session.Root(), value)

    return nil
}
```

**Use case**: Historical queries or time-travel debugging.

## Advanced Patterns

### Optimistic Concurrency Control

The enhanced API detects stale transactions:

```go
func OptimisticConcurrency(db *bmt.Nomt) error {
    // Session 1
    session1, _ := db.BeginWrite()
    session1.Insert([32]byte{1}, []byte("value1"))
    prepared1, _ := session1.Prepare()

    // Session 2 (concurrent)
    session2, _ := db.BeginWrite()
    session2.Insert([32]byte{2}, []byte("value2"))
    prepared2, _ := session2.Prepare()

    // First commit succeeds
    if err := prepared1.Commit(); err != nil {
        return err
    }

    // Second commit fails with stale transaction error
    err := prepared2.Commit()
    if err != nil {
        fmt.Printf("Expected stale transaction: %v\n", err)
        // Retry logic here
        return nil
    }

    return fmt.Errorf("should have detected stale transaction")
}
```

### Conditional Commit Based on Root

Only commit if state hasn't changed:

```go
func ConditionalCommit(db *bmt.Nomt, expectedPrevRoot [32]byte) error {
    session, err := db.BeginWrite()
    if err != nil {
        return err
    }

    // Make changes
    session.Insert([32]byte{1}, []byte("value"))

    prepared, err := session.Prepare()
    if err != nil {
        return err
    }

    // Check if previous root matches expectation
    if prepared.PrevRoot() != expectedPrevRoot {
        prepared.Rollback()
        return fmt.Errorf("state changed, aborting commit")
    }

    return prepared.Commit()
}
```

### Transaction Rollback

Explicitly rollback a prepared session:

```go
func TransactionRollback(db *bmt.Nomt) error {
    session, err := db.BeginWrite()
    if err != nil {
        return err
    }

    session.Insert([32]byte{1}, []byte("value"))

    prepared, err := session.Prepare()
    if err != nil {
        return err
    }

    // Check some condition
    newRoot := prepared.Root()
    if !isValidRoot(newRoot) {
        // Rollback instead of committing
        prepared.Rollback()
        return fmt.Errorf("invalid root, transaction rolled back")
    }

    return prepared.Commit()
}

func isValidRoot(root [32]byte) bool {
    // Custom validation logic
    return true
}
```

## Session Lifecycle

### WriteSession

1. **Created** - `BeginWrite()` or `BeginWriteWithWitness()`
2. **Active** - Accept Insert/Delete/Get operations
3. **Prepared** - `Prepare()` freezes session (no more operations allowed)
4. **Committed** - `PreparedSession.Commit()` applies to database
5. **Closed** - Session cannot be reused

### ReadSession

1. **Created** - `BeginRead()` or `BeginReadAt(root)`
2. **Active** - Get operations see consistent snapshot
3. **Closed** - `Close()` releases resources

### PreparedSession

1. **Created** - From `WriteSession.Prepare()`
2. **Ready** - Can inspect roots, generate witness
3. **Committed** - `Commit()` applies changes
4. **Rolled Back** - `Rollback()` discards changes

## Error Handling

```go
func RobustTransaction(db *bmt.Nomt) error {
    session, err := db.BeginWrite()
    if err != nil {
        return fmt.Errorf("begin failed: %w", err)
    }

    // Make changes
    if err := session.Insert([32]byte{1}, []byte("value")); err != nil {
        return fmt.Errorf("insert failed: %w", err)
    }

    // Prepare
    prepared, err := session.Prepare()
    if err != nil {
        return fmt.Errorf("prepare failed: %w", err)
    }

    // Commit with retry on stale transaction
    if err := prepared.Commit(); err != nil {
        if isStaleTransaction(err) {
            // Retry logic
            return retryTransaction(db)
        }
        return fmt.Errorf("commit failed: %w", err)
    }

    return nil
}

func isStaleTransaction(err error) bool {
    return err != nil &&
           (err.Error() == "stale transaction" ||
            strings.Contains(err.Error(), "root changed"))
}

func retryTransaction(db *bmt.Nomt) error {
    // Implement retry logic
    return nil
}
```

## Performance Considerations

### When to Use Simple API vs Enhanced Sessions

**Simple API** - Use when:
- Making single operations
- Don't need transaction semantics
- Don't need root tracking
- Maximum simplicity desired

**Enhanced Sessions** - Use when:
- Batch operations need atomicity
- Need to verify roots before commit
- Generating witnesses for verification
- Building speculative overlays
- Need snapshot isolation for reads
- Implementing optimistic concurrency control

### Memory Usage

- Each `WriteSession` holds an in-memory overlay
- `ReadSession` holds references to overlay chain
- `PreparedSession` freezes overlay state
- Clean up sessions when done to release memory

## Integration Examples

### Blockchain Block Processing

```go
func ProcessBlock(db *bmt.Nomt, block *Block) error {
    // Begin transaction for entire block
    session, err := db.BeginWriteWithWitness()
    if err != nil {
        return err
    }

    // Process all transactions in block
    for _, tx := range block.Transactions {
        if err := applyTransaction(session, tx); err != nil {
            return err
        }
    }

    // Prepare and check state root
    prepared, err := session.Prepare()
    if err != nil {
        return err
    }

    newRoot := prepared.Root()
    if newRoot != block.StateRoot {
        prepared.Rollback()
        return fmt.Errorf("state root mismatch")
    }

    // Generate witness for validators
    witness, err := prepared.Witness()
    if err != nil {
        return err
    }

    // Commit block
    if err := prepared.Commit(); err != nil {
        return err
    }

    publishWitness(witness)
    return nil
}

func applyTransaction(session *bmt.WriteSession, tx *Transaction) error {
    // Apply transaction operations
    return nil
}

func publishWitness(witness *bmt.Witness) {
    // Publish to network
}
```

### Database Migration

```go
func MigrateData(db *bmt.Nomt, migrations []Migration) error {
    for i, migration := range migrations {
        session, err := db.BeginWrite()
        if err != nil {
            return err
        }

        // Apply migration
        if err := migration.Apply(session); err != nil {
            return fmt.Errorf("migration %d failed: %w", i, err)
        }

        prepared, err := session.Prepare()
        if err != nil {
            return err
        }

        fmt.Printf("Migration %d: %x -> %x\n",
            i, prepared.PrevRoot(), prepared.Root())

        if err := prepared.Commit(); err != nil {
            return err
        }
    }

    return nil
}

type Migration interface {
    Apply(session *bmt.WriteSession) error
}
```

## See Also

- [API-NEXTSTEPS.md](API-NEXTSTEPS.md) - Detailed API design comparison
- [nomt.go](nomt.go) - Simple API implementation
- [sessions_enhanced.go](sessions_enhanced.go) - Enhanced sessions implementation
- [sessions_enhanced_test.go](sessions_enhanced_test.go) - Comprehensive test examples
