# Host Function Testing Framework

This template serves as a testing framework for **host functions**, providing two primary functionalities:

1. **Compare Test Vectors**: Validates the VM's behavior against predefined test vectors to ensure correctness.  
2. **Generate Test Vectors**: Automates the creation of test vectors for host functions. *(Note: Some host functions may require manual parameter adjustments.)*

## Table of Contents
1. [Key Features and Structures](#key-features-and-structures)  
2. [Generating Test Vectors](#generating-test-vectors)  
3. [Using Test Vectors for Testing](#using-test-vectors-for-testing)  

## Key Features and Structures

### 1. `ByteSlice`

The `ByteSlice` type prevents JSON serialization from automatically converting binary data into strings, representing binary data as an array of integers.


```go
type ByteSlice []byte
```

---

### 2. Structures with `ForTest`

Structures ending with `ForTest` (e.g., `ServiceAccountForTest`, `RefineMForTest`) address JSON's limitation in representing complex types like `common.Hash`.

#### `PageForTest`

Represents a memory page with a starting address and contents.

```go
type PageForTest struct {
    Start    uint32    `json:"start"`
    Contents ByteSlice `json:"contents"`
}
```

#### `RefineMForTest`

GP-0.5.0 eq B.3  

```go
type RefineMForTest struct {
    P ByteSlice    `json:"P"`
    U *PageForTest `json:"U"`
    I uint32       `json:"I"`
}
```

#### `RefineM_mapForTest`

GP-0.5.0 eq B.5  

```go
type RefineM_mapForTest map[uint32]*RefineMForTest
```

#### `ServiceAccountForTest`

GP-0.5.0 eq 9.3  

```go
type ServiceAccountForTest struct {
    Storage  map[string]ByteSlice `json:"s_map"`
    Lookup   map[string][]uint32  `json:"l_map"`
    Preimage map[string]ByteSlice `json:"p_map"`
    CodeHash string               `json:"code_hash"`
    Balance  uint64               `json:"balance"`
    GasLimitG uint64              `json:"min_item_gas"`
    GasLimitM uint64              `json:"min_memo_gas"`
}
```

#### `PartialStateForTest`

GP-0.5.0 eq 12.13  

```go
type PartialStateForTest struct {
    D                  map[uint32]*ServiceAccountForTest `json:"D"`
    UpcomingValidators types.Validators                  `json:"upcoming_validators"`
    QueueWorkReport    types.AuthorizationQueue          `json:"authorizations_pool"`
    PrivilegedState    types.Kai_state                   `json:"privileged_state"`
}
```

#### `DeferredTransferForTest`

GP-0.5.0 eq 12.14  

```go
type DeferredTransferForTest struct {
    SenderIndex   uint32    `json:"sender_index"`
    ReceiverIndex uint32    `json:"receiver_index"`
    Amount        uint64    `json:"amount"`
    Memo          ByteSlice `json:"memo"`
    GasLimit      uint64    `json:"gas_limit"`
}
```

#### `XContextForTest`

GP-0.5.0 eq B.6  

```go
type XContextForTest struct {
    D map[uint32]*ServiceAccountForTest `json:"D"`
    I uint32                            `json:"I"`
    S uint32                            `json:"S"`
    U *PartialStateForTest              `json:"U"`
    T []DeferredTransferForTest         `json:"T"`
}
```

---

### 3. Test Case Structures

#### `RefineTestcase`

Represents a test case for `refinement` operations.

```go
type RefineTestcase struct {
    Name                    string                `json:"name"`
    InitalGas               uint64                `json:"initial-gas"`
    InitialRegs             []uint32              `json:"initial-regs"`
    InitialMemoryPermission []pvm.PermissionRange `json:"initial-memory-permission"`
    InitialMemory           []PageForTest         `json:"initial-memory"`

    InitialRefineM_map   RefineM_mapForTest `json:"initial-refine-map"`
    InitialExportSegment []ByteSlice        `json:"initial-export-segment"`

    InitialImportSegment    []ByteSlice `json:"initial-import-segment"`
    InitialExportSegmentIdx uint32      `json:"initial-export-segment-index"`

    ExpectedGas    uint64        `json:"expected-gas"`
    ExpectedRegs   []uint32      `json:"expected-regs"`
    ExpectedMemory []PageForTest `json:"expected-memory"`

    ExpectedRefineM_map   RefineM_mapForTest `json:"expected-refine-map"`
    ExpectedExportSegment []ByteSlice        `json:"expected-export-segment"`
}
```

#### `AccumulateTestcase`

Represents a test case for `accumulation` operations.

```go
type AccumulateTestcase struct {
    Name                    string                `json:"name"`
    InitalGas               uint64                `json:"initial-gas"`
    InitialRegs             []uint32              `json:"initial-regs"`
    InitialMemoryPermission []pvm.PermissionRange `json:"initial-memory-permission"`
    InitialMemory           []PageForTest         `json:"initial-memory"`

    InitialXcontent_x *XContextForTest `json:"initial-xcontent-x"`
    InitialXcontent_y XContextForTest  `json:"initial-xcontent-y"`
    InitialTimeslot   uint32           `json:"initial-timeslot"`

    ExpectedGas    uint64        `json:"expected-gas"`
    ExpectedRegs   []uint32      `json:"expected-regs"`
    ExpectedMemory []PageForTest `json:"expected-memory"`

    ExpectedXcontent_x *XContextForTest `json:"expected-xcontent-x"`
    ExpectedXcontent_y XContextForTest  `json:"expected-xcontent-y"`
}
```

#### `GeneralTestcase`

Represents a test case for `general` operations.

```go
type GeneralTestcase struct {
    Name                    string                `json:"name"`
    InitalGas               uint64                `json:"initial-gas"`
    InitialRegs             []uint32              `json:"initial-regs"`
    InitialMemoryPermission []pvm.PermissionRange `json:"initial-memory-permission"`
    InitialMemory           []PageForTest         `json:"initial-memory"`

    InitialServiceAccount ServiceAccountForTest             `json:"initial-service-account"`
    InitialServiceIndex   uint32                            `json:"initial-service-index"`
    InitialDelta          map[uint32]*ServiceAccountForTest `json:"initial-delta"`

    ExpectedGas    uint64        `json:"expected-gas"`
    ExpectedRegs   []uint32      `json:"expected-regs"`
    ExpectedMemory []PageForTest `json:"expected-memory"`

    ExpectedXServiceAccount ServiceAccountForTest `json:"expected-service-account"`
}
```

## Generating Test Vectors

### Steps

1. **Choose the Test Type**: Identify the type of test (`Refine` (GP-0.5.0 B.8), `Accumulate` (GP-0.5.0 B.7), or `General` (GP-0.5.0 B.6)) you want to generate vectors for.  
2. **Define Function and Error Cases**: Set the test cases and their error cases.  
3. **Run the Generator Function**: Execute the corresponding test vector generation function.  
4. **Manually Adjust Test Vectors**: Adjust as needed.

### Example: Generating `Refine` Test Vectors

```go
func TestGenerateRefineTestVectors(t *testing.T) {
    dirPath := "../jamtestvectors/host_function"
    functions := []string{"Import", "Export"}
    errorCases := map[string][]uint32{
        "Import": {pvm.OK, pvm.OOB, pvm.NONE},
        "Export": {pvm.OK, pvm.OOB, pvm.FULL},
    }
    templateFileName := "./Templets/hostRefineTemplet.json"
    testCaseType := "Refine"
    GenerateTestVectors(t, dirPath, functions, errorCases, templateFileName, testCaseType)
}
```

Run:

```bash
make testGenerateRefineTestVectors
```


### Example: Generating `Accumulate` Test Vectors

```go
func TestGenerateAccumulateTestVectors(t *testing.T) {
    dirPath := "../jamtestvectors/host_function"
    functions := []string{"New"}
    errorCases := map[string][]uint32{
        "New":      {pvm.OK, pvm.OOB, pvm.CASH},
        "Solicit":  {pvm.OK, pvm.OOB, pvm.FULL, pvm.HUH},
        "Forget":   {pvm.OK, pvm.OOB, pvm.HUH},
        "Transfer": {pvm.OK, pvm.WHO, pvm.CASH, pvm.LOW, pvm.HIGH},
    }
    templateFileName := "./Templets/hostAccumulateTemplet.json"
    testCaseType := "Accumulate"
    GenerateTestVectors(t, dirPath, functions, errorCases, templateFileName, testCaseType)
}
```

Run:

```bash
make testGenerateAccumulateTestVectors
```

### Example: Manually Adjust `hostImportOK` Test Vectors

```go
func TestGenerateAccumulateTestVectors(t *testing.T) {

    ...

    testcase = RefineTestcase{}
    filename = "hostImportOK.json"
    filePath = filepath.Join(dirPath, filename)

    err = ReadJSONFile(filePath, &testcase)
    if err != nil {
        fmt.Printf("Failed to read test case: %v\n", err)
        return
    }

    testcase.InitialRegs[7] = 0 // the 0th import segment
    testcase.ExpectedRegs[7] = pvm.OK

    testcase.InitialRegs[8] = 4278124544 // 0xFEFF0000
    testcase.ExpectedRegs[8] = 4278124544

    testcase.InitialRegs[9] = 12 // segment length
    testcase.ExpectedRegs[9] = 12

    testcase.InitialMemoryPermission = []pvm.PermissionRange{
        {Start: 4278124544, Length: 12, Mode: 2},
    }

    testcase.InitialMemory = []PageForTest{}
    testcase.ExpectedMemory = []PageForTest{
        {Start: 4278124544, Contents: ByteSlice{10, 10, 11, 11, 12, 12, 13, 13, 14, 14, 15, 15}},
    }

    testcase.InitialRefineM_map = RefineM_mapForTest{}
    testcase.ExpectedRefineM_map = RefineM_mapForTest{}

    testcase.InitialImportSegment = []ByteSlice{
        {10, 10, 11, 11, 12, 12, 13, 13, 14, 14, 15, 15},
    }

    err = WriteJSONFile(filePath, testcase)
    if err != nil {
        fmt.Printf("Failed to write test case: %v\n", err)
        return
    }

    ...

}
```

## Using Test Vectors for Testing

### Example: Running Tests for Refine Vectors

```go
func TestRefine(t *testing.T) {
    node := SetupNodeEnv(t)
    functions := []string{"Import", "Export"}
    for _, name := range functions {
        hostidx, _ := pvm.GetHostFunctionDetails(name)
        dirPath := "../jamtestvectors/host_function"
        files, contents, err := FindAndReadJSONFiles(dirPath, name)
        if err != nil {
            fmt.Println("Error:", err)
            return
        }
        for i, content := range contents {
            targetStateDB := node.getPVMStateDB()
            vm := pvm.NewVMFortest(targetStateDB)
            fmt.Printf("Testing file %s\n", files[i])
            var testcase RefineTestcase
            err := json.Unmarshal([]byte(content), &testcase)
            if err != nil {
                fmt.Printf("Failed to parse JSON for file %s: %v\n", files[i], err)
                continue
            }
            InitPvmBase(vm, testcase)
            InitPvmRefine(vm, testcase)
            vm.InvokeHostCall(hostidx)
            CompareBase(vm, testcase)
            CompareRefine(vm, testcase)
        }
    }
}
```

Run:

```bash
make testRefine
```

### Output Examples

#### Pass Example:

Both `base` and `refine` comparisons pass, meaning all results match the test vector.

```bash
Testing file ../jamtestvectors/host_function/hostImportOK.json
Case hostImportOK base pass
Case hostImportOK refine pass
```

#### Failure Example:

A `refine` comparison failure indicates a mismatch in `RefineM_map` or `Export_segment`.

```bash
Testing file ../jamtestvectors/host_function/hostImportOOB.json
Case hostImportOOB base pass
Case hostImportOOB refine fail
Gas mismatch. Expected: 1000, Got: 900
```