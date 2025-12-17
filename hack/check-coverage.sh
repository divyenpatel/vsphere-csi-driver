#!/bin/bash

# Copyright 2025 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# This script calculates and displays the unit test coverage for the pkg directory.

set -o errexit
set -o nounset
set -o pipefail

# Get the root directory of the project
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Default values
COVERAGE_FILE="${ROOT_DIR}/coverage.out"
HTML_REPORT="${ROOT_DIR}/coverage.html"
MIN_COVERAGE=${MIN_COVERAGE:-0}  # Minimum required coverage percentage (default: 0, meaning no minimum)
VERBOSE=${VERBOSE:-false}
GENERATE_HTML=${GENERATE_HTML:-false}
SHOW_UNCOVERED=${SHOW_UNCOVERED:-false}

# Function to display usage
usage() {
    echo "Usage: $0 [OPTIONS]"
    echo ""
    echo "Options:"
    echo "  -m, --min-coverage <percent>  Set minimum required coverage percentage (default: 0)"
    echo "  -v, --verbose                 Show verbose output including per-package coverage"
    echo "  -h, --html                    Generate HTML coverage report"
    echo "  -u, --uncovered               Show files with no test coverage"
    echo "  --help                        Display this help message"
    echo ""
    echo "Environment Variables:"
    echo "  MIN_COVERAGE      Minimum required coverage percentage"
    echo "  VERBOSE           Set to 'true' for verbose output"
    echo "  GENERATE_HTML     Set to 'true' to generate HTML report"
    echo "  SHOW_UNCOVERED    Set to 'true' to show uncovered files"
    echo ""
    echo "Examples:"
    echo "  $0                           # Run with defaults"
    echo "  $0 -m 50                     # Require at least 50% coverage"
    echo "  $0 -v -h                     # Verbose output with HTML report"
    echo "  $0 --min-coverage 30 -v -u   # 30% min coverage, verbose, show uncovered"
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -m|--min-coverage)
            MIN_COVERAGE="$2"
            shift 2
            ;;
        -v|--verbose)
            VERBOSE=true
            shift
            ;;
        -h|--html)
            GENERATE_HTML=true
            shift
            ;;
        -u|--uncovered)
            SHOW_UNCOVERED=true
            shift
            ;;
        --help)
            usage
            exit 0
            ;;
        *)
            echo -e "${RED}Unknown option: $1${NC}"
            usage
            exit 1
            ;;
    esac
done

echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}  vSphere CSI Driver - Coverage Report${NC}"
echo -e "${BLUE}========================================${NC}"
echo ""

cd "${ROOT_DIR}"

# Clean up any existing coverage file
rm -f "${COVERAGE_FILE}" "${HTML_REPORT}"

echo -e "${YELLOW}Running tests with coverage for pkg/...${NC}"
echo ""

# Run tests with coverage for the pkg directory
# Using -covermode=atomic for accurate coverage with concurrent tests
if ! go test -coverprofile="${COVERAGE_FILE}" -covermode=atomic ./pkg/... 2>&1; then
    echo -e "${RED}Some tests failed. Coverage report may be incomplete.${NC}"
fi

echo ""

# Check if coverage file was generated
if [[ ! -f "${COVERAGE_FILE}" ]]; then
    echo -e "${RED}Error: Coverage file was not generated.${NC}"
    exit 1
fi

# Calculate total coverage
TOTAL_COVERAGE=$(go tool cover -func="${COVERAGE_FILE}" | grep "total:" | awk '{print $3}' | sed 's/%//')

echo -e "${BLUE}----------------------------------------${NC}"
echo -e "${GREEN}Total Coverage: ${TOTAL_COVERAGE}%${NC}"
echo -e "${BLUE}----------------------------------------${NC}"
echo ""

# Show per-package coverage if verbose
if [[ "${VERBOSE}" == "true" ]]; then
    echo -e "${YELLOW}Per-Package Coverage:${NC}"
    echo ""
    
    # Get unique packages and their coverage
    go tool cover -func="${COVERAGE_FILE}" | grep -v "total:" | while read line; do
        file=$(echo "$line" | awk '{print $1}')
        func=$(echo "$line" | awk '{print $2}')
        coverage=$(echo "$line" | awk '{print $3}')
        
        # Color code based on coverage
        coverage_num=$(echo "$coverage" | sed 's/%//')
        if (( $(echo "$coverage_num >= 80" | bc -l) )); then
            color="${GREEN}"
        elif (( $(echo "$coverage_num >= 50" | bc -l) )); then
            color="${YELLOW}"
        else
            color="${RED}"
        fi
        
        printf "  %-60s %-30s ${color}%s${NC}\n" "$file" "$func" "$coverage"
    done
    
    echo ""
fi

# Show summary by package
echo -e "${YELLOW}Coverage Summary by Package:${NC}"
echo ""

# Extract unique packages and calculate their coverage
declare -A pkg_statements
declare -A pkg_covered

while IFS= read -r line; do
    if [[ "$line" == "mode:"* ]] || [[ -z "$line" ]]; then
        continue
    fi
    
    # Parse coverage line: file:start.col,end.col statements count
    file=$(echo "$line" | cut -d: -f1)
    rest=$(echo "$line" | cut -d: -f2)
    statements=$(echo "$rest" | awk '{print $2}')
    count=$(echo "$rest" | awk '{print $3}')
    
    # Extract package path
    pkg=$(dirname "$file" | sed 's|sigs.k8s.io/vsphere-csi-driver/v3/||')
    
    # Accumulate statements and covered statements
    current_statements=${pkg_statements[$pkg]:-0}
    current_covered=${pkg_covered[$pkg]:-0}
    
    pkg_statements[$pkg]=$((current_statements + statements))
    if [[ "$count" -gt 0 ]]; then
        pkg_covered[$pkg]=$((current_covered + statements))
    fi
done < "${COVERAGE_FILE}"

# Print package summary
printf "  %-50s %10s %10s %10s\n" "Package" "Stmts" "Covered" "Coverage"
printf "  %-50s %10s %10s %10s\n" "-------" "-----" "-------" "--------"

for pkg in $(echo "${!pkg_statements[@]}" | tr ' ' '\n' | sort); do
    stmts=${pkg_statements[$pkg]}
    covered=${pkg_covered[$pkg]}
    
    if [[ $stmts -gt 0 ]]; then
        pct=$(echo "scale=1; $covered * 100 / $stmts" | bc)
    else
        pct="0.0"
    fi
    
    # Color code based on coverage
    if (( $(echo "$pct >= 80" | bc -l) )); then
        color="${GREEN}"
    elif (( $(echo "$pct >= 50" | bc -l) )); then
        color="${YELLOW}"
    else
        color="${RED}"
    fi
    
    printf "  %-50s %10d %10d ${color}%9s%%${NC}\n" "$pkg" "$stmts" "$covered" "$pct"
done

echo ""

# Show uncovered files if requested
if [[ "${SHOW_UNCOVERED}" == "true" ]]; then
    echo -e "${YELLOW}Files with 0% coverage:${NC}"
    echo ""
    
    go tool cover -func="${COVERAGE_FILE}" | grep "0.0%" | while read line; do
        file=$(echo "$line" | awk '{print $1}')
        echo -e "  ${RED}${file}${NC}"
    done
    
    echo ""
fi

# Generate HTML report if requested
if [[ "${GENERATE_HTML}" == "true" ]]; then
    echo -e "${YELLOW}Generating HTML coverage report...${NC}"
    go tool cover -html="${COVERAGE_FILE}" -o "${HTML_REPORT}"
    echo -e "${GREEN}HTML report generated: ${HTML_REPORT}${NC}"
    echo ""
fi

# Check minimum coverage requirement
if [[ "${MIN_COVERAGE}" -gt 0 ]]; then
    COVERAGE_INT=$(echo "$TOTAL_COVERAGE" | cut -d. -f1)
    
    if [[ "${COVERAGE_INT}" -lt "${MIN_COVERAGE}" ]]; then
        echo -e "${RED}ERROR: Coverage ${TOTAL_COVERAGE}% is below minimum required ${MIN_COVERAGE}%${NC}"
        exit 1
    else
        echo -e "${GREEN}SUCCESS: Coverage ${TOTAL_COVERAGE}% meets minimum required ${MIN_COVERAGE}%${NC}"
    fi
fi

echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}  Coverage analysis complete!${NC}"
echo -e "${BLUE}========================================${NC}"

# Clean up coverage file unless HTML was generated
if [[ "${GENERATE_HTML}" != "true" ]]; then
    rm -f "${COVERAGE_FILE}"
fi

exit 0

