.PHONY: coverage_host_test coverage_pvm clean clean_host clean_pvm refine accumulate general pvm

COVERAGE_FILES=coverage1.out coverage2.out coverage3.out
FINAL_COVERAGE=final_coverage.out
HTML_COVERAGE_HOST=coverage_host_test.html
HTML_COVERAGE_PVM=coverage_pvm.html

# Run host function tests and merge coverage reports
coverage_host_test: clean_host
	@clear
	@echo "Running TestRefine (with generation)..."
	@go test -run TestGenerateRefineTestVectors -tags tiny
	@go test -cover -coverprofile=coverage1.out -run TestRefine -tags tiny ./ 
	@echo "Running TestAccumulate (with generation)..."
	@go test -run TestGenerateAccumulateTestVectors -tags tiny
	@go test -cover -coverprofile=coverage2.out -run TestAccumulate -tags tiny ./ 
	@echo "Running TestGeneral (with generation)..."
	@go test -run TestGenerateGeneralTestVectors -tags tiny
	@go test -cover -coverprofile=coverage3.out -run TestGeneral -tags tiny ./ 
	@echo "Merging coverage reports..."
	@cat $(COVERAGE_FILES) | grep -h -v "mode: " > coverage.out
	@echo "mode: set" | cat - coverage.out > $(FINAL_COVERAGE)
	@echo "Generating HTML coverage report for host tests..."
	@go tool cover -html=$(FINAL_COVERAGE) -o $(HTML_COVERAGE_HOST)
	@echo "Coverage report generated: $(HTML_COVERAGE_HOST)"
	@echo "Deleting .out files..."
	@rm -f $(COVERAGE_FILES) coverage.out $(FINAL_COVERAGE)
	@echo "Temporary .out files deleted."

# Run PVM tests and generate coverage report
coverage_pvm: clean_pvm
	@clear
	@echo "Running PVM tests with coverage..."
	@go test -cover -coverprofile=coverage_pvm.out -run TestPVM -tags tiny ./ 
	@echo "Generating HTML coverage report for PVM tests..."
	@go tool cover -html=coverage_pvm.out -o $(HTML_COVERAGE_PVM)
	@echo "Coverage report generated: $(HTML_COVERAGE_PVM)"
	@echo "Deleting PVM .out file..."
	@rm -f coverage_pvm.out
	@echo "Temporary PVM .out file deleted."

# Clean up all coverage files
clean:
	@clear
	@rm -f $(COVERAGE_FILES) coverage.out coverage_pvm.out $(FINAL_COVERAGE) $(HTML_COVERAGE_HOST) $(HTML_COVERAGE_PVM)
	@echo "Cleaned up all coverage files"

# Clean up only host function test coverage files
clean_host:
	@clear
	@rm -f $(COVERAGE_FILES) coverage.out $(FINAL_COVERAGE) $(HTML_COVERAGE_HOST)
	@echo "Cleaned up host function coverage files"

# Clean up only PVM test coverage files
clean_pvm:
	@clear
	@rm -f coverage_pvm.out $(HTML_COVERAGE_PVM)
	@echo "Cleaned up PVM coverage files"

# Run refine test
refine:
	@clear
	@echo "Generating Refine test vectors..."
	@go test -run TestGenerateRefineTestVectors -tags tiny
	@echo "Running Refine host function testing..."
	@go test -run TestRefine -tags tiny

# Run accumulate test
accumulate:
	@clear
	@echo "Generating Accumulate test vectors..."
	@go test -run TestGenerateAccumulateTestVectors -tags tiny
	@echo "Running Accumulate host function testing..."
	@go test -run TestAccumulate -tags tiny

# Run general test
general:
	@clear
	@echo "Generating General test vectors..."
	@go test -run TestGenerateGeneralTestVectors -tags tiny
	@echo "Running General host function testing..."
	@go test -run TestGeneral -tags tiny

# Run PVM test without coverage
pvm:
	@clear
	@echo "Running PVM tests..."
	@go test -run TestPVM -tags tiny
