# go-ethereum Recipes 

To test EVM gas usage of Recipes.sol, where our goal is to compare PVM gas usage and EVM gas usage on very similar tests, and explore having EVM work parallel to PVM work.

### Compile Recipes.sol

1. solc --optimize --bin-runtime Recipes.sol 

This will generate hex byte code.  Put this into TestRunRecipeIdx0

### Run the test

2. go test -run=TestRunRecipeIdx0 > recipes.txt

This will run 3 tests and check the result's correctness:

```
% grep Output recipes.txt 
Output: 0000000000000000000000000000000000000000000000000000000000000095 (decimal: 149)
Output: 000000000000000000000000000000000000000000000000000000000000000c (decimal: 12)
Output: 000000000000000000000000000000000000000000000000000000000000088c (decimal: 2188)
```
